/**
 * Bootstrap: use cases and proofs for a repository that already exists.
 *
 *   repository ─scan─▶ DIGEST (files, tests, commits; deterministic, hashed)
 *              ─survey─▶ a BOOTSTRAP PROPOSAL: one intent per behavior the code shows today,
 *                        each grounded in files, tests and commits of the digest
 *              ─▶ review ─▶ derive ─▶ prove (against the code as it is) ─▶ apply
 *
 * The survey is the extract step reading code instead of prose: the LLM proposes,
 * people review. Nothing it says becomes a use case until a reviewer approves it,
 * a proof derived from it passes against today's code, and the proposal is applied.
 * A proof that fails here does not ask for a code change: it means the description
 * is wrong, or the code has a bug the team now has to decide about.
 *
 * Everything in this file is pure; scanning the disk and git lives in bootstrap/scan.ts.
 */
import { canonicalJson } from "./artifact.js";
import { proposalFromExtraction, ProposalError, SLUG, type ChangeKind, type Proposal, type ProposalPolicy, type ProposeOut, type UseCase } from "./proposal.js";
import { sha256Hex } from "./sha256.js";
import type { Domain } from "./spec/domain.js";
import { type DecisionOption, type Model, type Provability } from "./types.js";

// ---------------------------------------------------------------------------
// The digest (written by bootstrap/scan.ts)
// ---------------------------------------------------------------------------

export interface RepoFile {
  path: string;
  role: "source" | "test" | "doc" | "config" | "other";
  language?: string;
  lines: number;
  /** Exported or public names, read with simple per-language patterns. */
  symbols: string[];
}

export interface RepoTest {
  /** "<file>::<test name>" */
  id: string;
  file: string;
  name: string;
}

export interface RepoCommit {
  sha: string;
  date: string;
  subject: string;
  body?: string;
  files: string[];
  moreFiles?: number;
}

export interface RepoManifest {
  path: string;
  kind: "npm" | "go" | "cargo" | "dotnet" | "python" | "unknown";
  name?: string;
  entries: string[];
  scripts: string[];
}

export interface RepoArea {
  path: string;
  files: number;
  lines: number;
  tests: number;
  commits: number;
}

export interface RepoDigest {
  lastro: "repo-digest/v1";
  /** The commit the digest was read at (null outside git). */
  head: string | null;
  scope: string[];
  totals: { files: number; commits: number };
  truncated: { files: boolean; commits: boolean };
  since?: string;
  manifests: RepoManifest[];
  areas: RepoArea[];
  files: RepoFile[];
  tests: RepoTest[];
  commits: RepoCommit[];
  hash: string;
}

export function digestHash(d: Omit<RepoDigest, "hash"> & { hash?: string }): string {
  const { hash: _, ...core } = d;
  return "sha256:" + sha256Hex(canonicalJson(core));
}

// ---------------------------------------------------------------------------
// What a bootstrap proposal adds to a proposal
// ---------------------------------------------------------------------------

/** Where a proposal came from. Absent on proposals written from prose before bootstrap existed. */
export interface ProposalOrigin {
  kind: "prose" | "bootstrap";
  /** bootstrap: the commit the repository was read at. */
  head?: string | null;
  /** bootstrap: the digest's hash. */
  digest?: string;
  /** bootstrap: the folders it covers (empty: the whole repository). */
  scope?: string[];
}

/** Where in the repository a bootstrap intent was read: the reviewer checks these. */
export interface Grounds {
  files: string[];
  /** Test ids, "<file>::<test name>". */
  tests: string[];
  /** Full commit hashes. */
  commits: string[];
  confidence: "high" | "medium" | "low";
}

export interface SubjectDecl {
  /** Module path, relative to the folder that holds the workspace. */
  module: string;
  export?: string;
}

/** Models and subjects a proposal adds to `.lastro/config.json` when it is applied. Reviewed with the intents. */
export interface ProposalSetup {
  models: Record<string, Model>;
  subjects: Record<string, SubjectDecl>;
}

// ---------------------------------------------------------------------------
// The survey step's answer, checked
// ---------------------------------------------------------------------------

export interface SurveyOut {
  name: string;
  slug: string;
  description: string;
  useCases: {
    slug: string;
    change: "add" | "modify";
    name: string;
    /** The complete current behavior, as the code implements it. */
    description: string;
    confidence: "high" | "medium" | "low";
    behaviors: {
      slug: string;
      name: string;
      description: string;
      provability: Provability;
      beyondProofReason: string;
      files: string[];
      tests: string[];
      commits: string[];
    }[];
  }[];
  decisions: { slug: string; name: string; question: string; options: DecisionOption[]; recommended: number; behaviors: string[] }[];
  notCovered: { path: string; reason: string }[];
  models: { name: string; json: string }[];
  subjects: { name: string; module: string; export: string; model: string }[];
}

export interface SurveyInput {
  id: string;
  digest: RepoDigest;
  submittedBy: string;
  useCases: UseCase[];
  policy: ProposalPolicy;
  /** Use-case ids other open proposals reserved (see reservedUseCaseIds). */
  reserved?: string[];
  /** Models and subjects already declared in `.lastro/config.json`. */
  config: { models: Record<string, Model>; subjects: Record<string, SubjectDecl> };
}

const short = (sha: string) => sha.slice(0, 12);

/** Resolve a commit reference (full hash or a prefix of at least 7) against the digest. */
function resolveCommit(digest: RepoDigest, ref: string): string | undefined {
  const r = ref.trim().toLowerCase();
  if (r.length < 7) return undefined;
  const hits = digest.commits.filter((c) => c.sha.startsWith(r));
  return hits.length === 1 ? hits[0]!.sha : undefined;
}

function domainProblems(where: string, d: unknown): string[] {
  const x = d as Domain;
  if (!x || typeof x !== "object") return [`${where}: not a domain`];
  if (x.type === "bool") return [];
  if (x.type === "enum") return Array.isArray(x.values) && x.values.length && x.values.every((v) => typeof v === "string" || typeof v === "number") ? [] : [`${where}: enum needs values`];
  if (x.type === "int") return (x.min === undefined || Number.isInteger(x.min)) && (x.max === undefined || Number.isInteger(x.max)) ? [] : [`${where}: int bounds must be integers`];
  if (x.type === "string") return x.examples === undefined || (Array.isArray(x.examples) && x.examples.every((v) => typeof v === "string")) ? [] : [`${where}: string examples must be strings`];
  return [`${where}: unknown domain type "${(x as { type?: unknown }).type}"`];
}

/** Check a model's shape (the expressions in its defs are checked when proofs use them). */
export function modelProblems(where: string, m: unknown): string[] {
  const x = m as Model;
  if (!x || typeof x !== "object") return [`${where}: not a model`];
  const out: string[] = [];
  if (typeof x.name !== "string" || !x.name) out.push(`${where}: model needs a name`);
  for (const side of ["inputs", "outputs"] as const) {
    if (!x[side] || typeof x[side] !== "object" || !Object.keys(x[side]).length) out.push(`${where}: model needs ${side}`);
    else for (const [k, d] of Object.entries(x[side])) out.push(...domainProblems(`${where}.${side}.${k}`, d));
  }
  return out;
}

/**
 * The bootstrap proposal's prose: a report of what the survey read, one section
 * per use case and one line per behavior. Generated, never edited: each intent's
 * quote is anchored in it, and its grounds are listed under it.
 */
export function renderSurveyReport(digest: RepoDigest, out: SurveyOut): string {
  const commit = (sha: string) => {
    const c = digest.commits.find((x) => x.sha === sha);
    return c ? `${sha.slice(0, 7)} (${c.subject})` : sha.slice(0, 7);
  };
  const lines: string[] = [`# ${out.name}`, "", out.description, ""];
  const read = `Read from ${digest.head ? `commit ${short(digest.head)}` : "a folder outside git"}: ${digest.files.length} files, ${digest.tests.length} tests, ${digest.commits.length} of ${digest.totals.commits} commits${digest.scope.length ? `, scope ${digest.scope.join(", ")}` : ""}.`;
  lines.push(read, "");
  for (const uc of out.useCases ?? []) {
    lines.push(`## ${uc.name} (${uc.slug}, ${uc.change}, ${uc.confidence} confidence)`, "", uc.description, "");
    for (const b of uc.behaviors ?? []) {
      lines.push(`- ${b.description}`);
      const g: string[] = [];
      if (b.files?.length) g.push(`files ${b.files.map((f) => `\`${f}\``).join(", ")}`);
      if (b.tests?.length) g.push(`tests ${b.tests.map((t) => `\`${t}\``).join(", ")}`);
      const shas = (b.commits ?? []).map((c) => resolveCommit(digest, c) ?? c);
      if (shas.length) g.push(`commits ${shas.map(commit).join(", ")}`);
      if (g.length) lines.push(`  Grounds: ${g.join("; ")}.`);
    }
    lines.push("");
  }
  if (out.notCovered?.length) {
    lines.push("## Not covered", "");
    for (const n of out.notCovered) lines.push(`- \`${n.path}\`: ${n.reason}`);
    lines.push("");
  }
  return lines.join("\n").trimEnd() + "\n";
}

/**
 * Build a bootstrap proposal from the survey's answer. Every behavior becomes an
 * intent that adds (or, for a use case with no proofs yet, modifies) one use case;
 * its grounds must name files, tests and commits of the digest. Throws
 * ProposalError with every problem found.
 */
export function proposalFromSurvey(input: SurveyInput, out: SurveyOut): Proposal {
  const { digest, useCases } = input;
  const problems: string[] = [];
  const files = new Set(digest.files.map((f) => f.path));
  const tests = new Set(digest.tests.map((t) => t.id));
  const bySlug = new Map(useCases.map((u) => [u.slug, u]));
  const grounds: Grounds[] = [];

  if (!(out.useCases ?? []).length) problems.push("the survey found no use case");
  for (const uc of out.useCases ?? []) {
    const where = `use case "${uc.slug}"`;
    if (!SLUG.test(uc.slug ?? "")) problems.push(`${where}: slug is not kebab-case`);
    if (uc.change !== "add" && uc.change !== "modify") problems.push(`${where}: a bootstrap only adds or modifies use cases, not "${uc.change}"`);
    const existing = bySlug.get(uc.slug);
    if (uc.change === "add" && existing) problems.push(`${where}: already exists as ${existing.id}; modify it or leave it out`);
    if (uc.change === "modify" && !existing) problems.push(`${where}: modify names an unknown use case`);
    if (uc.change === "modify" && existing?.proofs.length) problems.push(`${where}: ${existing.id} already has proofs; change proven behavior with a proposal, not a bootstrap`);
    if (!["high", "medium", "low"].includes(uc.confidence)) problems.push(`${where}: confidence must be high, medium or low`);
    if (!(uc.behaviors ?? []).length) problems.push(`${where}: no behavior`);
    for (const b of uc.behaviors ?? []) {
      const bw = `behavior "${b.slug}"`;
      const g: Grounds = { files: [], tests: [], commits: [], confidence: uc.confidence };
      for (const f of b.files ?? []) {
        if (files.has(f)) g.files.push(f);
        else problems.push(`${bw}: file ${f} is not in the digest`);
      }
      for (const t of b.tests ?? []) {
        if (tests.has(t)) g.tests.push(t);
        else problems.push(`${bw}: test ${t} is not in the digest (use "<file>::<name>")`);
      }
      for (const c of b.commits ?? []) {
        const sha = resolveCommit(digest, c);
        if (sha) g.commits.push(sha);
        else problems.push(`${bw}: commit ${c} is not in the digest (or the prefix is ambiguous)`);
      }
      if (!(b.files ?? []).length && !(b.tests ?? []).length) problems.push(`${bw}: needs at least one file or test as grounds`);
      g.files = [...new Set(g.files)];
      g.tests = [...new Set(g.tests)];
      g.commits = [...new Set(g.commits)];
      grounds.push(g);
    }
  }

  // Models and subjects the proofs will need, reviewed with the intents.
  const setup: ProposalSetup = { models: {}, subjects: {} };
  for (const m of out.models ?? []) {
    let model: unknown;
    try {
      model = JSON.parse(m.json);
    } catch {
      problems.push(`model ${m.name}: not valid JSON`);
      continue;
    }
    const errs = modelProblems(`model ${m.name}`, model);
    if ((model as Model)?.name !== m.name) errs.push(`model ${m.name}: its json names "${(model as Model)?.name}"`);
    if (input.config.models[m.name]) errs.push(`model ${m.name} is already declared in .lastro/config.json`);
    problems.push(...errs);
    if (!errs.length) setup.models[m.name] = model as Model;
  }
  for (const s of out.subjects ?? []) {
    if (!s.name || !s.module) problems.push(`subject "${s.name}": needs a name and a module`);
    if (input.config.subjects[s.name]) problems.push(`subject ${s.name} is already declared in .lastro/config.json`);
    if (s.model && !setup.models[s.model] && !input.config.models[s.model]) problems.push(`subject ${s.name}: unknown model ${s.model}`);
    setup.subjects[s.name] = { module: s.module, ...(s.export && s.export !== "default" ? { export: s.export } : {}) };
  }
  if (problems.length) throw new ProposalError(problems);

  const prose = renderSurveyReport(digest, out);
  const propose: ProposeOut = {
    name: out.name,
    slug: out.slug,
    description: out.description,
    intents: out.useCases.flatMap((uc) =>
      uc.behaviors.map((b) => ({
        slug: b.slug,
        name: b.name,
        description: b.description,
        change: uc.change as ChangeKind,
        useCase: uc.slug,
        provability: b.provability,
        beyondProofReason: b.beyondProofReason,
        quote: b.description,
        replaces: [],
      })),
    ),
    useCases: out.useCases.map((uc) => ({ slug: uc.slug, change: uc.change, name: uc.name, after: uc.description })),
    decisions: (out.decisions ?? []).map((d) => ({ slug: d.slug, name: d.name, question: d.question, options: d.options, recommended: d.recommended, intents: d.behaviors })),
    notPromised: (out.notCovered ?? []).map((n) => `Not covered: ${n.path} (${n.reason})`),
  };
  const p = proposalFromExtraction({ id: input.id, prose, submittedBy: input.submittedBy, useCases, policy: input.policy, ...(input.reserved ? { reserved: input.reserved } : {}) }, propose);
  p.intents.forEach((it, k) => (it.grounds = grounds[k]));
  p.origin = { kind: "bootstrap", head: digest.head, digest: digest.hash, scope: [...digest.scope] };
  if (Object.keys(setup.models).length || Object.keys(setup.subjects).length) p.setup = setup;
  return p;
}

export const isBootstrap = (p: Proposal) => p.origin?.kind === "bootstrap";

/** Models and subjects for derive and prove: the config's plus the ones the proposal brings. */
export function setupOf(p: Proposal, config: { models: Record<string, Model>; subjects: Record<string, SubjectDecl> }): { models: Record<string, Model>; subjects: Record<string, SubjectDecl> } {
  return { models: { ...config.models, ...(p.setup?.models ?? {}) }, subjects: { ...config.subjects, ...(p.setup?.subjects ?? {}) } };
}

/** What a failing proof means in a bootstrap: the code is the reference, so the description or the proof is what is in doubt. */
export const BOOTSTRAP_FEEDBACK_NOTE =
  "This is a bootstrap proposal: the proofs describe the code as it is today, so do not change the code to make them pass. A failing proof means the use case or the proof misreads the code (amend the intent and derive again), or the code has a bug the team must decide about (reject the intent, or keep it and open a proposal that fixes the code).";

/** The digest's areas, largest first: how a large repository can split its bootstrap into one proposal per area. */
export function suggestScopes(digest: RepoDigest): string[] {
  return digest.areas.filter((a) => a.files > 0 && a.path !== ".").sort((a, b) => b.files - a.files || a.path.localeCompare(b.path)).map((a) => a.path);
}
