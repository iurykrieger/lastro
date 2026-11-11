import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { ProposalError, proposalFromSurvey, renderSurveyReport, type RepoDigest, type SurveyOut, type UseCase } from "../src/index.js";
import { roleOf, scanRepo, symbolsOf, testsOf } from "../src/bootstrap/scan.js";
import { main as cli } from "../src/cli.js";

// ---------------------------------------------------------------------------
// A small repository with history: a shipping fee, its tests, a fix
// ---------------------------------------------------------------------------

const MODEL = {
  name: "shipping",
  inputs: { subtotal: { type: "int", min: 0, max: 300 }, member: { type: "bool" } },
  outputs: { fee: { type: "int", min: 0, max: 50 } },
};

function git(dir: string, ...args: string[]) {
  return execFileSync("git", ["-C", dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", ...args], { encoding: "utf8" }).trim();
}

function fixtureRepo(): { dir: string; feat: string; fix: string } {
  const dir = mkdtempSync(join(tmpdir(), "lastro-repo-"));
  git(dir, "init", "-q", "-b", "main");
  mkdirSync(join(dir, "src"));
  mkdirSync(join(dir, "test"));
  writeFileSync(join(dir, "package.json"), JSON.stringify({ name: "shop", main: "src/shipping.mjs", scripts: { test: "node --test" } }, null, 2));
  writeFileSync(join(dir, "README.md"), "# Shop\n\nCharges shipping on orders.\n");
  writeFileSync(join(dir, "src", "shipping.mjs"), "export default function fee({ subtotal, member }) {\n  return { fee: member || subtotal > 100 ? 0 : 15 };\n}\n");
  git(dir, "add", "-A");
  git(dir, "commit", "-q", "-m", "feat: flat shipping fee of 15, free for members");
  const feat = git(dir, "rev-parse", "HEAD");
  writeFileSync(join(dir, "src", "shipping.mjs"), "export default function fee({ subtotal, member }) {\n  return { fee: member || subtotal >= 100 ? 0 : 15 };\n}\n");
  writeFileSync(join(dir, "test", "shipping.test.mjs"), 'import { test } from "node:test";\ntest("members ship free", () => {});\ntest("orders from 100 ship free", () => {});\n');
  git(dir, "add", "-A");
  git(dir, "commit", "-q", "-m", "fix: free shipping from 100, not above 100", "-m", "Support asked for 100 to count.\n\nCo-authored-by: Someone <x@example.com>");
  const fix = git(dir, "rev-parse", "HEAD");
  return { dir, feat, fix };
}

function survey(fix: string, over: Partial<SurveyOut> = {}): SurveyOut {
  return {
    name: "Bootstrap the shop",
    slug: "bootstrap-shop",
    description: "The use cases the shop implements today.",
    useCases: [
      {
        slug: "charge-shipping",
        change: "add",
        name: "Charge shipping",
        description: "An order pays a shipping fee of 15, except members and orders with a subtotal of 100 or more, which ship free.",
        confidence: "high",
        behaviors: [
          { slug: "members-ship-free", name: "Members ship free", description: "A member never pays shipping.", provability: "formal", beyondProofReason: "none", files: ["src/shipping.mjs"], tests: ["test/shipping.test.mjs::members ship free"], commits: [] },
          { slug: "free-from-100", name: "Free from 100", description: "An order with a subtotal of 100 or more ships free; below 100 a non-member pays 15.", provability: "formal", beyondProofReason: "none", files: ["src/shipping.mjs"], tests: ["test/shipping.test.mjs::orders from 100 ship free"], commits: [fix.slice(0, 7)] },
        ],
      },
    ],
    decisions: [],
    notCovered: [{ path: "README.md", reason: "documentation" }],
    models: [{ name: "shipping", json: JSON.stringify(MODEL) }],
    subjects: [{ name: "shipping", module: "src/shipping.mjs", export: "default", model: "shipping" }],
    ...over,
  };
}

const POLICY = { approvals: 1, submitterCanApprove: true };
const EMPTY = { models: {}, subjects: {} };

// ---------------------------------------------------------------------------
// Scan
// ---------------------------------------------------------------------------

test("scan: files, roles, symbols, tests, manifests and commits, deterministically", () => {
  const { dir, feat, fix } = fixtureRepo();
  const d = scanRepo(dir);
  assert.equal(d.lastro, "repo-digest/v1");
  assert.equal(d.head, fix);
  assert.deepEqual(d.files.map((f) => [f.path, f.role]), [["README.md", "doc"], ["package.json", "config"], ["src/shipping.mjs", "source"], ["test/shipping.test.mjs", "test"]]);
  assert.deepEqual(d.files.find((f) => f.path === "src/shipping.mjs")!.symbols, ["fee"]);
  assert.deepEqual(d.tests.map((t) => t.id), ["test/shipping.test.mjs::members ship free", "test/shipping.test.mjs::orders from 100 ship free"]);
  assert.deepEqual(d.manifests, [{ path: "package.json", kind: "npm", name: "shop", entries: ["src/shipping.mjs"], scripts: ["test"] }]);
  assert.deepEqual(d.commits.map((c) => [c.sha, c.subject]), [[fix, "fix: free shipping from 100, not above 100"], [feat, "feat: flat shipping fee of 15, free for members"]]);
  assert.equal(d.commits[0]!.body, "Support asked for 100 to count.", "trailers are dropped");
  assert.deepEqual(d.commits[0]!.files, ["src/shipping.mjs", "test/shipping.test.mjs"]);
  assert.deepEqual(d.totals, { files: 4, commits: 2 });
  assert.deepEqual(d.truncated, { files: false, commits: false });
  assert.equal(scanRepo(dir).hash, d.hash, "same checkout, same digest");

  const scoped = scanRepo(dir, { scope: ["src/"], maxCommits: 1 });
  assert.deepEqual(scoped.files.map((f) => f.path), ["src/shipping.mjs"]);
  assert.deepEqual(scoped.commits.map((c) => c.sha), [fix]);
  assert.deepEqual(scoped.truncated, { files: false, commits: true });
  assert.notEqual(scoped.hash, d.hash);
  assert.throws(() => scanRepo(dir, { scope: ["nope"] }), /scope "nope" matches no file/);
});

test("scan: per-language roles, symbols and tests", () => {
  assert.equal(roleOf("pkg/thing_test.go"), "test");
  assert.equal(roleOf("tests/Lastro.Tests/InteropTests.cs"), "test");
  assert.equal(roleOf("app/test_orders.py"), "test");
  assert.equal(roleOf("src/a.spec.tsx"), "test");
  assert.equal(roleOf("docs/guide.md"), "doc");
  assert.equal(roleOf("Cargo.toml"), "config");
  assert.deepEqual(symbolsOf("go", "func Price(x int) int {}\nfunc (s *Shop) Charge() {}\nfunc helper() {}\ntype Order struct{}"), ["Price", "Charge", "Order"]);
  assert.deepEqual(symbolsOf("python", "def charge():\n  pass\ndef _x(): pass\nclass Cart:\n"), ["charge", "Cart"]);
  assert.deepEqual(testsOf("go", "func TestPrice(t *testing.T) {}\nfunc helper() {}"), ["TestPrice"]);
  assert.deepEqual(testsOf("csharp", "[Fact]\n    public void Hashes_match() {}\n[Theory]\n[InlineData(1)]\npublic async Task Runs(int x) {}"), ["Hashes_match", "Runs"]);
  assert.deepEqual(testsOf("python", "def test_total():\n  pass\n"), ["test_total"]);
  assert.deepEqual(testsOf("rust", "#[test]\nfn proves() {}"), ["proves"]);
});

// ---------------------------------------------------------------------------
// Survey answer → bootstrap proposal
// ---------------------------------------------------------------------------

test("survey: behaviors become grounded intents of a bootstrap proposal, with its setup", () => {
  const { dir, fix } = fixtureRepo();
  const digest = scanRepo(dir);
  const p = proposalFromSurvey({ id: "P-001", digest, submittedBy: "ana", useCases: [], policy: POLICY, config: EMPTY }, survey(fix));
  assert.equal(p.status, "open");
  assert.deepEqual(p.origin, { kind: "bootstrap", head: fix, digest: digest.hash, scope: [] });
  assert.deepEqual(p.intents.map((i) => [i.id, i.change, i.useCase]), [["P-001.I-01", "add", "UC-001"], ["P-001.I-02", "add", "UC-001"]]);
  assert.deepEqual(p.intents[1]!.grounds, { files: ["src/shipping.mjs"], tests: ["test/shipping.test.mjs::orders from 100 ship free"], commits: [fix], confidence: "high" });
  assert.equal(p.prose.slice(p.intents[0]!.source.start, p.intents[0]!.source.end), "A member never pays shipping.");
  assert.match(p.prose, /Read from commit [0-9a-f]{12}: 4 files, 2 tests, 2 of 2 commits\./);
  assert.match(p.prose, /commits [0-9a-f]{7} \(fix: free shipping from 100, not above 100\)/);
  assert.deepEqual(p.useCases.map((c) => [c.useCase, c.change, c.after]), [["UC-001", "add", survey(fix).useCases[0]!.description]]);
  assert.deepEqual(p.setup, { models: { shipping: MODEL }, subjects: { shipping: { module: "src/shipping.mjs" } } });
  assert.deepEqual(p.notPromised, ["Not covered: README.md (documentation)"]);
  assert.equal(renderSurveyReport(digest, survey(fix)), p.prose, "the report is the prose");
});

test("survey: every ungrounded or out-of-bounds claim is refused at once", () => {
  const { dir, fix } = fixtureRepo();
  const digest: RepoDigest = scanRepo(dir);
  const proven: UseCase = { id: "UC-001", slug: "charge-shipping", name: "Charge shipping", description: "old", proofs: [{ id: "P-001.PF-01" } as UseCase["proofs"][number]], history: ["P-001"] };
  const bad = survey(fix);
  bad.useCases[0]!.change = "modify";
  bad.useCases[0]!.behaviors[0]!.files = ["src/missing.mjs"];
  bad.useCases[0]!.behaviors[0]!.tests = ["test/shipping.test.mjs::no such test"];
  bad.useCases[0]!.behaviors[1]!.commits = ["abc"];
  bad.models = [{ name: "shipping", json: JSON.stringify({ ...MODEL, outputs: {} }) }];
  try {
    proposalFromSurvey({ id: "P-002", digest, submittedBy: "ana", useCases: [proven], policy: POLICY, config: EMPTY }, bad);
    assert.fail("should refuse");
  } catch (e) {
    assert.ok(e instanceof ProposalError);
    assert.deepEqual(e.problems, [
      'use case "charge-shipping": UC-001 already has proofs; change proven behavior with a proposal, not a bootstrap',
      'behavior "members-ship-free": file src/missing.mjs is not in the digest',
      'behavior "members-ship-free": test test/shipping.test.mjs::no such test is not in the digest (use "<file>::<name>")',
      'behavior "free-from-100": commit abc is not in the digest (or the prefix is ambiguous)',
      "model shipping: model needs outputs",
      "subject shipping: unknown model shipping",
    ]);
  }
  // A recorded use case without proofs can be modified; one that is new cannot reuse its slug.
  const unproven = { ...proven, proofs: [] };
  const added = () => proposalFromSurvey({ id: "P-002", digest, submittedBy: "ana", useCases: [unproven], policy: POLICY, config: EMPTY }, survey(fix));
  assert.throws(added, /already exists as UC-001; modify it or leave it out/);
  const mod = survey(fix);
  mod.useCases[0]!.change = "modify";
  const p = proposalFromSurvey({ id: "P-002", digest, submittedBy: "ana", useCases: [unproven], policy: POLICY, config: { models: { shipping: MODEL as never }, subjects: {} } }, { ...mod, models: [] });
  assert.equal(p.useCases[0]!.before, "old");
  assert.equal(p.setup?.models.shipping, undefined, "a declared model is not proposed again");
});

// ---------------------------------------------------------------------------
// CLI: bootstrap → review → derive → prove (against today's code) → apply
// ---------------------------------------------------------------------------

const clause = (o: Record<string, unknown>) => ({ model: "shipping", subject: "shipping", kind: "clause", requires: "", assumptions: [], inputJson: "", expect: "", replaces: [], ...o });

test("cli: bootstrap an existing repository into proven use cases", async () => {
  const { dir, fix } = fixtureRepo();
  const ws = ["--dir", join(dir, ".lastro"), "--llm", "handoff"];

  let r = await cli(["scan", ...ws]);
  assert.equal(r.code, 0);
  assert.deepEqual(r.body.read, { files: 4, tests: 2, commits: 2 });
  assert.deepEqual(r.body.suggestedScopes, ["src", "test"]);

  r = await cli(["bootstrap", "--as", "ana", ...ws]);
  assert.equal(r.code, 3, JSON.stringify(r.body));
  assert.equal(r.body.step, "survey");
  const request = JSON.parse(readFileSync(r.body.request as string, "utf8")) as { user: string };
  assert.match(request.user, /<file path="src\/shipping.mjs">/);
  assert.match(request.user, /fix: free shipping from 100/);
  assert.ok(existsSync(join(dir, ".lastro", "config.json")), "bootstrap starts a workspace");
  writeFileSync(r.body.answer as string, JSON.stringify(survey(fix)));
  r = await cli(["bootstrap", "--as", "ana", ...ws]);
  assert.equal(r.code, 1, JSON.stringify(r.body));
  assert.equal((r.body.proposal as { id: string }).id, "P-001");
  assert.equal((r.body.origin as { kind: string }).kind, "bootstrap");
  assert.match(readFileSync(join(dir, ".lastro", "proposals", "P-001-bootstrap-shop.md"), "utf8"), /## Charge shipping \(charge-shipping, add, high confidence\)/);

  r = await cli(["review", "P-001", "--as", "ana", "approve-all", ...ws]);
  assert.equal((r.body.proposal as { status: string }).status, "approved");

  r = await cli(["derive", "P-001", ...ws]);
  assert.equal(r.code, 3);
  const derive = JSON.parse(readFileSync(r.body.request as string, "utf8")) as { user: string };
  assert.match(derive.user, /"shipping"/, "the proposal's models reach the derive step");
  // The second proof misreads the code (strictly above 100): a bootstrap proof that fails accuses the description, not the code.
  writeFileSync(
    r.body.answer as string,
    JSON.stringify({
      proofs: [
        clause({ intent: "P-001.I-01", slug: "members-free", name: "Members free", description: "members pay nothing", requires: JSON.stringify({ var: "in.member" }), ensures: JSON.stringify({ op: "eq", args: [{ var: "out.fee" }, { const: 0 }] }) }),
        clause({ intent: "P-001.I-02", slug: "free-above-100", name: "Free above 100", description: "non-members pay 15 up to 100", requires: JSON.stringify({ op: "and", args: [{ op: "not", args: [{ var: "in.member" }] }, { op: "le", args: [{ var: "in.subtotal" }, { const: 100 }] }] }), ensures: JSON.stringify({ op: "eq", args: [{ var: "out.fee" }, { const: 15 }] }) }),
      ],
      decisions: [],
    }),
  );
  r = await cli(["derive", "P-001", ...ws]);
  assert.equal(r.code, 0, JSON.stringify(r.body));

  r = await cli(["prove", "P-001", ...ws]);
  assert.equal(r.code, 1);
  assert.deepEqual((r.body.failures as { proof: string }[]).map((f) => f.proof), ["P-001.PF-02"]);
  assert.match(r.body.feedback as string, /^This is a bootstrap proposal: the proofs describe the code as it is today, so do not change the code/);
  assert.equal((await cli(["apply", "P-001", ...ws])).code, 1, "apply refuses until every proof passes");

  // The reviewer amends the reading; the proof is derived again and passes against the same code.
  await cli(["review", "P-001", "--as", "ana", "amend", "I-02", "A non-member pays 15 below 100 and nothing from 100 on.", ...ws]);
  await cli(["review", "P-001", "--as", "ana", "rewrite", "UC-001", "An order pays 15 for shipping, except members and orders of 100 or more.", ...ws]);
  r = await cli(["review", "P-001", "--as", "ana", "approve-all", ...ws]);
  assert.equal((r.body.proposal as { status: string }).status, "approved", JSON.stringify(r.body.pending));
  r = await cli(["derive", "P-001", ...ws]);
  assert.equal(r.code, 3, "the amended intent is a new derive request");
  writeFileSync(
    r.body.answer as string,
    JSON.stringify({
      proofs: [
        clause({ intent: "P-001.I-01", slug: "members-free", name: "Members free", description: "members pay nothing", requires: JSON.stringify({ var: "in.member" }), ensures: JSON.stringify({ op: "eq", args: [{ var: "out.fee" }, { const: 0 }] }) }),
        clause({ intent: "P-001.I-02", slug: "fee-below-100", name: "Fee below 100", description: "non-members pay 15 below 100, nothing from 100", requires: JSON.stringify({ op: "not", args: [{ var: "in.member" }] }), ensures: JSON.stringify({ op: "eq", args: [{ var: "out.fee" }, { op: "ite", args: [{ op: "lt", args: [{ var: "in.subtotal" }, { const: 100 }] }, { const: 15 }, { const: 0 }] }] }) }),
      ],
      decisions: [],
    }),
  );
  assert.equal((await cli(["derive", "P-001", ...ws])).code, 0);
  r = await cli(["prove", "P-001", ...ws]);
  assert.equal(r.code, 0, JSON.stringify(r.body));

  r = await cli(["apply", "P-001", ...ws]);
  assert.equal(r.code, 0, JSON.stringify(r.body));
  assert.deepEqual(r.body.config, { models: ["shipping"], subjects: ["shipping"] });
  const uc = JSON.parse(readFileSync(join(dir, ".lastro", "usecases", "UC-001-charge-shipping.json"), "utf8")) as UseCase;
  assert.deepEqual(uc.proofs.map((x) => [x.id, x.status]), [["P-001.PF-01", "passed"], ["P-001.PF-02", "passed"]]);
  assert.deepEqual(uc.history, ["P-001"]);
  const cfg = JSON.parse(readFileSync(join(dir, ".lastro", "config.json"), "utf8")) as { models: Record<string, unknown>; subjects: Record<string, unknown> };
  assert.deepEqual(cfg.subjects, { shipping: { module: "src/shipping.mjs" } });
  assert.deepEqual(Object.keys(cfg.models), ["shipping"]);

  // From now on the bootstrap's proofs are regression tests, like any other.
  assert.equal((await cli(["regress", ...ws])).code, 0);
  writeFileSync(join(dir, "regressed.mjs"), "export default ({ subtotal, member }) => ({ fee: member || subtotal > 100 ? 0 : 15 });\n");
  r = await cli(["regress", join(dir, "regressed.mjs"), ...ws]);
  assert.deepEqual(r.body.regressions, ["P-001.PF-02"]);
  // A second bootstrap sees the recorded use case and cannot add it again.
  r = await cli(["bootstrap", "--as", "ana", ...ws]);
  assert.equal(r.code, 3);
  writeFileSync(r.body.answer as string, JSON.stringify(survey(fix)));
  r = await cli(["bootstrap", "--as", "ana", ...ws]);
  assert.equal(r.body.status, "refused");
  assert.match(JSON.stringify(r.body.problems), /already exists as UC-001/);
});
