# shellcheck shell=bash
# scenario.sh — demo session for the README GIF, run by `herdr-demo record`
# (asumaran/herdr-demokit). Sourced by the kit; the helpers used below
# (demo_*) come from it.

DEMO_SESSION="gotoprdemo"
DEMO_OUT="docs/demo.gif"
DEMO_START_CWD="$HOME/Developer/asdev"

# gotopr lists every open PR of the account across the clones under its scan
# root, which for a public GIF must not be ~/Developer (work repos). The demo
# scans a throwaway root holding symlinks to personal repos only; the scan
# follows links, so the PRs resolve to the real checkouts and worktrees.
DEMO_SCAN_ROOT="${TMPDIR:-/tmp}/gotopr-demo-root"
DEMO_SESSION_ENV=(GOTOPR_ROOT="$DEMO_SCAN_ROOT")
SCAN_REPOS=(
  "$HOME/Developer/shopnest"
)

# Same sidebar as the goto demo: personal repos, shopnest's worktrees (each
# on a PR branch, so selecting those PRs focuses them without a checkout
# switch), bottom splits on the workspaces the demo visits.
REPOS=(
  "$HOME/Developer/shopnest"
  "$HOME/Developer/worktree-cli"
  "$HOME/Developer/asreviewer"
  "$HOME/Developer/aspage"
)
WORKTREES=(
  "$HOME/Developer/shopnest:$HOME/wt/shopnest/feat-cart-has-product"
  "$HOME/Developer/shopnest:$HOME/wt/shopnest/fix-SHOP-4-product-card-accessibility"
  "$HOME/Developer/shopnest:$HOME/wt/shopnest/test-format-price-util"
)
SPLITS=(
  "$HOME/Developer/asdev"
  "$HOME/wt/shopnest/fix-SHOP-4-product-card-accessibility"
  "$HOME/wt/shopnest/feat-cart-has-product"
)

# herdr hands every plugin one state dir regardless of session, so the demo
# run would leave prcache.json/repos.json holding the demo root's data. Park
# the real state during the recording and put it back afterwards.
STATE_DIR="$HOME/.local/state/herdr/plugins/asumaran.gotopr"
STATE_BACKUP=""

demo_build() {
  local version repo
  version="$(sed -n 's/^version = "\(.*\)"/\1/p' herdr-plugin.toml)"
  go build -ldflags "-X main.version=v${version}" -o gotopr .

  rm -rf "$DEMO_SCAN_ROOT"
  mkdir -p "$DEMO_SCAN_ROOT"
  for repo in "${SCAN_REPOS[@]}"; do ln -s "$repo" "$DEMO_SCAN_ROOT/$(basename "$repo")"; done

  if [[ -d "$STATE_DIR" ]]; then
    STATE_BACKUP="$(mktemp -d -t gotopr-state)"
    cp -R "$STATE_DIR/." "$STATE_BACKUP/"
    rm -rf "$STATE_DIR"
  fi
}

demo_teardown() {
  go build -o gotopr . 2>/dev/null || true
  rm -rf "$DEMO_SCAN_ROOT"
  if [[ -n "$STATE_BACKUP" ]]; then
    rm -rf "$STATE_DIR"
    mkdir -p "$STATE_DIR"
    cp -R "$STATE_BACKUP/." "$STATE_DIR/"
    rm -rf "$STATE_BACKUP"
  fi
}

demo_setup() {
  local repo pair target
  # Warm the (parked, now empty) state with the demo root's PRs so the first
  # popup renders the list instantly instead of an empty "refreshing…" frame.
  HERDR_PLUGIN_STATE_DIR="$STATE_DIR" GOTOPR_ROOT="$DEMO_SCAN_ROOT" ./gotopr -dump >/dev/null

  demo_adopt_repo "$(demo_first_workspace)" "$DEMO_START_CWD"
  for repo in "${REPOS[@]}"; do demo_open_repo "$repo" >/dev/null; done
  for pair in "${WORKTREES[@]}"; do demo_open_worktree "${pair%%:*}" "${pair#*:}"; done
  for target in "${SPLITS[@]}"; do demo_split_below "$target" >/dev/null; done
}
