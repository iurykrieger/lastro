/**
 * `lastro bootstrap --surfaces`: for a repository whose features have no surfaces yet, the survey step reads the code
 * and proposes the surfaces and dependencies of each. The answer becomes a bootstrap proposal whose intents only add
 * surfaces and dependencies and never change a description; it is reviewed like any other proposal.
 */
import { canonicalJson } from "./artifact.js";
import { surfaceProblems, type Surface } from "./features.js";
import { baselineOf, SLUG, type Proposal, type ProposalIntent, type ProposalPolicy, type UseCase, type UseCaseChange } from "./proposal.js";
import type { RepoDigest } from "./bootstrap.js";
import { ProposalError } from "./proposal.js";

export interface SurfacesOut {
  features: {
    slug: string;
    surfaces: Surface[];
    /** Slugs of the features this one needs. */
    dependsOn: string[];
    /** Paths from the digest where the surfaces were read. */
    files: string[];
    /** Set by a confused answer: a bootstrap of surfaces never rewrites what a feature does. */
    description?: string;
  }[];
}

export interface SurfacesInput {
  id: string;
  submittedBy: string;
  useCases: UseCase[];
  policy: ProposalPolicy;
  digest: RepoDigest;
}

/** The features a surfaces bootstrap proposes for: the ones with no surface yet. */
export const featuresWithoutSurfaces = (useCases: UseCase[]) => useCases.filter((u) => !u.surfaces?.length);

/** Build the bootstrap proposal. Throws ProposalError with every problem found. */
export function proposalFromSurfaces(input: SurfacesInput, out: SurfacesOut): Proposal {
  const { id, useCases, digest } = input;
  const problems: string[] = [];
  const bySlug = new Map(useCases.map((u) => [u.slug, u]));
  const open = new Set(featuresWithoutSurfaces(useCases).map((u) => u.slug));
  const files = new Set(digest.files.map((f) => f.path));
  const intents: ProposalIntent[] = [];
  const changes: UseCaseChange[] = [];
  for (const f of out.features ?? []) {
    const u = bySlug.get(f.slug);
    const at = `feature "${f.slug}"`;
    if (!u) {
      problems.push(`${at}: unknown feature`);
      continue;
    }
    if (!open.has(f.slug)) {
      problems.push(`${at}: ${u.id} already has surfaces; change them with a proposal`);
      continue;
    }
    if (f.description !== undefined && f.description.trim() && f.description.trim() !== u.description.trim()) {
      problems.push(`${at}: a surfaces bootstrap never changes a description`);
      continue;
    }
    if (!f.surfaces?.length && !f.dependsOn?.length) {
      problems.push(`${at}: adds no surface or dependency`);
      continue;
    }
    const deps = (f.dependsOn ?? []).map((s) => {
      const d = bySlug.get(s);
      if (!d) problems.push(`${at}: depends on unknown feature "${s}"`);
      return d?.id ?? s;
    });
    problems.push(...surfaceProblems({ id: u.id, surfaces: f.surfaces ?? [], dependsOn: deps }));
    for (const p of f.files ?? []) if (!files.has(p)) problems.push(`${at}: file ${p} is not in the digest`);
    const iid = `${id}.I-${String(intents.length + 1).padStart(2, "0")}`;
    const what = [
      ...(f.surfaces ?? []).map((s) => `${s.kind} ${s.locator} (${s.name})`),
      ...deps.map((d) => `depends on ${d}`),
    ].join("; ");
    intents.push({
      id: iid,
      slug: `surfaces-of-${u.slug}`.slice(0, 80).replace(/-+$/, ""),
      name: `Surfaces of ${u.name}`,
      description: `${u.id} is reached through: ${what}.`,
      change: "modify",
      useCase: u.id,
      provability: "empirical",
      source: { quote: "", start: 0, end: 0 },
      replaces: [],
      status: "proposed",
      approvals: [],
      grounds: { files: [...(f.files ?? [])], tests: [], commits: [], confidence: "medium" },
    });
    changes.push({
      useCase: u.id,
      slug: u.slug,
      change: "modify",
      name: u.name,
      before: u.description,
      after: u.description,
      intents: [iid],
      ...(f.surfaces?.length ? { surfaces: f.surfaces } : {}),
      ...(deps.length ? { dependsOn: deps } : {}),
    });
  }
  if (!intents.length && !problems.length) problems.push("the survey proposed no surface");
  for (const i of intents) if (!SLUG.test(i.slug)) problems.push(`${i.id}: slug "${i.slug}" is not kebab-case`);
  if (problems.length) throw new ProposalError(problems);
  const prose = intents.map((i) => `- ${i.description}`).join("\n");
  return {
    lastro: "proposal/v1",
    id,
    slug: "bootstrap-surfaces",
    name: "Bootstrap the feature map",
    description: `The surfaces and dependencies of ${intents.length} feature(s), read from the code. Descriptions are unchanged.`,
    status: "open",
    submittedBy: input.submittedBy,
    prose,
    baseline: baselineOf(useCases),
    intents,
    useCases: changes,
    decisions: [],
    threads: [],
    notPromised: ["A surfaces bootstrap does not change what any feature does."],
    policy: { ...input.policy },
    proofs: [],
    supersedes: [],
    origin: { kind: "bootstrap", head: digest.head, digest: digest.hash, scope: digest.scope, surfaces: true },
  };
}

export const SURFACES_SYSTEM = `You read an existing repository and write down how a user or a client reaches each of its features: the feature map.

You get the repository's digest, excerpts of its source and the features that have no surfaces yet (slug, name, description).

For each of those features:
- surfaces: every way to reach it. kind "screen" (a screen or a part of one), "route" (a URL path of a web app), "command" (a CLI command) or "api" (an HTTP method and path, or a library call). name is in the user's words ("the left sidebar's chat list"); locator is what a driver understands (a URL path, a CLI command line, "GET /api/chats", a CSS selector).
- dependsOn: the slugs of the features it needs to work (opening a chat depends on signing in). Only features from the list or already recorded.
- files: digest paths where you read them.
Never change what a feature does: leave its description out. Leave out a feature you cannot place.`;

const STR = { type: "string" };
const STR_ARR = { type: "array", items: STR };

export const SURFACES_SCHEMA = {
  type: "object",
  additionalProperties: false,
  required: ["features"],
  properties: {
    features: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["slug", "surfaces", "dependsOn", "files"],
        properties: {
          slug: STR,
          surfaces: {
            type: "array",
            items: { type: "object", additionalProperties: false, required: ["kind", "name", "locator"], properties: { kind: { type: "string", enum: ["screen", "route", "command", "api"] }, name: STR, locator: STR } },
          },
          dependsOn: STR_ARR,
          files: STR_ARR,
        },
      },
    },
  },
};

export function surfacesUserPrompt(digest: RepoDigest, excerpts: { path: string; text: string; truncated: boolean }[], useCases: UseCase[]): string {
  const open = featuresWithoutSurfaces(useCases).map((u) => ({ slug: u.slug, name: u.name, description: u.description }));
  const recorded = useCases.filter((u) => u.surfaces?.length).map((u) => ({ slug: u.slug, name: u.name }));
  const source = excerpts.map((e) => `<file path="${e.path}"${e.truncated ? ' truncated="true"' : ""}>\n${e.text}\n</file>`).join("\n");
  return [
    `<features-without-surfaces>\n${JSON.stringify(open, null, 2)}\n</features-without-surfaces>`,
    `<features-with-surfaces>\n${JSON.stringify(recorded)}\n</features-with-surfaces>`,
    `<digest>\n${canonicalJson({ files: digest.files.map((f) => ({ path: f.path, symbols: f.symbols })), manifests: digest.manifests })}\n</digest>`,
    `<source>\n${source}\n</source>`,
  ].join("\n\n");
}
