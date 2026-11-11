#!/usr/bin/env node
// Entry point for the Lastro plugin, in Claude Code and in Codex.
//
//   node lastro-plugin.mjs hook <Event>     a harness hook: event JSON on stdin, hook JSON on stdout
//   node lastro-plugin.mjs board [--line]   print the progress board (or one line, e.g. for a status line)
//   node lastro-plugin.mjs prove            build (if configured), prove, and print the board
//
// Both harnesses send the same hook fields (hook_event_name, cwd, tool_name, tool_input, stop_hook_active)
// and read the same answers (hookSpecificOutput.permissionDecision, additionalContext, decision/reason,
// systemMessage), so one script serves both.
import { readFileSync, realpathSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import {
  classify,
  editedPaths,
  isLastroCommand,
  isReviewCommand,
  loadWorkspace,
  nextStep,
  readState,
  renderBoard,
  renderLine,
  runCli,
  snapshot,
  writeState,
} from "./lib.mjs";

const AGENT_NAMES = /--as\s+["']?(claude|codex|agent|assistant|ai|bot)\b/i;

/** Codex sets PLUGIN_ROOT for plugin hooks (and CLAUDE_PLUGIN_ROOT for compatibility); Claude Code does not. */
export const harnessOf = (input, env = process.env) => env.LASTRO_HARNESS ?? (env.PLUGIN_ROOT || input.turn_id !== undefined ? "codex" : "claude");

const deny = (reason) => ({ hookSpecificOutput: { hookEventName: "PreToolUse", permissionDecision: "deny", permissionDecisionReason: reason } });
const ask = (reason) => ({ hookSpecificOutput: { hookEventName: "PreToolUse", permissionDecision: "ask", permissionDecisionReason: reason } });

/** Builds and proves; records the result in the plugin state. */
export function prove(ws) {
  if (!ws.config.prove) return { ok: false, error: 'No prove command: set "prove" in .lastro/plugin.json (the arguments after `lastro prove`).' };
  if (ws.config.build) {
    const b = spawnSync(ws.config.build, { cwd: ws.root, shell: true, encoding: "utf8", timeout: 600_000 });
    if (b.status !== 0) return { ok: false, error: `The build failed (\`${ws.config.build}\`):\n${(b.stderr || b.stdout).trim().slice(-4000)}` };
  }
  const r = runCli(ws, `prove ${ws.config.prove}`, { timeout: 600_000 });
  if (r.code === 2) return { ok: false, error: `\`${r.command}\` failed: ${r.body.error ?? JSON.stringify(r.body)}` };
  const state = readState(ws);
  writeState(ws, { ...state, dirty: false });
  return { ok: r.body.ok === true, body: r.body };
}

export function handle(event, input, env = process.env) {
  const ws = loadWorkspace(input.cwd ?? process.cwd(), env);
  if (!ws) return {}; // the repository does not use the plugin
  const harness = harnessOf(input, env);

  switch (event) {
    case "SessionStart":
    case "UserPromptSubmit": {
      const s = snapshot(ws);
      const context = [
        "This repository uses Lastro: every change in behavior goes through reviewed intents before any code is written.",
        "Intents are the code review. You are the author: write the proposal, never approve it. The user reviews it like a pull request.",
        "If this request changes behavior, follow the lastro `propose` skill before editing code.",
        "",
        renderBoard(s),
      ].join("\n");
      return { hookSpecificOutput: { hookEventName: event, additionalContext: context } };
    }

    case "PreToolUse": {
      const tool = input.tool_name ?? "";
      if (tool === "Bash" || tool === "exec_command" || tool === "shell") {
        const cmd = String(input.tool_input?.command ?? "");
        if (!isReviewCommand(cmd)) return {};
        if (AGENT_NAMES.test(cmd)) return deny("An agent cannot review its own proposal: `--as` must name the person who reviewed it.");
        // The review is the user's: in Claude Code the permission prompt is that click. Codex asks per its approval policy.
        return harness === "claude" ? ask(`Lastro review: this records YOUR review of the intents (like approving a pull request). Allow only if you made this call.\n${cmd}`) : {};
      }
      const paths = editedPaths(input.tool_input);
      if (!paths.length) return {};
      const kinds = paths.map((p) => [p, classify(ws, input.cwd ?? ws.root, p)]);
      const generated = kinds.filter(([, k]) => k === "generated").map(([p]) => p);
      if (generated.length) return deny(`${generated.join(", ")} ${generated.length > 1 ? "are" : "is"} written only by the Lastro CLI. Run the CLI command instead of editing it.`);
      if (kinds.some(([, k]) => k === "config")) return harness === "claude" ? ask("This changes how Lastro gates and proves this repository (.lastro/plugin.json).") : deny("Only the user changes .lastro/plugin.json.");
      if (ws.config.gate === "off" || !kinds.some(([, k]) => k === "code")) return {};
      const s = snapshot(ws);
      if (["implementing", "failed", "passed"].includes(s.phase)) return {};
      const pending = s.phase === "review-pending" ? `\nPending: ${s.pending.join("; ")}.` : "";
      return deny(`Lastro: no code before the intents are approved and the proofs derived. ${nextStep(s)}${pending}\n\n${renderLine(s)}`);
    }

    case "PostToolUse": {
      const tool = input.tool_name ?? "";
      if (tool === "Bash" || tool === "exec_command" || tool === "shell") {
        if (!isLastroCommand(String(input.tool_input?.command ?? ""))) return {};
        return { systemMessage: renderBoard(snapshot(ws)) };
      }
      const paths = editedPaths(input.tool_input);
      if (paths.some((p) => classify(ws, input.cwd ?? ws.root, p) === "code")) writeState(ws, { ...readState(ws), dirty: true });
      return {};
    }

    case "Stop": {
      let s = snapshot(ws);
      if (!["implementing", "failed", "passed"].includes(s.phase)) return { systemMessage: renderBoard(s) };
      const state = readState(ws);
      const specHash = s.spec.artifact.hash;
      const attempts = state.specHash === specHash ? state.attempts : 0;
      if (s.phase === "passed" && !state.dirty) {
        writeState(ws, { ...state, specHash, attempts: 0 });
        return { systemMessage: renderBoard(s) };
      }
      const r = prove(ws);
      s = snapshot(ws);
      if (r.ok) {
        writeState(ws, { ...readState(ws), specHash, attempts: 0 });
        return { systemMessage: renderBoard(s) };
      }
      const max = Number(ws.config.maxIterations);
      if (attempts >= max) {
        writeState(ws, { ...readState(ws), specHash, attempts });
        return { systemMessage: `${renderBoard(s)}\n\nThe proofs still fail after ${max} attempts: over to you.` };
      }
      writeState(ws, { ...readState(ws), specHash, attempts: attempts + 1 });
      const feedback = r.error ?? r.body.feedback;
      return {
        decision: "block",
        reason: `Lastro proofs failed (attempt ${attempts + 1} of ${max}). Read the approved intents in ${ws.file("intents.json")} and fix the code; do not change the intents or the spec.\n\n${feedback}`,
        systemMessage: renderBoard(s),
      };
    }
  }
  return {};
}

function main(argv) {
  const [cmd, arg] = argv;
  if (cmd === "hook") {
    let input = {};
    try {
      input = JSON.parse(readFileSync(0, "utf8") || "{}");
    } catch {
      /* no input */
    }
    let out;
    try {
      out = handle(arg ?? input.hook_event_name, input);
    } catch (e) {
      out = { systemMessage: `Lastro plugin error: ${e.message}` }; // never break the session
    }
    process.stdout.write(JSON.stringify(out) + "\n");
    return 0;
  }
  const ws = loadWorkspace(process.cwd());
  if (!ws) {
    process.stderr.write("No .lastro/plugin.json in this repository or above it.\n");
    return 2;
  }
  if (cmd === "board") {
    const s = snapshot(ws);
    process.stdout.write((argv.includes("--line") ? renderLine(s) : renderBoard(s)) + "\n");
    return s.phase === "passed" ? 0 : 1;
  }
  if (cmd === "prove") {
    const r = prove(ws);
    if (r.error) process.stderr.write(r.error + "\n");
    process.stdout.write(renderBoard(snapshot(ws)) + "\n");
    return r.ok ? 0 : 1;
  }
  process.stderr.write("usage: lastro-plugin.mjs hook <Event> | board [--line] | prove\n");
  return 2;
}

const invoked = (() => {
  try {
    return !!process.argv[1] && realpathSync(process.argv[1]) === realpathSync(fileURLToPath(import.meta.url));
  } catch {
    return false;
  }
})();
if (invoked) process.exitCode = main(process.argv.slice(2));
