// The plugin's hooks (UC-040, UC-041, UC-042, UC-044) as subjects Lastro can prove: the real handle() and renderBoard()
// from scripts/, on a throwaway repository whose `lastro` CLI is a stub that answers status and prove as the model's
// inputs say. The models are in .lastro/models/.
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { handle } from "../scripts/lastro-plugin.mjs";
import { readState, renderBoard } from "../scripts/lib.mjs";

const STUB = `const a = process.argv.slice(2);
const ok = process.env.LASTRO_STUB_OK === "1";
if (a[0] === "status" && a[1] && !a[1].startsWith("--")) console.log(JSON.stringify({ status: "status", pending: [] }));
else if (a[0] === "status") console.log(JSON.stringify({ status: "status", proposals: [{ id: "P-001", status: "open" }] }));
else if (a[0] === "prove") { console.log(JSON.stringify({ event: "start" })); console.log(JSON.stringify({ event: "result", ok, feedback: ok ? "" : "P-001.PF-01 fails" })); process.exitCode = ok ? 0 : 1; }
else console.log("{}");
`;

function withRepo(plugin, f) {
  const root = mkdtempSync(join(tmpdir(), "lastro-hooks-"));
  try {
    const dir = join(root, ".lastro");
    mkdirSync(join(dir, "proposals"), { recursive: true });
    writeFileSync(join(root, "stub.mjs"), STUB);
    writeFileSync(join(dir, "plugin.json"), JSON.stringify({ enabled: true, cli: `"${process.execPath}" "${join(root, "stub.mjs")}"`, ...plugin }));
    writeFileSync(join(dir, "config.json"), JSON.stringify({ models: {}, subjects: {}, review: { approvals: 1, submitterCanApprove: true } }));
    return f(root, dir);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

// UC-040: a review recorded through the CLI is the user's: Claude Code asks, and nobody may sign as an agent.
export function reviewGuard({ harness, as, action }) {
  return withRepo({}, (root) => {
    const command = `lastro review P-001 --as ${as} ${action} ${action === "approve-all" ? "" : "I-01"}`.trim();
    const out = handle("PreToolUse", { cwd: root, tool_name: harness === "claude" ? "Bash" : "shell", tool_input: { command } }, { LASTRO_HARNESS: harness });
    return { decision: out.hookSpecificOutput?.permissionDecision ?? "none" };
  });
}

// UC-041: the Stop hook proves a derived proposal and sends failures back, up to maxIterations times.
export function stopHook({ derived, lastRun, dirty, proves, buildFails, attempts }) {
  const plugin = { prove: "code.mjs", ...(buildFails ? { build: `"${process.execPath}" -e "process.exit(1)"` } : {}) };
  return withRepo(plugin, (root, dir) => {
    const hash = "sha256:approved";
    const proposal = {
      id: "P-001", slug: "change", name: "Change", status: "open", hash, policy: { approvals: 1, submitterCanApprove: true }, submittedBy: "ana",
      intents: [], decisions: [], useCases: [], notPromised: [],
      proofs: derived ? [{ id: "P-001.PF-01", description: "y is x", intent: "P-001.I-01" }] : [],
      ...(lastRun === "none" ? {} : { lastRun: { ok: lastRun === "passed", outcomes: [], regressions: [], replacedStillPass: [], unproved: [] } }),
    };
    writeFileSync(join(dir, "proposals", "P-001-change.json"), JSON.stringify(proposal));
    mkdirSync(join(dir, "plugin"), { recursive: true });
    writeFileSync(join(dir, "plugin", "state.json"), JSON.stringify({ attempts, dirty, proposalHash: hash }));
    process.env.LASTRO_STUB_OK = proves ? "1" : "0";
    const out = handle("Stop", { cwd: root }, {});
    const progress = join(dir, "plugin", "progress.jsonl");
    const events = existsSync(progress) ? readFileSync(progress, "utf8").trim().split("\n").map((l) => JSON.parse(l).event) : [];
    const n = /attempt (\d+) of (\d+)/.exec(out.reason ?? "");
    return {
      block: out.decision === "block",
      attempt: n ? Number(n[1]) : 0,
      max: n ? Number(n[2]) : 0,
      proved: events.includes("result"),
      buildFirst: events.length === 0 || events[0] === "build",
      failuresSent: out.decision === "block" && /fails|failed/.test(out.reason ?? ""),
    };
  });
}

// UC-041: a code edit after a proof marks it stale.
export function editMarksStale({ path }) {
  return withRepo({}, (root, dir) => {
    mkdirSync(join(dir, "plugin"), { recursive: true });
    writeFileSync(join(dir, "plugin", "state.json"), JSON.stringify({ attempts: 0, dirty: false, proposalHash: null }));
    const file = { code: join(root, "src", "fee.js"), proposal: join(dir, "proposals", "P-002-next.md") }[path];
    handle("PostToolUse", { cwd: root, tool_name: "Edit", tool_input: { file_path: file } }, {});
    return { dirty: readState({ statePath: join(dir, "plugin", "state.json") }).dirty === true };
  });
}

// UC-042: one line per proof on the board.
export function boardMark({ verdict, beyond }) {
  const passed = verdict === "proved" || verdict === "evidenced";
  const s = {
    phase: verdict === "pending" ? "implementing" : passed ? "passed" : "failed",
    ws: { config: { cli: "lastro" } },
    proposal: {
      id: "P-001", name: "Change", hash: "sha256:h", policy: { approvals: 1 }, useCases: [], decisions: [],
      intents: [{ id: "P-001.I-01", status: "approved", description: "y is x", provability: "formal" }, ...(beyond ? [{ id: "P-001.I-02", status: "approved", description: "fast", provability: "beyond-proof", beyondProofReason: "termination-timing" }] : [])],
      proofs: [{ id: "P-001.PF-01", description: "y is x", intent: "P-001.I-01" }],
    },
    run: verdict === "pending" ? null : { ok: passed, outcomes: [{ proof: "P-001.PF-01", role: "proposal", verdict, passed, ...(verdict === "violated" ? { counterexample: { input: { x: 3 }, output: { y: 4 } } } : {}) }], regressions: [], replacedStillPass: [] },
  };
  const board = renderBoard(s);
  const line = board.split("\n").find((l) => l.includes("PF-01")) ?? "";
  const mark = line.trim().split(" ")[0];
  return { mark: ["✓", "≈", "✗", "!", "○"].includes(mark) ? mark : "other", counterexample: board.includes('{"x":3}'), beyondListed: /I-02 beyond proof/.test(board), nextStep: /\nNext: /.test(board) };
}

// UC-044: a plan becomes a proposal's prose.
export function planToProposal({ tool, existing, heading }) {
  return withRepo({}, (root, dir) => {
    for (let k = 1; k <= existing; k++) writeFileSync(join(dir, "proposals", `P-00${k}-old-${k}.md`), "Old.\n");
    const plan = heading ? "# Faster checkout\n\nMake the checkout faster." : "Make the checkout faster.\nThen ship it.";
    const out = handle("PostToolUse", { cwd: root, tool_name: tool, tool_input: { plan }, tool_response: { plan } }, {});
    const added = readdirSync(join(dir, "proposals")).filter((f) => !/^P-00\d-old-/.test(f));
    const m = /^P-(\d+)-(.+)\.md$/.exec(added[0] ?? "");
    const context = out.hookSpecificOutput?.additionalContext ?? "";
    return {
      saved: added.length === 1,
      id: m ? Number(m[1]) : 0,
      slugFromHeading: !!m && m[2] === "faster-checkout",
      reminder: /propose/.test(context) && /before/i.test(context),
    };
  });
}
