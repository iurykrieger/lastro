// Runs the docs' quickstart through the real `lastro` CLI, so the commands, exit codes and outputs the docs
// show stay true. Needs the TypeScript SDK built (`npm run build -w lastro`); the Rust cross-check runs when
// the WASM prover was built (`npm run wasm`).
import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { copyFileSync, existsSync, mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const example = join(here, "..", "examples", "shipping");
const cli = join(here, "..", "..", "..", "sdks", "typescript", "dist", "src", "cli.js");
const wasm = join(here, "..", "..", "..", "sdks", "typescript", "wasm", "lastro_core.wasm");

function lastro(cwd, ...args) {
  const env = { ...process.env };
  delete env.ANTHROPIC_API_KEY; // the walkthrough uses the handoff, never the API
  const r = spawnSync(process.execPath, [cli, ...args], { cwd, env, encoding: "utf8" });
  return { code: r.status, body: JSON.parse(r.stdout) };
}

test("the quickstart runs as the docs show it", { skip: !existsSync(cli) && "build the TS SDK first: npm run build -w lastro" }, async () => {
  const dir = mkdtempSync(join(tmpdir(), "lastro-docs-"));
  for (const f of ["model.json", "prose.md", "shipping.mjs", "shipping-buggy.mjs"]) copyFileSync(join(example, f), join(dir, f));

  let r = lastro(dir, "init", "--model", "model.json");
  assert.equal(r.code, 0);
  assert.equal(r.body.status, "initialized");
  assert.equal(r.body.requiredApprovals, 1);

  // plan: the extraction is handed off to the calling agent (exit 3), which writes the answer and runs plan again.
  r = lastro(dir, "plan", "prose.md");
  assert.equal(r.code, 3);
  assert.equal(r.body.status, "needs-llm");
  assert.equal(r.body.step, "extract");
  assert.match(r.body.request, /extract-6e77fe54fe3e\.request\.json$/, "the request id the docs show");
  const request = JSON.parse(readFileSync(r.body.request, "utf8"));
  assert.ok(request.system && request.user && request.schema, "the request carries instructions, data and a JSON Schema");
  copyFileSync(join(example, "answers", "extract.json"), r.body.answer);

  r = lastro(dir, "plan", "prose.md");
  assert.equal(r.code, 1, "planned, but the review is pending");
  assert.deepEqual(r.body.intents.map((i) => i.id), ["I1", "I2", "I3", "I4"]);
  assert.equal(r.body.decisions[0].id, "D1");
  assert.equal(r.body.ready, false);

  r = lastro(dir, "review", "--as", "ana", "answer", "D1", "1");
  assert.equal(r.code, 0);
  assert.equal(r.body.decisions[0].chosen, 1);

  r = lastro(dir, "review", "--as", "ana", "approve-all");
  assert.equal(r.code, 0);
  assert.equal(r.body.ready, true);
  assert.deepEqual(r.body.intents[0].approvals, ["ana"]);

  r = lastro(dir, "spec");
  assert.equal(r.code, 3);
  assert.equal(r.body.step, "derive");
  assert.match(r.body.request, /derive-dca5b7ce966e\.request\.json$/);
  copyFileSync(join(example, "answers", "derive.json"), r.body.answer);

  r = lastro(dir, "spec");
  assert.equal(r.code, 0);
  assert.equal(r.body.status, "spec");
  assert.deepEqual(r.body.clauses.map((c) => c.id), ["C1", "C2", "C3"]);
  assert.equal(r.body.beyondProof[0].intentId, "I4");

  // The buggy implementation: an order of exactly 100 still pays.
  r = lastro(dir, "prove", "./shipping-buggy.mjs");
  assert.equal(r.code, 1);
  assert.equal(r.body.status, "not-proved");
  assert.equal(r.body.failures.length, 1);
  assert.equal(r.body.failures[0].id, "C1");
  assert.deepEqual(r.body.failures[0].input, { member: false, subtotal: 100 });
  assert.deepEqual(r.body.failures[0].output, { fee: 15 });
  if (existsSync(wasm)) assert.equal(r.body.enginesAgree, true);

  // --stream: the same run, one event per line, as the CLI page shows it.
  const streamed = spawnSync(process.execPath, [cli, "prove", "./shipping-buggy.mjs", "--stream"], { cwd: dir, encoding: "utf8", env: { ...process.env, ANTHROPIC_API_KEY: "" } });
  assert.equal(streamed.status, 1);
  const { STREAM } = await import("../src/content/outputs.ts");
  const shown = STREAM.split("\n").map((l) => JSON.parse(l));
  const real = streamed.stdout.trim().split(dir).join("/home/you/shop").split("\n").map((l) => JSON.parse(l));
  if (existsSync(wasm)) assert.deepEqual(real, shown);
  else assert.deepEqual(real.filter((e) => e.event !== "crosscheck").map((e) => e.event), shown.filter((e) => e.event !== "crosscheck").map((e) => e.event));

  r = lastro(dir, "feedback");
  assert.equal(r.code, 1);
  assert.match(r.body.feedback, /counterexample: \{"member":false,"subtotal":100\} returned \{"fee":15\}/);

  r = lastro(dir, "prove", "./shipping.mjs");
  assert.equal(r.code, 0);
  assert.equal(r.body.status, "proved");
  assert.equal(r.body.feedback, "Every proof passes.");
  const receipt = readFileSync(join(dir, ".lastro", "receipt.md"), "utf8");
  assert.match(receipt, /Proved: 3 · Under assumptions: 0 · Evidenced: 0 · Beyond proof: 1/);
  if (existsSync(wasm)) assert.match(receipt, /coverage 402\/402 inputs/);
});

test("the library example proves the same thing, with the same hashes", { skip: !existsSync(cli) && "build the TS SDK first: npm run build -w lastro" }, async () => {
  const { proveShipping } = await import("../examples/shipping/library.ts");
  const ask = async (req) => JSON.parse(readFileSync(join(example, "answers", `${req.step}.json`), "utf8"));
  const proof = await proveShipping(ask);
  assert.equal(proof.ok, true);
  assert.equal(proof.report.artifactHash, "sha256:c23c532655c4156f8854e1ce9795c7212de01c2973dc7f7341bbf1d02299129e");
  assert.match(proof.receipt, /Plan: `sha256:e68a4212f25b…`/);

  const buggy = (await import("../examples/shipping/shipping-buggy.mjs")).default;
  const failed = await proveShipping(ask, buggy);
  assert.equal(failed.ok, false);
  assert.deepEqual(failed.failures.map((f) => f.id), ["C1"]);
});
