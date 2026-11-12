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
import { appendFileSync, mkdirSync, readFileSync, readdirSync, realpathSync, writeFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import {
  allowsCodeEdits,
  classify,
  editedPaths,
  isLastroCommand,
  isReviewCommand,
  loadWorkspace,
  nextStep,
  readState,
  renderBoard,
  renderLine,
  renderReview,
  snapshot,
  writeState,
} from "./lib.mjs";

const AGENT_NAMES = /--as\s+["']?(claude|codex|agent|assistant|ai|bot)\b/i;

/** Codex sets PLUGIN_ROOT for plugin hooks (and CLAUDE_PLUGIN_ROOT for compatibility); Claude Code does not. */
export const harnessOf = (input, env = process.env) => env.LASTRO_HARNESS ?? (env.PLUGIN_ROOT || input.turn_id !== undefined ? "codex" : "claude");

const deny = (reason) => ({ hookSpecificOutput: { hookEventName: "PreToolUse", permissionDecision: "deny", permissionDecisionReason: reason } });
const ask = (reason) => ({ hookSpecificOutput: { hookEventName: "PreToolUse", permissionDecision: "ask", permissionDecisionReason: reason } });

/**
 * Why a source edit waits, and what the agent does now. This is where intents get extracted in every mode
 * (plan mode, auto mode, an informal plan or none): the first attempt to edit source code is the trigger.
 */
export function gateReason(ws, s, paths) {
  const cli = ws.config.cli;
  const extract = [
    `Before editing ${paths.join(", ")}, Lastro needs the intents of this change, reviewed by the user:`,
    `1. Write the change you are about to make as prose in .lastro/proposals/<slug>.md: the behavior that must hold after it, from the user's request and your plan (formal or not). No code.`,
    `2. Run \`${cli} propose .lastro/proposals/<slug>.md\` (exit 3: answer the handoff request with your own model and run it again). It becomes proposal <id>-<slug>, its intents changing the use cases in .lastro/usecases/.`,
    `3. Start the review: ask the user each decision, then approve / request changes / reject per intent, showing the proofs each one replaces (lastro \`propose\` skill). Then \`${cli} derive <id>\`.`,
  ];
  switch (s.phase) {
    case "no-workspace":
      return `Lastro: this repository is not set up yet. ${nextStep(s)}`;
    case "no-proposal":
      return extract.join("\n");
    case "passed":
      return [`Lastro: the approved change is done and every proof passes, so this edit is a new change.`, ...extract.slice(1)].join("\n") + `\n\n${renderLine(s)}`;
    case "review-pending":
      return `Lastro: the review is not done. ${nextStep(s)}\nPending: ${s.pending.join("; ")}.\n\n${renderLine(s)}`;
    default:
      return `Lastro: no code before the proposal's proofs are derived. ${nextStep(s)}\n\n${renderLine(s)}`;
  }
}

/**
 * Saves a plan as the prose of the next proposal, `.lastro/proposals/<id>-<slug>.md`, the slug taken from its first
 * heading. `lastro propose` on that file keeps its id and slug. An earlier proposal is never overwritten.
 */
export function saveProposal(ws, plan) {
  const title = (plan.match(/^#+\s*(.+)$/m)?.[1] ?? plan.trim().split("\n")[0]).trim();
  const slug = title.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "").slice(0, 48).replace(/-$/, "") || "plan";
  const dir = join(ws.dir, "proposals");
  mkdirSync(dir, { recursive: true });
  const max = readdirSync(dir).reduce((m, f) => Math.max(m, Number(f.match(/^P-(\d+)-/)?.[1] ?? 0)), 0);
  const file = join(dir, `P-${String(max + 1).padStart(3, "0")}-${slug}.md`);
  writeFileSync(file, plan.trim() + "\n");
  return relative(ws.root, file);
}

/** Builds and proves; records the result in the plugin state. */
/**
 * Builds and proves; records the result in the plugin state. While it runs, `.lastro/plugin/progress.jsonl`
 * holds the live events (`build`, then the CLI's `prove --stream` lines), which the Claude Code mod draws.
 */
export function prove(ws, proposal = "") {
  if (!ws.config.prove) return { ok: false, error: 'No prove command: set "prove" in .lastro/plugin.json (the arguments after `lastro prove`).' };
  const progress = join(ws.dir, "plugin", "progress.jsonl");
  mkdirSync(dirname(progress), { recursive: true });
  const line = (e) => JSON.stringify({ ...e, at: new Date().toISOString() }) + "\n";
  writeFileSync(progress, line({ event: "build", command: ws.config.build ?? null }));
  if (ws.config.build) {
    const b = spawnSync(ws.config.build, { cwd: ws.root, shell: true, encoding: "utf8", timeout: 600_000 });
    if (b.status !== 0) {
      const error = `The build failed (\`${ws.config.build}\`):\n${(b.stderr || b.stdout).trim().slice(-4000)}`;
      appendFileSync(progress, line({ event: "result", ok: false, error }));
      return { ok: false, error };
    }
  }
  // The CLI appends its stream straight to the file, so the events land as they happen.
  const command = `${ws.config.cli} prove ${proposal ? `${proposal} ` : ""}${ws.config.prove} --stream --dir ${JSON.stringify(ws.dir)} >> ${JSON.stringify(progress)}`;
  const r = spawnSync(command, { cwd: ws.root, shell: true, encoding: "utf8", timeout: 600_000 });
  const lines = readFileSync(progress, "utf8").trim().split("\n");
  let body;
  try {
    body = JSON.parse(lines.at(-1));
  } catch {
    body = {};
  }
  if (body.event !== "result" || body.error || r.status === 2) {
    const error = `\`${ws.config.cli} prove ${ws.config.prove}\` failed: ${body.error ?? (r.stderr || "no result").trim().slice(-4000)}`;
    if (body.event !== "result") appendFileSync(progress, line({ event: "result", ok: false, error }));
    return { ok: false, error };
  }
  const state = readState(ws);
  writeState(ws, { ...state, dirty: false });
  return { ok: body.ok === true, body };
}

/** What the agent of this session last got from the plugin (kept in .lastro/plugin/state.json). */
function seen(ws, input) {
  const c = readState(ws).context;
  return c && c.session === (input.session_id ?? "") ? c : {};
}

function remember(ws, input, patch) {
  const state = readState(ws);
  const session = input.session_id ?? "";
  const before = state.context?.session === session ? state.context : {};
  writeState(ws, { ...state, context: { ...before, ...patch, session } });
}

/** The proposal's intents and decisions, without their review state: a new key means a new review to show. */
function proposalKey(s) {
  const p = s.proposal;
  return JSON.stringify([p.id, (p.intents ?? []).map((i) => [i.id, i.description]), (p.decisions ?? []).map((d) => d.id)]);
}

export function handle(event, input, env = process.env) {
  const ws = loadWorkspace(input.cwd ?? process.cwd(), env);
  if (!ws) return {}; // the repository does not use the plugin
  const harness = harnessOf(input, env);

  switch (event) {
    case "SessionStart": {
      // Once per session (and again after a compaction): the rules and the full board.
      const s = snapshot(ws);
      remember(ws, input, { line: renderLine(s), review: null });
      const context = [
        "This repository uses Lastro: every change in behavior goes through reviewed intents before any code is written.",
        "Intents are the code review. You are the author: write the proposal, never approve it. The user reviews it like a pull request.",
        "The application is its use cases (.lastro/usecases/); every change is a proposal whose intents add, change or remove them.",
        "Before you edit any source file, in any mode (plan mode, auto mode, a quick fix), write the change as a proposal and start the user's review: lastro `propose` skill.",
        "",
        renderBoard(s),
      ].join("\n");
      return { hookSpecificOutput: { hookEventName: event, additionalContext: context } };
    }
    case "UserPromptSubmit": {
      // Every prompt stays in the conversation, so this adds one short line, and only when the board moved since the
      // agent last saw it in this session.
      const line = renderLine(snapshot(ws));
      if (seen(ws, input).line === line) return {};
      remember(ws, input, { line });
      const context = `${line}. Reminder: you author Lastro proposals and never approve them; extract the intents before the first source edit (lastro \`propose\` skill).`;
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
      // Code edits belong to an approved proposal whose proofs are not passing yet. Anything else is a new change.
      if (allowsCodeEdits(s.phase)) return {};
      const code = kinds.filter(([, k]) => k === "code").map(([p]) => p);
      return deny(gateReason(ws, s, code));
    }

    case "PostToolUse": {
      const tool = input.tool_name ?? "";
      if (tool === "ExitPlanMode") {
        // Claude Code's plan mode: the approved plan becomes the proposal's draft right away.
        const plan = input.tool_response?.plan ?? input.tool_input?.plan;
        if (typeof plan !== "string" || !plan.trim()) return {};
        const file = saveProposal(ws, plan);
        return {
          hookSpecificOutput: {
            hookEventName: "PostToolUse",
            additionalContext: `Lastro saved this plan as the prose of a new proposal, ${file}. Before editing any source file, rewrite it there as behavior (what must be true after the change, no code), run \`${ws.config.cli} propose ${file}\`, and start the user's review (lastro \`propose\` skill).`,
          },
          systemMessage: `Lastro: plan saved as ${file}; its intents come next, then your review.`,
        };
      }
      if (tool === "update_plan") {
        // Codex's step list: a reminder that the plan goes through Lastro before the first edit.
        return { hookSpecificOutput: { hookEventName: "PostToolUse", additionalContext: `Lastro: before the first source edit of this plan, write the change as prose in .lastro/proposals/<slug>.md, run \`${ws.config.cli} propose\` on it, and start the user's review (lastro \`propose\` skill).` } };
      }
      if (tool === "Bash" || tool === "exec_command" || tool === "shell") {
        if (!isLastroCommand(String(input.tool_input?.command ?? ""))) return {};
        const s = snapshot(ws);
        // systemMessage is shown to the user only; additionalContext is what costs the agent tokens.
        if (s.phase !== "review-pending") return { systemMessage: renderBoard(s) };
        const line = renderLine(s);
        const key = proposalKey(s);
        if (seen(ws, input).review === key) {
          // The same proposal, one more answer recorded: a one-line update, not the whole review again.
          remember(ws, input, { line });
          return { systemMessage: renderBoard(s), hookSpecificOutput: { hookEventName: "PostToolUse", additionalContext: `${line}.` } };
        }
        // A new or re-extracted proposal: the review goes to the user as the plugin renders it, once.
        remember(ws, input, { line, review: key });
        return {
          systemMessage: renderBoard(s),
          hookSpecificOutput: {
            hookEventName: "PostToolUse",
            additionalContext: `Show the user this review exactly as written (Markdown), then ask for the open decisions and each intent:\n\n${renderReview(s)}`,
          },
        };
      }
      const paths = editedPaths(input.tool_input);
      if (paths.some((p) => classify(ws, input.cwd ?? ws.root, p) === "code")) writeState(ws, { ...readState(ws), dirty: true });
      return {};
    }

    case "Stop": {
      let s = snapshot(ws);
      if (!["implementing", "failed", "passed"].includes(s.phase)) return { systemMessage: renderBoard(s) };
      const state = readState(ws);
      const proposalHash = s.proposal.hash;
      const attempts = state.proposalHash === proposalHash ? state.attempts : 0;
      if (s.phase === "passed" && !state.dirty) {
        writeState(ws, { ...state, proposalHash, attempts: 0 });
        return { systemMessage: renderBoard(s) };
      }
      const r = prove(ws, s.proposal.id);
      s = snapshot(ws);
      if (r.ok) {
        writeState(ws, { ...readState(ws), proposalHash, attempts: 0 });
        return { systemMessage: renderBoard(s) };
      }
      const max = Number(ws.config.maxIterations);
      if (attempts >= max) {
        writeState(ws, { ...readState(ws), proposalHash, attempts });
        return { systemMessage: `${renderBoard(s)}\n\nThe proofs still fail after ${max} attempts: over to you.` };
      }
      writeState(ws, { ...readState(ws), proposalHash, attempts: attempts + 1 });
      const feedback = r.error ?? r.body.feedback;
      const file = relative(ws.root, ws.file(join("proposals", `${s.proposal.id}-${s.proposal.slug}.json`)));
      return {
        decision: "block",
        reason: `Lastro proofs failed (attempt ${attempts + 1} of ${max}). Read the approved intents and the resulting use cases in ${file} and fix the code; do not change the proposal or the use cases. A failing use-case proof is a regression: behavior this change must keep.\n\n${feedback}`,
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
  if (cmd === "review") {
    process.stdout.write(renderReview(snapshot(ws)) + "\n");
    return 0;
  }
  if (cmd === "prove") {
    const r = prove(ws, snapshot(ws).proposal?.id);
    if (r.error) process.stderr.write(r.error + "\n");
    process.stdout.write(renderBoard(snapshot(ws)) + "\n");
    return r.ok ? 0 : 1;
  }
  process.stderr.write("usage: lastro-plugin.mjs hook <Event> | board [--line] | review | prove\n");
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
