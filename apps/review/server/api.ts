/**
 * The review app's API, served by Vite (dev and preview) from the `.lastro/` workspace on disk.
 *
 *   GET  /api/workspace                  use cases, proposals, the default reviewer and the LLM mode
 *   POST /api/proposals/:id/review       { as, action } → the proposal after the action
 *   GET  /api/events                     server-sent "change" events when a file in .lastro/ changes
 *
 * Every write goes through the TypeScript SDK (ProposalReview, approveProposal, recombineAfter): the app
 * holds no review logic of its own. Collaboration is file-backed: reviewers on one machine share this
 * server (actions are applied one at a time to the latest file, and every open page reloads on change);
 * reviewers on other machines share the files through git, like any other change to the repository.
 */
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, watch, type FSWatcher } from "node:fs";
import type { IncomingMessage, ServerResponse } from "node:http";
import { join, resolve } from "node:path";
import type { Connect, Plugin } from "vite";
import {
  approveProposal,
  baselineOf,
  pendingProposal,
  recombineAfter,
  reviewProposal,
  type AskJson,
  type Proposal,
} from "../../../sdks/typescript/src/index.ts";
import { sha256Hex } from "../../../sdks/typescript/src/sha256.ts";
import { openWorkspace, writeJson, type RunRecord } from "../../../sdks/typescript/src/workspace.ts";
import type { ReviewAction, WorkspaceSnapshot } from "../src/lib/api-types.ts";

/** Names an agent would use. Intents are reviewed by people; the agent is their author, never a reviewer. */
export const AGENT_NAMES = /^(claude|codex|agent|assistant|ai|bot)$/i;

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
    public readonly extra: Record<string, unknown> = {},
  ) {
    super(message);
  }
}

function defaultReviewer(): string {
  // Set (even empty) wins over git: an empty value makes every reviewer say who they are.
  if (process.env.LASTRO_REVIEWER !== undefined) return process.env.LASTRO_REVIEWER.trim();
  try {
    return execFileSync("git", ["config", "user.name"], { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).trim();
  } catch {
    return "";
  }
}

/** The same handoff the CLI uses (`--llm handoff`): one request file per question, the answer next to it. */
function handoffAsk(dir: string): AskJson {
  return async (req) => {
    const id = `${req.step}-${sha256Hex(JSON.stringify(req)).slice(0, 12)}`;
    const answer = join(dir, "llm", `${id}.answer.json`);
    if (existsSync(answer)) return JSON.parse(readFileSync(answer, "utf8")) as unknown;
    const request = join(dir, "llm", `${id}.request.json`);
    writeJson(request, {
      instructions: `Act as the system described in "system". Read "user". Write ONLY a JSON value matching "schema" to ${answer}, then ask for the recombination again.`,
      ...req,
    });
    throw new ApiError(202, "The LLM has not answered yet: an agent (or a person) writes the answer file, then recombine again.", {
      status: "needs-llm",
      request,
      answer,
    });
  };
}

async function pickAsk(dir: string): Promise<AskJson> {
  if (process.env.ANTHROPIC_API_KEY) {
    const { claudeAsk } = await import("../../../sdks/typescript/src/llm/claude.ts");
    return claudeAsk();
  }
  return handoffAsk(dir);
}

/** Lines of a `.runs.jsonl` that parse; a line being appended at this instant is skipped. */
function readRunsTolerant(file: string): RunRecord[] {
  if (!existsSync(file)) return [];
  return readFileSync(file, "utf8")
    .split("\n")
    .flatMap((l) => {
      try {
        return l.trim() ? [JSON.parse(l) as RunRecord] : [];
      } catch {
        return [];
      }
    });
}

export function createApi(dir: string) {
  const ws = openWorkspace(dir);
  let queue: Promise<unknown> = Promise.resolve();
  /** Actions are applied one at a time, each to the latest file on disk. */
  const serial = <T>(f: () => Promise<T>): Promise<T> => {
    const next = queue.then(f, f);
    queue = next.catch(() => undefined);
    return next;
  };

  function snapshot(): WorkspaceSnapshot {
    if (!existsSync(dir)) throw new ApiError(404, `no .lastro workspace at ${dir}`);
    const proposals = ws.proposals();
    return {
      dir,
      reviewer: defaultReviewer(),
      llm: process.env.ANTHROPIC_API_KEY ? "anthropic" : "handoff",
      useCases: ws.useCases(),
      proposals,
      // A proposal's proof-run history, appended by `lastro prove` (ws.runs); tolerant while a run is being written.
      runs: Object.fromEntries(
        proposals.map((p) => {
          try {
            return [p.id, ws.runs(p)];
          } catch {
            return [p.id, readRunsTolerant(ws.proposalPath(p, ".runs.jsonl"))];
          }
        }),
      ),
    };
  }

  async function review(ref: string, reviewer: string, action: ReviewAction): Promise<{ proposal: Proposal; note?: string }> {
    const who = (reviewer ?? "").trim();
    if (!who) throw new ApiError(400, "Say who you are before reviewing.");
    if (AGENT_NAMES.test(who)) throw new ApiError(403, `"${who}" is an agent's name: intents are reviewed by people.`);
    return serial(async () => {
      const before = ws.proposal(ref);
      const r = reviewProposal(before, who);
      switch (action.type) {
        case "approve":
          r.approve(action.intent);
          break;
        case "amend":
          r.amend(action.intent, action.description.trim(), {}, action.note?.trim() || "amended by reviewer");
          break;
        case "reject":
          r.reject(action.intent, action.note.trim() || "rejected");
          break;
        case "answer":
          r.answer(action.decision, action.option);
          break;
        case "comment":
          r.comment(action.on, action.text.trim());
          break;
        case "reply":
          r.reply(action.thread, action.text.trim());
          break;
        case "resolve":
          r.resolve(action.thread);
          break;
        case "rewrite":
          r.rewriteAfter(action.useCase, action.after.trim());
          break;
        case "recombine": {
          const out = await recombineAfter(await pickAsk(dir), r.done(), action.useCase, action.instructions?.trim() || undefined);
          r.rewriteAfter(action.useCase, out.after, out.name);
          break;
        }
        default:
          throw new ApiError(400, `unknown action ${(action as { type: string }).type}`);
      }
      let p = r.done();
      let note: string | undefined;
      // Fully reviewed: approve it, bound to the current use cases (as `lastro review` does).
      if (!pendingProposal(p).length && !p.hash) {
        const useCases = ws.useCases();
        if (baselineOf(useCases) === p.baseline) p = approveProposal(p, useCases);
        else note = `The use cases changed since ${p.id} was extracted: run \`lastro propose\` on its prose again before it can be approved.`;
      }
      ws.writeProposal(p);
      return { proposal: p, ...(note ? { note } : {}) };
    });
  }

  return { dir, snapshot, review };
}

const readBody = (req: IncomingMessage) =>
  new Promise<string>((ok, fail) => {
    let s = "";
    req.on("data", (c) => (s += c));
    req.on("end", () => ok(s));
    req.on("error", fail);
  });

function send(res: ServerResponse, status: number, body: unknown) {
  res.statusCode = status;
  res.setHeader("content-type", "application/json; charset=utf-8");
  res.setHeader("cache-control", "no-store");
  res.end(JSON.stringify(body));
}

/** Vite plugin: the API on the dev server and on `vite preview`. */
export function lastroReviewApi(opts: { dir?: string } = {}): Plugin {
  const dir = resolve(process.env.LASTRO_DIR ?? opts.dir ?? ".lastro");
  const api = createApi(dir);
  const clients = new Set<ServerResponse>();
  const watchers = new Map<string, FSWatcher>();
  let timer: NodeJS.Timeout | undefined;

  const notify = () => {
    clearTimeout(timer);
    timer = setTimeout(() => {
      for (const c of clients) c.write(`event: change\ndata: {}\n\n`);
    }, 120);
  };

  /**
   * Watch the workspace and the folders that hold its entities, one directory at a time (a directory
   * watch keeps reporting its files while they are replaced or deleted). New folders are picked up as
   * they appear.
   */
  function startWatching() {
    for (const d of [dir, join(dir, "usecases"), join(dir, "proposals")]) {
      if (watchers.has(d) || !existsSync(d)) continue;
      const w = watch(d, () => {
        if (d === dir) startWatching();
        notify();
      });
      w.on("error", () => {
        w.close();
        watchers.delete(d);
      });
      watchers.set(d, w);
    }
  }

  const middleware: Connect.NextHandleFunction = async (req, res, next) => {
    const url = new URL(req.url ?? "/", "http://x");
    if (!url.pathname.startsWith("/api/")) return next();
    try {
      if (req.method === "GET" && url.pathname === "/api/workspace") return send(res, 200, api.snapshot());
      if (req.method === "GET" && url.pathname === "/api/events") {
        startWatching();
        res.writeHead(200, { "content-type": "text/event-stream", "cache-control": "no-store", connection: "keep-alive" });
        res.write(": connected\n\n");
        clients.add(res);
        req.on("close", () => clients.delete(res));
        return;
      }
      const m = url.pathname.match(/^\/api\/proposals\/([^/]+)\/review$/);
      if (req.method === "POST" && m) {
        const body = JSON.parse((await readBody(req)) || "{}") as { as?: string; action?: ReviewAction };
        if (!body.action) throw new ApiError(400, "missing action");
        return send(res, 200, await api.review(decodeURIComponent(m[1]!), body.as ?? "", body.action));
      }
      return send(res, 404, { error: `no route ${req.method} ${url.pathname}` });
    } catch (e) {
      if (e instanceof ApiError) return send(res, e.status, { error: e.message, ...e.extra });
      return send(res, 400, { error: (e as Error).message });
    }
  };

  return {
    name: "lastro-review-api",
    configureServer(server) {
      server.middlewares.use(middleware);
    },
    configurePreviewServer(server) {
      server.middlewares.use(middleware);
    },
  };
}
