// The Lastro plugin's logic, shared by Claude Code and Codex. It never decides anything itself: every proposal,
// intent, decision and proof comes from the `lastro` CLI and the files it keeps in `.lastro/`. The plugin only
// reads them to (1) keep code edits behind an approved proposal with derived proofs, (2) run the proofs when the agent stops and hand the
// failures back, and (3) render the progress board people see.
import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, isAbsolute, join, relative, resolve, sep } from "node:path";

export const PLUGIN_FILE = join(".lastro", "plugin.json");

/** Files only the CLI writes (relative to the workspace). The agent never edits them by hand. */
export const GENERATED = ["config.json", "intents.json", "spec.json", "proof.json", "receipt.md"];

/** Folders whose JSON files only the CLI writes: the use cases, the proposals and their receipts. */
const isGenerated = (inWs) =>
  GENERATED.includes(inWs) || inWs.startsWith("plugin/") || inWs.startsWith("usecases/") || (inWs.startsWith("proposals/") && (inWs.endsWith(".json") || inWs.endsWith(".receipt.md") || inWs.endsWith(".runs.jsonl")));

export const DEFAULTS = {
  /** The command that runs the Lastro CLI: `lastro`, `npx lastro`, `lastro-go`, `node path/to/cli.js`... */
  cli: "lastro",
  /** The workspace folder, relative to the repository root. */
  dir: ".lastro",
  /** Shell command that builds the code under proof before proving (optional). */
  build: null,
  /** The arguments after `lastro prove`, e.g. "dist/fee.js --export shippingFee" (required to prove). */
  prove: null,
  /** true: the hooks run (code edits wait for an approved proposal, proofs run on Stop, the board shows); false: the plugin does nothing. */
  enabled: true,
  /** Paths (globs, relative to the root) the gate never blocks. `.lastro/` proposals and handoff answers are always allowed. */
  allow: [],
  /** How many times the Stop hook sends failing proofs back to the agent before handing over to the user. */
  maxIterations: 5,
};

// ---------------------------------------------------------------------------
// Workspace
// ---------------------------------------------------------------------------

/** Finds the repository root that holds `.lastro/plugin.json`, walking up from `cwd`. */
export function findRoot(cwd) {
  let dir = resolve(cwd);
  for (;;) {
    if (existsSync(join(dir, PLUGIN_FILE))) return dir;
    const up = dirname(dir);
    if (up === dir) return null;
    dir = up;
  }
}

const readJson = (p) => {
  try {
    return JSON.parse(readFileSync(p, "utf8"));
  } catch {
    return undefined;
  }
};

export function loadWorkspace(cwd, env = process.env) {
  const root = findRoot(cwd);
  if (!root) return null;
  const raw = readJson(join(root, PLUGIN_FILE)) ?? {};
  const config = { ...DEFAULTS, ...raw };
  // Before `enabled`, the switch was `gate: "on" | "off"`; an old file keeps meaning what it meant.
  if (raw.enabled === undefined && raw.gate === "off") config.enabled = false;
  config.enabled = config.enabled !== false;
  delete config.gate;
  if (env.LASTRO_CLI) config.cli = env.LASTRO_CLI;
  const dir = resolve(root, config.dir);
  return { root, dir, config, file: (name) => join(dir, name), statePath: join(dir, "plugin", "state.json") };
}

/** Runs the CLI in the repository root; every command prints one JSON object. */
export function runCli(ws, args, opts = {}) {
  const cmd = `${ws.config.cli} ${args} --dir ${shellQuote(ws.dir)}`;
  const r = spawnSync(cmd, { cwd: ws.root, shell: true, encoding: "utf8", env: opts.env ?? process.env, timeout: opts.timeout ?? 120_000 });
  let body;
  try {
    body = JSON.parse(r.stdout);
  } catch {
    body = { error: (r.stderr || r.stdout || r.error?.message || "no output").trim() };
  }
  return { code: r.status ?? 2, body, command: cmd };
}

const shellQuote = (s) => (/^[\w./:@-]+$/.test(s) ? s : `'${s.replace(/'/g, `'\\''`)}'`);

export function readState(ws) {
  return readJson(ws.statePath) ?? { attempts: 0, dirty: false, proposalHash: null };
}

export function writeState(ws, state) {
  mkdirSync(dirname(ws.statePath), { recursive: true });
  writeFileSync(ws.statePath, JSON.stringify(state, null, 2) + "\n");
}

/** The proposal file the CLI keeps for `id` ("P-001"), read straight from `.lastro/proposals/`. */
export function readProposal(ws, id) {
  const dir = ws.file("proposals");
  if (!id || !existsSync(dir)) return undefined;
  const f = readdirSync(dir).find((x) => x.startsWith(`${id}-`) && x.endsWith(".json"));
  return f ? readJson(join(dir, f)) : undefined;
}

/** The use cases, for the descriptions of the proofs an intent replaces. */
function readUseCases(ws) {
  const dir = ws.file("usecases");
  return existsSync(dir) ? readdirSync(dir).filter((f) => f.endsWith(".json")).map((f) => readJson(join(dir, f))).filter(Boolean) : [];
}

/**
 * Where the current proposal (the latest one not applied or closed) stands. Phases, in order:
 *   no-workspace · no-proposal · review-pending · needs-proofs · implementing · failed · passed
 */
export function snapshot(ws) {
  if (!existsSync(ws.file("config.json"))) return { phase: "no-workspace", ws };
  const list = runCli(ws, "status");
  const open = (Array.isArray(list.body.proposals) ? list.body.proposals : []).filter((p) => p.status !== "applied" && p.status !== "closed");
  const current = open.at(-1);
  const proposal = current && readProposal(ws, current.id);
  if (!proposal) return { phase: "no-proposal", ws, cliError: list.body.error };

  const status = runCli(ws, `status ${proposal.id}`);
  const pending = Array.isArray(status.body.pending) ? status.body.pending : [];
  const state = readState(ws);
  const run = proposal.lastRun;
  // A code edit after the last run makes it stale until the next one.
  const stale = !!run && state.dirty && state.proposalHash === proposal.hash;

  let phase;
  if (!proposal.hash) phase = "review-pending";
  else if (!proposal.proofs.length) phase = "needs-proofs";
  else if (!run || stale) phase = "implementing";
  else phase = run.ok ? "passed" : "failed";

  return { phase, ws, proposal, pending: proposal.hash ? [] : pending, run: run && !stale ? run : null, stale, state, useCases: readUseCases(ws), cliError: status.body.error };
}

/** The next thing to do, as an instruction for the agent. */
export function nextStep(s) {
  const cli = s.ws?.config.cli ?? "lastro";
  const id = s.proposal?.id ?? "<P>";
  switch (s.phase) {
    case "no-workspace":
      return `Set up the workspace first: draft the model of the decision core with the user, then run \`${cli} init --model <model.json>\`.`;
    case "no-proposal":
      return s.useCases?.length
        ? `Write the proposed change as prose in .lastro/proposals/<slug>.md and run \`${cli} propose .lastro/proposals/<slug>.md\`.`
        : `No use case is recorded yet. If the repository already has code, start with \`${cli} scan\` and \`${cli} bootstrap\` (lastro \`bootstrap\` skill); otherwise write the first change as prose in .lastro/proposals/<slug>.md and run \`${cli} propose\` on it.`;
    case "review-pending":
      return `The user reviews the proposal like a pull request: answer each open decision, then approve, amend or reject each intent (\`${cli} review ${id} --as <user> ...\`, recording only the user's own answers).`;
    case "needs-proofs":
      return `Every intent is reviewed. Derive the proofs with \`${cli} derive ${id}\`.`;
    case "implementing":
      return "Implement the approved intents, then end your turn: the proofs run when you stop.";
    case "failed":
      return s.proposal?.origin?.kind === "bootstrap"
        ? "This bootstrap's proofs describe the code as it is: do not change the code. Show the user each failing proof; on their word amend the intent (the reading was wrong) or reject it (the code has a bug to fix in a later proposal), then derive and prove again."
        : "Fix the code until every proof passes, without breaking the use cases' proofs.";
    case "passed":
      return `Every proof passes. The user applies it with \`${cli} apply ${id}\`.`;
  }
}

// ---------------------------------------------------------------------------
// The progress board
// ---------------------------------------------------------------------------

const VERDICT_MARK = { proved: "✓", evidenced: "≈", violated: "✗", vacuous: "!", weak: "!", "not-run": "?" };

/** Short ids inside a proposal: "P-001.I-03" reads "I-03". */
const short = (s, id) => (s.proposal && id.startsWith(`${s.proposal.id}.`) ? id.slice(s.proposal.id.length + 1) : id);

/** An intent goes forward once it has the approvals the proposal's policy requires, like a pull request. */
function approvalsOf(p, i) {
  if (i.status !== "approved" && i.status !== "amended") return 0;
  return (i.approvals ?? []).filter((a) => p.policy?.submitterCanApprove !== false || a !== p.submittedBy).length;
}
const requiredOf = (p) => Math.max(1, Number(p?.policy?.approvals ?? 1));
const isApproved = (p, i) => approvalsOf(p, i) >= requiredOf(p);

/** Counts for the one-line view and for tests. */
export function tally(s) {
  const p = s.proposal;
  const intents = p?.intents ?? [];
  const decisions = p?.decisions ?? [];
  const proofs = p?.proofs ?? [];
  const own = (s.run?.outcomes ?? []).filter((o) => o.role === "proposal");
  const passed = own.filter((o) => o.passed).length;
  return {
    intents: intents.length,
    approved: intents.filter((i) => isApproved(p, i)).length,
    rejected: intents.filter((i) => i.status === "rejected").length,
    decisions: decisions.length,
    answered: decisions.filter((d) => d.chosen !== undefined && d.chosen !== null).length,
    proofs: proofs.length,
    passed: s.run ? passed : 0,
    failed: s.run ? own.length - passed : 0,
    regressions: s.run?.regressions.length ?? 0,
  };
}

const PHASE_LABEL = {
  "no-workspace": "not set up",
  "no-proposal": "no proposal yet",
  "review-pending": "waiting for the user's review",
  "needs-proofs": "reviewed, proofs not derived yet",
  implementing: "proofs ready, implementation in progress",
  failed: "proofs FAILED",
  passed: "all proofs PASSED",
};

/** One line, for a status line or a short system message. */
export function renderLine(s) {
  const t = tally(s);
  const head = s.proposal ? `Lastro ${s.proposal.id}` : "Lastro";
  switch (s.phase) {
    case "no-workspace":
    case "no-proposal":
      return `${head} · ${PHASE_LABEL[s.phase]}`;
    case "review-pending":
      return `${head} · review ${t.approved}/${t.intents} intents approved · ${t.answered}/${t.decisions} decisions answered`;
    case "needs-proofs":
      return `${head} · ${t.approved}/${t.intents} intents approved · proofs not derived yet`;
    case "implementing":
      return `${head} · ${t.proofs} proofs pending · ○ ${t.proofs}`;
    default:
      return `${head} · proofs ${t.passed}/${t.proofs} passed${t.failed ? ` · ✗ ${t.failed} failed` : ""}${t.regressions ? ` · ✗ ${t.regressions} regressions` : ""} · ${s.phase === "passed" ? "PASSED" : "FAILED"}`;
  }
}

/** Board width: long statements wrap under their text instead of running past the terminal's edge. */
const BOARD_WIDTH = 100;

/** Word-wraps one board line: `first` prefixes its first line, `rest` the lines that continue it. */
export function wrap(text, first, rest, width = BOARD_WIDTH) {
  const lines = [];
  let line = first;
  let empty = true;
  for (const word of text.split(/\s+/).filter(Boolean)) {
    if (!empty && line.length + 1 + word.length > width) {
      lines.push(line);
      line = rest + word;
    } else line += (empty || line.endsWith(" ") ? "" : " ") + word;
    empty = false;
  }
  lines.push(line);
  return lines;
}

const plural = (n, word) => `${n} ${word}${n === 1 ? "" : "s"}`;

const CHANGE_LABEL = { add: "new", modify: "changed", remove: "removed" };

/** Intents grouped by the use case they change, in the proposal's order. */
export function groupByUseCase(p) {
  return (p.useCases ?? []).map((c) => ({ change: c, intents: p.intents.filter((i) => c.intents.includes(i.id)) }));
}

/** The proofs an intent breaks on purpose, with what each one guaranteed. */
function replacedLines(s, i) {
  const all = (s.useCases ?? []).flatMap((u) => u.proofs ?? []);
  return (i.replaces ?? []).map((r) => {
    const old = all.find((x) => x.id === r.proof);
    return `replaces ${r.proof}${old ? ` (${old.description})` : ""}: ${r.reason}`;
  });
}

/**
 * The review as Markdown, for the agent to show the user as-is: the open decisions, then the intents grouped by the
 * use case they change, each with the quote it came from and the proofs it replaces. Every item starts with a list
 * marker, so chat apps that join single newlines still show one per line.
 */
/** Where a bootstrap intent was read: its confidence, files, tests and commits. */
function groundsLine(g) {
  const parts = [`${g.confidence} confidence`];
  if (g.files?.length) parts.push(`files ${g.files.map((f) => `\`${f}\``).join(", ")}`);
  if (g.tests?.length) parts.push(`tests ${g.tests.map((t) => `\`${t}\``).join(", ")}`);
  if (g.commits?.length) parts.push(`commits ${g.commits.map((c) => `\`${c.slice(0, 7)}\``).join(", ")}`);
  return `↳ ${parts.join("; ")}`;
}

export function renderReview(s) {
  const p = s.proposal;
  if (!p) return `Lastro: no proposal to review. ${nextStep(s)}`;
  const required = requiredOf(p);
  const open = p.decisions.filter((d) => d.chosen === undefined || d.chosen === null);
  const t = tally(s);
  const out = [`**${p.id} ${p.name}, for review:** ${plural(t.intents, "intent")} on ${plural(p.useCases.length, "use case")}, ${t.approved} approved · ${open.length} of ${t.decisions} decisions open`];
  if (p.origin?.kind === "bootstrap")
    out.push("", `_Bootstrap: read from ${p.origin.head ? `commit ${p.origin.head.slice(0, 12)}` : "the working tree"}${p.origin.scope?.length ? `, scope ${p.origin.scope.join(", ")}` : ""}. Each intent is what the code does today; check its grounds._`);
  if (open.length) {
    out.push("", "**Decisions**", "");
    for (const d of open) {
      out.push(`- **${d.id}** ${d.question}`);
      d.options.forEach((o, n) => out.push(`  ${n}. ${o.label ?? o}${o.consequence ? `: ${o.consequence}` : ""}${n === d.recommended ? " _(recommended)_" : ""}`));
    }
  }
  const status = (i) =>
    isApproved(p, i) ? (i.status === "amended" ? "amended" : "approved") : i.status === "rejected" ? "rejected" : required > 1 ? `${approvalsOf(p, i)}/${required} approvals` : "to review";
  for (const { change, intents } of groupByUseCase(p)) {
    out.push("", `**${change.useCase} ${change.name}** (${CHANGE_LABEL[change.change]}${change.stale ? ", description stale" : ""})`, "");
    for (const i of intents) {
      out.push(`- **${short(s, i.id)}** ${i.description} _(${status(i)})_`);
      if (i.grounds) out.push(`  ${groundsLine(i.grounds)}`);
      else if (i.source?.quote) out.push(`  > ${i.source.quote.replace(/\s+/g, " ").trim()}`);
      for (const r of replacedLines(s, i)) out.push(`  - ⚠ ${r}`);
    }
  }
  const threads = (p.threads ?? []).filter((x) => !x.resolved);
  if (threads.length) {
    out.push("", "**Open threads**", "");
    for (const x of threads) out.push(`- **${x.id}** on ${short(s, x.on)}, ${x.by}: ${x.comments.at(-1)?.text ?? ""}`);
  }
  return out.join("\n");
}

/** The full board: stages, the review while it is open, and one line per proof. */
export function renderBoard(s) {
  if (s.phase === "no-workspace") return `Lastro · ${PHASE_LABEL[s.phase]}\n${nextStep(s)}`;
  const p = s.proposal;
  const t = tally(s);
  const out = [`Lastro${p ? ` · ${p.id} ${p.name}` : ""} · ${PHASE_LABEL[s.phase]}`, ""];
  const reviewDone = !!p?.hash;
  const stage = (done, current, label) => out.push(`  ${done ? "✓" : current ? "▸" : "○"} ${label}`);
  stage(!!p, s.phase === "no-proposal", `Intents      ${t.intents ? `${t.intents} extracted on ${plural(p.useCases.length, "use case")}` : "none yet"}`);
  stage(!!p && t.decisions === t.answered, s.phase === "review-pending", `Decisions    ${t.answered}/${t.decisions} answered`);
  stage(reviewDone, s.phase === "review-pending", `Approval     ${t.approved}/${t.intents} intents approved${t.rejected ? `, ${t.rejected} rejected` : ""}`);
  stage(!!t.proofs, s.phase === "needs-proofs", `Proofs       ${t.proofs ? `${t.proofs} derived` : "not derived"}`);
  stage(s.phase === "passed", ["implementing", "failed"].includes(s.phase), `Verification ${s.run ? `${t.passed}/${t.proofs} passed${t.failed ? `, ${t.failed} failed` : ""}${t.regressions ? `, ${t.regressions} regressions` : ""}` : "not run"}`);

  if (s.phase === "review-pending") {
    const open = p.decisions.filter((d) => d.chosen === undefined || d.chosen === null);
    if (open.length) {
      out.push("", "  Decisions to make");
      for (const d of open) out.push(`    ? ${d.id} ${d.question}`, ...d.options.map((o, i) => `        ${i}. ${o.label ?? o}${i === d.recommended ? " (recommended)" : ""}`));
    }
    const required = requiredOf(p);
    for (const { change, intents } of groupByUseCase(p)) {
      out.push("", `  ${change.useCase} ${change.name} (${CHANGE_LABEL[change.change]})`);
      for (const i of intents) {
        const id = short(s, i.id);
        const mark = isApproved(p, i) ? "✓" : i.status === "rejected" ? "✗" : "○";
        const votes = required > 1 && i.status !== "rejected" ? ` (${approvalsOf(p, i)}/${required} approvals)` : "";
        const indent = `    ${" ".repeat(mark.length + id.length + 2)}`;
        out.push(...wrap(`${mark} ${id}${votes} ${i.description}`, "    ", indent));
        for (const r of replacedLines(s, i)) out.push(...wrap(`⚠ ${r}`, indent, `${indent}  `));
      }
    }
  }

  if (t.proofs) {
    out.push("", "  Proofs");
    const results = new Map((s.run?.outcomes ?? []).filter((o) => o.role === "proposal").map((o) => [o.proof, o]));
    for (const x of p.proofs) {
      const r = results.get(x.id);
      const mark = r ? (VERDICT_MARK[r.verdict] ?? "?") : "○";
      const verdict = r ? r.verdict : "pending";
      const id = short(s, x.id);
      out.push(...wrap(`${x.description} [${short(s, x.intent)}]`, `    ${mark} ${id} ${verdict.padEnd(9)} `, `    ${" ".repeat(mark.length + id.length + 12)}`));
      if (r?.counterexample) out.push(`        counterexample ${JSON.stringify(r.counterexample.input)} → ${JSON.stringify(r.counterexample.output)}`);
    }
    for (const i of p.intents.filter((i) => i.provability === "beyond-proof" && i.status !== "rejected"))
      out.push(`    – ${short(s, i.id)} beyond proof (${i.beyondProofReason}): needs a human sign-off`);
  }
  if (s.run) {
    const regressions = s.run.outcomes.filter((o) => o.role === "regression" && !o.passed);
    if (regressions.length) {
      out.push("", "  Regressions (use cases' proofs this change broke)");
      for (const r of regressions) out.push(`    ✗ ${r.proof} ${r.useCase}${r.counterexample ? ` counterexample ${JSON.stringify(r.counterexample.input)} → ${JSON.stringify(r.counterexample.output)}` : ""}`);
    }
    for (const id of s.run.replacedStillPass ?? []) out.push(`    ! ${id} is replaced by this change but still passes: the behavior may not have changed`);
    if (s.run.enginesAgree === false) out.push("", "  ✗ The TypeScript engine and the Rust prover disagree.");
  }
  if (s.phase !== "passed" || p) out.push("", `Next: ${nextStep(s)}`);
  return out.join("\n");
}

// ---------------------------------------------------------------------------
// Paths touched by an edit tool (Claude Code: Edit/Write/MultiEdit/NotebookEdit; Codex: apply_patch)
// ---------------------------------------------------------------------------

export function editedPaths(toolInput) {
  const out = new Set();
  for (const k of ["file_path", "notebook_path", "path"]) if (typeof toolInput?.[k] === "string") out.add(toolInput[k]);
  const visit = (v) => {
    if (typeof v === "string") {
      for (const m of v.matchAll(/^\*\*\* (?:Add|Update|Delete) File: (.+)$/gm)) out.add(m[1].trim());
      for (const m of v.matchAll(/^\*\*\* Move to: (.+)$/gm)) out.add(m[1].trim());
    } else if (v && typeof v === "object") for (const x of Object.values(v)) visit(x);
  };
  visit(toolInput);
  return [...out];
}

export function globToRegExp(glob) {
  let re = "";
  for (let i = 0; i < glob.length; i++) {
    const c = glob[i];
    if (c === "*" && glob[i + 1] === "*") {
      re += glob[i + 2] === "/" ? "(?:.*/)?" : ".*";
      i += glob[i + 2] === "/" ? 2 : 1;
    } else if (c === "*") re += "[^/]*";
    else if (c === "?") re += "[^/]";
    else re += c.replace(/[.+^${}()|[\]\\]/g, "\\$&");
  }
  return new RegExp(`^${re}$`);
}

/** How the gate treats one path: "free" (always allowed), "generated" (CLI only), "config", or "code". */
export function classify(ws, cwd, p) {
  const abs = isAbsolute(p) ? p : resolve(cwd, p);
  const rel = relative(ws.root, abs).split(sep).join("/");
  if (rel.startsWith("..")) return "free"; // outside the repository
  if (rel === PLUGIN_FILE.split(sep).join("/")) return "config";
  const inWs = relative(ws.dir, abs).split(sep).join("/");
  if (!inWs.startsWith("..")) {
    if (isGenerated(inWs)) return "generated";
    return "free"; // proposals, handoff answers, notes
  }
  if (ws.config.allow.some((g) => globToRegExp(g).test(rel))) return "free";
  return "code";
}

/** A `lastro review` command that records the user's approval or answer. */
export function isReviewCommand(cmd) {
  return /\blastro(?:-go)?\b[^\n;&|]*\breview\b[^\n;&|]*\b(?:approve|approve-all|answer|amend|reject|resolve|rewrite|recombine)\b/.test(cmd) || /cli\.js\b[^\n;&|]*\breview\b/.test(cmd);
}

export const isLastroCommand = (cmd) => /\blastro(?:-go)?\b|lastro[/\\].*cli\.js/.test(cmd);
