/**
 * End-to-end demo, offline: prose -> intents -> review -> freeze -> plan -> verify (TS + Rust) -> receipt.
 *   npm run example                   (correct implementation)
 *   npm run example -- <bug>          (plant a bug, e.g. refuse-everyone)
 *   npm run example -- --no-review    (approve the AI's intents as-is, including its mistake)
 */
import { existsSync, readFileSync } from "node:fs";
import { freeze, loadRustProver, renderReceipt, review, tabulate, validatePlan, verify } from "../../src/index.js";
import { BUGS, FIXED_I5, deriveLoginPlan, extractLogin, makeLogin } from "./login.js";

const WASM = new URL("../../../wasm/lastro_core.wasm", import.meta.url);

const args = process.argv.slice(2);
const bug = args.find((a) => !a.startsWith("--"));
if (bug && !BUGS.some((b) => b.id === bug)) {
  console.error(`Unknown bug '${bug}'. Options: ${BUGS.map((b) => b.id).join(", ")}`);
  process.exit(2);
}

let r = review(extractLogin()).approveAll().answer("D1", 0).answer("D2", 0);
if (!args.includes("--no-review")) r = r.amend("I5", FIXED_I5, {}, "the AI read 'invalid'; the prose says the email wins");
const artifact = freeze(r.done());
const plan = deriveLoginPlan(artifact);
const uncovered = validatePlan(artifact, plan);
if (uncovered.length) console.warn("Intents with no obligation:", uncovered);

const subject = makeLogin(bug);
const report = await verify(artifact, plan, { subject });
const rust = existsSync(WASM) ? (await loadRustProver(readFileSync(WASM))).prove(plan, await tabulate(plan.model, subject)) : undefined;
if (!rust) console.warn("(wasm/lastro_core.wasm not found: run `npm run wasm` for the Rust cross-check)");
console.log(renderReceipt(report, plan, rust));
process.exit(report.ok ? 0 : 1);
