// The TypeScript CLI's decision cores (UC-030, UC-031, UC-032, UC-037, UC-043) as subjects Lastro can prove: each
// export takes a model's inputs and runs the real CLI (dist/src/cli.js, in process) on a throwaway workspace, answering
// the LLM steps by handoff. The models are in .lastro/models/.
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { main } from "../dist/src/cli.js";

const CLI = fileURLToPath(new URL("../dist/src/cli.js", import.meta.url));

const MODEL = { name: "toy", inputs: { x: { type: "int", min: 0, max: 9 } }, outputs: { y: { type: "int", min: 0, max: 9 } } };
const PROSE = "Return the input unchanged.";
const EXTRACTION = {
  name: "Echo", slug: "echo", description: "The code echoes its input.", decisions: [], notPromised: [],
  intents: [{ slug: "echo-input", name: "Echo the input", description: "y is always x.", change: "add", useCase: "echo", provability: "formal", beyondProofReason: "none", quote: PROSE, replaces: [] }],
  useCases: [{ slug: "echo", change: "add", name: "Echo", after: "The code returns its input unchanged." }],
};
const PROOF = (intent) => ({
  intent, slug: "y-is-x", name: "y is x", description: "y is always x.", model: "toy", subject: "toy", kind: "clause",
  requires: "", ensures: JSON.stringify({ op: "eq", args: [{ var: "out.y" }, { var: "in.x" }] }), assumptions: [], inputJson: "", expect: "", replaces: [],
});

async function withRoot(f) {
  const root = mkdtempSync(join(tmpdir(), "lastro-cli-subject-"));
  try {
    writeFileSync(join(root, "model.json"), JSON.stringify(MODEL));
    writeFileSync(join(root, "echo.mjs"), "export default ({ x }) => ({ y: x });\nexport const named = ({ x }) => ({ y: x });\n");
    writeFileSync(join(root, "broken.mjs"), "export default ({ x }) => ({ y: x === 3 ? 4 : x });\n");
    writeFileSync(join(root, "prose.md"), PROSE + "\n");
    const dir = join(root, ".lastro");
    const cli = (...argv) => main([...argv, "--dir", dir, "--llm", "handoff"]);
    return await f({ root, dir, cli });
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

/** Runs a CLI command, answering each LLM handoff with `answer(step)` until it stops asking. */
async function answering(cli, argv, answer) {
  for (let k = 0; k < 5; k++) {
    const r = await cli(...argv);
    if (r.code !== 3) return r;
    writeFileSync(r.body.answer, JSON.stringify(answer(r.body.step)));
  }
  throw new Error("the CLI kept asking");
}

/** A workspace with P-001 proposed, reviewed by ana, and its proof derived. */
async function derived(c) {
  await c.cli("init", "--model", join(c.root, "model.json"), "--subject", "./echo.mjs");
  await answering(c.cli, ["propose", join(c.root, "prose.md"), "--as", "ana"], () => EXTRACTION);
  await c.cli("review", "P-001", "--as", "ana", "approve-all");
  const r = await answering(c.cli, ["derive", "P-001"], () => ({ proofs: [PROOF("P-001.I-01")], decisions: [] }));
  if (r.code !== 0) throw new Error(`derive failed: ${JSON.stringify(r.body)}`);
}

// UC-030: one JSON object per command, and the exit code says what happened.
export async function cliExits({ situation }) {
  return withRoot(async (c) => {
    let r;
    if (situation === "no-command") r = await main(["--dir", c.dir]);
    else if (situation === "unknown-command") r = await c.cli("frobnicate");
    else {
      await c.cli("init", "--model", join(c.root, "model.json"), "--approvals", "2");
      if (situation === "status-empty") r = await c.cli("status");
      else if (situation === "propose-unanswered") r = await c.cli("propose", join(c.root, "prose.md"), "--as", "ana");
      else {
        await answering(c.cli, ["propose", join(c.root, "prose.md"), "--as", "ana"], () => EXTRACTION);
        if (situation === "review-without-as") r = await c.cli("review", "P-001", "approve-all");
        else if (situation === "derive-pending") r = await c.cli("derive", "P-001");
        else r = await c.cli("status");
      }
    }
    const config = existsSync(join(c.dir, "config.json")) ? JSON.parse(readFileSync(join(c.dir, "config.json"), "utf8")) : undefined;
    return {
      exit: r.code,
      json: !!r.body && typeof r.body === "object" && !Array.isArray(r.body),
      refusedPending: r.body?.status === "review-pending",
      approvalsWritten: !config || config.review?.approvals === 2,
    };
  });
}

// UC-031: LLM steps handed to the calling agent through request and answer files.
export async function handoff({ answered, rerun }) {
  return withRoot(async (c) => {
    await c.cli("init", "--model", join(c.root, "model.json"));
    const first = await c.cli("propose", join(c.root, "prose.md"), "--as", "ana");
    const request = first.code === 3 ? JSON.parse(readFileSync(first.body.request, "utf8")) : {};
    let sameId = true;
    if (rerun) sameId = (await c.cli("propose", join(c.root, "prose.md"), "--as", "ana")).body.answer === first.body.answer;
    let after = first;
    if (answered) {
      writeFileSync(first.body.answer, JSON.stringify(EXTRACTION));
      after = await c.cli("propose", join(c.root, "prose.md"), "--as", "ana");
    }
    return {
      exit: after.code,
      requestFile: first.code === 3 && existsSync(first.body.request) && first.body.request.endsWith(".request.json"),
      requestComplete: ["instructions", "system", "user", "schema"].every((k) => k in request),
      answerNamed: first.code === 3 && String(first.body.answer).endsWith(".answer.json"),
      sameId,
    };
  });
}

// UC-032: extracting from one section of a document.
export async function section({ heading, nested }) {
  return withRoot(async (c) => {
    await c.cli("init", "--model", join(c.root, "model.json"));
    const doc = ["# Doc", "", "Intro text.", "", "## Fee", "", "Fee body.", "", ...(nested ? ["### Detail", "", "Sub body.", ""] : []), "## Other", "", "Other body.", ""].join("\n");
    writeFileSync(join(c.root, "doc.md"), doc);
    const name = { exact: "Fee", "other-case": "fEE", unknown: "Nope" }[heading];
    const r = await c.cli("propose", join(c.root, "doc.md"), "--section", name, "--as", "ana");
    const user = r.code === 3 ? JSON.parse(readFileSync(r.body.request, "utf8")).user : "";
    return { exit: r.code, body: user.includes("Fee body."), sub: user.includes("Sub body."), other: user.includes("Other body.") || user.includes("Intro text.") };
  });
}

// UC-037: the code under proof is a JavaScript module export.
export async function subjectModule({ exportName }) {
  return withRoot(async (c) => {
    await derived(c);
    const extra = { default: [], named: ["--export", "named"], missing: ["--export", "nothere"] }[exportName];
    const r = await c.cli("prove", "P-001", join(c.root, "echo.mjs"), ...extra);
    return { exit: r.code, proved: r.body.ok === true };
  });
}

// UC-043: prove --stream prints one event per line and ends with the same result.
export async function stream({ codeOk }) {
  return withRoot(async (c) => {
    await derived(c);
    const code = join(c.root, codeOk ? "echo.mjs" : "broken.mjs");
    const run = (...extra) => spawnSync(process.execPath, [CLI, "prove", "P-001", code, ...extra, "--dir", c.dir, "--llm", "handoff"], { encoding: "utf8" });
    const plain = run();
    const streamed = run("--stream");
    const events = streamed.stdout.trim().split("\n").map((l) => JSON.parse(l));
    const names = events.map((e) => e.event);
    const { event, ...result } = events.at(-1);
    return {
      sameExit: plain.status === streamed.status,
      oneObjectPerLine: events.every((e) => typeof e.event === "string"),
      startFirst: names[0] === "start",
      resultLast: event === "result",
      sameResult: JSON.stringify(result) === JSON.stringify(JSON.parse(plain.stdout)),
      clauseEvents: names.filter((n) => n === "clause").length,
      proofs: 1,
    };
  });
}
