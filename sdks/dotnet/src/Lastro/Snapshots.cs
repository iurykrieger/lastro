using System.Globalization;
using System.Text.Json.Nodes;
using System.Text.RegularExpressions;

namespace Lastro;

// Before and after snapshots, and what a reviewer must still do before a proposal is proved. A port of the TS SDK's
// snapshots.ts and of the io part of evidence.ts: the same files, byte for byte, so their SHA-256 match.
//
// Every intent gets a before snapshot of each feature it touches, taken when the proposal is extracted, and the same
// snapshot on the new code at the proof run (the after). This port takes the contract and examples snapshots of io
// models; screens and data need the TS SDK's drivers.

/// <summary>One snapshot file, as the proposal records it.</summary>
public sealed class SnapshotRecord
{
    /// <summary>The intent that changed the feature, or <c>repository</c> for a whole-repository snapshot.</summary>
    public string Intent { get; set; } = "";
    public string? Feature { get; set; }
    /// <summary><c>screen</c>, <c>contract</c>, <c>data</c> or <c>examples</c>.</summary>
    public string Kind { get; set; } = "";
    /// <summary><c>before</c> or <c>after</c>.</summary>
    public string Side { get; set; } = "";
    /// <summary>Relative to the workspace folder: docs/&lt;proposal&gt;/&lt;feature&gt;/&lt;intent&gt;/&lt;side&gt;/&lt;file&gt;.</summary>
    public string Path { get; set; } = "";
    public string Sha256 { get; set; } = "";
}

/// <summary>A field or column added, removed or changed in type; a removal or a type change is breaking.</summary>
public sealed class FieldChange
{
    public string Path { get; set; } = "";
    /// <summary><c>added</c>, <c>removed</c> or <c>type</c>.</summary>
    public string Change { get; set; } = "";
    public string? Before { get; set; }
    public string? After { get; set; }
    public bool Breaking { get; set; }
}

/// <summary>One example input with its before and after outputs (an output object or <c>{"error": ...}</c>), as written.</summary>
public sealed class ExampleRow
{
    public JsonNode? Input { get; set; }
    public JsonNode? Before { get; set; }
    public JsonNode? After { get; set; }
}

/// <summary>An after snapshot compared with its before.</summary>
public sealed class Comparison
{
    public string Intent { get; set; } = "";
    public string? Feature { get; set; }
    public string Kind { get; set; } = "";
    /// <summary>The file name both sides share.</summary>
    public string Name { get; set; } = "";
    public double? PixelShare { get; set; }
    public List<FieldChange>? Diff { get; set; }
    public bool? Breaking { get; set; }
    public List<ExampleRow>? ChangedRows { get; set; }
}

/// <summary>An example whose output changed although no approved clause applies to its input.</summary>
public sealed class UnexpectedChange
{
    public string Id { get; set; } = "";
    public string Intent { get; set; } = "";
    public string? Feature { get; set; }
    public JsonNode? Input { get; set; }
    public JsonNode? Before { get; set; }
    public JsonNode? After { get; set; }
    public List<string> AcceptedBy { get; set; } = [];
}

/// <summary>A reviewer's look at an intent beyond proof for human judgment, after the proof run.</summary>
public sealed class SignOff
{
    public string Intent { get; set; } = "";
    public string By { get; set; } = "";
    /// <summary><c>signed-off</c> or <c>changes-requested</c>.</summary>
    public string Verdict { get; set; } = "";
    public string? Note { get; set; }
    /// <summary>Screenshots the sign-off makes the reference of their scenario step.</summary>
    public List<string>? References { get; set; }
}

/// <summary>What still blocks "proved" after a run.</summary>
public sealed record Due(List<string> Signoffs, List<string> Unexpected, List<string> ChangesRequested);

/// <summary>What taking snapshots needs: the workspace folder, models, subjects and use cases.</summary>
public sealed record SnapshotContext(string Dir, IReadOnlyDictionary<string, Model> Models, IReadOnlyDictionary<string, Subject> Subjects, IReadOnlyList<UseCase> UseCases);

public static class Snapshots
{
    /// <summary>The whole space is used when it has at most this many points; otherwise a fixed-seed sample.</summary>
    public const int ExamplesLimit = 1000;

    /// <summary>The kinds of snapshot a feature gets, from its surfaces, its model and its data.</summary>
    public static List<string> Kinds(IReadOnlyList<Surface>? surfaces, bool hasModel, bool hasData)
    {
        var kinds = new HashSet<string>((surfaces ?? []).Select(s => s.Kind));
        var o = new List<string>();
        if (kinds.Contains("screen") || kinds.Contains("route")) o.Add("screen");
        if (kinds.Contains("api") || kinds.Contains("command") || hasModel) o.Add("contract");
        if (hasData) o.Add("data");
        if (hasModel) o.Add("examples");
        return o;
    }

    private static readonly Regex Unsafe = new("[^A-Za-z0-9._-]");

    /// <summary>Where a snapshot file lives, relative to the workspace folder; no feature means a repository snapshot.</summary>
    public static string PathOf(string proposal, string side, string file, string? feature = null, string? intent = null)
    {
        string Safe(string s) => Unsafe.Replace(s, "_");
        if (feature is null || intent is null) return $"docs/{proposal}/repository/{side}/{Safe(file)}";
        var i = intent.IndexOf(proposal + ".", StringComparison.Ordinal);
        var short_ = i >= 0 ? intent.Remove(i, proposal.Length + 1) : intent;
        return $"docs/{proposal}/{Safe(feature)}/{Safe(short_)}/{side}/{Safe(file)}";
    }

    /// <summary>Every field added, removed or changed in type, in path order.</summary>
    public static List<FieldChange> StructuralDiff(IReadOnlyDictionary<string, string> before, IReadOnlyDictionary<string, string> after)
    {
        var paths = before.Keys.Union(after.Keys).OrderBy(x => x, StringComparer.Ordinal);
        var o = new List<FieldChange>();
        foreach (var p in paths)
        {
            var hasB = before.TryGetValue(p, out var b);
            var hasA = after.TryGetValue(p, out var a);
            if (hasB && hasA && a == b) continue;
            if (!hasB) o.Add(new FieldChange { Path = p, Change = "added", After = a });
            else if (!hasA) o.Add(new FieldChange { Path = p, Change = "removed", Before = b, Breaking = true });
            else o.Add(new FieldChange { Path = p, Change = "type", Before = b, After = a, Breaking = true });
        }
        return o;
    }

    /// <summary>A model's inputs and outputs with their domains, as a contract.</summary>
    public static JsonObject ModelContract(Model m)
    {
        static string Show(Domain d) => d.Type switch
        {
            "enum" => "enum" + JsJson.Stringify(new JsonArray((d.Values ?? []).Select(v => (JsonNode?)JsonValue.Create(Display(v))).ToArray())),
            "int" => $"int[{(d.Min is { } a ? JsJson.Number(a) : "")}..{(d.Max is { } b ? JsJson.Number(b) : "")}]",
            _ => d.Type,
        };
        var o = new JsonObject();
        foreach (var k in m.Inputs.Keys.OrderBy(x => x, StringComparer.Ordinal)) o["in." + k] = Show(m.Inputs[k]);
        foreach (var k in m.Outputs.Keys.OrderBy(x => x, StringComparer.Ordinal)) o["out." + k] = Show(m.Outputs[k]);
        return o;
    }

    /// <summary>String(v) in JavaScript, for the values of an enum domain.</summary>
    private static string Display(Value v)
    {
        var n = LastroJson.ToNode(v);
        return n switch
        {
            null => "null",
            JsonValue jv when jv.TryGetValue<string>(out var s) => s,
            _ => JsJson.Stringify(n),
        };
    }

    /// <summary>The scenarios' inputs (as written), then the model's whole space (up to 1,000 points) or a fixed-seed sample.</summary>
    public static (List<JsonNode> Inputs, bool WholeSpace) ExampleInputs(Model m, IEnumerable<JsonNode> scenarioInputs, long seed = 42)
    {
        var seen = new HashSet<string>(StringComparer.Ordinal);
        var o = new List<JsonNode>();
        void Add(JsonNode e)
        {
            if (seen.Add(JsJson.Canonical(e))) o.Add(e);
        }
        foreach (var s in scenarioInputs) Add(s);
        if (Domains.SpaceSize(m.Inputs) <= ExamplesLimit)
        {
            foreach (var e in Domains.EnumerateSpace(m.Inputs)) Add(LastroJson.ToNode(e)!);
            return (o, true);
        }
        var next = Domains.Rng(seed);
        for (var k = 0; k < ExamplesLimit; k++) Add(LastroJson.ToNode(Domains.SampleSpace(m.Inputs, next))!);
        return (o, false);
    }

    private static string Canon(JsonNode? n) => n is null ? "null" : JsJson.Canonical(n);

    /// <summary>The rows whose output changed between the before and the after code.</summary>
    public static List<ExampleRow> ChangedRows(IEnumerable<ExampleRow> rows) => rows.Where(r => Canon(r.Before) != Canon(r.After)).ToList();

    private static Env EnvOf(JsonNode? n) => n is JsonObject ? LastroJson.FromNode<Env>(n) : new Env();

    /// <summary>
    /// The changed rows no clause explains: none of the clauses has an if-part that holds for the input (a clause with
    /// no if-part applies to every input). Ids start at U<paramref name="start"/>.
    /// </summary>
    public static List<UnexpectedChange> UnexpectedChanges(IEnumerable<ExampleRow> rows, IReadOnlyList<Expr?> requires, string intent, string? feature, int start = 1)
    {
        var o = new List<UnexpectedChange>();
        foreach (var r in ChangedRows(rows))
        {
            var output = r.After is JsonObject a && !a.ContainsKey("error") ? EnvOf(a) : new Env();
            var input = EnvOf(r.Input);
            var explained = requires.Any(c =>
            {
                if (c is null) return true;
                try { return Lsl.Holds(c, new EvalContext(input, output, null)); }
                catch (LslException) { return false; }
            });
            if (!explained)
                o.Add(new UnexpectedChange { Id = $"U{start + o.Count}", Intent = intent, Feature = feature, Input = r.Input?.DeepClone(), Before = r.Before?.DeepClone(), After = r.After?.DeepClone() });
        }
        return o;
    }

    /// <summary>Sign-offs due, unexpected changes not accepted and intents with changes requested; each needs the policy's reviewers.</summary>
    public static Due ReviewDue(IReadOnlyList<string> judgment, IReadOnlyList<SignOff> signoffs, IReadOnlyList<UnexpectedChange> unexpected, int required)
    {
        var need = Math.Max(1, required);
        var requested = judgment.Where(i => signoffs.Any(s => s.Intent == i && s.Verdict == "changes-requested")).ToList();
        var due = judgment.Where(i => !requested.Contains(i) && signoffs.Where(s => s.Intent == i && s.Verdict == "signed-off").Select(s => s.By).Distinct().Count() < need).ToList();
        var open = unexpected.Where(u => u.AcceptedBy.Distinct().Count() < need).Select(u => u.Id).ToList();
        return new Due(due, open, requested);
    }

    private static string Json2(JsonNode node) => JsJson.Stringify(node, 2) + "\n";

    /// <summary>Takes one side's snapshots for every forward intent (at extraction: every intent), writes and records them.</summary>
    public static async Task<Proposal> Take(Proposal p, string side, SnapshotContext ctx)
    {
        var intents = side == "before" ? p.Intents : Proposals.ForwardIntents(p);
        var taken = new List<(SnapshotRecord Record, string Content)>();
        void Write(string path, string content, SnapshotRecord rec)
        {
            rec.Path = path;
            rec.Sha256 = Canonical.Sha256Hex(content);
            taken.Add((rec, content));
        }
        foreach (var it in intents)
        {
            var change = p.UseCases.FirstOrDefault(c => c.UseCase == it.UseCase);
            var u = ctx.UseCases.FirstOrDefault(x => x.Id == it.UseCase);
            if (side == "before" && u is null) continue; // a new feature has no before
            if (side == "after" && change?.Change == "remove") continue; // a removed one has no after
            var proofs = (u?.Proofs ?? []).Concat(p.Proofs.Where(x => x.UseCase == it.UseCase)).ToList();
            var surfaces = (side == "after" ? change?.Surfaces : u?.Surfaces) ?? u?.Surfaces ?? change?.Surfaces ?? [];
            var io = proofs.Where(x => x.Check.Kind is "clause" or "scenario").Select(x => (x.Model, x.Subject)).Distinct()
                .Where(k => ctx.Models.ContainsKey(k.Model) && ctx.Subjects.ContainsKey(k.Subject)).ToList();
            var kinds = Kinds(surfaces, io.Count > 0, false);
            SnapshotRecord Rec(string kind) => new() { Intent = it.Id, Feature = it.UseCase, Kind = kind, Side = side };
            if (kinds.Contains("contract"))
                foreach (var (m, _) in io)
                    Write(PathOf(p.Id, side, $"model-{m}.json", it.UseCase, it.Id), Json2(new JsonObject { ["model"] = m, ["fields"] = ModelContract(ctx.Models[m]) }), Rec("contract"));
            if (kinds.Contains("examples"))
                foreach (var (m, s) in io)
                {
                    var model = ctx.Models[m];
                    var scen = proofs.Where(x => x.Model == m && x.Check.Kind == "scenario").Select(x => LastroJson.ToNode(x.Check.Input ?? new Env())!);
                    var (inputs, whole) = ExampleInputs(model, scen);
                    var rows = new JsonArray();
                    foreach (var input in inputs)
                    {
                        JsonNode? output;
                        try { output = LastroJson.ToNode(await ctx.Subjects[s](EnvOf(input))); }
                        catch (Exception e) { output = new JsonObject { ["error"] = e.Message }; }
                        rows.Add(new JsonObject { ["input"] = input.DeepClone(), [side] = output });
                    }
                    Write(PathOf(p.Id, side, $"examples-{m}.json", it.UseCase, it.Id), Json2(new JsonObject { ["model"] = m, ["subject"] = s, ["wholeSpace"] = whole, ["rows"] = rows }), Rec("examples"));
                }
        }
        foreach (var (rec, content) in taken)
        {
            var path = System.IO.Path.Combine(ctx.Dir, rec.Path);
            Directory.CreateDirectory(System.IO.Path.GetDirectoryName(path)!);
            await File.WriteAllTextAsync(path, content);
        }
        var next = LastroJson.Clone(p);
        var all = (next.Snapshots ?? []).Where(x => x.Side != side).Concat(taken.Select(t => t.Record)).ToList();
        all.Sort((a, b) => string.CompareOrdinal(a.Path, b.Path));
        next.Snapshots = all;
        if (side == "after")
        {
            var (comparisons, unexpected) = Compare(next, ctx.Dir);
            next.Comparisons = comparisons;
            next.Unexpected = unexpected;
        }
        return next;
    }

    private static string NameOf(string path) => path[(path.LastIndexOf('/') + 1)..];

    /// <summary>Compares every after snapshot with its before; collects the unexpected example changes, keeping acceptances.</summary>
    public static (List<Comparison> Comparisons, List<UnexpectedChange> Unexpected) Compare(Proposal p, string dir)
    {
        var comparisons = new List<Comparison>();
        var unexpected = new List<UnexpectedChange>();
        JsonNode Read(SnapshotRecord r) => JsonNode.Parse(File.ReadAllText(System.IO.Path.Combine(dir, r.Path)))!;
        var snaps = p.Snapshots ?? [];
        foreach (var after in snaps.Where(x => x.Side == "after"))
        {
            var before = snaps.FirstOrDefault(x => x.Side == "before" && x.Intent == after.Intent && x.Feature == after.Feature && x.Kind == after.Kind && NameOf(x.Path) == NameOf(after.Path));
            if (before is null || after.Kind == "screen") continue;
            var c = new Comparison { Intent = after.Intent, Feature = after.Feature, Kind = after.Kind, Name = NameOf(after.Path) };
            if (after.Kind is "contract" or "data")
            {
                Dictionary<string, string> Fields(JsonNode n) => (n["fields"]?.AsObject() ?? []).ToDictionary(kv => kv.Key, kv => kv.Value?.GetValue<string>() ?? "", StringComparer.Ordinal);
                c.Diff = StructuralDiff(Fields(Read(before)), Fields(Read(after)));
                c.Breaking = c.Diff.Any(d => d.Breaking);
                comparisons.Add(c);
                continue;
            }
            var b = Read(before);
            var a = Read(after);
            var model = b["model"]?.GetValue<string>() ?? "";
            var byInput = new Dictionary<string, JsonNode?>(StringComparer.Ordinal);
            foreach (var r in b["rows"]?.AsArray() ?? []) byInput.TryAdd(Canon(r!["input"]), r!["before"]);
            var rows = new List<ExampleRow>();
            foreach (var r in a["rows"]?.AsArray() ?? [])
                if (byInput.TryGetValue(Canon(r!["input"]), out var old))
                    rows.Add(new ExampleRow { Input = r["input"]?.DeepClone(), Before = old?.DeepClone(), After = r["after"]?.DeepClone() });
            c.ChangedRows = ChangedRows(rows);
            comparisons.Add(c);
            var requires = p.Proofs.Where(x => x.Model == model && x.Check.Kind is "clause" or "scenario")
                .Select(x => x.Check.Kind == "clause" ? x.Check.Requires : EqualsInput(x.Check.Input ?? new Env())).ToList();
            unexpected.AddRange(UnexpectedChanges(c.ChangedRows, requires, after.Intent, after.Feature, unexpected.Count + 1));
        }
        foreach (var u in unexpected)
        {
            var old = (p.Unexpected ?? []).FirstOrDefault(x => x.Intent == u.Intent && Canon(x.Input) == Canon(u.Input) && Canon(x.After) == Canon(u.After));
            if (old is not null) u.AcceptedBy = [.. old.AcceptedBy];
        }
        return (comparisons, unexpected);
    }

    private static Expr EqualsInput(Env input)
    {
        var parts = input.Keys.OrderBy(k => k, StringComparer.Ordinal).Select(k => L.Eq(L.V("in." + k), L.C(input[k]))).ToList();
        return parts.Count == 1 ? parts[0] : L.And([.. parts.Count > 0 ? parts : [L.C(true)]]);
    }

    /// <summary>The receipt's map of the features the proposal changes and of the ones depending on them.</summary>
    internal static List<string> FeatureMapSection(Proposal p, IReadOnlyList<UseCase> useCases)
    {
        var features = useCases.Select(u =>
        {
            var c = p.UseCases.FirstOrDefault(x => x.UseCase == u.Id && x.Change == "modify");
            return c is null ? new Linked(u.Id, u.Slug, u.Name, u.Surfaces, u.DependsOn) : new Linked(u.Id, u.Slug, u.Name, c.Surfaces, c.DependsOn);
        }).Concat(p.UseCases.Where(c => c.Change == "add").Select(c => new Linked(c.UseCase, c.Slug, c.Name, c.Surfaces, c.DependsOn))).ToList();
        var removed = p.UseCases.Where(c => c.Change == "remove").Select(c => c.UseCase).ToList();
        if (!features.Any(f => f.Surfaces is { Count: > 0 } || f.DependsOn is { Count: > 0 })) return [];
        var L = new List<string> { "## Feature map", "" };
        try
        {
            var map = Features.Build(features, removed);
            var changed = p.UseCases.Select(c => c.UseCase).ToList();
            var affected = Features.AffectedBy(map, changed);
            L.Add(affected.Count > 0 ? $"Affected: {string.Join(", ", affected)} (their proofs and snapshots run again with this proposal's)." : "No other feature depends on the ones this proposal changes.");
            L.Add("");
            foreach (var f in map.Features.Where(x => changed.Contains(x.Id) || affected.Contains(x.Id)))
            {
                var s = string.Join(", ", f.Surfaces.Select(x => $"{x.Kind} `{x.Locator}`"));
                L.Add($"- {f.Id} {f.Name}: {(s.Length > 0 ? s : "no surface")}{(f.DependsOn.Count > 0 ? $"; depends on {string.Join(", ", f.DependsOn)}" : "")}");
            }
        }
        catch (FeatureMapException e)
        {
            L.Add($"The map is invalid: {e.Message}");
        }
        L.Add("");
        return L;
    }

    private static string Raw(JsonNode? n) => n is null ? "null" : JsJson.Stringify(n);

    /// <summary>The receipt's state machines, snapshots, unexpected changes and sign-offs.</summary>
    internal static List<string> EvidenceSections(Proposal p)
    {
        var L = new List<string>();
        foreach (var (name, mermaid) in (p.Statecharts ?? []).OrderBy(kv => kv.Key, StringComparer.Ordinal))
            L.AddRange([$"## State machine: {name}", "", "```mermaid", mermaid.TrimEnd(), "```", ""]);
        if (p.Snapshots is { Count: > 0 })
        {
            L.AddRange(["## Snapshots", ""]);
            foreach (var s in p.Snapshots) L.Add($"- {s.Side} {s.Kind} `{s.Path}` sha256:{s.Sha256}");
            foreach (var c in p.Comparisons ?? [])
            {
                var what = c.Kind switch
                {
                    "screen" => $"{((c.PixelShare ?? 0) * 100).ToString("F2", CultureInfo.InvariantCulture)}% of pixels changed",
                    "examples" => $"{c.ChangedRows?.Count ?? 0} row(s) changed",
                    _ => $"{c.Diff?.Count ?? 0} field change(s){(c.Breaking == true ? ", breaking" : "")}",
                };
                L.Add($"- {c.Intent}{(c.Feature is not null ? $" {c.Feature}" : "")} {c.Kind} {c.Name}: {what}");
                foreach (var d in c.Diff ?? [])
                    L.Add($"  - {d.Change} `{d.Path}`{(string.IsNullOrEmpty(d.Before) ? "" : $" {d.Before}")}{(string.IsNullOrEmpty(d.After) ? "" : $" → {d.After}")}{(d.Breaking ? " (breaking)" : "")}");
            }
            L.Add("");
        }
        if (p.Unexpected is { Count: > 0 })
        {
            L.AddRange(["## Unexpected changes", ""]);
            foreach (var u in p.Unexpected)
                L.Add($"- **{u.Id}** ({u.Intent}) `{Raw(u.Input)}`: `{Raw(u.Before)}` → `{Raw(u.After)}` · {(u.AcceptedBy.Count > 0 ? $"accepted by {string.Join(", ", u.AcceptedBy)}" : "not accepted")}");
            L.Add("");
        }
        var judgment = Proposals.JudgmentIntents(p);
        if (p.Signoffs is { Count: > 0 } || judgment.Count > 0)
        {
            L.AddRange(["## Sign-offs", ""]);
            foreach (var i in judgment)
            {
                var by = (p.Signoffs ?? []).Where(s => s.Intent == i).Select(s => $"{s.Verdict} by {s.By}{(s.Note is not null ? $" ({s.Note})" : "")}").ToList();
                L.Add($"- {i}: {(by.Count > 0 ? string.Join("; ", by) : "waiting")}");
            }
            L.Add("");
        }
        return L;
    }
}

public static partial class Proposals
{
    /// <summary>Intents beyond proof for human judgment, which wait for a sign-off after the proof run.</summary>
    public static List<string> JudgmentIntents(Proposal p) =>
        ForwardIntents(p).Where(i => i.Provability == "beyond-proof" && i.BeyondProofReason == "human-judgment").Select(i => i.Id).ToList();

    /// <summary>Sign-offs due, unexpected changes not accepted and intents with changes requested (empty before a run).</summary>
    public static Due DueOf(Proposal p) =>
        p.LastRun is null ? new Due([], [], []) : Snapshots.ReviewDue(JudgmentIntents(p), p.Signoffs ?? [], p.Unexpected ?? [], p.Policy.Approvals);
}
