using System.Diagnostics;
using System.Globalization;
using System.Reflection;
using System.Text.Json.Nodes;
using System.Text.RegularExpressions;
using Lastro.Claude;

namespace Lastro.Cli;

/// <summary>
/// <c>lastro</c> on .NET: the SDK as a command line, for people and for any agent harness. Every command
/// prints one JSON object and keeps its state in <c>.lastro/</c>, in the same format as the TypeScript CLI,
/// so the two are interchangeable on one workspace.
///
/// Exit codes: 0 done/ok · 1 not ok (pending review, refused, failing proofs) · 2 usage or error ·
/// 3 an LLM answer is needed (handoff).
/// </summary>
public static class Cli
{
    public const string Usage = """
        lastro <command> [options]

          init --model <model.json> [--subject <assembly.dll>#<Namespace.Type.Method>] [--approvals N] [--submitter-cannot-approve]
                                                        start a .lastro/ workspace (or add a model to it)
          propose <prose-file> [--section <heading>] [--as <submitter>] [--proposal <P>]
                                                        create a proposal and extract its intents against the use cases;
                                                        --proposal re-extracts an existing one and keeps its review
          status [<P>]                                  proposals, or one proposal's intents and pending items
          review <P> --as <name> <action> ...           approve <I..> | approve-all | amend <I> <text> | reject <I> <note>
                                                        answer <D> <option> | comment <I|PF|UC|prose> <text> | reply <T> <text>
                                                        resolve <T> | rewrite <UC> <after> | recombine <UC> [instructions]
          derive <P>                                    derive the pending proofs of the approved intents
          prove [<P>] [<assembly.dll> --export <Namespace.Type.Method>] [--stream]
                                                        run its proofs and every use case's proofs (regression);
                                                        writes the receipt next to the proposal
          feedback <P>                                  the last run's failures, as a brief for whoever writes the code
          apply <P>                                     attach the proofs to their use cases and sync .lastro/usecases/
          regress [<assembly.dll> --export <Namespace.Type.Method>]
                                                        run every use case's proofs alone

          <P> is a proposal id (P-001, or 1) or slug; without it, the latest open proposal.
          --dir <path>          workspace (default .lastro)
          --llm anthropic|handoff
        """;

    public sealed class ExitException(int code, JsonObject body) : Exception(body["error"]?.ToString() ?? body["status"]?.ToString() ?? code.ToString(CultureInfo.InvariantCulture))
    {
        public int Code { get; } = code;
        public JsonObject Body { get; } = body;
    }

    /// <summary>Flags that never take a value.</summary>
    private static readonly HashSet<string> BooleanFlags = new(StringComparer.Ordinal) { "stream", "submitter-cannot-approve" };

    /// <summary>One <c>prove --stream</c> progress event every this many runs of the code, as in the TS CLI.</summary>
    public const int StreamProgressEvery = 100;

    /// <summary>The intent-artifact/v0 workspace files, which proposal/v1 refuses.</summary>
    private static readonly string[] LegacyFiles = ["intents.json", "spec.json", "proof.json"];

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

    private static T ReadJson<T>(string path) => LastroJson.Deserialize<T>(File.ReadAllText(path));

    /// <summary>Writes <c>JSON.stringify(value, null, 2) + "\n"</c>, as the TS CLI does.</summary>
    private static void WriteJson(string path, JsonNode? value)
    {
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        File.WriteAllText(path, JsJson.Stringify(value, indent: 2) + "\n");
    }

    private static void WriteJson<T>(string path, T value) => WriteJson(path, LastroJson.ToNode(value));

    private static JsonObject Obj(params (string Key, JsonNode? Value)[] items)
    {
        var o = new JsonObject();
        foreach (var (k, v) in items) o[k] = v;
        return o;
    }

    private static JsonArray Strings(IEnumerable<string> xs) => new([.. xs.Select(x => (JsonNode)JsonValue.Create(x)!)]);

    private static JsonObject With(JsonObject head, JsonObject tail)
    {
        foreach (var (k, v) in tail) head[k] = v?.DeepClone();
        return head;
    }

    // -----------------------------------------------------------------------
    // Workspace (same files as the TS CLI's lastro/workspace)
    // -----------------------------------------------------------------------

    private sealed class Config
    {
        /// <summary>Models in config order.</summary>
        public Dictionary<string, Model> Models { get; } = new(StringComparer.Ordinal);
        /// <summary>The TS CLI's <c>{module, export}</c> entries, as written.</summary>
        public JsonObject Subjects { get; set; } = [];
        public ProposalPolicy Review { get; set; } = new();
    }

    private sealed class Workspace(string dir)
    {
        public string Dir { get; } = dir;

        public string File(params string[] parts) => Path.Combine([Dir, .. parts]);

        public bool Exists => System.IO.File.Exists(File("config.json"));

        public List<string> Legacy() => LegacyFiles.Where(f => System.IO.File.Exists(File(f))).ToList();

        public Config Config()
        {
            if (!Exists) throw new ExitException(2, Obj(("error", $"no workspace at {Dir}: run `lastro init --model model.json`")));
            var raw = JsonNode.Parse(System.IO.File.ReadAllText(File("config.json"))) as JsonObject ?? [];
            var c = new Config();
            if (raw["models"] is JsonObject models)
                foreach (var (name, m) in models) c.Models[name] = LastroJson.FromNode<Model>(m);
            if (raw["model"] is JsonObject legacyModel)
            {
                var m = LastroJson.FromNode<Model>(legacyModel); // the v0 `model` key is still read
                c.Models.TryAdd(m.Name, m);
            }
            if (raw["subjects"] is JsonObject subjects) c.Subjects = (JsonObject)subjects.DeepClone();
            var review = raw["review"] as JsonObject;
            var approvals = review?["approvals"] is JsonValue a ? a.GetValue<int>() : raw["requiredApprovals"] is JsonValue r ? r.GetValue<int>() : 1;
            c.Review = new ProposalPolicy
            {
                Approvals = Math.Max(1, approvals),
                SubmitterCanApprove = review?["submitterCanApprove"] is not JsonValue s || s.GetValue<bool>(),
            };
            return c;
        }

        public void WriteConfig(Config c)
        {
            var models = new JsonObject();
            foreach (var (name, m) in c.Models) models[name] = LastroJson.ToNode(m);
            WriteJson(File("config.json"), Obj(("models", models), ("subjects", c.Subjects.DeepClone()), ("review", LastroJson.ToNode(c.Review))));
        }

        private static IEnumerable<string> JsonFiles(string dir) =>
            Directory.Exists(dir) ? Directory.GetFiles(dir, "*.json").Select(Path.GetFileName).OrderBy(f => f, StringComparer.Ordinal)!.Cast<string>() : [];

        public List<UseCase> UseCases() => JsonFiles(File("usecases")).Select(f => ReadJson<UseCase>(File("usecases", f))).ToList();

        public void WriteUseCase(UseCase u) => WriteJson(File("usecases", Proposals.FileNameOf(u.Id, u.Slug)), u);

        public void RemoveUseCase(UseCase u) => System.IO.File.Delete(File("usecases", Proposals.FileNameOf(u.Id, u.Slug)));

        public List<Proposal> All() => JsonFiles(File("proposals")).Select(f => ReadJson<Proposal>(File("proposals", f))).ToList();

        public string NextProposalId() => Proposals.NextId("P", Directory.Exists(File("proposals"))
            ? Directory.GetFiles(File("proposals")).Select(f => Regex.Match(Path.GetFileName(f), "^(P-\\d+)-")).Where(m => m.Success).Select(m => m.Groups[1].Value)
            : []);

        /// <summary>A proposal by id ("P-001", "1") or slug; without one, the latest that is not applied or closed.</summary>
        public Proposal Proposal(string? reference)
        {
            var all = All();
            if (string.IsNullOrEmpty(reference))
                return all.LastOrDefault(p => p.Status is not ("applied" or "closed")) ?? throw new ExitException(2, Obj(("error", "no open proposal: run `lastro propose <prose-file>`")));
            var id = int.TryParse(reference, NumberStyles.None, CultureInfo.InvariantCulture, out var n) ? $"P-{n:000}" : reference;
            return all.FirstOrDefault(p => p.Id == id || p.Slug == reference) ?? throw new ExitException(2, Obj(("error", $"no proposal {reference}")));
        }

        public string ProposalPath(Proposal p, string ext) => File("proposals", $"{p.Id}-{p.Slug}{ext}");

        public void WriteProposal(Proposal p) => WriteJson(ProposalPath(p, ".json"), p);

        /// <summary>Adds one line to <c>&lt;id&gt;-&lt;slug&gt;.runs.jsonl</c>: every prove run, with its time and commit.</summary>
        public void AppendRun(Proposal p, ProposalRun run)
        {
            var path = ProposalPath(p, ".runs.jsonl");
            var n = 1 + (System.IO.File.Exists(path) ? System.IO.File.ReadAllLines(path).Count(l => !string.IsNullOrWhiteSpace(l)) : 0);
            var line = Obj(("run", n), ("at", DateTime.UtcNow.ToString("yyyy-MM-dd'T'HH:mm:ss.fff'Z'", CultureInfo.InvariantCulture)));
            var root = Path.GetDirectoryName(Dir)!;
            if (Git(root, "rev-parse", "HEAD") is { } head)
            {
                line["commit"] = head.Trim();
                if (Git(root, "status", "--porcelain") is { } st && st.Trim() != "") line["dirty"] = true;
            }
            var r = LastroJson.ToNode(run)!.AsObject();
            foreach (var k in new[] { "ok", "outcomes", "regressions", "replacedStillPass", "unproved", "enginesAgree" })
                if (r[k] is { } v) line[k] = v.DeepClone();
            System.IO.File.AppendAllText(path, JsJson.Stringify(line) + "\n");
        }

        private static string? Git(string cwd, params string[] args)
        {
            try
            {
                var psi = new ProcessStartInfo("git") { RedirectStandardOutput = true, RedirectStandardError = true, UseShellExecute = false };
                foreach (var a in (string[])["-C", cwd, .. args]) psi.ArgumentList.Add(a);
                using var p = Process.Start(psi)!;
                var o = p.StandardOutput.ReadToEnd();
                p.WaitForExit();
                return p.ExitCode == 0 ? o : null;
            }
            catch (Exception e) when (e is System.ComponentModel.Win32Exception or InvalidOperationException)
            {
                return null;
            }
        }
    }

    // -----------------------------------------------------------------------
    // LLM
    // -----------------------------------------------------------------------

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

    // -----------------------------------------------------------------------
    // Output
    // -----------------------------------------------------------------------

    /// <summary>The body of the Markdown section whose heading is <paramref name="title"/>, up to the next heading of the same or a higher level.</summary>
    private static string Section(string markdown, string title)
    {
        var lines = markdown.Split('\n');
        var heading = new Regex("^(#{1,6})\\s");
        static string Norm(string s) => s.Trim().ToLowerInvariant();
        var start = Array.FindIndex(lines, l => heading.IsMatch(l) && Norm(l.TrimStart('#')) == Norm(title));
        if (start < 0) throw new ExitException(2, Obj(("error", $"no section \"{title}\"")));
        var level = heading.Match(lines[start]).Groups[1].Value.Length;
        var end = lines.Length;
        for (var i = start + 1; i < lines.Length; i++)
            if (heading.Match(lines[i]) is { Success: true } m && m.Groups[1].Value.Length <= level)
            {
                end = i;
                break;
            }
        return string.Join("\n", lines[(start + 1)..end]);
    }

    /// <summary>One proposal as the CLI prints it, key for key the TS CLI's summary().</summary>
    private static JsonObject Summary(Proposal p, IReadOnlyList<UseCase> useCases)
    {
        var described = new Dictionary<string, string>(StringComparer.Ordinal);
        foreach (var x in useCases.SelectMany(u => u.Proofs)) described[x.Id] = x.Description;
        var pending = Proposals.Pending(p);
        var changes = new JsonArray();
        foreach (var c in p.UseCases)
        {
            var o = Obj(("useCase", c.UseCase), ("slug", c.Slug), ("change", c.Change), ("name", c.Name));
            if (c.Stale == true) o["stale"] = true;
            o["intents"] = Strings(c.Intents);
            changes.Add(o);
        }
        var intents = new JsonArray();
        foreach (var i in p.Intents)
            intents.Add(Obj(("id", i.Id), ("change", i.Change), ("useCase", i.UseCase), ("status", i.Status), ("approvals", Strings(i.Approvals)),
                ("name", i.Name), ("description", i.Description), ("provability", i.Provability),
                ("replaces", new JsonArray([.. i.Replaces.Select(r => (JsonNode)Obj(("proof", r.Proof), ("reason", r.Reason), ("description", described.TryGetValue(r.Proof, out var d) ? d : null)))]))));
        var decisions = new JsonArray();
        foreach (var d in p.Decisions)
            decisions.Add(Obj(("id", d.Id), ("origin", d.Origin), ("question", d.Question), ("options", Strings(d.Options.Select(o => o.Label))),
                ("chosen", d.Chosen is { } c ? JsonValue.Create(c) : null), ("recommended", d.Recommended)));
        var threads = new JsonArray();
        foreach (var t in p.Threads.Where(t => !t.Resolved))
            threads.Add(Obj(("id", t.Id), ("on", t.On), ("by", t.By), ("comments", LastroJson.ToNode(t.Comments))));
        var proofs = new JsonArray();
        foreach (var x in p.Proofs)
        {
            var o = Obj(("id", x.Id), ("intent", x.Intent), ("useCase", x.UseCase), ("kind", x.Check.Kind), ("description", x.Description), ("status", x.Status));
            if (x.Replaces is { Count: > 0 }) o["replaces"] = Strings(x.Replaces);
            proofs.Add(o);
        }
        return Obj(
            ("proposal", Obj(("id", p.Id), ("slug", p.Slug), ("name", p.Name), ("status", p.Status), ("submittedBy", p.SubmittedBy), ("policy", LastroJson.ToNode(p.Policy)), ("approved", p.Hash is not null))),
            ("useCases", changes), ("intents", intents), ("decisions", decisions), ("threads", threads), ("supersedes", Strings(p.Supersedes)),
            ("proofs", proofs), ("pending", Strings(pending)), ("ready", pending.Count == 0));
    }

    private static JsonObject RunBody(ProposalRun run) => Obj(
        ("ok", run.Ok), ("enginesAgree", run.EnginesAgree is { } ea ? JsonValue.Create(ea) : null),
        ("failures", LastroJson.ToNode(run.Outcomes.Where(x => x.Role != "superseded" && !x.Passed).ToList())),
        ("regressions", Strings(run.Regressions)), ("replacedStillPass", Strings(run.ReplacedStillPass)), ("unproved", Strings(run.Unproved)));

    private static ProofSetup Setup(Config cfg)
    {
        var subjects = cfg.Subjects.Select(kv => kv.Key).ToList();
        return new ProofSetup(cfg.Models, subjects.Count > 0 ? subjects : [.. cfg.Models.Keys]);
    }

    private static string WriteReceipt(Workspace ws, Proposal p, IReadOnlyList<UseCase> useCases)
    {
        var path = ws.ProposalPath(p, ".receipt.md");
        File.WriteAllText(path, Proposals.RenderReceipt(p, useCases) + "\n");
        return path;
    }

    /// <summary>
    /// The code under proof, by subject name: an assembly's static method (positional <c>.dll</c> with <c>--export</c>,
    /// or the config's subject when its module is an assembly).
    /// </summary>
    private static Dictionary<string, Subject> SubjectsOf(Args args, List<string> rest, Config cfg, Workspace ws, Action<JsonObject>? stream)
    {
        var names = cfg.Subjects.Select(kv => kv.Key).ToList();
        if (names.Count > 1)
            throw new ExitException(2, Obj(("error", $"the workspace declares {names.Count} subjects ({string.Join(", ", names)}); the .NET CLI proves exactly one per run")));
        var name = names.Count == 1 ? names[0] : cfg.Models.Keys.FirstOrDefault() ?? "default";
        var assembly = rest.FirstOrDefault(x => x.EndsWith(".dll", StringComparison.OrdinalIgnoreCase));
        var export = args.Flags.GetValueOrDefault("export");
        if (assembly is null && names.Count == 1 && cfg.Subjects[names[0]] is JsonObject entry && entry["module"]?.GetValue<string>() is { } module
            && module.EndsWith(".dll", StringComparison.OrdinalIgnoreCase))
        {
            assembly = Path.GetFullPath(module, Path.GetDirectoryName(ws.Dir)!);
            export ??= entry["export"]?.GetValue<string>();
        }
        if (assembly is null)
            throw new ExitException(2, Obj(("error", "the code under proof is missing: <assembly.dll> --export <Namespace.Type.Method>, or init --subject <assembly.dll>#<Namespace.Type.Method>")));
        var subject = LoadSubject(Path.GetFullPath(assembly), export);
        if (stream is not null)
        {
            var inner = subject;
            var runs = 0;
            subject = input =>
            {
                if (++runs % StreamProgressEvery == 0) stream(Obj(("event", "progress"), ("runs", runs)));
                return inner(input);
            };
        }
        return new Dictionary<string, Subject>(StringComparer.Ordinal) { [name] = subject };
    }

    /// <param name="emit">Receives the <c>prove --stream</c> events (start, progress, proof, crosscheck) in the
    /// TS CLI's order and shape; the caller prints the closing <c>{"event":"result",...}</c>.</param>
    public static async Task<(int Code, JsonObject Body)> RunAsync(string[] argv, Action<JsonObject>? emit = null)
    {
        var args = Parse(argv);
        var cmd = args.Positional.FirstOrDefault();
        var rest = args.Positional.Skip(1).ToList();
        var ws = new Workspace(Path.GetFullPath(args.Flags.GetValueOrDefault("dir") ?? ".lastro"));
        string Arg(List<string> xs, int i) => i < xs.Count ? xs[i] : "";

        try
        {
            if (cmd is not null && cmd != "init" && ws.Legacy() is { Count: > 0 } legacy)
                throw new ExitException(2, Obj(("error", $"{ws.Dir} holds {string.Join(", ", legacy)} from the old workspace format, which this version does not read. Delete them and run `lastro propose <prose-file>` again."),
                    ("legacy", Strings(legacy))));
            if (cmd is "plan" or "spec")
                throw new ExitException(2, Obj(("error", $"`lastro {cmd}` was replaced: use `lastro propose`, `lastro review` and `lastro derive`"), ("usage", Usage)));

            switch (cmd)
            {
                case "init":
                {
                    if (!args.Flags.TryGetValue("model", out var modelPath)) throw new ExitException(2, Obj(("error", "init needs --model <model.json>")));
                    var model = ReadJson<Model>(Path.GetFullPath(modelPath));
                    var cfg = ws.Exists ? ws.Config() : new Config();
                    cfg.Models[model.Name] = model;
                    if (args.Flags.TryGetValue("subject", out var s) && s != "true")
                    {
                        var hash = s.IndexOf('#');
                        var reference = Obj(("module", hash < 0 ? s : s[..hash]));
                        if (hash >= 0 && hash < s.Length - 1) reference["export"] = s[(hash + 1)..];
                        cfg.Subjects[args.Flags.GetValueOrDefault("subject-name") ?? model.Name] = reference;
                    }
                    if (args.Flags.TryGetValue("approvals", out var ap))
                    {
                        if (!int.TryParse(ap, NumberStyles.Integer, CultureInfo.InvariantCulture, out var n)) throw new ExitException(2, Obj(("error", "--approvals needs a number")));
                        cfg.Review.Approvals = Math.Max(1, n);
                    }
                    if (args.Flags.ContainsKey("submitter-cannot-approve")) cfg.Review.SubmitterCanApprove = false;
                    ws.WriteConfig(cfg);
                    return (0, Obj(("status", "initialized"), ("dir", ws.Dir), ("models", Strings(cfg.Models.Keys)), ("subjects", Strings(cfg.Subjects.Select(kv => kv.Key))),
                        ("review", LastroJson.ToNode(cfg.Review))));
                }
                case "propose":
                {
                    var cfg = ws.Config();
                    if (rest.Count == 0) throw new ExitException(2, Obj(("error", "propose needs a prose file")));
                    var proseFile = Path.GetFullPath(rest[0]);
                    var text = File.ReadAllText(proseFile);
                    if (args.Flags.TryGetValue("section", out var section)) text = Section(text, section);
                    var prose = text.Trim();
                    var useCases = ws.UseCases();
                    // A prose file already in .lastro/proposals/ named <id>-<slug>.md names its proposal: re-extract it if it exists.
                    var inProposals = Path.GetDirectoryName(proseFile) == ws.File("proposals");
                    var named = inProposals ? Regex.Match(Path.GetFileName(proseFile), "^(P-\\d+)-([a-z0-9]+(?:-[a-z0-9]+)*)\\.md$") : Match.Empty;
                    var all = ws.All();
                    Proposal? previous = args.Flags.TryGetValue("proposal", out var pref) ? ws.Proposal(pref)
                        : named.Success ? all.FirstOrDefault(x => x.Id == named.Groups[1].Value) : null;
                    var id = previous?.Id ?? (named.Success ? named.Groups[1].Value : ws.NextProposalId());
                    var submittedBy = args.Flags.GetValueOrDefault("as") ?? previous?.SubmittedBy;
                    if (string.IsNullOrEmpty(submittedBy)) submittedBy = Environment.GetEnvironmentVariable("LASTRO_USER");
                    if (string.IsNullOrEmpty(submittedBy)) submittedBy = "submitter";
                    var p = await Proposals.ExtractAsync(PickAsk(args, ws.Dir), new ProposeInput(id, prose, submittedBy, useCases, cfg.Review, Proposals.ReservedUseCaseIds(all, id))).ConfigureAwait(false);
                    if (previous is not null)
                    {
                        p = Proposals.CarryReview(previous, p);
                        p.Slug = previous.Slug; // slugs are stable
                    }
                    else if (named.Success) p.Slug = named.Groups[2].Value;
                    ws.WriteProposal(p);
                    var md = ws.ProposalPath(p, ".md");
                    if (md != proseFile)
                    {
                        File.WriteAllText(md, prose + "\n");
                        if (inProposals && !named.Success) File.Delete(proseFile); // a draft in .lastro/proposals/ becomes <id>-<slug>.md
                    }
                    return (Proposals.Pending(p).Count == 0 ? 0 : 1, With(Obj(("status", "proposed"), ("file", ws.ProposalPath(p, ".json"))), Summary(p, useCases)));
                }
                case "status":
                {
                    var useCases = ws.UseCases();
                    var reference = Arg(rest, 0) is { Length: > 0 } r0 ? r0 : args.Flags.GetValueOrDefault("proposal");
                    if (string.IsNullOrEmpty(reference))
                    {
                        var all = ws.All();
                        var list = new JsonArray([.. all.Select(p => (JsonNode)Obj(("id", p.Id), ("slug", p.Slug), ("name", p.Name), ("status", p.Status), ("pending", Proposals.Pending(p).Count)))]);
                        var open = all.Count(p => p.Status is not ("applied" or "closed"));
                        return (open > 0 ? 1 : 0, Obj(("status", "status"), ("useCases", useCases.Count), ("proofs", useCases.Sum(u => u.Proofs.Count)),
                            ("unproved", Strings(useCases.Where(u => u.Proofs.Count == 0).Select(u => u.Id))), ("proposals", list)));
                    }
                    var one = ws.Proposal(reference);
                    return (Proposals.Pending(one).Count == 0 ? 0 : 1, With(Obj(("status", "status")), Summary(one, useCases)));
                }
                case "review":
                {
                    if (!args.Flags.TryGetValue("as", out var who) || who == "true") throw new ExitException(2, Obj(("error", "review needs --as <name>")));
                    if (rest.Count < 2) throw new ExitException(2, Obj(("error", "review needs a proposal and an action"), ("usage", Usage)));
                    var (reference, action, xs) = (rest[0], rest[1], rest.Skip(2).ToList());
                    var useCases = ws.UseCases();
                    var r = Proposals.Review(ws.Proposal(reference), who);
                    var tail = string.Join(" ", xs.Skip(1));
                    switch (action)
                    {
                        case "approve": r.Approve([.. xs]); break;
                        case "approve-all": r.ApproveAll(); break;
                        case "amend": r.Amend(Arg(xs, 0), tail); break;
                        case "reject": r.Reject(Arg(xs, 0), tail == "" ? "rejected" : tail); break;
                        case "answer":
                            if (!int.TryParse(Arg(xs, 1), NumberStyles.Integer, CultureInfo.InvariantCulture, out var option)) throw new ExitException(2, Obj(("error", "answer needs <decision> <option-index>")));
                            r.Answer(Arg(xs, 0), option);
                            break;
                        case "comment": r.Comment(Arg(xs, 0), tail); break;
                        case "reply": r.Reply(Arg(xs, 0), tail); break;
                        case "resolve": r.Resolve(Arg(xs, 0)); break;
                        case "rewrite": r.RewriteAfter(Arg(xs, 0), tail); break;
                        case "recombine":
                            var (name, after) = await Proposals.RecombineAsync(PickAsk(args, ws.Dir), r.Done(), Arg(xs, 0), tail).ConfigureAwait(false);
                            r.RewriteAfter(Arg(xs, 0), after, name);
                            break;
                        default: throw new ExitException(2, Obj(("error", $"unknown review action {action}"), ("usage", Usage)));
                    }
                    var p = r.Done();
                    // Fully reviewed: approve it, bound to the current use cases.
                    if (Proposals.Pending(p).Count == 0 && p.Hash is null)
                    {
                        if (Proposals.BaselineOf(useCases) != p.Baseline)
                        {
                            ws.WriteProposal(p);
                            throw new ExitException(1, Obj(("status", "stale"), ("error", $"the use cases changed since {p.Id} was extracted: run `lastro propose <prose-file> --proposal {p.Id}` again")));
                        }
                        p = Proposals.Approve(p, useCases);
                    }
                    ws.WriteProposal(p);
                    return (0, With(Obj(("status", "reviewed"), ("reviewer", who)), Summary(p, useCases)));
                }
                case "derive":
                {
                    var cfg = ws.Config();
                    var useCases = ws.UseCases();
                    var p = ws.Proposal(Arg(rest, 0));
                    if (p.Hash is null) throw new ExitException(1, Obj(("status", "review-pending"), ("pending", Strings(Proposals.Pending(p)))));
                    var next = await Proposals.DeriveAsync(PickAsk(args, ws.Dir), p, useCases, Setup(cfg)).ConfigureAwait(false);
                    ws.WriteProposal(next);
                    if (next.Hash is null)
                        return (1, With(Obj(("status", "back-to-review"), ("note", $"Derivation raised decisions. Answer them with `lastro review {next.Id} --as <name> answer <D> <option>`, review the intents they reopened, and run `lastro derive {next.Id}` again.")),
                            Summary(next, useCases)));
                    return (0, With(Obj(("status", "derived")), Summary(next, useCases)));
                }
                case "prove":
                {
                    var cfg = ws.Config();
                    var useCases = ws.UseCases();
                    var p = ws.Proposal(rest.FirstOrDefault(x => !x.EndsWith(".dll", StringComparison.OrdinalIgnoreCase)));
                    var stream = args.Flags.ContainsKey("stream") ? emit : null;
                    var subjects = SubjectsOf(args, rest, cfg, ws, stream);
                    stream?.Invoke(Obj(("event", "start"), ("proposal", p.Id), ("proofs", Strings(p.Proofs.Select(x => x.Id)))));
                    var (proposal, run) = await Proposals.ProveAsync(p, useCases, new RunOptions(subjects, cfg.Models, RustProver.Locate())).ConfigureAwait(false);
                    if (stream is not null)
                    {
                        foreach (var o in run.Outcomes)
                        {
                            var ev = Obj(("event", "proof"), ("proof", o.Proof), ("role", o.Role), ("verdict", o.Verdict), ("passed", o.Passed));
                            if (o.Counterexample is { } cx) ev["counterexample"] = LastroJson.ToNode(cx);
                            stream(ev);
                        }
                        if (run.EnginesAgree is { } agree) stream(Obj(("event", "crosscheck"), ("enginesAgree", agree)));
                    }
                    ws.WriteProposal(proposal);
                    ws.AppendRun(proposal, run);
                    var receipt = WriteReceipt(ws, proposal, useCases);
                    return (run.Ok ? 0 : 1, With(With(Obj(("status", run.Ok ? "proved" : "not-proved"), ("proposal", proposal.Id)), RunBody(run)),
                        Obj(("receipt", receipt), ("feedback", Proposals.Feedback(proposal, useCases)))));
                }
                case "feedback":
                {
                    var useCases = ws.UseCases();
                    var p = ws.Proposal(Arg(rest, 0));
                    return (p.LastRun is { Ok: true } ? 0 : 1, Obj(("status", "feedback"), ("proposal", p.Id), ("feedback", Proposals.Feedback(p, useCases))));
                }
                case "apply":
                {
                    var useCases = ws.UseCases();
                    var p = ws.Proposal(Arg(rest, 0));
                    var applied = Proposals.Apply(p, useCases);
                    foreach (var u in useCases.Where(u => applied.Removed.Contains(u.Id))) ws.RemoveUseCase(u);
                    foreach (var u in applied.UseCases.Where(u => applied.Written.Contains(u.Id))) ws.WriteUseCase(u);
                    ws.WriteProposal(applied.Proposal);
                    var receipt = WriteReceipt(ws, applied.Proposal, useCases);
                    return (0, Obj(("status", "applied"), ("proposal", p.Id), ("written", Strings(applied.Written)), ("removed", Strings(applied.Removed)),
                        ("baseline", applied.Baseline), ("unproved", Strings(p.LastRun?.Unproved ?? [])), ("receipt", receipt)));
                }
                case "regress":
                {
                    var cfg = ws.Config();
                    var useCases = ws.UseCases();
                    var run = await Proposals.RegressAsync(useCases, new RunOptions(SubjectsOf(args, rest, cfg, ws, null), cfg.Models, RustProver.Locate())).ConfigureAwait(false);
                    return (run.Ok ? 0 : 1, With(Obj(("status", run.Ok ? "no-regressions" : "regressions")), RunBody(run)));
                }
                default:
                    throw new ExitException(2, Obj(("error", cmd is null ? "no command" : $"unknown command {cmd}"), ("usage", Usage)));
            }
        }
        catch (ExitException e)
        {
            return (e.Code, e.Body);
        }
        catch (ProposalException e)
        {
            return (1, Obj(("status", "refused"), ("problems", Strings(e.Problems))));
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
        if (string.IsNullOrEmpty(export)) throw new ExitException(2, Obj(("error", "the code under proof needs --export <Namespace.Type.Method>")));
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
