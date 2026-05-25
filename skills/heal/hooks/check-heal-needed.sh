#!/usr/bin/env bash
# check-heal-needed.sh — sample Claude Code PostToolUse hook for /heal.
#
# Reads the tool exit code from $CLAUDE_TOOL_EXIT_CODE (Claude Code passes
# the most recently invoked skill's exit code in this var; verify against
# your Claude Code version's hook env contract).
#
# Behavior:
#   * exit 0 silently if the validation passed
#   * print a Claude-readable instruction to stdout otherwise, telling
#     Claude to propose an EditPlan and run /heal again
#   * print the iteration count + cap so Claude knows when to stop
#
# This script does NOT invoke /heal directly. Claude reads the stdout
# instruction and decides on the next action. Loop control lives in
# Claude's reasoning + the heal-state.json counter.

set -euo pipefail

exit_code="${CLAUDE_TOOL_EXIT_CODE:-0}"
if [[ "$exit_code" == "0" ]]; then
  exit 0
fi

repo_root="$(pwd)"
state_file="${repo_root}/.harness/runtime/heal-state.json"

iteration=0
max=3
if [[ -f "$state_file" ]]; then
  iteration=$(grep -Eo '"iteration"[[:space:]]*:[[:space:]]*[0-9]+' "$state_file" | grep -Eo '[0-9]+$' || echo 0)
  max=$(grep -Eo '"max_iterations"[[:space:]]*:[[:space:]]*[0-9]+' "$state_file" | grep -Eo '[0-9]+$' || echo 3)
fi

if (( iteration >= max )); then
  cat <<EOF
Heal exhausted after ${iteration}/${max} attempts. Manual intervention needed.
Review the latest verdict.json under .harness/runtime/use-cases/ and inspect
heal-state.json for the prior attempts.
EOF
  exit 0
fi

cat <<EOF
Validation failed (exit ${exit_code}). Heal iteration ${iteration}/${max}.
Propose an EditPlan that addresses the failing signal's heal_hint and run:

    echo '<edit-plan-json>' | /heal <usecase-id>

EditPlan shape:

    {"files":[{"path":"...","op":"write","content":"..."}],"rationale":"..."}
EOF
