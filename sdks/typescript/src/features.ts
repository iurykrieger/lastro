/**
 * The feature map: how a user or client reaches each feature (its surfaces) and which features it needs (dependsOn).
 *
 * A surface has a kind (screen, route, command or api), a name in the user's words and a locator the driver
 * understands (a URL path, a CLI command, an HTTP method and path, a selector). Both lists are optional on a feature
 * file, so files written before them stay valid. The map is deterministic: features in id order, every list sorted.
 */
import type { UseCase } from "./proposal.js";

export type SurfaceKind = "screen" | "route" | "command" | "api";
export const SURFACE_KINDS: readonly SurfaceKind[] = ["screen", "route", "command", "api"];

export interface Surface {
  kind: SurfaceKind;
  /** In the user's words: "the left sidebar's chat list". */
  name: string;
  /** What the driver understands: "/chats", "lastro map", "GET /api/chats", "[data-test=chat-list]". */
  locator: string;
}

/** The surfaces and dependencies a feature may carry. */
export interface FeatureLinks {
  surfaces?: Surface[];
  dependsOn?: string[];
}

export class FeatureMapError extends Error {
  constructor(
    message: string,
    /** The features the error is about (for a cycle, every feature in it, in cycle order). */
    readonly features: string[] = [],
  ) {
    super(message);
  }
}

/** Problems with one feature's surfaces; empty when they are valid. */
export function surfaceProblems(f: { id: string } & FeatureLinks): string[] {
  const errs: string[] = [];
  (f.surfaces ?? []).forEach((s, i) => {
    const at = `${f.id}.surfaces[${i}]`;
    if (!SURFACE_KINDS.includes(s?.kind)) errs.push(`${at}: kind must be one of ${SURFACE_KINDS.join(", ")}`);
    if (typeof s?.name !== "string" || !s.name.trim()) errs.push(`${at}: name is required`);
    if (typeof s?.locator !== "string" || !s.locator.trim()) errs.push(`${at}: locator is required`);
  });
  if (f.dependsOn !== undefined && (!Array.isArray(f.dependsOn) || f.dependsOn.some((d) => typeof d !== "string")))
    errs.push(`${f.id}.dependsOn: must be a list of feature ids`);
  return errs;
}

export interface MapEntry {
  id: string;
  slug: string;
  name: string;
  surfaces: Surface[];
  /** False when the feature has no surface: the driver cannot reach it. */
  reachable: boolean;
  dependsOn: string[];
  /** The features that depend on this one directly. */
  dependents: string[];
}

export interface FeatureMap {
  features: MapEntry[];
}

type Linked = Pick<UseCase, "id" | "slug" | "name"> & FeatureLinks;

const byId = (a: string, b: string) => (a < b ? -1 : a > b ? 1 : 0);

/**
 * Build the map. Throws FeatureMapError when a surface is malformed, when a feature depends on a feature that does
 * not exist (or that `removed` lists), or when the dependencies form a cycle, naming the features in it.
 */
export function featureMap(features: Linked[], removed: string[] = []): FeatureMap {
  const problems = features.flatMap(surfaceProblems);
  if (problems.length) throw new FeatureMapError(problems.join("; "));
  const gone = new Set(removed);
  const live = features.filter((f) => !gone.has(f.id)).sort((a, b) => byId(a.id, b.id));
  const ids = new Set(live.map((f) => f.id));
  for (const f of live)
    for (const d of [...(f.dependsOn ?? [])].sort(byId)) {
      if (gone.has(d)) throw new FeatureMapError(`${f.id} depends on ${d}, which is removed`, [f.id, d]);
      if (!ids.has(d)) throw new FeatureMapError(`${f.id} depends on ${d}, which does not exist`, [f.id, d]);
    }
  const deps = new Map(live.map((f) => [f.id, [...new Set(f.dependsOn ?? [])].sort(byId)]));
  const cycle = findCycle(live.map((f) => f.id), deps);
  if (cycle) throw new FeatureMapError(`dependency cycle: ${[...cycle, cycle[0]].join(" -> ")}`, cycle);
  return {
    features: live.map((f) => ({
      id: f.id,
      slug: f.slug,
      name: f.name,
      surfaces: [...(f.surfaces ?? [])].sort((a, b) => byId(a.kind + "\0" + a.locator, b.kind + "\0" + b.locator)),
      reachable: (f.surfaces ?? []).length > 0,
      dependsOn: deps.get(f.id)!,
      dependents: live.filter((g) => deps.get(g.id)!.includes(f.id)).map((g) => g.id),
    })),
  };
}

/** The first cycle found by a depth-first search in id order, starting at its smallest id. */
function findCycle(ids: string[], deps: Map<string, string[]>): string[] | undefined {
  const state = new Map<string, 1 | 2>();
  const stack: string[] = [];
  const visit = (id: string): string[] | undefined => {
    state.set(id, 1);
    stack.push(id);
    for (const d of deps.get(id) ?? []) {
      if (state.get(d) === 1) {
        const loop = stack.slice(stack.indexOf(d));
        const k = loop.indexOf([...loop].sort(byId)[0]!);
        return [...loop.slice(k), ...loop.slice(0, k)];
      }
      if (!state.has(d)) {
        const c = visit(d);
        if (c) return c;
      }
    }
    stack.pop();
    state.set(id, 2);
    return undefined;
  };
  for (const id of ids)
    if (!state.has(id)) {
      const c = visit(id);
      if (c) return c;
    }
  return undefined;
}

/** Every feature that depends, directly or through others, on one of `changed`; the changed ones are not listed. */
export function affectedBy(map: FeatureMap, changed: string[]): string[] {
  const dependents = new Map(map.features.map((f) => [f.id, f.dependents]));
  const seen = new Set<string>();
  const queue = [...changed].sort(byId);
  while (queue.length) {
    for (const d of dependents.get(queue.shift()!) ?? [])
      if (!seen.has(d)) {
        seen.add(d);
        queue.push(d);
      }
  }
  for (const c of changed) seen.delete(c);
  return [...seen].sort(byId);
}

/** The map as text, the output of `lastro map`. */
export function renderFeatureMap(map: FeatureMap, affected: string[] = []): string {
  const lines: string[] = [];
  for (const f of map.features) {
    lines.push(`${f.id} ${f.name}${affected.includes(f.id) ? "  (affected)" : ""}`);
    if (!f.reachable) lines.push("  not reachable by the driver: no surface");
    for (const s of f.surfaces) lines.push(`  ${s.kind} ${s.locator}: ${s.name}`);
    if (f.dependsOn.length) lines.push(`  depends on ${f.dependsOn.join(", ")}`);
    if (f.dependents.length) lines.push(`  needed by ${f.dependents.join(", ")}`);
  }
  return lines.join("\n") + "\n";
}

/**
 * The surfaces and dependencies of a modified feature: the ones it has, plus the ones added, minus the ones removed.
 * Removing one it does not have is refused, so a modify can never drop the surfaces it did not mention.
 */
export function mergeLinks(existing: FeatureLinks, change: { addSurfaces?: Surface[]; removeSurfaces?: string[]; addDependsOn?: string[]; removeDependsOn?: string[] }): FeatureLinks {
  const surfaces = [...(existing.surfaces ?? [])];
  for (const loc of change.removeSurfaces ?? []) {
    const k = surfaces.findIndex((s) => s.locator === loc);
    if (k < 0) throw new FeatureMapError(`cannot remove surface ${loc}: the feature does not have it`);
    surfaces.splice(k, 1);
  }
  for (const s of change.addSurfaces ?? []) if (!surfaces.some((x) => x.locator === s.locator)) surfaces.push(s);
  const deps = [...(existing.dependsOn ?? [])];
  for (const d of change.removeDependsOn ?? []) {
    const k = deps.indexOf(d);
    if (k < 0) throw new FeatureMapError(`cannot remove dependency on ${d}: the feature does not have it`);
    deps.splice(k, 1);
  }
  for (const d of change.addDependsOn ?? []) if (!deps.includes(d)) deps.push(d);
  return { surfaces, dependsOn: deps };
}
