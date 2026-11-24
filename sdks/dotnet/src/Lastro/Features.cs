namespace Lastro;

// The feature map: how a user or client reaches each feature (its surfaces) and which features it needs (dependsOn).
// A port of the TS SDK's features.ts: same checks, same order, same messages.

/// <summary>One way to reach a feature: its kind, a name in the user's words and a locator a driver understands.</summary>
public sealed class Surface
{
    /// <summary><c>screen</c>, <c>route</c>, <c>command</c> or <c>api</c>.</summary>
    public string Kind { get; set; } = "";
    public string Name { get; set; } = "";
    public string Locator { get; set; } = "";
}

/// <summary>A malformed surface, a dependency on a missing or removed feature, or a dependency cycle.</summary>
public sealed class FeatureMapException(string message, IReadOnlyList<string>? cycle = null) : Exception(message)
{
    /// <summary>The features of the dependency cycle, from the smallest id; empty when there is none.</summary>
    public IReadOnlyList<string> Cycle { get; } = cycle ?? [];
}

/// <summary>What the map needs of a feature.</summary>
public sealed record Linked(string Id, string Slug, string Name, List<Surface>? Surfaces = null, List<string>? DependsOn = null);

/// <summary>One feature in the map.</summary>
public sealed class MapEntry
{
    public string Id { get; set; } = "";
    public string Slug { get; set; } = "";
    public string Name { get; set; } = "";
    public List<Surface> Surfaces { get; set; } = [];
    /// <summary>False when the feature has no surface: the driver cannot reach it.</summary>
    public bool Reachable { get; set; }
    public List<string> DependsOn { get; set; } = [];
    /// <summary>The features that depend on this one directly.</summary>
    public List<string> Dependents { get; set; } = [];
}

/// <summary>Every feature in id order.</summary>
public sealed class FeatureMap
{
    public List<MapEntry> Features { get; set; } = [];
}

/// <summary>What a modify does to a feature's surfaces and dependencies.</summary>
public sealed class LinksChange
{
    public List<Surface> AddSurfaces { get; set; } = [];
    public List<string> RemoveSurfaces { get; set; } = [];
    public List<string> AddDependsOn { get; set; } = [];
    public List<string> RemoveDependsOn { get; set; } = [];
}

public static class Features
{
    public static readonly string[] SurfaceKinds = ["screen", "route", "command", "api"];

    private static int ById(string a, string b) => string.CompareOrdinal(a, b);

    public static List<Linked> LinkedOf(IEnumerable<UseCase> useCases) =>
        useCases.Select(u => new Linked(u.Id, u.Slug, u.Name, u.Surfaces, u.DependsOn)).ToList();

    /// <summary>The problems with one feature's surfaces.</summary>
    public static List<string> SurfaceProblems(string id, IReadOnlyList<Surface>? surfaces)
    {
        var errs = new List<string>();
        for (var i = 0; i < (surfaces?.Count ?? 0); i++)
        {
            var s = surfaces![i];
            var at = $"{id}.surfaces[{i}]";
            if (!SurfaceKinds.Contains(s.Kind)) errs.Add($"{at}: kind must be one of {string.Join(", ", SurfaceKinds)}");
            if (string.IsNullOrWhiteSpace(s.Name)) errs.Add($"{at}: name is required");
            if (string.IsNullOrWhiteSpace(s.Locator)) errs.Add($"{at}: locator is required");
        }
        return errs;
    }

    /// <summary>Checks the features and builds the map; every problem is reported at once.</summary>
    public static FeatureMap Build(IReadOnlyList<Linked> features, IReadOnlyList<string>? removed = null)
    {
        var problems = features.SelectMany(f => SurfaceProblems(f.Id, f.Surfaces)).ToList();
        if (problems.Count > 0) throw new FeatureMapException(string.Join("; ", problems));
        var gone = new HashSet<string>(removed ?? [], StringComparer.Ordinal);
        var live = features.Where(f => !gone.Contains(f.Id)).ToList();
        live.Sort((a, b) => ById(a.Id, b.Id));
        var ids = new HashSet<string>(live.Select(f => f.Id), StringComparer.Ordinal);
        foreach (var f in live)
        {
            var sorted = (f.DependsOn ?? []).ToList();
            sorted.Sort(ById);
            foreach (var d in sorted)
            {
                if (gone.Contains(d)) problems.Add($"{f.Id} depends on {d}, which is removed");
                else if (!ids.Contains(d)) problems.Add($"{f.Id} depends on {d}, which does not exist");
            }
        }
        var deps = new Dictionary<string, List<string>>(StringComparer.Ordinal);
        foreach (var f in live)
        {
            var ds = (f.DependsOn ?? []).Distinct(StringComparer.Ordinal).Where(ids.Contains).ToList();
            ds.Sort(ById);
            deps[f.Id] = ds;
        }
        var cycle = FindCycle(live.Select(f => f.Id).ToList(), deps);
        if (cycle is not null) problems.Add($"dependency cycle: {string.Join(" -> ", cycle.Append(cycle[0]))}");
        if (problems.Count > 0) throw new FeatureMapException(string.Join("; ", problems), cycle);
        return new FeatureMap
        {
            Features = live.Select(f =>
            {
                var surfaces = (f.Surfaces ?? []).ToList();
                var keyed = surfaces.Select((s, i) => (s, i)).ToList();
                keyed.Sort((x, y) => { var c = ById(x.s.Kind + "\0" + x.s.Locator, y.s.Kind + "\0" + y.s.Locator); return c != 0 ? c : x.i - y.i; });
                return new MapEntry
                {
                    Id = f.Id,
                    Slug = f.Slug,
                    Name = f.Name,
                    Surfaces = keyed.Select(x => x.s).ToList(),
                    Reachable = surfaces.Count > 0,
                    DependsOn = deps[f.Id],
                    Dependents = live.Where(g => deps[g.Id].Contains(f.Id)).Select(g => g.Id).ToList(),
                };
            }).ToList(),
        };
    }

    private static List<string>? FindCycle(List<string> ids, Dictionary<string, List<string>> deps)
    {
        var state = new Dictionary<string, int>(StringComparer.Ordinal);
        var stack = new List<string>();
        List<string>? Visit(string id)
        {
            state[id] = 1;
            stack.Add(id);
            foreach (var d in deps.TryGetValue(id, out var ds) ? ds : [])
            {
                if (state.TryGetValue(d, out var s) && s == 1)
                {
                    var loop = stack.Skip(stack.IndexOf(d)).ToList();
                    var min = loop.OrderBy(x => x, StringComparer.Ordinal).First();
                    var k = loop.IndexOf(min);
                    return [.. loop.Skip(k), .. loop.Take(k)];
                }
                if (!state.ContainsKey(d) && Visit(d) is { } c) return c;
            }
            stack.RemoveAt(stack.Count - 1);
            state[id] = 2;
            return null;
        }
        foreach (var id in ids)
            if (!state.ContainsKey(id) && Visit(id) is { } c) return c;
        return null;
    }

    /// <summary>Every feature that depends, directly or not, on one of <paramref name="changed"/>; those are left out.</summary>
    public static List<string> AffectedBy(FeatureMap map, IReadOnlyList<string> changed)
    {
        var dependents = map.Features.ToDictionary(f => f.Id, f => f.Dependents, StringComparer.Ordinal);
        var seen = new HashSet<string>(StringComparer.Ordinal);
        var queue = new Queue<string>(changed.OrderBy(x => x, StringComparer.Ordinal));
        while (queue.Count > 0)
            foreach (var d in dependents.TryGetValue(queue.Dequeue(), out var ds) ? ds : [])
                if (seen.Add(d)) queue.Enqueue(d);
        return seen.Where(x => !changed.Contains(x)).OrderBy(x => x, StringComparer.Ordinal).ToList();
    }

    /// <summary>The map as text, the output of <c>lastro map</c>.</summary>
    public static string Render(FeatureMap map, IReadOnlyList<string>? affected = null)
    {
        var L = new List<string>();
        foreach (var f in map.Features)
        {
            L.Add($"{f.Id} {f.Name}{((affected ?? []).Contains(f.Id) ? "  (affected)" : "")}");
            if (!f.Reachable) L.Add("  not reachable by the driver: no surface");
            foreach (var s in f.Surfaces) L.Add($"  {s.Kind} {s.Locator}: {s.Name}");
            if (f.DependsOn.Count > 0) L.Add($"  depends on {string.Join(", ", f.DependsOn)}");
            if (f.Dependents.Count > 0) L.Add($"  needed by {string.Join(", ", f.Dependents)}");
        }
        return string.Join("\n", L) + "\n";
    }

    /// <summary>Applies a change to existing links; removing what the feature does not have is refused.</summary>
    public static (List<Surface> Surfaces, List<string> DependsOn) MergeLinks(IReadOnlyList<Surface>? surfaces, IReadOnlyList<string>? dependsOn, LinksChange c)
    {
        var outS = (surfaces ?? []).ToList();
        foreach (var loc in c.RemoveSurfaces)
        {
            var k = outS.FindIndex(s => s.Locator == loc);
            if (k < 0) throw new FeatureMapException($"cannot remove surface {loc}: the feature does not have it");
            outS.RemoveAt(k);
        }
        foreach (var s in c.AddSurfaces)
            if (!outS.Any(x => x.Locator == s.Locator)) outS.Add(s);
        var deps = (dependsOn ?? []).ToList();
        foreach (var d in c.RemoveDependsOn)
        {
            var k = deps.IndexOf(d);
            if (k < 0) throw new FeatureMapException($"cannot remove dependency on {d}: the feature does not have it");
            deps.RemoveAt(k);
        }
        foreach (var d in c.AddDependsOn)
            if (!deps.Contains(d)) deps.Add(d);
        return (outS, deps);
    }

    /// <summary>
    /// The surfaces and dependencies of one extracted change. An add takes the ones proposed; a modify keeps the use
    /// case's own and only adds or removes those the change names; a remove has none. Problems are collected.
    /// </summary>
    internal static (List<Surface>? Surfaces, List<string>? DependsOn) LinksOf(string change, LinksChange? l, UseCase? existing, Func<string, string?> idOf, List<string> problems, string at)
    {
        if (l is null || change == "remove")
            return change == "remove" || existing is null ? (null, null) : (NullIfEmpty(existing.Surfaces), NullIfEmpty(existing.DependsOn));
        if (change == "add" && (l.RemoveSurfaces.Count > 0 || l.RemoveDependsOn.Count > 0))
        {
            problems.Add($"{at}: an add has no surfaces or dependencies to remove");
            return (null, null);
        }
        List<string> Ids(List<string> slugs) => slugs.Select(s =>
        {
            var id = idOf(s);
            if (id is null) problems.Add($"{at}: depends on unknown use case \"{s}\"");
            return id ?? s;
        }).ToList();
        var add = Ids(l.AddDependsOn);
        var remove = Ids(l.RemoveDependsOn);
        try
        {
            var (s, d) = MergeLinks(change == "add" ? null : existing?.Surfaces, change == "add" ? null : existing?.DependsOn,
                new LinksChange { AddSurfaces = l.AddSurfaces, RemoveSurfaces = l.RemoveSurfaces, AddDependsOn = add, RemoveDependsOn = remove });
            problems.AddRange(SurfaceProblems(at, s));
            return (NullIfEmpty(s), NullIfEmpty(d));
        }
        catch (FeatureMapException e)
        {
            problems.Add($"{at}: {e.Message}");
            return (null, null);
        }
    }

    internal static List<T>? NullIfEmpty<T>(List<T>? xs) => xs is { Count: > 0 } ? xs : null;
}
