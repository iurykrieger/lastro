// Regenerates src/content/outputs.ts: the real output of the TypeScript CLI and the Lastro plugin on
// examples/shipping, with paths shortened to /home/you/shop. Run it after changing the CLI or the plugin:
//   npm run build -w lastro && node apps/docs/scripts/outputs.mjs
// test/walkthrough.test.mjs and plugins/lastro/test check that the docs still match.
import { spawnSync } from "node:child_process";
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repo = join(here, "..", "..", "..");
const example = join(here, "..", "examples", "shipping");
const cli = join(repo, "sdks", "typescript", "dist", "src", "cli.js");
const { handle } = await import(join(repo, "plugins", "lastro", "scripts", "lastro-plugin.mjs"));
const { loadWorkspace, renderBoard, snapshot } = await import(join(repo, "plugins", "lastro", "scripts", "lib.mjs"));

const dir = mkdtempSync(join(tmpdir(), "lastro-docs-"));
for (const f of ["model.json", "prose.md", "shipping.mjs", "shipping-buggy.mjs"]) copyFileSync(join(example, f), join(dir, f));
const env = { ...process.env };
delete env.ANTHROPIC_API_KEY;
const short = (s) => s.split(dir).join("/home/you/shop").split(`node ${cli}`).join("lastro");
const run = (...args) => {
  const r = spawnSync(process.execPath, [cli, ...args], { cwd: dir, env, encoding: "utf8" });
  return { code: r.status, text: short(r.stdout.trim()), body: JSON.parse(r.stdout) };
};

const OUT = {};
OUT.init = run("init", "--model", "model.json").text;
let r = run("propose", "prose.md", "--as", "ana");
OUT.proposeNeedsLlm = r.text;
copyFileSync(join(example, "answers", "propose.json"), r.body.answer);
OUT.proposed = run("propose", "prose.md", "--as", "ana").text;
run("review", "P-001", "--as", "ana", "answer", "D1", "1");
OUT.reviewed = run("review", "P-001", "--as", "ana", "approve-all").text;
r = run("derive", "P-001");
OUT.deriveNeedsLlm = r.text;
copyFileSync(join(example, "answers", "proofs.json"), r.body.answer);
OUT.derived = run("derive", "P-001").text;
OUT.notProved = run("prove", "P-001", "./shipping-buggy.mjs").text;
const streamed = spawnSync(process.execPath, [cli, "prove", "P-001", "./shipping-buggy.mjs", "--stream"], { cwd: dir, env, encoding: "utf8" });
const STREAM = short(streamed.stdout.trim());
OUT.feedback = run("feedback", "P-001").text;
OUT.proved = run("prove", "P-001", "./shipping.mjs").text;
const RECEIPT = short(readFileSync(join(dir, ".lastro", "proposals", "P-001-shipping-fee.receipt.md"), "utf8").trim());
OUT.applied = run("apply", "P-001").text;
OUT.status = run("status").text;
OUT.regress = run("regress", "./shipping.mjs").text;
const USECASE = readFileSync(join(dir, ".lastro", "usecases", "UC-001-charge-shipping.json"), "utf8").trim();
const DERIVE_REQUEST_KEYS = Object.keys(JSON.parse(readFileSync(r.body.request, "utf8")));

// The plugin's board on the same example, at three moments.
const pdir = mkdtempSync(join(tmpdir(), "lastro-docs-plugin-"));
for (const f of ["model.json", "shipping.mjs", "shipping-buggy.mjs"]) copyFileSync(join(example, f), join(pdir, f));
mkdirSync(join(pdir, ".lastro", "proposals"), { recursive: true });
copyFileSync(join(example, "prose.md"), join(pdir, ".lastro", "proposals", "shipping.md"));
writeFileSync(join(pdir, ".lastro", "plugin.json"), JSON.stringify({ cli: `node ${cli}`, prove: "./shipping-buggy.mjs" }));
const prun = (...args) => JSON.parse(spawnSync(process.execPath, [cli, ...args], { cwd: pdir, env, encoding: "utf8" }).stdout);
prun("init", "--model", "model.json");
r = prun("propose", ".lastro/proposals/shipping.md", "--as", "ana");
copyFileSync(join(example, "answers", "propose.json"), r.answer);
prun("propose", ".lastro/proposals/shipping.md", "--as", "ana");
const board = () => renderBoard(snapshot(loadWorkspace(pdir))).split(`node ${cli}`).join("lastro");
const BOARDS = { review: board() };
prun("review", "P-001", "--as", "ana", "answer", "D1", "1");
prun("review", "P-001", "--as", "ana", "approve-all");
r = prun("derive", "P-001");
copyFileSync(join(example, "answers", "proofs.json"), r.answer);
prun("derive", "P-001");
BOARDS.implementing = board();
BOARDS.failed = handle("Stop", { cwd: pdir }, { LASTRO_HARNESS: "claude" }).systemMessage.split(`node ${cli}`).join("lastro");

const ts = `// Real output of the TypeScript CLI on apps/docs/examples/shipping (paths shortened to /home/you/shop).
// Regenerate with \`node apps/docs/scripts/outputs.mjs\`; test/walkthrough.test.mjs replays the same commands and asserts the
// parts the docs rely on.
export const OUT = ${JSON.stringify(OUT, null, 2)};

export const RECEIPT = ${JSON.stringify(RECEIPT)};

export const USECASE = ${JSON.stringify(USECASE)};

export const DERIVE_REQUEST_KEYS = ${JSON.stringify(DERIVE_REQUEST_KEYS)};

// The plugin progress board on the same example (plugins/lastro); plugins/lastro/test checks these against the real board.
export const BOARDS = ${JSON.stringify(BOARDS)};

// \`lastro prove P-001 ./shipping-buggy.mjs --stream\` on the same example; test/walkthrough.test.mjs replays it.
export const STREAM = ${JSON.stringify(STREAM)};
`;
writeFileSync(join(here, "..", "src", "content", "outputs.ts"), ts);
console.log("wrote src/content/outputs.ts");
