#!/usr/bin/env node
/**
 * `lastro`: the SDK as a command line, for people and for any agent harness
 * (Claude Code, Codex, a CI job, a shell script). Every command prints one
 * JSON object on stdout and keeps its state as JSON files in `.lastro/`.
 *
 * Exit codes: 0 done/ok · 1 not ok (pending review, contradictions, failing
 * proofs) · 2 usage or error · 3 an LLM answer is needed (see "handoff").
 *
 * The two AI steps (extracting intents, deriving proofs) run on:
 *   --llm anthropic  the Anthropic API (ANTHROPIC_API_KEY, `@anthropic-ai/sdk` installed)
 *   --llm handoff    the calling agent's own model: lastro writes the request
 *                    (instructions, data, JSON Schema) to a file and exits 3;
 *                    the agent writes its JSON answer to the named path and
 *                    runs the same command again.
 * The default is anthropic when ANTHROPIC_API_KEY is set, handoff otherwise.
 */
import { existsSync, readFileSync, realpathSync, unlinkSync, writeFileSync } from "node:fs";
import { basename, dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { loadRustProver, type RustProver } from "./rust.js";
import { sha256Hex } from "./sha256.js";
import type { AskJson } from "./llm/llm.js";
import { deriveProofs, extractProposal, recombineAfter } from "./llm/proposal.js";
import {
  ProposalError,
  applyProposal,
  approveProposal,
  baselineOf,
  carryProposalReview,
  pendingProposal,
  proposalFeedback,
  reservedUseCaseIds,
  proveProposal,
  regress,
  renderProposalReceipt,
  reviewProposal,
  type Proposal,
  type ProposalRun,
  type UseCase,
} from "./proposal.js";
import { LEGACY_FILES, WorkspaceError, openWorkspace, writeJson, type Workspace, type WorkspaceConfig } from "./workspace.js";
import type { Model, Subject } from "./types.js";

class Exit extends Error {
  constructor(
    public readonly code: number,
    public readonly body: Record<string, unknown>,
  ) {
    super(String(body.error ?? body.status ?? code));
  }
}

const USAGE = `lastro <command> [options]

  init --model <model.json> [--subject <module.js>[#export]] [--approvals N] [--submitter-cannot-approve]
                                                start a .lastro/ workspace (or add a model to it)
  propose <prose-file> [--section <heading>] [--as <submitter>] [--proposal <P>]
                                                create a proposal and extract its intents against the use cases;
                                                --proposal re-extracts an existing one and keeps its review
  status [<P>]                                  proposals, or one proposal's intents and pending items
  review <P> --as <name> <action> ...           approve <I..> | approve-all | amend <I> <text> | reject <I> <note>
                                                answer <D> <option> | comment <I|PF|UC|prose> <text> | reply <T> <text>
                                                resolve <T> | rewrite <UC> <after> | recombine <UC> [instructions]
  derive <P>                                    derive the pending proofs of the approved intents
  prove <P> [<module.js>] [--export name] [--stream]
                                                run its proofs and every use case's proofs (regression);
                                                writes the receipt next to the proposal
  feedback <P>                                  the last run's failures, as a brief for whoever writes the code
  apply <P>                                     attach the proofs to their use cases and sync .lastro/usecases/
  regress [<module.js>] [--export name]         run every use case's proofs alone

  <P> is a proposal id (P-001, or 1) or slug; without it, the latest open proposal.
  --dir <path>          workspace (default .lastro)
  --llm anthropic|handoff`;

interface Args {
  _: string[];
  flags: Record<string, string>;
}

/** Flags that never take a value. */
const BOOLEAN_FLAGS = new Set(["stream", "submitter-cannot-approve"]);

function parse(argv: string[]): Args {
  const out: Args = { _: [], flags: {} };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]!;
    if (a.startsWith("--")) {
      const k = a.slice(2);
      const v = argv[i + 1];
      if (v === undefined || v.startsWith("--") || BOOLEAN_FLAGS.has(k)) out.flags[k] = "true";
      else {
        out.flags[k] = v;
        i++;
      }
    } else out._.push(a);
  }
  return out;
}

const readJson = <T>(p: string): T => JSON.parse(readFileSync(p, "utf8")) as T;

/** handoff: a deterministic request file per question; the answer file next to it. */
function handoffAsk(dir: string): AskJson {
  return async (req) => {
    const id = `${req.step}-${sha256Hex(JSON.stringify(req)).slice(0, 12)}`;
    const answer = join(dir, "llm", `${id}.answer.json`);
    if (existsSync(answer)) return readJson<unknown>(answer);
    const request = join(dir, "llm", `${id}.request.json`);
    writeJson(request, {
      instructions: `Act as the system described in "system". Read "user". Write ONLY a JSON value matching "schema" to ${answer}, then run the same lastro command again.`,
      ...req,
    });
    throw new Exit(3, { status: "needs-llm", step: req.step, request, answer });
  };
}

async function anthropicAsk(): Promise<AskJson> {
  try {
    const { claudeAsk } = await import("./llm/claude.js");
    return claudeAsk();
  } catch (e) {
    throw new Exit(2, { error: `--llm anthropic needs @anthropic-ai/sdk installed: ${(e as Error).message}` });
  }
}

async function pickAsk(args: Args, dir: string): Promise<AskJson> {
  const mode = args.flags.llm ?? (process.env.ANTHROPIC_API_KEY ? "anthropic" : "handoff");
  if (mode === "anthropic") return anthropicAsk();
  if (mode === "handoff") return handoffAsk(dir);
  throw new Exit(2, { error: `unknown --llm ${mode}` });
}

async function rustProver(): Promise<RustProver | undefined> {
  const wasm = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "wasm", "lastro_core.wasm");
  if (!existsSync(wasm)) return undefined;
  return loadRustProver(readFileSync(wasm));
}

/** The body of the Markdown section whose heading is `title`, up to the next heading of the same or a higher level. */
export function section(markdown: string, title: string): string {
  const lines = markdown.split("\n");
  const norm = (s: string) => s.trim().toLowerCase();
  const start = lines.findIndex((l) => /^#{1,6}\s/.test(l) && norm(l.replace(/^#+/, "")) === norm(title));
  if (start < 0) throw new Exit(2, { error: `no section "${title}"` });
  const level = lines[start]!.match(/^#+/)![0].length;
  let end = lines.length;
  for (let i = start + 1; i < lines.length; i++) {
    const m = lines[i]!.match(/^(#{1,6})\s/);
    if (m && m[1]!.length <= level) {
      end = i;
      break;
    }
  }
  return lines.slice(start + 1, end).join("\n");
}

/** One proposal as the CLI prints it: what a reviewer needs, nothing more. */
export function summary(p: Proposal, useCases: UseCase[]) {
  const proofs = useCases.flatMap((u) => u.proofs);
  const pending = pendingProposal(p);
  return {
    proposal: { id: p.id, slug: p.slug, name: p.name, status: p.status, submittedBy: p.submittedBy, policy: p.policy, approved: !!p.hash },
    useCases: p.useCases.map((c) => ({ useCase: c.useCase, slug: c.slug, change: c.change, name: c.name, ...(c.stale ? { stale: true } : {}), intents: c.intents })),
    intents: p.intents.map((i) => ({
      id: i.id,
      change: i.change,
      useCase: i.useCase,
      status: i.status,
      approvals: i.approvals,
      name: i.name,
      description: i.description,
      provability: i.provability,
      replaces: i.replaces.map((r) => ({ ...r, description: proofs.find((x) => x.id === r.proof)?.description ?? null })),
    })),
    decisions: p.decisions.map((d) => ({ id: d.id, origin: d.origin, question: d.question, options: d.options.map((o) => o.label), chosen: d.chosen ?? null, recommended: d.recommended })),
    threads: p.threads.filter((t) => !t.resolved).map((t) => ({ id: t.id, on: t.on, by: t.by, comments: t.comments })),
    supersedes: p.supersedes,
    proofs: p.proofs.map((x) => ({ id: x.id, intent: x.intent, useCase: x.useCase, kind: x.check.kind, description: x.description, status: x.status, ...(x.replaces ? { replaces: x.replaces } : {}) })),
    pending,
    ready: pending.length === 0,
  };
}

function runBody(run: ProposalRun) {
  return {
    ok: run.ok,
    enginesAgree: run.enginesAgree ?? null,
    failures: run.outcomes.filter((x) => x.role !== "superseded" && !x.passed),
    regressions: run.regressions,
    replacedStillPass: run.replacedStillPass,
    unproved: run.unproved,
  };
}

/**
 * `prove --stream`: one compact JSON object per line, in this order, the same in every Lastro CLI:
 *   {"event":"start","proposal":..,"proofs":[ids]}
 *   {"event":"progress","runs":n}                       every 100 runs of the code under proof
 *   {"event":"proof","proof":..,"role":..,"verdict":..,"passed":bool[,"counterexample":{input,output}]}
 *   {"event":"crosscheck","enginesAgree":bool}          when the Rust prover ran
 *   {"event":"result", ...the body prove prints without --stream}
 */
export const STREAM_PROGRESS_EVERY = 100;

export type Emit = (event: Record<string, unknown>) => void;

/** The code under proof, by subject name: the config's subjects, or one module given on the command line. */
async function loadSubjects(ws: Workspace, cfg: WorkspaceConfig, args: Args, module: string | undefined): Promise<Record<string, Subject>> {
  const load = async (path: string, name: string, label: string): Promise<Subject> => {
    const mod = (await import(pathToFileURL(path).href)) as Record<string, unknown>;
    const fn = mod[name] as Subject | undefined;
    if (typeof fn !== "function") throw new Exit(2, { error: `${label} has no function export "${name}"` });
    return fn;
  };
  const names = Object.keys(cfg.subjects);
  if (module) {
    if (names.length > 1) throw new Exit(2, { error: `the workspace declares ${names.length} subjects (${names.join(", ")}); a module on the command line needs exactly one` });
    return { [names[0] ?? Object.keys(cfg.models)[0] ?? "default"]: await load(resolve(module), args.flags.export ?? "default", module) };
  }
  const out: Record<string, Subject> = {};
  for (const [n, ref] of Object.entries(cfg.subjects)) out[n] = await load(resolve(ws.root, ref.module), ref.export ?? "default", ref.module);
  if (!names.length) throw new Exit(2, { error: "no subject: pass the module under proof, or declare subjects in .lastro/config.json (lastro init --subject)" });
  return out;
}

function counting(subjects: Record<string, Subject>, stream: Emit | undefined): Record<string, Subject> {
  if (!stream) return subjects;
  let runs = 0;
  return Object.fromEntries(
    Object.entries(subjects).map(([k, fn]) => [
      k,
      (input: Parameters<Subject>[0]) => {
        if (++runs % STREAM_PROGRESS_EVERY === 0) stream({ event: "progress", runs });
        return fn(input);
      },
    ]),
  );
}

function writeReceipt(ws: Workspace, p: Proposal, useCases: UseCase[]): string {
  const path = ws.proposalPath(p, ".receipt.md");
  writeFileSync(path, renderProposalReceipt(p, useCases) + "\n");
  return path;
}

export async function main(argv: string[], emit?: Emit): Promise<{ code: number; body: Record<string, unknown> }> {
  const args = parse(argv);
  const [cmd, ...rest] = args._;
  const ws = openWorkspace(resolve(args.flags.dir ?? ".lastro"));

  try {
    if (cmd && cmd !== "init" && ws.legacy().length)
      throw new Exit(2, {
        error: `${ws.dir} holds ${ws.legacy().join(", ")} from the old workspace format, which this version does not read. Delete them and run \`lastro propose <prose-file>\` again.`,
        legacy: ws.legacy(),
      });
    if (["plan", "spec"].includes(cmd ?? "")) throw new Exit(2, { error: `\`lastro ${cmd}\` was replaced: use \`lastro propose\`, \`lastro review\` and \`lastro derive\``, usage: USAGE });

    switch (cmd) {
      case "init": {
        if (!args.flags.model) throw new Exit(2, { error: "init needs --model <model.json>" });
        const model = readJson<Model>(resolve(args.flags.model));
        const cfg: WorkspaceConfig = ws.exists() ? ws.config() : { models: {}, subjects: {}, review: { approvals: 1, submitterCanApprove: true } };
        cfg.models[model.name] = model;
        if (args.flags.subject) {
          const [module, exp] = args.flags.subject.split("#");
          cfg.subjects[args.flags["subject-name"] ?? model.name] = { module: module!, ...(exp ? { export: exp } : {}) };
        }
        if (args.flags.approvals) cfg.review.approvals = Math.max(1, Number(args.flags.approvals));
        if (args.flags["submitter-cannot-approve"]) cfg.review.submitterCanApprove = false;
        ws.writeConfig(cfg);
        return { code: 0, body: { status: "initialized", dir: ws.dir, models: Object.keys(cfg.models), subjects: Object.keys(cfg.subjects), review: cfg.review } };
      }
      case "propose": {
        const cfg = ws.config();
        if (!rest[0]) throw new Exit(2, { error: "propose needs a prose file" });
        const text = readFileSync(resolve(rest[0]), "utf8");
        const prose = (args.flags.section ? section(text, args.flags.section) : text).trim();
        const useCases = ws.useCases();
        // A prose file already in .lastro/proposals/ named <id>-<slug>.md names its proposal: re-extract it if it exists.
        const proseFile = resolve(rest[0]);
        const inProposals = dirname(proseFile) === ws.file("proposals");
        const named = inProposals ? basename(proseFile).match(/^(P-\d+)-([a-z0-9]+(?:-[a-z0-9]+)*)\.md$/) : null;
        const existing = named ? ws.proposals().find((p) => p.id === named[1]) : undefined;
        const previous = args.flags.proposal ? ws.proposal(args.flags.proposal) : existing;
        const id = previous?.id ?? named?.[1] ?? ws.nextProposalId();
        const submittedBy = args.flags.as ?? previous?.submittedBy ?? process.env.LASTRO_USER ?? "submitter";
        let p = await extractProposal(await pickAsk(args, ws.dir), { id, prose, submittedBy, useCases, policy: cfg.review, reserved: reservedUseCaseIds(ws.proposals(), id) });
        if (previous) p = carryProposalReview(previous, p);
        const slug = previous?.slug ?? named?.[2];
        if (slug) p.slug = slug; // slugs are stable
        ws.writeProposal(p);
        const md = ws.proposalPath(p, ".md");
        if (md !== proseFile) {
          writeFileSync(md, prose + "\n");
          if (inProposals && !named) unlinkSync(proseFile); // a draft in .lastro/proposals/ becomes <id>-<slug>.md
        }
        const s = summary(p, useCases);
        return { code: s.ready ? 0 : 1, body: { status: "proposed", file: ws.proposalPath(p), ...s } };
      }
      case "status": {
        const useCases = ws.useCases();
        if (!rest[0] && !args.flags.proposal) {
          const all = ws.proposals().map((p) => ({ id: p.id, slug: p.slug, name: p.name, status: p.status, pending: pendingProposal(p).length }));
          const open = all.filter((p) => p.status !== "applied" && p.status !== "closed");
          return { code: open.length ? 1 : 0, body: { status: "status", useCases: useCases.length, proofs: useCases.reduce((n, u) => n + u.proofs.length, 0), unproved: useCases.filter((u) => !u.proofs.length).map((u) => u.id), proposals: all } };
        }
        const p = ws.proposal(rest[0] ?? args.flags.proposal);
        const s = summary(p, useCases);
        return { code: s.ready ? 0 : 1, body: { status: "status", ...s } };
      }
      case "review": {
        const who = args.flags.as;
        if (!who) throw new Exit(2, { error: "review needs --as <name>" });
        const [ref, action, ...xs] = rest;
        if (!ref || !action) throw new Exit(2, { error: "review needs a proposal and an action", usage: USAGE });
        const useCases = ws.useCases();
        const before = ws.proposal(ref);
        const r = reviewProposal(before, who);
        if (action === "approve") r.approve(...xs);
        else if (action === "approve-all") r.approveAll();
        else if (action === "amend") r.amend(xs[0]!, xs.slice(1).join(" "));
        else if (action === "reject") r.reject(xs[0]!, xs.slice(1).join(" ") || "rejected");
        else if (action === "answer") r.answer(xs[0]!, Number(xs[1]));
        else if (action === "comment") r.comment(xs[0]!, xs.slice(1).join(" "));
        else if (action === "reply") r.reply(xs[0]!, xs.slice(1).join(" "));
        else if (action === "resolve") r.resolve(xs[0]!);
        else if (action === "rewrite") r.rewriteAfter(xs[0]!, xs.slice(1).join(" "));
        else if (action === "recombine") {
          const out = await recombineAfter(await pickAsk(args, ws.dir), r.done(), xs[0]!, xs.slice(1).join(" ") || undefined);
          r.rewriteAfter(xs[0]!, out.after, out.name);
        } else throw new Exit(2, { error: `unknown review action ${action}`, usage: USAGE });
        let p = r.done();
        // Fully reviewed: approve it, bound to the current use cases.
        if (!pendingProposal(p).length && !p.hash) {
          if (baselineOf(useCases) !== p.baseline) {
            ws.writeProposal(p);
            throw new Exit(1, { status: "stale", error: `the use cases changed since ${p.id} was extracted: run \`lastro propose <prose-file> --proposal ${p.id}\` again` });
          }
          p = approveProposal(p, useCases);
        }
        ws.writeProposal(p);
        return { code: 0, body: { status: "reviewed", reviewer: who, ...summary(p, useCases) } };
      }
      case "derive": {
        const cfg = ws.config();
        const useCases = ws.useCases();
        const p = ws.proposal(rest[0]);
        if (!p.hash) throw new Exit(1, { status: "review-pending", pending: pendingProposal(p) });
        const next = await deriveProofs(await pickAsk(args, ws.dir), p, useCases, { models: cfg.models, subjects: Object.keys(cfg.subjects).length ? Object.keys(cfg.subjects) : Object.keys(cfg.models) });
        ws.writeProposal(next);
        if (!next.hash)
          return { code: 1, body: { status: "back-to-review", note: `Derivation raised decisions. Answer them with \`lastro review ${next.id} --as <name> answer <D> <option>\`, review the intents they reopened, and run \`lastro derive ${next.id}\` again.`, ...summary(next, useCases) } };
        return { code: 0, body: { status: "derived", ...summary(next, useCases) } };
      }
      case "prove": {
        const cfg = ws.config();
        const useCases = ws.useCases();
        const looksLikeModule = (x: string | undefined) => !!x && /\.(m?js|cjs|ts)$/.test(x);
        const ref = looksLikeModule(rest[0]) ? undefined : rest[0];
        const module = looksLikeModule(rest[0]) ? rest[0] : rest[1];
        const p = ws.proposal(ref);
        const stream = args.flags.stream === "true" && emit ? emit : undefined;
        const subjects = counting(await loadSubjects(ws, cfg, args, module), stream);
        stream?.({ event: "start", proposal: p.id, proofs: p.proofs.map((x) => x.id) });
        const { proposal, run } = await proveProposal(p, useCases, { subjects, models: cfg.models, rust: await rustProver() });
        if (stream) {
          for (const o of run.outcomes) stream({ event: "proof", proof: o.proof, role: o.role, verdict: o.verdict, passed: o.passed, ...(o.counterexample ? { counterexample: o.counterexample } : {}) });
          if (run.enginesAgree !== undefined) stream({ event: "crosscheck", enginesAgree: run.enginesAgree });
        }
        ws.writeProposal(proposal);
        ws.appendRun(proposal, run);
        const receipt = writeReceipt(ws, proposal, useCases);
        return { code: run.ok ? 0 : 1, body: { status: run.ok ? "proved" : "not-proved", proposal: proposal.id, ...runBody(run), receipt, feedback: proposalFeedback(proposal, useCases) } };
      }
      case "feedback": {
        const p = ws.proposal(rest[0]);
        return { code: p.lastRun?.ok ? 0 : 1, body: { status: "feedback", proposal: p.id, feedback: proposalFeedback(p, ws.useCases()) } };
      }
      case "apply": {
        const useCases = ws.useCases();
        const p = ws.proposal(rest[0]);
        const applied = applyProposal(p, useCases);
        for (const id of applied.removed) {
          const u = useCases.find((x) => x.id === id);
          if (u) ws.removeUseCase(u);
        }
        for (const id of applied.written) ws.writeUseCase(applied.useCases.find((u) => u.id === id)!);
        ws.writeProposal(applied.proposal);
        const receipt = writeReceipt(ws, applied.proposal, useCases);
        return { code: 0, body: { status: "applied", proposal: p.id, written: applied.written, removed: applied.removed, baseline: applied.baseline, unproved: p.lastRun?.unproved ?? [], receipt } };
      }
      case "regress": {
        const cfg = ws.config();
        const useCases = ws.useCases();
        const run = await regress(useCases, { subjects: await loadSubjects(ws, cfg, args, rest[0]), models: cfg.models, rust: await rustProver() });
        return { code: run.ok ? 0 : 1, body: { status: run.ok ? "no-regressions" : "regressions", ...runBody(run) } };
      }
      default:
        throw new Exit(2, { error: cmd ? `unknown command ${cmd}` : "no command", usage: USAGE });
    }
  } catch (e) {
    if (e instanceof Exit) return { code: e.code, body: e.body };
    if (e instanceof ProposalError) return { code: 1, body: { status: "refused", problems: e.problems } };
    if (e instanceof WorkspaceError) return { code: 2, body: { error: e.message } };
    return { code: 2, body: { error: (e as Error).message } };
  }
}

export { LEGACY_FILES };

const invoked = (() => {
  try {
    return !!process.argv[1] && realpathSync(process.argv[1]) === realpathSync(fileURLToPath(import.meta.url));
  } catch {
    return false;
  }
})();
if (invoked) {
  const argv = process.argv.slice(2);
  const streaming = argv.includes("--stream");
  main(argv, (event) => process.stdout.write(JSON.stringify(event) + "\n")).then(({ code, body }) => {
    process.stdout.write((streaming ? JSON.stringify({ event: "result", ...body }) : JSON.stringify(body, null, 2)) + "\n");
    process.exitCode = code;
  });
}
