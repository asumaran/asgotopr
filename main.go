// gotopr: a herdr plugin popup that lists your open GitHub PRs (author or
// assignee) across the repos cloned under ~/Developer, grouped by repo, with
// fuzzy search and a markdown preview of the PR description. Selecting a PR
// lands you on its checkout: an existing worktree is opened/focused in the
// herdr sidebar; otherwise the main clone switches to the branch (offering to
// stash first when dirty) and the repo is selected in the sidebar.
//
// PR data comes from the gh CLI (two GraphQL searches + one batched body
// query), cached stale-while-revalidate so the popup renders instantly.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// version is the release tag; overridden at build time via
// -ldflags "-X main.version=vX.Y.Z" (see scripts/release.sh and CI).
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print the embedded version")
	dump := flag.Bool("dump", false, "print discovered repos and PRs (no TUI)")
	query := flag.String("query", "", "with -dump: print filter scores for this query")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	repos, scanCache := scanRepos(devRoot(), loadRepoScanCache())
	saveRepoScanCache(scanCache)
	slugs := slugMap(repos)
	cache := loadPRCache()
	stale := time.Since(cache.FetchedAt) >= prCacheFresh

	if *dump {
		runDump(cache, stale, repos, slugs, *query)
		return
	}

	// Terminal background detection must happen before the program owns
	// stdin; see the glamourStyle comment in preview.go.
	if !lipgloss.HasDarkBackground() {
		glamourStyle = "light"
	}

	ti := textinput.New()
	ti.Prompt = promptText()
	ti.PromptStyle = lipgloss.NewStyle() // colors are already baked into the prompt
	ti.Focus()

	m := model{
		slugs:           slugs,
		cache:           cache,
		refreshing:      stale,
		pendingSearches: 2,
		ti:              ti,
		listVP:          viewport.New(50, 20),
		prevVP:          viewport.New(40, 17),
		help:            help.New(),
		keys:            defaultKeys(),
		renders:         map[string]string{},
		width:           94,
		height:          24,
	}
	if !stale {
		m.pendingSearches = 0
	}
	m.setEntries(cache.PRs)
	m.applyFilter()
	m.resize()
	m.renderList()

	res, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	runAction(res.(model).action)
}

// runDump prints the discovered state without a TUI: repos, grouped PRs with
// their worktree resolution, and (with -query) filter scores. It refreshes
// synchronously when the cache is stale, so it exercises the same fetch path
// the TUI uses in the background.
func runDump(cache prCache, stale bool, repos []localRepo, slugs map[string][]localRepo, query string) {
	if stale {
		fresh, err := refreshSynchronously(cache)
		if err != nil {
			fmt.Fprintln(os.Stderr, "refresh failed, using cache:", err)
		} else {
			cache = fresh
			savePRCache(cache)
		}
	}
	fmt.Printf("cache: %d PRs, fetched %s\n", len(cache.PRs), relTime(cache.FetchedAt))
	fmt.Printf("repos: %d GitHub clones under %s\n", len(repos), homeRel(devRoot()))
	for _, r := range repos {
		fmt.Printf("  %-24s %s\n", r.Name, r.Slug)
	}

	entries := buildEntries(cache.PRs, slugs)
	fmt.Printf("entries: %d PRs with a local clone\n", len(entries))
	lastSlug := ""
	for _, e := range entries {
		if e.pr.RepoSlug != lastSlug {
			fmt.Printf("%s (%s)\n", e.repo.Name, homeRel(e.repo.Path))
			lastSlug = e.pr.RepoSlug
		}
		wt := "no local branch"
		if w := worktreeForBranch(e.repo.Path, e.pr.HeadRefName); w != nil {
			if w.Path == e.repo.Path {
				wt = "checked out in main clone"
			} else {
				wt = "worktree " + homeRel(w.Path)
			}
		} else if branchExists(e.repo.Path, e.pr.HeadRefName) {
			wt = "local branch, not checked out"
		}
		fmt.Printf("  #%-6d %-40s %s [%s] updated %s, body %dB (%s)\n",
			e.pr.Number, truncate(e.pr.Title, 40), e.pr.HeadRefName,
			strings.Join(e.pr.Roles, ","), relTime(e.pr.UpdatedAt), len(e.pr.Body), wt)
	}

	if query != "" {
		q := strings.ToLower(query)
		titles, branches, metas := corpora(entries)
		fmt.Printf("query %q:\n", query)
		for _, r := range buildRows(entries, q, titles, branches, metas) {
			if r.kind != "pr" {
				continue
			}
			fmt.Printf("  %5d  #%d %s\n", r.score, r.e.pr.Number, truncate(r.e.pr.Title, 60))
		}
	}
}
