package main

// Filtering and ranking. Entries (one per PR, pre-grouped by repo) carry
// three lowercased corpora matched independently per keystroke — title,
// branch, and metadata (#number, Jira ticket, repo slug/dir name) — keeping
// asgoto's behavior: typing "1234" finds PR #1234, typing a branch fragment
// finds its PR, ticket keys work too. Only title matches produce highlight
// indexes; branch/meta hits have nothing visible to highlight.

import (
	"sort"
	"strconv"
	"strings"
)

// entry is one selectable PR with the local clone its action targets.
type entry struct {
	pr   prItem
	repo localRepo
}

type row struct {
	kind  string // "header" | "pr"
	e     *entry // nil for headers
	repo  localRepo
	match bool
	score int
	idx   []int // matched rune positions in the title, for highlighting
}

// buildEntries filters PRs to those with a local clone and orders them
// grouped by repo: repos by their newest PR first, PRs within a repo newest
// first. Assumes prs is already sorted newest-first (mergePRs does).
func buildEntries(prs []prItem, slugs map[string][]localRepo) []*entry {
	grouped := map[string][]*entry{}
	var slugOrder []string // by newest PR, since prs is newest-first
	for _, p := range prs {
		clones, ok := slugs[p.RepoSlug]
		if !ok {
			continue
		}
		if _, seen := grouped[p.RepoSlug]; !seen {
			slugOrder = append(slugOrder, p.RepoSlug)
		}
		grouped[p.RepoSlug] = append(grouped[p.RepoSlug], &entry{pr: p, repo: primaryClone(clones)})
	}
	var out []*entry
	for _, slug := range slugOrder {
		g := grouped[slug]
		sort.SliceStable(g, func(i, j int) bool {
			return g[i].pr.UpdatedAt.After(g[j].pr.UpdatedAt)
		})
		out = append(out, g...)
	}
	return out
}

// corpora returns the three parallel lowercased search texts for entries.
func corpora(entries []*entry) (titles, branches, metas []string) {
	for _, e := range entries {
		titles = append(titles, strings.ToLower(e.pr.Title))
		branches = append(branches, strings.ToLower(e.pr.HeadRefName))
		meta := "#" + strconv.Itoa(e.pr.Number)
		if t := ticketFrom(e.pr.HeadRefName, e.pr.Title); t != "" {
			meta += " " + strings.ToLower(t)
		}
		meta += " " + e.pr.RepoSlug + " " + strings.ToLower(e.repo.Name)
		metas = append(metas, meta)
	}
	return titles, branches, metas
}

type hit struct {
	score  int
	idx    []int // only from title matches
	branch bool  // best score came from the branch corpus
}

// findHits matches the query's terms over all corpora (see findFields).
func findHits(q string, titles, branches, metas []string) map[int]hit {
	hits := map[int]hit{}
	for i, h := range findFields(q, titles, branches, metas) {
		hits[i] = hit{score: h.Score, idx: h.Any[0], branch: h.Field == 1}
	}
	return hits
}

// matchBonus biases ranking beyond the raw fuzzy score: exact PR number and
// exact repo-name queries jump to the obvious target, branch matches beat
// title matches on ties (branches are what you type when switching), drafts
// sink slightly.
func matchBonus(e *entry, h hit, q string) int {
	bonus := 0
	if n := strings.TrimSpace(q); n == strconv.Itoa(e.pr.Number) || n == "#"+strconv.Itoa(e.pr.Number) {
		bonus += 30
	}
	tail := e.pr.RepoSlug[strings.Index(e.pr.RepoSlug, "/")+1:]
	if q == strings.ToLower(e.repo.Name) || q == tail {
		bonus += 10
	}
	if h.branch {
		bonus += 2
	}
	if e.pr.IsDraft {
		bonus -= 2
	}
	return bonus
}

// buildRows turns entries into display rows: a header per repo, then its PRs.
// When filtering, only matching PRs (and their headers) survive and they are
// ranked: best match first, the repo that holds it on top.
func buildRows(entries []*entry, q string, titles, branches, metas []string) []row {
	filtering := hasTerms(q)
	var hits map[int]hit
	if filtering {
		hits = findHits(q, titles, branches, metas)
	}
	var prs []row
	for i, e := range entries {
		h, ok := hits[i]
		if filtering && !ok {
			continue
		}
		score := 0
		if filtering {
			score = h.score + matchBonus(e, h, q)
		}
		prs = append(prs, row{kind: "pr", e: e, repo: e.repo, match: ok, score: score, idx: h.idx})
	}
	if filtering {
		// equal scores: the most recently updated PR first
		sort.SliceStable(prs, func(i, j int) bool { return prs[i].e.pr.UpdatedAt.After(prs[j].e.pr.UpdatedAt) })
		prs = rank(prs, func(r row) int { return r.score }, func(r row) string { return r.e.pr.RepoSlug })
	}
	var rows []row
	lastSlug := ""
	for _, r := range prs {
		if r.e.pr.RepoSlug != lastSlug {
			rows = append(rows, row{kind: "header", repo: r.repo})
			lastSlug = r.e.pr.RepoSlug
		}
		rows = append(rows, r)
	}
	return rows
}

// firstPR returns the index of the first selectable row, or -1.
func firstPR(rows []row) int {
	for i, r := range rows {
		if r.kind == "pr" {
			return i
		}
	}
	return -1
}
