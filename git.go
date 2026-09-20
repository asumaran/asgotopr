package main

// Git helpers. The read-only discovery paths (git dir, origin URL, GitHub
// slug) read the filesystem directly with no subprocess, so scanning ~30
// repos at startup stays in the low milliseconds. Anything that mutates or
// needs porcelain output (worktree list, status, fetch, switch, stash) shells
// out to git.

import (
	"fmt"
	"os/exec"
	"strings"
)

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
