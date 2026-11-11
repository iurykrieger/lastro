// Runs the docs' "Existing repositories" page through the real `lastro` CLI: scan, bootstrap (the survey answered
// from examples/shipping/answers/bootstrap.json), review, derive, prove against the code as it is, apply.
// Needs the TypeScript SDK built (`npm run build -w lastro`).
import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { copyFileSync, existsSync, mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const example = join(here, "..", "examples", "shipping");
const cli = join(here, "..", "..", "..", "sdks", "typescript", "dist", "src", "cli.js");
const page = readFileSync(join(here, "..", "src", "pages", "bootstrap.tsx"), "utf8");

function lastro(cwd, ...args) {
  const env = { ...process.env };
  delete env.ANTHROPIC_API_KEY;
  const r = spawnSync(process.execPath, [cli, ...args, "--llm", "handoff"], { cwd, env, encoding: "utf8" });
  return { code: r.status, body: JSON.parse(r.stdout) };
}

test("the existing-repository page runs as the docs show it", { skip: !existsSync(cli) && "build the TS SDK first: npm run build -w lastro" }, () => {
  const dir = mkdtempSync(join(tmpdir(), "lastro-docs-bootstrap-"));
  for (const f of ["model.json", "shipping.mjs", "shipping-buggy.mjs"]) copyFileSync(join(example, f), join(dir, f));
  const git = (...a) => execFileSync("git", ["-C", dir, "-c", "user.name=Docs", "-c", "user.email=docs@example.com", "-c", "commit.gpgsign=false", ...a], { encoding: "utf8" });
  git("init", "-q", "-b", "main");
  git("add", "-A");
  git("commit", "-q", "-m", "feat: shipping fee");

  let r = lastro(dir, "scan", "--max-commits", "400");
  assert.equal(r.code, 0);
  assert.deepEqual(r.body.read, { files: 3, tests: 0, commits: 1 });

  r = lastro(dir, "bootstrap", "--as", "ana");
  assert.equal(r.code, 3);
  assert.equal(r.body.step, "survey");
  copyFileSync(join(example, "answers", "bootstrap.json"), r.body.answer);
  r = lastro(dir, "bootstrap", "--as", "ana");
  assert.equal(r.code, 1, JSON.stringify(r.body));
  assert.equal(r.body.origin.kind, "bootstrap");
  assert.deepEqual(r.body.intents[0].grounds, { files: ["shipping.mjs"], tests: [], commits: [], confidence: "medium" });

  // The report the page shows is the real one, up to the commit hash.
  const report = readFileSync(join(dir, ".lastro", "proposals", "P-001-bootstrap-checkout.md"), "utf8").replace(/commit [0-9a-f]{12}/, "commit c0d1f9a7534d");
  const shown = page.match(/const REPORT = `([\s\S]*?)`;/)[1].replace(/\\`/g, "`");
  assert.equal(report.trimEnd(), shown);

  lastro(dir, "init", "--model", "model.json");
  lastro(dir, "review", "P-001", "--as", "ana", "approve-all");
  r = lastro(dir, "derive", "P-001");
  copyFileSync(join(example, "answers", "proofs.json"), r.body.answer);
  assert.equal(lastro(dir, "derive", "P-001").code, 0);

  r = lastro(dir, "prove", "P-001", "shipping-buggy.mjs");
  assert.equal(r.code, 1);
  assert.match(r.body.feedback, /^This is a bootstrap proposal: the proofs describe the code as it is today/);
  r = lastro(dir, "prove", "P-001", "shipping.mjs");
  assert.equal(r.code, 0, JSON.stringify(r.body));
  r = lastro(dir, "apply", "P-001");
  assert.equal(r.code, 0);
  assert.deepEqual(r.body.written, ["UC-001"]);
});
