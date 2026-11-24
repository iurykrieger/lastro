/**
 * Driving the application through its surfaces, over the protocols the market already uses (Node only).
 *
 *   screen / route  W3C WebDriver with any WebDriver server (chromedriver, geckodriver, Electron's), or Appium for
 *                   Android and iOS (the same protocol with appium:* capabilities)
 *   api             HTTP, called directly
 *   command         a process
 *   any kind        Lastro's own JSON lines protocol, only when no standard protocol fits
 *
 * `.lastro/config.json` names them under `drivers`. Lastro ships no browser, device or Appium server. A driver that
 * fails to start or times out (30 s per action by default) fails the scenario with the driver's error, never passes it.
 */
import { spawn, type ChildProcess } from "node:child_process";
import { createInterface } from "node:readline";
import { canonicalJson } from "./artifact.js";
import type { SurfaceKind } from "./features.js";
import { rng } from "./spec/domain.js";
import { pixelShare } from "./png.js";
import type { Env } from "./types.js";
import { exploreMachine, type MachineClause, type MachineModel, type MachineStep } from "./machine.js";

export type Json = null | boolean | number | string | Json[] | { [k: string]: Json };

export type SurfaceDriver =
  | { webdriver: string; capabilities?: Record<string, Json>; baseUrl?: string }
  | { appium: string; capabilities?: Record<string, Json> }
  | { baseUrl: string; headers?: Record<string, string> }
  | { process: true; cwd?: string }
  | { jsonlines: string; cwd?: string };

export interface DriverConfig {
  screen?: SurfaceDriver;
  /** Falls back to `screen`. */
  route?: SurfaceDriver;
  api?: SurfaceDriver;
  command?: SurfaceDriver;
  /** Per action, in milliseconds. Default 30,000. */
  timeoutMs?: number;
  /** A command that prints the structure of the application's data as JSON: { "tables": {...} } or a JSON Schema. */
  data?: { command: string; cwd?: string };
}

export type Protocol = "webdriver" | "appium" | "http" | "process" | "jsonlines" | "none";

export const DEFAULT_TIMEOUT_MS = 30_000;

/** The protocol a surface kind is driven over, from the configuration. */
export function protocolFor(kind: SurfaceKind, drivers: DriverConfig | undefined): Protocol {
  const d = (drivers?.[kind] ?? (kind === "route" ? drivers?.screen : undefined)) as Record<string, unknown> | undefined;
  if (!d) return "none";
  if (typeof d.jsonlines === "string") return "jsonlines";
  if (kind === "screen" || kind === "route") return typeof d.appium === "string" ? "appium" : typeof d.webdriver === "string" ? "webdriver" : "none";
  if (kind === "api") return typeof d.baseUrl === "string" ? "http" : "none";
  return d.process === true ? "process" : "none";
}

/** One action and what the driver observed after it. */
export type Action = Record<string, Json>;
export type Observation = Record<string, Json>;

export interface Session {
  act(action: Action): Promise<Observation>;
  /** A PNG of the screen, for screen and route surfaces. */
  screenshot(): Promise<Uint8Array | undefined>;
  close(): Promise<void>;
}

export class DriverError extends Error {}

function withTimeout<T>(p: Promise<T>, ms: number, what: string): Promise<T> {
  let timer: NodeJS.Timeout | undefined;
  return Promise.race([
    p.finally(() => clearTimeout(timer)),
    new Promise<T>((_, reject) => {
      timer = setTimeout(() => reject(new DriverError(`${what} timed out after ${ms} ms`)), ms);
    }),
  ]);
}

/** Open a session on a surface. Throws DriverError when no driver is configured or it fails to start. */
export async function openSession(kind: SurfaceKind, locator: string, drivers: DriverConfig | undefined, root = process.cwd()): Promise<Session> {
  const protocol = protocolFor(kind, drivers);
  const d = (drivers?.[kind] ?? drivers?.screen) as Record<string, unknown>;
  const ms = drivers?.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  const start = async (): Promise<Session> => {
    switch (protocol) {
      case "webdriver":
      case "appium":
        return webdriverSession(String(protocol === "appium" ? d.appium : d.webdriver), (d.capabilities as Record<string, Json>) ?? {}, typeof d.baseUrl === "string" ? d.baseUrl : undefined, locator, ms);
      case "http":
        return httpSession(String(d.baseUrl), (d.headers as Record<string, string>) ?? {});
      case "process":
        return processSession(typeof d.cwd === "string" ? d.cwd : root, locator, ms);
      case "jsonlines":
        return jsonLinesSession(String(d.jsonlines), typeof d.cwd === "string" ? d.cwd : root, kind, locator, ms);
      case "none":
        throw new DriverError(`no driver configured for ${kind} surfaces: add one under "drivers" in .lastro/config.json`);
    }
  };
  const starting = start();
  try {
    return await withTimeout(starting, ms, `starting the ${protocol} driver`);
  } catch (e) {
    // A session that opens after the timeout is closed, so its driver does not outlive the scenario.
    starting.then((s) => s.close()).catch(() => undefined);
    throw e;
  }
}

// ---------------------------------------------------------------------------
// W3C WebDriver (and Appium, which speaks the same protocol)
// ---------------------------------------------------------------------------

async function wd(server: string, method: string, path: string, body?: Json): Promise<Json> {
  const res = await fetch(`${server.replace(/\/$/, "")}${path}`, { method, headers: { "content-type": "application/json" }, ...(body !== undefined ? { body: JSON.stringify(body) } : {}) });
  const json = (await res.json().catch(() => ({}))) as { value?: Json };
  const value = json.value ?? null;
  if (!res.ok || (value && typeof value === "object" && !Array.isArray(value) && "error" in value))
    throw new DriverError(`WebDriver ${method} ${path}: ${(value as { message?: string })?.message ?? res.status}`);
  return value;
}

async function webdriverSession(server: string, capabilities: Record<string, Json>, baseUrl: string | undefined, locator: string, ms: number): Promise<Session> {
  const created = (await wd(server, "POST", "/session", { capabilities: { alwaysMatch: capabilities } })) as { sessionId: string };
  const s = `/session/${created.sessionId}`;
  const element = async (selector: string) => {
    const using = selector.startsWith("~") ? "accessibility id" : selector.startsWith("//") ? "xpath" : "css selector";
    const el = (await wd(server, "POST", `${s}/element`, { using, value: using === "accessibility id" ? selector.slice(1) : selector })) as Record<string, string>;
    return Object.values(el)[0]!;
  };
  const url = (to: string) => (baseUrl && to.startsWith("/") ? `${baseUrl.replace(/\/$/, "")}${to}` : to);
  if (baseUrl || /^https?:/.test(locator)) await withTimeout(wd(server, "POST", `${s}/url`, { url: url(locator) }), ms, "opening the surface");
  return {
    async act(a) {
      const run = async (): Promise<Observation> => {
        if (typeof a.navigate === "string") await wd(server, "POST", `${s}/url`, { url: url(a.navigate) });
        if (typeof a.click === "string") await wd(server, "POST", `${s}/element/${await element(a.click)}/click`, {});
        if (typeof a.type === "string") await wd(server, "POST", `${s}/element/${await element(a.type)}/value`, { text: String(a.text ?? "") });
        const obs: Observation = {};
        if (typeof a.read === "string") obs.text = (await wd(server, "GET", `${s}/element/${await element(a.read)}/text`)) as string;
        obs.url = (await wd(server, "GET", `${s}/url`).catch(() => null)) as Json;
        return obs;
      };
      return withTimeout(run(), ms, `action ${canonicalJson(a)}`);
    },
    async screenshot() {
      const b64 = (await withTimeout(wd(server, "GET", `${s}/screenshot`), ms, "taking a screenshot")) as string;
      return Uint8Array.from(Buffer.from(b64, "base64"));
    },
    async close() {
      await wd(server, "DELETE", s).catch(() => undefined);
    },
  };
}

// ---------------------------------------------------------------------------
// HTTP and processes
// ---------------------------------------------------------------------------

function httpSession(baseUrl: string, headers: Record<string, string>): Session {
  return {
    async act(a) {
      const method = String(a.method ?? "GET");
      const res = await fetch(`${baseUrl.replace(/\/$/, "")}${String(a.path ?? "/")}`, {
        method,
        headers: { ...headers, ...(a.body !== undefined ? { "content-type": "application/json" } : {}), ...((a.headers as Record<string, string>) ?? {}) },
        ...(a.body !== undefined ? { body: JSON.stringify(a.body) } : {}),
      });
      const text = await res.text();
      let body: Json = text;
      try {
        body = JSON.parse(text) as Json;
      } catch {
        /* not JSON: keep the text */
      }
      return { status: res.status, body };
    },
    screenshot: async () => undefined,
    close: async () => undefined,
  };
}

/** Spawn a shell command in its own process group, so that stopping it also stops what the shell started. */
function spawnGroup(command: string, cwd: string): ChildProcess {
  return spawn(command, { cwd, shell: true, stdio: ["pipe", "pipe", "pipe"], detached: process.platform !== "win32" });
}

/** Stop a process started by spawnGroup and everything it started. */
function stopGroup(child: ChildProcess): void {
  if (child.exitCode !== null || child.signalCode !== null) return;
  try {
    if (process.platform !== "win32" && child.pid) process.kill(-child.pid, "SIGKILL");
    else child.kill("SIGKILL");
  } catch {
    /* already gone */
  }
}

function processSession(cwd: string, locator: string, ms: number): Session {
  return {
    act(a) {
      const line = typeof a.run === "string" ? a.run : [locator, ...((a.args as string[]) ?? [])].join(" ");
      let child: ChildProcess | undefined;
      return withTimeout(
        new Promise<Observation>((resolve, reject) => {
          child = spawnGroup(line, cwd);
          let stdout = "";
          let stderr = "";
          child.stdout!.on("data", (d) => (stdout += d));
          child.stderr!.on("data", (d) => (stderr += d));
          child.on("error", (e) => reject(new DriverError(`could not run ${line}: ${e.message}`)));
          child.on("close", (code) => resolve({ exitCode: code ?? -1, stdout, stderr }));
          if (typeof a.stdin === "string") child.stdin!.end(a.stdin);
          else child.stdin!.end();
        }),
        ms,
        `running ${line}`,
      ).finally(() => child && stopGroup(child));
    },
    screenshot: async () => undefined,
    close: async () => undefined,
  };
}

// ---------------------------------------------------------------------------
// Lastro's JSON lines protocol: only when no standard protocol fits
// ---------------------------------------------------------------------------

/**
 * The driver is a process. Lastro writes one JSON object per line and reads one back:
 *   -> {"open": {"kind": "screen", "locator": "/chats"}}          <- {"ok": true}
 *   -> {"act": {...action}}                                       <- {"observed": {...}, "screenshot"?: "<base64 PNG>"}
 *   -> {"screenshot": true}                                       <- {"screenshot": "<base64 PNG>"}
 *   -> {"close": true}                                            <- {"ok": true}
 * Any answer with "error" fails the step with that error.
 */
async function jsonLinesSession(command: string, cwd: string, kind: SurfaceKind, locator: string, ms: number): Promise<Session> {
  const child = spawnGroup(command, cwd);
  const lines = createInterface({ input: child.stdout! });
  const waiting: ((l: string) => void)[] = [];
  const buffered: string[] = [];
  let exited: string | undefined;
  lines.on("line", (l) => (waiting.length ? waiting.shift()!(l) : buffered.push(l)));
  child.on("exit", (code) => (exited = `the driver exited with code ${code}`));
  child.on("error", (e) => (exited = `the driver could not start: ${e.message}`));
  const ask = async (msg: Json, what: string): Promise<Record<string, Json>> => {
    if (exited) throw new DriverError(exited);
    child.stdin!.write(JSON.stringify(msg) + "\n");
    const line = await withTimeout(new Promise<string>((resolve, reject) => {
      if (buffered.length) return resolve(buffered.shift()!);
      waiting.push(resolve);
      child.once("exit", () => reject(new DriverError(exited ?? "the driver exited")));
    }), ms, what);
    const out = JSON.parse(line) as Record<string, Json>;
    if (out.error) throw new DriverError(String(out.error));
    return out;
  };
  child.stdin!.on("error", () => undefined);
  try {
    await ask({ open: { kind, locator } }, "starting the JSON lines driver");
  } catch (e) {
    stopGroup(child);
    throw e;
  }
  let last: Uint8Array | undefined;
  return {
    async act(a) {
      const out = await ask({ act: a }, `action ${canonicalJson(a)}`);
      last = typeof out.screenshot === "string" ? Uint8Array.from(Buffer.from(out.screenshot, "base64")) : undefined;
      return (out.observed as Observation) ?? {};
    },
    async screenshot() {
      if (last) return last;
      const out = await ask({ screenshot: true }, "taking a screenshot");
      return typeof out.screenshot === "string" ? Uint8Array.from(Buffer.from(out.screenshot, "base64")) : undefined;
    },
    async close() {
      await ask({ close: true }, "closing the driver").catch(() => undefined);
      stopGroup(child);
    },
  };
}

// ---------------------------------------------------------------------------
// Driven scenarios
// ---------------------------------------------------------------------------

/** One step: an input (a user action, a request, a command) and the output expected from it. */
export interface ScenarioStep {
  input: Action;
  /** Every field given must equal the observed one (objects compared field by field, recursively). */
  expect: Observation;
  /** A reference screenshot (PNG bytes) the screen must match within the scenario's tolerance. */
  reference?: Uint8Array;
}

export interface DrivenScenario {
  id: string;
  kind: SurfaceKind;
  locator: string;
  steps: ScenarioStep[];
  /** The share of pixels (0..1) a screenshot may differ from its reference. Default 0. */
  tolerance?: number;
}

export interface StepResult {
  observed?: Observation;
  /** The screenshot taken after the step (screen and route surfaces). */
  screenshot?: Uint8Array;
  pixelShare?: number;
  passed: boolean;
  problem?: string;
}

export interface DrivenResult {
  scenarioId: string;
  passed: boolean;
  /** Set when the driver failed to start, timed out or broke: the scenario fails with it. */
  driverError?: string;
  steps: StepResult[];
  evidence: "empirical" | "none";
}

/** True when every field of `expected` equals the observed one, recursively. */
export function matches(expected: Json, observed: Json | undefined): boolean {
  if (expected === null || typeof expected !== "object") return expected === observed;
  if (observed === null || typeof observed !== "object") return false;
  if (Array.isArray(expected)) return Array.isArray(observed) && expected.length === observed.length && expected.every((x, i) => matches(x, observed[i]));
  return Object.entries(expected).every(([k, v]) => matches(v, (observed as Record<string, Json>)[k]));
}

/** Run a scenario through its surface's driver and check every step. */
export async function runDrivenScenario(s: DrivenScenario, drivers: DriverConfig | undefined, open: typeof openSession = openSession): Promise<DrivenResult> {
  const steps: StepResult[] = [];
  let session: Session | undefined;
  try {
    session = await open(s.kind, s.locator, drivers);
    for (const step of s.steps) {
      const observed = await session.act(step.input);
      const screen = s.kind === "screen" || s.kind === "route" ? await session.screenshot() : undefined;
      const r: StepResult = { observed, passed: matches(step.expect, observed), ...(screen ? { screenshot: screen } : {}) };
      if (!r.passed) r.problem = `expected ${canonicalJson(step.expect)}, observed ${canonicalJson(observed)}`;
      if (step.reference) {
        r.pixelShare = screen ? pixelShare(step.reference, screen) : 1;
        if (r.pixelShare > (s.tolerance ?? 0)) {
          r.passed = false;
          r.problem = [r.problem, `${(r.pixelShare * 100).toFixed(2)}% of pixels changed, over the ${((s.tolerance ?? 0) * 100).toFixed(2)}% tolerance`].filter(Boolean).join("; ");
        }
      }
      steps.push(r);
    }
  } catch (e) {
    return { scenarioId: s.id, passed: false, driverError: (e as Error).message, steps, evidence: "none" };
  } finally {
    await session?.close().catch(() => undefined);
  }
  const passed = steps.every((x) => x.passed);
  return { scenarioId: s.id, passed, steps, evidence: passed ? "empirical" : "none" };
}

// ---------------------------------------------------------------------------
// Properties over action sequences
// ---------------------------------------------------------------------------

export interface PropertyCheck {
  id: string;
  kind: SurfaceKind;
  locator: string;
  /** The actions a user or client can take there. */
  actions: Action[];
  /** Must hold on the observed state after every action. */
  invariant: (observed: Observation) => boolean;
  sequences?: number;
  length?: number;
  seed?: number;
}

export interface PropertyResult {
  checkId: string;
  verdict: "proved" | "evidenced" | "violated" | "nondeterministic";
  evidence: "formal" | "empirical" | "counterexample" | "none";
  note?: string;
  sequences: number;
  actions: number;
  /** Rule of three: the failure rate per action is below this with 95% confidence. */
  failureRateUpperBound95?: number;
  /** The shortest failing prefix of the first failing sequence. */
  prefix?: Action[];
  driverError?: string;
}

/** The action sequences a check runs: the same for the same seed. */
export function actionSequences(actions: Action[], sequences: number, length: number, seed: number): Action[][] {
  const next = rng(seed);
  return Array.from({ length: sequences }, () => Array.from({ length }, () => actions[Math.floor(next() * actions.length)]!));
}

/** The first k actions of a sequence that first fails at action k (1-based); empty when it never fails. */
export function shortestFailingPrefix(seq: Action[], failAt: number | undefined): Action[] {
  return failAt === undefined || failAt > seq.length ? [] : seq.slice(0, failAt);
}

/**
 * Run fixed-seed action sequences through the driver, checking the invariant after every action. A failing sequence is
 * cut to its shortest failing prefix and replayed once: violated if the replay fails again, nondeterministic otherwise.
 * A property that holds on every sequence is evidenced, never proved (a state-machine model proves it; see machine.ts).
 */
export async function checkProperty(c: PropertyCheck, drivers: DriverConfig | undefined, open: typeof openSession = openSession): Promise<PropertyResult> {
  const seqs = actionSequences(c.actions, c.sequences ?? 100, c.length ?? 10, c.seed ?? 42);
  let actions = 0;
  const runSeq = async (seq: Action[]): Promise<number | undefined> => {
    const s = await open(c.kind, c.locator, drivers);
    try {
      for (let k = 0; k < seq.length; k++) {
        actions++;
        if (!c.invariant(await s.act(seq[k]!))) return k + 1;
      }
      return undefined;
    } finally {
      await s.close().catch(() => undefined);
    }
  };
  try {
    for (const seq of seqs) {
      const failAt = await runSeq(seq);
      if (failAt === undefined) continue;
      const prefix = shortestFailingPrefix(seq, failAt);
      const again = await runSeq(prefix);
      return again !== undefined
        ? { checkId: c.id, verdict: "violated", evidence: "counterexample", sequences: seqs.length, actions, prefix }
        : { checkId: c.id, verdict: "nondeterministic", evidence: "none", sequences: seqs.length, actions, prefix };
    }
  } catch (e) {
    return { checkId: c.id, verdict: "violated", evidence: "none", sequences: seqs.length, actions, driverError: (e as Error).message };
  }
  return { checkId: c.id, verdict: "evidenced", evidence: "empirical", sequences: seqs.length, actions, failureRateUpperBound95: Math.min(1, 3 / Math.max(1, actions)) };
}

/**
 * A property whose surface has a state-machine model. It is first checked over the model (exhaustively, by BFS):
 * proved there, the action sequences then check that the running application follows the model, comparing the
 * observed state with the model's after every action. Proved when both hold; violated when the model violates it.
 */
export interface ModelledProperty extends Omit<PropertyCheck, "invariant" | "actions"> {
  model: MachineModel;
  step: MachineStep;
  /** The property, as an invariant over the model's state. */
  invariant: Extract<MachineClause, { kind: "invariant" }>;
  /** Each action with the model event it raises (actor 1). */
  actions: { action: Action; event: string; input?: Record<string, Json> }[];
}

export async function checkModelledProperty(c: ModelledProperty, drivers: DriverConfig | undefined, open: typeof openSession = openSession): Promise<PropertyResult> {
  const report = await exploreMachine(c.model, c.step, [c.invariant]);
  const r = report.clauses[0]!;
  if (r.verdict === "violated") return { checkId: c.id, verdict: "violated", evidence: "counterexample", sequences: 0, actions: 0, note: `violated over the model (${r.method})` };
  // Follow the model: after each action, every state variable the driver observes must equal the model's.
  const shared = Object.keys(c.model.state);
  const initial = () => Object.fromEntries(shared.map((k) => [k, c.model.initial[k] as Json])) as Record<string, Json>;
  const byAction = new Map(c.actions.map((a) => [canonicalJson(a.action), a]));
  const res = await checkProperty(
    {
      id: c.id,
      kind: c.kind,
      locator: c.locator,
      actions: c.actions.map((a) => a.action),
      ...(c.sequences !== undefined ? { sequences: c.sequences } : {}),
      ...(c.length !== undefined ? { length: c.length } : {}),
      ...(c.seed !== undefined ? { seed: c.seed } : {}),
      invariant: (o) => o[FOLLOWS] !== false,
    },
    drivers,
    async (kind, locator, d) => {
      const s = await open(kind, locator, d);
      let state = initial();
      return {
        async act(a) {
          const obs = await s.act(a);
          const ev = byAction.get(canonicalJson(a))!;
          const next = await c.step({ ...(state as Env), ...((ev.input ?? {}) as Env), actor: 1, event: ev.event });
          if (next) state = { ...state, ...(next as Record<string, Json>) };
          return { ...obs, [FOLLOWS]: shared.every((k) => !(k in obs) || canonicalJson(obs[k]!) === canonicalJson(state[k]!)) };
        },
        screenshot: () => s.screenshot(),
        close: () => s.close(),
      };
    },
  );
  if (res.verdict !== "evidenced") return { ...res, note: "the application does not follow the model" };
  if (r.verdict !== "proved") return { ...res, note: `the model was explored by ${r.method}: too large to prove` };
  return { ...res, verdict: "proved", evidence: "formal", note: `proved over the model (${report.states} states); ${res.sequences} driven sequences follow it` };
}

const FOLLOWS = "__followsModel";
