## v0.14.1 (2026-09-24)

* refactor(ui): take the shared frame without the context line (00f14b0)

## v0.14.0 (2026-09-21)

* docs: describe the new shared files and flashes (f173e08)
* refactor: share the runners and the preview tail (b1f4c15)
* docs: describe the opener, -dump and the header (94a0f1e)
* fix(ui): fixed preview header, report failures (42f9a10)
* docs: match the docs to the shared helpers (9fe1b4f)
* fix(ui): quit on ctrl+c anywhere, mark bad refresh (28572e6)
* test(pty): cover q, name the docs' bullets alike (44ef998)
* fix(list): say why the list is empty, in the list (5f38cb8)

## v0.13.2 (2026-09-20)

* docs(readme): list the keys in a table (135610b)
* refactor(list): keep the cursor in view with scrollTo (bc3c795)

## v0.13.1 (2026-09-20)

* refactor(git): run git through the shared runGit (986328b)

## v0.13.0 (2026-09-20)

* feat(ui): open an options and keys panel with f1 (b71626d)

## v0.12.0 (2026-09-20)

* feat(keys): copy the PR URL with ctrl+y (4f3fbda)

## v0.11.0 (2026-09-20)

* refactor(state): share one state dir with the shell (fee4bbd)
* feat(search): match query terms in any order (858e757)

## v0.10.0 (2026-09-19)

* feat(ui): move the counter under the list (49db6c2)

## v0.9.2 (2026-09-19)

* fix: shorten only paths inside the home dir (f028725)

## v0.9.1 (2026-09-19)

* refactor: share the last duplicated helpers (70d505f)

## v0.9.0 (2026-09-19)

* feat(ui): placeholder in the filter, name only standalone (dc203d3)
* feat(ui): page the list, jump to its ends, expand the help (75fd1c7)
* feat(filter): rank the rows while a query is on (55b9e13)
* refactor(ui): one highlight implementation for the family (24f1f3a)
* fix(filter): match the query where it occurs whole (3568238)

## v0.8.0 (2026-09-19)

* feat(ui): mark matches like asgitlog, selected row too (3c29ce4)
* feat(ui): show the list's position under the list (a564ef4)
* docs(dev): link the plugin from the checkout with $PWD (6b87378)
* ci: spend less time on CI and on releases (7f608b6)

## v0.7.0 (2026-09-19)

* feat: support linux and share the release process (c5aec13)

## v0.6.0 (2026-09-19)

* refactor: rename gotopr to asgotopr (331c3e6)

## v0.5.0 (2026-09-19)

* feat(mouse): move the selection with the wheel over the list (193dc01)
* feat(ui): resize the list with shift+arrows (02bfd12)
* docs(demo): re-record the GIF with the single-frame layout (240b945)

## v0.4.0 (2026-09-18)

* feat(ui): adopt the family's single-frame layout (50517b6)
* ci: run gofmt, vet and tests on push (0c4089a)
* chore: add the MIT license (d628a4d)
* fix(release): pass a tag message so signed tags work headless (9611bd4)

## v0.3.0 (2026-09-11)

* docs: re-record the demo GIF (a390675)
* chore(plugin): raise the popup height to 90% (5a9c737)
* feat(ui): open the selected PR in the browser (9e089dc)
* fix(ui): scroll only the preview with the wheel (6f9f98f)
* feat(preview): show labels, checks and review state (07395b4)
* feat(ui): extend the selected row across the list (5138b4c)
* feat(ui): select a PR with a left click (ffd785c)
* fix(ui): latch the wheel target column per gesture (fea9bc6)
* feat(ui): move the selection with the mouse wheel (8a71118)
* test: add pty driver for end-to-end TUI checks (09f78ef)
* feat(ui): scroll columns independently with the mouse wheel (fde966b)
* refactor(ui): migrate to bubbletea v2 (1c6d7e5)
* feat(demo): add scripted README demo GIF (5c9f79f)

## v0.2.0 (2026-08-05)

* feat(ui): widen preview pane to a 30/70 list/preview split (1a01111)

## v0.1.0 (2026-08-01)

* ci: add release tooling and binary workflow (85d669c)
* docs: add README, contributor guide and changelog (bbdb52b)
* test: cover parsing, ranking and mode transitions (ba68df4)
* feat(ui): picker popup, preview and switch flow (36e9839)
* feat(filter): fuzzy search and ranking over PRs (e90afa9)
* feat(github): fetch open PRs via gh with caching (41628e5)
* feat(repos): discover local clones and git state (caf46ea)
* chore: scaffold herdr plugin manifest and module (be3993d)
