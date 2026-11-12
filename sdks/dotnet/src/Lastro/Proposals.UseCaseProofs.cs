using System.Globalization;
using System.Text.Json.Nodes;
using System.Text.RegularExpressions;

namespace Lastro;

/// <summary>The use cases derive --usecases worked on, with their new pending proofs.</summary>
public sealed class UseCaseDerivation
{
    /// <summary>The targets with their new pending proofs (earlier pending or failed ones are replaced).</summary>
    public List<UseCase> UseCases { get; set; } = [];
    public List<ProofEntry> Derived { get; set; } = [];
    public List<Unprovable> Unprovable { get; set; } = [];
}

/// <summary>A target use case the declared models cannot express, with the reason.</summary>
public sealed class Unprovable
{
    public string UseCase { get; set; } = "";
    public string Reason { get; set; } = "";
}

/// <summary>The result of proving the use cases' proofs (<c>prove --usecases</c>).</summary>
public sealed class UseCaseRun
{
    public bool Ok { get; set; }
    public List<ProofOutcome> Outcomes { get; set; } = [];
    /// <summary>Use-case proofs (pending or failed before the run) that failed: they go to the user, never to the code.</summary>
    public List<string> Failed { get; set; } = [];
    public List<string> Regressions { get; set; } = [];
    public List<string> Unproved { get; set; } = [];
    public bool? EnginesAgree { get; set; }
    public string? Note { get; set; }
}

// Proofs derived straight from the established use cases (P-004), ported from the TS SDK's usecase-proofs.ts.
// A use case's description was already reviewed: these proofs need no proposal and no intents. They are numbered
// inside their use case (UC-003.PF-01), start pending, and become regression tests once they pass.
public static partial class Proposals
{
    /// <summary>Printed with every failing use-case proof: it never goes back to the code.</summary>
    public const string UseCaseFeedbackNote =
        "These proofs come from the use cases' own descriptions, and the code is the reference: a failing one means the description is wrong (change it with a proposal) or the code has a bug the user decides about. Do not change the code to make it pass.";

    private static readonly Regex UseCaseId = new(@"^UC-\d+$", RegexOptions.CultureInvariant);

    /// <summary>A use case's passed proofs: its regression tests. Pending and failed ones are not.</summary>
    public static List<ProofEntry> PassedProofs(UseCase u) => u.Proofs.Where(x => x.Status == "passed").ToList();

    /// <summary>True when <paramref name="s"/> is a comma-separated list of use case ids (<c>UC-003,UC-016</c>).</summary>
    public static bool IsUseCaseList(string? s) => !string.IsNullOrEmpty(s) && s.Split(',').All(x => UseCaseId.IsMatch(x.Trim()));

    /// <summary>
    /// The use cases derive --usecases works on: without ids, every use case with no passed proof; with ids, those,
    /// refusing any that already has passed proofs (its proofs change only through a proposal).
    /// </summary>
    public static List<UseCase> UseCaseTargets(IReadOnlyList<UseCase> useCases, IReadOnlyList<string>? ids = null)
    {
        if (ids is null || ids.Count == 0) return useCases.Where(u => PassedProofs(u).Count == 0).ToList();
        var problems = new List<string>();
        var o = new List<UseCase>();
        foreach (var id in ids)
        {
            var u = useCases.FirstOrDefault(x => x.Id == id || x.Slug == id);
            if (u is null) problems.Add($"no use case {id}");
            else if (PassedProofs(u).Count > 0) problems.Add($"{u.Id} already has proofs; they change only through a proposal");
            else if (!o.Contains(u)) o.Add(u);
        }
        if (problems.Count > 0) throw new ProposalException(problems);
        return o;
    }

    private static int NextProofNumber(UseCase u)
    {
        var re = new Regex($"^{Regex.Escape(u.Id)}\\.PF-(\\d+)$", RegexOptions.CultureInvariant);
        var nums = u.Proofs.Select(x => re.Match(x.Id)).Where(m => m.Success).Select(m => int.Parse(m.Groups[1].Value, CultureInfo.InvariantCulture)).ToList();
        return (nums.Count > 0 ? nums.Max() : 0) + 1;
    }

    /// <summary>
    /// Checks the derive step's answer for <paramref name="targets"/> and attaches the pending proofs. Every target gets
    /// at least one proof or is listed as unprovable; anything else is refused, with every problem listed.
    /// </summary>
    public static UseCaseDerivation FromUseCaseDerivation(IReadOnlyList<UseCase> useCases, IReadOnlyList<string> targets, ProofSetup setup, JsonNode? answer)
    {
        var problems = new List<string>();
        var wanted = targets.ToHashSet(StringComparer.Ordinal);
        var byId = useCases.Where(u => wanted.Contains(u.Id)).ToList();
        foreach (var t in targets)
            if (byId.All(u => u.Id != t)) problems.Add($"no use case {t}");
        var next = byId.ToDictionary(u => u.Id, NextProofNumber, StringComparer.Ordinal);
        var slugs = new Dictionary<string, HashSet<string>>(StringComparer.Ordinal);
        var derived = new List<ProofEntry>();
        var answered = A(answer?["proofs"]);
        foreach (var d in answered)
        {
            var (ucId, slug, modelName, subject) = (S(d?["useCase"]), S(d?["slug"]), S(d?["model"]), S(d?["subject"]));
            var u = byId.FirstOrDefault(x => x.Id == ucId);
            if (u is null)
            {
                problems.Add($"proof \"{slug}\": {ucId} is not one of the use cases to prove ({(targets.Count > 0 ? string.Join(", ", targets) : "none")})");
                continue;
            }
            var pid = $"{u.Id}.PF-{Pad(next[u.Id]++, 2)}";
            if (!Slug.IsMatch(slug)) problems.Add($"{pid}: slug \"{slug}\" is not kebab-case");
            if (!slugs.TryGetValue(u.Id, out var seen)) slugs[u.Id] = seen = new HashSet<string>(StringComparer.Ordinal);
            if (!seen.Add(slug)) problems.Add($"{pid}: duplicate proof slug \"{slug}\"");
            var hasModel = setup.Models.TryGetValue(modelName, out var model);
            if (!hasModel) problems.Add($"{pid}: unknown model \"{modelName}\" (declared: {Declared(setup.Models.Keys)})");
            if (!setup.Subjects.Contains(subject)) problems.Add($"{pid}: unknown subject \"{subject}\" (declared: {Declared(setup.Subjects)})");
            if (!hasModel) continue;
            if (CheckFromAnswer(pid, d, model!, problems) is { } check)
                derived.Add(new ProofEntry { Id = pid, Slug = slug, Name = S(d?["name"]), Description = S(d?["description"]), UseCase = u.Id, Model = modelName, Subject = subject, Check = check, Status = "pending" });
        }
        var unprovable = new List<Unprovable>();
        foreach (var x in A(answer?["unprovable"]))
        {
            var (ucId, reason) = (S(x?["useCase"]), S(x?["reason"]));
            if (byId.All(u => u.Id != ucId)) problems.Add($"unprovable {ucId}: not one of the use cases to prove");
            else if (string.IsNullOrWhiteSpace(reason)) problems.Add($"unprovable {ucId}: needs a reason");
            else unprovable.Add(new Unprovable { UseCase = ucId, Reason = reason });
        }
        foreach (var u in byId)
        {
            var has = answered.Any(d => S(d?["useCase"]) == u.Id);
            var listed = unprovable.Any(x => x.UseCase == u.Id);
            if (!has && !listed) problems.Add($"{u.Id}: no proof derived and not listed as unprovable");
            if (has && listed) problems.Add($"{u.Id}: has proofs and is also listed as unprovable");
        }
        if (problems.Count > 0) throw new ProposalException(problems);
        var updated = byId.Select(u =>
        {
            var v = LastroJson.Clone(u);
            var mine = derived.Where(x => x.UseCase == u.Id).ToList();
            if (mine.Count > 0 || unprovable.Any(x => x.UseCase == u.Id)) v.Proofs = [.. PassedProofs(v), .. mine.Select(LastroJson.Clone)];
            return v;
        }).ToList();
        return new UseCaseDerivation { UseCases = updated, Derived = derived, Unprovable = unprovable };
    }

    /// <summary>The user turn of the derive step for established use cases, built like the TS SDK's.</summary>
    public static string DeriveUseCaseProofsUserPrompt(IReadOnlyList<UseCase> targets, IReadOnlyList<UseCase> useCases, ProofSetup setup)
    {
        var wanted = targets.Select(u => u.Id).ToHashSet(StringComparer.Ordinal);
        var models = new JsonObject();
        foreach (var (name, model) in setup.Models) models[name] = LastroJson.ToNode(model);
        var toProve = new JsonArray([.. targets.Select(u => (JsonNode)new JsonObject { ["id"] = u.Id, ["name"] = u.Name, ["description"] = u.Description })]);
        var proven = new JsonArray([.. useCases.Where(u => !wanted.Contains(u.Id) && PassedProofs(u).Count > 0).Select(u =>
        {
            var v = LastroJson.Clone(u);
            v.Proofs = PassedProofs(u);
            return (JsonNode)new JsonObject { ["id"] = u.Id, ["name"] = u.Name, ["proofs"] = ProofView(v) };
        })]);
        return JsJson.Stringify(new JsonObject
        {
            ["models"] = models, ["subjects"] = new JsonArray([.. setup.Subjects.Select(s => (JsonNode)JsonValue.Create(s)!)]),
            ["useCasesToProve"] = toProve, ["provenUseCases"] = proven,
        }, 2);
    }

    public static LlmRequest DeriveUseCaseProofsRequest(IReadOnlyList<UseCase> targets, IReadOnlyList<UseCase> useCases, ProofSetup setup) =>
        new("derive", LlmPrompts.DeriveUseCaseProofsSystem, DeriveUseCaseProofsUserPrompt(targets, useCases, setup), (JsonObject)JsonNode.Parse(LlmPrompts.DeriveUseCaseProofsSchemaJson)!);

    /// <summary>Asks the LLM for proofs of established use cases (every one with no passed proof, or ids) and checks its answer.</summary>
    public static async Task<UseCaseDerivation> DeriveUseCasesAsync(AskJson ask, IReadOnlyList<UseCase> useCases, IReadOnlyList<string>? ids, ProofSetup setup, CancellationToken cancellationToken = default)
    {
        var targets = UseCaseTargets(useCases, ids);
        if (targets.Count == 0) return new UseCaseDerivation();
        var answer = await ask(DeriveUseCaseProofsRequest(targets, useCases, setup), cancellationToken).ConfigureAwait(false);
        return FromUseCaseDerivation(useCases, [.. targets.Select(u => u.Id)], setup, answer);
    }

    /// <summary>
    /// Runs every use case's pending and failed proofs (role <c>usecase</c>) with every passed one (role
    /// <c>regression</c>). A use-case proof that passes becomes passed; one that fails becomes failed. Returns the use
    /// cases whose proofs changed.
    /// </summary>
    public static async Task<(List<UseCase> UseCases, UseCaseRun Run)> ProveUseCasesAsync(IReadOnlyList<UseCase> useCases, RunOptions o)
    {
        var set = useCases.SelectMany(u => u.Proofs).Select(x => (x, x.Status == "passed" ? "regression" : "usecase")).ToList();
        var (outcomes, agree) = await RunProofsAsync(set, o).ConfigureAwait(false);
        var changed = new List<UseCase>();
        var after = new List<UseCase>();
        foreach (var u in useCases)
        {
            if (u.Proofs.All(x => x.Status == "passed"))
            {
                after.Add(u);
                continue;
            }
            var v = LastroJson.Clone(u);
            foreach (var pr in v.Proofs.Where(x => x.Status != "passed"))
            {
                var r = outcomes.FirstOrDefault(x => x.Proof == pr.Id);
                if (r is null) continue;
                pr.Status = r.Passed ? "passed" : "failed";
                pr.Verdict = r.Verdict;
                pr.Evidence = r.Verdict switch { "proved" => "formal", "not-run" => "none", _ => "empirical" };
            }
            changed.Add(v);
            after.Add(v);
        }
        var run = new UseCaseRun
        {
            Outcomes = outcomes,
            Failed = outcomes.Where(x => x.Role == "usecase" && !x.Passed).Select(x => x.Proof).ToList(),
            Regressions = outcomes.Where(x => x.Role == "regression" && !x.Passed).Select(x => x.Proof).ToList(),
            Unproved = after.Where(u => PassedProofs(u).Count == 0).Select(u => u.Id).ToList(),
            EnginesAgree = agree,
        };
        run.Ok = run.Failed.Count == 0 && run.Regressions.Count == 0 && agree != false;
        if (run.Failed.Count > 0) run.Note = UseCaseFeedbackNote;
        return (changed, run);
    }
}
