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

/** init → propose (with the extraction answered) → review pending. */
function planned(config, approvals = 1) {
  const dir = repoWith(config);
  assert.equal(lastro(dir, "init", "--model", "model.json", "--approvals", String(approvals)).code, 0);
  const r = lastro(dir, "propose", ".lastro/proposals/shipping.md", "--as", "ana");
  assert.equal(r.code, 3);
  copyFileSync(join(example, "answers", "propose.json"), r.body.answer);
  assert.equal(lastro(dir, "propose", ".lastro/proposals/shipping.md", "--as", "ana").code, 1);
  return dir;
}

/** ... → the user's review → proofs derived. */
function specified(config) {
  const dir = planned(config);
  lastro(dir, "review", "P-001", "--as", "ana", "answer", "D1", "1");
  lastro(dir, "review", "P-001", "--as", "ana", "approve-all");
  const r = lastro(dir, "derive", "P-001");
  copyFileSync(join(example, "answers", "proofs.json"), r.body.answer);
  assert.equal(lastro(dir, "derive", "P-001").code, 0);
  return dir;
}

const edit = (dir, file_path, env = claude) => handle("PreToolUse", { cwd: dir, tool_name: "Write", tool_input: { file_path, content: "x" } }, env);
const decision = (out) => out.hookSpecificOutput?.permissionDecision ?? "allow";

test("without .lastro/plugin.json the hooks do nothing", () => {
  const dir = mkdtempSync(join(tmpdir(), "lastro-plugin-none-"));
  assert.deepEqual(edit(dir, "src/a.js"), {});
  assert.deepEqual(handle("Stop", { cwd: dir }, claude), {});
});

test("enabled: false turns every hook off; an older gate: \"off\" still means the same", { skip }, () => {
  for (const config of [{ enabled: false }, { gate: "off" }]) {
    const dir = planned(config);
    assert.equal(loadWorkspace(dir).config.enabled, false);
    assert.equal("gate" in loadWorkspace(dir).config, false);
    assert.deepEqual(edit(dir, "src/fee.js"), {});
    assert.deepEqual(handle("Stop", { cwd: dir }, claude), {});
    assert.deepEqual(handle("SessionStart", { cwd: dir }, claude), {});
  }
  const on = planned({ gate: "on" });
  assert.equal(loadWorkspace(on).config.enabled, true);
  assert.equal(decision(edit(on, "src/fee.js")), "deny");
});

test("init without --model starts an empty workspace that the gate accepts", { skip }, () => {
  const dir = repoWith({ enabled: true });
  const r = lastro(dir, "init");
  assert.equal(r.code, 0);
  assert.deepEqual(r.body.models, []);
  assert.equal(lastro(dir, "init", "--subject", "./shipping.mjs").code, 2);
  assert.equal(snapshot(loadWorkspace(dir)).phase, "no-proposal");
  assert.equal(decision(edit(dir, "src/fee.js")), "deny");
});

test("paths: Claude Code edits and Codex patches", () => {
  assert.deepEqual(editedPaths({ file_path: "/r/src/a.ts" }), ["/r/src/a.ts"]);
  const patch = "*** Begin Patch\n*** Update File: src/fee.js\n@@\n-a\n+b\n*** Add File: src/new.js\n+x\n*** Update File: src/old.js\n*** Move to: src/moved.js\n*** End Patch";
  assert.deepEqual(editedPaths({ command: ["apply_patch", patch] }).sort(), ["src/fee.js", "src/moved.js", "src/new.js", "src/old.js"]);
  assert.ok(globToRegExp("docs/**").test("docs/a/b.md"));
  assert.ok(globToRegExp("**/*.md").test("README.md"));
  assert.ok(!globToRegExp("src/*.js").test("src/a/b.js"));
  assert.ok(isReviewCommand("npx lastro review P-001 --as ana approve I-01"));
  assert.ok(isReviewCommand("lastro-go review P-001 --as ana answer D1 0"));
  assert.ok(isReviewCommand("lastro review 1 --as ana resolve T1"));
  assert.ok(!isReviewCommand("lastro status"));
});

test("before any review, code edits wait; proposals and allowed paths do not", { skip }, () => {
  const dir = repoWith();
  lastro(dir, "init", "--model", "model.json");
  const ws = loadWorkspace(dir);
  assert.equal(snapshot(ws).phase, "no-proposal");
  const out = edit(dir, "src/fee.js");
  assert.equal(decision(out), "deny");
  assert.match(out.hookSpecificOutput.permissionDecisionReason, / propose \.lastro\/proposals/);
  assert.equal(decision(edit(dir, ".lastro/proposals/next.md")), "allow");
  assert.equal(decision(edit(dir, "docs/notes.md")), "allow");
  assert.equal(classify(ws, dir, join(dir, ".lastro", "llm", "x.answer.json")), "free");
  assert.equal(classify(ws, dir, join(dir, ".lastro", "usecases", "UC-001-x.json")), "generated");
  assert.equal(classify(ws, dir, join(dir, ".lastro", "proposals", "P-001-x.json")), "generated");
  assert.equal(classify(ws, dir, join(dir, ".lastro", "proposals", "P-001-x.md")), "free");
});

test("review pending: the board lists decisions and intents, edits are denied, the CLI files are protected", { skip }, () => {
  const dir = planned();
  const s = snapshot(loadWorkspace(dir));
  assert.equal(s.phase, "review-pending");
  assert.deepEqual(tally(s), { intents: 4, approved: 0, rejected: 0, decisions: 1, answered: 0, proofs: 0, passed: 0, failed: 0, regressions: 0 });
  const board = renderBoard(s);
  assert.match(board, /^Lastro · P-001 Shipping fee at checkout · waiting for the user's review/);
  assert.match(board, /\? D1 Is the 100 threshold measured before or after discounts\?/);
  assert.match(board, /1\. After discounts \(recommended\)/);
  assert.match(board, /UC-001 Charge shipping at checkout \(new\)/);
  assert.match(board, /○ I-01 An order with a subtotal of 100 or more pays no shipping\./);
  assert.equal(renderLine(s), "Lastro P-001 · review 0/4 intents approved · 0/1 decisions answered");

  const out = edit(dir, "src/fee.js");
  assert.equal(decision(out), "deny");
  assert.match(out.hookSpecificOutput.permissionDecisionReason, /D1 unanswered/);
  assert.equal(decision(edit(dir, ".lastro/proposals/P-001-shipping-fee.json")), "deny");
  assert.equal(decision(edit(dir, ".lastro/usecases/UC-001-charge-shipping.json", codex)), "deny");
  assert.equal(decision(edit(dir, ".lastro/plugin.json")), "ask");
  assert.equal(decision(edit(dir, ".lastro/plugin.json", codex)), "deny");

  // The UserPromptSubmit context tells the agent it is the author and shows the board.
  const ctx = handle("UserPromptSubmit", { cwd: dir, prompt: "go" }, claude).hookSpecificOutput.additionalContext;
  assert.match(ctx, /You are the author/);
  assert.match(ctx, /Decisions    0\/1 answered/);
});

test("with two required approvals, the board shows each intent's approvals, like a pull request", { skip }, () => {
  const dir = planned({}, 2);
  lastro(dir, "review", "P-001", "--as", "ana", "answer", "D1", "1");
  lastro(dir, "review", "P-001", "--as", "ana", "approve-all");
  const s = snapshot(loadWorkspace(dir));
  assert.equal(s.phase, "review-pending", "one reviewer is not enough");
  assert.match(renderBoard(s), /○ I-01 \(1\/2 approvals\) An order/);
  assert.equal(decision(edit(dir, "src/fee.js")), "deny");
  lastro(dir, "review", "P-001", "--as", "bruno", "approve-all");
  assert.equal(snapshot(loadWorkspace(dir)).phase, "needs-proofs");
});

test("the review is the user's: Claude Code asks, an agent name is refused", { skip }, () => {
  const dir = planned();
  const bash = (command, env = claude) => handle("PreToolUse", { cwd: dir, tool_name: "Bash", tool_input: { command } }, env);
  assert.equal(decision(bash(`${CLI} review P-001 --as ana approve I-01`)), "ask");
  assert.equal(decision(bash("lastro review P-001 --as claude approve-all")), "deny");
  assert.equal(decision(bash("lastro review P-001 --as codex approve I-01", codex)), "deny");
  assert.equal(decision(bash("lastro review P-001 --as ana approve I-01", codex)), "allow");
  assert.equal(decision(bash("lastro status")), "allow");
  // After a lastro command the board is shown to the user.
  assert.match(handle("PostToolUse", { cwd: dir, tool_name: "Bash", tool_input: { command: "lastro status" } }, claude).systemMessage, /^Lastro · P-001 Shipping fee at checkout/);
});

test("approved but no proofs: still gated; after derive, edits go through and the proofs are pending", { skip }, () => {
  const dir = planned();
  lastro(dir, "review", "P-001", "--as", "ana", "answer", "D1", "1");
  lastro(dir, "review", "P-001", "--as", "ana", "approve-all");
  assert.equal(snapshot(loadWorkspace(dir)).phase, "needs-proofs");
  assert.match(edit(dir, "src/fee.js").hookSpecificOutput.permissionDecisionReason, / derive P-001`/);

  const r = lastro(dir, "derive", "P-001");
  copyFileSync(join(example, "answers", "proofs.json"), r.body.answer);
  lastro(dir, "derive", "P-001");
  const s = snapshot(loadWorkspace(dir));
  assert.equal(s.phase, "implementing");
  assert.equal(decision(edit(dir, "src/fee.js")), "allow");
  const board = renderBoard(s);
  assert.match(board, /○ PF-01 pending/);
  assert.match(board, /– I-04 beyond proof \(termination-timing\)/);
  assert.equal(renderLine(s), "Lastro P-001 · 4 proofs pending · ○ 4");
});

test("Stop proves: failures go back to the agent until the attempts run out", { skip }, () => {
  const dir = specified();
  handle("PostToolUse", { cwd: dir, tool_name: "Write", tool_input: { file_path: join(dir, "shipping-buggy.mjs") } }, claude);
  let out = handle("Stop", { cwd: dir, stop_hook_active: false }, claude);
  assert.equal(out.decision, "block");
  assert.match(out.reason, /attempt 1 of 2/);
  assert.match(out.reason, /P-001.PF-01 \(P-001.I-01\)/);
  assert.match(out.reason, /\.lastro\/proposals\/P-001-shipping-fee\.json/);
  assert.match(out.reason, /counterexample: \{"member":false,"subtotal":100\} returned \{"fee":15\}/);
  assert.match(out.systemMessage, /proofs FAILED/);
  assert.match(out.systemMessage, /✗ PF-01 violated/);
  assert.match(out.systemMessage, /✓ PF-02 proved/);
  assert.match(out.systemMessage, /Verification 3\/4 passed, 1 failed/);
  assert.equal(renderLine(snapshot(loadWorkspace(dir))), "Lastro P-001 · proofs 3/4 passed · ✗ 1 failed · FAILED");

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
  assert.match(out.systemMessage, /✓ PF-01 proved/);
  assert.match(out.systemMessage, /apply P-001/);
  assert.equal(renderLine(snapshot(loadWorkspace(dir))), "Lastro P-001 · proofs 4/4 passed · PASSED");
  // An edit after the proof makes it stale: pending again until the next Stop.
  handle("PostToolUse", { cwd: dir, tool_name: "apply_patch", tool_input: { command: "*** Begin Patch\n*** Update File: shipping.mjs\n*** End Patch" } }, codex);
  assert.equal(snapshot(loadWorkspace(dir)).phase, "implementing");
  assert.match(handle("Stop", { cwd: dir }, codex).systemMessage, /all proofs PASSED/);
});

test("an amended intent withdraws the approval and gates the code again", { skip }, () => {
  const dir = specified({ prove: "./shipping.mjs" });
  lastro(dir, "review", "P-001", "--as", "ana", "amend", "I-03", "A non-member order under 100 pays a shipping fee of exactly 20.");
  // The amendment withdraws the approval and the proofs: the use case's description must be rewritten and reviewed again.
  const s = snapshot(loadWorkspace(dir));
  assert.equal(s.phase, "review-pending");
  assert.match(renderReview(s), /\(new, description stale\)/);
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
  assert.equal(line.stdout.trim(), "Lastro P-001 · proofs 4/4 passed · PASSED");
  assert.ok(existsSync(join(dir, ".lastro", "proposals", "P-001-shipping-fee.receipt.md")));
  assert.ok(readFileSync(join(dir, ".lastro", "plugin", "state.json"), "utf8").includes('"dirty": false'));
});

test("both manifests and the hooks file point at files that exist", () => {
  const root = join(here, "..");
  const codexManifest = JSON.parse(readFileSync(join(root, ".codex-plugin", "plugin.json"), "utf8"));
  const claudeManifest = JSON.parse(readFileSync(join(root, ".claude-plugin", "plugin.json"), "utf8"));
  assert.equal(codexManifest.name, claudeManifest.name);
  assert.equal(codexManifest.version, claudeManifest.version);
  assert.ok(existsSync(join(root, codexManifest.hooks)));
  for (const s of ["setup", "bootstrap", "propose", "verify"]) assert.match(readFileSync(join(root, "skills", s, "SKILL.md"), "utf8"), new RegExp(`^---\\nname: ${s}\\n`));
  const hooks = JSON.parse(readFileSync(join(root, "hooks", "hooks.json"), "utf8")).hooks;
  assert.deepEqual(Object.keys(hooks), ["SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"]);
  for (const groups of Object.values(hooks)) for (const g of groups) for (const h of g.hooks) assert.match(h.command, /\$\{CLAUDE_PLUGIN_ROOT\}\/scripts\/lastro-plugin\.mjs" hook \w+$/);
});

test("the review reads one item per line, grouped by use case, with the quote each intent came from", { skip }, () => {
  const dir = planned();
  const review = renderReview(snapshot(loadWorkspace(dir)));
  const lines = review.split("\n");
  assert.equal(lines[0], "**P-001 Shipping fee at checkout, for review:** 4 intents on 1 use case, 0 approved · 1 of 1 decisions open");
  // Every decision, option and intent is its own list item, so a chat that joins single newlines keeps them apart.
  assert.ok(lines.includes("- **D1** Is the 100 threshold measured before or after discounts?"));
  assert.ok(lines.some((l) => /^  1\. After discounts.*_\(recommended\)_$/.test(l)));
  assert.deepEqual(lines.filter((l) => /^\*\*[A-Z]/.test(l) && !l.startsWith("**P-001")), ["**Decisions**", "**UC-001 Charge shipping at checkout** (new)"]);
  const i1 = lines.indexOf("- **I-01** An order with a subtotal of 100 or more pays no shipping. _(to review)_");
  assert.ok(i1 > 0);
  assert.match(lines[i1 + 1], /^  > \S/);
  assert.equal(lines.filter((l) => /^- \*\*I-\d/.test(l)).length, 4);

  // After a lastro command, while the review is open, the agent gets it to show as-is.
  const out = handle("PostToolUse", { cwd: dir, tool_name: "Bash", tool_input: { command: "lastro status" } }, claude);
  assert.ok(out.hookSpecificOutput.additionalContext.endsWith(review));

  lastro(dir, "review", "P-001", "--as", "ana", "answer", "D1", "1");
  lastro(dir, "review", "P-001", "--as", "ana", "approve", "I-01");
  assert.match(renderReview(snapshot(loadWorkspace(dir))), /^- \*\*I-01\*\* .* _\(approved\)_$/m);
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
  lastro(dir, "review", "P-001", "--as", "ana", "answer", "D1", "1");
  lastro(dir, "review", "P-001", "--as", "ana", "approve-all");
  const r = lastro(dir, "derive", "P-001");
  copyFileSync(join(example, "answers", "proofs.json"), r.body.answer);
  lastro(dir, "derive", "P-001");
  assert.equal(asDocs(renderBoard(snapshot(loadWorkspace(dir)))), docs.implementing);
  assert.equal(asDocs(handle("Stop", { cwd: dir }, claude).systemMessage), docs.failed);
});

test("in any mode, the first source edit triggers the extraction: the reason says how", { skip }, () => {
  const dir = repoWith();
  lastro(dir, "init", "--model", "model.json");
  const reason = edit(dir, "src/fee.js").hookSpecificOutput.permissionDecisionReason;
  assert.match(reason, /^Before editing src\/fee\.js, Lastro needs the intents of this change, reviewed by the user:/);
  assert.match(reason, /formal or not/);
  assert.match(reason, / propose \.lastro\/proposals\/<slug>\.md/);
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

test("plan mode: the approved plan becomes the prose of the next proposal; Codex's update_plan gets a reminder", { skip }, () => {
  const dir = repoWith();
  lastro(dir, "init", "--model", "model.json");
  const plan = "# Free shipping for members\n\n1. Members never pay shipping.\n2. Update the fee function.";
  const out = handle("PostToolUse", { cwd: dir, tool_name: "ExitPlanMode", tool_input: {}, tool_response: { plan, filePath: "/home/u/.claude/plans/x.md" } }, claude);
  assert.match(out.hookSpecificOutput.additionalContext, /new proposal, \.lastro\/proposals\/P-001-free-shipping-for-members\.md/);
  assert.match(out.hookSpecificOutput.additionalContext, / propose \.lastro\/proposals\/P-001-free-shipping-for-members\.md/);
  assert.equal(readFileSync(join(dir, ".lastro", "proposals", "P-001-free-shipping-for-members.md"), "utf8"), plan + "\n");
  const again = handle("PostToolUse", { cwd: dir, tool_name: "ExitPlanMode", tool_input: {}, tool_response: { plan } }, claude);
  assert.match(again.hookSpecificOutput.additionalContext, /P-002-free-shipping-for-members\.md/, "an earlier proposal is never overwritten");
  const codexPlan = handle("PostToolUse", { cwd: dir, tool_name: "update_plan", tool_input: { plan: [{ step: "edit fee", status: "pending" }] } }, codex);
  assert.match(codexPlan.hookSpecificOutput.additionalContext, /before the first source edit of this plan/);
});

test("proving streams its events to .lastro/plugin/progress.jsonl, for the mod to draw", { skip }, () => {
  const dir = specified({ build: "true" });
  handle("Stop", { cwd: dir }, claude);
  const events = readFileSync(join(dir, ".lastro", "plugin", "progress.jsonl"), "utf8").trim().split("\n").map((l) => JSON.parse(l));
  assert.deepEqual(events.map((e) => e.event).filter((e) => e !== "progress"), ["build", "start", "proof", "proof", "proof", "proof", "crosscheck", "result"].filter((e) => e !== "crosscheck" || events.some((x) => x.event === "crosscheck")));
  assert.ok(events.some((e) => e.event === "progress"), "the shipping space has 402 inputs: progress events");
  assert.deepEqual(events.filter((e) => e.event === "proof").map((e) => [e.proof, e.verdict]), [["P-001.PF-01", "violated"], ["P-001.PF-02", "proved"], ["P-001.PF-03", "proved"], ["P-001.PF-04", "evidenced"]]);
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

test("a bootstrap's failing proofs go to the user's review, never back to the code", { skip }, () => {
  const dir = repoWith();
  assert.equal(lastro(dir, "init", "--model", "model.json").code, 0);
  assert.match(edit(dir, join(dir, "shipping.mjs")).hookSpecificOutput.permissionDecisionReason, /No use case is recorded yet.*cli\.js bootstrap` \(lastro `bootstrap` skill\)/);
  let r = lastro(dir, "bootstrap", "--as", "ana");
  assert.equal(r.code, 3);
  assert.equal(r.body.step, "survey");
  copyFileSync(join(example, "answers", "bootstrap.json"), r.body.answer);
  r = lastro(dir, "bootstrap", "--as", "ana");
  assert.equal(r.code, 1, JSON.stringify(r.body));
  lastro(dir, "review", "P-001", "--as", "ana", "approve-all");
  r = lastro(dir, "derive", "P-001");
  copyFileSync(join(example, "answers", "proofs.json"), r.body.answer);
  const review = renderReview(snapshot(loadWorkspace(dir)));
  assert.match(review, /_Bootstrap: read from the working tree\. Each intent is what the code does today; check its grounds\._/);
  assert.match(review, /- \*\*I-01\*\* An order with a subtotal of 100 or more pays no shipping\. _\(approved\)_\n  ↳ medium confidence; files `shipping\.mjs`/);
  assert.equal(lastro(dir, "derive", "P-001").code, 0);

  handle("PostToolUse", { cwd: dir, tool_name: "Write", tool_input: { file_path: join(dir, "notes.txt") } }, claude);
  const out = handle("Stop", { cwd: dir, stop_hook_active: false }, claude);
  assert.equal(out.decision, undefined, "no fix-the-code loop");
  assert.match(out.systemMessage, /proofs FAILED/);
  assert.match(out.systemMessage, /do not change the code/);
});
