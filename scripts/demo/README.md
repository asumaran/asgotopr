# Demo recording

Scenario for re-recording the README demo GIF (`docs/demo.gif`) with
[herdr-demokit](https://github.com/asumaran/herdr-demokit):

```bash
herdr-demo record            # from the repo root; writes docs/demo.gif
herdr-demo doctor            # check the toolchain first
```

- `scenario.sh` — the isolated herdr session (`gotoprdemo`): the personal
  repos and shopnest worktrees in the sidebar, the bottom splits, and two
  things specific to gotopr. `GOTOPR_ROOT` is pointed (through the session
  server's env) at a throwaway directory holding symlinks to personal repos
  only, so the popup lists their PRs and none from work clones under
  `~/Developer`; and the plugin's state dir
  (`~/.local/state/herdr/plugins/asumaran.gotopr`) is parked during the take
  and restored afterwards, since herdr hands the plugin the same state dir in
  every session and the demo would otherwise leave the real cache holding
  the demo root's data. `demo_setup` warms that state with `./gotopr -dump`
  so the first popup renders instantly. `demo_build` stamps `./gotopr` with
  the manifest version; `demo_teardown` restores the dev build.
- `keys.json` — `prefix+d` -> popup -> type `access` -> enter (lands on the
  `fix/SHOP-4-product-card-accessibility` worktree) -> `prefix+d` -> type
  `hasprod` -> enter (`feat/cart-has-product`). Both PRs have worktrees, so
  no checkout switch happens on camera.

Besides the kit's toolchain, this needs the `asumaran.gotopr` plugin
registered with the `prefix+d` `plugin_action` keybind, `gh` authenticated,
and the repos/worktrees listed in `scenario.sh` to exist with their PRs
open.
