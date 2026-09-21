package main

// Disk caches, stale-while-revalidate: everything renders instantly from the
// last saved snapshot while background fetches refresh it. Cache files live
// in the herdr-injected per-plugin state dir; standalone runs (e.g. -dump
// outside herdr) use that same directory (statedir.go).

import (
	"path/filepath"
	"time"
)

// prCacheFresh is how recent the cached PR snapshot must be to skip the
// background refresh entirely. It only debounces rapid reopen cycles; older
// snapshots still render immediately while they revalidate.
const prCacheFresh = 60 * time.Second

type prCache struct {
	FetchedAt time.Time `json:"fetched_at"`
	PRs       []prItem  `json:"prs"`
}

func (c prCache) byURL() map[string]prItem {
	m := make(map[string]prItem, len(c.PRs))
	for _, p := range c.PRs {
		m[p.URL] = p
	}
	return m
}

func prCacheFile() string {
	return filepath.Join(stateDir(), "prcache.json")
}

func loadPRCache() prCache {
	var c prCache
	readJSONFile(prCacheFile(), &c)
	return c
}

func savePRCache(c prCache) { writeJSONFile(prCacheFile(), c) }

// stateDir is where asgotopr keeps its runtime state.
func stateDir() string { return stateDirFor("asgotopr") }
