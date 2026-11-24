/**
 * What the review app shows for one intent: the feature's place in the feature map, the statechart of its
 * state-machine model, its before snapshots during review and, after the proof run, the before and after snapshots
 * with their comparison and every proof's verdict with its counterexample. Pure: the app's server calls it.
 */
import { affectedBy, featureMap, type MapEntry } from "./features.js";
import type { Proposal, UseCase } from "./proposal.js";
import type { Comparison, SnapshotKind, SnapshotRecord } from "./snapshots.js";

export interface IntentEvidence {
  intent: string;
  feature: string;
  /** The feature in the map after the proposal, with the features it affects; null when the map is invalid. */
  map: { entry: MapEntry; affected: string[] } | null;
  mapError?: string;
  /** Mermaid statecharts of the state-machine models the intent's proofs use. */
  statecharts: { model: string; mermaid: string }[];
  before: SnapshotRecord[];
  /** Empty until the proof run takes them. */
  after: SnapshotRecord[];
  /** Once the after snapshots are taken: images side by side for screens, a diff for contracts and data, the changed rows for examples. */
  comparisons: (Comparison & { view: "images" | "diff" | "rows" })[];
  verdicts: { proof: string; verdict: string; passed: boolean; counterexample?: { input: unknown; output: unknown } }[];
}

export const comparisonView = (kind: SnapshotKind): "images" | "diff" | "rows" => (kind === "screen" ? "images" : kind === "examples" ? "rows" : "diff");

export function intentEvidence(p: Proposal, intentId: string, useCases: UseCase[]): IntentEvidence {
  const it = p.intents.find((i) => i.id === intentId || i.id === `${p.id}.${intentId}`);
  if (!it) throw new Error(`Unknown intent ${intentId}`);
  const live = p.useCases;
  const features = [
    ...useCases.map((u) => {
      const c = live.find((x) => x.useCase === u.id && x.change === "modify");
      return c ? { ...u, surfaces: c.surfaces, dependsOn: c.dependsOn } : u;
    }),
    ...live.filter((c) => c.change === "add").map((c) => ({ id: c.useCase, slug: c.slug, name: c.name, surfaces: c.surfaces, dependsOn: c.dependsOn })),
  ];
  let map: IntentEvidence["map"] = null;
  let mapError: string | undefined;
  try {
    const m = featureMap(features, live.filter((c) => c.change === "remove").map((c) => c.useCase));
    const entry = m.features.find((f) => f.id === it.useCase);
    if (entry) map = { entry, affected: affectedBy(m, [it.useCase]) };
    else mapError = `${it.useCase} is removed by ${p.id}`;
  } catch (e) {
    mapError = (e as Error).message;
  }
  const models = [...new Set(p.proofs.filter((x) => x.intent === it.id && x.check.kind === "machine").map((x) => x.model))].sort();
  const mine = (s: SnapshotRecord) => s.intent === it.id;
  const run = p.lastRun;
  return {
    intent: it.id,
    feature: it.useCase,
    map,
    ...(mapError ? { mapError } : {}),
    statecharts: models.filter((m) => p.statecharts?.[m]).map((m) => ({ model: m, mermaid: p.statecharts![m]! })),
    before: (p.snapshots ?? []).filter((s) => mine(s) && s.side === "before"),
    after: (p.snapshots ?? []).filter((s) => mine(s) && s.side === "after"),
    comparisons: (p.comparisons ?? []).filter((c) => c.intent === it.id).map((c) => ({ ...c, view: comparisonView(c.kind) })),
    verdicts: run
      ? run.outcomes
          .filter((o) => o.role === "proposal" && o.intent === it.id)
          .map((o) => ({ proof: o.proof, verdict: o.verdict, passed: o.passed, ...(o.verdict === "violated" && o.counterexample ? { counterexample: o.counterexample } : {}) }))
      : [],
  };
}
