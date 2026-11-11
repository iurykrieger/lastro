import type { IntentArtifact, Model, ProofPlan } from "../types.js";

/** prose -> proposed intents (non-deterministic; reviewed by a human afterwards). */
export interface IntentExtractor {
  extract(prose: string, context?: string): Promise<IntentArtifact>;
}

/** approved intents + model vocabulary -> proof plan (non-deterministic; checked by validatePlan). */
export interface ProofDeriver {
  derive(artifact: IntentArtifact, model: Model): Promise<ProofPlan>;
}

/** Deterministic stand-ins for tests, examples and offline use. */
export class StaticExtractor implements IntentExtractor {
  constructor(private readonly build: (prose: string) => IntentArtifact) {}
  async extract(prose: string): Promise<IntentArtifact> {
    return this.build(prose);
  }
}

export class StaticDeriver implements ProofDeriver {
  constructor(private readonly build: (artifact: IntentArtifact, model: Model) => ProofPlan) {}
  async derive(artifact: IntentArtifact, model: Model): Promise<ProofPlan> {
    return this.build(artifact, model);
  }
}
