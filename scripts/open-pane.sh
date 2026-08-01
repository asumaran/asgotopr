#!/bin/sh
# Entry point for the "open" plugin action: opens the picker pane (placement
# and size come from the [[panes]] entry in herdr-plugin.toml). GOTOPR_POPUP_WIDTH
# / GOTOPR_POPUP_HEIGHT (e.g. "60%") override the manifest size. Plugin
# commands are argv arrays with no shell expansion, so this wrapper exists to
# resolve HERDR_BIN_PATH and the overrides at runtime.
set -eu
exec "${HERDR_BIN_PATH:-herdr}" plugin pane open --plugin asumaran.gotopr --entrypoint picker \
  ${GOTOPR_POPUP_WIDTH:+--width "$GOTOPR_POPUP_WIDTH"} \
  ${GOTOPR_POPUP_HEIGHT:+--height "$GOTOPR_POPUP_HEIGHT"}
