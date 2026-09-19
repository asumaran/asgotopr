# CLAUDE.md

Guidance for working in this repository.

## What this is

`gotopr` is a herdr plugin popup that lists the user's open GitHub PRs
(author or assignee) across the repos cloned under `~/Developer`, grouped by
repo, with fuzzy search and a glamour-rendered preview of the PR description.
Selecting a PR opens its checkout: an existing worktree is focused in the
herdr sidebar via `herdr worktree open` (added when missing); otherwise the
main clone switches to the branch (offering a stash when dirty) and the repo
is selected in the sidebar. Open, pick, exit — same lifecycle as
`herdr-goto`, which this repo is modeled on.

Distributed as a herdr plugin (`herdr plugin install asumaran/gotopr`; the
manifest's `[[build]]` runs `scripts/fetch-binary.sh`). Each GitHub Release
attaches `gotopr-darwin-arm64`. There is no published library.

## Stack & layout

Go single module, single `package main`, static binary. TUI: Bubble Tea v2 +
bubbles v2 (`textinput`, `viewport`, `key`, `help`), lipgloss v2,
`sahilm/fuzzy` for matching, glamour v2 for the markdown preview. The charm
v2 modules are imported under their canonical `charm.land/<name>/v2` paths
(the `github.com/charmbracelet/<name>/v2` spelling is rejected by `go get`).
Files are split by concern but everything stays in `package main` (helpers
were lifted from goto's single-file layout):

- `main.go` — flags (`-version`, `-dump`, `-query`), model construction,
  `tea.NewProgram`, post-quit herdr exec, `runDump`.
- `github.go` — gh GraphQL searches (`author:@me`/`assignee:@me`, no bodies),
  batched body fetch, `planRefresh` (updatedAt invalidation), merge/dedup.
- `repos.go` — `~/Developer` scan → slug → clones map, mtime-keyed scan
  cache, `primaryClone` (twin-clone rule). The scan follows symlinked
  directories (a curated `GOTOPR_ROOT` of links, used by the demo).
- `git.go` — subprocess-free discovery (`resolveGitDir`, `originURL`,
  `githubSlug*`) + porcelain worktree parsing, `isDirty`, `branchExists`,
  `runGit`.
- `cache.go` — state dir resolution, `prcache.json` load/save, 60s freshness
  debounce.
- `filter.go` — entries, corpora, fuzzy hits, `matchBonus` ranking, row
  building, header-skipping navigation.
- `frame.go` — the single-frame layout shared by the family: `hline`, `fit`,
  `framed`, `frameHead`, `splitMain`, `scrollPos` and the section rows (`mainY`,
  `listY`, `frameRows`, each with or without the optional context line).
- `ui.go` — the bubbletea model/Update/View, modes, selection flow, mouse
  routing, styles.
- `preview.go` — glamour rendering as a `tea.Cmd`, per-(URL,width,updatedAt)
  render cache, instant non-glamour header (title, refs, label chips, and
  aligned Checks/Review/Diff/Opened facts, then a rule).
- `confirm.go` — stash/switch/error flow: `performSwitchCmd`, `stashCmd`,
  dialog views.
- `scripts/demo/` — the demo scenario (`scenario.sh` + `keys.json`) that
  `herdr-demo record` (asumaran/herdr-demokit, the recording tool shared by
  the herdr plugins) uses to re-record `docs/demo.gif`; see
  `scripts/demo/README.md`. Uses a disposable herdr session (`gotoprdemo`)
  and a symlink-only `GOTOPR_ROOT`, and parks the plugin's state dir during
  the take.

## Build & run

```bash
go build -o gotopr .    # plugin runs ./gotopr from the repo root
./gotopr -dump          # repos + PRs + worktree resolution, no TTY (refreshes when stale)
./gotopr -dump -query x # additionally prints filter scores
go vet ./... && go test ./...
herdr plugin link ~/Developer/gotopr   # link does NOT run [[build]]; go build yourself
```

Keybinding (user config): `prefix+d` / `ctrl+alt+d` → `plugin_action`
`asumaran.gotopr.open` → `scripts/open-pane.sh` → `herdr plugin pane open`.

## Behaviour / decisions

- **Layout**: one rounded frame of sections split by shared edges, the layout
  asgitlog introduced and every picker of the family follows (`frame.go`, the
  same file in each repo): the filter input (the border over it carries the
  matches/total counter and the refresh mark), the main section (list and
  preview split by a divider; its bottom edge carries the preview's scroll
  position), and the help. A context line on top is only for what the rest of
  the screen cannot say (asgitlog: repo and branch); a title is not context,
  so there is none here. The list starts on screen row `listY`, one cell in
  from the left side, which is what the click-to-row math uses. Errors and
  notices take the help line.
- **Data**: two GraphQL searches fetch metadata WITHOUT bodies; bodies come
  in one aliased query only for PRs whose `updatedAt` moved past the cache
  (body edits always bump updatedAt, so it is a safe invalidation key). On a
  bodies-fetch failure the fresh list is shown but NOT persisted, so the next
  run refetches instead of freezing empty bodies. The header metadata (labels,
  head-commit check rollup with per-context counts, reviewDecision, latest
  approvals, pending review requests, mergeable, diff stats, comment/commit
  counts, base ref, author, createdAt) rides on the search query itself:
  checks and reviews do not bump `updatedAt`, so they are refetched on every
  refresh instead of being invalidated. `-dump` prints a second line per PR
  with checks/review/labels.
- **Preview header height is dynamic**: labels wrap and failed checks add a
  line, so `syncPreviewHeight` (called from `resize` and `updatePreview`)
  measures `previewHeader` with `lipgloss.Height` and fits the body viewport
  under it. It is not tied to the preview key because a refresh can change the
  header (new check results) without changing `updatedAt`.
- **Caching**: stale-while-revalidate; first frame always renders from
  `prcache.json`; snapshots fresher than 60s skip the refresh. The repo scan
  caches parsed slugs keyed by `.git/config` mtime in `repos.json`.
- **Selection**: worktree-for-branch is resolved via
  `git worktree list --porcelain` across ALL clones of the slug; a hit maps
  to `herdr worktree open --cwd <clone> --path <checkout> --focus`
  (idempotent, creates sidebar entries). No hit → dirty check → optional
  stash (`git stash push -u -m "gotopr: switching to <branch>"`, never
  auto-restored) → fetch `pull/<N>/head:<branch>` when the branch is missing
  (covers forks) → `git switch` → `worktree open` on the repo root itself.
- **Browser**: `ctrl+o` queues the selected PR's URL (`m.browse`) and quits;
  `runAction` opens it after the TUI exits (same post-quit rule as the herdr
  action) and never touches the checkout. `openURL` prefers an AppleScript
  `make new tab` in Chrome's front window when Chrome is running with a
  window, because `open <url>` lets Chrome pick its `profile.last_used`,
  which is not the last focused window/profile. Falls back to `open`;
  `GOTOPR_OPENER` replaces the whole thing.
- **Errors surface inside the TUI** (modeError); the herdr action runs only
  after quit, because quitting closes the popup and post-exit output is lost.
- **Never query the terminal behind bubbletea's back**: only the program
  owns stdin, so `Init` issues `tea.RequestBackgroundColor()` and the
  `tea.BackgroundColorMsg` reply picks the glamour style (`IsDark()` →
  "dark"/"light"). Frames before the reply render with "dark"; when the style
  flips, the render cache is cleared and the current preview re-renders
  (renders carry the style they used and stale ones are dropped). Don't call
  glamour's `WithAutoStyle` or lipgloss's `HasDarkBackground` from inside the
  program: their OSC reply would race the input reader.
- **Mouse**: the wheel always scrolls the description preview, wherever the
  pointer is. Routing by column (and a per-gesture latch to survive trackpad
  inertia) was tried and dropped: SGR reports carry no gesture phase, so
  inertia drifting over the other column is indistinguishable from a new
  gesture and every time-based heuristic misroutes one of the two. The list
  is driven by the keys; a left click on a list row moves the selection there
  (`handleClick`: row = Y-listY + list YOffset, inside the frame's left side) and never opens the PR; opening
  stays on enter. The mouse mode is declared per frame in `View()`
  (`MouseModeCellMotion` in modeFilter, `MouseModeNone` in the
  confirm/busy/error dialogs). v2's input parser reassembles SGR reports split
  across reads, so fast wheel bursts never leak into the filter (v1 needed a
  workaround for that).
- **Alt screen** is also declared per frame (`tea.View.AltScreen`); there is
  no `tea.WithAltScreen` program option in v2.
- Same-origin twin clones: PR listed once under `primaryClone` (dir name ==
  remote repo name, else lexicographic); no per-clone duplicate rows.
- Search corpus: title + branch + `#number`/ticket/slug/dirname metas; exact
  PR number +30, exact repo name +10, branch hit +2, draft −2, ties broken by
  newer `updatedAt`. Digits are plain search text.

## Testing

Unit tests cover the pure logic (parsing, merge/dedup, invalidation, ranking,
grouping, mode transitions, View content, wheel routing, background-color
style flip). For end-to-end TUI verification without a TTY,
`scripts/pty-check.py ./gotopr [dark|light]` (python3 + `pyte`) spawns the
binary on a pty, answers the OSC 10/11 + CSI 6n + DA1 queries, replays
keystrokes and SGR wheel bursts, and asserts on pyte-rendered frames (prompt
stays clean, each column scrolls on its own, keys unchanged, clean exit). It
runs against a throwaway sandbox (`GOTOPR_ROOT` of fake clones, a synthetic
`prcache.json` in `HERDR_PLUGIN_STATE_DIR`, a fake `HERDR_BIN_PATH` that logs
argv) and never touches the real plugin state.

## Commits & branches

- Conventional Commits: `type(scope): description`.
- Never mention AI tooling in commits, PRs, or any repo-visible text.
- Default branch is `main`. Don't commit, tag, or push unless explicitly
  asked (releasing is an explicit, separate request).

## Releasing

`scripts/release.sh <X.Y.Z>` — clean-tree + vet/build/test gate, CHANGELOG
generation from commit subjects, manifest version sync, commit + tag + GitHub
release; CI (`.github/workflows/release.yml`) attaches `gotopr-darwin-arm64`.
Releasing never touches the linked plugin's `./gotopr`; rebuild locally to
keep testing dev code.
