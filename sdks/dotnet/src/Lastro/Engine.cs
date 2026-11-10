namespace Lastro;

public sealed class VerifyOptions
{
    public Subject? Subject { get; init; }
    public IReadOnlyList<Trace>? Traces { get; init; }
    /// <summary>Max input-space size enumerated exhaustively (default 100 000).</summary>
    public double ExhaustiveLimit { get; init; } = 100_000;
    /// <summary>Samples drawn when the space is too large (default 2000).</summary>
    public int Samples { get; init; } = 2000;
    public long Seed { get; init; } = 42;
    public bool Mutation { get; init; } = true;
    /// <summary>Max output-space size for the consistency check (default 4096).</summary>
    public double ConsistencyLimit { get; init; } = 4096;

    public VerifyOptions With(Subject? subject, IReadOnlyList<Trace>? traces) => new()
    {
        Subject = subject,
        Traces = traces,
        ExhaustiveLimit = ExhaustiveLimit,
        Samples = Samples,
        Seed = Seed,
        Mutation = Mutation,
        ConsistencyLimit = ConsistencyLimit,
    };
}

public enum ClauseStatus { IfPartFalse, Holds, Fails, Error }

public sealed record ClauseExplanation(string ClauseId, ClauseStatus Status, string? Error = null);

/// <summary>
/// The verification engine: contrasts a plan with the program's final output.
/// Finite input space: exhaustive enumeration, <c>proved</c>. Otherwise seeded sampling, <c>evidenced</c>.
/// Every clause must kill a planted mutant (else <c>weak</c>); clauses whose if-part never holds are
/// <c>vacuous</c>; jointly unsatisfiable clauses come back as decision cards.
/// </summary>
public static class Engine
{
    private sealed record Run(Env Input, Env Output, string? Ref = null);

    private sealed record Mutant(string Label, Func<Env, Env> Apply);

    /// <summary>Per-clause status for one run ("3 of 8 clauses apply, all hold").</summary>
    public static List<ClauseExplanation> ExplainRun(ProofPlan plan, Env input, Env output) =>
        plan.Clauses.Select(c =>
        {
            try
            {
                var ctx = new EvalContext(input, output, plan.Model.Defs);
                if (c.Requires is not null && !Lsl.Holds(c.Requires, ctx)) return new ClauseExplanation(c.Id, ClauseStatus.IfPartFalse);
                return new ClauseExplanation(c.Id, Lsl.Holds(c.Ensures, ctx) ? ClauseStatus.Holds : ClauseStatus.Fails);
            }
            catch (LslException e)
            {
                return new ClauseExplanation(c.Id, ClauseStatus.Error, e.Message);
            }
        }).ToList();

    /// <summary>
    /// For every input, is there at least one output satisfying all applicable clauses?
    /// If not, the spec contradicts itself and the team must decide.
    /// </summary>
    public static List<DecisionCard> CheckConsistency(ProofPlan plan, VerifyOptions? opts = null)
    {
        opts ??= new VerifyOptions();
        var cards = new List<DecisionCard>();
        if (plan.Clauses.Count == 0 || Domains.SpaceSize(plan.Model.Outputs) > opts.ConsistencyLimit) return cards;
        var outputs = Domains.EnumerateSpace(plan.Model.Outputs).ToList();
        var inputs = InputSet(plan.Model, opts).Inputs;
        var seen = new HashSet<string>(StringComparer.Ordinal);
        foreach (var input in inputs)
        {
            var applicable = plan.Clauses.Where(c => c.Requires is null || outputs.Any(o => SafeHolds(c.Requires, input, o, plan))).ToList();
            var ok = outputs.Any(output => plan.Clauses.All(c =>
            {
                var ctx = new EvalContext(input, output, plan.Model.Defs);
                return c.Requires is null || !Lsl.Holds(c.Requires, ctx) || Lsl.Holds(c.Ensures, ctx);
            }));
            if (ok) continue;
            var key = string.Join(",", applicable.Select(c => c.Id));
            if (!seen.Add(key)) continue;
            cards.Add(new DecisionCard
            {
                Id = $"X{cards.Count + 1}",
                IntentIds = applicable.SelectMany(c => c.IntentIds).Distinct(StringComparer.Ordinal).ToList(),
                Question = $"Clauses {key} cannot all hold for input {input}. Which one should give way?",
                Options = applicable.Select(c => new DecisionOption { Label = $"Relax {c.Id}", Consequence = c.Description }).ToList(),
                Recommended = 0,
                Origin = DecisionOrigin.ConsistencyCheck,
            });
        }
        return cards;
    }

    private static bool SafeHolds(Expr e, Env input, Env output, ProofPlan plan)
    {
        try
        {
            return Lsl.Holds(e, new EvalContext(input, output, plan.Model.Defs));
        }
        catch (LslException)
        {
            return true;
        }
    }

    private static (List<Env> Inputs, bool Exhaustive) InputSet(Model model, VerifyOptions opts)
    {
        if (Domains.SpaceSize(model.Inputs) <= opts.ExhaustiveLimit) return (Domains.EnumerateSpace(model.Inputs).ToList(), true);
        var next = Domains.Rng(opts.Seed);
        var list = new List<Env>(opts.Samples);
        for (var i = 0; i < opts.Samples; i++) list.Add(Domains.SampleSpace(model.Inputs, next));
        return (list, false);
    }

    private static bool OutputsInDomain(Model model, Env output) => model.Outputs.All(kv => Domains.Contains(kv.Value, output.Get(kv.Key)));

    public static async Task<VerificationReport> VerifyAsync(IntentArtifact artifact, ProofPlan plan, VerifyOptions? opts = null)
    {
        opts ??= new VerifyOptions();
        var check = Plans.Check(artifact, plan);
        if (check.Errors.Count > 0) throw new PlanException(check.Errors);
        var model = plan.Model;
        var defs = model.Defs;

        // 1. Run the program over the input set.
        var runs = new List<Run>();
        var exhaustive = false;
        var runErrors = new List<string>();
        if (opts.Subject is { } subject)
        {
            var set = InputSet(model, opts);
            exhaustive = set.Exhaustive;
            foreach (var input in set.Inputs)
            {
                try
                {
                    var output = await subject(input.Copy()).ConfigureAwait(false);
                    foreach (var (k, d) in model.Outputs)
                        if (!Domains.Contains(d, output.Get(k))) runErrors.Add($"output '{k}'={output.Get(k)} outside its domain for {input}");
                    runs.Add(new Run(input, output));
                }
                catch (Exception e)
                {
                    runErrors.Add($"subject threw for {input}: {e.Message}");
                }
            }
            if (runErrors.Count > 0) runs = runs.Where(r => OutputsInDomain(model, r.Output)).ToList();
        }

        // 2. Clauses.
        var mutants = opts.Mutation ? OutputMutants(model) : [];
        var traces = opts.Traces ?? [];
        var clauses = plan.Clauses.Select(c => CheckClause(c, runs, exhaustive, mutants, traces, model)).ToList();
        if (runErrors.Count > 0)
            foreach (var r in clauses)
                r.Note = string.Join("; ", new[] { r.Note, $"{runErrors.Count} invalid runs (e.g. {runErrors[0]})" }.Where(x => !string.IsNullOrEmpty(x)));

        // 3. Scenarios (Given/When/Then).
        var scenarios = new List<ScenarioResult>();
        foreach (var s in plan.Scenarios)
        {
            if (opts.Subject is null)
            {
                scenarios.Add(new ScenarioResult { ScenarioId = s.Id, Status = ScenarioStatus.Failed, Error = "no subject to run" });
                continue;
            }
            try
            {
                var output = await opts.Subject(s.Input.Copy()).ConfigureAwait(false);
                var ok = Lsl.Holds(s.Expect, new EvalContext(s.Input, output, defs));
                scenarios.Add(new ScenarioResult { ScenarioId = s.Id, Status = ok ? ScenarioStatus.Passed : ScenarioStatus.Failed, Output = output });
            }
            catch (Exception e)
            {
                scenarios.Add(new ScenarioResult { ScenarioId = s.Id, Status = ScenarioStatus.Failed, Error = e.Message });
            }
        }

        // 4. Spec consistency.
        var decisions = CheckConsistency(plan, opts);

        // 5. Roll up per intent.
        var intents = Rollup(artifact, plan, clauses, scenarios, check.Uncovered);
        var summary = Enum.GetValues<IntentVerdict>().ToDictionary(v => v, _ => 0);
        foreach (var i in intents) summary[i.Verdict]++;

        return new VerificationReport
        {
            ArtifactHash = artifact.Hash!,
            PlanHash = Canonical.HashOf(plan),
            Model = model.Name,
            Clauses = clauses,
            Scenarios = scenarios,
            Intents = intents,
            Assumptions = plan.Assumptions,
            BeyondProof = plan.BeyondProof,
            NotPromised = artifact.NotPromised,
            Decisions = decisions,
            Summary = summary,
            Ok = summary[IntentVerdict.Violated] == 0 && summary[IntentVerdict.Gap] == 0 && decisions.Count == 0,
        };
    }

    /// <summary>"Plant one bug" on the output side: negate a boolean, force a field to a constant, nudge an int.</summary>
    private static List<Mutant> OutputMutants(Model model)
    {
        var ms = new List<Mutant>();
        foreach (var (field, d) in model.Outputs)
        {
            if (d.Type == "bool") ms.Add(new Mutant($"¬{field}", o => With(o, field, o.Get(field).IsBool ? !o.Get(field).AsBool : Value.Missing)));
            if (Domains.Size(d) <= 16)
                foreach (var v in Domains.Enumerate(d)) ms.Add(new Mutant($"{field}:={v}", o => With(o, field, v)));
            if (d.Type == "int")
                foreach (var delta in new[] { 1, -1 })
                    ms.Add(new Mutant($"{field}{(delta > 0 ? "+" : "-")}1", o => With(o, field, o.Get(field).IsNumber ? o.Get(field).AsNumber + delta : Value.Missing)));
        }
        return ms;
    }

    private static Env With(Env o, string field, Value v)
    {
        var copy = o.Copy();
        copy[field] = v;
        return copy;
    }

    private static ClauseResult CheckClause(Clause c, List<Run> runs, bool exhaustive, List<Mutant> mutants, IReadOnlyList<Trace> traces, Model model)
    {
        var defs = model.Defs;
        bool Applies(Run r) => c.Requires is null || Lsl.Holds(c.Requires, new EvalContext(r.Input, r.Output, defs));
        bool Ok(Run r) => Lsl.Holds(c.Ensures, new EvalContext(r.Input, r.Output, defs));

        var result = new ClauseResult { ClauseId = c.Id, Verdict = ClauseVerdict.NotRun, Evidence = Evidence.None, Method = CheckMethod.None, Checked = 0 };
        try
        {
            if (runs.Count > 0)
            {
                var applicable = runs.Where(Applies).ToList();
                result.Checked = applicable.Count;
                result.Method = exhaustive ? CheckMethod.Exhaustive : CheckMethod.Sampling;
                var bad = applicable.FirstOrDefault(r => !Ok(r));
                if (bad is not null)
                {
                    result.Verdict = ClauseVerdict.Violated;
                    result.Evidence = exhaustive ? Evidence.Formal : Evidence.Empirical;
                    result.Counterexample = new Counterexample { Input = bad.Input, Output = bad.Output };
                }
                else if (applicable.Count == 0)
                {
                    result.Verdict = ClauseVerdict.Vacuous;
                    result.Note = "no run makes the if-part true: the clause holds vacuously and proves nothing";
                }
                else if (exhaustive)
                {
                    result.Verdict = ClauseVerdict.Proved;
                    result.Evidence = Evidence.Formal;
                }
                else
                {
                    result.Verdict = ClauseVerdict.Evidenced;
                    result.Evidence = Evidence.Empirical;
                    result.FailureRateUpperBound95 = Math.Min(1, 3.0 / applicable.Count);
                }

                // Mutation check: the clause must reject at least one planted bug.
                if (mutants.Count > 0 && result.Verdict is ClauseVerdict.Proved or ClauseVerdict.Evidenced)
                {
                    var killed = 0;
                    foreach (var m in mutants)
                    {
                        var caught = runs.Any(r =>
                        {
                            var mr = new Run(r.Input, m.Apply(r.Output));
                            return Applies(mr) && !Ok(mr);
                        });
                        if (caught) killed++;
                    }
                    result.Mutants = new MutantStats { Total = mutants.Count, Killed = killed };
                    if (killed == 0)
                    {
                        result.Verdict = ClauseVerdict.Weak;
                        result.Note = "no planted mutant was rejected: the clause verifies but rules out no bug (vibe verification)";
                    }
                }
            }

            // Production traces: the model proof can hold while reality disagrees.
            if (traces.Count > 0)
            {
                var tRuns = traces.Select(t => new Run(t.Input, t.Output, t.Ref)).ToList();
                var applicable = tRuns.Where(Applies).ToList();
                result.TracesChecked = applicable.Count;
                var bad = tRuns.FirstOrDefault(r => Applies(r) && !Ok(r));
                if (bad is not null)
                {
                    var provedBefore = result.Verdict == ClauseVerdict.Proved;
                    result.Verdict = ClauseVerdict.Violated;
                    result.Evidence = Evidence.Empirical;
                    result.TraceViolation = new Trace { Input = bad.Input, Output = bad.Output, Ref = bad.Ref };
                    if (provedBefore) result.Note = "proved on the model, but a real trace violates it: a trusted piece (assumption) does not hold";
                }
                else if (runs.Count == 0 && applicable.Count > 0)
                {
                    result.Method = CheckMethod.Traces;
                    result.Checked = applicable.Count;
                    result.Verdict = ClauseVerdict.Evidenced;
                    result.Evidence = Evidence.Empirical;
                    result.FailureRateUpperBound95 = Math.Min(1, 3.0 / applicable.Count);
                }
            }
        }
        catch (LslException e)
        {
            result.Verdict = ClauseVerdict.Violated;
            result.Note = $"evaluation error: {e.Message}";
        }
        return result;
    }

    private static List<IntentResult> Rollup(IntentArtifact artifact, ProofPlan plan, List<ClauseResult> clauses, List<ScenarioResult> scenarios, IReadOnlyList<string> uncovered)
    {
        return Artifacts.ApprovedIntents(artifact).Select(intent =>
        {
            var id = intent.Id;
            var cIds = plan.Clauses.Where(c => c.IntentIds.Contains(id)).Select(c => c.Id).ToList();
            var cRes = clauses.Where(r => cIds.Contains(r.ClauseId)).ToList();
            var aIds = plan.Assumptions.Where(a => a.IntentIds.Contains(id)).Select(a => a.Id);
            var assumedByClauses = plan.Clauses.Where(c => cIds.Contains(c.Id)).SelectMany(c => c.Assumes ?? []);
            var allAssumptions = aIds.Concat(assumedByClauses).Distinct(StringComparer.Ordinal).ToList();
            var sIds = plan.Scenarios.Where(s => s.IntentIds.Contains(id)).Select(s => s.Id).ToList();
            var sRes = scenarios.Where(r => sIds.Contains(r.ScenarioId)).ToList();
            var beyond = plan.BeyondProof.Where(b => b.IntentId == id).ToList();
            var notes = new List<string>();

            IntentResult Result(IntentVerdict verdict, List<Evidence> evidence, double confidence, List<string>? n = null) => new()
            {
                IntentId = id,
                Statement = intent.Statement,
                Verdict = verdict,
                Evidence = evidence,
                Clauses = cIds,
                Assumptions = allAssumptions,
                Scenarios = sIds,
                Confidence = confidence,
                Notes = n ?? notes,
            };

            var evidence = cRes.Select(r => r.Evidence).Where(e => e != Evidence.None).Distinct().ToList();
            if (allAssumptions.Count > 0) evidence.Add(Evidence.Trusted);
            if (sRes.Count > 0 && !evidence.Contains(Evidence.Empirical)) evidence.Add(Evidence.Empirical);

            if (uncovered.Contains(id)) return Result(IntentVerdict.Gap, [Evidence.None], 0, ["approved intent with no derived obligation"]);

            var violated = cRes.Where(r => r.Verdict == ClauseVerdict.Violated).ToList();
            var failedScenarios = sRes.Where(r => r.Status == ScenarioStatus.Failed).ToList();
            if (violated.Count > 0 || failedScenarios.Count > 0)
            {
                foreach (var r in violated) notes.Add($"{r.ClauseId} violated");
                foreach (var r in failedScenarios) notes.Add($"{r.ScenarioId} failed{(r.Error is not null ? $": {r.Error}" : "")}");
                return Result(IntentVerdict.Violated, evidence, 0);
            }

            foreach (var r in cRes.Where(r => r.Verdict is ClauseVerdict.Weak or ClauseVerdict.Vacuous)) notes.Add($"{r.ClauseId} {Wire.Of(r.Verdict)}: {r.Note ?? "undefined"}");
            foreach (var r in cRes.Where(r => r.Verdict == ClauseVerdict.NotRun)) notes.Add($"{r.ClauseId} not run (no subject/traces)");
            var useful = cRes.Where(r => r.Verdict is ClauseVerdict.Proved or ClauseVerdict.Evidenced).ToList();

            if (beyond.Count > 0 && useful.Count == 0)
            {
                foreach (var b in beyond) notes.Add($"beyond proof ({Wire.Of(b.Reason)}): {b.Note}");
                if (sRes.Count > 0) notes.Add($"{sRes.Count} scenario(s) passed as partial evidence");
                return Result(IntentVerdict.BeyondProof, evidence.Count > 0 ? evidence : [Evidence.None], 0);
            }

            if (useful.Count == 0)
            {
                if (sRes.Count > 0)
                {
                    notes.Add("examples only (scenarios), no sampling: confidence not quantified");
                    return Result(IntentVerdict.Evidenced, evidence, 0);
                }
                if (allAssumptions.Count > 0)
                {
                    notes.Add("backed by trusted pieces only");
                    return Result(IntentVerdict.Evidenced, evidence, 0);
                }
                return Result(IntentVerdict.Gap, [Evidence.None], 0);
            }

            if (useful.All(r => r.Verdict == ClauseVerdict.Proved) && useful.Count == cRes.Count)
                return Result(allAssumptions.Count > 0 ? IntentVerdict.ProvedUnderAssumptions : IntentVerdict.Proved, evidence, 1);
            var bound = useful.Max(r => r.FailureRateUpperBound95 ?? 0);
            if (useful.Count != cRes.Count) notes.Add("not every clause carries a useful guarantee");
            return Result(IntentVerdict.Evidenced, evidence, 1 - bound);
        }).ToList();
    }
}
