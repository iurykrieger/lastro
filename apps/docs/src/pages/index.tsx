import type { ComponentType } from "react";
import { Installation, Introduction, Quickstart } from "./start";
import { ExampleBugs, ExampleLogin, ExampleReview, ExampleShipping, ExampleTrust, ExamplesOverview } from "./examples";
import { ConceptDecisions, ConceptIntents, ConceptPipeline, ConceptReceipts, ConceptReview, ConceptSpec, ConceptVerdicts } from "./concepts";
import { CliLlm, CliOverview, CliWorkspace } from "./cli";
import { PluginAgents } from "./plugins";
import { SdkDotnet, SdkGo, SdkInterop, SdkTypeScript } from "./sdks";
import { RefDataModel, RefLsl, RefRust } from "./reference";

/** Slug → page. Every slug in content/nav.ts must have a page here (the e2e tests visit them all). */
export const PAGE_COMPONENTS: Record<string, ComponentType> = {
  introduction: Introduction,
  installation: Installation,
  quickstart: Quickstart,
  "examples/overview": ExamplesOverview,
  "examples/shipping": ExampleShipping,
  "examples/login": ExampleLogin,
  "examples/bugs": ExampleBugs,
  "examples/review": ExampleReview,
  "examples/trust": ExampleTrust,
  "concepts/pipeline": ConceptPipeline,
  "concepts/intents": ConceptIntents,
  "concepts/review": ConceptReview,
  "concepts/decisions": ConceptDecisions,
  "concepts/spec": ConceptSpec,
  "concepts/verdicts": ConceptVerdicts,
  "concepts/receipts": ConceptReceipts,
  "cli/overview": CliOverview,
  "cli/workspace": CliWorkspace,
  "cli/llm": CliLlm,
  "plugins/agents": PluginAgents,
  "sdks/typescript": SdkTypeScript,
  "sdks/go": SdkGo,
  "sdks/dotnet": SdkDotnet,
  "sdks/interop": SdkInterop,
  "reference/lsl": RefLsl,
  "reference/data-model": RefDataModel,
  "reference/rust-prover": RefRust,
};
