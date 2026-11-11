// The quickstart as a library call: the same pipeline as the CLI, in one function.
// Run it with Claude: ANTHROPIC_API_KEY=... node library.ts
import { existsSync, readFileSync } from "node:fs";
import { agentFeedback, createLastro, loadRustProver, type AskJson, type Model, type Subject } from "lastro";
import shippingFee from "./shipping.mjs";

const read = (file: string) => readFileSync(new URL(file, import.meta.url), "utf8");

export async function proveShipping(ask: AskJson, subject: Subject = shippingFee) {
  const model = JSON.parse(read("./model.json")) as Model;
  const prose = read("./prose.md").trim();
  // The Rust prover ships next to the package (npm run wasm). Optional, recommended in CI.
  const wasm = new URL("../../wasm/lastro_core.wasm", import.meta.resolve("lastro"));
  const rust = existsSync(wasm) ? await loadRustProver(readFileSync(wasm)) : undefined;

  const lastro = createLastro({ model, ask, requiredApprovals: 1, rust });

  let intents = await lastro.plan(prose); // AI: extract intents
  intents = lastro
    .review(intents, "ana")
    .answer("D1", 1) // "After discounts"
    .approveAll()
    .done();
  if (lastro.pending(intents).length) throw new Error(`review pending: ${lastro.pending(intents).join("; ")}`);

  const spec = await lastro.spec(intents); // AI: derive the formal spec, then validate it
  if (!spec.ok) throw new Error("the spec contradicts itself: answer lastro.returnDecisions(spec)");

  const proof = await lastro.prove(spec, subject); // deterministic, in both engines
  if (!proof.ok) console.error(agentFeedback(proof));
  return proof;
}

if (import.meta.main) {
  const { claudeAsk } = await import("lastro/claude");
  const proof = await proveShipping(claudeAsk());
  console.log(proof.receipt);
  process.exitCode = proof.ok ? 0 : 1;
}
