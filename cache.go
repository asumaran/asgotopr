package main

// Disk caches, stale-while-revalidate: everything renders instantly from the
// last saved snapshot while background fetches refresh it. Cache files live
// in the herdr-injected per-plugin state dir; standalone runs (e.g. -dump
// outside herdr) fall back to a fixed path under ~/.config/herdr.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func stateDir() string {
	// Running as a herdr plugin: herdr creates and injects a per-plugin state
	// dir; runtime state must live there, not in the plugin checkout.
	if dir := os.Getenv("HERDR_PLUGIN_STATE_DIR"); dir != "" {
		return dir
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		if h, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(h, ".config")
		}
	}
	return filepath.Join(base, "herdr", "asgotopr-tui")
}

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
	if data, err := os.ReadFile(prCacheFile()); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	return c
}

func savePRCache(c prCache) {
	if data, err := json.Marshal(c); err == nil {
		path := prCacheFile()
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, data, 0o644)
	}
}
