package main

import (
	"testing"
	"time"
)

func TestOverlaySharedPRs(t *testing.T) {
	t0 := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	shared := emptySharedPRs()
	shared.Pulls["https://github.com/o/r/pull/2"] = sharedPR{Number: 2, State: "draft", Title: "now a draft", UpdatedAt: t0.Add(time.Hour)}
	shared.Pulls["https://github.com/o/r/pull/3"] = sharedPR{Number: 3, State: "merged", UpdatedAt: t0.Add(time.Hour)}
	shared.Pulls["https://github.com/o/r/pull/4"] = sharedPR{Number: 4, State: "closed", UpdatedAt: t0.Add(-time.Hour)}
	prs := []prItem{
		{URL: "https://github.com/o/r/pull/1", Number: 1, HeadRefName: "feature", UpdatedAt: t0},
		{URL: "https://github.com/o/r/pull/2", Number: 2, Title: "open", UpdatedAt: t0},
		{URL: "https://github.com/o/r/pull/3", Number: 3, UpdatedAt: t0},
		{URL: "https://github.com/o/r/pull/4", Number: 4, UpdatedAt: t0},
	}
	got := overlaySharedPRs(prs, shared)
	if len(got) != 3 || got[0].Number != 1 || got[1].Number != 2 || got[2].Number != 4 {
		t.Fatalf("got %+v, want 1, 2 and 4 (3 merged since)", got)
	}
	if !got[1].IsDraft || got[1].Title != "now a draft" {
		t.Errorf("a newer shared version wins: %+v", got[1])
	}
	if prs[1].IsDraft || len(prs) != 4 {
		t.Error("the overlay is a display layer: the cached slice is not touched")
	}
}
