/** The docs' table of contents: sidebar, search, and previous/next links all read from here. */
export interface NavPage {
  slug: string;
  title: string;
  description: string;
  /** Extra words for search. */
  keywords?: string;
}

export interface NavSection {
  title: string;
  pages: NavPage[];
}

export const NAV: NavSection[] = [
  {
    title: "Getting started",
    pages: [
      { slug: "introduction", title: "Introduction", description: "What Lastro is, what it is not, and how the pieces fit.", keywords: "overview pdd proof driven development" },
      { slug: "installation", title: "Installation", description: "Build the SDKs, the CLIs and the Rust prover from the repository.", keywords: "install setup npm go dotnet cargo wasm" },
      { slug: "quickstart", title: "Quickstart", description: "From a paragraph of prose to a proved function in eight commands.", keywords: "tutorial walkthrough shipping example cli" },
    ],
  },
  {
    title: "Concepts",
    pages: [
      { slug: "concepts/pipeline", title: "The pipeline", description: "Plan, review, spec, prove: what is AI, what is deterministic, and who writes the code.", keywords: "flow stages harness agnostic" },
      { slug: "concepts/intents", title: "Prose and intents", description: "How prose becomes reviewable intents, each anchored to the words it came from.", keywords: "extract intent map quote kind provability" },
      { slug: "concepts/review", title: "Review and approval policy", description: "Approve, amend, reject; how many approvals an intent needs; freezing by hash.", keywords: "approve amend reject policy requiredApprovals freeze hash carry review" },
      { slug: "concepts/decisions", title: "Decisions", description: "Ambiguities and contradictions come back as decision cards; answers become intents.", keywords: "decision card contradiction consistency question" },
      { slug: "concepts/spec", title: "Model and formal spec", description: "The model, LSL clauses, trusted pieces, scenarios and what is beyond proof.", keywords: "model domain lsl clause assumption scenario trusted beyond proof plan" },
      { slug: "concepts/verdicts", title: "Proved, trusted, evidenced", description: "What each verdict promises, and where proof stops.", keywords: "proved evidenced trusted assumptions violated vacuous weak mutants gap rule of three" },
      { slug: "concepts/receipts", title: "Receipts and feedback", description: "The receipt for a pull request and the failure brief for whoever writes the code.", keywords: "receipt markdown agent feedback failures counterexample two engines" },
    ],
  },
  {
    title: "CLI",
    pages: [
      { slug: "cli/overview", title: "Commands", description: "init, plan, status, review, spec, prove and feedback, with their JSON and exit codes.", keywords: "commands flags exit codes json" },
      { slug: "cli/workspace", title: "The .lastro/ workspace", description: "Every file the CLI writes, and how the three CLIs share one workspace.", keywords: "workspace files config intents spec proof receipt" },
      { slug: "cli/llm", title: "LLM handoff", description: "Run the two AI steps on the Anthropic API, or hand them to the calling agent's own model.", keywords: "handoff anthropic api key agent harness claude code codex exit 3" },
    ],
  },
  {
    title: "SDKs",
    pages: [
      { slug: "sdks/typescript", title: "TypeScript", description: "createLastro, review, spec, prove, and the building blocks underneath.", keywords: "node npm createLastro claudeAsk ts" },
      { slug: "sdks/go", title: "Go", description: "lastro.New, subjects in any language, and the lastro-go CLI.", keywords: "golang ProcessSubject TableSubject lastro-go" },
      { slug: "sdks/dotnet", title: ".NET", description: "LastroClient, Lastro.Claude and the lastro .NET tool.", keywords: "csharp c# dotnet nuget LastroClient" },
      { slug: "sdks/interop", title: "Interoperability", description: "Same wire format, hashes and verdicts across SDKs, checked in CI.", keywords: "cross check hashes canonical json interop" },
    ],
  },
  {
    title: "Reference",
    pages: [
      { slug: "reference/lsl", title: "LSL", description: "The Lastro Spec Language: a JSON AST, never evaluated as code.", keywords: "operators ops expression json ast smt-lib" },
      { slug: "reference/data-model", title: "Data model", description: "Domains, intents, decisions, plans and reports as JSON.", keywords: "types schema domain json intent artifact proof plan report" },
      { slug: "reference/rust-prover", title: "Rust prover", description: "lastro-core: the independent second engine, as a CLI and as WASM.", keywords: "rust lastro-core wasm lastro-prove coverage" },
    ],
  },
];

export const PAGES: NavPage[] = NAV.flatMap((s) => s.pages);
export const sectionOf = (slug: string) => NAV.find((s) => s.pages.some((p) => p.slug === slug));
