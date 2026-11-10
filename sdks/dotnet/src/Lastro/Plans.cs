namespace Lastro;

public sealed record PlanCheck(IReadOnlyList<string> Errors, IReadOnlyList<string> Uncovered);

public sealed class PlanException(IReadOnlyList<string> errors)
    : Exception($"Invalid proof plan:\n - {string.Join("\n - ", errors)}")
{
    public IReadOnlyList<string> Errors { get; } = errors;
}

/// <summary>Deterministic checks on a derived plan. The deriver may be an LLM; this keeps its output honest.</summary>
public static class Plans
{
    public static PlanCheck Check(IntentArtifact artifact, ProofPlan plan)
    {
        var errors = new List<string>();
        if (artifact.Hash is null || !Artifacts.IsIntact(artifact)) errors.Add("artifact is not frozen or was modified after freeze()");
        if (plan.ArtifactHash != artifact.Hash) errors.Add($"plan was derived from {plan.ArtifactHash}, artifact is {artifact.Hash ?? "undefined"}");

        var approved = Artifacts.ApprovedIntents(artifact).Select(i => i.Id).ToList();
        var approvedSet = approved.ToHashSet(StringComparer.Ordinal);
        var known = artifact.Intents.Select(i => i.Id).ToHashSet(StringComparer.Ordinal);
        void RefIntents(string where, IReadOnlyList<string> ids)
        {
            if (ids.Count == 0) errors.Add($"{where}: must reference at least one intent");
            foreach (var id in ids)
            {
                if (!known.Contains(id)) errors.Add($"{where}: unknown intent {id}");
                else if (!approvedSet.Contains(id)) errors.Add($"{where}: intent {id} was not approved (rejected intents must not be built)");
            }
        }

        var seen = new HashSet<string>(StringComparer.Ordinal);
        void Unique(string id)
        {
            if (!seen.Add(id)) errors.Add($"duplicate id {id}");
        }

        var model = plan.Model;
        if (model.Outputs.Count == 0) errors.Add("model has no outputs");
        foreach (var (name, def) in model.Defs ?? []) errors.AddRange(Lsl.Validate(def.Body, model, def.Params, $"def {name}"));

        var assumptionIds = plan.Assumptions.Select(a => a.Id).ToHashSet(StringComparer.Ordinal);
        var scenarioIds = plan.Scenarios.Select(s => s.Id).ToHashSet(StringComparer.Ordinal);

        foreach (var c in plan.Clauses)
        {
            Unique(c.Id);
            RefIntents(c.Id, c.IntentIds);
            if (c.Requires is not null) errors.AddRange(Lsl.Validate(c.Requires, model, null, $"{c.Id}.requires"));
            errors.AddRange(Lsl.Validate(c.Ensures, model, null, $"{c.Id}.ensures"));
            foreach (var t in c.Assumes ?? []) if (!assumptionIds.Contains(t)) errors.Add($"{c.Id}: unknown assumption {t}");
        }
        foreach (var a in plan.Assumptions)
        {
            Unique(a.Id);
            RefIntents(a.Id, a.IntentIds);
            foreach (var s in a.CheckedBy) if (!scenarioIds.Contains(s)) errors.Add($"{a.Id}: checkedBy unknown scenario {s}");
        }
        foreach (var s in plan.Scenarios)
        {
            Unique(s.Id);
            RefIntents(s.Id, s.IntentIds);
            errors.AddRange(Lsl.Validate(s.Expect, model, null, $"{s.Id}.expect"));
            foreach (var (k, d) in model.Inputs)
            {
                if (!s.Input.TryGetValue(k, out var v)) errors.Add($"{s.Id}: input '{k}' missing");
                else if (!Domains.Contains(d, v)) errors.Add($"{s.Id}: input '{k}'={v} outside its domain");
            }
        }
        foreach (var b in plan.BeyondProof) RefIntents($"beyondProof({b.IntentId})", [b.IntentId]);

        var covered = new HashSet<string>(StringComparer.Ordinal);
        foreach (var ids in plan.Clauses.Select(c => c.IntentIds).Concat(plan.Assumptions.Select(a => a.IntentIds)).Concat(plan.Scenarios.Select(s => s.IntentIds)))
            covered.UnionWith(ids);
        foreach (var b in plan.BeyondProof) covered.Add(b.IntentId);
        var uncovered = approved.Where(i => !covered.Contains(i)).ToList();
        return new PlanCheck(errors, uncovered);
    }

    /// <summary>Throws on structural errors; returns approved intents with no obligation (reported as gaps).</summary>
    public static IReadOnlyList<string> Validate(IntentArtifact artifact, ProofPlan plan)
    {
        var r = Check(artifact, plan);
        if (r.Errors.Count > 0) throw new PlanException(r.Errors);
        return r.Uncovered;
    }
}
