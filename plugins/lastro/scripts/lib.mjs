// The Lastro plugin's logic, shared by Claude Code and Codex. It never decides anything itself: every intent,
// decision, spec and proof comes from the `lastro` CLI and the files it keeps in `.lastro/`. The plugin only
// reads them to (1) keep code edits behind an approved spec, (2) run the proofs when the agent stops and hand the
// failures back, and (3) render the progress board people see.
import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, isAbsolute, join, relative, resolve, sep } from "node:path";

export const PLUGIN_FILE = join(".lastro", "plugin.json");

/** Files only the CLI writes. The agent never edits them by hand. */
export const GENERATED = ["config.json", "intents.json", "spec.json", "proof.json", "receipt.md"];

export const DEFAULTS = {
  /** The command that runs the Lastro CLI: `lastro`, `npx lastro`, `lastro-go`, `node path/to/cli.js`... */
  cli: "lastro",
  /** The workspace folder, relative to the repository root. */
  dir: ".lastro",
  /** Shell command that builds the code under proof before proving (optional). */
  build: null,
  /** The arguments after `lastro prove`, e.g. "dist/fee.js --export shippingFee" (required to prove). */
  prove: null,
  /** "on": code edits wait for approved intents and a current spec; "off": never block. */
  gate: "on",
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
  const config = { ...DEFAULTS, ...(readJson(join(root, PLUGIN_FILE)) ?? {}) };
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
  return readJson(ws.statePath) ?? { attempts: 0, dirty: false, specKey: null };
}

export function writeState(ws, state) {
  mkdirSync(dirname(ws.statePath), { recursive: true });
  writeFileSync(ws.statePath, JSON.stringify(state, null, 2) + "\n");
}

/** What the reviewed intents look like; a spec is current only if it was derived from exactly this review. */
function reviewKey(a) {
  if (!a) return null;
  return JSON.stringify({
    intents: (a.intents ?? []).map((i) => [i.id, i.statement, i.status]),
    decisions: (a.decisions ?? []).map((d) => [d.id, d.chosen ?? null]),
  });
}

/**
 * Where the current proposal stands. Phases, in order:
 *   no-workspace · no-proposal · review-pending · needs-spec · implementing · failed · passed
 */
export function snapshot(ws) {
  const config = existsSync(ws.file("config.json"));
  if (!config) return { phase: "no-workspace", ws };
  const intents = readJson(ws.file("intents.json"));
  if (!intents) return { phase: "no-proposal", ws };

  const status = runCli(ws, "status");
  const pending = Array.isArray(status.body.pending) ? status.body.pending : [];
  const spec = readJson(ws.file("spec.json"));
  const specCurrent = !!spec && reviewKey(spec.artifact) === reviewKey(intents);
  const proof = readJson(ws.file("proof.json"));
  const proofCurrent = specCurrent && !!proof && proof.report?.artifactHash === spec.artifact.hash;
  const state = readState(ws);

  let phase;
  if (pending.length) phase = "review-pending";
  else if (!specCurrent) phase = "needs-spec";
  else if (!proofCurrent || state.dirty) phase = "implementing";
  else phase = proof.ok ? "passed" : "failed";

  return { phase, ws, intents, pending, spec: specCurrent ? spec : null, proof: proofCurrent ? proof : null, stale: proofCurrent && state.dirty, state, cliError: status.body.error };
}

/** The next thing to do, as an instruction for the agent. */
export function nextStep(s) {
  const cli = s.ws?.config.cli ?? "lastro";
  switch (s.phase) {
    case "no-workspace":
      return `Set up the workspace first: draft the model of the decision core with the user, then run \`${cli} init --model <model.json>\`.`;
    case "no-proposal":
      return `Write the proposed change as prose in .lastro/proposals/<slug>.md and run \`${cli} plan .lastro/proposals/<slug>.md\`.`;
    case "review-pending":
      return `The user reviews the proposal like a pull request: answer each open decision, then approve, amend or reject each intent (\`${cli} review --as <user> ...\`, recording only the user's own answers).`;
    case "needs-spec":
      return `Every intent is approved. Derive the proofs with \`${cli} spec\`.`;
    case "implementing":
      return "Implement the approved intents, then end your turn: the proofs run when you stop.";
    case "failed":
      return "Fix the code until every proof passes.";
    case "passed":
      return "Every proof passes.";
  }
}

// ---------------------------------------------------------------------------
// The progress board
// ---------------------------------------------------------------------------

const CLAUSE_MARK = { proved: "✓", "proved-under-assumptions": "✓", evidenced: "≈", violated: "✗", vacuous: "!", weak: "!" };
const INTENT_MARK = { proved: "✓", "proved-under-assumptions": "✓", evidenced: "≈", violated: "✗", "beyond-proof": "–", gap: "!" };

/** Counts for the one-line view and for tests. */
/** Approvals the policy requires per intent (`lastro init --approvals N`). */
const requiredOf = (s) => Number(readJson(s.ws.file("config.json"))?.requiredApprovals ?? 1);

/** An intent counts as approved once it has as many approvals as the policy requires, like a pull request. */
const isApproved = (i, required) => i.status === "approved" && (i.approvals ?? []).length >= required;

export function tally(s) {
  const intents = s.intents?.intents ?? [];
  const required = s.ws ? requiredOf(s) : 1;
  const decisions = s.intents?.decisions ?? [];
  const clauses = s.spec?.plan.clauses ?? [];
  const results = s.proof?.report.clauses ?? [];
  const passed = results.filter((r) => ["proved", "proved-under-assumptions", "evidenced"].includes(r.verdict)).length;
  return {
    intents: intents.length,
    approved: intents.filter((i) => isApproved(i, required)).length,
    decisions: decisions.length,
    answered: decisions.filter((d) => d.chosen !== undefined && d.chosen !== null).length,
    clauses: clauses.length,
    passed: s.proof && !s.stale ? passed : 0,
    failed: s.proof && !s.stale ? results.length - passed : 0,
  };
}

const PHASE_LABEL = {
  "no-workspace": "not set up",
  "no-proposal": "no proposal yet",
  "review-pending": "waiting for the user's review",
  "needs-spec": "approved, proofs not derived yet",
  implementing: "proofs ready, implementation in progress",
  failed: "proofs FAILED",
  passed: "all proofs PASSED",
};

/** One line, for a status line or a short system message. */
export function renderLine(s) {
  const t = tally(s);
  const head = "Lastro";
  switch (s.phase) {
    case "no-workspace":
    case "no-proposal":
      return `${head} · ${PHASE_LABEL[s.phase]}`;
    case "review-pending":
      return `${head} · review ${t.approved}/${t.intents} intents approved · ${t.answered}/${t.decisions} decisions answered`;
    case "needs-spec":
      return `${head} · ${t.approved}/${t.intents} intents approved · proofs not derived yet`;
    case "implementing":
      return `${head} · ${t.clauses} proofs pending · ○ ${t.clauses}`;
    default:
      return `${head} · proofs ${t.passed}/${t.clauses} passed${t.failed ? ` · ✗ ${t.failed} failed` : ""} · ${s.phase === "passed" ? "PASSED" : "FAILED"}`;
  }
}

/** The full board: stages, intents, decisions and one line per proof. */
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

const KIND_LABEL = { behavior: "Behavior", invariant: "Never", preserve: "Keeps working", "non-functional": "Non-functional" };

/** Intents grouped under readable headings, in the order their kinds first appear. */
export function groupByKind(intents) {
  const groups = new Map();
  for (const i of intents) {
    const label = KIND_LABEL[i.kind] ?? (i.kind ? String(i.kind) : "Intents");
    if (!groups.has(label)) groups.set(label, []);
    groups.get(label).push(i);
  }
  return groups;
}

/**
 * The review as Markdown, for the agent to show the user as-is: one list item per decision and per intent, grouped,
 * each with the quote it came from. Every item starts with a list marker, so chat apps that join single newlines
 * still show one per line.
 */
export function renderReview(s) {
  if (!s.intents) return `Lastro: no proposal to review. ${nextStep(s)}`;
  const cfg = readJson(s.ws.file("config.json")) ?? {};
  const required = Number(cfg.requiredApprovals ?? 1);
  const intents = s.intents.intents ?? [];
  const decisions = s.intents.decisions ?? [];
  const open = decisions.filter((d) => d.chosen === undefined || d.chosen === null);
  const approved = intents.filter((i) => isApproved(i, required)).length;
  const out = [`**Proposal for review:** ${intents.length} intents, ${approved} approved · ${open.length} of ${decisions.length} decisions open`];
  if (open.length) {
    out.push("", "**Decisions**", "");
    for (const d of open) {
      out.push(`- **${d.id}** ${d.question}`);
      d.options.forEach((o, n) => out.push(`  ${n}. ${o.label ?? o}${o.consequence ? `: ${o.consequence}` : ""}${n === d.recommended ? " _(recommended)_" : ""}`));
    }
  }
  const status = (i) =>
    isApproved(i, required) ? "approved" : i.status === "rejected" ? "rejected" : required > 1 ? `${(i.approvals ?? []).length}/${required} approvals` : "to review";
  for (const [kind, group] of groupByKind(intents)) {
    out.push("", `**${kind}**`, "");
    for (const i of group) {
      out.push(`- **${i.id}** ${i.statement} _(${status(i)})_`);
      if (i.source?.quote) out.push(`  > ${i.source.quote.replace(/\s+/g, " ").trim()}`);
    }
  }
  return out.join("\n");
}

export function renderBoard(s) {
  if (s.phase === "no-workspace") return `Lastro · ${PHASE_LABEL[s.phase]}\n${nextStep(s)}`;
  const t = tally(s);
  const cfg = readJson(s.ws.file("config.json")) ?? {};
  const model = cfg.model?.name ?? "";
  const required = Number(cfg.requiredApprovals ?? 1);
  const out = [`Lastro${model ? ` · ${model}` : ""} · ${PHASE_LABEL[s.phase]}`, ""];
  const reviewDone = s.phase !== "no-proposal" && s.phase !== "review-pending";
  const stage = (done, current, label) => out.push(`  ${done ? "✓" : current ? "▸" : "○"} ${label}`);
  stage(s.phase !== "no-proposal", s.phase === "no-proposal", `Intents      ${t.intents ? `${t.intents} extracted` : "none yet"}`);
  stage(t.decisions === t.answered && s.phase !== "no-proposal", s.phase === "review-pending", `Decisions    ${t.answered}/${t.decisions} answered`);
  stage(reviewDone, s.phase === "review-pending", `Approval     ${t.approved}/${t.intents} intents approved`);
  stage(!!s.spec, s.phase === "needs-spec", `Proofs       ${s.spec ? `${t.clauses} derived` : "not derived"}`);
  const verified = s.phase === "passed";
  stage(verified, ["implementing", "failed"].includes(s.phase), `Verification ${s.proof && !s.stale ? `${t.passed}/${t.clauses} passed${t.failed ? `, ${t.failed} failed` : ""}` : "not run"}`);

  if (s.phase === "review-pending") {
    const open = (s.intents.decisions ?? []).filter((d) => d.chosen === undefined || d.chosen === null);
    if (open.length) {
      out.push("", "  Decisions to make");
      for (const d of open) out.push(`    ? ${d.id} ${d.question}`, ...d.options.map((o, i) => `        ${i}. ${o.label ?? o}${i === d.recommended ? " (recommended)" : ""}`));
    }
    for (const [kind, intents] of groupByKind(s.intents.intents)) {
      out.push("", `  ${kind}`);
      for (const i of intents) {
        const mark = isApproved(i, required) ? "✓" : i.status === "rejected" ? "✗" : "○";
        const votes = required > 1 && i.status !== "rejected" ? ` (${(i.approvals ?? []).length}/${required} approvals)` : "";
        out.push(...wrap(`${mark} ${i.id}${votes} ${i.statement}`, "    ", `    ${" ".repeat(mark.length + i.id.length + 2)}`));
      }
    }
  }

  if (s.spec) {
    out.push("", "  Proofs");
    const results = new Map((s.proof && !s.stale ? s.proof.report.clauses : []).map((r) => [r.clauseId, r]));
    for (const c of s.spec.plan.clauses) {
      const r = results.get(c.id);
      const mark = r ? (CLAUSE_MARK[r.verdict] ?? "?") : "○";
      const verdict = r ? r.verdict : "pending";
      out.push(...wrap(`${c.description} [${c.intentIds.join(", ")}]`, `    ${mark} ${c.id} ${verdict.padEnd(9)} `, `    ${" ".repeat(mark.length + c.id.length + 12)}`));
      if (r?.counterexample) out.push(`        counterexample ${JSON.stringify(r.counterexample.input)} → ${JSON.stringify(r.counterexample.output)}`);
    }
    for (const b of s.spec.plan.beyondProof ?? []) out.push(`    – ${b.intentId} beyond proof (${b.reason}): needs a human sign-off`);
  }
  if (s.proof && !s.stale && s.proof.enginesAgree === false) out.push("", "  ✗ The TypeScript engine and the Rust prover disagree.");
  if (s.phase !== "passed") out.push("", `Next: ${nextStep(s)}`);
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
    if (GENERATED.includes(inWs) || inWs.startsWith("plugin/")) return "generated";
    return "free"; // proposals, handoff answers, notes
  }
  if (ws.config.allow.some((g) => globToRegExp(g).test(rel))) return "free";
  return "code";
}

/** A `lastro review` command that records the user's approval or answer. */
export function isReviewCommand(cmd) {
  return /\blastro(?:-go)?\b[^\n;&|]*\breview\b[^\n;&|]*\b(?:approve|approve-all|answer|amend|reject)\b/.test(cmd) || /cli\.js\b[^\n;&|]*\breview\b/.test(cmd);
}

export const isLastroCommand = (cmd) => /\blastro(?:-go)?\b|lastro[/\\].*cli\.js/.test(cmd);
