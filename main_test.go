package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

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
	nav, isPR := defaultListNav(), func(i int) bool { return rows[i].kind == "pr" }
	if got := nav.move(tea.KeyPressMsg{Code: tea.KeyDown}, 4, len(rows), 10, isPR); got != 4 {
		t.Errorf("down from the last PR = %d, want 4", got)
	}
	if got := nav.move(tea.KeyPressMsg{Code: tea.KeyUp}, 4, len(rows), 10, isPR); got != 2 {
		t.Errorf("up over a header = %d, want 2", got)
	}
}

func TestFilterExactNumberBeatsFuzzy(t *testing.T) {
	entries := testEntries()
	titles, branches, metas := corpora(entries)
	rows := buildRows(entries, "100", titles, branches, metas)
	best := firstPR(rows)
	if best < 0 || rows[best].e.pr.Number != 100 {
		t.Fatalf("query 100: best = %v", best)
	}
}

func TestFilterByBranchAndTicket(t *testing.T) {
	entries := testEntries()
	titles, branches, metas := corpora(entries)

	rows := buildRows(entries, "fed-123", titles, branches, metas)
	best := firstPR(rows)
	if best < 0 || rows[best].e.pr.Number != 205 {
		t.Fatalf("ticket query: best row = %v", best)
	}

	rows = buildRows(entries, "docs/readme", titles, branches, metas)
	best = firstPR(rows)
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
	best := firstPR(rows)
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
	best := firstPR(rows)
	if best < 0 || rows[best].e.pr.URL != "new" {
		t.Fatalf("tiebreak: best = %v, want the newer PR", best)
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

// The matcher gets the text as it is shown and the query as it is typed: case
// never decides a match, and the offsets are bytes into the title on screen.
func TestFilterFoldsCase(t *testing.T) {
	entries := testEntries()
	entries[0].pr.Title = "Fix Login flow"
	titles, branches, metas := corpora(entries)
	if titles[0] != "Fix Login flow" {
		t.Fatalf("the corpus is the shown text: %q", titles[0])
	}
	for _, q := range []string{"login", "LOGIN", "'Login", "fed-123", "ALPHA"} {
		rows := buildRows(entries, q, titles, branches, metas)
		if first := firstPR(rows); first < 0 {
			t.Errorf("%q matched nothing", q)
		}
	}
	rows := buildRows(entries, "LOGIN", titles, branches, metas)
	r := rows[firstPR(rows)]
	if r.e.pr.URL != "a1" || len(r.idx) == 0 || r.idx[0] != 4 {
		t.Errorf("LOGIN: %s idx %v, want a1 with the match at byte 4 of the title", r.e.pr.URL, r.idx)
	}
}

// dumpInputs is what main hands runDump, built from testEntries: the cache,
// the clones and the clones by slug. Nothing is read from the real state dir
// or the real ~/Developer, and the clones are fake paths, so git only answers
// "no local branch".
func dumpInputs(t *testing.T) (prCache, []localRepo, map[string][]localRepo) {
	t.Helper()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	t.Setenv("ASGOTOPR_ROOT", t.TempDir())
	cache := prCache{FetchedAt: time.Now()}
	var repos []localRepo
	slugs := map[string][]localRepo{}
	for _, e := range testEntries() {
		cache.PRs = append(cache.PRs, e.pr)
		if _, seen := slugs[e.repo.Slug]; !seen {
			repos = append(repos, e.repo)
			slugs[e.repo.Slug] = []localRepo{e.repo}
		}
	}
	return cache, repos, slugs
}

// TestRunDump covers -dump on a fresh cache (no refresh, so no network): the
// counts on top, every clone, and every PR once under its repo.
func TestRunDump(t *testing.T) {
	cache, repos, slugs := dumpInputs(t)
	var out bytes.Buffer
	runDump(&out, cache, false, repos, slugs, "")
	got := out.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if !strings.HasPrefix(lines[0], "cache: 3 PRs, fetched ") {
		t.Errorf("first line %q, want the cache summary", lines[0])
	}
	for _, want := range []string{"repos: 2 GitHub clones under ", "entries: 3 PRs with a local clone\n", "alpha (/d/alpha)\n", "beta (/d/beta)\n"} {
		if strings.Count(got, want) != 1 {
			t.Errorf("%q is in the dump %d times, want once:\n%s", want, strings.Count(got, want), got)
		}
	}
	for _, title := range []string{"fix login flow", "add dashboard widgets", "update readme"} {
		if strings.Count(got, title) != 1 {
			t.Errorf("PR %q is listed %d times, want once:\n%s", title, strings.Count(got, title), got)
		}
	}
	// 3 summary lines, 2 clones, 2 repo headers, 2 lines per PR.
	if len(lines) != 13 {
		t.Errorf("got %d lines, want 13:\n%s", len(lines), got)
	}
	if n := strings.Count(got, "(no local branch)"); n != 3 {
		t.Errorf("%d PRs without a local branch, want the 3 of the fake clones:\n%s", n, got)
	}
}

// TestRunDumpQuery covers -dump -query: the matches with their scores, best
// first, instead of the grouped list.
func TestRunDumpQuery(t *testing.T) {
	cache, repos, slugs := dumpInputs(t)
	var out bytes.Buffer
	runDump(&out, cache, false, repos, slugs, "alpha")
	got := out.String()
	_, matches, ok := strings.Cut(got, "query \"alpha\":\n")
	if !ok {
		t.Fatalf("no query line:\n%s", got)
	}
	lines := strings.Split(strings.TrimRight(matches, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d matches, want the 2 PRs of alpha:\n%s", len(lines), got)
	}
	var scores []int
	for i, want := range []string{"#100 fix login flow", "#205 add dashboard widgets"} {
		score, rest, _ := strings.Cut(strings.TrimSpace(lines[i]), "  ")
		n, err := strconv.Atoi(score)
		if err != nil || rest != want {
			t.Errorf("match %d is %q, want a score and %q", i, lines[i], want)
		}
		scores = append(scores, n)
	}
	if scores[0] < scores[1] {
		t.Errorf("scores %v, want the best first", scores)
	}
	// The matches replace the list: no PR of the other repo, no list rows.
	for _, not := range []string{"update readme", "no local branch", "checks ", "alpha (/d/alpha)"} {
		if strings.Contains(got, not) {
			t.Errorf("the query dump has %q, a piece of the full listing:\n%s", not, got)
		}
	}
}
