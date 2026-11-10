namespace Lastro;

/// <summary>
/// The receipt: what was proved, what was trusted, what is only evidenced, and where proof stops.
/// Markdown meant for a pull request. Same text as the TypeScript SDK's <c>renderReceipt</c>.
/// </summary>
public static class Receipt
{
    private static readonly Dictionary<IntentVerdict, string> Label = new()
    {
        [IntentVerdict.Proved] = "✅ Proved",
        [IntentVerdict.ProvedUnderAssumptions] = "🟢 Proved under assumptions",
        [IntentVerdict.Evidenced] = "🟡 Evidenced (confidence)",
        [IntentVerdict.Violated] = "❌ Violated",
        [IntentVerdict.BeyondProof] = "⚪ Beyond proof",
        [IntentVerdict.Gap] = "🕳️ Gap",
    };

    public static string Render(VerificationReport report, ProofPlan? plan = null, RustProofReport? rust = null)
    {
        var lines = new List<string>();
        var s = report.Summary;
        int S(IntentVerdict v) => s.TryGetValue(v, out var n) ? n : 0;
        lines.Add($"# Lastro receipt — {report.Model}");
        lines.Add("");
        lines.Add($"**Result:** {(report.Ok ? "✅ OK" : "❌ Not accepted")}  ");
        lines.Add($"Proved: {S(IntentVerdict.Proved)} · Under assumptions: {S(IntentVerdict.ProvedUnderAssumptions)} · Evidenced: {S(IntentVerdict.Evidenced)} · " +
                  $"Beyond proof: {S(IntentVerdict.BeyondProof)} · Violated: {S(IntentVerdict.Violated)} · Gaps: {S(IntentVerdict.Gap)}");
        lines.Add("");
        lines.Add($"Intents: `{Head(report.ArtifactHash)}…` · Plan: `{Head(report.PlanHash)}…`");
        if (rust is not null)
        {
            var agree = rust.Clauses.All(c => report.Clauses.FirstOrDefault(x => x.ClauseId == c.ClauseId)?.Verdict == c.Verdict);
            lines.Add($"Independent check: `{rust.Engine}` · coverage {rust.Coverage.Rows}/{(rust.Coverage.Space is { } sp ? JsJson.Number(sp) : "∞")} inputs · table `{Head(rust.TableDigest)}…` · {(agree ? "verdicts agree" : "⚠️ verdicts DISAGREE")}");
        }
        lines.Add("");

        lines.AddRange(["## Intents", "", "| Intent | Verdict | Evidence | Confidence | Backed by |", "|---|---|---|---|---|"]);
        foreach (var i in report.Intents)
        {
            var refs = string.Join(", ", i.Clauses.Concat(i.Assumptions).Concat(i.Scenarios));
            if (refs.Length == 0) refs = "—";
            var conf = i.Verdict is IntentVerdict.Proved or IntentVerdict.ProvedUnderAssumptions ? "proof" : i.Confidence > 0 ? $"{Inv.F(i.Confidence * 100, 1)}%" : "—";
            lines.Add($"| **{i.IntentId}** {Esc(i.Statement)} | {Label[i.Verdict]} | {string.Join(" + ", i.Evidence.Select(Wire.Of))} | {conf} | {refs} |");
        }
        var noted = report.Intents.Where(i => i.Notes.Count > 0).ToList();
        if (noted.Count > 0)
        {
            lines.Add("");
            foreach (var i in noted) foreach (var n in i.Notes) lines.Add($"- **{i.IntentId}**: {n}");
        }

        lines.AddRange(["", "## Formal clauses", ""]);
        foreach (var r in report.Clauses)
        {
            var c = plan?.Clauses.FirstOrDefault(x => x.Id == r.ClauseId);
            var how = r.Method switch
            {
                CheckMethod.Exhaustive => $"exhaustive over {r.Checked} applicable inputs",
                CheckMethod.Sampling => $"{r.Checked} applicable samples; failure rate ≤ {Inv.F((r.FailureRateUpperBound95 ?? 1) * 100, 2)}% (95%)",
                CheckMethod.Traces => $"{r.Checked} real traces",
                _ => "not run",
            };
            var mut = r.Mutants is { } m ? $" · mutants killed {m.Killed}/{m.Total}" : "";
            lines.Add($"- **{r.ClauseId}** `{Wire.Of(r.Verdict)}` ({Wire.Of(r.Evidence)}) — {how}{mut}");
            if (c is not null)
            {
                lines.Add($"  - {c.Description}");
                lines.Add($"  - `{Lsl.Formal(c.Requires, c.Ensures, " ⇒ ")}`");
            }
            if (r.Counterexample is { } ce) lines.Add($"  - Counterexample: input `{ce.Input}` → output `{ce.Output}`");
            if (r.TraceViolation is { } tv) lines.Add($"  - Violating trace{(string.IsNullOrEmpty(tv.Ref) ? "" : $" ({tv.Ref})")}: `{tv.Input}`");
            if (!string.IsNullOrEmpty(r.Note)) lines.Add($"  - Note: {r.Note}");
        }

        if (report.Scenarios.Count > 0)
        {
            lines.AddRange(["", "## Scenarios (empirical evidence, not proof)", ""]);
            foreach (var r in report.Scenarios)
            {
                var sc = plan?.Scenarios.FirstOrDefault(x => x.Id == r.ScenarioId);
                lines.Add($"- **{r.ScenarioId}** {(r.Status == ScenarioStatus.Passed ? "✅" : "❌")} {(sc is not null ? $"Given {sc.Given}; when {sc.When}; then {sc.Then}" : "")}{(r.Error is not null ? $" — {r.Error}" : "")}");
            }
        }

        if (report.Assumptions.Count > 0)
        {
            lines.AddRange(["", "## Trusted pieces", ""]);
            foreach (var a in report.Assumptions)
            {
                var tests = a.CheckedBy.Select(id =>
                {
                    var r = report.Scenarios.FirstOrDefault(x => x.ScenarioId == id);
                    return $"{id}{(r is not null ? (r.Status == ScenarioStatus.Passed ? " ✅" : " ❌") : " (planned)")}";
                }).ToList();
                lines.Add($"- **{a.Id}** `{a.Piece}`: {a.Contract}");
                lines.Add($"  - You accept: {a.YouAccept}");
                lines.Add($"  - If it's wrong: {a.IfWrong}");
                lines.Add($"  - Checked by: {(tests.Count > 0 ? string.Join(", ", tests) : "no test")}");
            }
        }

        if (report.BeyondProof.Count > 0)
        {
            lines.AddRange(["", "## Where proof stops", ""]);
            foreach (var b in report.BeyondProof) lines.Add($"- **{b.IntentId}** ({Wire.Of(b.Reason)}): {b.Note}");
        }
        if (report.NotPromised.Count > 0)
        {
            lines.AddRange(["", "## Not promised", ""]);
            foreach (var n in report.NotPromised) lines.Add($"- {n}");
        }
        if (report.Decisions.Count > 0)
        {
            lines.AddRange(["", "## Pending decisions (spec contradictions)", ""]);
            foreach (var d in report.Decisions) lines.Add($"- **{d.Id}** {d.Question}");
        }
        return string.Join("\n", lines) + "\n";
    }

    private static string Head(string s) => s.Length > 19 ? s[..19] : s;

    private static string Esc(string s) => s.Replace("|", "\\|");
}
