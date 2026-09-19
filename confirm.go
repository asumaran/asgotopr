package main

// The switch/stash flow that runs when a PR's branch is not checked out
// anywhere: modeConfirmStash asks what to do with a dirty tree, modeBusy runs
// the git work as a tea.Cmd, modeError surfaces git failures inside the TUI
// (after quit the popup is gone, so there is nowhere else to show them).

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type stashedMsg struct{ err error }

type switchedMsg struct{ err error }

func stashCmd(repo localRepo, branch string) tea.Cmd {
	return func() tea.Msg {
		err := runGit(repo.Path, "stash", "push", "-u", "-m", "asgotopr: switching to "+branch)
		return stashedMsg{err: err}
	}
}

// performSwitchCmd makes the PR's branch current in the main clone: fetch it
// if needed (via the pull/N/head ref, which covers fork PRs and never-fetched
// branches alike), then switch.
func performSwitchCmd(e *entry) tea.Cmd {
	repo, pr := e.repo, e.pr
	return func() tea.Msg {
		if !branchExists(repo.Path, pr.HeadRefName) {
			if err := runGit(repo.Path, "fetch", "origin",
				fmt.Sprintf("pull/%d/head:%s", pr.Number, pr.HeadRefName)); err != nil {
				return switchedMsg{err: err}
			}
		} else if !pr.IsCrossRepo {
			// Best-effort update of an existing branch; offline still works.
			_ = runGit(repo.Path, "fetch", "origin", pr.HeadRefName)
		}
		return switchedMsg{err: runGit(repo.Path, "switch", pr.HeadRefName)}
	}
}

// confirmView renders the stash prompt shown in place of the preview column.
func confirmView(e *entry, width int) string {
	lines := []string{
		"",
		stTitle.Render(truncate("Uncommitted changes", width)),
		"",
		truncate("Working tree in "+homeRel(e.repo.Path), width),
		truncate("has uncommitted changes.", width),
		"",
		truncate("Stash and switch to "+e.pr.HeadRefName+"?", width),
		"",
		stKeyHint.Render("[s]") + " stash & switch",
		stKeyHint.Render("[f]") + " switch anyway",
		stKeyHint.Render("[esc]") + " cancel",
	}
	return strings.Join(lines, "\n")
}

// errorView renders a git failure in place of the preview column.
func errorView(msg string, width int) string {
	var lines []string
	lines = append(lines, "", stError.Render("Error"), "")
	for _, l := range wrapPlain(msg, width) {
		lines = append(lines, l)
	}
	lines = append(lines, "", stKeyHint.Render("[esc]")+" back")
	return strings.Join(lines, "\n")
}

// busyView renders the in-flight state in place of the preview column.
func busyView(msg string, width int) string {
	return "\n" + stDim.Render(truncate(msg, width))
}

// wrapPlain hard-wraps plain (non-ANSI) text to width, preserving words when
// possible.
func wrapPlain(s string, width int) []string {
	if width < 8 {
		width = 8
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		cur := ""
		for _, w := range words {
			switch {
			case cur == "":
				cur = w
			case len(cur)+1+len(w) <= width:
				cur += " " + w
			default:
				lines = append(lines, cur)
				cur = w
			}
		}
		lines = append(lines, cur)
	}
	return lines
}
