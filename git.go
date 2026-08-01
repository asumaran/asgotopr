package main

// Git helpers. The read-only discovery paths (git dir, origin URL, GitHub
// slug) read the filesystem directly with no subprocess, so scanning ~30
// repos at startup stays in the low milliseconds. Anything that mutates or
// needs porcelain output (worktree list, status, fetch, switch, stash) shells
// out to git.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// resolveGitDir returns the git dir of the checkout at path, handling both
// main checkouts (.git dir) and linked worktrees (.git file with a "gitdir:"
// pointer). Returns "" for non-repos.
func resolveGitDir(path string) string {
	if path == "" {
		return ""
	}
	gitdir := filepath.Join(path, ".git")
	if fi, err := os.Stat(gitdir); err != nil {
		return ""
	} else if !fi.IsDir() {
		data, err := os.ReadFile(gitdir)
		if err != nil {
			return ""
		}
		target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir:"))
		if target == "" {
			return ""
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(path, target)
		}
		gitdir = target
	}
	return gitdir
}

// githubSlug resolves "owner/repo" from the origin remote of the repo at
// repoRoot, reading the git config file directly. Returns "" when the origin
// is missing or not on github.com.
func githubSlug(repoRoot string) string {
	gitdir := resolveGitDir(repoRoot)
	if gitdir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(gitdir, "config"))
	if err != nil {
		// A linked-worktree gitdir keeps config in the common dir.
		common, cerr := os.ReadFile(filepath.Join(gitdir, "commondir"))
		if cerr != nil {
			return ""
		}
		target := strings.TrimSpace(string(common))
		if !filepath.IsAbs(target) {
			target = filepath.Join(gitdir, target)
		}
		if data, err = os.ReadFile(filepath.Join(target, "config")); err != nil {
			return ""
		}
	}
	return githubSlugFromURL(originURL(string(data)))
}

// originURL scans git config content for the url of [remote "origin"].
func originURL(config string) string {
	inOrigin := false
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inOrigin = line == `[remote "origin"]`
			continue
		}
		if !inOrigin {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "url"); ok {
			rest = strings.TrimSpace(rest)
			if v, ok := strings.CutPrefix(rest, "="); ok {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// githubSlugFromURL extracts "owner/repo" from the ssh/https GitHub remote
// URL forms. Non-GitHub hosts return "".
func githubSlugFromURL(url string) string {
	var rest string
	switch {
	case strings.HasPrefix(url, "git@github.com:"):
		rest = strings.TrimPrefix(url, "git@github.com:")
	case strings.HasPrefix(url, "ssh://git@github.com/"):
		rest = strings.TrimPrefix(url, "ssh://git@github.com/")
	case strings.HasPrefix(url, "https://github.com/"):
		rest = strings.TrimPrefix(url, "https://github.com/")
	default:
		return ""
	}
	rest = strings.TrimSuffix(strings.TrimSuffix(rest, "/"), ".git")
	if strings.Count(rest, "/") != 1 {
		return ""
	}
	return rest
}

// ---- worktree detection ----

type wtEntry struct {
	Path     string
	Branch   string // short name; "" when detached or bare
	Detached bool
	Bare     bool
}

// parseWorktreePorcelain parses `git worktree list --porcelain` output.
// Entries are blank-line separated blocks of "worktree <path>" followed by
// attribute lines (HEAD, branch, detached, bare, locked, prunable).
func parseWorktreePorcelain(out string) []wtEntry {
	var entries []wtEntry
	var cur *wtEntry
	flush := func() {
		if cur != nil {
			entries = append(entries, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &wtEntry{Path: strings.TrimPrefix(line, "worktree ")}
		case cur == nil:
			// Attribute line before any worktree header; ignore.
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			cur.Detached = true
		case line == "bare":
			cur.Bare = true
		}
	}
	flush()
	return entries
}

// listWorktrees returns the worktrees of the repo at repoPath (the main
// checkout is included as the first entry).
func listWorktrees(repoPath string) ([]wtEntry, error) {
	out, err := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, gitErr("worktree list", err)
	}
	return parseWorktreePorcelain(string(out)), nil
}

// worktreeForBranch returns the checkout (main or linked) where branch is
// currently checked out, or nil when none.
func worktreeForBranch(repoPath, branch string) *wtEntry {
	entries, err := listWorktrees(repoPath)
	if err != nil {
		return nil
	}
	for i := range entries {
		if entries[i].Branch == branch {
			return &entries[i]
		}
	}
	return nil
}

// isDirty reports whether the working tree at path has uncommitted changes
// (staged, unstaged or untracked).
func isDirty(path string) bool {
	out, err := exec.Command("git", "-C", path, "status", "--porcelain").Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// branchExists reports whether the short branch name exists locally.
func branchExists(path, branch string) bool {
	err := exec.Command("git", "-C", path, "show-ref", "--verify", "--quiet", "refs/heads/"+branch).Run()
	return err == nil
}

// gitErr wraps a git subprocess failure, surfacing stderr when available
// (exec.ExitError only carries it when captured via Output()).
func gitErr(op string, err error) error {
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		return fmt.Errorf("git %s: %s", op, strings.TrimSpace(string(ee.Stderr)))
	}
	return fmt.Errorf("git %s: %w", op, err)
}

// runGit runs a git command in dir and returns a descriptive error on
// failure (stderr included).
func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}
