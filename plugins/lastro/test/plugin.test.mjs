// The plugin's hooks and board, driven through the real `lastro` CLI on the docs' shipping example.
// Needs the TypeScript SDK built (`npm run build -w lastro`).
import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { handle } from "../scripts/lastro-plugin.mjs";
import { classify, editedPaths, globToRegExp, isReviewCommand, loadWorkspace, renderBoard, renderLine, renderReview, snapshot, tally, wrap } from "../scripts/lib.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const repo = join(here, "..", "..", "..");
const example = join(repo, "apps", "docs", "examples", "shipping");
const cliJs = join(repo, "sdks", "typescript", "dist", "src", "cli.js");
const plugin = join(here, "..", "scripts", "lastro-plugin.mjs");
const skip = !existsSync(cliJs) && "build the TS SDK first: npm run build -w lastro";

delete process.env.ANTHROPIC_API_KEY; // the LLM steps go through the handoff, answered from the example
const CLI = `node ${cliJs}`;
const claude = { LASTRO_HARNESS: "claude" };
const codex = { LASTRO_HARNESS: "codex" };

function lastro(dir, ...args) {
  const r = spawnSync(process.execPath, [cliJs, ...args], { cwd: dir, encoding: "utf8" });
  return { code: r.status, body: JSON.parse(r.stdout) };
}

function repoWith(config = {}) {
  const dir = mkdtempSync(join(tmpdir(), "lastro-plugin-"));
  for (const f of ["model.json", "shipping.mjs", "shipping-buggy.mjs"]) copyFileSync(join(example, f), join(dir, f));
  mkdirSync(join(dir, ".lastro", "proposals"), { recursive: true });
  copyFileSync(join(example, "prose.md"), join(dir, ".lastro", "proposals", "shipping.md"));
  writeFileSync(join(dir, ".lastro", "plugin.json"), JSON.stringify({ cli: CLI, prove: "./shipping-buggy.mjs", maxIterations: 2, allow: ["docs/**"], ...config }));
  return dir;
}

/** init → plan (with the extraction answered) → review pending. */
function planned(config) {
  const dir = repoWith(config);
  assert.equal(lastro(dir, "init", "--model", "model.json").code, 0);
  const r = lastro(dir, "plan", ".lastro/proposals/shipping.md");
  assert.equal(r.code, 3);
  copyFileSync(join(example, "answers", "extract.json"), r.body.answer);
  assert.equal(lastro(dir, "plan", ".lastro/proposals/shipping.md").code, 1);
  return dir;
}

/** ... → the user's review → spec derived. */
function specified(config) {
  const dir = planned(config);
  lastro(dir, "review", "--as", "ana", "answer", "D1", "1");
  lastro(dir, "review", "--as", "ana", "approve-all");
  const r = lastro(dir, "spec");
  copyFileSync(join(example, "answers", "derive.json"), r.body.answer);
  assert.equal(lastro(dir, "spec").code, 0);
  return dir;
}

const edit = (dir, file_path, env = claude) => handle("PreToolUse", { cwd: dir, tool_name: "Write", tool_input: { file_path, content: "x" } }, env);
const decision = (out) => out.hookSpecificOutput?.permissionDecision ?? "allow";

test("without .lastro/plugin.json the hooks do nothing", () => {
  const dir = mkdtempSync(join(tmpdir(), "lastro-plugin-none-"));
  assert.deepEqual(edit(dir, "src/a.js"), {});
  assert.deepEqual(handle("Stop", { cwd: dir }, claude), {});
});

test("paths: Claude Code edits and Codex patches", () => {
  assert.deepEqual(editedPaths({ file_path: "/r/src/a.ts" }), ["/r/src/a.ts"]);
  const patch = "*** Begin Patch\n*** Update File: src/fee.js\n@@\n-a\n+b\n*** Add File: src/new.js\n+x\n*** Update File: src/old.js\n*** Move to: src/moved.js\n*** End Patch";
  assert.deepEqual(editedPaths({ command: ["apply_patch", patch] }).sort(), ["src/fee.js", "src/moved.js", "src/new.js", "src/old.js"]);
  assert.ok(globToRegExp("docs/**").test("docs/a/b.md"));
  assert.ok(globToRegExp("**/*.md").test("README.md"));
  assert.ok(!globToRegExp("src/*.js").test("src/a/b.js"));
  assert.ok(isReviewCommand("npx lastro review --as ana approve I1"));
  assert.ok(isReviewCommand("lastro-go review --as ana answer D1 0"));
  assert.ok(!isReviewCommand("lastro status"));
});

test("before any review, code edits wait; proposals and allowed paths do not", { skip }, () => {
  const dir = repoWith();
  lastro(dir, "init", "--model", "model.json");
  const ws = loadWorkspace(dir);
  assert.equal(snapshot(ws).phase, "no-proposal");
  const out = edit(dir, "src/fee.js");
  assert.equal(decision(out), "deny");
  assert.match(out.hookSpecificOutput.permissionDecisionReason, / plan \.lastro\/proposals/);
  assert.equal(decision(edit(dir, ".lastro/proposals/next.md")), "allow");
  assert.equal(decision(edit(dir, "docs/notes.md")), "allow");
  assert.equal(classify(ws, dir, join(dir, ".lastro", "llm", "x.answer.json")), "free");
});

test("review pending: the board lists decisions and intents, edits are denied, the CLI files are protected", { skip }, () => {
  const dir = planned();
  const s = snapshot(loadWorkspace(dir));
  assert.equal(s.phase, "review-pending");
  assert.deepEqual(tally(s), { intents: 4, approved: 0, decisions: 1, answered: 0, clauses: 0, passed: 0, failed: 0 });
  const board = renderBoard(s);
  assert.match(board, /waiting for the user's review/);
  assert.match(board, /\? D1 Is the 100 threshold measured before or after discounts\?/);
  assert.match(board, /1\. After discounts \(recommended\)/);
  assert.match(board, /○ I1 An order with a subtotal of 100 or more pays no shipping\./);
  assert.equal(renderLine(s), "Lastro · review 0/4 intents approved · 0/1 decisions answered");

  const out = edit(dir, "src/fee.js");
  assert.equal(decision(out), "deny");
  assert.match(out.hookSpecificOutput.permissionDecisionReason, /D1 unanswered/);
  assert.equal(decision(edit(dir, ".lastro/intents.json")), "deny");
  assert.equal(decision(edit(dir, ".lastro/spec.json", codex)), "deny");
  assert.equal(decision(edit(dir, ".lastro/plugin.json")), "ask");
  assert.equal(decision(edit(dir, ".lastro/plugin.json", codex)), "deny");

  // The SessionStart context tells the agent it is the author and shows the board.
  const ctx = handle("SessionStart", { cwd: dir, session_id: "s1" }, claude).hookSpecificOutput.additionalContext;
  assert.match(ctx, /You are the author/);
  assert.match(ctx, /Decisions    0\/1 answered/);
});

test("context costs tokens: prompts get one short line, and only when the board moved", { skip }, () => {
  const dir = planned();
  const prompt = (session_id) => handle("UserPromptSubmit", { cwd: dir, session_id, prompt: "go" }, claude).hookSpecificOutput?.additionalContext;
  handle("SessionStart", { cwd: dir, session_id: "s1" }, claude);
  assert.equal(prompt("s1"), undefined); // the agent already has this board
  lastro(dir, "review", "--as", "ana", "answer", "D1", "1");
  const line = prompt("s1");
  assert.match(line, /^Lastro · review 0\/4 intents approved · 1\/1 decisions answered\. Reminder: you author/);
  assert.ok(line.length < 300);
  assert.equal(prompt("s1"), undefined);
  assert.match(prompt("s2"), /^Lastro · review/); // another session has not seen it
});

test("the full review reaches the agent once per proposal; later answers add one line", { skip }, () => {
  const dir = planned();
  const bash = (command) => handle("PostToolUse", { cwd: dir, session_id: "s1", tool_name: "Bash", tool_input: { command } }, claude);
  const first = bash("lastro status");
  assert.match(first.hookSpecificOutput.additionalContext, /^Show the user this review exactly as written/);
  assert.match(first.systemMessage, /^Lastro · shipping_fee/);
  lastro(dir, "review", "--as", "ana", "approve", "I1");
  assert.equal(bash("lastro review --as ana approve I1").hookSpecificOutput.additionalContext, "Lastro · review 1/4 intents approved · 0/1 decisions answered.");
  // Re-extracting with a changed statement is a new proposal to review: shown in full again.
  lastro(dir, "review", "--as", "ana", "amend", "I2", "A member never pays shipping.");
  assert.match(bash("lastro review --as ana amend I2 x").hookSpecificOutput.additionalContext, /^Show the user this review/);
});

test("with two required approvals, the board shows each intent's approvals, like a pull request", { skip }, () => {
  const dir = repoWith();
  lastro(dir, "init", "--model", "model.json", "--approvals", "2");
  const r = lastro(dir, "plan", ".lastro/proposals/shipping.md");
  copyFileSync(join(example, "answers", "extract.json"), r.body.answer);
  lastro(dir, "plan", ".lastro/proposals/shipping.md");
  lastro(dir, "review", "--as", "ana", "answer", "D1", "1");
  lastro(dir, "review", "--as", "ana", "approve-all");
  const s = snapshot(loadWorkspace(dir));
  assert.equal(s.phase, "review-pending", "one reviewer is not enough");
  assert.match(renderBoard(s), /○ I1 \(1\/2 approvals\) An order/);
  assert.equal(decision(edit(dir, "src/fee.js")), "deny");
  lastro(dir, "review", "--as", "bruno", "approve-all");
  assert.equal(snapshot(loadWorkspace(dir)).phase, "needs-spec");
});

test("the review is the user's: Claude Code asks, an agent name is refused", { skip }, () => {
  const dir = planned();
  const bash = (command, env = claude) => handle("PreToolUse", { cwd: dir, tool_name: "Bash", tool_input: { command } }, env);
  assert.equal(decision(bash(`${CLI} review --as ana approve I1`)), "ask");
  assert.equal(decision(bash("lastro review --as claude approve-all")), "deny");
  assert.equal(decision(bash("lastro review --as codex approve I1", codex)), "deny");
  assert.equal(decision(bash("lastro review --as ana approve I1", codex)), "allow");
  assert.equal(decision(bash("lastro status")), "allow");
  // After a lastro command the board is shown to the user.
  assert.match(handle("PostToolUse", { cwd: dir, tool_name: "Bash", tool_input: { command: "lastro status" } }, claude).systemMessage, /^Lastro · shipping_fee/);
});

test("approved but no spec: still gated; after spec, edits go through and the proofs are pending", { skip }, () => {
  const dir = planned();
  lastro(dir, "review", "--as", "ana", "answer", "D1", "1");
  lastro(dir, "review", "--as", "ana", "approve-all");
  assert.equal(snapshot(loadWorkspace(dir)).phase, "needs-spec");
  assert.match(edit(dir, "src/fee.js").hookSpecificOutput.permissionDecisionReason, / spec`/);

  const r = lastro(dir, "spec");
  copyFileSync(join(example, "answers", "derive.json"), r.body.answer);
  lastro(dir, "spec");
  const s = snapshot(loadWorkspace(dir));
  assert.equal(s.phase, "implementing");
  assert.equal(decision(edit(dir, "src/fee.js")), "allow");
  const board = renderBoard(s);
  assert.match(board, /○ C1 pending/);
  assert.match(board, /– I4 beyond proof \(termination-timing\)/);
  assert.equal(renderLine(s), "Lastro · 3 proofs pending · ○ 3");
});

test("Stop proves: failures go back to the agent until the attempts run out", { skip }, () => {
  const dir = specified();
  handle("PostToolUse", { cwd: dir, tool_name: "Write", tool_input: { file_path: join(dir, "shipping-buggy.mjs") } }, claude);
  let out = handle("Stop", { cwd: dir, stop_hook_active: false }, claude);
  assert.equal(out.decision, "block");
  assert.match(out.reason, /attempt 1 of 2/);
  assert.match(out.reason, /C1 \(I1\)/);
  assert.match(out.reason, /counterexample: \{"member":false,"subtotal":100\} returned \{"fee":15\}/);
  assert.match(out.systemMessage, /proofs FAILED/);
  assert.match(out.systemMessage, /✗ C1 violated/);
  assert.match(out.systemMessage, /✓ C2 proved/);
  assert.match(out.systemMessage, /Verification 2\/3 passed, 1 failed/);
  assert.equal(renderLine(snapshot(loadWorkspace(dir))), "Lastro · proofs 2/3 passed · ✗ 1 failed · FAILED");

  out = handle("Stop", { cwd: dir, stop_hook_active: true }, claude);
  assert.equal(out.decision, "block");
  assert.match(out.reason, /attempt 2 of 2/);
  out = handle("Stop", { cwd: dir, stop_hook_active: true }, claude);
  assert.equal(out.decision, undefined, "after maxIterations the user takes over");
  assert.match(out.systemMessage, /over to you/);
});

test("Stop proves the fixed code: passed, and no block", { skip }, () => {
  const dir = specified({ prove: "./shipping.mjs" });
  const out = handle("Stop", { cwd: dir }, codex);
  assert.equal(out.decision, undefined);
  assert.match(out.systemMessage, /all proofs PASSED/);
  assert.match(out.systemMessage, /✓ C1 proved/);
  assert.equal(renderLine(snapshot(loadWorkspace(dir))), "Lastro · proofs 3/3 passed · PASSED");
  // An edit after the proof makes it stale: pending again until the next Stop.
  handle("PostToolUse", { cwd: dir, tool_name: "apply_patch", tool_input: { command: "*** Begin Patch\n*** Update File: shipping.mjs\n*** End Patch" } }, codex);
  assert.equal(snapshot(loadWorkspace(dir)).phase, "implementing");
  assert.match(handle("Stop", { cwd: dir }, codex).systemMessage, /all proofs PASSED/);
});

test("an amended intent makes the spec stale and gates the code again", { skip }, () => {
  const dir = specified({ prove: "./shipping.mjs" });
  lastro(dir, "review", "--as", "ana", "amend", "I3", "A non-member order under 100 pays a shipping fee of exactly 20.");
  // The amended intent no longer matches the spec: the proofs must be derived again before any code.
  assert.equal(snapshot(loadWorkspace(dir)).phase, "needs-spec");
  assert.equal(decision(edit(dir, "shipping.mjs")), "deny");
});

test("the script runs as a hook and as a board command", { skip }, () => {
  const dir = specified({ prove: "./shipping.mjs" });
  const hook = spawnSync(process.execPath, [plugin, "hook", "PreToolUse"], { cwd: dir, input: JSON.stringify({ cwd: dir, tool_name: "Write", tool_input: { file_path: "a.js" } }), encoding: "utf8" });
  assert.equal(hook.status, 0);
  assert.deepEqual(JSON.parse(hook.stdout), {});
  const prove = spawnSync(process.execPath, [plugin, "prove"], { cwd: dir, encoding: "utf8" });
  assert.equal(prove.status, 0, prove.stderr);
  assert.match(prove.stdout, /all proofs PASSED/);
  const line = spawnSync(process.execPath, [plugin, "board", "--line"], { cwd: dir, encoding: "utf8" });
  assert.equal(line.stdout.trim(), "Lastro · proofs 3/3 passed · PASSED");
  assert.ok(existsSync(join(dir, ".lastro", "receipt.md")));
  assert.ok(readFileSync(join(dir, ".lastro", "plugin", "state.json"), "utf8").includes('"dirty": false'));
});

test("both manifests and the hooks file point at files that exist", () => {
  const root = join(here, "..");
  const codexManifest = JSON.parse(readFileSync(join(root, ".codex-plugin", "plugin.json"), "utf8"));
  const claudeManifest = JSON.parse(readFileSync(join(root, ".claude-plugin", "plugin.json"), "utf8"));
  assert.equal(codexManifest.name, claudeManifest.name);
  assert.equal(codexManifest.version, claudeManifest.version);
  assert.ok(existsSync(join(root, codexManifest.hooks)));
  for (const s of ["setup", "propose", "verify"]) assert.match(readFileSync(join(root, "skills", s, "SKILL.md"), "utf8"), new RegExp(`^---\\nname: ${s}\\n`));
  const hooks = JSON.parse(readFileSync(join(root, "hooks", "hooks.json"), "utf8")).hooks;
  assert.deepEqual(Object.keys(hooks), ["SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"]);
  for (const groups of Object.values(hooks)) for (const g of groups) for (const h of g.hooks) assert.match(h.command, /\$\{CLAUDE_PLUGIN_ROOT\}\/scripts\/lastro-plugin\.mjs" hook \w+$/);
});

test("the review reads one item per line, grouped, with the quote each intent came from", { skip }, () => {
  const dir = planned();
  const review = renderReview(snapshot(loadWorkspace(dir)));
  const lines = review.split("\n");
  assert.equal(lines[0], "**Proposal for review:** 4 intents, 0 approved · 1 of 1 decisions open");
  // Every decision, option and intent is its own list item, so a chat that joins single newlines keeps them apart.
  assert.ok(lines.includes("- **D1** Is the 100 threshold measured before or after discounts?"));
  assert.ok(lines.some((l) => /^  1\. After discounts.*_\(recommended\)_$/.test(l)));
  assert.deepEqual(lines.filter((l) => /^\*\*[A-Z]/.test(l) && !l.startsWith("**Proposal")), ["**Decisions**", "**Behavior**", "**Non-functional**"]);
  const i1 = lines.indexOf("- **I1** An order with a subtotal of 100 or more pays no shipping. _(to review)_");
  assert.ok(i1 > 0);
  assert.match(lines[i1 + 1], /^  > \S/);
  assert.equal(lines.filter((l) => /^- \*\*I\d/.test(l)).length, 4);

  // After a lastro command, while the review is open, the agent gets it to show as-is.
  const out = handle("PostToolUse", { cwd: dir, tool_name: "Bash", tool_input: { command: "lastro status" } }, claude);
  assert.ok(out.hookSpecificOutput.additionalContext.endsWith(review));

  lastro(dir, "review", "--as", "ana", "answer", "D1", "1");
  lastro(dir, "review", "--as", "ana", "approve", "I1");
  assert.match(renderReview(snapshot(loadWorkspace(dir))), /^- \*\*I1\*\* .* _\(approved\)_$/m);
});

test("long board lines wrap under their text", () => {
  const text = "A non-member order under 100 pays a shipping fee of exactly 15, shown at checkout before the customer confirms the order and pays.";
  const lines = wrap(`○ I3 ${text}`, "    ", "         ", 60);
  assert.ok(lines.length > 1 && lines.every((l) => l.length <= 60));
  assert.ok(lines.slice(1).every((l) => l.startsWith("         ") && l[9] !== " "));
  assert.equal(lines.join(" ").replace(/\s+/g, " ").trim(), `○ I3 ${text}`);
});

test("the boards the docs show are the real ones", { skip }, () => {
  const ts = readFileSync(join(repo, "apps", "docs", "src", "content", "outputs.ts"), "utf8");
  const docs = JSON.parse(ts.match(/^export const BOARDS = (.*);$/m)[1]);
  const asDocs = (board) => board.split(CLI).join("lastro");
  const dir = planned({ prove: "./shipping-buggy.mjs" });
  assert.equal(asDocs(renderBoard(snapshot(loadWorkspace(dir)))), docs.review);
  lastro(dir, "review", "--as", "ana", "answer", "D1", "1");
  lastro(dir, "review", "--as", "ana", "approve-all");
  const r = lastro(dir, "spec");
  copyFileSync(join(example, "answers", "derive.json"), r.body.answer);
  lastro(dir, "spec");
  assert.equal(asDocs(renderBoard(snapshot(loadWorkspace(dir)))), docs.implementing);
  assert.equal(asDocs(handle("Stop", { cwd: dir }, claude).systemMessage), docs.failed);
});

test("in any mode, the first source edit triggers the extraction: the reason says how", { skip }, () => {
  const dir = repoWith();
  lastro(dir, "init", "--model", "model.json");
  const reason = edit(dir, "src/fee.js").hookSpecificOutput.permissionDecisionReason;
  assert.match(reason, /^Before editing src\/fee\.js, Lastro needs the intents of this change, reviewed by the user:/);
  assert.match(reason, /formal or not/);
  assert.match(reason, / plan \.lastro\/proposals\/<slug>\.md/);
  assert.match(reason, /approve \/ request changes \/ reject per intent/);
});

test("once every proof passes, the next source edit is a new change and needs new intents", { skip }, () => {
  const dir = specified({ prove: "./shipping.mjs" });
  assert.equal(decision(edit(dir, "shipping.mjs")), "allow", "implementing the approved change");
  handle("Stop", { cwd: dir }, claude);
  assert.equal(snapshot(loadWorkspace(dir)).phase, "passed");
  const out = edit(dir, "shipping.mjs", codex);
  assert.equal(decision(out), "deny");
  assert.match(out.hookSpecificOutput.permissionDecisionReason, /the approved change is done and every proof passes, so this edit is a new change/);
  assert.equal(decision(edit(dir, ".lastro/proposals/next.md")), "allow", "writing the next proposal is fine");
});

test("plan mode: the approved plan is saved as a proposal draft; Codex's update_plan gets a reminder", { skip }, () => {
  const dir = repoWith();
  lastro(dir, "init", "--model", "model.json");
  const plan = "# Free shipping for members\n\n1. Members never pay shipping.\n2. Update the fee function.";
  const out = handle("PostToolUse", { cwd: dir, tool_name: "ExitPlanMode", tool_input: {}, tool_response: { plan, filePath: "/home/u/.claude/plans/x.md" } }, claude);
  assert.match(out.hookSpecificOutput.additionalContext, /saved this plan as \.lastro\/proposals\/free-shipping-for-members\.md/);
  assert.equal(readFileSync(join(dir, ".lastro", "proposals", "free-shipping-for-members.md"), "utf8"), plan + "\n");
  const again = handle("PostToolUse", { cwd: dir, tool_name: "ExitPlanMode", tool_input: {}, tool_response: { plan } }, claude);
  assert.match(again.hookSpecificOutput.additionalContext, /free-shipping-for-members-2\.md/, "an earlier proposal is never overwritten");
  const codexPlan = handle("PostToolUse", { cwd: dir, tool_name: "update_plan", tool_input: { plan: [{ step: "edit fee", status: "pending" }] } }, codex);
  assert.match(codexPlan.hookSpecificOutput.additionalContext, /before the first source edit of this plan/);
});

test("proving streams its events to .lastro/plugin/progress.jsonl, for the mod to draw", { skip }, () => {
  const dir = specified({ build: "true" });
  handle("Stop", { cwd: dir }, claude);
  const events = readFileSync(join(dir, ".lastro", "plugin", "progress.jsonl"), "utf8").trim().split("\n").map((l) => JSON.parse(l));
  assert.deepEqual(events.map((e) => e.event).filter((e) => e !== "progress"), ["build", "start", "clause", "clause", "clause", "scenario", "crosscheck", "result"].filter((e) => e !== "crosscheck" || events.some((x) => x.event === "crosscheck")));
  assert.ok(events.some((e) => e.event === "progress"), "the shipping space has 402 inputs: progress events");
  assert.deepEqual(events.filter((e) => e.event === "clause").map((e) => [e.clauseId, e.verdict]), [["C1", "violated"], ["C2", "proved"], ["C3", "proved"]]);
  assert.equal(events.at(-1).ok, false);
});

test("a failed build is reported in the stream and sent back to the agent", { skip }, () => {
  const dir = specified({ build: "echo 'tsc: error TS2322' >&2; exit 1" });
  const out = handle("Stop", { cwd: dir }, claude);
  assert.equal(out.decision, "block");
  assert.match(out.reason, /The build failed[\s\S]*TS2322/);
  const last = readFileSync(join(dir, ".lastro", "plugin", "progress.jsonl"), "utf8").trim().split("\n").at(-1);
  assert.match(JSON.parse(last).error, /TS2322/);
});
