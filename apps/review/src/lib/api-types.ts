import type { Proposal, UseCase } from "@lastro/sdk";

/** What the app loads from the workspace. */
export interface WorkspaceSnapshot {
  /** The `.lastro/` folder the server reads and writes. */
  dir: string;
  /** The default reviewer: LASTRO_REVIEWER, else `git config user.name`. */
  reviewer: string;
  /** How "recombine" reaches the LLM: Claude (ANTHROPIC_API_KEY set) or a handoff file an agent answers. */
  llm: "anthropic" | "handoff";
  useCases: UseCase[];
  proposals: Proposal[];
}

/** One review action, applied by the SDK's ProposalReview on the server. */
export type ReviewAction =
  | { type: "approve"; intent: string }
  | { type: "amend"; intent: string; description: string; note?: string }
  | { type: "reject"; intent: string; note: string }
  | { type: "answer"; decision: string; option: number }
  | { type: "comment"; on: string; text: string }
  | { type: "reply"; thread: string; text: string }
  | { type: "resolve"; thread: string }
  | { type: "rewrite"; useCase: string; after: string }
  | { type: "recombine"; useCase: string; instructions?: string };

export interface ReviewResult {
  proposal: Proposal;
  note?: string;
}
