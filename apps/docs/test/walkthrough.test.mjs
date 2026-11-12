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
  const text = r.stdout.trim().split(cwd).join("/home/you/shop");
  return { code: r.status, body: JSON.parse(r.stdout), shown: JSON.parse(text) };
}

test("the quickstart runs as the docs show it", { skip: !existsSync(cli) && "build the TS SDK first: npm run build -w lastro" }, async () => {
  const { OUT, RECEIPT, USECASE, PROOF, STREAM } = await import("../src/content/outputs.ts");
  const rust = existsSync(wasm);
  // Without the WASM prover there is no cross-check, so only compare what does not depend on it.
  const same = (real, key) => {
    const strip = (o) => JSON.parse(JSON.stringify(o, (k, v) => (k === "enginesAgree" ? undefined : v)));
    if (rust) assert.deepEqual(real, JSON.parse(OUT[key]), `OUT.${key} matches the real output`);
    else assert.deepEqual(strip(real), strip(JSON.parse(OUT[key])), `OUT.${key} matches the real output`);
  };
  const dir = mkdtempSync(join(tmpdir(), "lastro-docs-"));
  for (const f of ["model.json", "prose.md", "shipping.mjs", "shipping-buggy.mjs"]) copyFileSync(join(example, f), join(dir, f));

  let r = lastro(dir, "init", "--model", "model.json");
  assert.equal(r.code, 0);
  same(r.shown, "init");

  // propose: the extraction is handed off to the calling agent (exit 3), which writes the answer and runs propose again.
  r = lastro(dir, "propose", "prose.md", "--as", "ana");
  assert.equal(r.code, 3);
  assert.equal(r.body.status, "needs-llm");
  assert.equal(r.body.step, "extract");
  same(r.shown, "proposeNeedsLlm");
  const request = JSON.parse(readFileSync(r.body.request, "utf8"));
  assert.ok(request.system && request.user && request.schema, "the request carries instructions, data and a JSON Schema");
  copyFileSync(join(example, "answers", "propose.json"), r.body.answer);

  r = lastro(dir, "propose", "prose.md", "--as", "ana");
  assert.equal(r.code, 1, "proposed, but the review is pending");
  same(r.shown, "proposed");
  assert.ok(existsSync(join(dir, ".lastro", "proposals", "P-001-shipping-fee.json")));

  r = lastro(dir, "review", "P-001", "--as", "ana", "answer", "D1", "1");
  assert.equal(r.code, 0);
  r = lastro(dir, "review", "P-001", "--as", "ana", "approve-all");
  assert.equal(r.code, 0);
  same(r.shown, "reviewed");

  r = lastro(dir, "derive", "P-001");
  assert.equal(r.code, 3);
  assert.equal(r.body.step, "derive");
  same(r.shown, "deriveNeedsLlm");
  copyFileSync(join(example, "answers", "proofs.json"), r.body.answer);
  r = lastro(dir, "derive", "P-001");
  assert.equal(r.code, 0);
  same(r.shown, "derived");

  // The buggy implementation: an order of exactly 100 still pays.
  r = lastro(dir, "prove", "P-001", "./shipping-buggy.mjs");
  assert.equal(r.code, 1);
  assert.equal(r.body.status, "not-proved");
  assert.deepEqual(r.body.failures.map((f) => f.proof), ["P-001.PF-01"]);
  assert.deepEqual(r.body.failures[0].counterexample, { input: { member: false, subtotal: 100 }, output: { fee: 15 } });
  same(r.shown, "notProved");

  // --stream: the same run, one event per line, as the CLI page shows it.
  const streamed = spawnSync(process.execPath, [cli, "prove", "P-001", "./shipping-buggy.mjs", "--stream"], { cwd: dir, encoding: "utf8", env: { ...process.env, ANTHROPIC_API_KEY: "" } });
  assert.equal(streamed.status, 1);
  const shown = STREAM.split("\n").map((l) => JSON.parse(l));
  const real = streamed.stdout.trim().split(dir).join("/home/you/shop").split("\n").map((l) => JSON.parse(l));
  if (rust) assert.deepEqual(real, shown);
  else assert.deepEqual(real.filter((e) => e.event !== "crosscheck").map((e) => e.event), shown.filter((e) => e.event !== "crosscheck").map((e) => e.event));

  r = lastro(dir, "feedback", "P-001");
  assert.equal(r.code, 1);
  same(r.shown, "feedback");

  r = lastro(dir, "prove", "P-001", "./shipping.mjs");
  assert.equal(r.code, 0);
  assert.equal(r.body.status, "proved");
  same(r.shown, "proved");
  const receipt = readFileSync(join(dir, ".lastro", "proposals", "P-001-shipping-fee.receipt.md"), "utf8").trim();
  if (rust) assert.equal(receipt, RECEIPT);

  r = lastro(dir, "apply", "P-001");
  assert.equal(r.code, 0);
  same(r.shown, "applied");
  assert.equal(readFileSync(join(dir, ".lastro", "usecases", "UC-001-charge-shipping.json"), "utf8").trim(), USECASE);
  assert.equal(readFileSync(join(dir, ".lastro", "proof", "P-001.PF-01-free-from-100.json"), "utf8").trim(), PROOF);

  r = lastro(dir, "status");
  same(r.shown, "status");
  r = lastro(dir, "regress", "./shipping.mjs");
  assert.equal(r.code, 0);
  same(r.shown, "regress");
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
