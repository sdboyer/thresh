#!/usr/bin/env bash
# Thresh's reportorial surface: a Claude Code Stop hook. At the end of a turn it
# taps the shoulder about consequential boundaries the working tree touched.
#
# Safe to install GLOBALLY (~/.claude/settings.json): it is silent and free in any
# directory without a .thresh/, and it never blocks -- notify, not gate.
#
# Install:  copy this file to ~/.claude/hooks/run-thresh.sh (chmod +x) and add
#   "hooks": {"Stop": [{"matcher": "", "hooks": [{"type": "command",
#     "command": "~/.claude/hooks/run-thresh.sh", "timeout": 30}]}]}
# The `thresh` binary should be on PATH (go install from the thresh checkout);
# GOPATH/bin is tried as a fallback for GUI-launched sessions with a thin PATH.
set -uo pipefail
INPUT=$(cat)

# Never participate in continuation loops.
if [ "$(printf '%s' "$INPUT" | jq -r '.stop_hook_active // false')" = "true" ]; then
  exit 0
fi

CWD=$(printf '%s' "$INPUT" | jq -r '.cwd // "."')

# Not a thresh repo: nothing to say, and don't even look for the binary.
[ -d "$CWD/.thresh" ] || exit 0

find_thresh() {
  command -v thresh 2>/dev/null && return 0
  if command -v go >/dev/null 2>&1; then
    local gp
    gp=$(go env GOPATH 2>/dev/null)
    [ -n "$gp" ] && [ -x "$gp/bin/thresh" ] && { echo "$gp/bin/thresh"; return 0; }
  fi
  [ -x "$HOME/go/bin/thresh" ] && { echo "$HOME/go/bin/thresh"; return 0; }
  return 1
}

COH=$(find_thresh) || {
  jq -n '{systemMessage: "⚠️ thresh: this repo has .thresh/ but the thresh binary was not found on PATH or in GOPATH/bin (go install it from the thresh checkout)."}'
  exit 0
}

ERRFILE=$(mktemp)
trap 'rm -f "$ERRFILE"' EXIT
OUT=$("$COH" check --repo "$CWD" 2>"$ERRFILE")
RC=$?
ERR=$(cat "$ERRFILE")

if [ "$RC" -ne 0 ]; then
  # Do not vanish on error (e.g. invalid annotations): surface it. Still never block.
  jq -n --arg msg "⚠️ thresh could not run (signals suppressed):"$'\n'"$ERR" '{systemMessage: $msg}'
  exit 0
fi

# Surface only when there are actual signals; stay silent otherwise.
if printf '%s' "$OUT" | grep -q '## Thresh'; then
  jq -n --arg msg "$OUT" '{systemMessage: $msg}'
fi
exit 0
