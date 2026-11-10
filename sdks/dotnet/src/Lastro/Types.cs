using System.Text.Json.Serialization;

namespace Lastro;

// ---------------------------------------------------------------------------
// Intents
// ---------------------------------------------------------------------------

/// <summary>The five limits of formal proof (PDD Kit's "where proof stops").</summary>
[JsonConverter(typeof(KebabEnumConverter<BeyondProofReason>))]
public enum BeyondProofReason { ExternalCode, WorldFacts, TerminationTiming, HumanJudgment, Unstated }

/// <summary>How the extractor proposes the intent can be checked.</summary>
[JsonConverter(typeof(KebabEnumConverter<Provability>))]
public enum Provability { Formal, Empirical, BeyondProof }

[JsonConverter(typeof(KebabEnumConverter<IntentKind>))]
public enum IntentKind { Behavior, Invariant, Preserve, NonFunctional }

[JsonConverter(typeof(KebabEnumConverter<IntentStatus>))]
public enum IntentStatus { Proposed, Approved, Rejected, Amended }

[JsonConverter(typeof(KebabEnumConverter<DecisionOrigin>))]
public enum DecisionOrigin { Extraction, ConsistencyCheck, Verification }

/// <summary>Where in the prose an intent came from (the intent map).</summary>
public sealed class SourceSpan
{
    public string Quote { get; set; } = "";
    public int Start { get; set; }
    public int End { get; set; }
}

public sealed class Intent
{
    public string Id { get; set; } = "";
    public string Statement { get; set; } = "";
    public IntentKind Kind { get; set; }
    public Provability Provability { get; set; }
    public BeyondProofReason? BeyondProofReason { get; set; }
    public SourceSpan Source { get; set; } = new();
    public IntentStatus Status { get; set; }
    /// <summary>Reviewer note, set when amended or rejected.</summary>
    public string? ReviewNote { get; set; }
    /// <summary>The original statement, kept when a reviewer amends it.</summary>
    public string? OriginalStatement { get; set; }
    /// <summary>Reviewers who approved the current statement.</summary>
    public List<string>? Approvals { get; set; }
    /// <summary>Set when the intent records the answer to a decision (for example a contradiction in the spec).</summary>
    public string? DecisionId { get; set; }
}

public sealed class DecisionOption
{
    public string Label { get; set; } = "";
    public string Consequence { get; set; } = "";
}

/// <summary>One question, options with consequences, one recommended (PDD Kit decision card).</summary>
public sealed class DecisionCard
{
    public string Id { get; set; } = "";
    public List<string> IntentIds { get; set; } = [];
    public string Question { get; set; } = "";
    public List<DecisionOption> Options { get; set; } = [];
    public int Recommended { get; set; }
    public int? Chosen { get; set; }
    public DecisionOrigin Origin { get; set; }
}

public sealed class ReviewPolicy
{
    /// <summary>Distinct reviewers who must approve each intent before it reaches the spec.</summary>
    public int RequiredApprovals { get; set; } = 1;
}

public sealed class IntentArtifact
{
    public string Lastro { get; set; } = "intent-artifact/v0";
    public string Prose { get; set; } = "";
    public List<Intent> Intents { get; set; } = [];
    public List<DecisionCard> Decisions { get; set; } = [];
    /// <summary>Explicitly not promised (timing, rate limits...).</summary>
    public List<string> NotPromised { get; set; } = [];
    public ReviewPolicy? Policy { get; set; }
    /// <summary>Set by <see cref="Artifacts.Freeze"/>.</summary>
    public string? Hash { get; set; }
}

// ---------------------------------------------------------------------------
// Proof plan
// ---------------------------------------------------------------------------

/// <summary>The vocabulary of the proofs: inputs (including facts from trusted pieces), outputs and shared defs.</summary>
public sealed class Model
{
    public string Name { get; set; } = "";
    public Dictionary<string, Domain> Inputs { get; set; } = new(StringComparer.Ordinal);
    public Dictionary<string, Domain> Outputs { get; set; } = new(StringComparer.Ordinal);
    public Dictionary<string, Def>? Defs { get; set; }
}

/// <summary>A formal obligation: for every input, requires(input, output) ⇒ ensures(input, output).</summary>
public sealed class Clause
{
    public string Id { get; set; } = "";
    public List<string> IntentIds { get; set; } = [];
    public string Description { get; set; } = "";
    public Expr? Requires { get; set; }
    public Expr Ensures { get; set; } = L.C(true);
    /// <summary>Trusted assumptions this clause relies on.</summary>
    public List<string>? Assumes { get; set; }
}

/// <summary>A trusted piece: the boundary of the proof (PDD Kit trust card).</summary>
public sealed class Assumption
{
    public string Id { get; set; } = "";
    public List<string> IntentIds { get; set; } = [];
    public string Piece { get; set; } = "";
    public string Contract { get; set; } = "";
    public string YouAccept { get; set; } = "";
    public string IfWrong { get; set; } = "";
    public List<string> CheckedBy { get; set; } = [];
}

/// <summary>An empirical obligation: Given/When/Then, checked by running the program.</summary>
public sealed class Scenario
{
    public string Id { get; set; } = "";
    public List<string> IntentIds { get; set; } = [];
    public string Given { get; set; } = "";
    public string When { get; set; } = "";
    public string Then { get; set; } = "";
    public Env Input { get; set; } = new();
    public Expr Expect { get; set; } = L.C(true);
}

public sealed class BeyondProofItem
{
    public string IntentId { get; set; } = "";
    public BeyondProofReason Reason { get; set; }
    public string Note { get; set; } = "";
}

public sealed class ProofPlan
{
    public string Lastro { get; set; } = "proof-plan/v0";
    public string ArtifactHash { get; set; } = "";
    public Model Model { get; set; } = new();
    public List<Clause> Clauses { get; set; } = [];
    public List<Assumption> Assumptions { get; set; } = [];
    public List<Scenario> Scenarios { get; set; } = [];
    public List<BeyondProofItem> BeyondProof { get; set; } = [];
}

// ---------------------------------------------------------------------------
// Verification
// ---------------------------------------------------------------------------

/// <summary>The program's final output: a decision function over the model inputs.</summary>
public delegate Task<Env> Subject(Env input);

public static class Subjects
{
    public static Subject From(Func<Env, Env> f) => input => Task.FromResult(f(input));
    public static Subject From(Func<Env, Task<Env>> f) => input => f(input);
}

/// <summary>A recorded real execution.</summary>
public sealed class Trace
{
    public Env Input { get; set; } = new();
    public Env Output { get; set; } = new();
    public string? Ref { get; set; }
}

[JsonConverter(typeof(KebabEnumConverter<Evidence>))]
public enum Evidence { Formal, Empirical, Trusted, None }

[JsonConverter(typeof(KebabEnumConverter<ClauseVerdict>))]
public enum ClauseVerdict { Proved, Evidenced, Violated, Vacuous, Weak, NotRun }

[JsonConverter(typeof(KebabEnumConverter<CheckMethod>))]
public enum CheckMethod { Exhaustive, Sampling, Traces, None }

[JsonConverter(typeof(KebabEnumConverter<ScenarioStatus>))]
public enum ScenarioStatus { Passed, Failed }

[JsonConverter(typeof(KebabEnumConverter<IntentVerdict>))]
public enum IntentVerdict { Proved, ProvedUnderAssumptions, Evidenced, Violated, BeyondProof, Gap }

public sealed class Counterexample
{
    public Env Input { get; set; } = new();
    public Env Output { get; set; } = new();
}

public sealed class MutantStats
{
    public int Total { get; set; }
    public int Killed { get; set; }
}

public sealed class ClauseResult
{
    public string ClauseId { get; set; } = "";
    public ClauseVerdict Verdict { get; set; }
    public Evidence Evidence { get; set; }
    public CheckMethod Method { get; set; }
    public int Checked { get; set; }
    /// <summary>For sampling: upper bound of the failure rate at 95% (rule of three).</summary>
    public double? FailureRateUpperBound95 { get; set; }
    public Counterexample? Counterexample { get; set; }
    public MutantStats? Mutants { get; set; }
    public int? TracesChecked { get; set; }
    public Trace? TraceViolation { get; set; }
    public string? Note { get; set; }
}

public sealed class ScenarioResult
{
    public string ScenarioId { get; set; } = "";
    public ScenarioStatus Status { get; set; }
    public Env? Output { get; set; }
    public string? Error { get; set; }
}

public sealed class IntentResult
{
    public string IntentId { get; set; } = "";
    public string Statement { get; set; } = "";
    public IntentVerdict Verdict { get; set; }
    public List<Evidence> Evidence { get; set; } = [];
    public List<string> Clauses { get; set; } = [];
    public List<string> Assumptions { get; set; } = [];
    public List<string> Scenarios { get; set; } = [];
    /// <summary>1 for proved; 1 - upper bound for evidenced; 0 for violated or gap.</summary>
    public double Confidence { get; set; }
    public List<string> Notes { get; set; } = [];
}

public sealed class VerificationReport
{
    public string Lastro { get; set; } = "verification-report/v0";
    public string ArtifactHash { get; set; } = "";
    public string PlanHash { get; set; } = "";
    public string Model { get; set; } = "";
    public List<ClauseResult> Clauses { get; set; } = [];
    public List<ScenarioResult> Scenarios { get; set; } = [];
    public List<IntentResult> Intents { get; set; } = [];
    public List<Assumption> Assumptions { get; set; } = [];
    public List<BeyondProofItem> BeyondProof { get; set; } = [];
    public List<string> NotPromised { get; set; } = [];
    /// <summary>Contradictions in the spec, returned to the team as decisions.</summary>
    public List<DecisionCard> Decisions { get; set; } = [];
    public Dictionary<IntentVerdict, int> Summary { get; set; } = [];
    public bool Ok { get; set; }
}
