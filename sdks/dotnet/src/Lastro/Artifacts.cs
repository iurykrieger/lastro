using System.Text.RegularExpressions;

namespace Lastro;

/// <summary>An intent as an extractor (or a reviewer adding one) proposes it.</summary>
public sealed record IntentDraft(string Statement, IntentKind Kind, Provability Provability, string Quote, BeyondProofReason? BeyondProofReason = null);

/// <summary>A decision as an extractor proposes it.</summary>
public sealed record DecisionDraft(IReadOnlyList<string> IntentIds, string Question, IReadOnlyList<DecisionOption> Options, int Recommended);

/// <summary>Optional changes to an intent's classification when amending it.</summary>
public sealed record IntentPatch(IntentKind? Kind = null, Provability? Provability = null, BeyondProofReason? BeyondProofReason = null);

public sealed class ArtifactException(IReadOnlyList<string> problems)
    : Exception($"Invalid intent artifact:\n - {string.Join("\n - ", problems)}")
{
    public IReadOnlyList<string> Problems { get; } = problems;
}

/// <summary>
/// Intent artifacts: creation, review rules, freezing and re-extraction.
/// Extraction is non-deterministic; review is where a human catches "the AI misunderstood";
/// <see cref="Freeze"/> pins the reviewed artifact with a content hash, and everything after is bound to it.
/// </summary>
public static class Artifacts
{
    /// <summary>Builds an artifact from extractor output. Quotes are anchored to the prose (intent map).</summary>
    public static IntentArtifact Create(string prose, IReadOnlyList<IntentDraft> drafts, IReadOnlyList<DecisionDraft>? decisions = null, IReadOnlyList<string>? notPromised = null)
    {
        var problems = new List<string>();
        var intents = new List<Intent>();
        for (var i = 0; i < drafts.Count; i++)
        {
            var d = drafts[i];
            var id = $"I{i + 1}";
            var start = Locate(prose, d.Quote);
            if (start < 0) problems.Add($"{id}: quote not found in prose: \"{d.Quote}\"");
            if (d.Provability == Provability.BeyondProof && d.BeyondProofReason is null)
                problems.Add($"{id}: beyond-proof intent needs a beyondProofReason");
            intents.Add(new Intent
            {
                Id = id,
                Statement = d.Statement,
                Kind = d.Kind,
                Provability = d.Provability,
                BeyondProofReason = d.BeyondProofReason,
                Source = new SourceSpan { Quote = d.Quote, Start = Math.Max(start, 0), End = Math.Max(start, 0) + d.Quote.Length },
                Status = IntentStatus.Proposed,
            });
        }
        var ids = intents.Select(x => x.Id).ToHashSet(StringComparer.Ordinal);
        var cards = new List<DecisionCard>();
        var ds = decisions ?? [];
        for (var i = 0; i < ds.Count; i++)
        {
            var d = ds[i];
            var id = $"D{i + 1}";
            if (d.Options.Count < 2) problems.Add($"{id}: a decision needs at least two options");
            if (d.Recommended < 0 || d.Recommended >= d.Options.Count) problems.Add($"{id}: recommended out of range");
            foreach (var r in d.IntentIds) if (!ids.Contains(r)) problems.Add($"{id}: unknown intent {r}");
            cards.Add(new DecisionCard
            {
                Id = id,
                IntentIds = [.. d.IntentIds],
                Question = d.Question,
                Options = d.Options.Select(o => new DecisionOption { Label = o.Label, Consequence = o.Consequence }).ToList(),
                Recommended = d.Recommended,
                Origin = DecisionOrigin.Extraction,
            });
        }
        if (problems.Count > 0) throw new ArtifactException(problems);
        return new IntentArtifact { Prose = prose, Intents = intents, Decisions = cards, NotPromised = [.. notPromised ?? []] };
    }

    private static readonly Regex Ws = new(@"\s+", RegexOptions.Compiled);

    internal static string Norm(string s) => Ws.Replace(s, " ").Trim().ToLowerInvariant();

    /// <summary>Whitespace-insensitive search, so a quote with reflowed lines still anchors.</summary>
    internal static int Locate(string prose, string quote)
    {
        var exact = prose.IndexOf(quote, StringComparison.Ordinal);
        if (exact >= 0) return exact;
        var q = Norm(quote);
        if (q.Length == 0) return -1;
        for (var i = 0; i < prose.Length; i++)
        {
            if (i > 0 && !char.IsWhiteSpace(prose[i - 1])) continue;
            var slice = prose.Substring(i, Math.Min(quote.Length * 2, prose.Length - i));
            if (Norm(slice).StartsWith(q, StringComparison.Ordinal)) return i;
        }
        return -1;
    }

    /// <summary>Starts a review as one person. <paramref name="reviewer"/> is required when the policy asks for several approvals.</summary>
    public static Review Review(IntentArtifact artifact, string? reviewer = null) => new(artifact, reviewer);

    public static int RequiredApprovals(IntentArtifact a) => Math.Max(1, a.Policy?.RequiredApprovals ?? 1);

    /// <summary>Approvals an intent has collected (legacy single-reviewer approvals count as one).</summary>
    public static int ApprovalCount(Intent it) =>
        it.Status is not (IntentStatus.Approved or IntentStatus.Amended) ? 0 : it.Approvals?.Count ?? 1;

    /// <summary>What blocks freezing: intents without enough approvals and unanswered decisions.</summary>
    public static List<string> PendingReview(IntentArtifact a)
    {
        var p = new List<string>();
        var need = RequiredApprovals(a);
        foreach (var it in a.Intents)
        {
            if (it.Status == IntentStatus.Proposed && (it.Approvals is null || it.Approvals.Count == 0)) p.Add($"{it.Id} not reviewed");
            else if (it.Status != IntentStatus.Rejected && ApprovalCount(it) < need) p.Add($"{it.Id} needs {need - ApprovalCount(it)} more approval(s)");
        }
        foreach (var d in a.Decisions) if (d.Chosen is null) p.Add($"{d.Id} unanswered: {d.Question}");
        return p;
    }

    public static IntentArtifact Freeze(IntentArtifact a)
    {
        var pending = PendingReview(a);
        if (pending.Count > 0) throw new ArtifactException(pending);
        var copy = LastroJson.Clone(a);
        copy.Hash = null;
        copy.Hash = Canonical.HashOf(copy);
        return copy;
    }

    public static IntentArtifact Thaw(IntentArtifact a)
    {
        var copy = LastroJson.Clone(a);
        copy.Hash = null;
        return copy;
    }

    /// <summary>True when the artifact was not modified after <see cref="Freeze"/>.</summary>
    public static bool IsIntact(IntentArtifact a)
    {
        if (a.Hash is null) return false;
        var copy = LastroJson.Clone(a);
        copy.Hash = null;
        return Canonical.HashOf(copy) == a.Hash;
    }

    public static List<Intent> ApprovedIntents(IntentArtifact a)
    {
        var need = RequiredApprovals(a);
        return a.Intents.Where(x => ApprovalCount(x) >= need).ToList();
    }

    /// <summary>
    /// Sends spec problems back to the team: contradictions (or any decision raised after derivation)
    /// become unanswered decision cards on a thawed copy, so it cannot be frozen until someone answers them.
    /// </summary>
    public static IntentArtifact RaiseDecisions(IntentArtifact a, IEnumerable<DecisionCard> cards)
    {
        var output = Thaw(a);
        var known = output.Decisions.Select(d => d.Question).ToHashSet(StringComparer.Ordinal);
        foreach (var c in cards)
        {
            if (known.Contains(c.Question)) continue;
            var copy = LastroJson.Clone(c);
            copy.Id = $"D{output.Decisions.Count + 1}";
            copy.Chosen = null;
            output.Decisions.Add(copy);
        }
        return output;
    }

    /// <summary>
    /// Carries a previous review over to a fresh extraction of edited prose. Intents whose statement
    /// matches a reviewed one keep its verdict; an amendment is reapplied when the AI repeats its old
    /// reading of the same quote; decisions with the same question keep their answer.
    /// </summary>
    public static IntentArtifact CarryReview(IntentArtifact previous, IntentArtifact next)
    {
        var output = LastroJson.Clone(next);
        var reviewed = previous.Intents.Where(i => i.Status != IntentStatus.Proposed).ToList();
        foreach (var it in output.Intents)
        {
            var same = reviewed.FirstOrDefault(p => Norm(p.Statement) == Norm(it.Statement));
            var amended = reviewed.FirstOrDefault(p =>
                p.Status == IntentStatus.Amended && !string.IsNullOrEmpty(p.OriginalStatement)
                && Norm(p.OriginalStatement) == Norm(it.Statement) && Norm(p.Source.Quote) == Norm(it.Source.Quote));
            if (same is not null)
            {
                it.Status = same.Status;
                if (same.Approvals is not null) it.Approvals = [.. same.Approvals];
                if (!string.IsNullOrEmpty(same.ReviewNote)) it.ReviewNote = same.ReviewNote;
                if (!string.IsNullOrEmpty(same.OriginalStatement)) it.OriginalStatement = same.OriginalStatement;
            }
            else if (amended is not null)
            {
                it.OriginalStatement = it.Statement;
                it.Statement = amended.Statement;
                it.Status = IntentStatus.Amended;
                if (amended.Approvals is not null) it.Approvals = [.. amended.Approvals];
                if (!string.IsNullOrEmpty(amended.ReviewNote)) it.ReviewNote = amended.ReviewNote;
            }
        }
        foreach (var d in output.Decisions)
        {
            var prev = previous.Decisions.FirstOrDefault(p => Norm(p.Question) == Norm(d.Question));
            if (prev?.Chosen is { } chosen && chosen < d.Options.Count) d.Chosen = chosen;
        }
        if (previous.Policy is not null) output.Policy = new ReviewPolicy { RequiredApprovals = previous.Policy.RequiredApprovals };
        return output;
    }
}

/// <summary>One person's review of an artifact. Chain approve/amend/reject/answer, then <see cref="Done"/>.</summary>
public sealed class Review
{
    private readonly IntentArtifact _a;
    private readonly string? _reviewer;

    public Review(IntentArtifact artifact, string? reviewer = null)
    {
        if (artifact.Hash is not null) throw new InvalidOperationException("Artifact is frozen; review a copy created with Artifacts.Thaw()");
        _a = LastroJson.Clone(artifact);
        _reviewer = reviewer;
    }

    private void Sign(Intent it, bool reset = false)
    {
        if (_reviewer is null) return;
        var prev = reset ? [] : it.Approvals ?? [];
        it.Approvals = prev.Contains(_reviewer) ? prev : [.. prev, _reviewer];
    }

    private Intent Find(string id) => _a.Intents.FirstOrDefault(x => x.Id == id) ?? throw new KeyNotFoundException($"Unknown intent {id}");

    public Review Approve(params string[] ids)
    {
        foreach (var id in ids)
        {
            var it = Find(id);
            if (it.Status != IntentStatus.Amended) it.Status = IntentStatus.Approved;
            Sign(it);
        }
        return this;
    }

    /// <summary>Approves every intent this reviewer has not signed yet (rejected ones stay rejected).</summary>
    public Review ApproveAll()
    {
        foreach (var it in _a.Intents.ToList()) if (it.Status != IntentStatus.Rejected) Approve(it.Id);
        return this;
    }

    /// <summary>The AI got it wrong and the intent must not be built at all.</summary>
    public Review Reject(string id, string note)
    {
        var it = Find(id);
        it.Status = IntentStatus.Rejected;
        it.ReviewNote = _reviewer is not null ? $"{note} ({_reviewer})" : note;
        it.Approvals = null;
        return this;
    }

    /// <summary>The AI got it partially wrong: rewrite the statement. Amended intents count as approved; other approvals reset.</summary>
    public Review Amend(string id, string statement, IntentPatch? patch = null, string note = "amended by reviewer")
    {
        var it = Find(id);
        it.OriginalStatement ??= it.Statement;
        it.Statement = statement;
        if (patch?.Kind is { } k) it.Kind = k;
        if (patch?.Provability is { } p) it.Provability = p;
        if (patch?.BeyondProofReason is { } r) it.BeyondProofReason = r;
        it.Status = IntentStatus.Amended;
        it.ReviewNote = note;
        Sign(it, reset: true);
        return this;
    }

    /// <summary>Adds an intent the extractor missed (an unstated requirement made explicit).</summary>
    public Review Add(IntentDraft draft)
    {
        var start = Artifacts.Locate(_a.Prose, draft.Quote);
        _a.Intents.Add(new Intent
        {
            Id = $"I{_a.Intents.Count + 1}",
            Statement = draft.Statement,
            Kind = draft.Kind,
            Provability = draft.Provability,
            BeyondProofReason = draft.BeyondProofReason,
            Source = new SourceSpan { Quote = draft.Quote, Start = Math.Max(start, 0), End = Math.Max(start, 0) + draft.Quote.Length },
            Status = IntentStatus.Approved,
            ReviewNote = start < 0 ? "added by reviewer (not in prose)" : "added by reviewer",
        });
        return this;
    }

    /// <summary>
    /// Answers a decision. A decision raised after extraction (a contradiction in the spec) changes
    /// what is promised: its answer becomes an intent, which the team approves like any other.
    /// </summary>
    public Review Answer(string decisionId, int option)
    {
        var d = _a.Decisions.FirstOrDefault(x => x.Id == decisionId) ?? throw new KeyNotFoundException($"Unknown decision {decisionId}");
        if (option < 0 || option >= d.Options.Count) throw new ArgumentOutOfRangeException(nameof(option), $"Option {option} out of range for {decisionId}");
        d.Chosen = option;
        if (d.Origin != DecisionOrigin.Extraction)
        {
            var o = d.Options[option];
            var statement = $"{d.Question} Decided: {o.Label}{(string.IsNullOrEmpty(o.Consequence) ? "" : $" ({o.Consequence})")}.";
            var existing = _a.Intents.FirstOrDefault(x => x.DecisionId == d.Id);
            var it = existing ?? new Intent
            {
                Id = $"I{_a.Intents.Count + 1}",
                Statement = statement,
                Kind = IntentKind.Behavior,
                Provability = Provability.Formal,
                Source = new SourceSpan { Quote = "", Start = 0, End = 0 },
                Status = IntentStatus.Proposed,
                DecisionId = d.Id,
            };
            it.Statement = statement;
            it.Status = IntentStatus.Proposed;
            it.ReviewNote = $"answer to decision {d.Id}";
            it.Approvals = null;
            it.OriginalStatement = null;
            if (existing is null) _a.Intents.Add(it);
        }
        return this;
    }

    public Review NotPromised(params string[] items)
    {
        _a.NotPromised.AddRange(items);
        return this;
    }

    public IntentArtifact Done() => LastroJson.Clone(_a);
}
