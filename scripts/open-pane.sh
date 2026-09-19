#!/bin/sh
# Entry point for the "open" plugin action: opens the picker pane (placement
# and size come from the [[panes]] entry in herdr-plugin.toml). ASGOTOPR_POPUP_WIDTH
# / ASGOTOPR_POPUP_HEIGHT (e.g. "60%") override the manifest size. Plugin
# commands are argv arrays with no shell expansion, so this wrapper exists to
# resolve HERDR_BIN_PATH and the overrides at runtime.
set -eu
exec "${HERDR_BIN_PATH:-herdr}" plugin pane open --plugin asumaran.asgotopr --entrypoint picker \
  ${ASGOTOPR_POPUP_WIDTH:+--width "$ASGOTOPR_POPUP_WIDTH"} \
  ${ASGOTOPR_POPUP_HEIGHT:+--height "$ASGOTOPR_POPUP_HEIGHT"}
