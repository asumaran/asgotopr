package main

// Git helpers. The read-only discovery paths (git dir, origin URL, GitHub
// slug) read the filesystem directly with no subprocess, so scanning ~30
// repos at startup stays in the low milliseconds. Anything that mutates or
// needs porcelain output (worktree list, status, fetch, switch, stash) shells
// out to git through the family's `runGit` (gitrun.go).

import (
	"context"
	"fmt"
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
	out, err := gitIn(repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktreePorcelain(out), nil
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
	out, err := gitIn(path, "status", "--porcelain")
	return err == nil && strings.TrimSpace(out) != ""
}

// branchExists reports whether the short branch name exists locally.
func branchExists(path, branch string) bool {
	return gitDo(path, "show-ref", "--verify", "--quiet", "refs/heads/"+branch) == nil
}

// gitIn runs git in dir through the family's runGit (gitrun.go), which
// reports git's own stderr, and names the command in the error.
func gitIn(dir string, args ...string) (string, error) {
	out, err := runGit(context.Background(), append([]string{"-C", dir}, args...)...)
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// gitDo is gitIn for the commands run for their effect.
func gitDo(dir string, args ...string) error {
	_, err := gitIn(dir, args...)
	return err
}
