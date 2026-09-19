package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestGithubSlugFromURL(t *testing.T) {
	cases := map[string]string{
		"git@github.com:asumaran/asgotopr.git": "asumaran/asgotopr",
		"git@github.com:asumaran/asgotopr":     "asumaran/asgotopr",
		"ssh://git@github.com/owner/repo.git":  "owner/repo",
		"https://github.com/owner/repo":        "owner/repo",
		"https://github.com/owner/repo.git":    "owner/repo",
		"https://github.com/owner/repo/":       "owner/repo",
		"https://gitlab.com/owner/repo.git":    "",
		"/Users/asumaran/Developer/asreviewer": "",
		"git@github.com:owner/group/sub.git":   "",
		"":                                     "",
	}
	for url, want := range cases {
		if got := githubSlugFromURL(url); got != want {
			t.Errorf("githubSlugFromURL(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestOriginURL(t *testing.T) {
	config := `[core]
	repositoryformatversion = 0
[remote "upstream"]
	url = git@github.com:other/upstream.git
[remote "origin"]
	url = git@github.com:asumaran/asgotopr.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`
	if got := originURL(config); got != "git@github.com:asumaran/asgotopr.git" {
		t.Errorf("originURL = %q", got)
	}
	if got := originURL("[core]\n\tbare = false\n"); got != "" {
		t.Errorf("originURL with no origin = %q, want empty", got)
	}
}

func TestMergePRsDedupsRoles(t *testing.T) {
	a := []prItem{
		{URL: "u1", Number: 1, UpdatedAt: time.Unix(100, 0), Roles: []string{"author"}},
		{URL: "u2", Number: 2, UpdatedAt: time.Unix(300, 0), Roles: []string{"author"}},
	}
	b := []prItem{
		{URL: "u1", Number: 1, UpdatedAt: time.Unix(100, 0), Roles: []string{"assignee"}},
		{URL: "u3", Number: 3, UpdatedAt: time.Unix(200, 0), Roles: []string{"assignee"}},
	}
	got := mergePRs(a, b)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	// Sorted newest first.
	if got[0].URL != "u2" || got[1].URL != "u3" || got[2].URL != "u1" {
		t.Errorf("order = %s,%s,%s", got[0].URL, got[1].URL, got[2].URL)
	}
	for _, p := range got {
		if p.URL == "u1" {
			if len(p.Roles) != 2 || !contains(p.Roles, "author") || !contains(p.Roles, "assignee") {
				t.Errorf("u1 roles = %v", p.Roles)
			}
		}
	}
}

func TestPlanRefresh(t *testing.T) {
	t1 := time.Unix(100, 0)
	t2 := time.Unix(200, 0)
	cached := map[string]prItem{
		"unchanged": {URL: "unchanged", UpdatedAt: t1, Body: "cached body"},
		"changed":   {URL: "changed", UpdatedAt: t1, Body: "old body"},
		"gone":      {URL: "gone", UpdatedAt: t1, Body: "x"},
	}
	fresh := []prItem{
		{URL: "unchanged", UpdatedAt: t1},
		{URL: "changed", UpdatedAt: t2},
		{URL: "new", UpdatedAt: t2},
	}
	merged, stale := planRefresh(cached, fresh)
	if len(merged) != 3 {
		t.Fatalf("merged len = %d", len(merged))
	}
	if merged[0].Body != "cached body" {
		t.Errorf("unchanged body = %q, want carried over", merged[0].Body)
	}
	if merged[1].Body != "" || merged[2].Body != "" {
		t.Errorf("changed/new bodies should be empty until fetched")
	}
	if len(stale) != 2 || stale[0].URL != "changed" || stale[1].URL != "new" {
		t.Errorf("stale = %v", stale)
	}
	// "gone" (merged/closed) is pruned simply by not being in fresh.
	for _, p := range merged {
		if p.URL == "gone" {
			t.Error("gone PR survived the refresh")
		}
	}
}

func TestPrimaryClone(t *testing.T) {
	exact := localRepo{Slug: "asumaran/asreviewer", Path: "/d/asreviewer", Name: "asreviewer"}
	other := localRepo{Slug: "asumaran/asreviewer", Path: "/d/asreviewer-server", Name: "asreviewer-server"}
	if got := primaryClone([]localRepo{other, exact}); got.Path != exact.Path {
		t.Errorf("primaryClone = %s, want exact-name clone", got.Path)
	}
	if got := primaryClone([]localRepo{exact, other}); got.Path != exact.Path {
		t.Errorf("primaryClone (reordered) = %s, want exact-name clone", got.Path)
	}
	// No exact name: lexicographically first path wins.
	a := localRepo{Slug: "o/r", Path: "/d/aaa", Name: "aaa"}
	b := localRepo{Slug: "o/r", Path: "/d/bbb", Name: "bbb"}
	if got := primaryClone([]localRepo{b, a}); got.Path != a.Path {
		t.Errorf("primaryClone no-exact = %s, want %s", got.Path, a.Path)
	}
	one := localRepo{Slug: "o/r", Path: "/d/x", Name: "x"}
	if got := primaryClone([]localRepo{one}); got.Path != one.Path {
		t.Errorf("primaryClone single = %s", got.Path)
	}
}

func TestParseWorktreePorcelain(t *testing.T) {
	out := `worktree /Users/x/Developer/repo
HEAD 1111111111111111111111111111111111111111
branch refs/heads/main

worktree /Users/x/wt/repo/feat-thing
HEAD 2222222222222222222222222222222222222222
branch refs/heads/feat/thing

worktree /Users/x/wt/repo/detached-one
HEAD 3333333333333333333333333333333333333333
detached

worktree /Users/x/bare
bare
`
	entries := parseWorktreePorcelain(out)
	if len(entries) != 4 {
		t.Fatalf("len = %d, want 4", len(entries))
	}
	if entries[0].Branch != "main" || entries[0].Path != "/Users/x/Developer/repo" {
		t.Errorf("main entry = %+v", entries[0])
	}
	if entries[1].Branch != "feat/thing" {
		t.Errorf("branch with slash = %q", entries[1].Branch)
	}
	if !entries[2].Detached || entries[2].Branch != "" {
		t.Errorf("detached entry = %+v", entries[2])
	}
	if !entries[3].Bare {
		t.Errorf("bare entry = %+v", entries[3])
	}
}

func testEntries() []*entry {
	repoA := localRepo{Slug: "org/alpha", Path: "/d/alpha", Name: "alpha"}
	repoB := localRepo{Slug: "org/beta", Path: "/d/beta", Name: "beta"}
	return []*entry{
		{pr: prItem{URL: "a1", Number: 100, Title: "fix login flow", HeadRefName: "fix/login",
			RepoSlug: "org/alpha", UpdatedAt: time.Unix(300, 0)}, repo: repoA},
		{pr: prItem{URL: "a2", Number: 205, Title: "add dashboard widgets", HeadRefName: "feat/FED-123-widgets",
			RepoSlug: "org/alpha", UpdatedAt: time.Unix(200, 0), IsDraft: true}, repo: repoA},
		{pr: prItem{URL: "b1", Number: 7, Title: "update readme", HeadRefName: "docs/readme",
			RepoSlug: "org/beta", UpdatedAt: time.Unix(100, 0)}, repo: repoB},
	}
}

func TestBuildRowsGroupingAndNavigation(t *testing.T) {
	entries := testEntries()
	titles, branches, metas := corpora(entries)
	rows := buildRows(entries, "", titles, branches, metas)
	kinds := []string{"header", "pr", "pr", "header", "pr"}
	if len(rows) != len(kinds) {
		t.Fatalf("rows len = %d, want %d", len(rows), len(kinds))
	}
	for i, k := range kinds {
		if rows[i].kind != k {
			t.Errorf("rows[%d].kind = %s, want %s", i, rows[i].kind, k)
		}
	}
	if got := firstPR(rows); got != 1 {
		t.Errorf("firstPR = %d, want 1", got)
	}
	// Down from the last PR stays put; up from a PR after a header skips it.
	if got := nextPR(rows, 4, +1); got != 4 {
		t.Errorf("nextPR down from last = %d, want 4", got)
	}
	if got := nextPR(rows, 4, -1); got != 2 {
		t.Errorf("nextPR up over header = %d, want 2", got)
	}
}

func TestFilterExactNumberBeatsFuzzy(t *testing.T) {
	entries := testEntries()
	titles, branches, metas := corpora(entries)
	rows := buildRows(entries, "100", titles, branches, metas)
	best := bestMatch(rows)
	if best < 0 || rows[best].e.pr.Number != 100 {
		t.Fatalf("query 100: best = %v", best)
	}
}

func TestFilterByBranchAndTicket(t *testing.T) {
	entries := testEntries()
	titles, branches, metas := corpora(entries)

	rows := buildRows(entries, "fed-123", titles, branches, metas)
	best := bestMatch(rows)
	if best < 0 || rows[best].e.pr.Number != 205 {
		t.Fatalf("ticket query: best row = %v", best)
	}

	rows = buildRows(entries, "docs/readme", titles, branches, metas)
	best = bestMatch(rows)
	if best < 0 || rows[best].e.pr.Number != 7 {
		t.Fatalf("branch query: best row = %v", best)
	}
}

func TestFilterRepoNameSurfacesGroup(t *testing.T) {
	entries := testEntries()
	titles, branches, metas := corpora(entries)
	rows := buildRows(entries, "alpha", titles, branches, metas)
	prCount := 0
	for _, r := range rows {
		if r.kind == "pr" {
			prCount++
			if r.e.pr.RepoSlug != "org/alpha" {
				t.Errorf("unexpected repo in results: %s", r.e.pr.RepoSlug)
			}
		}
	}
	if prCount != 2 {
		t.Errorf("repo-name query matched %d PRs, want 2", prCount)
	}
	best := bestMatch(rows)
	// Tie on repo bonus: the non-draft, newer PR should win (draft -2).
	if best < 0 || rows[best].e.pr.Number != 100 {
		t.Fatalf("best for repo query = %v", best)
	}
}

func TestUpdatedAtTiebreak(t *testing.T) {
	repo := localRepo{Slug: "o/r", Path: "/d/r", Name: "r"}
	entries := []*entry{
		{pr: prItem{URL: "old", Number: 1, Title: "same title", HeadRefName: "b1",
			RepoSlug: "o/r", UpdatedAt: time.Unix(100, 0)}, repo: repo},
		{pr: prItem{URL: "new", Number: 2, Title: "same title", HeadRefName: "b2",
			RepoSlug: "o/r", UpdatedAt: time.Unix(200, 0)}, repo: repo},
	}
	titles, branches, metas := corpora(entries)
	rows := buildRows(entries, "same title", titles, branches, metas)
	best := bestMatch(rows)
	if best < 0 || rows[best].e.pr.URL != "new" {
		t.Fatalf("tiebreak: best = %v, want the newer PR", best)
	}
}

func TestTicketFrom(t *testing.T) {
	if got := ticketFrom("feat/FED-2030-add-metrics"); got != "FED-2030" {
		t.Errorf("ticketFrom = %q", got)
	}
	if got := ticketFrom("no ticket here", "plat-1193 fix"); got != "PLAT-1193" {
		t.Errorf("ticketFrom fallback = %q", got)
	}
	if got := ticketFrom("e2e-tests", "v1-2"); got != "" {
		t.Errorf("ticketFrom false positive = %q", got)
	}
}

func TestConfirmModeTransitions(t *testing.T) {
	e := &entry{pr: prItem{URL: "u", Number: 1, HeadRefName: "feat/x", RepoSlug: "o/r"},
		repo: localRepo{Slug: "o/r", Path: "/tmp/nonexistent-asgotopr-test", Name: "r"}}
	m := model{mode: modeConfirmStash, pending: e, keys: defaultKeys()}

	// esc cancels back to filter mode.
	res, _ := m.handleKey(keyMsg("esc"))
	got := res.(model)
	if got.mode != modeFilter || got.pending != nil {
		t.Errorf("esc: mode = %v, pending = %v", got.mode, got.pending)
	}

	// f switches without stashing (busy mode, switch command issued).
	m.mode = modeConfirmStash
	m.pending = e
	res, cmd := m.handleKey(keyMsg("f"))
	got = res.(model)
	if got.mode != modeBusy || cmd == nil {
		t.Errorf("f: mode = %v, cmd nil = %v", got.mode, cmd == nil)
	}

	// s stashes first.
	m.mode = modeConfirmStash
	m.pending = e
	res, cmd = m.handleKey(keyMsg("s"))
	got = res.(model)
	if got.mode != modeBusy || cmd == nil {
		t.Errorf("s: mode = %v, cmd nil = %v", got.mode, cmd == nil)
	}

	// A stash failure lands in modeError; esc returns to filter.
	res2, _ := got.Update(stashedMsg{err: errFake})
	got = res2.(model)
	if got.mode != modeError {
		t.Errorf("stash error: mode = %v", got.mode)
	}
	res2, _ = got.handleKey(keyMsg("esc"))
	got = res2.(model)
	if got.mode != modeFilter {
		t.Errorf("esc from error: mode = %v", got.mode)
	}
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		r := []rune(k)
		return tea.KeyPressMsg{Code: r[0], Text: k}
	}
}

var errFake = &fakeErr{}

type fakeErr struct{}

func (*fakeErr) Error() string { return "fake failure" }

func TestScanReposFollowsSymlinkedClones(t *testing.T) {
	clones := t.TempDir()
	repo := filepath.Join(clones, "shopnest")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := "[remote \"origin\"]\n\turl = git@github.com:asumaran/shopnest.git\n"
	if err := os.WriteFile(filepath.Join(repo, ".git", "config"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	link := filepath.Join(root, "shopnest")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(clones, "missing"), filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	repos, _ := scanRepos(root, repoScanCache{})
	if len(repos) != 1 {
		t.Fatalf("want 1 repo through the symlink, got %d: %+v", len(repos), repos)
	}
	if repos[0].Slug != "asumaran/shopnest" || repos[0].Path != link || repos[0].Name != "shopnest" {
		t.Errorf("unexpected repo: %+v", repos[0])
	}
}
