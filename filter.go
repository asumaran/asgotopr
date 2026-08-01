package main

// Filtering and ranking. Entries (one per PR, pre-grouped by repo) carry
// three lowercased corpora matched independently per keystroke — title,
// branch, and metadata (#number, Jira ticket, repo slug/dir name) — keeping
// goto's behavior: typing "1234" finds PR #1234, typing a branch fragment
// finds its PR, ticket keys work too. Only title matches produce highlight
// indexes; branch/meta hits have nothing visible to highlight.

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sahilm/fuzzy"
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

// ticketRe matches a Jira-style ticket key: a project key of 2+ letters, a
// dash and digits (FED-2030, plat-1193).
var ticketRe = regexp.MustCompile(`(?i)\b([a-z][a-z]+-[0-9]+)\b`)

func ticketFrom(sources ...string) string {
	for _, s := range sources {
		if m := ticketRe.FindString(s); m != "" {
			return strings.ToUpper(m)
		}
	}
	return ""
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

// findHits runs the fuzzy matcher over all corpora and keeps the best score
// per entry.
func findHits(q string, titles, branches, metas []string) map[int]hit {
	hits := map[int]hit{}
	for _, mt := range fuzzy.Find(q, titles) {
		hits[mt.Index] = hit{score: mt.Score, idx: mt.MatchedIndexes}
	}
	for _, mt := range fuzzy.Find(q, branches) {
		if h, ok := hits[mt.Index]; !ok || mt.Score > h.score {
			hits[mt.Index] = hit{score: mt.Score, branch: true}
		}
	}
	for _, mt := range fuzzy.Find(q, metas) {
		if h, ok := hits[mt.Index]; !ok || mt.Score > h.score {
			hits[mt.Index] = hit{score: mt.Score}
		}
	}
	return hits
}

// matchBonus biases ranking beyond the raw fuzzy score: exact PR number and
// exact repo-name queries jump to the obvious target, branch matches beat
// title matches on ties (branches are what you type when switching), drafts
// sink slightly.
func matchBonus(e *entry, h hit, q string) int {
	bonus := 0
	if q == strconv.Itoa(e.pr.Number) || q == "#"+strconv.Itoa(e.pr.Number) {
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
// When filtering, only matching PRs (and their headers) survive.
func buildRows(entries []*entry, q string, titles, branches, metas []string) []row {
	filtering := q != ""
	var hits map[int]hit
	if filtering {
		hits = findHits(q, titles, branches, metas)
	}
	var rows []row
	lastSlug := ""
	for i, e := range entries {
		h, ok := hits[i]
		if filtering && !ok {
			continue
		}
		if e.pr.RepoSlug != lastSlug {
			rows = append(rows, row{kind: "header", repo: e.repo})
			lastSlug = e.pr.RepoSlug
		}
		score := 0
		if filtering {
			score = h.score + matchBonus(e, h, q)
		}
		rows = append(rows, row{kind: "pr", e: e, repo: e.repo, match: ok, score: score, idx: h.idx})
	}
	return rows
}

// bestMatch returns the index of the highest-scored matching PR row; ties go
// to the most recently updated PR. -1 when nothing matches.
func bestMatch(rows []row) int {
	best := -1
	for i, r := range rows {
		if r.kind != "pr" || !r.match {
			continue
		}
		if best == -1 || r.score > rows[best].score ||
			(r.score == rows[best].score && r.e.pr.UpdatedAt.After(rows[best].e.pr.UpdatedAt)) {
			best = i
		}
	}
	return best
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

// nextPR walks from cur in direction dir (+1/-1) to the next selectable row,
// returning cur when there is none.
func nextPR(rows []row, cur, dir int) int {
	for i := cur + dir; i >= 0 && i < len(rows); i += dir {
		if rows[i].kind == "pr" {
			return i
		}
	}
	return cur
}
