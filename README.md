# asgotopr

A [herdr](https://github.com/asumaran/herdr) plugin popup that lists your open
GitHub PRs (author or assignee) across the repos cloned under `~/Developer`,
grouped by repo, with fuzzy search and a preview of each PR: labels, CI
checks, review state, diff size and the rendered markdown description.
Selecting a PR lands you on its checkout:

- an existing worktree for the PR's branch is opened/focused in the herdr
  sidebar (added when missing);
- otherwise the main clone switches to the branch (offering to stash first
  when the tree is dirty) and the repo is selected in the sidebar (added when
  missing).

![asgotopr demo: popup over herdr listing open PRs with a preview, fuzzy search, jump to the PR's worktree](docs/demo.gif)

Sibling of [asgoto](https://github.com/asumaran/asgoto): same
open-pick-exit popup pattern, same fuzzy search feel, but the universe is
your PRs on GitHub instead of the workspaces already in herdr.

## Install

```
herdr plugin install asumaran/asgotopr
```

The manifest's `[[build]]` runs `scripts/fetch-binary.sh`, which downloads the
release binary matching the manifest version and falls back to `go build`
(`ASGOTOPR_BUILD_FROM_SOURCE=1` skips the download). Requires herdr >= 0.7.5 and
the [`gh` CLI](https://cli.github.com) authenticated (`gh auth login`).

Bind a key to the `open` action in `~/.config/herdr/config.toml`:

```toml
[[keys.command]]
key = ["prefix+d", "ctrl+alt+d"]
type = "plugin_action"
command = "asumaran.asgotopr.open"
description = "asgotopr (PR switcher)"
```

## Usage

The filter input is focused on open, so just type. A query of several words matches them in any order (`login fix` finds "fix login flow"), and a word starting with `'` must occur as typed instead of fuzzily (`'dex`). Search is fuzzy over the PR
title, the head branch, the PR number (`1234` finds `#1234`), the Jira ticket
key in branch/title, and the repo name.

| key | action |
| --- | --- |
| `enter` | open the selected PR's checkout |
| `ctrl+o` | open the PR in the browser instead |
| `ctrl+y` | copy the PR's URL to the clipboard; the help line confirms it |
| `↑/↓`, `ctrl+p`/`ctrl+n` | move the cursor |
| PgDn/PgUp | move the cursor a page |
| `alt+↑`/`alt+↓`, Home/End | top or bottom of the list |
| `shift+↓`/`shift+↑`, mouse wheel over the preview | scroll the description |
| mouse wheel over the list | move the cursor |
| `f1` | open the panel with every key (`esc` closes it) |
| `shift+←`/`shift+→` | resize the list; the split is remembered (the list takes a quarter of the width by default) |
| click | select a row (`enter` still opens it) |
| `esc`, `ctrl+c`, `q` with an empty filter | quit |

With a query the list is a search result: the best match comes first, with
its group on top, and the cursor starts on it. Rows that match equally well
stay in their usual order, most recently updated first.

When the PR's branch needs a checkout switch and the working tree has
uncommitted changes, asgotopr offers: `[s]` stash & switch (stash message
`asgotopr: switching to <branch>`, findable later with `git stash list`; there
is no auto-restore), `[f]` switch anyway, `[esc]` cancel. Fork PRs and
never-fetched branches are fetched via `pull/<N>/head` before switching.
In that dialog, and in the one that reports a failed switch, `esc` only steps
back to the list; `ctrl+c` quits from anywhere.

Pasting into the filter (a terminal paste or `ctrl+v`) filters like typing
does. A query of spaces only, or a bare `~` or `'`, is not a query yet: the list
stays as it is and the cursor does not move.

## Behavior notes

- PR data comes from two GraphQL searches (`author:@me` / `assignee:@me`,
  open PRs only, first 100 each, no pagination) filtered to repos with a
  GitHub `origin` cloned directly under `~/Developer`.
- Everything is cached stale-while-revalidate in the plugin state dir: the
  popup renders instantly from the last snapshot while a background refresh
  runs (skipped entirely when the snapshot is under 60s old). PR bodies are
  only refetched when the PR's `updatedAt` moved, in a single batched query.
- When the same origin is cloned twice, the PR is listed once under the
  clone whose directory name matches the remote repo name; a worktree on the
  PR's branch under any clone wins regardless.
- Worktree detection uses `git worktree list --porcelain`, so it works for
  repos not yet in the herdr sidebar and does not assume `~/wt` conventions.
- Missing/unauthenticated `gh`, network failures and rate limits degrade to
  the cached snapshot. The error takes the help line until the next key, and
  the edge over the filter keeps a red `refresh failed` mark, so a list that
  is not fresh says so.

## Development

```bash
go build -o asgotopr .   # local build (plugin runs ./asgotopr from the repo root)
./asgotopr -dump         # print repos, PRs and worktree resolution (no TTY)
./asgotopr -dump -query cart   # additionally print filter scores for a query
./asgotopr -version      # print the embedded version
go vet ./... && go test ./...
scripts/pty-check.py ./asgotopr   # end-to-end TUI check on a pty (python3 + pyte)
herdr plugin link "$PWD"   # register the working copy (no build step)
```

Runtime state (`prcache.json`, `repos.json`, the divider's `split-columns`)
lives in `HERDR_PLUGIN_STATE_DIR`; standalone runs use the same directory
(`~/.local/state/herdr/plugins/asumaran.asgotopr/`).

`ASGOTOPR_ROOT` overrides the `~/Developer` scan root. `ASGOTOPR_OPENER`
replaces the browser opener `ctrl+o` uses and `ASGOTOPR_CLIPBOARD` the
clipboard command `ctrl+y` feeds the URL to (`pbcopy` on macOS, else
`wl-copy`, `xclip` or `xsel`); the tests and the pty check point all three at
a sandbox. `ASGOTOPR_POPUP_WIDTH` / `ASGOTOPR_POPUP_HEIGHT` override the popup
size from the manifest.

## Demo recording

`docs/demo.gif` is recorded with
[asdemokit](https://github.com/asumaran/asdemokit): `asdemo
record` from the repo root replays `scripts/demo/keys.json` against an
isolated herdr session described by `scripts/demo/scenario.sh`. The scenario
points `ASGOTOPR_ROOT` at a throwaway directory of symlinks to personal repos,
so only those PRs show up; the scan follows symlinked clones for that reason.

## Releasing

`scripts/release.sh <X.Y.Z>` gates on a clean tree + green vet/build/test,
generates the CHANGELOG entry from commit subjects, syncs the manifest
version, commits, tags and publishes the GitHub release; CI then attaches
the `asgotopr-<os>-<arch>` binaries (macOS and Linux, arm64 and amd64), the assets `fetch-binary.sh` downloads on installs.
