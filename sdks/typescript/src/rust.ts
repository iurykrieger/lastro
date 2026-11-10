/**
 * Bridge to the Rust proof core (`crates/lastro-core`), compiled to WASM.
 *
 * The TypeScript engine and the Rust prover are independent implementations
 * of the same semantics. The Rust side does not trust the caller: it
 * enumerates the declared input space itself and only says `proved` when the
 * implementation's table covers every point of it.
 *
 * Usage (Node):   const prover = await loadRustProver(readFileSync("lastro_core.wasm"))
 * Usage (browser): const prover = await loadRustProver(await (await fetch(url)).arrayBuffer())
 */
import { enumerateSpace, spaceSize } from "./spec/domain.js";
import type { Env, Model, ProofPlan, Subject } from "./types.js";

export interface RustClauseProof {
  clauseId: string;
  verdict: "proved" | "evidenced" | "violated" | "vacuous" | "weak";
  evidence: "formal" | "empirical" | "none";
  checked: number;
  counterexample?: { input: Env; output: Env };
  failureRateUpperBound95?: number;
  mutants: { total: number; killed: number };
  error?: string;
}

export interface RustProofReport {
  engine: string;
  model: string;
  coverage: {
    space: number | null;
    rows: number;
    complete: boolean;
    missing: number;
    firstMissing?: Env;
    outOfDomain: number;
    nondeterministic: number;
  };
  clauses: RustClauseProof[];
  contradictions: { input: Env; clauses: string[] }[];
  tableDigest: string;
  ok: boolean;
}

export interface Row {
  input: Env;
  output: Env;
}

export interface RustProver {
  prove(plan: Pick<ProofPlan, "model" | "clauses">, table: Row[]): RustProofReport;
}

interface Exports {
  memory: WebAssembly.Memory;
  lastro_alloc(len: number): number;
  lastro_free(ptr: number, len: number): void;
  lastro_prove(ptr: number, len: number): bigint;
}

export async function loadRustProver(wasm: BufferSource | WebAssembly.Module): Promise<RustProver> {
  const instance =
    wasm instanceof WebAssembly.Module
      ? await WebAssembly.instantiate(wasm, {})
      : (await WebAssembly.instantiate(wasm as BufferSource, {})).instance;
  const ex = instance.exports as unknown as Exports;
  const enc = new TextEncoder();
  const dec = new TextDecoder();

  return {
    prove(plan, table) {
      const request = JSON.stringify({ model: { ...plan.model, defs: plan.model.defs ?? {} }, clauses: plan.clauses, table });
      const bytes = enc.encode(request);
      const ptr = ex.lastro_alloc(bytes.length);
      new Uint8Array(ex.memory.buffer, ptr, bytes.length).set(bytes);
      const packed = ex.lastro_prove(ptr, bytes.length);
      ex.lastro_free(ptr, bytes.length);
      const outPtr = Number(packed >> 32n);
      const outLen = Number(packed & 0xffffffffn);
      const json = dec.decode(new Uint8Array(ex.memory.buffer, outPtr, outLen).slice());
      ex.lastro_free(outPtr, outLen);
      const parsed = JSON.parse(json) as RustProofReport | { error: string };
      if ("error" in parsed) throw new Error(`lastro-core: ${parsed.error}`);
      return parsed;
    },
  };
}

/** Run the implementation over the whole (finite) input space: the table the Rust prover checks. */
export async function tabulate(model: Model, subject: Subject, limit = 100_000): Promise<Row[]> {
  const size = spaceSize(model.inputs);
  if (size > limit) throw new Error(`input space has ${size} points (limit ${limit}); use sampling instead`);
  const rows: Row[] = [];
  for (const input of enumerateSpace(model.inputs)) rows.push({ input, output: await subject({ ...input }) });
  return rows;
}
