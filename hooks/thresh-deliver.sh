#!/usr/bin/env bash
# Thresh's agent-facing surface. The Stop hook's systemMessage reaches only the
# human; this delivers taps to the agent as context, each with its ID, so the agent
# that crossed a boundary hears about it and can later record whether it helped
# (`thresh verdict`). Each tap is delivered to a session once, and again only if
# its message changes. It never blocks -- notify, not gate.
#
# Install for two events (alongside run-thresh.sh as the Stop hook): copy this file
# to ~/.claude/hooks/thresh-deliver.sh (chmod +x) and add
#   "PostToolUse": [{"matcher": "Edit|Write|MultiEdit|NotebookEdit", "hooks": [{"type": "command",
#     "command": "~/.claude/hooks/thresh-deliver.sh", "timeout": 30}]}],
#   "UserPromptSubmit": [{"hooks": [{"type": "command",
#     "command": "~/.claude/hooks/thresh-deliver.sh", "timeout": 30}]}]
# PostToolUse tells the agent mid-turn, while it can still change course;
# UserPromptSubmit catches what edits made through other tools (Bash) crossed.
set -uo pipefail
INPUT=$(cat)

EVENT=$(printf '%s' "$INPUT" | jq -r '.hook_event_name // ""')
CWD=$(printf '%s' "$INPUT" | jq -r '.cwd // "."')
SESSION=$(printf '%s' "$INPUT" | jq -r '.session_id // ""')
TRANSCRIPT=$(printf '%s' "$INPUT" | jq -r '.transcript_path // ""')

# Not a thresh repo: nothing to say, and don't even look for the binary.
[ -d "$CWD/.thresh" ] || exit 0
[ -n "$SESSION" ] || exit 0

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

# A missing binary or a failed run is surfaced by the Stop hook at end of turn;
# repeating it after every edit would only be noise.
THRESH=$(find_thresh) || exit 0
OUT=$("$THRESH" check --repo "$CWD" --session "$SESSION" --transcript "$TRANSCRIPT" --new 2>/dev/null) || exit 0

[ -n "$OUT" ] || exit 0
jq -n --arg ev "$EVENT" --arg ctx "$OUT" \
  '{hookSpecificOutput: {hookEventName: $ev, additionalContext: $ctx}}'
exit 0
