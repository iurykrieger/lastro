using System.Text.Json.Serialization;

namespace Lastro;

public sealed class LastroOptions
{
    /// <summary>The vocabulary of the proofs: inputs, outputs and helper definitions.</summary>
    public required Model Model { get; init; }
    /// <summary>One LLM transport for every AI step. Each step can also be replaced below.</summary>
    public AskJson? Ask { get; init; }
    public IIntentExtractor? Extractor { get; init; }
    public IProofDeriver? Deriver { get; init; }
    /// <summary>Distinct reviewers who must approve each intent (your team's policy). Default 1.</summary>
    public int RequiredApprovals { get; init; } = 1;
    /// <summary>Independent Rust prover (see <see cref="RustProver.Locate"/>). Recommended in CI.</summary>
    public IRustProver? Rust { get; init; }
    /// <summary>Exhaustive limit, samples, seed, mutation and consistency settings. Subject and traces are ignored.</summary>
    public VerifyOptions? Verify { get; init; }
}

/// <summary>Stage 2: the formal spec derived from an approved, fully reviewed artifact.</summary>
public sealed class Spec
{
    public IntentArtifact Artifact { get; set; } = new();
    public ProofPlan Plan { get; set; } = new();
    /// <summary>Clauses that cannot all hold together. Send them back with <see cref="LastroClient.ReturnDecisions"/>.</summary>
    public List<DecisionCard> Contradictions { get; set; } = [];
    public bool Ok { get; set; }
}

[JsonConverter(typeof(KebabEnumConverter<FailureKind>))]
public enum FailureKind { Clause, Scenario }

/// <summary>One clause or scenario the code broke, with the input that shows it. Plain data for any agent.</summary>
public sealed class ProofFailure
{
    public FailureKind Kind { get; set; }
    public string Id { get; set; } = "";
    public List<string> IntentIds { get; set; } = [];
    public string Description { get; set; } = "";
    /// <summary>The formal statement, in LSL notation.</summary>
    public string Formal { get; set; } = "";
    public Env Input { get; set; } = new();
    [JsonIgnore(Condition = JsonIgnoreCondition.Never)]
    public Env? Output { get; set; }
}

/// <summary>Stage 3: the code checked against the spec.</summary>
public sealed class Proof
{
    public bool Ok { get; set; }
    public VerificationReport Report { get; set; } = new();
    public RustProofReport? Rust { get; set; }
    /// <summary>Both engines gave every clause the same verdict (null without a Rust prover).</summary>
    public bool? EnginesAgree { get; set; }
    /// <summary>Code problems, with counterexamples: what the coding agent (or person) must fix.</summary>
    public List<ProofFailure> Failures { get; set; } = [];
    /// <summary>Spec problems (vacuous or weak clauses, intents with no obligation): what the team must fix.</summary>
    public List<string> SpecGaps { get; set; } = [];
    public string Receipt { get; set; } = "";
}

/// <summary>Turning proofs into plain data and briefs for whatever writes the code.</summary>
public static class Feedback
{
    public static List<ProofFailure> FailuresOf(ProofPlan plan, VerificationReport report)
    {
        var output = new List<ProofFailure>();
        foreach (var r in report.Clauses)
        {
            if (r.Verdict != ClauseVerdict.Violated || r.Counterexample is null) continue;
            var c = plan.Clauses.First(x => x.Id == r.ClauseId);
            output.Add(new ProofFailure
            {
                Kind = FailureKind.Clause,
                Id = c.Id,
                IntentIds = c.IntentIds,
                Description = c.Description,
                Formal = $"{(c.Requires is not null ? $"requires {Lsl.Show(c.Requires)}; " : "")}ensures {Lsl.Show(c.Ensures)}",
                Input = r.Counterexample.Input,
                Output = r.Counterexample.Output,
            });
        }
        foreach (var s in report.Scenarios)
        {
            if (s.Status != ScenarioStatus.Failed) continue;
            var sc = plan.Scenarios.First(x => x.Id == s.ScenarioId);
            output.Add(new ProofFailure
            {
                Kind = FailureKind.Scenario,
                Id = sc.Id,
                IntentIds = sc.IntentIds,
                Description = $"given {sc.Given}, when {sc.When}, then {sc.Then}",
                Formal = $"expect {Lsl.Show(sc.Expect)}",
                Input = sc.Input,
                Output = s.Output,
            });
        }
        return output;
    }

    public static List<string> SpecGapsOf(VerificationReport report)
    {
        var output = new List<string>();
        foreach (var r in report.Clauses)
            if (r.Verdict is ClauseVerdict.Vacuous or ClauseVerdict.Weak) output.Add($"{r.ClauseId} is {Wire.Of(r.Verdict)}: {r.Note ?? ""}".Trim());
        foreach (var i in report.Intents) if (i.Verdict == IntentVerdict.Gap) output.Add($"{i.IntentId} has no proof obligation");
        foreach (var d in report.Decisions) output.Add($"Contradiction: {d.Question}");
        return output;
    }

    /// <summary>Failures as a prompt-ready brief for whatever writes the code (a coding agent or a person).</summary>
    public static string AgentFeedback(IReadOnlyList<ProofFailure> failures, IReadOnlyList<string> specGaps)
    {
        if (failures.Count == 0) return "Every proof passes." + (specGaps.Count > 0 ? $" Spec gaps for the team: {string.Join("; ", specGaps)}" : "");
        var lines = new List<string> { "The implementation breaks these proof obligations. Fix the code so each holds for every input, without breaking the others:" };
        foreach (var f in failures)
            lines.Add($"- {f.Id} ({string.Join(", ", f.IntentIds)}): {f.Description}\n  {f.Formal}\n  counterexample: {f.Input} returned {(f.Output is null ? "null" : f.Output.ToString())}");
        return string.Join("\n", lines);
    }

    public static string AgentFeedback(Proof proof) => AgentFeedback(proof.Failures, proof.SpecGaps);
}

/// <summary>
/// The Lastro pipeline behind one small interface (the .NET counterpart of <c>createLastro</c>).
/// <code>
/// You ──plan──▶ intents ──review (policy)──▶ spec ──prove──▶ proof + receipt
///                  ▲                          │                │
///                  └─ contradictions: decisions┘                └─ failures ──▶ whoever writes the code
/// </code>
/// Lastro's only AI steps are extracting intents and deriving the proofs. It never writes the
/// implementation: any coding agent or person does, and <see cref="ProveAsync(Spec, Subject, IReadOnlyList{Trace}?)"/> judges it.
/// </summary>
public sealed class LastroClient
{
    private readonly LastroOptions _o;
    private readonly IIntentExtractor? _extractor;
    private readonly IProofDeriver? _deriver;
    private readonly LlmGapReviewer? _gapReviewer;
    private readonly ReviewPolicy _policy;

    public LastroClient(LastroOptions options)
    {
        _o = options;
        _extractor = options.Extractor ?? (options.Ask is { } a ? new LlmIntentExtractor(a) : null);
        _deriver = options.Deriver ?? (options.Ask is { } b ? new LlmProofDeriver(b) : null);
        _gapReviewer = options.Ask is { } c ? new LlmGapReviewer(c) : null;
        _policy = new ReviewPolicy { RequiredApprovals = options.RequiredApprovals };
    }

    public Model Model => _o.Model;

    public int RequiredApprovals => _policy.RequiredApprovals;

    private static T Need<T>(T? x, string what) where T : class =>
        x ?? throw new InvalidOperationException($"{what} needs an LLM: pass Ask or a custom {what.ToLowerInvariant()} in LastroOptions");

    private IntentArtifact WithPolicy(IntentArtifact a)
    {
        if (a.Policy is not null) return a;
        var copy = LastroJson.Clone(a);
        copy.Policy = new ReviewPolicy { RequiredApprovals = _policy.RequiredApprovals };
        return copy;
    }

    /// <summary>1. Plan: extract reviewable intents from prose. Pass the previous artifact to keep its review.</summary>
    public async Task<IntentArtifact> PlanAsync(string prose, IntentArtifact? previous = null, CancellationToken cancellationToken = default)
    {
        var fresh = await Need(_extractor, "Extractor").ExtractAsync(prose, null, cancellationToken).ConfigureAwait(false);
        fresh.Policy = new ReviewPolicy { RequiredApprovals = _policy.RequiredApprovals };
        return previous is not null ? Artifacts.CarryReview(previous, fresh) : fresh;
    }

    /// <summary>Review as one person. Chain approve/amend/reject/answer, then Done().</summary>
    public Review Review(IntentArtifact artifact, string reviewer)
    {
        var a = artifact.Hash is not null ? Artifacts.Reopen(artifact) : artifact;
        return new Review(WithPolicy(a), reviewer);
    }

    /// <summary>What still blocks the spec: missing approvals and unanswered decisions.</summary>
    public List<string> Pending(IntentArtifact artifact) => Artifacts.PendingReview(WithPolicy(artifact));

    /// <summary>2. Formal spec: approve the reviewed intents, derive clauses, validate, look for contradictions.</summary>
    public async Task<Spec> SpecAsync(IntentArtifact artifact, CancellationToken cancellationToken = default)
    {
        var approved = Artifacts.Approve(WithPolicy(artifact));
        var plan = await Need(_deriver, "Deriver").DeriveAsync(approved, _o.Model, cancellationToken).ConfigureAwait(false);
        Plans.Validate(approved, plan);
        var contradictions = Engine.CheckConsistency(plan, _o.Verify);
        return new Spec { Artifact = approved, Plan = plan, Contradictions = contradictions, Ok = contradictions.Count == 0 };
    }

    /// <summary>Contradictions go back to the team as decision cards; the artifact must be reviewed again.</summary>
    public IntentArtifact ReturnDecisions(Spec spec) => Artifacts.RaiseDecisions(spec.Artifact, spec.Contradictions);

    /// <summary>3. Prove: check the code against every clause (and the Rust prover, if configured).</summary>
    public async Task<Proof> ProveAsync(Spec spec, Subject subject, IReadOnlyList<Trace>? traces = null)
    {
        var opts = (_o.Verify ?? new VerifyOptions()).With(subject, traces ?? []);
        var report = await Engine.VerifyAsync(spec.Artifact, spec.Plan, opts).ConfigureAwait(false);
        RustProofReport? rust = null;
        bool? agree = null;
        if (_o.Rust is { } prover)
        {
            rust = prover.Prove(spec.Plan, await RustProver.TabulateAsync(spec.Plan.Model, subject).ConfigureAwait(false));
            agree = report.Clauses.All(c => rust.Clauses.FirstOrDefault(r => r.ClauseId == c.ClauseId)?.Verdict == c.Verdict);
        }
        return new Proof
        {
            Ok = report.Ok && (rust?.Ok ?? true) && agree != false,
            Report = report,
            Rust = rust,
            EnginesAgree = agree,
            Failures = Feedback.FailuresOf(spec.Plan, report),
            SpecGaps = Feedback.SpecGapsOf(report),
            Receipt = Receipt.Render(report, spec.Plan, rust),
        };
    }

    public Task<Proof> ProveAsync(Spec spec, Func<Env, Env> subject, IReadOnlyList<Trace>? traces = null) => ProveAsync(spec, Subjects.From(subject), traces);

    /// <summary>LLM gap review: what the proofs cannot see. Evidence for humans, never proof.</summary>
    public Task<List<Gap>> GapReviewAsync(Spec spec, string source, CancellationToken cancellationToken = default) =>
        Need(_gapReviewer, "Gap reviewer").ReviewAsync(spec.Artifact, spec.Plan, source, cancellationToken);
}
