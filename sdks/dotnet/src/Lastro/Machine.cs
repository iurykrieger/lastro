using System.Globalization;
using System.Text.Json.Nodes;
using System.Text.RegularExpressions;

namespace Lastro;

// State-machine proofs: a port of the TS SDK's machine.ts (breadth-first exploration, random walks, statecharts).
//
// A machine model has shared state variables, per-actor (local) variables, an initial state and events with finite
// inputs. Actors are copies of the same process: their events interleave in every possible order. The code under
// proof is a step function from a state and an event, raised by an actor, to the next state (null when the event is
// not enabled there). Exploration is breadth first in one canonical order (actors ascending, events by name, inputs in
// domain order), so every engine visits the same states and finds the same shortest counterexample. A space of at
// most Limit reachable states is visited completely and proves; a larger one is walked at random with a fixed seed and
// only evidences. Proving a larger space by SMT is in the TS SDK only.

/// <summary>A state machine: shared and per-actor variables, their initial values and the events.</summary>
public sealed class MachineModel
{
    public string Name { get; set; } = "";
    public string Kind { get; set; } = "machine";
    public Dictionary<string, Domain> State { get; set; } = new(StringComparer.Ordinal);
    public Dictionary<string, Domain>? Local { get; set; }
    public int? Actors { get; set; }
    public Env Initial { get; set; } = new();
    public Dictionary<string, Dictionary<string, Domain>> Events { get; set; } = new(StringComparer.Ordinal);
    /// <summary>Kept as written: the TS SDK proves large spaces from them by SMT.</summary>
    public JsonNode? Transitions { get; set; }
}

/// <summary>An invariant (<c>Holds</c>), a step clause (<c>Requires</c>, <c>Ensures</c>) or a reachability condition.</summary>
public sealed class MachineClause
{
    public string Id { get; set; } = "";
    /// <summary><c>invariant</c>, <c>step</c> or <c>reach</c>.</summary>
    public string Kind { get; set; } = "invariant";
    public string? Description { get; set; }
    public Expr? Holds { get; set; }
    public Expr? Requires { get; set; }
    public Expr? Ensures { get; set; }
    public Expr? Condition { get; set; }
}

/// <summary>The code under proof: null when the event is not enabled in that state.</summary>
public delegate Task<Env?> MachineStep(Env input);

/// <summary>One event raised by one actor.</summary>
public sealed class MachineEvent
{
    public int Actor { get; set; }
    public string Event { get; set; } = "";
    public Env Input { get; set; } = new();
}

public sealed class MachineClauseResult
{
    public string ClauseId { get; set; } = "";
    public string Kind { get; set; } = "";
    public string Verdict { get; set; } = "not-run";
    public string Evidence { get; set; } = "none";
    /// <summary><c>bfs</c> or <c>random-walk</c>.</summary>
    public string Method { get; set; } = "bfs";
    public int Checked { get; set; }
    public List<MachineEvent>? Path { get; set; }
    public Env? State { get; set; }
    public double? FailureRateUpperBound95 { get; set; }
    public string? Note { get; set; }
}

public sealed class MachineReport
{
    public string Model { get; set; } = "";
    public string Method { get; set; } = "bfs";
    public int States { get; set; }
    public int Transitions { get; set; }
    public bool Complete { get; set; }
    public List<MachineClauseResult> Clauses { get; set; } = [];
    public List<string> Errors { get; set; } = [];
}

/// <summary>One step the code was asked about: the step input and the next state it returned (null: not enabled).</summary>
public sealed class StepRow
{
    public Env Input { get; set; } = new();
    public Env? Output { get; set; }
}

public sealed class StatechartState
{
    public string Id { get; set; } = "";
    public string Label { get; set; } = "";
    public bool Initial { get; set; }
    public bool Forbidden { get; set; }
}

public sealed class StatechartTransition
{
    public string From { get; set; } = "";
    public string To { get; set; } = "";
    public string Event { get; set; } = "";
    public bool OnPath { get; set; }
}

/// <summary>The shared state (<c>shared</c>) or one actor's own variables (<c>actor &lt;k&gt;</c>).</summary>
public sealed class StatechartRegion
{
    public string Name { get; set; } = "";
    public List<StatechartState> States { get; set; } = [];
    public List<StatechartTransition> Transitions { get; set; } = [];
}

public sealed class Statechart
{
    public string Model { get; set; } = "";
    public List<StatechartRegion> Regions { get; set; } = [];
}

public static class Machines
{
    private static int ActorsOf(MachineModel m) => Math.Max(1, m.Actors ?? 1);
    private static IEnumerable<string> Sorted(IEnumerable<string> keys) => keys.OrderBy(k => k, StringComparer.Ordinal);
    private static Dictionary<string, Domain> LocalOf(MachineModel m) => m.Local ?? new(StringComparer.Ordinal);
    private static string Key(Env s) => JsJson.Canonical(LastroJson.ToNode(s));

    /// <summary>What is wrong with a model: infinite domains, no events, an initial value out of its domain.</summary>
    public static List<string> Problems(MachineModel m)
    {
        var errs = new List<string>();
        void Finite(Dictionary<string, Domain> t, string at)
        {
            foreach (var (k, d) in t)
                if (double.IsInfinity(Domains.Size(d))) errs.Add($"{at}.{k}: the domain must be finite");
        }
        Finite(m.State, "state");
        Finite(LocalOf(m), "local");
        foreach (var (e, inputs) in m.Events) Finite(inputs, $"events.{e}");
        if (m.Events.Count == 0) errs.Add("events: a machine needs at least one event");
        foreach (var (k, d) in m.State.Concat(LocalOf(m)))
            if (!m.Initial.TryGetValue(k, out var v) || !Domains.Contains(d, v))
                errs.Add($"initial.{k}: {(m.Initial.TryGetValue(k, out var x) ? JsJson.Stringify(LastroJson.ToNode(x)) : "undefined")} is not in its domain");
        if (m.Actors is < 1) errs.Add("actors: must be a positive integer");
        foreach (var k in LocalOf(m).Keys)
            if (m.State.ContainsKey(k)) errs.Add($"local.{k}: also a shared variable");
        return errs;
    }

    private static Env Initial(MachineModel m)
    {
        var s = new Env();
        foreach (var k in Sorted(m.State.Keys)) s[k] = m.Initial[k];
        for (var a = 1; a <= ActorsOf(m); a++)
            foreach (var v in Sorted(LocalOf(m).Keys)) s[$"{v}_{a}"] = m.Initial[v];
        return s;
    }

    private static List<MachineEvent> EventsOf(MachineModel m)
    {
        var o = new List<MachineEvent>();
        for (var actor = 1; actor <= ActorsOf(m); actor++)
            foreach (var e in Sorted(m.Events.Keys))
                foreach (var input in Domains.EnumerateSpace(m.Events[e]))
                    o.Add(new MachineEvent { Actor = actor, Event = e, Input = input });
        return o;
    }

    private static Env StepInput(MachineModel m, Env s, MachineEvent e)
    {
        var x = new Env { ["actor"] = e.Actor, ["event"] = e.Event };
        foreach (var (k, v) in e.Input) x[k] = v;
        foreach (var k in Sorted(m.State.Keys)) x[k] = s[k];
        foreach (var v in Sorted(LocalOf(m).Keys)) x[v] = s[$"{v}_{e.Actor}"];
        return x;
    }

    private static string Show(MachineEvent e) => $"{e.Event}{(e.Input.Count > 0 ? JsJson.Canonical(LastroJson.ToNode(e.Input)) : "")} by actor {e.Actor}";

    private static (Env? Next, string? Problem) Next(MachineModel m, Env s, MachineEvent e, Env output)
    {
        var n = s.Copy();
        foreach (var (k, d) in m.State)
        {
            var v = output.TryGetValue(k, out var o) ? o : s[k];
            if (!Domains.Contains(d, v)) return (null, $"{k}={JsJson.Stringify(LastroJson.ToNode(v))} is outside its domain after {Show(e)}");
            n[k] = v;
        }
        foreach (var (k, d) in LocalOf(m))
        {
            var own = $"{k}_{e.Actor}";
            var v = output.TryGetValue(k, out var o) ? o : s[own];
            if (!Domains.Contains(d, v)) return (null, $"{k}={JsJson.Stringify(LastroJson.ToNode(v))} is outside its domain after {Show(e)}");
            n[own] = v;
        }
        return (n, null);
    }

    private static Env StepOutput(MachineModel m, Env n, int actor)
    {
        var o = new Env();
        foreach (var k in m.State.Keys) o[k] = n[k];
        foreach (var k in LocalOf(m).Keys) o[k] = n[$"{k}_{actor}"];
        return o;
    }

    private sealed record Node(Env State, int Parent, MachineEvent? Via);

    private static List<MachineEvent> PathTo(List<Node> nodes, int i)
    {
        var path = new List<MachineEvent>();
        for (var k = i; k >= 0 && nodes[k].Via is { } via; k = nodes[k].Parent) path.Insert(0, via);
        return path;
    }

    /// <summary>Explores a machine against its clauses with the code's step function.</summary>
    public static async Task<MachineReport> ExploreAsync(MachineModel m, MachineStep step, IReadOnlyList<MachineClause> clauses, int limit = 100_000, long seed = 42, int walks = 200, int depth = 50)
    {
        var errors = Problems(m);
        var results = clauses.Select(c => new MachineClauseResult { ClauseId = c.Id, Kind = c.Kind }).ToList();
        MachineReport Report(string method, int states, int transitions, bool complete) =>
            new() { Model = m.Name, Method = method, States = states, Transitions = transitions, Complete = complete, Clauses = results, Errors = errors };
        if (errors.Count > 0)
        {
            foreach (var r in results) r.Note = "the model or the clause is invalid";
            return Report("bfs", 0, 0, false);
        }
        if (limit <= 0) limit = 100_000;
        var events = EventsOf(m);
        var applied = new Dictionary<string, int>(StringComparer.Ordinal);
        bool? Check(MachineClauseResult r, Expr e, EvalContext ctx)
        {
            try { return Lsl.Holds(e, ctx); }
            catch (LslException ex)
            {
                if (r.Verdict == "not-run") (r.Verdict, r.Evidence, r.Note) = ("violated", "none", $"evaluation error: {ex.Message}");
                return null;
            }
        }
        void OnState(List<Node> nodes, int i, string method)
        {
            var s = nodes[i].State;
            var ctx = new EvalContext(s, new Env(), null);
            for (var k = 0; k < clauses.Count; k++)
            {
                var (c, r) = (clauses[k], results[k]);
                if (r.Verdict != "not-run") continue;
                if (c.Kind == "invariant" && Check(r, c.Holds!, ctx) == false)
                    (r.Verdict, r.Evidence, r.Method, r.Path, r.State) = ("violated", "counterexample", method, PathTo(nodes, i), s);
                if (c.Kind == "reach" && Check(r, c.Condition!, ctx) == true)
                    (r.Verdict, r.Evidence, r.Method, r.Path, r.State, r.Note) = ("proved", "formal", method, PathTo(nodes, i), s, "reached: the path is a witness");
            }
        }
        void OnStep(List<Node> nodes, int from, MachineEvent e, Env next, string method)
        {
            for (var k = 0; k < clauses.Count; k++)
            {
                var (c, r) = (clauses[k], results[k]);
                if (c.Kind != "step" || r.Verdict != "not-run") continue;
                var ctx = new EvalContext(StepInput(m, nodes[from].State, e), StepOutput(m, next, e.Actor), null);
                if (c.Requires is not null && Check(r, c.Requires, ctx) != true) continue;
                applied[c.Id] = applied.GetValueOrDefault(c.Id) + 1;
                if (Check(r, c.Ensures!, ctx) == false)
                    (r.Verdict, r.Evidence, r.Method, r.Path, r.State) = ("violated", "counterexample", method, [.. PathTo(nodes, from), e], next);
            }
        }
        async Task<(Env? Next, string? Problem)> Run(Env s, MachineEvent e)
        {
            Env? output;
            try { output = await step(StepInput(m, s, e)).ConfigureAwait(false); }
            catch (Exception ex) { return (null, $"the step function threw on {Show(e)}: {ex.Message}"); }
            return output is null ? (null, null) : Next(m, s, e, output);
        }

        var nodes = new List<Node> { new(Initial(m), -1, null) };
        var seen = new HashSet<string>(StringComparer.Ordinal) { Key(nodes[0].State) };
        OnState(nodes, 0, "bfs");
        var transitions = 0;
        var complete = true;
        for (var i = 0; i < nodes.Count && complete; i++)
            foreach (var e in events)
            {
                var (n, problem) = await Run(nodes[i].State, e).ConfigureAwait(false);
                if (problem is not null)
                {
                    if (!errors.Contains(problem)) errors.Add(problem);
                    continue;
                }
                if (n is null) continue;
                transitions++;
                OnStep(nodes, i, e, n, "bfs");
                var k = Key(n);
                if (seen.Contains(k)) continue;
                if (nodes.Count >= limit)
                {
                    complete = false;
                    break;
                }
                seen.Add(k);
                nodes.Add(new Node(n, i, e));
                OnState(nodes, nodes.Count - 1, "bfs");
            }
        if (complete)
        {
            for (var k = 0; k < clauses.Count; k++)
            {
                var (c, r) = (clauses[k], results[k]);
                r.Checked = c.Kind == "step" ? applied.GetValueOrDefault(c.Id) : nodes.Count;
                if (r.Verdict != "not-run") continue;
                if (c.Kind == "reach") (r.Verdict, r.Evidence, r.Note) = ("violated", "formal", $"no reachable state satisfies the condition ({nodes.Count} states visited)");
                else if (c.Kind == "step" && applied.GetValueOrDefault(c.Id) == 0) (r.Verdict, r.Note) = ("vacuous", "no reachable step makes the if-part true: the clause proves nothing");
                else (r.Verdict, r.Evidence) = ("proved", "formal");
            }
            if (errors.Count > 0)
                foreach (var r in results.Where(r => r.Verdict == "proved")) (r.Verdict, r.Evidence, r.Note) = ("violated", "none", errors[0]);
            return Report("bfs", nodes.Count, transitions, true);
        }

        // Too many states: random walks with a fixed seed. A clause that holds is evidenced, never proved.
        var next = Domains.Rng(seed);
        var steps = 0;
        for (var w = 0; w < walks; w++)
        {
            var walk = new List<Node> { new(Initial(m), -1, null) };
            for (var d = 0; d < depth; d++)
            {
                var here = walk[^1].State;
                var enabled = new List<(MachineEvent E, Env N)>();
                foreach (var e in events)
                    if (await Run(here, e).ConfigureAwait(false) is ({ } n, null)) enabled.Add((e, n));
                if (enabled.Count == 0) break;
                var (pe, pn) = enabled[(int)Math.Floor(next() * enabled.Count)];
                steps++;
                OnStep(walk, walk.Count - 1, pe, pn, "random-walk");
                walk.Add(new Node(pn, walk.Count - 1, pe));
                OnState(walk, walk.Count - 1, "random-walk");
            }
        }
        for (var k = 0; k < clauses.Count; k++)
        {
            var (c, r) = (clauses[k], results[k]);
            if (r.Verdict != "not-run") continue;
            (r.Method, r.Checked) = ("random-walk", steps);
            if (c.Kind == "reach") (r.Verdict, r.Evidence, r.Note) = ("evidenced", "none", "not reached by the walks; the space is too large to show it unreachable");
            else if (c.Kind == "step" && applied.GetValueOrDefault(c.Id) == 0) (r.Verdict, r.Note) = ("vacuous", "no step of the walks makes the if-part true");
            else
            {
                var n = c.Kind == "step" ? applied[c.Id] : steps;
                (r.Verdict, r.Evidence, r.FailureRateUpperBound95) = ("evidenced", "empirical", Math.Min(1, 3.0 / Math.Max(1, n)));
                r.Note = $"more than {limit} reachable states: explored by {walks} random walks of up to {depth} steps (seed {seed})";
            }
        }
        return Report("random-walk", nodes.Count, transitions, false);
    }

    /// <summary>A step function that answers from a recorded step table.</summary>
    public static MachineStep TableStep(IEnumerable<StepRow> rows)
    {
        var by = new Dictionary<string, Env?>(StringComparer.Ordinal);
        foreach (var r in rows) by[Key(r.Input)] = r.Output;
        return input =>
        {
            var k = Key(input);
            return by.TryGetValue(k, out var o) ? Task.FromResult(o) : throw new InvalidOperationException($"the table has no row for {k}");
        };
    }

    private static readonly Regex ActorSuffix = new("_\\d+$");

    private static string JsString(Value v) => v.Kind switch
    {
        ValueKind.String => v.AsString,
        ValueKind.Number => JsJson.Number(v.AsNumber),
        ValueKind.Bool => v.AsBool ? "true" : "false",
        _ => "undefined",
    };

    /// <summary>The machine as the user sees it: forbidden states marked, the counterexample path's edges flagged.</summary>
    public static async Task<Statechart> StatechartAsync(MachineModel m, MachineStep step, IReadOnlyList<MachineClause>? clauses = null, IReadOnlyList<MachineEvent>? path = null, int limit = 100_000)
    {
        if (limit <= 0) limit = 100_000;
        var regions = new List<(string Name, int Actor, List<string> Vars)>();
        if (m.State.Count > 0) regions.Add(("shared", 0, Sorted(m.State.Keys).ToList()));
        if (LocalOf(m).Count > 0)
            for (var a = 1; a <= ActorsOf(m); a++) regions.Add(($"actor {a}", a, Sorted(LocalOf(m).Keys).Select(v => $"{v}_{a}").ToList()));
        var outR = regions.Select(r => new StatechartRegion { Name = r.Name }).ToList();
        var invariants = (clauses ?? []).Where(c => c.Kind == "invariant").ToList();
        bool Forbidden(Env s) => invariants.Any(c =>
        {
            try { return !Lsl.Holds(c.Holds!, new EvalContext(s, new Env(), null)); }
            catch (Exception) { return false; }
        });
        string AddState(int r, Env s, bool initial)
        {
            var vars = regions[r].Vars;
            var id = "s" + JsJson.Canonical(new JsonArray([.. vars.Select(v => s.TryGetValue(v, out var x) ? x.ToNode() : null)]));
            var label = string.Join(", ", vars.Select(v => $"{ActorSuffix.Replace(v, "")}={(s.TryGetValue(v, out var x) ? JsString(x) : "undefined")}"));
            if (label == "") label = "(no state)";
            var st = outR[r].States.FirstOrDefault(x => x.Id == id);
            if (st is null) outR[r].States.Add(new StatechartState { Id = id, Label = label, Initial = initial, Forbidden = Forbidden(s) });
            else if (Forbidden(s)) st.Forbidden = true;
            return id;
        }
        var events = EventsOf(m);
        var queue = new List<Env> { Initial(m) };
        var seen = new HashSet<string>(StringComparer.Ordinal) { Key(queue[0]) };
        for (var r = 0; r < regions.Count; r++) AddState(r, queue[0], true);
        var onPath = new HashSet<string>(StringComparer.Ordinal);
        var s0 = queue[0];
        foreach (var e in path ?? [])
        {
            Env? o;
            try { o = await step(StepInput(m, s0, e)).ConfigureAwait(false); }
            catch (Exception) { break; }
            if (o is null) break;
            var (n, problem) = Next(m, s0, e, o);
            if (problem is not null) break;
            for (var k = 0; k < regions.Count; k++)
            {
                if (regions[k].Actor != 0 && regions[k].Actor != e.Actor) continue;
                onPath.Add($"{k}|{AddState(k, s0, false)}|{AddState(k, n!, false)}|{e.Event}");
            }
            s0 = n!;
        }
        for (var i = 0; i < queue.Count && queue.Count <= limit; i++)
        {
            var s = queue[i];
            foreach (var e in events)
            {
                Env? o;
                try { o = await step(StepInput(m, s, e)).ConfigureAwait(false); }
                catch (Exception) { continue; }
                if (o is null) continue;
                var (n, problem) = Next(m, s, e, o);
                if (problem is not null) continue;
                for (var k = 0; k < regions.Count; k++)
                {
                    var r = regions[k];
                    if (r.Actor != 0 && r.Actor != e.Actor) continue;
                    var from = AddState(k, s, false);
                    var to = AddState(k, n!, false);
                    if (from == to && r.Actor == 0 && Sorted(m.State.Keys).All(v => s[v].Equals(n![v])) && regions.Count > 1) continue;
                    var t = outR[k].Transitions;
                    if (!t.Any(x => x.From == from && x.To == to && x.Event == e.Event))
                        t.Add(new StatechartTransition { From = from, To = to, Event = e.Event, OnPath = onPath.Contains($"{k}|{from}|{to}|{e.Event}") });
                }
                if (seen.Add(Key(n!))) queue.Add(n!);
            }
        }
        foreach (var r in outR)
        {
            r.States = r.States.OrderBy(x => x.Id, StringComparer.Ordinal).ToList();
            string K(StatechartTransition t) => JsJson.Canonical(new JsonArray(t.From, t.To, t.Event));
            r.Transitions = r.Transitions.OrderBy(K, StringComparer.Ordinal).ToList();
        }
        return new Statechart { Model = m.Name, Regions = outR };
    }

    /// <summary>The statechart in Mermaid (stateDiagram-v2), one concurrent region per actor; forbidden states styled.</summary>
    public static string Mermaid(Statechart c)
    {
        var ids = new Dictionary<string, string>(StringComparer.Ordinal);
        string Id(int region, string s)
        {
            var k = $"{region}|{s}";
            if (!ids.TryGetValue(k, out var v)) ids[k] = v = $"r{region}_{ids.Count.ToString(CultureInfo.InvariantCulture)}";
            return v;
        }
        var lines = new List<string> { "stateDiagram-v2", $"  state \"{c.Model}\" as machine {{" };
        for (var k = 0; k < c.Regions.Count; k++)
        {
            var r = c.Regions[k];
            if (k > 0) lines.Add("    --");
            lines.Add($"    %% {r.Name}");
            foreach (var s in r.States)
            {
                lines.Add($"    state \"{s.Label.Replace("\"", "'", StringComparison.Ordinal)}\" as {Id(k, s.Id)}");
                if (s.Initial) lines.Add($"    [*] --> {Id(k, s.Id)}");
            }
            foreach (var t in r.Transitions) lines.Add($"    {Id(k, t.From)} --> {Id(k, t.To)} : {t.Event}{(t.OnPath ? " (counterexample)" : "")}");
        }
        lines.Add("  }");
        var bad = c.Regions.SelectMany((r, k) => r.States.Where(s => s.Forbidden).Select(s => Id(k, s.Id))).ToList();
        if (bad.Count > 0) lines.AddRange(["  classDef forbidden fill:#fdd,stroke:#c00", $"  class {string.Join(",", bad)} forbidden"]);
        return string.Join("\n", lines) + "\n";
    }
}
