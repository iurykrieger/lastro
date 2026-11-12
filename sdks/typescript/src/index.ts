export * from "./types.js";
export { L, evaluate, holds, show, validateExpr, freeVars, toSmtLib, LslError, OPS } from "./spec/expr.js";
export type { Expr, Op } from "./spec/expr.js";
export { D, domainSize, spaceSize, enumerateSpace, inDomain } from "./spec/domain.js";
export type { Domain } from "./spec/domain.js";
export {
  createArtifact,
  review,
  Review,
  approve,
  reopen,
  isApproved,
  pendingReview,
  approvedIntents,
  carryReview,
  raiseDecisions,
  requiredApprovals,
  approvalCount,
  canonicalJson,
  hashOf,
  ArtifactError,
} from "./artifact.js";
export type { IntentDraft, DecisionDraft } from "./artifact.js";
export { checkPlan, validatePlan, PlanError } from "./plan.js";
export { verify, explainRun, checkConsistency } from "./verify/engine.js";
export type { VerifyOptions, ClauseExplanation, ClauseStatus } from "./verify/engine.js";
export { renderReceipt } from "./receipt.js";
export { StaticExtractor, StaticDeriver } from "./llm/interfaces.js";
export {
  LlmIntentExtractor,
  LlmProofDeriver,
  LlmGapReviewer,
  GAP_REVIEW_SYSTEM,
  GAP_REVIEW_SCHEMA,
  EXTRACT_SYSTEM,
  EXTRACT_SCHEMA,
  DERIVE_SYSTEM,
  DERIVE_SCHEMA,
  extractUserPrompt,
  deriveUserPrompt,
  artifactFromExtraction,
  planFromDerivation,
} from "./llm/llm.js";
export type { AskJson, ExtractOut, DeriveOut, Gap } from "./llm/llm.js";
export { createLastro, failuresOf, specGapsOf, agentFeedback } from "./lastro.js";
export type { Lastro, LastroOptions, Spec, Proof, ProofFailure } from "./lastro.js";
export type { IntentExtractor, ProofDeriver } from "./llm/interfaces.js";
export { loadRustProver, tabulate } from "./rust.js";
export type { RustProver, RustProofReport, RustClauseProof, Row } from "./rust.js";
export {
  CHANGE_KINDS,
  DEFAULT_POLICY,
  SLUG,
  ProposalError,
  ProposalReview,
  anchor,
  applyProposal,
  approveProposal,
  baselineOf,
  carryProposalReview,
  countedApprovals,
  fileNameOf,
  forwardIntents,
  isProposalApproved,
  liveChanges,
  nextId,
  passedProofs,
  pendingProposal,
  proofsFromDerivation,
  proposalFeedback,
  proposalFromExtraction,
  reservedUseCaseIds,
  proveProposal,
  readPolicy,
  regress,
  renderProposalReceipt,
  reviewProposal,
  runProofs,
  supersedesOf,
  withStatus,
} from "./proposal.js";
export type {
  Applied,
  ChangeKind,
  DeriveV1Out,
  ProofCheck,
  ProofEntry,
  ProofOutcome,
  ProofReplacement,
  ProofSetup,
  Proposal,
  ProposalDecision,
  ProposalIntent,
  ProposalIntentStatus,
  ProposalPolicy,
  ProposalRun,
  ProposalStatus,
  ProposeInput,
  ProposeOut,
  RunOptions,
  Thread,
  ThreadComment,
  UseCase,
  UseCaseChange,
} from "./proposal.js";
export {
  PROPOSE_SYSTEM,
  PROPOSE_SCHEMA,
  RECOMBINE_SYSTEM,
  RECOMBINE_SCHEMA,
  DERIVE_PROOFS_SYSTEM,
  DERIVE_PROOFS_SCHEMA,
  DERIVE_USECASE_PROOFS_SYSTEM,
  DERIVE_USECASE_PROOFS_SCHEMA,
  deriveUseCaseProofsUserPrompt,
  deriveUseCaseProofs,
  proposeUserPrompt,
  recombineUserPrompt,
  deriveProofsUserPrompt,
  extractProposal,
  recombineAfter,
  deriveProofs,
} from "./llm/proposal.js";
// Proofs derived straight from the established use cases.
export { useCaseTargets, proofsFromUseCases, proveUseCases, isUseCaseList, USECASE_FEEDBACK_NOTE } from "./usecase-proofs.js";
export type { UseCaseDeriveOut, UseCaseDerivation, UseCaseRun } from "./usecase-proofs.js";
// Bootstrap (pure). Reading the repository from disk is Node-only: import it from "lastro/scan".
export { proposalFromSurvey, renderSurveyReport, modelProblems, digestHash, isBootstrap, setupOf, suggestScopes, BOOTSTRAP_FEEDBACK_NOTE } from "./bootstrap.js";
export type { RepoDigest, RepoFile, RepoTest, RepoCommit, RepoManifest, RepoArea, ProposalOrigin, Grounds, ProposalSetup, SubjectDecl, SurveyOut, SurveyInput } from "./bootstrap.js";
export { SURVEY_SYSTEM, SURVEY_SCHEMA, surveyUserPrompt, surveyProposal } from "./llm/bootstrap.js";
export type { Excerpt } from "./llm/bootstrap.js";
