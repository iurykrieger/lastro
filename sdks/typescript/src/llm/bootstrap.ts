/**
 * The survey step, transport-free: the extract step for a repository that already
 * exists. It reads the repository's digest (and source excerpts) and proposes the
 * use cases the code implements today, each behavior grounded in files, tests and
 * commits. The answer is checked by proposalFromSurvey (../bootstrap.ts).
 */
import { BEYOND_PROOF_REASONS, type Model } from "../types.js";
import { proposalFromSurvey, type RepoDigest, type SurveyInput, type SurveyOut } from "../bootstrap.js";
import type { Proposal, UseCase } from "../proposal.js";
import type { AskJson } from "./llm.js";

const STR = { type: "string" };
const STR_ARR = { type: "array", items: STR };
const OPTIONS = {
  type: "array",
  items: { type: "object", additionalProperties: false, required: ["label", "consequence"], properties: { label: STR, consequence: STR } },
};

export const SURVEY_SYSTEM = `You read an existing repository and write down the use cases its code implements today, as reviewable intents. You describe what the code DOES, not what it should do: people review your reading, and every behavior they approve becomes a proof that must pass against the code as it is.

You get the repository's digest (files with their exported symbols, tests by name, manifests, the newest commits with the files they touched), excerpts of its source, and the use cases already recorded.

Rules:
- A use case is one thing the application does for someone (a command, an endpoint, a screen, a library call, a job), with a stable kebab-case slug, a name, and "description": its complete current behavior as the code implements it, inputs, outputs, rules and errors included. Prefer what a user or caller sees over internal structure.
- Each use case lists its behaviors. A behavior is one checkable promise the code keeps today (one rule, one branch, one error case). Its description states that promise precisely, in one or two sentences; it becomes an intent a person approves, amends or rejects.
- Ground every behavior: "files" are paths from the digest that implement it, "tests" are test ids from the digest ("<file>::<name>") that exercise it, "commits" are hashes from the digest whose subject or files show why it is so (a feat or fix commit is the team stating the intended behavior). At least one file or test. Never cite anything that is not in the digest.
- Read tests as written-down behavior and commit history as intent: a "fix:" commit tells you which behavior is deliberate. When code, tests and commits disagree (a test that expects something the code does not do, a TODO, a revert), do not pick: add a decision for the reviewer.
- confidence: "high" when tests or a clear commit confirm the reading, "medium" when it rests on reading the code alone, "low" when it is a guess.
- change: "add" for a new use case; "modify" only for a recorded use case that has no proofs and whose description is wrong or incomplete. Leave out recorded use cases you agree with. Never touch a recorded use case that has proofs.
- provability: "formal" for a pure input-to-output decision (a function of its arguments), "empirical" when only running tests can check it, "beyond-proof" where proof stops; beyondProofReason is "none" unless beyond-proof: external-code, world-facts, termination-timing, human-judgment or unstated.
- models and subjects: only for formal behaviors decided by a function you can call with plain JSON. A model is JSON {"name","inputs":{name:domain},"outputs":{name:domain}} with finite domains where possible: {"type":"bool"}, {"type":"enum","values":[...]}, {"type":"int","min":..,"max":..}, {"type":"string","examples":[...]}. A subject names the module (as a runnable path, e.g. the built file) and its export, and the model it is checked against. Leave both empty when no such function exists; the proofs will be written later.
- notCovered: folders or files you did not turn into use cases, and why (generated code, tooling, too large to read now). Be honest: a partial bootstrap is fine.
- name, slug and description at the top level describe this bootstrap proposal.
- Slugs are kebab-case and unique within your answer.`;

export const SURVEY_SCHEMA = {
  type: "object",
  additionalProperties: false,
  required: ["name", "slug", "description", "useCases", "decisions", "notCovered", "models", "subjects"],
  properties: {
    name: STR,
    slug: STR,
    description: STR,
    useCases: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["slug", "change", "name", "description", "confidence", "behaviors"],
        properties: {
          slug: STR,
          change: { type: "string", enum: ["add", "modify"] },
          name: STR,
          description: STR,
          confidence: { type: "string", enum: ["high", "medium", "low"] },
          behaviors: {
            type: "array",
            items: {
              type: "object",
              additionalProperties: false,
              required: ["slug", "name", "description", "provability", "beyondProofReason", "files", "tests", "commits"],
              properties: {
                slug: STR,
                name: STR,
                description: STR,
                provability: { type: "string", enum: ["formal", "empirical", "beyond-proof"] },
                beyondProofReason: { type: "string", enum: ["none", ...BEYOND_PROOF_REASONS] },
                files: STR_ARR,
                tests: STR_ARR,
                commits: STR_ARR,
              },
            },
          },
        },
      },
    },
    decisions: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["slug", "name", "question", "options", "recommended", "behaviors"],
        properties: { slug: STR, name: STR, question: STR, options: OPTIONS, recommended: { type: "integer" }, behaviors: STR_ARR },
      },
    },
    notCovered: {
      type: "array",
      items: { type: "object", additionalProperties: false, required: ["path", "reason"], properties: { path: STR, reason: STR } },
    },
    models: {
      type: "array",
      items: { type: "object", additionalProperties: false, required: ["name", "json"], properties: { name: STR, json: { type: "string", description: "the model as a JSON string" } } },
    },
    subjects: {
      type: "array",
      items: { type: "object", additionalProperties: false, required: ["name", "module", "export", "model"], properties: { name: STR, module: STR, export: STR, model: STR } },
    },
  },
};

/** The digest as the prompt shows it: compact, without the hash. */
function digestView(d: RepoDigest) {
  return {
    head: d.head,
    scope: d.scope,
    totals: d.totals,
    truncated: d.truncated,
    manifests: d.manifests,
    areas: d.areas,
    files: d.files.map((f) => ({ path: f.path, role: f.role, lines: f.lines, ...(f.symbols.length ? { symbols: f.symbols } : {}) })),
    tests: d.tests.map((t) => t.id),
    commits: d.commits.map((c) => ({ sha: c.sha, date: c.date.slice(0, 10), subject: c.subject, ...(c.body ? { body: c.body } : {}), files: c.files, ...(c.moreFiles ? { moreFiles: c.moreFiles } : {}) })),
  };
}

export interface Excerpt {
  path: string;
  text: string;
  truncated: boolean;
}

export function surveyUserPrompt(digest: RepoDigest, excerpts: Excerpt[], useCases: UseCase[], models: Record<string, Model> = {}): string {
  const recorded = useCases.map((u) => ({ slug: u.slug, name: u.name, description: u.description, proofs: u.proofs.length }));
  const source = excerpts.map((e) => `<file path="${e.path}"${e.truncated ? ' truncated="true"' : ""}>\n${e.text}\n</file>`).join("\n");
  return [
    `<use-cases>\n${JSON.stringify(recorded, null, 2)}\n</use-cases>`,
    ...(Object.keys(models).length ? [`<declared-models>\n${JSON.stringify(Object.keys(models))}\n</declared-models>`] : []),
    `<digest>\n${JSON.stringify(digestView(digest), null, 2)}\n</digest>`,
    `<source>\n${source}\n</source>`,
    "Excerpts are cut to a budget. If you can read the repository yourself, open any file of the digest you need; cite only paths, tests and commits that are in the digest.",
  ].join("\n\n");
}

/** The survey step: the repository read into an open bootstrap proposal. */
export async function surveyProposal(ask: AskJson, input: SurveyInput, excerpts: Excerpt[]): Promise<Proposal> {
  const out = (await ask({ step: "survey", system: SURVEY_SYSTEM, user: surveyUserPrompt(input.digest, excerpts, input.useCases, input.config.models), schema: SURVEY_SCHEMA })) as SurveyOut;
  return proposalFromSurvey(input, out);
}
