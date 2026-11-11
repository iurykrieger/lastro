/**
 * The `.lastro/` workspace on disk (Node only): config, use cases and proposals.
 *
 *   .lastro/config.json                       models, subjects and the review policy
 *   .lastro/usecases/<id>-<slug>.json         one file per use case, with its regression proofs
 *   .lastro/proposals/<id>-<slug>.md          a proposal's prose
 *   .lastro/proposals/<id>-<slug>.json        a proposal
 *   .lastro/proposals/<id>-<slug>.receipt.md  its last receipt
 *   .lastro/llm/                              LLM handoff requests and answers
 */
import { existsSync, mkdirSync, readdirSync, readFileSync, unlinkSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileNameOf, nextId, readPolicy, type Proposal, type ProposalPolicy, type UseCase } from "./proposal.js";
import type { Model } from "./types.js";

export interface SubjectRef {
  /** Module path, relative to the folder that holds the workspace. */
  module: string;
  /** Export name (default "default"). */
  export?: string;
}

export interface WorkspaceConfig {
  models: Record<string, Model>;
  subjects: Record<string, SubjectRef>;
  review: ProposalPolicy;
}

/** Files of the intent-artifact/v0 workspace, which proposal/v1 refuses. */
export const LEGACY_FILES = ["intents.json", "spec.json", "proof.json"];

export class WorkspaceError extends Error {}

const readJson = <T>(p: string): T => JSON.parse(readFileSync(p, "utf8")) as T;
export const writeJson = (p: string, v: unknown) => {
  mkdirSync(dirname(p), { recursive: true });
  writeFileSync(p, JSON.stringify(v, null, 2) + "\n");
};

export function openWorkspace(dir: string) {
  const file = (...parts: string[]) => join(dir, ...parts);
  const listJson = (sub: string) => (existsSync(file(sub)) ? readdirSync(file(sub)).filter((f) => f.endsWith(".json")).sort() : []);

  const ws = {
    dir,
    file,
    /** The folder module paths in the config are relative to. */
    root: dirname(dir),

    exists: () => existsSync(file("config.json")),

    /** intents.json, spec.json or proof.json from the old workspace format. */
    legacy: () => LEGACY_FILES.filter((f) => existsSync(file(f))),

    config(): WorkspaceConfig {
      if (!existsSync(file("config.json"))) throw new WorkspaceError(`no workspace at ${dir}: run \`lastro init --model model.json\``);
      const raw = readJson<{ models?: Record<string, Model>; model?: Model; subjects?: Record<string, SubjectRef>; review?: Partial<ProposalPolicy>; requiredApprovals?: number }>(file("config.json"));
      const models = { ...(raw.models ?? {}) };
      if (raw.model && !models[raw.model.name]) models[raw.model.name] = raw.model; // the v0 `model` key is still read
      return { models, subjects: raw.subjects ?? {}, review: readPolicy(raw) };
    },

    writeConfig(c: WorkspaceConfig) {
      writeJson(file("config.json"), c);
    },

    useCases(): UseCase[] {
      return listJson("usecases").map((f) => readJson<UseCase>(file("usecases", f)));
    },

    writeUseCase(u: UseCase) {
      writeJson(file("usecases", fileNameOf(u)), u);
    },

    removeUseCase(u: { id: string; slug: string }) {
      const p = file("usecases", fileNameOf(u));
      if (existsSync(p)) unlinkSync(p);
    },

    proposals(): Proposal[] {
      return listJson("proposals").map((f) => readJson<Proposal>(file("proposals", f)));
    },

    nextProposalId(): string {
      const ids = existsSync(file("proposals")) ? readdirSync(file("proposals")).map((f) => f.match(/^(P-\d+)-/)?.[1]).filter((x): x is string => !!x) : [];
      return nextId("P", ids);
    },

    /** A proposal by id ("P-001", "1") or slug; without one, the latest that is not applied or closed. */
    proposal(ref?: string): Proposal {
      const all = ws.proposals();
      if (!ref) {
        const open = all.filter((p) => p.status !== "applied" && p.status !== "closed");
        const last = open[open.length - 1];
        if (!last) throw new WorkspaceError("no open proposal: run `lastro propose <prose-file>`");
        return last;
      }
      const id = /^\d+$/.test(ref) ? `P-${ref.padStart(3, "0")}` : ref;
      const p = all.find((x) => x.id === id || x.slug === ref);
      if (!p) throw new WorkspaceError(`no proposal ${ref}`);
      return p;
    },

    proposalPath: (p: { id: string; slug: string }, ext = ".json") => file("proposals", `${p.id}-${p.slug}${ext}`),

    writeProposal(p: Proposal) {
      writeJson(ws.proposalPath(p), p);
    },
  };
  return ws;
}

export type Workspace = ReturnType<typeof openWorkspace>;
