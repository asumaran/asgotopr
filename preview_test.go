package main

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestSummarizeChecks(t *testing.T) {
	ctxs := []checkContext{
		{Typename: "CheckRun", Name: "lint", Status: "COMPLETED", Conclusion: "SUCCESS"},
		{Typename: "CheckRun", Name: "test", Status: "COMPLETED", Conclusion: "FAILURE"},
		{Typename: "CheckRun", Name: "test", Status: "COMPLETED", Conclusion: "CANCELLED"}, // re-run, same name
		{Typename: "CheckRun", Name: "build", Status: "IN_PROGRESS"},
		{Typename: "CheckRun", Name: "docs", Status: "COMPLETED", Conclusion: "SKIPPED"},
		{Typename: "StatusContext", Context: "ci/legacy", State: "ERROR"},
		{Typename: "StatusContext", Context: "ci/ok", State: "SUCCESS"},
		{Typename: "StatusContext", Context: "ci/wait", State: "PENDING"},
	}
	cs := summarizeChecks("FAILURE", 8, ctxs)
	want := checkSummary{State: "FAILURE", Total: 8, Passed: 2, Failed: 3, Pending: 2, Skipped: 1,
		FailedNames: []string{"test", "ci/legacy"}}
	if cs.State != want.State || cs.Total != want.Total || cs.Passed != want.Passed ||
		cs.Failed != want.Failed || cs.Pending != want.Pending || cs.Skipped != want.Skipped ||
		strings.Join(cs.FailedNames, ",") != strings.Join(want.FailedNames, ",") {
		t.Errorf("summarizeChecks = %+v, want %+v", cs, want)
	}
}

func headerPR() prItem {
	return prItem{
		URL: "u", Number: 42, Title: "add thing", HeadRefName: "feat/thing", BaseRefName: "main",
		Author: "me", IsDraft: true, Mergeable: "CONFLICTING",
		CreatedAt: time.Now().Add(-48 * time.Hour), UpdatedAt: time.Now().Add(-time.Hour),
		Labels:    []prLabel{{Name: "bug", Color: "d73a4a"}, {Name: "frontend", Color: "zzz"}},
		Additions: 10, Deletions: 3, ChangedFiles: 2, Commits: 4, Comments: 5,
		ReviewDecision: "CHANGES_REQUESTED", Approvals: 1, ReviewRequests: 2,
		Checks: checkSummary{State: "FAILURE", Total: 3, Passed: 1, Failed: 1, Pending: 1, FailedNames: []string{"test"}},
	}
}

func TestPreviewHeaderFacts(t *testing.T) {
	h := ansi.Strip(previewHeader(headerPR(), 80))
	for _, want := range []string{
		"add thing", "#42 · feat/thing → main", "DRAFT", "CONFLICTS",
		"bug", "frontend",
		"✗ 1 failed", "● 1 pending", "✓ 1 passed", "↳ test",
		"✗ changes requested", "1 approval", "2 reviewers pending", "5 comments",
		"+10 −3", "2 files", "4 commits",
		"2d ago by me", "updated 1h ago",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("header missing %q:\n%s", want, h)
		}
	}
	for i, line := range strings.Split(previewHeader(headerPR(), 30), "\n") {
		if w := ansi.StringWidth(line); w > 30 {
			t.Errorf("header line %d wider than column (%d): %q", i, w, ansi.Strip(line))
		}
	}
}

func TestPreviewHeaderMinimal(t *testing.T) {
	// A PR from an older cache has none of the extra fields.
	pr := prItem{URL: "u", Number: 1, Title: "old", HeadRefName: "b", UpdatedAt: time.Now()}
	h := ansi.Strip(previewHeader(pr, 60))
	for _, want := range []string{"#1 · b", "Checks  none", "Review  not reviewed", "Opened  updated just now"} {
		if !strings.Contains(h, want) {
			t.Errorf("minimal header missing %q:\n%s", want, h)
		}
	}
	for _, bad := range []string{"→", "DRAFT", "Diff", "by "} {
		if strings.Contains(h, bad) {
			t.Errorf("minimal header should not contain %q:\n%s", bad, h)
		}
	}
}

func TestWrapChips(t *testing.T) {
	chips := []string{"aaaa", "bbbb", "cccc"}
	got := wrapChips(chips, 9) // "aaaa bbbb" fits, "cccc" wraps
	if len(got) != 2 || got[0] != "aaaa bbbb" || got[1] != "cccc" {
		t.Errorf("wrapChips = %q", got)
	}
}

func TestChipForeground(t *testing.T) {
	if fg, ok := chipForeground("fbca04"); !ok || fg != "#000000" {
		t.Errorf("light background should get black text, got %q %v", fg, ok)
	}
	if fg, ok := chipForeground("6842f2"); !ok || fg != "#ffffff" {
		t.Errorf("dark background should get white text, got %q %v", fg, ok)
	}
	if _, ok := chipForeground("nope"); ok {
		t.Errorf("invalid color should not parse")
	}
}

func TestPreviewHeightFollowsHeader(t *testing.T) {
	m := testModel(t)
	m.entries[0].pr = headerPR()
	m.applyFilter()
	m.renderList()
	m.updatePreview()
	hh := lipgloss.Height(previewHeader(m.currentRow().e.pr, m.prevW()))
	if hh < 8 {
		t.Fatalf("expected a tall header, got %d lines", hh)
	}
	if got, want := m.prevVP.Height(), m.bodyH()-hh-1; got != want {
		t.Errorf("preview height = %d, want %d (body %d - header %d - a blank line)", got, want, m.bodyH(), hh)
	}
	col := m.rightColumn()
	if got := lipgloss.Height(col); got != m.bodyH() {
		t.Errorf("right column height = %d, want bodyH %d", got, m.bodyH())
	}
}
