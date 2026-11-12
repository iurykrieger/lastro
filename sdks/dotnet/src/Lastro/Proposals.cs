using System.Globalization;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Text.Json.Serialization;
using System.Text.RegularExpressions;

namespace Lastro;

// Use cases, proposals, intents and proofs (proposal/v1), field for field the TS SDK's
// (sdks/typescript/src/proposal.ts), so the CLIs share one .lastro/ workspace and compute the same hashes.
//
//   prose ─▶ PROPOSAL ─extract─▶ INTENTS (add / modify / remove one use case) ─▶ review ─▶ approve
//         ─derive─▶ PROOFS (pending) ─▶ code (anyone) ─▶ prove (+ every use case's proofs) ─▶ apply

/// <summary>How proposals are reviewed (the <c>review</c> block of <c>.lastro/config.json</c>).</summary>
public sealed class ProposalPolicy
{
    /// <summary>Distinct reviewers who must approve each intent. Default 1.</summary>
    public int Approvals { get; set; } = 1;
    /// <summary>Whether the person who submitted the prose counts as a reviewer. Default true.</summary>
    public bool SubmitterCanApprove { get; set; } = true;
}

/// <summary>A clause ("requires ⇒ ensures" for every input) or a scenario (one input and an expectation).</summary>
public sealed class ProofCheck
{
    /// <summary><c>clause</c> or <c>scenario</c>.</summary>
    public string Kind { get; set; } = "clause";
    public Expr? Requires { get; set; }
    public Expr? Ensures { get; set; }
    public List<string>? Assumptions { get; set; }
    public Env? Input { get; set; }
    public Expr? Expect { get; set; }
}

/// <summary>A deterministic check of one intent. Once it passes and its proposal is applied, it belongs to its use case.</summary>
public sealed class ProofEntry
{
    public string Id { get; set; } = "";
    public string Slug { get; set; } = "";
    public string Name { get; set; } = "";
    public string Description { get; set; } = "";
    public string Intent { get; set; } = "";
    public string UseCase { get; set; } = "";
    public string Model { get; set; } = "";
    public string Subject { get; set; } = "";
    public ProofCheck Check { get; set; } = new();
    /// <summary>The superseded proofs this one takes the place of.</summary>
    public List<string>? Replaces { get; set; }
    /// <summary><c>pending</c>, <c>passed</c> or <c>failed</c>.</summary>
    public string Status { get; set; } = "pending";
    public string? Verdict { get; set; }
    public string? Evidence { get; set; }
}

/// <summary>One thing the application does, as the code implements it, with the proofs that keep it working.</summary>
public sealed class UseCase
{
    public string Id { get; set; } = "";
    public string Slug { get; set; } = "";
    public string Name { get; set; } = "";
    public string Description { get; set; } = "";
    public List<ProofEntry> Proofs { get; set; } = [];
    /// <summary>Applied proposals that added or changed it.</summary>
    public List<string> History { get; set; } = [];
}

public sealed class ProofReplacement
{
    public string Proof { get; set; } = "";
    public string Reason { get; set; } = "";
}

/// <summary>Adds, modifies or removes one use case, in natural language.</summary>
public sealed class ProposalIntent
{
    public string Id { get; set; } = "";
    public string Slug { get; set; } = "";
    public string Name { get; set; } = "";
    public string Description { get; set; } = "";
    /// <summary><c>add</c>, <c>modify</c> or <c>remove</c>.</summary>
    public string Change { get; set; } = "add";
    public string UseCase { get; set; } = "";
    public string Provability { get; set; } = "formal";
    public string? BeyondProofReason { get; set; }
    public SourceSpan Source { get; set; } = new();
    public List<ProofReplacement> Replaces { get; set; } = [];
    /// <summary><c>proposed</c>, <c>approved</c>, <c>amended</c> or <c>rejected</c>.</summary>
    public string Status { get; set; } = "proposed";
    public List<string> Approvals { get; set; } = [];
    public string? ReviewNote { get; set; }
    public string? OriginalDescription { get; set; }
    /// <summary>Kept as written (the TS SDK's bootstrap sets it), so hashes match.</summary>
    public JsonNode? Grounds { get; set; }
}

/// <summary>What a proposal does to one use case.</summary>
public sealed class UseCaseChange
{
    public string UseCase { get; set; } = "";
    public string Slug { get; set; } = "";
    public string Change { get; set; } = "add";
    public string Name { get; set; } = "";
    /// <summary>Copied from the use case by the SDK (modify and remove), never by the LLM.</summary>
    public string? Before { get; set; }
    /// <summary>The complete new description, every intent combined (add and modify).</summary>
    public string? After { get; set; }
    public List<string> Intents { get; set; } = [];
    /// <summary>Set when an intent of this use case was amended or rejected: After must be rewritten.</summary>
    public bool? Stale { get; set; }
}

public sealed class ProposalDecision
{
    public string Id { get; set; } = "";
    public string Slug { get; set; } = "";
    public string Name { get; set; } = "";
    /// <summary><c>extraction</c>, <c>derivation</c> or <c>consistency-check</c>.</summary>
    public string Origin { get; set; } = "extraction";
    public string Question { get; set; } = "";
    public List<DecisionOption> Options { get; set; } = [];
    public int Recommended { get; set; }
    public List<string> Intents { get; set; } = [];
    public int? Chosen { get; set; }
    public string? AnsweredBy { get; set; }
}

public sealed class ThreadComment
{
    public string By { get; set; } = "";
    public string Text { get; set; } = "";
}

/// <summary>A review conversation on an intent, a proof, a use case or the prose.</summary>
public sealed class ReviewThread
{
    public string Id { get; set; } = "";
    public string On { get; set; } = "";
    public string By { get; set; } = "";
    public List<ThreadComment> Comments { get; set; } = [];
    public bool Resolved { get; set; }
    public string? ResolvedBy { get; set; }
}

/// <summary>The input that breaks a proof and what the code returned.</summary>
public sealed class ProofCounterexample
{
    public Env Input { get; set; } = new();
    [JsonIgnore(Condition = JsonIgnoreCondition.Never)]
    public Env? Output { get; set; }
}

/// <summary>One proof's result in a run.</summary>
public sealed class ProofOutcome
{
    public string Proof { get; set; } = "";
    public string UseCase { get; set; } = "";
    public string Intent { get; set; } = "";
    /// <summary><c>proposal</c> (derived for it), <c>regression</c> (a use case's proof) or <c>superseded</c> (replaced by it).</summary>
    public string Role { get; set; } = "proposal";
    public string Verdict { get; set; } = "not-run";
    public bool Passed { get; set; }
    public ProofCounterexample? Counterexample { get; set; }
    public string? Note { get; set; }
}

/// <summary>The result of proving a proposal (or of <c>regress</c>).</summary>
public sealed class ProposalRun
{
    public bool Ok { get; set; }
    public List<ProofOutcome> Outcomes { get; set; } = [];
    public List<string> Regressions { get; set; } = [];
    public List<string> ReplacedStillPass { get; set; } = [];
    public List<string> Unproved { get; set; } = [];
    public bool? EnginesAgree { get; set; }
}

/// <summary>One change: its prose, the intents extracted from it, its review and its proofs.</summary>
public sealed class Proposal
{
    public string Lastro { get; set; } = "proposal/v1";
    public string Id { get; set; } = "";
    public string Slug { get; set; } = "";
    public string Name { get; set; } = "";
    public string Description { get; set; } = "";
    /// <summary><c>open</c>, <c>changes-requested</c>, <c>approved</c>, <c>proving</c>, <c>proved</c>, <c>applied</c> or <c>closed</c>.</summary>
    public string Status { get; set; } = "open";
    public string SubmittedBy { get; set; } = "";
    public string Prose { get; set; } = "";
    public string Baseline { get; set; } = "";
    public List<ProposalIntent> Intents { get; set; } = [];
    public List<UseCaseChange> UseCases { get; set; } = [];
    public List<ProposalDecision> Decisions { get; set; } = [];
    public List<ReviewThread> Threads { get; set; } = [];
    public List<string> NotPromised { get; set; } = [];
    public ProposalPolicy Policy { get; set; } = new();
    public List<ProofEntry> Proofs { get; set; } = [];
    public List<string> Supersedes { get; set; } = [];
    public ProposalRun? LastRun { get; set; }
    public string? Hash { get; set; }
    /// <summary>Kept as written (the TS SDK's bootstrap sets them); approval binds them when present.</summary>
    public JsonNode? Origin { get; set; }
    public JsonNode? Setup { get; set; }
}

/// <summary>Every problem found in an LLM answer or a refused step.</summary>
public sealed class ProposalException(IReadOnlyList<string> problems) : Exception("Invalid proposal:\n - " + string.Join("\n - ", problems))
{
    public IReadOnlyList<string> Problems { get; } = problems;
}

/// <summary>The input of a proposal's extraction.</summary>
public sealed record ProposeInput(string Id, string Prose, string SubmittedBy, IReadOnlyList<UseCase> UseCases, ProposalPolicy Policy, IReadOnlyList<string>? Reserved = null);

/// <summary>What proofs may name: the declared models (in config order) and subjects.</summary>
public sealed record ProofSetup(IReadOnlyDictionary<string, Model> Models, IReadOnlyList<string> Subjects);

/// <summary>The code under proof by subject name, the models, and optionally the Rust prover.</summary>
public sealed record RunOptions(IReadOnlyDictionary<string, Subject> Subjects, IReadOnlyDictionary<string, Model> Models, IRustProver? Rust = null, VerifyOptions? Verify = null);

/// <summary>The result of <c>Proposals.Apply</c>.</summary>
public sealed record Applied(Proposal Proposal, List<UseCase> UseCases, List<string> Written, List<string> Removed, string Baseline);

public static partial class Proposals
{
    public static readonly string[] ChangeKinds = ["add", "modify", "remove"];

    public static readonly Regex Slug = new("^[a-z0-9]+(-[a-z0-9]+)*$", RegexOptions.CultureInvariant);

    private static readonly string[] Reasons = ["external-code", "world-facts", "termination-timing", "human-judgment", "unstated"];

    private static string Pad(int n, int width) => n.ToString(CultureInfo.InvariantCulture).PadLeft(width, '0');

    /// <summary>The next free number after every id ("UC-001" style). Numbers are never reused.</summary>
    public static string NextId(string prefix, IEnumerable<string> ids)
    {
        var re = new Regex($"^{prefix}-(\\d+)$", RegexOptions.CultureInvariant);
        var max = 0;
        foreach (var id in ids)
            if (re.Match(id) is { Success: true } m) max = Math.Max(max, int.Parse(m.Groups[1].Value, CultureInfo.InvariantCulture));
        return $"{prefix}-{Pad(max + 1, 3)}";
    }

    public static string FileNameOf(string id, string slug) => $"{id}-{slug}.json";

    /// <summary>sha256 over the canonical use cases without their proofs: what the prose was read against.</summary>
    public static string BaselineOf(IEnumerable<UseCase> useCases)
    {
        var core = new JsonArray();
        foreach (var u in useCases.OrderBy(u => u.Id, StringComparer.Ordinal))
            core.Add(new JsonObject { ["id"] = u.Id, ["slug"] = u.Slug, ["name"] = u.Name, ["description"] = u.Description });
        return "sha256:" + Canonical.Sha256Hex(JsJson.Canonical(core));
    }

    /// <summary>Whitespace- and case-insensitive search, so a quote with reflowed lines still anchors.</summary>
    public static SourceSpan? Anchor(string prose, string quote)
    {
        var exact = string.IsNullOrEmpty(quote) ? -1 : prose.IndexOf(quote, StringComparison.Ordinal);
        if (exact >= 0) return new SourceSpan { Quote = quote, Start = exact, End = exact + quote.Length };
        var words = quote.Split((char[]?)null, StringSplitOptions.RemoveEmptyEntries);
        if (words.Length == 0) return null;
        var m = Regex.Match(prose, string.Join("\\s+", words.Select(Regex.Escape)), RegexOptions.IgnoreCase | RegexOptions.CultureInvariant);
        return m.Success ? new SourceSpan { Quote = m.Value, Start = m.Index, End = m.Index + m.Length } : null;
    }

    /// <summary>Use-case ids reserved by the adds of proposals that are not applied or closed (excluding <paramref name="except"/>).</summary>
    public static List<string> ReservedUseCaseIds(IEnumerable<Proposal> proposals, string? except = null) =>
        proposals.Where(p => p.Id != except && p.Status != "applied" && p.Status != "closed")
            .SelectMany(p => p.UseCases.Where(c => c.Change == "add").Select(c => c.UseCase)).ToList();

    private static string S(JsonNode? n) => n is JsonValue v && v.GetValueKind() == JsonValueKind.String ? v.GetValue<string>() : "";

    private static int I(JsonNode? n) => n is JsonValue v && v.GetValueKind() == JsonValueKind.Number ? (int)v.GetValue<double>() : 0;

    private static List<JsonNode?> A(JsonNode? n) => n is JsonArray a ? [.. a] : [];

    private static List<DecisionOption> Options(JsonNode? n) =>
        A(n).Select(o => new DecisionOption { Label = S(o?["label"]), Consequence = S(o?["consequence"]) }).ToList();

    private static string ChangeWord(string k) => k == "add" ? "added" : k == "modify" ? "modified" : "removed";

    /// <summary>Builds a proposal from the extract step's answer. Throws <see cref="ProposalException"/> with every problem found.</summary>
    public static Proposal FromExtraction(ProposeInput input, JsonNode? answer)
    {
        var o = answer as JsonObject ?? throw new ProposalException(["the extraction is not a JSON object"]);
        var (id, prose, useCases) = (input.Id, input.Prose, input.UseCases);
        var problems = new List<string>();
        UseCase? BySlug(string s) => useCases.FirstOrDefault(u => u.Slug == s);
        var reserved = new List<(string Slug, string Id)>();
        var next = int.Parse(NextId("UC", useCases.Select(u => u.Id).Concat(input.Reserved ?? []))[3..], CultureInfo.InvariantCulture);
        string? UcId(string slug, string change)
        {
            if (BySlug(slug) is { } e) return e.Id;
            if (change != "add") return null;
            var r = reserved.FirstOrDefault(x => x.Slug == slug);
            if (r.Id is not null) return r.Id;
            var nid = $"UC-{Pad(next++, 3)}";
            reserved.Add((slug, nid));
            return nid;
        }
        var outSlug = S(o["slug"]);
        if (!Slug.IsMatch(outSlug)) problems.Add($"proposal slug \"{outSlug}\" is not kebab-case");
        var seen = new HashSet<string>(StringComparer.Ordinal);
        var intents = new List<ProposalIntent>();
        var n = 0;
        foreach (var d in A(o["intents"]))
        {
            var iid = $"{id}.I-{Pad(++n, 2)}";
            var (slug, change, ucSlug, quote, provability) = (S(d?["slug"]), S(d?["change"]), S(d?["useCase"]), S(d?["quote"]), S(d?["provability"]));
            if (!Slug.IsMatch(slug)) problems.Add($"{iid}: slug \"{slug}\" is not kebab-case");
            if (!seen.Add(slug)) problems.Add($"{iid}: duplicate intent slug \"{slug}\"");
            if (!ChangeKinds.Contains(change)) problems.Add($"{iid}: unknown change \"{change}\"");
            var existing = BySlug(ucSlug);
            if (change == "add" && existing is not null) problems.Add($"{iid}: add names existing use case {existing.Id} ({ucSlug}); an add reserves a new slug");
            if (change != "add" && existing is null) problems.Add($"{iid}: {change} names unknown use case \"{ucSlug}\"");
            if (change == "add" && !Slug.IsMatch(ucSlug)) problems.Add($"{iid}: new use-case slug \"{ucSlug}\" is not kebab-case");
            var source = Anchor(prose, quote);
            if (source is null) problems.Add($"{iid}: quote not found in prose: \"{quote}\"");
            var rawReason = S(d?["beyondProofReason"]);
            var reason = rawReason is "" or "none" ? null : rawReason;
            if (provability == "beyond-proof" && reason is null) problems.Add($"{iid}: beyond-proof intent needs a beyondProofReason");
            if (reason is not null && !Reasons.Contains(reason)) problems.Add($"{iid}: unknown beyondProofReason \"{reason}\"");
            var useCase = UcId(ucSlug, change) ?? ucSlug;
            var replaces = A(d?["replaces"]).Select(r => new ProofReplacement { Proof = S(r?["proof"]), Reason = S(r?["reason"]) }).ToList();
            if (change == "remove" && existing is not null) replaces = existing.Proofs.Select(p => new ProofReplacement { Proof = p.Id, Reason = "the use case is removed" }).ToList();
            else if (change == "add" && replaces.Count > 0) problems.Add($"{iid}: an add cannot replace proofs");
            else
                foreach (var r in replaces)
                {
                    var owner = useCases.FirstOrDefault(u => u.Proofs.Any(p => p.Id == r.Proof));
                    if (owner is null) problems.Add($"{iid}: replaces unknown proof {r.Proof}");
                    else if (owner.Id != useCase) problems.Add($"{iid}: replaces {r.Proof}, which belongs to {owner.Id}, not {useCase}");
                }
            intents.Add(new ProposalIntent
            {
                Id = iid, Slug = slug, Name = S(d?["name"]), Description = S(d?["description"]), Change = change, UseCase = useCase, Provability = provability,
                BeyondProofReason = reason, Source = source ?? new SourceSpan { Quote = quote, Start = 0, End = 0 }, Replaces = replaces,
            });
        }

        // Exactly one UseCaseChange per touched use case; kinds must fit together.
        var decisions = new List<ProposalDecision>();
        var changes = new List<UseCaseChange>();
        var outUseCases = A(o["useCases"]);
        foreach (var uc in intents.Select(i => i.UseCase).Distinct(StringComparer.Ordinal).ToList())
        {
            var its = intents.Where(i => i.UseCase == uc).ToList();
            var slug = reserved.FirstOrDefault(x => x.Id == uc).Slug ?? useCases.FirstOrDefault(u => u.Id == uc)?.Slug ?? uc;
            var entries = outUseCases.Where(c => S(c?["slug"]) == slug).ToList();
            if (entries.Count != 1)
            {
                problems.Add($"{uc} ({slug}): expected exactly one use-case change, got {entries.Count}");
                continue;
            }
            var entry = entries[0];
            var entryChange = S(entry?["change"]);
            var kinds = its.Select(i => i.Change).Distinct(StringComparer.Ordinal).ToList();
            if (kinds.Count > 1)
                decisions.Add(new ProposalDecision
                {
                    Slug = $"change-kind-{slug}", Name = $"How {slug} changes", Origin = "extraction",
                    Question = $"The intents on {uc} ({slug}) disagree on whether it is added, modified or removed. Which one is meant?",
                    Options = kinds.Select(k => new DecisionOption { Label = k, Consequence = $"{uc} is {ChangeWord(k)}; reject the intents that say otherwise." }).ToList(),
                    Recommended = 0, Intents = its.Select(i => i.Id).ToList(),
                });
            else if (entryChange != kinds[0]) problems.Add($"{uc} ({slug}): the use-case change says {entryChange} but its intents say {kinds[0]}");
            var existing = useCases.FirstOrDefault(u => u.Id == uc);
            var after = S(entry?["after"]);
            if (entryChange != "remove" && string.IsNullOrWhiteSpace(after)) problems.Add($"{uc} ({slug}): {entryChange} needs the complete after description");
            var name = S(entry?["name"]);
            changes.Add(new UseCaseChange
            {
                UseCase = uc, Slug = slug, Change = entryChange, Name = name != "" ? name : existing?.Name is { Length: > 0 } en ? en : slug,
                Before = existing is not null && entryChange != "add" ? existing.Description : null,
                After = entryChange != "remove" ? after : null,
                Intents = its.Select(i => i.Id).ToList(),
            });
        }
        foreach (var c in outUseCases)
        {
            var slug = S(c?["slug"]);
            if (!changes.Any(x => x.Slug == slug) && !problems.Any(p => p.Contains($"({slug})", StringComparison.Ordinal))) problems.Add($"use-case change \"{slug}\" has no intent");
        }

        var intentBySlug = intents.ToDictionary(i => i.Slug, i => i.Id, StringComparer.Ordinal);
        foreach (var d in A(o["decisions"]))
        {
            var dslug = S(d?["slug"]) is { Length: > 0 } ds ? ds : $"decision-{decisions.Count + 1}";
            var options = Options(d?["options"]);
            var recommended = I(d?["recommended"]);
            if (options.Count < 2) problems.Add($"decision \"{dslug}\" needs at least two options");
            if (recommended < 0 || recommended >= options.Count) problems.Add($"decision \"{dslug}\": recommended out of range");
            var refs = new List<string>();
            foreach (var x in A(d?["intents"]).Select(S))
            {
                if (intentBySlug.TryGetValue(x, out var iid)) refs.Add(iid);
                else if (intents.Any(i => i.Id == x)) refs.Add(x);
                else problems.Add($"decision \"{dslug}\": unknown intent \"{x}\"");
            }
            var question = S(d?["question"]);
            decisions.Insert(0, new ProposalDecision
            {
                Slug = dslug, Name = S(d?["name"]) is { Length: > 0 } dn ? dn : question, Origin = "extraction", Question = question,
                Options = options, Recommended = recommended, Intents = refs,
            });
        }
        if (intents.Count == 0) problems.Add("the extraction found no intent");
        if (problems.Count > 0) throw new ProposalException(problems);
        for (var k = 0; k < decisions.Count; k++) decisions[k].Id = $"D{k + 1}";
        return new Proposal
        {
            Id = id, Slug = outSlug, Name = S(o["name"]), Description = S(o["description"]), SubmittedBy = input.SubmittedBy, Prose = prose,
            Baseline = BaselineOf(useCases), Intents = intents, UseCases = changes, Decisions = decisions,
            NotPromised = A(o["notPromised"]).Select(S).ToList(),
            Policy = new ProposalPolicy { Approvals = input.Policy.Approvals, SubmitterCanApprove = input.Policy.SubmitterCanApprove },
        };
    }

    private static string Norm(string s) => Regex.Replace(s, "\\s+", " ").Trim().ToLowerInvariant();

    /// <summary>
    /// Carries a previous review over to a fresh extraction of edited prose, like a pull request keeps the
    /// approvals of untouched files. Same rules as the TS SDK's carryProposalReview.
    /// </summary>
    public static Proposal CarryReview(Proposal previous, Proposal next)
    {
        var o = LastroJson.Clone(next);
        string Key(ProposalIntent i, string d) => $"{i.Change}|{i.UseCase}|{Norm(d)}";
        var reviewed = previous.Intents.Where(i => i.Status != "proposed" || i.Approvals.Count > 0).ToList();
        foreach (var it in o.Intents)
        {
            var same = reviewed.FirstOrDefault(p => Key(p, p.Description) == Key(it, it.Description));
            var amended = reviewed.FirstOrDefault(p => p.Status == "amended" && p.OriginalDescription is not null && Key(p, p.OriginalDescription) == Key(it, it.Description) && Norm(p.Source.Quote) == Norm(it.Source.Quote));
            var from = same ?? amended;
            if (from is null) continue;
            if (amended is not null && same is null)
            {
                it.OriginalDescription = it.Description;
                it.Description = amended.Description;
                it.Replaces = LastroJson.Clone(amended.Replaces);
            }
            else if (same?.OriginalDescription is not null) it.OriginalDescription = same.OriginalDescription;
            it.Status = from.Status;
            it.Approvals = [.. from.Approvals];
            if (from.ReviewNote is not null) it.ReviewNote = from.ReviewNote;
        }
        foreach (var d in o.Decisions)
        {
            var prev = previous.Decisions.FirstOrDefault(p => Norm(p.Question) == Norm(d.Question));
            if (prev?.Chosen is { } c && c < d.Options.Count)
            {
                d.Chosen = c;
                if (prev.AnsweredBy is not null) d.AnsweredBy = prev.AnsweredBy;
            }
        }
        o.Threads = LastroJson.Clone(previous.Threads);
        o.Policy = new ProposalPolicy { Approvals = previous.Policy.Approvals, SubmitterCanApprove = previous.Policy.SubmitterCanApprove };
        o.SubmittedBy = previous.SubmittedBy;
        return WithStatus(o);
    }

    // -----------------------------------------------------------------------
    // Review
    // -----------------------------------------------------------------------

    private static bool Counts(ProposalIntent i) => i.Status is "approved" or "amended";

    /// <summary>Approvals that count under the proposal's policy.</summary>
    public static List<string> CountedApprovals(Proposal p, ProposalIntent it) =>
        Counts(it) ? it.Approvals.Where(a => p.Policy.SubmitterCanApprove || a != p.SubmittedBy).ToList() : [];

    /// <summary>Intents that go forward to derivation and implementation.</summary>
    public static List<ProposalIntent> ForwardIntents(Proposal p) => p.Intents.Where(Counts).ToList();

    /// <summary>Use-case changes that still apply (at least one intent not rejected).</summary>
    public static List<UseCaseChange> LiveChanges(Proposal p)
    {
        var live = ForwardIntents(p).Select(i => i.Id).ToHashSet(StringComparer.Ordinal);
        return p.UseCases.Where(c => c.Intents.Any(live.Contains)).ToList();
    }

    /// <summary>What blocks approval: unreviewed intents, missing approvals, stale use cases, open decisions and threads.</summary>
    public static List<string> Pending(Proposal p)
    {
        var o = new List<string>();
        var need = Math.Max(1, p.Policy.Approvals);
        foreach (var it in p.Intents)
        {
            if (it.Status == "rejected") continue;
            var n = CountedApprovals(p, it).Count;
            if (it.Status == "proposed" && it.Approvals.Count == 0) o.Add($"{it.Id} not reviewed");
            else if (n < need) o.Add($"{it.Id} needs {need - n} more approval(s){(!p.Policy.SubmitterCanApprove ? " from someone other than the submitter" : "")}");
        }
        foreach (var c in p.UseCases)
            if (c.Stale == true && c.Intents.Any(i => p.Intents.FirstOrDefault(x => x.Id == i)?.Status != "rejected")) o.Add($"{c.UseCase} after is stale: rewrite it or recombine it");
        foreach (var d in p.Decisions)
            if (d.Chosen is null) o.Add($"{d.Id} unanswered: {d.Question}");
        foreach (var t in p.Threads)
            if (!t.Resolved) o.Add($"{t.Id} open thread on {t.On}");
        return o;
    }

    /// <summary>The part of a proposal that approval binds.</summary>
    private static JsonObject ReviewCore(Proposal p)
    {
        var all = LastroJson.ToNode(p)!.AsObject();
        var core = new JsonObject();
        foreach (var k in new[] { "lastro", "id", "slug", "name", "description", "submittedBy", "prose", "baseline", "intents", "useCases", "decisions", "threads", "notPromised", "policy", "supersedes", "origin", "setup" })
            if (all[k] is { } v) core[k] = v.DeepClone();
        return core;
    }

    public static List<string> SupersedesOf(Proposal p) =>
        ForwardIntents(p).SelectMany(i => i.Replaces.Select(r => r.Proof)).Distinct(StringComparer.Ordinal).OrderBy(x => x, StringComparer.Ordinal).ToList();

    private static string HashOf(Proposal p) => "sha256:" + Canonical.Sha256Hex(JsJson.Canonical(ReviewCore(p)));

    /// <summary>True when the reviewed part has not changed since approval.</summary>
    public static bool IsApproved(Proposal p) => p.Hash is not null && HashOf(p) == p.Hash;

    /// <summary>The status that follows from the proposal's content (applied and closed are kept).</summary>
    public static Proposal WithStatus(Proposal p)
    {
        if (p.Status is "applied" or "closed") return p;
        if (IsApproved(p))
            p.Status = p.Proofs.Count == 0 ? "approved" : p.LastRun is { Ok: true } && p.Proofs.All(x => x.Status == "passed") ? "proved" : "proving";
        else
        {
            p.Hash = null;
            var stale = p.UseCases.Any(c => c.Stale == true);
            var raised = p.Decisions.Any(d => d.Origin != "extraction" && d.Chosen is null);
            p.Status = stale || raised ? "changes-requested" : "open";
        }
        return p;
    }

    public static ProposalReview Review(Proposal p, string reviewer) => new(p, reviewer);

    /// <summary>
    /// Approves a fully reviewed proposal. The hash binds the baseline: if the use cases changed since
    /// extraction, the approval is refused and the prose must be extracted again.
    /// </summary>
    public static Proposal Approve(Proposal p, IReadOnlyList<UseCase> useCases)
    {
        if (IsApproved(p)) return p;
        var pending = Pending(p);
        if (BaselineOf(useCases) != p.Baseline) pending.Add($"the use cases changed since {p.Id} was extracted: run propose again");
        if (pending.Count > 0) throw new ProposalException(pending);
        var o = LastroJson.Clone(p);
        o.Hash = null;
        o.Supersedes = SupersedesOf(o);
        o.Proofs = [];
        o.LastRun = null;
        o.Hash = HashOf(o);
        return WithStatus(o);
    }
}

/// <summary>One person's review of a proposal. Chain the actions, then call <see cref="Done"/>.</summary>
public sealed class ProposalReview
{
    private readonly Proposal _p;
    private readonly string _reviewer;

    public ProposalReview(Proposal proposal, string reviewer)
    {
        if (proposal.Status is "applied" or "closed") throw new InvalidOperationException($"{proposal.Id} is {proposal.Status}");
        if (string.IsNullOrEmpty(reviewer)) throw new InvalidOperationException("review needs the reviewer's name");
        _p = LastroJson.Clone(proposal);
        _reviewer = reviewer;
    }

    private ProposalIntent Intent(string id) =>
        _p.Intents.FirstOrDefault(x => x.Id == id || x.Id == $"{_p.Id}.{id}" || x.Slug == id) ?? throw new InvalidOperationException($"Unknown intent {id}");

    private UseCaseChange Change(string useCase) =>
        _p.UseCases.FirstOrDefault(x => x.UseCase == useCase || x.Slug == useCase) ?? throw new InvalidOperationException($"{useCase} is not touched by {_p.Id}");

    /// <summary>Signs intents. Approving an intent accepts the replacement of the proofs it lists.</summary>
    public ProposalReview Approve(params string[] ids)
    {
        if (_reviewer == _p.SubmittedBy && !_p.Policy.SubmitterCanApprove)
            throw new InvalidOperationException($"{_reviewer} submitted {_p.Id}; under this policy the submitter can amend but not approve");
        foreach (var id in ids)
        {
            var it = Intent(id);
            if (it.Status == "rejected") throw new InvalidOperationException($"{it.Id} is rejected; amend it to bring it back");
            if (it.Status == "proposed") it.Status = "approved";
            if (!it.Approvals.Contains(_reviewer)) it.Approvals.Add(_reviewer);
        }
        return this;
    }

    /// <summary>Approves every intent that is not rejected.</summary>
    public ProposalReview ApproveAll() => Approve([.. _p.Intents.Where(i => i.Status != "rejected").Select(i => i.Id)]);

    /// <summary>Rewrites an intent. It needs approval again; its use case's after becomes stale.</summary>
    public ProposalReview Amend(string id, string description)
    {
        var it = Intent(id);
        it.OriginalDescription ??= it.Description;
        it.Description = description;
        it.Status = "amended";
        it.ReviewNote = $"amended by reviewer ({_reviewer})";
        it.Approvals = [_reviewer];
        Change(it.UseCase).Stale = true;
        return this;
    }

    /// <summary>The intent should not be built. Its use case's after becomes stale.</summary>
    public ProposalReview Reject(string id, string note)
    {
        var it = Intent(id);
        it.Status = "rejected";
        it.ReviewNote = $"{note} ({_reviewer})";
        it.Approvals = [];
        var c = Change(it.UseCase);
        c.Stale = c.Intents.Any(i => _p.Intents.FirstOrDefault(x => x.Id == i)?.Status != "rejected");
        return this;
    }

    /// <summary>Writes a use case's after by hand (or the LLM's recombination). Its intents need review again.</summary>
    public ProposalReview RewriteAfter(string useCase, string after, string? name = null)
    {
        var c = Change(useCase);
        if (c.Change == "remove") throw new InvalidOperationException($"{c.UseCase} is removed; it has no after");
        c.After = after;
        if (!string.IsNullOrEmpty(name)) c.Name = name;
        c.Stale = null;
        foreach (var i in c.Intents)
        {
            var it = Intent(i);
            if (it.Status == "rejected") continue;
            it.Approvals = [];
            if (it.Status == "approved") it.Status = "proposed";
        }
        return this;
    }

    public ProposalReview Answer(string decisionId, int option)
    {
        var d = _p.Decisions.FirstOrDefault(x => x.Id == decisionId || x.Slug == decisionId) ?? throw new InvalidOperationException($"Unknown decision {decisionId}");
        if (option < 0 || option >= d.Options.Count) throw new InvalidOperationException($"Option {option} out of range for {d.Id}");
        d.Chosen = option;
        d.AnsweredBy = _reviewer;
        return this;
    }

    /// <summary>Starts a thread on an intent, a proof, a use case or the prose.</summary>
    public ProposalReview Comment(string on, string text)
    {
        var full = $"{_p.Id}.{on}";
        var target = on == "prose" ? on
            : _p.Intents.FirstOrDefault(i => i.Id == on || i.Id == full || i.Slug == on)?.Id
              ?? _p.Proofs.FirstOrDefault(x => x.Id == on || x.Id == full || x.Slug == on)?.Id
              ?? _p.UseCases.FirstOrDefault(c => c.UseCase == on || c.Slug == on)?.UseCase
              ?? throw new InvalidOperationException($"Nothing called {on} in {_p.Id}");
        _p.Threads.Add(new ReviewThread { Id = $"T{_p.Threads.Count + 1}", On = target, By = _reviewer, Comments = [new ThreadComment { By = _reviewer, Text = text }] });
        return this;
    }

    private ReviewThread Thread(string id) => _p.Threads.FirstOrDefault(x => x.Id == id) ?? throw new InvalidOperationException($"Unknown thread {id}");

    public ProposalReview Reply(string threadId, string text)
    {
        Thread(threadId).Comments.Add(new ThreadComment { By = _reviewer, Text = text });
        return this;
    }

    /// <summary>Threads are resolved by whoever started them or by the submitter.</summary>
    public ProposalReview Resolve(string threadId)
    {
        var t = Thread(threadId);
        if (_reviewer != t.By && _reviewer != _p.SubmittedBy) throw new InvalidOperationException($"{t.Id} can be resolved by {t.By} or {_p.SubmittedBy}");
        t.Resolved = true;
        t.ResolvedBy = _reviewer;
        return this;
    }

    /// <summary>Ends the review. Any change after approval withdraws it: derived proofs and runs go with it.</summary>
    public Proposal Done()
    {
        var o = LastroJson.Clone(_p);
        o.Supersedes = Proposals.SupersedesOf(o);
        if (o.Hash is not null && !Proposals.IsApproved(o))
        {
            o.Hash = null;
            o.Proofs = [];
            o.LastRun = null;
        }
        return Proposals.WithStatus(o);
    }
}
