using System.Reflection;
using System.Text.Json;
using System.Text.Json.Nodes;
using Lastro.Claude;

namespace Lastro.Cli;

/// <summary>
/// <c>lastro</c> on .NET: the SDK as a command line, for people and for any agent harness. Every command
/// prints one JSON object and keeps its state in <c>.lastro/</c>, in the same format as the TypeScript CLI,
/// so the two are interchangeable on one workspace.
///
/// Exit codes: 0 done/ok · 1 not ok (pending review, contradictions, failing proofs) · 2 usage or error ·
/// 3 an LLM answer is needed (handoff).
/// </summary>
public static class Cli
{
    public const string Usage = """
        lastro <command> [options]

          init --model <model.json> [--approvals N]     start a .lastro/ workspace
          plan <prose-file>                             extract intents (keeps earlier review)
          status                                        intents, approvals, pending items
          review --as <name> <action> ...               approve <I1 I2..> | approve-all | amend <I> <text>
                                                        reject <I> <note> | answer <D> <option-index>
          spec                                          derive the formal spec; contradictions come back as decisions
          prove <assembly.dll> --export <Namespace.Type.Method> [--traces file.json] [--stream]
                                                        check the code against the spec; writes proof.json, receipt.md;
                                                        --stream prints one JSON line per event, as the TS CLI does
          feedback                                      the last proof's failures, as a brief for whoever writes the code

          --dir <path>          workspace (default .lastro)
          --llm anthropic|handoff
        """;

    public sealed class ExitException(int code, JsonObject body) : Exception(body["error"]?.ToString() ?? body["status"]?.ToString() ?? code.ToString(System.Globalization.CultureInfo.InvariantCulture))
    {
        public int Code { get; } = code;
        public JsonObject Body { get; } = body;
    }

    /// <summary>Flags that never take a value.</summary>
    private static readonly HashSet<string> BooleanFlags = new(StringComparer.Ordinal) { "stream" };

    /// <summary>One <c>prove --stream</c> progress event every this many runs of the code, as in the TS CLI.</summary>
    public const int StreamProgressEvery = 100;

    /// <summary>True when <c>--stream</c> is on the command line.</summary>
    public static bool IsStreaming(string[] argv) => Parse(argv).Flags.ContainsKey("stream");

    private sealed record Args(List<string> Positional, Dictionary<string, string> Flags);

    private static Args Parse(string[] argv)
    {
        var args = new Args([], new Dictionary<string, string>(StringComparer.Ordinal));
        for (var i = 0; i < argv.Length; i++)
        {
            var a = argv[i];
            if (a.StartsWith("--", StringComparison.Ordinal))
            {
                var k = a[2..];
                if (i + 1 >= argv.Length || argv[i + 1].StartsWith("--", StringComparison.Ordinal) || BooleanFlags.Contains(k)) args.Flags[k] = "true";
                else args.Flags[k] = argv[++i];
            }
            else args.Positional.Add(a);
        }
        return args;
    }

    private sealed class Config
    {
        public Model Model { get; set; } = new();
        public int RequiredApprovals { get; set; } = 1;
    }

    private static T ReadJson<T>(string path) => LastroJson.Deserialize<T>(File.ReadAllText(path));

    private static void WriteJson<T>(string path, T value)
    {
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        File.WriteAllText(path, JsJson.Stringify(LastroJson.ToNode(value), indent: 2) + "\n");
    }

    private static JsonObject Obj(params (string Key, JsonNode? Value)[] items)
    {
        var o = new JsonObject();
        foreach (var (k, v) in items) o[k] = v;
        return o;
    }

    /// <summary>handoff: a deterministic request file per question, the answer file next to it.</summary>
    private static AskJson HandoffAsk(string dir) => (req, _) =>
    {
        var id = req.Id;
        var answer = Path.Combine(dir, "llm", $"{id}.answer.json");
        if (File.Exists(answer)) return Task.FromResult(JsonNode.Parse(File.ReadAllText(answer)));
        var request = Path.Combine(dir, "llm", $"{id}.request.json");
        var body = new JsonObject
        {
            ["instructions"] = $"Act as the system described in \"system\". Read \"user\". Write ONLY a JSON value matching \"schema\" to {answer}, then run the same lastro command again.",
        };
        foreach (var (k, v) in req.ToJson()) body[k] = v?.DeepClone();
        WriteJson(request, body);
        throw new ExitException(3, Obj(("status", "needs-llm"), ("step", req.Step), ("request", request), ("answer", answer)));
    };

    private static AskJson PickAsk(Args args, string dir)
    {
        var mode = args.Flags.GetValueOrDefault("llm") ?? (string.IsNullOrEmpty(Environment.GetEnvironmentVariable("ANTHROPIC_API_KEY")) ? "handoff" : "anthropic");
        return mode switch
        {
            "anthropic" => ClaudeAsk.Create(),
            "handoff" => HandoffAsk(dir),
            _ => throw new ExitException(2, Obj(("error", $"unknown --llm {mode}"))),
        };
    }

    private static JsonObject Summary(IntentArtifact a, IReadOnlyList<string> pending)
    {
        var intents = new JsonArray();
        foreach (var i in a.Intents)
        {
            var o = Obj(("id", i.Id), ("status", Wire.Of(i.Status)), ("approvals", LastroJson.ToNode(i.Approvals ?? [])), ("statement", i.Statement));
            if (i.DecisionId is not null) o["decisionId"] = i.DecisionId;
            intents.Add(o);
        }
        var decisions = new JsonArray();
        foreach (var d in a.Decisions)
            decisions.Add(Obj(("id", d.Id), ("origin", Wire.Of(d.Origin)), ("question", d.Question), ("options", LastroJson.ToNode(d.Options.Select(o => o.Label).ToList())),
                ("chosen", d.Chosen is { } c ? JsonValue.Create(c) : null), ("recommended", d.Recommended)));
        return Obj(("intents", intents), ("decisions", decisions), ("pending", LastroJson.ToNode(pending)), ("ready", pending.Count == 0));
    }

    private static JsonObject With(JsonObject head, JsonObject tail)
    {
        foreach (var (k, v) in tail) head[k] = v?.DeepClone();
        return head;
    }

    /// <param name="emit">Receives the <c>prove --stream</c> events (start, progress, clause, scenario, crosscheck) in the
    /// TS CLI's order and shape; the caller prints the closing <c>{"event":"result",...}</c>.</param>
    public static async Task<(int Code, JsonObject Body)> RunAsync(string[] argv, Action<JsonObject>? emit = null)
    {
        var args = Parse(argv);
        var cmd = args.Positional.FirstOrDefault();
        var rest = args.Positional.Skip(1).ToList();
        var dir = Path.GetFullPath(args.Flags.GetValueOrDefault("dir") ?? ".lastro");
        string F(string name) => Path.Combine(dir, name);
        Config Cfg()
        {
            if (!File.Exists(F("config.json"))) throw new ExitException(2, Obj(("error", $"no workspace at {dir}: run `lastro init --model model.json`")));
            return ReadJson<Config>(F("config.json"));
        }
        var intentsPath = F("intents.json");
        IntentArtifact Intents()
        {
            if (!File.Exists(intentsPath)) throw new ExitException(2, Obj(("error", $"no intents yet at {intentsPath}: run `lastro plan <prose-file>`")));
            return ReadJson<IntentArtifact>(intentsPath);
        }

        try
        {
            switch (cmd)
            {
                case "init":
                {
                    if (!args.Flags.TryGetValue("model", out var modelPath)) throw new ExitException(2, Obj(("error", "init needs --model <model.json>")));
                    var model = ReadJson<Model>(Path.GetFullPath(modelPath));
                    var approvals = int.Parse(args.Flags.GetValueOrDefault("approvals") ?? "1", System.Globalization.CultureInfo.InvariantCulture);
                    WriteJson(F("config.json"), new Config { Model = model, RequiredApprovals = approvals });
                    return (0, Obj(("status", "initialized"), ("dir", dir), ("model", model.Name), ("requiredApprovals", approvals)));
                }
                case "plan":
                {
                    var cfg = Cfg();
                    if (rest.Count == 0) throw new ExitException(2, Obj(("error", "plan needs a prose file")));
                    var prose = File.ReadAllText(Path.GetFullPath(rest[0])).Trim();
                    var lastro = new LastroClient(new LastroOptions { Model = cfg.Model, RequiredApprovals = cfg.RequiredApprovals, Ask = PickAsk(args, dir) });
                    var previous = File.Exists(intentsPath) ? ReadJson<IntentArtifact>(intentsPath) : null;
                    var a = await lastro.PlanAsync(prose, previous).ConfigureAwait(false);
                    WriteJson(intentsPath, a);
                    var pending = lastro.Pending(a);
                    return (pending.Count > 0 ? 1 : 0, With(Obj(("status", "planned")), Summary(a, pending)));
                }
                case "status":
                {
                    var cfg = Cfg();
                    var a = Intents();
                    var lastro = new LastroClient(new LastroOptions { Model = cfg.Model, RequiredApprovals = cfg.RequiredApprovals });
                    var pending = lastro.Pending(a);
                    return (pending.Count > 0 ? 1 : 0, With(Obj(("status", "status")), Summary(a, pending)));
                }
                case "review":
                {
                    var cfg = Cfg();
                    if (!args.Flags.TryGetValue("as", out var who)) throw new ExitException(2, Obj(("error", "review needs --as <name>")));
                    var action = rest.FirstOrDefault();
                    var xs = rest.Skip(1).ToList();
                    var lastro = new LastroClient(new LastroOptions { Model = cfg.Model, RequiredApprovals = cfg.RequiredApprovals });
                    var r = lastro.Review(Intents(), who);
                    switch (action)
                    {
                        case "approve": r.Approve([.. xs]); break;
                        case "approve-all": r.ApproveAll(); break;
                        case "amend": r.Amend(xs[0], string.Join(" ", xs.Skip(1))); break;
                        case "reject": r.Reject(xs[0], xs.Count > 1 ? string.Join(" ", xs.Skip(1)) : "rejected"); break;
                        case "answer": r.Answer(xs[0], int.Parse(xs[1], System.Globalization.CultureInfo.InvariantCulture)); break;
                        default: throw new ExitException(2, Obj(("error", $"unknown review action {action ?? ""}")));
                    }
                    var a = r.Done();
                    WriteJson(intentsPath, a);
                    return (0, With(Obj(("status", "reviewed"), ("reviewer", who)), Summary(a, lastro.Pending(a))));
                }
                case "spec":
                {
                    var cfg = Cfg();
                    var lastro = new LastroClient(new LastroOptions { Model = cfg.Model, RequiredApprovals = cfg.RequiredApprovals, Ask = PickAsk(args, dir) });
                    var a = Intents();
                    var pending = lastro.Pending(a);
                    if (pending.Count > 0) throw new ExitException(1, Obj(("status", "review-pending"), ("pending", LastroJson.ToNode(pending))));
                    var spec = await lastro.SpecAsync(a).ConfigureAwait(false);
                    if (!spec.Ok)
                    {
                        var back = lastro.ReturnDecisions(spec);
                        WriteJson(intentsPath, back);
                        return (1, With(Obj(("status", "contradictions"),
                            ("note", "The spec contradicts itself. Answer the new decisions with `lastro review --as <name> answer <D> <option>`, approve the intents they create, and run `lastro spec` again.")),
                            Summary(back, lastro.Pending(back))));
                    }
                    WriteJson(F("spec.json"), spec);
                    var clauses = new JsonArray();
                    foreach (var c in spec.Plan.Clauses) clauses.Add(Obj(("id", c.Id), ("intentIds", LastroJson.ToNode(c.IntentIds)), ("description", c.Description)));
                    return (0, Obj(("status", "spec"), ("artifactHash", spec.Artifact.Hash), ("clauses", clauses), ("beyondProof", LastroJson.ToNode(spec.Plan.BeyondProof))));
                }
                case "prove":
                {
                    var cfg = Cfg();
                    if (rest.Count == 0) throw new ExitException(2, Obj(("error", "prove needs the assembly that holds the function under proof")));
                    if (!File.Exists(F("spec.json"))) throw new ExitException(2, Obj(("error", "no spec yet: run `lastro spec`")));
                    var spec = ReadJson<Spec>(F("spec.json"));
                    var subject = LoadSubject(Path.GetFullPath(rest[0]), args.Flags.GetValueOrDefault("export"));
                    var traces = args.Flags.TryGetValue("traces", out var tp) ? ReadJson<List<Trace>>(Path.GetFullPath(tp)) : [];
                    var stream = args.Flags.ContainsKey("stream") ? emit : null;
                    if (stream is not null)
                    {
                        var inner = subject;
                        var runs = 0;
                        subject = input =>
                        {
                            if (++runs % StreamProgressEvery == 0) stream(Obj(("event", "progress"), ("runs", runs)));
                            return inner(input);
                        };
                        stream(Obj(("event", "start"), ("model", spec.Plan.Model.Name), ("clauses", LastroJson.ToNode(spec.Plan.Clauses.Select(c => c.Id).ToList())),
                            ("scenarios", LastroJson.ToNode(spec.Plan.Scenarios.Select(x => x.Id).ToList()))));
                    }
                    var lastro = new LastroClient(new LastroOptions { Model = cfg.Model, RequiredApprovals = cfg.RequiredApprovals, Rust = RustProver.Locate() });
                    var proof = await lastro.ProveAsync(spec, subject, traces).ConfigureAwait(false);
                    if (stream is not null)
                    {
                        foreach (var r in proof.Report.Clauses)
                        {
                            var clause = spec.Plan.Clauses.First(c => c.Id == r.ClauseId);
                            var ev = Obj(("event", "clause"), ("clauseId", r.ClauseId), ("intentIds", LastroJson.ToNode(clause.IntentIds)), ("verdict", LastroJson.ToNode(r.Verdict)), ("evidence", LastroJson.ToNode(r.Evidence)));
                            if (r.Counterexample is { } cx) ev["counterexample"] = LastroJson.ToNode(cx);
                            stream(ev);
                        }
                        foreach (var r in proof.Report.Scenarios) stream(Obj(("event", "scenario"), ("scenarioId", r.ScenarioId), ("status", LastroJson.ToNode(r.Status))));
                        if (proof.EnginesAgree is { } agree) stream(Obj(("event", "crosscheck"), ("enginesAgree", agree)));
                    }
                    WriteJson(F("proof.json"), proof);
                    File.WriteAllText(F("receipt.md"), proof.Receipt + "\n");
                    return (proof.Ok ? 0 : 1, Obj(("status", proof.Ok ? "proved" : "not-proved"), ("ok", proof.Ok), ("enginesAgree", proof.EnginesAgree is { } ea ? JsonValue.Create(ea) : null),
                        ("failures", LastroJson.ToNode(proof.Failures)), ("specGaps", LastroJson.ToNode(proof.SpecGaps)), ("receipt", F("receipt.md")), ("feedback", Feedback.AgentFeedback(proof))));
                }
                case "feedback":
                {
                    if (!File.Exists(F("proof.json"))) throw new ExitException(2, Obj(("error", "no proof yet: run `lastro prove`")));
                    var proof = ReadJson<Proof>(F("proof.json"));
                    return (proof.Failures.Count > 0 ? 1 : 0, Obj(("status", "feedback"), ("feedback", Feedback.AgentFeedback(proof))));
                }
                default:
                    throw new ExitException(2, Obj(("error", cmd is null ? "no command" : $"unknown command {cmd}"), ("usage", Usage)));
            }
        }
        catch (ExitException e)
        {
            return (e.Code, e.Body);
        }
        catch (Exception e)
        {
            return (2, Obj(("error", e.Message)));
        }
    }

    /// <summary>
    /// Loads <c>Namespace.Type.Method</c> from an assembly as a subject. The method is static, takes one argument
    /// (<see cref="Env"/>, or a dictionary of string to object) and returns the same kinds, optionally as a Task.
    /// Integers arrive as <c>long</c>, other numbers as <c>double</c>. The assembly does not need to reference Lastro.
    /// </summary>
    public static Subject LoadSubject(string assemblyPath, string? export)
    {
        if (string.IsNullOrEmpty(export)) throw new ExitException(2, Obj(("error", "prove needs --export <Namespace.Type.Method>")));
        var asm = Assembly.LoadFrom(assemblyPath);
        var dot = export.LastIndexOf('.');
        if (dot < 0) throw new ExitException(2, Obj(("error", $"--export must be Namespace.Type.Method, got {export}")));
        var type = asm.GetType(export[..dot]) ?? throw new ExitException(2, Obj(("error", $"{assemblyPath} has no type {export[..dot]}")));
        var method = type.GetMethods(BindingFlags.Public | BindingFlags.Static).FirstOrDefault(m => m.Name == export[(dot + 1)..] && m.GetParameters().Length == 1)
                     ?? throw new ExitException(2, Obj(("error", $"{type.FullName} has no public static method {export[(dot + 1)..]} with one parameter")));
        var paramType = method.GetParameters()[0].ParameterType;
        return async input =>
        {
            object arg = paramType.IsAssignableFrom(typeof(Env)) && paramType != typeof(object) ? input : input.ToObjects();
            if (!paramType.IsInstanceOfType(arg)) throw new InvalidOperationException($"{export} must take Env or IDictionary<string, object?>");
            object? result;
            try
            {
                result = method.Invoke(null, [arg]);
            }
            catch (TargetInvocationException e) when (e.InnerException is not null)
            {
                throw e.InnerException;
            }
            if (result is Task task)
            {
                await task.ConfigureAwait(false);
                result = task.GetType().GetProperty("Result")?.GetValue(task);
            }
            return result switch
            {
                Env env => env,
                IEnumerable<KeyValuePair<string, object?>> d => Env.From(d),
                JsonObject o => LastroJson.FromNode<Env>(o),
                _ => throw new InvalidOperationException($"{export} returned {result?.GetType().Name ?? "null"}, expected a dictionary"),
            };
        };
    }
}

public static class Program
{
    public static async Task<int> Main(string[] argv)
    {
        var streaming = Cli.IsStreaming(argv);
        var (code, body) = await Cli.RunAsync(argv, e => Console.Out.Write(JsJson.Stringify(e) + "\n")).ConfigureAwait(false);
        if (streaming)
        {
            var result = new JsonObject { ["event"] = "result" };
            foreach (var (k, v) in body) result[k] = v?.DeepClone();
            Console.Out.Write(JsJson.Stringify(result) + "\n");
        }
        else Console.Out.Write(JsJson.Stringify(body, indent: 2) + "\n");
        return code;
    }
}
