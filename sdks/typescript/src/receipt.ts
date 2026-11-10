/**
 * The receipt: what was proved, what was trusted, what is only evidenced,
 * and where proof stops. Meant to be pasted into a pull request.
 */
import type { IntentVerdict, ProofPlan, VerificationReport } from "./types.js";
import type { RustProofReport } from "./rust.js";
import { show } from "./spec/expr.js";

const VERDICT_LABEL: Record<IntentVerdict, string> = {
  proved: "✅ Proved",
  "proved-under-assumptions": "🟢 Proved under assumptions",
  evidenced: "🟡 Evidenced (confidence)",
  violated: "❌ Violated",
  "beyond-proof": "⚪ Beyond proof",
  gap: "🕳️ Gap",
};

export function renderReceipt(report: VerificationReport, plan?: ProofPlan, rust?: RustProofReport): string {
  const L: string[] = [];
  const s = report.summary;
  L.push(`# Lastro receipt — ${report.model}`, "");
  L.push(`**Result:** ${report.ok ? "✅ OK" : "❌ Not accepted"}  `);
  L.push(
    `Proved: ${s.proved} · Under assumptions: ${s["proved-under-assumptions"]} · Evidenced: ${s.evidenced} · ` +
      `Beyond proof: ${s["beyond-proof"]} · Violated: ${s.violated} · Gaps: ${s.gap}`,
    "",
  );
  L.push(`Intents: \`${report.artifactHash.slice(0, 19)}…\` · Plan: \`${report.planHash.slice(0, 19)}…\``);
  if (rust) {
    const agree = rust.clauses.every((c) => report.clauses.find((x) => x.clauseId === c.clauseId)?.verdict === c.verdict);
    L.push(
      `Independent check: \`${rust.engine}\` · coverage ${rust.coverage.rows}/${rust.coverage.space ?? "∞"} inputs · table \`${rust.tableDigest.slice(0, 19)}…\` · ${agree ? "verdicts agree" : "⚠️ verdicts DISAGREE"}`,
    );
  }
  L.push("");

  L.push("## Intents", "", "| Intent | Verdict | Evidence | Confidence | Backed by |", "|---|---|---|---|---|");
  for (const i of report.intents) {
    const refs = [...i.clauses, ...i.assumptions, ...i.scenarios].join(", ") || "—";
    const conf = i.verdict === "proved" || i.verdict === "proved-under-assumptions" ? "proof" : i.confidence > 0 ? `${(i.confidence * 100).toFixed(1)}%` : "—";
    L.push(`| **${i.intentId}** ${esc(i.statement)} | ${VERDICT_LABEL[i.verdict]} | ${i.evidence.join(" + ")} | ${conf} | ${refs} |`);
  }
  const noted = report.intents.filter((i) => i.notes.length);
  if (noted.length) {
    L.push("");
    for (const i of noted) for (const n of i.notes) L.push(`- **${i.intentId}**: ${n}`);
  }

  L.push("", "## Formal clauses", "");
  for (const r of report.clauses) {
    const c = plan?.clauses.find((x) => x.id === r.clauseId);
    const how =
      r.method === "exhaustive"
        ? `exhaustive over ${r.checked} applicable inputs`
        : r.method === "sampling"
          ? `${r.checked} applicable samples; failure rate ≤ ${((r.failureRateUpperBound95 ?? 1) * 100).toFixed(2)}% (95%)`
          : r.method === "traces"
            ? `${r.checked} real traces`
            : "not run";
    const mut = r.mutants ? ` · mutants killed ${r.mutants.killed}/${r.mutants.total}` : "";
    L.push(`- **${r.clauseId}** \`${r.verdict}\` (${r.evidence}) — ${how}${mut}`);
    if (c) L.push(`  - ${c.description}`, `  - \`${c.requires ? `${show(c.requires)} ⇒ ` : ""}${show(c.ensures)}\``);
    if (r.counterexample) L.push(`  - Counterexample: input \`${JSON.stringify(r.counterexample.input)}\` → output \`${JSON.stringify(r.counterexample.output)}\``);
    if (r.traceViolation) L.push(`  - Violating trace${r.traceViolation.ref ? ` (${r.traceViolation.ref})` : ""}: \`${JSON.stringify(r.traceViolation.input)}\``);
    if (r.note) L.push(`  - Note: ${r.note}`);
  }

  if (report.scenarios.length) {
    L.push("", "## Scenarios (empirical evidence, not proof)", "");
    for (const r of report.scenarios) {
      const sc = plan?.scenarios.find((x) => x.id === r.scenarioId);
      L.push(`- **${r.scenarioId}** ${r.status === "passed" ? "✅" : "❌"} ${sc ? `Given ${sc.given}; when ${sc.when}; then ${sc.then}` : ""}${r.error ? ` — ${r.error}` : ""}`);
    }
  }

  if (report.assumptions.length) {
    L.push("", "## Trusted pieces", "");
    for (const a of report.assumptions) {
      const tests = a.checkedBy.map((id) => {
        const r = report.scenarios.find((x) => x.scenarioId === id);
        return `${id}${r ? (r.status === "passed" ? " ✅" : " ❌") : " (planned)"}`;
      });
      L.push(`- **${a.id}** \`${a.piece}\`: ${a.contract}`);
      L.push(`  - You accept: ${a.youAccept}`, `  - If it's wrong: ${a.ifWrong}`, `  - Checked by: ${tests.join(", ") || "no test"}`);
    }
  }

  if (report.beyondProof.length) {
    L.push("", "## Where proof stops", "");
    for (const b of report.beyondProof) L.push(`- **${b.intentId}** (${b.reason}): ${b.note}`);
  }
  if (report.notPromised.length) {
    L.push("", "## Not promised", "");
    for (const n of report.notPromised) L.push(`- ${n}`);
  }
  if (report.decisions.length) {
    L.push("", "## Pending decisions (spec contradictions)", "");
    for (const d of report.decisions) L.push(`- **${d.id}** ${d.question}`);
  }
  return L.join("\n") + "\n";
}

function esc(s: string): string {
  return s.replace(/\|/g, "\\|");
}
