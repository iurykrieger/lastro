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
import { existsSync, mkdirSync, readFileSync, realpathSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { createLastro, agentFeedback, type Spec } from "./lastro.js";
import { loadRustProver, type RustProver } from "./rust.js";
import { sha256Hex } from "./sha256.js";
import type { AskJson } from "./llm/llm.js";
import type { IntentArtifact, Model, Subject, Trace } from "./types.js";

class Exit extends Error {
  constructor(
    public readonly code: number,
    public readonly body: Record<string, unknown>,
  ) {
    super(String(body.error ?? body.status ?? code));
  }
}

const USAGE = `lastro <command> [options]

  init --model <model.json> [--approvals N]     start a .lastro/ workspace
  plan <prose-file>                             extract intents (keeps earlier review)
  status                                        intents, approvals, pending items
  review --as <name> <action> ...               approve <I1 I2..> | approve-all | amend <I> <text>
                                                reject <I> <note> | answer <D> <option-index>
  spec                                          derive the formal spec; contradictions come back as decisions
  prove <module.js> [--export name] [--traces file.json]
                                                check the code against the spec; writes proof.json, receipt.md
  feedback                                      the last proof's failures, as a brief for whoever writes the code

  --dir <path>          workspace (default .lastro)
  --llm anthropic|handoff`;

interface Args {
  _: string[];
  flags: Record<string, string>;
}

function parse(argv: string[]): Args {
  const out: Args = { _: [], flags: {} };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]!;
    if (a.startsWith("--")) {
      const k = a.slice(2);
      const v = argv[i + 1];
      if (v === undefined || v.startsWith("--")) out.flags[k] = "true";
      else {
        out.flags[k] = v;
        i++;
      }
    } else out._.push(a);
  }
  return out;
}

const readJson = <T>(p: string): T => JSON.parse(readFileSync(p, "utf8")) as T;
const writeJson = (p: string, v: unknown) => {
  mkdirSync(dirname(p), { recursive: true });
  writeFileSync(p, JSON.stringify(v, null, 2) + "\n");
};

interface Config {
  model: Model;
  requiredApprovals: number;
}

function workspace(args: Args) {
  const dir = resolve(args.flags.dir ?? ".lastro");
  const file = (name: string) => join(dir, name);
  const config = (): Config => {
    if (!existsSync(file("config.json"))) throw new Exit(2, { error: `no workspace at ${dir}: run \`lastro init --model model.json\`` });
    return readJson<Config>(file("config.json"));
  };
  return { dir, file, config };
}

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

function summary(a: IntentArtifact, pending: string[]) {
  return {
    intents: a.intents.map((i) => ({ id: i.id, status: i.status, approvals: i.approvals ?? [], statement: i.statement, ...(i.decisionId ? { decisionId: i.decisionId } : {}) })),
    decisions: a.decisions.map((d) => ({ id: d.id, origin: d.origin, question: d.question, options: d.options.map((o) => o.label), chosen: d.chosen ?? null, recommended: d.recommended })),
    pending,
    ready: pending.length === 0,
  };
}

export async function main(argv: string[]): Promise<{ code: number; body: Record<string, unknown> }> {
  const args = parse(argv);
  const [cmd, ...rest] = args._;
  const ws = workspace(args);
  const intentsPath = ws.file("intents.json");

  try {
    switch (cmd) {
      case "init": {
        if (!args.flags.model) throw new Exit(2, { error: "init needs --model <model.json>" });
        const model = readJson<Model>(resolve(args.flags.model));
        const requiredApprovals = Number(args.flags.approvals ?? 1);
        writeJson(ws.file("config.json"), { model, requiredApprovals });
        return { code: 0, body: { status: "initialized", dir: ws.dir, model: model.name, requiredApprovals } };
      }
      case "plan": {
        const cfg = ws.config();
        if (!rest[0]) throw new Exit(2, { error: "plan needs a prose file" });
        const prose = readFileSync(resolve(rest[0]), "utf8").trim();
        const lastro = createLastro({ model: cfg.model, requiredApprovals: cfg.requiredApprovals, ask: await pickAsk(args, ws.dir) });
        const previous = existsSync(intentsPath) ? readJson<IntentArtifact>(intentsPath) : undefined;
        const a = await lastro.plan(prose, previous);
        writeJson(intentsPath, a);
        const pending = lastro.pending(a);
        return { code: pending.length ? 1 : 0, body: { status: "planned", ...summary(a, pending) } };
      }
      case "status": {
        const cfg = ws.config();
        const a = readJson<IntentArtifact>(intentsPath);
        const lastro = createLastro({ model: cfg.model, requiredApprovals: cfg.requiredApprovals });
        const pending = lastro.pending(a);
        return { code: pending.length ? 1 : 0, body: { status: "status", ...summary(a, pending) } };
      }
      case "review": {
        const cfg = ws.config();
        const who = args.flags.as;
        if (!who) throw new Exit(2, { error: "review needs --as <name>" });
        const [action, ...xs] = rest;
        const lastro = createLastro({ model: cfg.model, requiredApprovals: cfg.requiredApprovals });
        const r = lastro.review(readJson<IntentArtifact>(intentsPath), who);
        if (action === "approve") r.approve(...xs);
        else if (action === "approve-all") r.approveAll();
        else if (action === "amend") r.amend(xs[0]!, xs.slice(1).join(" "));
        else if (action === "reject") r.reject(xs[0]!, xs.slice(1).join(" ") || "rejected");
        else if (action === "answer") r.answer(xs[0]!, Number(xs[1]));
        else throw new Exit(2, { error: `unknown review action ${action ?? ""}` });
        const a = r.done();
        writeJson(intentsPath, a);
        const pending = lastro.pending(a);
        return { code: 0, body: { status: "reviewed", reviewer: who, ...summary(a, pending) } };
      }
      case "spec": {
        const cfg = ws.config();
        const lastro = createLastro({ model: cfg.model, requiredApprovals: cfg.requiredApprovals, ask: await pickAsk(args, ws.dir) });
        const a = readJson<IntentArtifact>(intentsPath);
        const pending = lastro.pending(a);
        if (pending.length) throw new Exit(1, { status: "review-pending", pending });
        const spec = await lastro.spec(a);
        if (!spec.ok) {
          const back = lastro.returnDecisions(spec);
          writeJson(intentsPath, back);
          return { code: 1, body: { status: "contradictions", note: "The spec contradicts itself. Answer the new decisions with `lastro review --as <name> answer <D> <option>`, approve the intents they create, and run `lastro spec` again.", ...summary(back, lastro.pending(back)) } };
        }
        writeJson(ws.file("spec.json"), spec);
        return { code: 0, body: { status: "spec", artifactHash: spec.artifact.hash, clauses: spec.plan.clauses.map((c) => ({ id: c.id, intentIds: c.intentIds, description: c.description })), beyondProof: spec.plan.beyondProof } };
      }
      case "prove": {
        const cfg = ws.config();
        if (!rest[0]) throw new Exit(2, { error: "prove needs the module that exports the function under proof" });
        if (!existsSync(ws.file("spec.json"))) throw new Exit(2, { error: "no spec yet: run `lastro spec`" });
        const spec = readJson<Spec>(ws.file("spec.json"));
        const mod = (await import(pathToFileURL(resolve(rest[0])).href)) as Record<string, unknown>;
        const name = args.flags.export ?? "default";
        const subject = mod[name] as Subject | undefined;
        if (typeof subject !== "function") throw new Exit(2, { error: `${rest[0]} has no function export "${name}"` });
        const traces = args.flags.traces ? readJson<Trace[]>(resolve(args.flags.traces)) : [];
        const lastro = createLastro({ model: cfg.model, requiredApprovals: cfg.requiredApprovals, rust: await rustProver() });
        const proof = await lastro.prove(spec, subject, { traces });
        writeJson(ws.file("proof.json"), proof);
        writeFileSync(ws.file("receipt.md"), proof.receipt + "\n");
        return {
          code: proof.ok ? 0 : 1,
          body: { status: proof.ok ? "proved" : "not-proved", ok: proof.ok, enginesAgree: proof.enginesAgree ?? null, failures: proof.failures, specGaps: proof.specGaps, receipt: ws.file("receipt.md"), feedback: agentFeedback(proof) },
        };
      }
      case "feedback": {
        const proof = readJson<{ failures: []; specGaps: string[] }>(ws.file("proof.json"));
        return { code: proof.failures.length ? 1 : 0, body: { status: "feedback", feedback: agentFeedback(proof as never) } };
      }
      default:
        throw new Exit(2, { error: cmd ? `unknown command ${cmd}` : "no command", usage: USAGE });
    }
  } catch (e) {
    if (e instanceof Exit) return { code: e.code, body: e.body };
    return { code: 2, body: { error: (e as Error).message } };
  }
}

const invoked = (() => {
  try {
    return !!process.argv[1] && realpathSync(process.argv[1]) === realpathSync(fileURLToPath(import.meta.url));
  } catch {
    return false;
  }
})();
if (invoked) {
  main(process.argv.slice(2)).then(({ code, body }) => {
    process.stdout.write(JSON.stringify(body, null, 2) + "\n");
    process.exitCode = code;
  });
}
