# CLAUDE.md

Guidance for working in this repository.

## What this is

`asgotopr` is a herdr plugin popup that lists the user's open GitHub PRs
(author or assignee) across the repos cloned under `~/Developer`, grouped by
repo, with fuzzy search and a glamour-rendered preview of the PR description.
Selecting a PR opens its checkout: an existing worktree is focused in the
herdr sidebar via `herdr worktree open` (added when missing); otherwise the
main clone switches to the branch (offering a stash when dirty) and the repo
is selected in the sidebar. Open, pick, exit: the same lifecycle as
`asgoto`, which this repo is modeled on.

Distributed as a herdr plugin (`herdr plugin install asumaran/asgotopr`; the
manifest's `[[build]]` runs `scripts/fetch-binary.sh`). Each GitHub Release
attaches the `asgotopr-<os>-<arch>` binaries (macOS and Linux, arm64 and amd64). There is no published library.

## Stack & layout

Go single module, single `package main`, static binary. TUI: Bubble Tea v2 +
bubbles v2 (`textinput`, `viewport`, `key`, `help`), lipgloss v2,
`sahilm/fuzzy` for matching, glamour v2 for the markdown preview. The charm
v2 modules are imported under their canonical `charm.land/<name>/v2` paths
(the `github.com/charmbracelet/<name>/v2` spelling is rejected by `go get`).
Files are split by concern but everything stays in `package main` (helpers
were lifted from asgoto's single-file layout):

- `main.go`: flags (`-version`, `-dump`, `-query`), model construction,
  `tea.NewProgram`, post-quit herdr exec, `runDump`.
- `github.go`: gh GraphQL searches (`author:@me`/`assignee:@me`, no bodies),
  batched body fetch, `planRefresh` (updatedAt invalidation), merge/dedup.
- `repos.go`: `~/Developer` scan → slug → clones map, mtime-keyed scan
  cache, `primaryClone` (twin-clone rule). The scan follows symlinked
  directories (a curated `ASGOTOPR_ROOT` of links, used by the demo).
- `git.go`: porcelain worktree parsing, `isDirty`, `branchExists`, `gitIn` /
  `gitDo` (git in a given checkout). The subprocess-free remote discovery is
  in `gitremote.go`.
- `gitrun.go`: `runGit`: git in the current directory, with git's own stderr
  as the error, and `insideWorkTree`. The same file in every tool of the
  family that needs it.
- `cache.go`: `prcache.json` load/save, 60s freshness debounce, and
  `stateDir()`, a wrapper over `stateDirFor` (`statedir.go`).
- `filter.go`: entries, corpora, fuzzy hits, `matchBonus` ranking, row
  building, header-skipping navigation.
- `match.go`: `findTight`/`tighten`, the fuzzy matcher with one correction: it
  is greedy (first candidate for each rune, left to right), so a query that
  occurs in one piece could still match scattered letters before it. When the
  query occurs whole, that occurrence is the match, for the highlight and for
  the score. `hasTerms` says whether a query searches for anything: spaces and
  a bare `~` or `'` do not, so they never filter, rank or move the cursor. The
  same file in every tool of the family.
- `text.go`: `truncate`, `padRight`, `padLeft`: fitting text, styled or not,
  into cells. The same file in every tool of the family.
- `statedir.go`: `stateDirFor`: the state dir herdr injects
  (`HERDR_PLUGIN_STATE_DIR`) or, when the tool runs on its own, the same
  directory worked out
  (`${XDG_STATE_HOME:-~/.local/state}/herdr/plugins/asumaran.asgotopr`), so
  the popup and a run from the shell share settings and caches. The same file
  in every tool of the family.
- `age.go`: `compactAge` (`5m`, `3h`, `2d`, `6w`, `2y`) for a list column and
  `relTime` (`3h ago`) for a sentence. The same file in every tool of the
  family that shows an age.
- `markdown.go`: `renderMarkdown` (glamour with a fixed style, never
  auto-detected), `glamourStyle` and `setPreviewStyle`. The same file in every
  tool of the family that renders Markdown.
- `listmouse.go`: `inList`, `rowUnder`, `wheelKey`: the mouse over the list.
  The wheel goes through the same code as the arrows; a click moves the
  cursor and never opens anything. The same file in every tool of the family.
- `prompt.go`: the filter input: its prompt (with the tool's name only outside
  herdr's popup), the placeholder, the `(dev)` mark on the edge over the
  input. `typeInto` hands a message to the input and reports whether the query
  changed: a key, a terminal paste and the input's own `ctrl+v` all edit it,
  and the caller filters again only when it did. The same file in every tool
  of the family.
- `helpfoot.go`: the help line at the foot, cut to the width, and the key that
  opens the panel. `footLine` is what the foot shows: a flash first, then a
  notice in the error color, else the help. The same file in every tool of the
  family.
- `panel.go`: the panel `f1` opens over the frame: options to change in
  place and every key under them (`option`, `panel`, `panelLines`,
  `overlay`). The same file in every tool of the family.
- `listnav.go`: `listNav`: the keys that move the cursor through a list and
  where each one takes it, group headers skipped. `scrollTo` keeps the cursor
  in view together with the row `withHeader` names: the header of its group
  when that is the row right above. `emptyList` is what a list says instead of
  rows: the error that kept it from loading, in the error color, `No matches`,
  or the tool's own reason. The same file in every tool of the family.
- `highlight.go`: `highlight`/`highlightFrom`, `matchOver`, `onSel`,
  `selPad` and the `stSel`/`stMatch` styles: how a match and the selected row
  look. The same file in every tool of the family.
- `flash.go`: `flash`, `flashMsg`, `clearFlashMsg`: a confirmation that takes
  the help line for a moment. The same file in every tool of the family.
- `clipboard.go`: `copyCmd`: feeds a text to the system clipboard and reports
  it with a `flashMsg`; `ASGOTOPR_CLIPBOARD` replaces the command. The same
  file in every tool of the family.
- `border.go`: `hline`, `framed`, `fit`, `scrollPos`: the primitives the frame
  is drawn with (an edge with texts set into it, a line between the frame's
  sides, the position a scrolled viewport reports on an edge). `fitLines` is
  content as exactly so many lines of a width, and `popupView` is the
  `tea.View` every tool returns: the alt screen and, while the mouse is on,
  cell-motion mouse reports. The same file in every tool of the family.
- `homepath.go`: `tildePath`, `homeDir`, `homeRel`: a path with the home
  directory abbreviated to `~`. The same file in every tool of the family that
  shows paths.
- `refreshmark.go`: `refreshMark`: what the edge over the input says about a
  list that is fetched in the background and shown from a cache meanwhile:
  `refreshing…` while the fetch runs and, after one that failed, a standing
  `refresh failed` in the error color. The same file in every tool of the
  family that needs it.
- `rank.go`: `rank`: with a query the list is a search result, best score
  first; in a grouped list the groups go by their best item and keep their
  items together, and equal scores keep the list's own order. The same file in
  every tool of the family that ranks its matches.
- `herdrbin.go`: `herdrBin`: where the herdr executable is (`HERDR_BIN_PATH`,
  which the server hands to plugin commands, else `herdr` on `PATH`). The same
  file in every tool of the family that talks to herdr.
- `gitremote.go`: `resolveGitDir`, `originURL`, `githubSlug`,
  `githubSlugFromURL`: what a checkout says about its remote, read straight
  from the filesystem with no subprocess, so scanning dozens of repos at
  startup stays in the low milliseconds. The same file in every tool of the
  family that needs it.
- `ticket.go`: `ticketFrom`: the ticket key (`KEY-123`, uppercased) found in a
  branch name, a title or a folder name. The same file in every tool of the
  family that needs it.
- `openurl.go`: `openURL`: hands a URL to the browser. On macOS a Chrome that
  is already up gets a new tab in its front window, else `open`; `xdg-open`
  elsewhere; `ASGOTOPR_OPENER` replaces all of it. The same file in every tool
  of the family that opens one.
- `frame.go`: the single-frame layout the pickers share: `frameHead`,
  `splitMain` (list and preview) and the section rows (`mainY`, `listY`,
  `frameRows`, each with or without the optional context line), drawn with the
  primitives of `border.go`. Copied, not imported: the same file ships in
  asgoto, asgotoissues, asgotonotes, asgotosession and asgotochanged (all
  under github.com/asumaran), and there is no shared library. A pull request
  only needs to change it here; the maintainer ports the change to the other
  copies.
- `split.go`: the divider between the list and the preview: `loadSplit`,
  `saveSplit`, `stepSplit`, `splitWidths`, `moveSplit` (one step, remembered)
  and `sizePanes` (the list and the preview get their share of the main
  section). Copied, not imported, like `frame.go`: the same file ships in
  asgotoissues, asgotonotes, asgotosession and asgotochanged.
- `ui.go`: the bubbletea model/Update/View, modes, selection flow, mouse
  routing, styles.
- `preview.go`: glamour rendering as a `tea.Cmd`, per-(URL,width,updatedAt)
  render cache, instant non-glamour header (title, refs, label chips, and
  aligned Checks/Review/Diff/Opened facts; `rightColumn` puts one blank line
  under it).
- `confirm.go`: stash/switch/error flow: `performSwitchCmd`, `stashCmd`,
  dialog views.
- `scripts/demo/`: the demo scenario (`scenario.sh` + `keys.json`) that
  `asdemo record` (asumaran/asdemokit, the recording tool shared by
  the herdr plugins) uses to re-record `docs/demo.gif`; see
  `scripts/demo/README.md`. Uses a disposable herdr session (`asgotoprdemo`)
  and a symlink-only `ASGOTOPR_ROOT`, and parks the plugin's state dir during
  the take.

## Build & run

```bash
go build -o asgotopr .    # plugin runs ./asgotopr from the repo root
./asgotopr -dump          # repos + PRs + worktree resolution, no TTY (refreshes when stale)
./asgotopr -dump -query x # the matches and their scores instead of the list
go vet ./... && go test ./...
scripts/pty-check.py ./asgotopr   # end-to-end TUI check on a pty (python3 + pyte)
herdr plugin link "$PWD"   # link does NOT run [[build]]; go build yourself
```

Keybinding (user config): `prefix+d` / `ctrl+alt+d` → `plugin_action`
`asumaran.asgotopr.open` → `scripts/open-pane.sh` → `herdr plugin pane open`.

## Behaviour / decisions

- **Layout**: one rounded frame of sections split by shared edges, the layout
  asgitlog introduced and every picker of the family follows (`frame.go`, the
  same file in each repo): the filter input (the border over it carries the
  refresh mark), the main section (list and preview split by a divider; its bottom edge carries the
  matches/total counter under the list and, while the preview overflows, its
  scroll position on the right), and the help. A context line on top is only for what the
  rest of the screen cannot say (asgitlog: repo and branch); a title is not
  context, so there is none here. The list starts on screen row `listY`, one
  cell in from the left side, which is what the click-to-row math uses. Errors
  and notices take the help line.
- **Moving through the list** is the same in every tool of the family and
  comes from `listnav.go` (the same file in each repo): arrows or
  `ctrl+p`/`ctrl+n` a row, `pgup`/`pgdn` a page, `alt+↑`/`alt+↓` or
  `home`/`end` the ends. `home`/`end` are taken from the filter input's caret
  on purpose (`←`/`→` and `ctrl+e` still move it). The preview scrolls with
  `shift+↑`/`shift+↓` only. The keys are listed in the panel.
- **The filter input** comes from `prompt.go` (the same file in every tool of
  the family). Inside herdr's popup the prompt is the arrow alone, because the
  pane's title (`[[panes]] title` in the manifest, the tool's name) already
  says which tool it is, and a placeholder says what the filter searches. Run
  on its own the prompt carries the tool's name. A build that is not a release
  says `(dev)` on the edge over the input, never inside the
  prompt.
  herdr sets `HERDR_PLUGIN_ENTRYPOINT_ID` for a plugin pane; that is how the
  two cases are told apart.
  Whatever reaches the input goes through `toInput`: a key, a paste from the
  terminal (`tea.PasteMsg`) and the input's own `ctrl+v` filter the list the
  same way (`typeInto`), and a message that leaves the query alone (a caret
  move, the blink) never moves the cursor. A paste under the open panel is
  dropped. A query made only of spaces, or a bare `~` or `'`, is not a query
  (`hasTerms`): it does not filter, rank or move the cursor.
- **Help and options**: the line at the foot shows the tool's own actions,
  the panel's key and the quit keys (`helpfoot.go`). `f1` opens the panel (`panel.go`, the same file in
  every tool of the family): the options on top, to change with `←`/`→` or
  `space`, and every key in columns under them, laid out by bubbles' `help`
  from `FullHelp()`. The panel is spliced over the middle of the frame, which
  keeps its size; while it is open it takes every key and the mouse, and `esc`
  closes it before it does anything else. `?` is not a help key: the filter
  has the focus, so it is text. Moving, scrolling and resizing are listed in
  the panel only, so the help line stays short enough for a narrow popup. A
  message (error, notice) takes the help line's place.
  This tool has no options, so the panel lists the keys alone and the help
  line says `f1 help`.
- **Filter matches** look the same in every tool of the family and come from
  one place, `highlight.go` (the same file in each repo; it also owns `stSel`
  and `stMatch`): a match is the match color plus an underline on top of the
  style the text already has, and the selected row shows them too. That row
  is never one big `stSel.Render` around styled text, because the reset that
  ends a match would cut the background: every piece is rendered over `stSel`
  (`highlight(s, idx, stSel)`) and `selPad` fills the rest. Do not write a
  local highlighter.
- **Resizable list**: `shift+←/→` move the divider in 5% steps, as in
  asgitlog. The setting is the PREVIEW's share of the width, clamped to
  30-85 and saved as `split-columns` in the state dir; the default is 75
  (list 25%, preview 75%), the same in every picker of the family. Rows
  must degrade for a narrow list instead of truncating their last columns.
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
  stash (`git stash push -u -m "asgotopr: switching to <branch>"`, never
  auto-restored) → fetch `pull/<N>/head:<branch>` when the branch is missing
  (covers forks) → `git switch` → `worktree open` on the repo root itself.
- **Browser**: `ctrl+o` queues the selected PR's URL (`m.browse`) and quits;
  `runAction` opens it after the TUI exits (same post-quit rule as the herdr
  action) and never touches the checkout. `openURL` prefers an AppleScript
  `make new tab` in Chrome's front window when Chrome is running with a
  window, because `open <url>` lets Chrome pick its `profile.last_used`,
  which is not the last focused window/profile. Falls back to `open`;
  `ASGOTOPR_OPENER` replaces the whole thing. `openURL` lives in `openurl.go`,
  the same file in every tool that opens the browser.
- **Copy**: `ctrl+y` copies the selected PR's URL (`copyCmd` in the shared
  `clipboard.go`) and the help line confirms it for a moment (`flash.go`,
  shown by the shared `footLine` ahead of an error). `ASGOTOPR_CLIPBOARD` replaces the
  clipboard command (the tests point it at a stub).
- **Errors surface inside the TUI**: a failed switch or stash opens the error
  dialog (`modeError`); the herdr action runs only after quit, because
  quitting closes the popup and post-exit output is lost. `ctrl+c` quits from
  every mode, the confirm and error dialogs included, as in every tool of the
  family; `esc` (and `enter` on the error) only steps back to the list.
- **A failed refresh** keeps the cached list on screen. The error (`netErr`)
  takes the help line through the shared `footLine`, in the error color, until
  the next key gives the help back; the edge over the input keeps a red
  `refresh failed` mark (`refreshMark`, `m.stale`) for as long as the list is
  the cached one, so the state outlives the message. An error that keeps a
  list from loading at all is shown in the list in the error color in every
  tool of the family (`emptyList`); here a fetch error never empties the
  list, so it has no such case.
- **Never query the terminal behind bubbletea's back**: only the program
  owns stdin, so `Init` issues `tea.RequestBackgroundColor()` and the
  `tea.BackgroundColorMsg` reply picks the glamour style (`IsDark()` →
  "dark"/"light"). Frames before the reply render with "dark"; when the style
  flips, the render cache is cleared and the current preview re-renders
  (renders carry the style they used and stale ones are dropped). Don't call
  glamour's `WithAutoStyle` or lipgloss's `HasDarkBackground` from inside the
  program: their OSC reply would race the input reader.
- **Mouse**: the wheel follows the pointer, as in asgitlog: over the list it
  moves the selection (`overList`, one PR per report, through the same code as
  the arrow keys), anywhere else it scrolls the description preview. For a
  while every report went to the preview, because SGR reports carry no gesture
  phase and trackpad inertia drifting over the other column cannot be told
  from a new gesture; a list the wheel did nothing on was the worse of the
  two. Do not add a time-based latch: every one tried misrouted one case or
  the other. A left click on a list row moves the selection there
  (`handleClick`: row = Y-listY + list YOffset, inside the frame's left side) and never opens the PR; opening
  stays on enter. The mouse mode is declared per frame in `View()`
  (`MouseModeCellMotion` in modeFilter, `MouseModeNone` in the
  confirm/busy/error dialogs). v2's input parser reassembles SGR reports split
  across reads, so fast wheel bursts never leak into the filter (v1 needed a
  workaround for that).
- **Alt screen and mouse mode** are declared per frame in `View()`; there is
  no `tea.WithAltScreen` program option in v2.
- Same-origin twin clones: PR listed once under `primaryClone` (dir name ==
  remote repo name, else lexicographic); no per-clone duplicate rows.
- Search corpus: title + branch + `#number`/ticket/slug/dirname metas; exact
  PR number +30, exact repo name +10, branch hit +2, draft −2. Digits are
  plain search text.
- **A query makes the list a search result**: PRs are ranked, best match
  first, the repo that holds it on top with its PRs kept together, and the
  cursor sits on the first one (`rank` in `rank.go`, the same file in every
  picker of the family). Equal scores go to the newer `updatedAt`. A score
  says how good the match is and nothing about the length of the text
  (`match.go`).

## Testing

Unit tests cover the pure logic (parsing, merge/dedup, invalidation, ranking,
grouping, mode transitions, View content, wheel routing, background-color
style flip). For end-to-end TUI verification without a TTY,
`scripts/pty-check.py ./asgotopr [dark|light]` (python3 + `pyte`) spawns the
binary on a pty, answers the OSC 10/11 + CSI 6n + DA1 queries, replays
keystrokes and SGR wheel bursts, and asserts on pyte-rendered frames (prompt
stays clean, each column scrolls on its own, keys unchanged, clean exit). It
runs against a throwaway sandbox (`ASGOTOPR_ROOT` of fake clones, a synthetic
`prcache.json` in `HERDR_PLUGIN_STATE_DIR`, a fake `HERDR_BIN_PATH` that logs
argv) and never touches the real plugin state.

## Commits & branches

- Conventional Commits: `type(scope): description` (feat, fix, chore, docs,
  style, refactor, test, perf).
- Never mention AI tooling in commits, PRs, or any repo-visible text as the
  author of changes.
- Default branch is `main`. Don't commit, tag, or push unless explicitly
  asked (releasing is an explicit, separate request).

## Releasing

`scripts/release.sh <X.Y.Z>`: clean-tree + vet/build/test gate, CHANGELOG
generation from commit subjects, manifest version sync, commit + tag + GitHub
release; CI (`.github/workflows/release.yml`) attaches the `asgotopr-<os>-<arch>` binaries (macOS and Linux, arm64 and amd64).
Releasing never touches the linked plugin's `./asgotopr`; rebuild locally to
keep testing dev code.
