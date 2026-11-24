/**
 * An SMT solver run as a process (Node only): any solver that reads SMT-LIB 2 on stdin, such as `z3 -in -smt2` or
 * `cvc5 --lang smt2`. `.lastro/config.json` names it under `smt.command`; Lastro ships no solver of its own.
 */
import { spawn } from "node:child_process";
import { parseValues, smtSymbol, type SmtAnswer, type SmtSolver } from "./smt.js";

export const DEFAULT_SMT_COMMAND = "z3 -in -smt2";
export const DEFAULT_SMT_TIMEOUT_MS = 20_000;

export class SolverError extends Error {}

/** A solver that runs `command` once per script. A timeout or a crash answers unknown, never a verdict. */
export function commandSolver(command = DEFAULT_SMT_COMMAND, timeoutMs = DEFAULT_SMT_TIMEOUT_MS): SmtSolver {
  return (script, values) =>
    new Promise((resolve) => {
      const child = spawn(command, { shell: true, stdio: ["pipe", "pipe", "pipe"] });
      let out = "";
      let done = false;
      const finish = (answer: SmtAnswer, vals: Record<string, unknown> = {}) => {
        if (done) return;
        done = true;
        clearTimeout(timer);
        child.kill("SIGKILL");
        resolve({ answer, values: vals as Record<string, never> });
      };
      const timer = setTimeout(() => finish("unknown"), timeoutMs);
      child.stdout.on("data", (d) => (out += d));
      child.on("error", () => finish("unknown"));
      child.on("close", () => {
        const first = out.trim().split(/\s+/)[0];
        const answer: SmtAnswer = first === "sat" || first === "unsat" ? first : "unknown";
        finish(answer, answer === "sat" ? parseValues(out) : {});
      });
      child.stdin.on("error", () => undefined);
      child.stdin.end(`${script}\n${values.length ? `(get-value (${values.map(smtSymbol).join(" ")}))\n` : ""}(exit)\n`);
    });
}

/** Whether `command` runs: a quick check before relying on it. */
export async function solverAvailable(command = DEFAULT_SMT_COMMAND): Promise<boolean> {
  const r = await commandSolver(command, 5000)("(check-sat)", []);
  return r.answer === "sat";
}
