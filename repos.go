package main

// Local repo discovery: scan ~/Developer for git clones with a GitHub origin
// and build a slug -> clones map. The scan reads .git/config files directly
// (no subprocess); a small mtime-keyed cache in repos.json avoids re-parsing
// configs that haven't changed, keeping warm startups nearly free.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type localRepo struct {
	Slug string `json:"slug"` // lowercased "owner/repo"
	Path string `json:"path"` // main clone, e.g. ~/Developer/gotopr
	Name string `json:"name"` // directory base name
}

type repoScanEntry struct {
	Path        string `json:"path"`
	Slug        string `json:"slug"`
	ConfigMTime int64  `json:"config_mtime"` // unix seconds of .git/config
}

type repoScanCache struct {
	Entries []repoScanEntry `json:"entries"`
}

// devRoot is where local clones live. Overridable for tests.
func devRoot() string {
	if r := os.Getenv("GOTOPR_ROOT"); r != "" {
		return r
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(h, "Developer")
}

// scanRepos walks the immediate children of root and returns every git clone
// with a GitHub origin, plus the refreshed scan cache to persist.
func scanRepos(root string, cache repoScanCache) ([]localRepo, repoScanCache) {
	cached := make(map[string]repoScanEntry, len(cache.Entries))
	for _, e := range cache.Entries {
		cached[e.Path] = e
	}

	var repos []localRepo
	var fresh repoScanCache
	dirs, err := os.ReadDir(root)
	if err != nil {
		return nil, fresh
	}
	for _, d := range dirs {
		path := filepath.Join(root, d.Name())
		if !isDirOrDirLink(path, d) {
			continue
		}
		gitdir := resolveGitDir(path)
		if gitdir == "" {
			continue
		}
		mtime := int64(0)
		if fi, err := os.Stat(filepath.Join(gitdir, "config")); err == nil {
			mtime = fi.ModTime().Unix()
		}
		slug := ""
		if e, ok := cached[path]; ok && e.ConfigMTime == mtime {
			slug = e.Slug
		} else {
			slug = strings.ToLower(githubSlug(path))
		}
		fresh.Entries = append(fresh.Entries, repoScanEntry{Path: path, Slug: slug, ConfigMTime: mtime})
		if slug == "" {
			continue // no origin, or origin not on github.com
		}
		repos = append(repos, localRepo{Slug: slug, Path: path, Name: d.Name()})
	}
	return repos, fresh
}

// isDirOrDirLink reports whether a scan-root entry is a directory, following
// symlinks: a root made of links to clones elsewhere (e.g. a curated
// GOTOPR_ROOT for a demo or a test) scans like the real thing. Dangling or
// file links are skipped. The link path itself is kept as the clone path, so
// everything downstream (worktree listing, herdr workspaces) sees the
// directory the user pointed at.
func isDirOrDirLink(path string, d os.DirEntry) bool {
	if d.IsDir() {
		return true
	}
	if d.Type()&os.ModeSymlink == 0 {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// slugMap groups clones by slug. A slug maps to multiple clones when the same
// origin is cloned more than once.
func slugMap(repos []localRepo) map[string][]localRepo {
	m := map[string][]localRepo{}
	for _, r := range repos {
		m[r.Slug] = append(m[r.Slug], r)
	}
	return m
}

// primaryClone picks the clone a PR is listed under when the same origin is
// cloned more than once: the directory named exactly like the remote repo
// wins, then the lexicographically first path (deterministic).
func primaryClone(clones []localRepo) localRepo {
	nameMatches := func(r localRepo) bool {
		tail := r.Slug[strings.Index(r.Slug, "/")+1:]
		return strings.EqualFold(r.Name, tail)
	}
	best := clones[0]
	for _, c := range clones[1:] {
		ce, be := nameMatches(c), nameMatches(best)
		if ce != be {
			if ce {
				best = c
			}
			continue
		}
		if c.Path < best.Path {
			best = c
		}
	}
	return best
}

// ---- scan cache persistence ----

func reposCacheFile() string {
	return filepath.Join(stateDir(), "repos.json")
}

func loadRepoScanCache() repoScanCache {
	var c repoScanCache
	if data, err := os.ReadFile(reposCacheFile()); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	return c
}

func saveRepoScanCache(c repoScanCache) {
	if data, err := json.Marshal(c); err == nil {
		path := reposCacheFile()
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, data, 0o644)
	}
}
