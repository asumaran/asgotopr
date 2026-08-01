package main

// GitHub data source, via the gh CLI. Two GraphQL search calls (author:@me /
// assignee:@me) fetch every open PR's metadata WITHOUT the body; bodies are
// fetched afterwards in a single aliased query, but only for PRs whose
// updatedAt moved past the cached copy (editing a PR body always bumps
// updatedAt, so it is a safe invalidation key).

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type prItem struct {
	URL           string    `json:"url"` // identity key (dedup + cache)
	Number        int       `json:"number"`
	Title         string    `json:"title"`
	Body          string    `json:"body"`
	UpdatedAt     time.Time `json:"updated_at"`
	HeadRefName   string    `json:"head_ref_name"`
	IsDraft       bool      `json:"is_draft"`
	IsCrossRepo   bool      `json:"is_cross_repo"`
	HeadRepoOwner string    `json:"head_repo_owner"`
	RepoSlug      string    `json:"repo_slug"` // lowercased "owner/repo"
	Roles         []string  `json:"roles"`     // "author", "assignee"
}

const ghTimeout = 8 * time.Second

const searchQuery = `query($q: String!) {
  search(query: $q, type: ISSUE, first: 100) {
    nodes {
      ... on PullRequest {
        number title url updatedAt headRefName isDraft isCrossRepository
        repository { nameWithOwner }
        headRepositoryOwner { login }
      }
    }
  }
}`

type searchResp struct {
	Data struct {
		Search struct {
			Nodes []struct {
				Number              int                            `json:"number"`
				Title               string                         `json:"title"`
				URL                 string                         `json:"url"`
				UpdatedAt           time.Time                      `json:"updatedAt"`
				HeadRefName         string                         `json:"headRefName"`
				IsDraft             bool                           `json:"isDraft"`
				IsCrossRepository   bool                           `json:"isCrossRepository"`
				Repository          struct{ NameWithOwner string } `json:"repository"`
				HeadRepositoryOwner struct {
					Login string `json:"login"`
				} `json:"headRepositoryOwner"`
			} `json:"nodes"`
		} `json:"search"`
	} `json:"data"`
}

func ghGraphQL(ctx context.Context, query string, vars map[string]string) ([]byte, error) {
	args := []string{"api", "graphql", "-f", "query=" + query}
	for k, v := range vars {
		args = append(args, "-f", k+"="+v)
	}
	out, err := exec.CommandContext(ctx, "gh", args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("gh: %s", firstLine(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("gh: %w", err)
	}
	return out, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

// fetchSearch runs one search (role = "author" | "assignee") and returns the
// open PRs for that role, without bodies.
func fetchSearch(role string) ([]prItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	q := fmt.Sprintf("is:pr is:open %s:@me archived:false", role)
	out, err := ghGraphQL(ctx, searchQuery, map[string]string{"q": q})
	if err != nil {
		return nil, err
	}
	var resp searchResp
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("gh: bad search response: %w", err)
	}
	var prs []prItem
	for _, n := range resp.Data.Search.Nodes {
		if n.URL == "" { // non-PR node decoded as zero struct
			continue
		}
		prs = append(prs, prItem{
			URL:           n.URL,
			Number:        n.Number,
			Title:         n.Title,
			UpdatedAt:     n.UpdatedAt,
			HeadRefName:   n.HeadRefName,
			IsDraft:       n.IsDraft,
			IsCrossRepo:   n.IsCrossRepository,
			HeadRepoOwner: n.HeadRepositoryOwner.Login,
			RepoSlug:      strings.ToLower(n.Repository.NameWithOwner),
			Roles:         []string{role},
		})
	}
	return prs, nil
}

// mergePRs deduplicates search results by URL, unioning roles, and sorts by
// UpdatedAt (newest first) for deterministic downstream grouping.
func mergePRs(results ...[]prItem) []prItem {
	byURL := map[string]*prItem{}
	var order []string
	for _, prs := range results {
		for _, p := range prs {
			if cur, ok := byURL[p.URL]; ok {
				for _, r := range p.Roles {
					if !contains(cur.Roles, r) {
						cur.Roles = append(cur.Roles, r)
					}
				}
				continue
			}
			pp := p
			byURL[p.URL] = &pp
			order = append(order, p.URL)
		}
	}
	merged := make([]prItem, 0, len(order))
	for _, u := range order {
		merged = append(merged, *byURL[u])
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].UpdatedAt.After(merged[j].UpdatedAt)
	})
	return merged
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// planRefresh carries cached bodies over into fresh search results and
// reports which PRs still need their body fetched: those that are new or
// whose updatedAt moved past the cached copy.
func planRefresh(cached map[string]prItem, fresh []prItem) (merged []prItem, stale []prItem) {
	merged = make([]prItem, len(fresh))
	for i, p := range fresh {
		if c, ok := cached[p.URL]; ok && !p.UpdatedAt.After(c.UpdatedAt) {
			p.Body = c.Body
		} else {
			stale = append(stale, p)
		}
		merged[i] = p
	}
	return merged, stale
}

// fetchBodies fetches the body of every given PR in one aliased GraphQL
// query. Returns url -> body.
func fetchBodies(prs []prItem) (map[string]string, error) {
	if len(prs) == 0 {
		return map[string]string{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()

	var b strings.Builder
	b.WriteString("query {\n")
	for i, p := range prs {
		owner, name, ok := strings.Cut(p.RepoSlug, "/")
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "  p%d: repository(owner: %q, name: %q) { pullRequest(number: %d) { body } }\n",
			i, owner, name, p.Number)
	}
	b.WriteString("}")

	out, err := ghGraphQL(ctx, b.String(), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data map[string]struct {
			PullRequest struct {
				Body string `json:"body"`
			} `json:"pullRequest"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("gh: bad bodies response: %w", err)
	}
	bodies := make(map[string]string, len(prs))
	for i, p := range prs {
		if node, ok := resp.Data[fmt.Sprintf("p%d", i)]; ok {
			bodies[p.URL] = node.PullRequest.Body
		}
	}
	return bodies, nil
}

// applyBodies stamps fetched bodies onto the matching items.
func applyBodies(prs []prItem, bodies map[string]string) []prItem {
	for i := range prs {
		if body, ok := bodies[prs[i].URL]; ok {
			prs[i].Body = body
		}
	}
	return prs
}

// ---- bubbletea plumbing ----

type searchMsg struct {
	role string
	prs  []prItem
	err  error
}

type bodiesMsg struct {
	bodies map[string]string
	err    error
}

func fetchSearchCmd(role string) tea.Cmd {
	return func() tea.Msg {
		prs, err := fetchSearch(role)
		return searchMsg{role: role, prs: prs, err: err}
	}
}

func fetchBodiesCmd(stale []prItem) tea.Cmd {
	return func() tea.Msg {
		bodies, err := fetchBodies(stale)
		return bodiesMsg{bodies: bodies, err: err}
	}
}

// refreshSynchronously runs the whole refresh inline (used by -dump): both
// searches, merge, body fetch for stale entries. Returns the refreshed cache.
func refreshSynchronously(cache prCache) (prCache, error) {
	author, errA := fetchSearch("author")
	assignee, errB := fetchSearch("assignee")
	if errA != nil {
		return cache, errA
	}
	if errB != nil {
		return cache, errB
	}
	merged, stale := planRefresh(cache.byURL(), mergePRs(author, assignee))
	bodies, err := fetchBodies(stale)
	if err != nil {
		return cache, err
	}
	return prCache{FetchedAt: time.Now(), PRs: applyBodies(merged, bodies)}, nil
}
