using System.Text.Json.Nodes;
using System.Text.RegularExpressions;
using Lastro.Cli;
using Xunit;
using static Lastro.Tests.Support;

namespace Lastro.Tests;

/// <summary>
/// The .NET SDK against the other two engines: the TypeScript SDK (hashes, verdicts, receipts, handoff ids,
/// sampling, the CLI workspace) and the Rust prover (verdict by verdict).
/// </summary>
public class InteropTests
{
    private static readonly Lazy<JsonObject> Ts = new(() =>
    {
        var (code, stdout, stderr) = Run("node", [Path.Combine(AppContext.BaseDirectory, "crosscheck.mjs"), RepoRoot]);
        if (code != 0) throw new InvalidOperationException($"crosscheck.mjs failed: {stderr}");
        return (JsonObject)JsonNode.Parse(stdout)!;
    });

    private static IntentArtifact CaseArtifact(string name) => name switch
    {
        "oneWay" => Reviewed(d1: 1),
        "misread" => Reviewed(fixI5: false),
        _ => Reviewed(),
    };

    [SkippableFact]
    public async Task TypeScriptAndDotNetAgreeOnHashesVerdictsAndReceipts()
    {
        Require(TsBuilt, TsMissing);
        foreach (var (name, tsCase) in Ts.Value["cases"]!.AsObject())
        {
            var artifact = CaseArtifact(name);
            var plan = LoginExample.Derive(artifact);
            Assert.Equal(tsCase!["artifactHash"]!.GetValue<string>(), artifact.Hash);
            Assert.Equal(tsCase["planHash"]!.GetValue<string>(), Canonical.HashOf(plan));

            // An artifact approved by the TypeScript SDK is intact for the .NET SDK, and vice versa.
            var tsArtifact = LastroJson.FromNode<IntentArtifact>(tsCase["artifact"]);
            Assert.True(Artifacts.IsApproved(tsArtifact), name);

            foreach (var (bug, expected) in tsCase["bugs"]!.AsObject())
            {
                var report = await Engine.VerifyAsync(artifact, plan, new VerifyOptions { Subject = LoginExample.Subject(bug == "none" ? null : bug) });
                var where = $"{name}/{bug}";
                Assert.Equal(expected!["ok"]!.GetValue<bool>(), report.Ok);
                foreach (var c in report.Clauses) Assert.True(expected["clauses"]![c.ClauseId]!.GetValue<string>() == Wire.Of(c.Verdict), $"{where} {c.ClauseId}");
                foreach (var i in report.Intents) Assert.Equal(expected["intents"]![i.IntentId]!.GetValue<string>(), Wire.Of(i.Verdict));
                Assert.Equal(expected["receipt"]!.GetValue<string>(), Receipt.Render(report, plan));
            }
        }
    }

    [SkippableFact]
    public void BothSdksSendTheSameLlmRequests()
    {
        Require(TsBuilt, TsMissing);
        var extract = new LlmRequest("extract", LlmPrompts.ExtractSystem, LlmPrompts.ExtractUserPrompt(LoginExample.Prose), LlmPrompts.ExtractSchema);
        Assert.Equal(Ts.Value["extractId"]!.GetValue<string>(), extract.Id);
        var derive = new LlmRequest("derive", LlmPrompts.DeriveSystem, LlmPrompts.DeriveUserPrompt(Reviewed(), LoginExample.Model), LlmPrompts.DeriveSchema);
        Assert.Equal(Ts.Value["deriveId"]!.GetValue<string>(), derive.Id);
    }

    [SkippableFact]
    public async Task SamplingAndContradictionsAreBitIdentical()
    {
        Require(TsBuilt, TsMissing);
        var big = Artifacts.Approve(Artifacts.Review(Artifacts.Create("x doubles", [new("y = 2x", IntentKind.Behavior, Provability.Formal, "x doubles")])).ApproveAll().Done());
        var plan = new ProofPlan
        {
            ArtifactHash = big.Hash!,
            Model = new Model { Name = "double", Inputs = new() { ["x"] = D.Int(0, 1_000_000), ["s"] = D.String("a", "b") }, Outputs = new() { ["y"] = D.Int(-1000, 10_000_000) } },
            Clauses = [new() { Id = "C1", IntentIds = ["I1"], Description = "y = 2x", Requires = L.Gt(L.In("x"), 500_000), Ensures = L.Eq(L.Out("y"), L.Mul(L.C(2), L.In("x"))) }],
        };
        var expected = Ts.Value["sampling"]!;
        Assert.Equal(expected["artifactHash"]!.GetValue<string>(), big.Hash);
        Assert.Equal(expected["planHash"]!.GetValue<string>(), Canonical.HashOf(plan));
        var r = await Engine.VerifyAsync(big, plan, new VerifyOptions { Subject = Subjects.From(i => new Env { ["y"] = 2 * i["x"].AsNumber }), Samples = 500 });
        Assert.Equal(expected["checked"]!.GetValue<int>(), r.Clauses[0].Checked);
        Assert.Equal(expected["bound"]!.GetValue<double>(), r.Clauses[0].FailureRateUpperBound95);

        plan.Model = new Model { Name = "c", Inputs = new() { ["x"] = D.Int(0, 9) }, Outputs = new() { ["y"] = D.Int(0, 5) } };
        plan.Clauses =
        [
            new() { Id = "C1", IntentIds = ["I1"], Description = "y is 1 when x=3", Requires = L.Eq(L.In("x"), 3), Ensures = L.Eq(L.Out("y"), 1) },
            new() { Id = "C2", IntentIds = ["I1"], Description = "y is 2 when x=3", Requires = L.Eq(L.In("x"), 3), Ensures = L.Eq(L.Out("y"), 2) },
        ];
        Assert.Equal(JsJson.Stringify(Ts.Value["contradictions"]), JsJson.Stringify(LastroJson.ToNode(Engine.CheckConsistency(plan))));
    }

    private static readonly Lazy<RustProver?> Prover = new(() => RustProver.Locate(RepoRoot));

    private const string RustMissing = "lastro-prove not found (run `cargo build --release -p lastro-core` or set LASTRO_PROVE)";

    [SkippableFact]
    public async Task RustProverAgreesWithTheDotNetEngineOnEveryBug()
    {
        Require(Prover.Value is not null, RustMissing);
        var a = Reviewed();
        var plan = LoginExample.Derive(a);
        foreach (var bug in new string?[] { null }.Concat(LoginExample.Bugs))
        {
            var subject = LoginExample.Subject(bug);
            var ts = await Engine.VerifyAsync(a, plan, new VerifyOptions { Subject = subject });
            var rs = Prover.Value!.Prove(plan, await RustProver.TabulateAsync(plan.Model, subject));
            Assert.True(rs.Coverage.Complete);
            Assert.Equal(72, rs.Coverage.Space);
            foreach (var c in ts.Clauses) Assert.Equal(c.Verdict, rs.Clauses.First(x => x.ClauseId == c.ClauseId).Verdict);
            Assert.Equal(ts.Clauses.All(c => c.Verdict == ClauseVerdict.Proved), rs.Ok);
        }
    }

    [SkippableFact]
    public async Task RustProverRefusesToCallAPartialTableAProof()
    {
        Require(Prover.Value is not null, RustMissing);
        var plan = LoginExample.Derive(Reviewed());
        var table = (await RustProver.TabulateAsync(plan.Model, LoginExample.Subject())).Take(50).ToList();
        var rs = Prover.Value!.Prove(plan, table);
        Assert.False(rs.Coverage.Complete);
        Assert.Equal(22, rs.Coverage.Missing);
        Assert.DoesNotContain(rs.Clauses, c => c.Verdict == ClauseVerdict.Proved);
    }

    [SkippableFact]
    public async Task ProofWithTheRustProverReportsAgreementInTheReceipt()
    {
        Require(Prover.Value is not null, RustMissing);
        var lastro = new LastroClient(new LastroOptions
        {
            Model = LoginExample.Model,
            Extractor = new StaticExtractor(LoginExample.Extract),
            Deriver = new StaticDeriver((a, _) => LoginExample.Derive(a)),
            Rust = Prover.Value,
        });
        var spec = await lastro.SpecAsync(lastro.Review(ReviewedLogin(), "ana").ApproveAll().Done());
        var proof = await lastro.ProveAsync(spec, LoginExample.Subject());
        Assert.True(proof.Ok);
        Assert.True(proof.EnginesAgree);
        Assert.Contains("coverage 72/72 inputs", proof.Receipt);
        Assert.Contains("verdicts agree", proof.Receipt);
    }

    // -----------------------------------------------------------------------
    // CLI
    // -----------------------------------------------------------------------

    // The shipping example of the docs: prose, model, and the answers an agent writes to the two handoffs.
    private static string Example => Path.Combine(RepoRoot, "apps", "docs", "examples", "shipping");

    private static readonly string Assembly = typeof(ShippingExample).Assembly.Location;

    private static string Workspace()
    {
        var dir = Path.Combine(Path.GetTempPath(), "lastro-dotnet-" + Guid.NewGuid().ToString("N")[..8]);
        Directory.CreateDirectory(dir);
        foreach (var f in new[] { "model.json", "prose.md", "shipping.mjs", "shipping-buggy.mjs" }) File.Copy(Path.Combine(Example, f), Path.Combine(dir, f));
        return dir;
    }

    private static async Task<(int Code, JsonObject Body)> DotNet(string dir, params string[] args) =>
        await Cli.Cli.RunAsync([.. args, "--dir", Path.Combine(dir, ".lastro"), "--llm", "handoff"]);

    private static (int Code, JsonObject Body) Node(string dir, params string[] args)
    {
        var (code, stdout, stderr) = Run("node", [Path.Combine(RepoRoot, "sdks/typescript/dist/src/cli.js"), .. args, "--dir", Path.Combine(dir, ".lastro"), "--llm", "handoff"], dir);
        return (code, JsonNode.Parse(stdout) as JsonObject ?? throw new InvalidOperationException(stderr));
    }

    private static void Answer(JsonObject body, string answer) => File.Copy(Path.Combine(Example, "answers", answer), body["answer"]!.GetValue<string>(), overwrite: true);

    private static void Same(string what, JsonNode? ts, JsonNode? dotnet) =>
        Assert.True(JsonNode.DeepEquals(ts, dotnet), $"{what} differs:\nTS     {ts?.ToJsonString()}\n.NET   {dotnet?.ToJsonString()}");

    private static string Text(JsonObject body, string key) => body[key]!.GetValue<string>();

    [Fact]
    public async Task CliRunsProposeReviewDeriveProveApplyAnsweringLlmStepsByHandoff()
    {
        var dir = Workspace();
        var lastro = Path.Combine(dir, ".lastro");
        var r = await DotNet(dir, "init", "--model", Path.Combine(dir, "model.json"), "--subject", $"{Assembly}#Lastro.Tests.ShippingExample.Fee", "--approvals", "2");
        Assert.True(r.Code == 0, r.Body.ToJsonString());

        r = await DotNet(dir, "propose", Path.Combine(dir, "prose.md"), "--as", "ana");
        Assert.Equal(3, r.Code);
        Assert.Equal("extract", Text(r.Body, "step"));
        Answer(r.Body, "propose.json");
        r = await DotNet(dir, "propose", Path.Combine(dir, "prose.md"), "--as", "ana");
        Assert.True(r.Code == 1, r.Body.ToJsonString());
        Assert.Equal("P-001", Text(r.Body["proposal"]!.AsObject(), "id"));
        Assert.True(File.Exists(Path.Combine(lastro, "proposals", "P-001-shipping-fee.md")));

        Assert.Equal("review-pending", Text((await DotNet(dir, "derive", "P-001")).Body, "status"));
        await DotNet(dir, "review", "P-001", "--as", "ana", "answer", "D1", "1");
        await DotNet(dir, "review", "P-001", "--as", "ana", "comment", "I-01", "Is 100 included?");
        r = await DotNet(dir, "review", "P-001", "--as", "ana", "approve-all");
        Assert.False(r.Body["proposal"]!["approved"]!.GetValue<bool>());
        Assert.Contains(r.Body["pending"]!.AsArray(), x => x!.GetValue<string>().StartsWith("T1 open thread", StringComparison.Ordinal));
        await DotNet(dir, "review", "P-001", "--as", "bruno", "reply", "T1", "Yes: 100 or more.");
        await DotNet(dir, "review", "P-001", "--as", "ana", "resolve", "T1");
        r = await DotNet(dir, "review", "P-001", "--as", "bruno", "approve-all");
        Assert.True(r.Body["proposal"]!["approved"]!.GetValue<bool>(), r.Body.ToJsonString());
        Assert.Equal("approved", Text(r.Body["proposal"]!.AsObject(), "status"));

        r = await DotNet(dir, "derive", "P-001");
        Assert.Equal(3, r.Code);
        Assert.Equal("derive", Text(r.Body, "step"));
        Answer(r.Body, "proofs.json");
        r = await DotNet(dir, "derive", "P-001");
        Assert.True(r.Code == 0, r.Body.ToJsonString());
        Assert.Equal("proving", Text(r.Body["proposal"]!.AsObject(), "status"));

        r = await DotNet(dir, "prove", "P-001", Assembly, "--export", "Lastro.Tests.ShippingExample.BuggyFee");
        Assert.True(r.Code == 1, r.Body.ToJsonString());
        Assert.Contains("counterexample", Text(r.Body, "feedback"));
        Assert.Equal(1, (await DotNet(dir, "feedback", "P-001")).Code);
        Assert.Equal(1, (await DotNet(dir, "apply", "P-001")).Code);

        // The subject declared at init.
        r = await DotNet(dir, "prove", "P-001");
        Assert.True(r.Code == 0, r.Body.ToJsonString());
        var runs = File.ReadAllLines(Path.Combine(lastro, "proposals", "P-001-shipping-fee.runs.jsonl")).Select(l => JsonNode.Parse(l)!).ToList();
        Assert.Equal([1, 2], runs.Select(x => x["run"]!.GetValue<int>()));
        Assert.Equal([false, true], runs.Select(x => x["ok"]!.GetValue<bool>()));
        Assert.Contains("Every proof passes", Text((await DotNet(dir, "feedback", "P-001")).Body, "feedback"));

        r = await DotNet(dir, "apply", "P-001");
        Assert.True(r.Code == 0, r.Body.ToJsonString());
        var useCase = LastroJson.Deserialize<UseCase>(File.ReadAllText(Path.Combine(lastro, "usecases", "UC-001-charge-shipping.json")));
        Assert.Equal(["P-001"], useCase.History);
        Assert.Empty(useCase.Proofs); // each proof has its own file in .lastro/proof/
        Assert.NotEmpty(Directory.GetFiles(Path.Combine(lastro, "proof"), "P-001.PF-*.json"));
        Assert.Contains("➕ UC-001", File.ReadAllText(Path.Combine(lastro, "proposals", "P-001-shipping-fee.receipt.md")));

        Assert.Equal(0, (await DotNet(dir, "regress")).Code);
        r = await DotNet(dir, "regress", Assembly, "--export", "Lastro.Tests.ShippingExample.BuggyFee");
        Assert.Equal(1, r.Code);
        Assert.Equal("regressions", Text(r.Body, "status"));
        r = await DotNet(dir, "status");
        Assert.Equal(0, r.Code);
        Assert.Equal(1, r.Body["useCases"]!.GetValue<int>());
    }

    [Fact]
    public async Task OldWorkspacesAndCommandsAreRefused()
    {
        var dir = Workspace();
        Assert.Equal(2, (await DotNet(dir, "plan", Path.Combine(dir, "prose.md"))).Code);
        await DotNet(dir, "init", "--model", Path.Combine(dir, "model.json"));
        File.WriteAllText(Path.Combine(dir, ".lastro", "intents.json"), "{}");
        var r = await DotNet(dir, "status");
        Assert.Equal(2, r.Code);
        Assert.Equal(["intents.json"], r.Body["legacy"]!.AsArray().Select(x => x!.GetValue<string>()));
    }

    [SkippableFact]
    public async Task OneWorkspaceSharedByTheDotNetAndTypeScriptClis()
    {
        Require(TsBuilt, TsMissing);
        var dir = Workspace();
        var lastro = Path.Combine(dir, ".lastro");

        Assert.Equal(0, Node(dir, "init", "--model", Path.Combine(dir, "model.json"), "--subject", "./shipping.mjs").Code);
        Same("init", Node(dir, "init", "--model", Path.Combine(dir, "model.json")).Body, (await DotNet(dir, "init", "--model", Path.Combine(dir, "model.json"))).Body);

        // Both CLIs ask the very same extract question (same request id, so one answer serves both).
        var n = Node(dir, "propose", Path.Combine(dir, "prose.md"), "--as", "ana");
        Assert.Equal(3, n.Code);
        Same("the propose handoff", n.Body, (await DotNet(dir, "propose", Path.Combine(dir, "prose.md"), "--as", "ana")).Body);
        Answer(n.Body, "propose.json");
        Assert.Equal(1, Node(dir, "propose", Path.Combine(dir, "prose.md"), "--as", "ana").Code);
        Same("status of a TS proposal", Node(dir, "status", "P-001").Body, (await DotNet(dir, "status", "P-001")).Body);

        // .NET reviews what TypeScript proposed; TypeScript sees the approval as valid (same hash).
        await DotNet(dir, "review", "P-001", "--as", "ana", "answer", "D1", "1");
        Assert.True((await DotNet(dir, "review", "P-001", "--as", "ana", "approve-all")).Body["proposal"]!["approved"]!.GetValue<bool>());
        var ts = Node(dir, "status", "P-001").Body;
        Same("status of a .NET-approved proposal", ts, (await DotNet(dir, "status", "P-001")).Body);
        Assert.Equal("approved", Text(ts["proposal"]!.AsObject(), "status"));

        // Both ask the same derive question; TypeScript derives, .NET proves.
        n = Node(dir, "derive", "P-001");
        Same("the derive handoff", n.Body, (await DotNet(dir, "derive", "P-001")).Body);
        Answer(n.Body, "proofs.json");
        Assert.Equal(0, Node(dir, "derive", "P-001").Code);
        var file = Path.Combine(lastro, "proposals", "P-001-shipping-fee.json");
        Assert.Equal(File.ReadAllText(file), JsJson.Stringify(LastroJson.ToNode(LastroJson.Deserialize<Proposal>(File.ReadAllText(file))), 2) + "\n");

        var receipt = Path.Combine(lastro, "proposals", "P-001-shipping-fee.receipt.md");
        static string NoEngines(string s) => Regex.Replace(s, " Engines agree: \\w+\\.", "");
        static JsonObject Comparable(JsonObject body)
        {
            var c = (JsonObject)body.DeepClone();
            c.Remove("enginesAgree");
            c.Remove("receipt");
            return c;
        }
        n = Node(dir, "prove", "P-001", Path.Combine(dir, "shipping-buggy.mjs"));
        Assert.Equal(1, n.Code);
        var tsReceipt = File.ReadAllText(receipt);
        var r = await DotNet(dir, "prove", "P-001", Assembly, "--export", "Lastro.Tests.ShippingExample.BuggyFee");
        Assert.Equal(1, r.Code);
        Same("prove (failing)", Comparable(n.Body), Comparable(r.Body));
        Assert.Equal(NoEngines(tsReceipt), NoEngines(File.ReadAllText(receipt)));
        Same("feedback", Node(dir, "feedback", "P-001").Body, (await DotNet(dir, "feedback", "P-001")).Body);

        // prove --stream: the same events (progress and crosscheck aside: they depend on the subject and the provers found).
        var events = new List<JsonObject>();
        var streamed = await Cli.Cli.RunAsync(["prove", "P-001", Assembly, "--export", "Lastro.Tests.ShippingExample.BuggyFee", "--stream", "--dir", lastro], events.Add);
        Assert.Equal(1, streamed.Code);
        events.Add(With(new JsonObject { ["event"] = "result" }, streamed.Body));
        var (code, stdout, stderr) = Run("node", [Path.Combine(RepoRoot, "sdks/typescript/dist/src/cli.js"), "prove", "P-001", Path.Combine(dir, "shipping-buggy.mjs"), "--stream", "--dir", lastro], dir);
        Assert.True(code == 1, stderr);
        static List<string> Events(IEnumerable<JsonObject> evs) => evs
            .Where(e => e["event"]!.GetValue<string>() is not ("progress" or "crosscheck"))
            .Select(e => JsJson.Stringify(Comparable(e)))
            .ToList();
        var tsEvents = Events(stdout.Split('\n', StringSplitOptions.RemoveEmptyEntries).Select(l => (JsonObject)JsonNode.Parse(l)!));
        Assert.StartsWith("{\"event\":\"start\",\"proposal\":\"P-001\"", tsEvents[0]);
        Assert.Equal(tsEvents, Events(events));

        // .NET proves the fixed code; TypeScript applies what .NET proved.
        r = await DotNet(dir, "prove", "P-001", Assembly, "--export", "Lastro.Tests.ShippingExample.Fee");
        Assert.True(r.Code == 0, r.Body.ToJsonString());
        n = Node(dir, "apply", "P-001");
        Assert.True(n.Code == 0, n.Body.ToJsonString());
        var uc = Path.Combine(lastro, "usecases", "UC-001-charge-shipping.json");
        Assert.Equal(["id", "slug", "name", "description", "history"], ((JsonObject)JsonNode.Parse(File.ReadAllText(uc))!).Select(kv => kv.Key));
        foreach (var pf in Directory.GetFiles(Path.Combine(lastro, "proof")))
            Assert.Equal(File.ReadAllText(pf), JsJson.Stringify(LastroJson.ToNode(LastroJson.Deserialize<ProofEntry>(File.ReadAllText(pf))), 2) + "\n");
        Same("status after apply", Node(dir, "status").Body, (await DotNet(dir, "status")).Body);
        Same("regress", Comparable(Node(dir, "regress", Path.Combine(dir, "shipping.mjs")).Body), Comparable((await DotNet(dir, "regress", Assembly, "--export", "Lastro.Tests.ShippingExample.Fee")).Body));

        // Proofs straight from an established use case: both CLIs ask the same derive question, the .NET CLI writes the
        // pending proof to .lastro/proof/ and proves it, and the TS CLI reads the same status.
        File.WriteAllText(Path.Combine(lastro, "usecases", "UC-002-members-ship-free.json"),
            """{"id":"UC-002","slug":"members-ship-free","name":"Members ship free","description":"A member never pays shipping.","history":[]}""");
        n = Node(dir, "derive", "--usecases");
        Assert.Equal(3, n.Code);
        Same("the derive --usecases handoff", n.Body, (await DotNet(dir, "derive", "--usecases")).Body);
        File.WriteAllText(Text(n.Body, "answer"), """
            {"proofs":[{"useCase":"UC-002","slug":"members-free","name":"Members free","description":"A member never pays shipping.","model":"shipping_fee","subject":"shipping_fee",
            "kind":"clause","requires":"{\"var\":\"in.member\"}","ensures":"{\"op\":\"eq\",\"args\":[{\"var\":\"out.fee\"},{\"const\":0}]}","assumptions":[],"inputJson":"","expect":""}],"unprovable":[]}
            """);
        r = await DotNet(dir, "derive", "--usecases");
        Assert.True(r.Code == 0, r.Body.ToJsonString());
        var st = (await DotNet(dir, "status")).Body;
        Same("status with a pending use-case proof", Node(dir, "status").Body, st);
        Assert.Equal("[\"UC-002.PF-01\"]", st["pendingProofs"]!.ToJsonString());
        r = await DotNet(dir, "prove", "--usecases", Assembly, "--export", "Lastro.Tests.ShippingExample.Fee");
        Assert.True(r.Code == 0, r.Body.ToJsonString());
        Assert.Equal("[\"UC-002.PF-01\"]", r.Body["passed"]!.ToJsonString());
        Same("status after prove --usecases", Node(dir, "status").Body, (await DotNet(dir, "status")).Body);
        r = await DotNet(dir, "derive", "--usecases", "UC-002");
        Assert.Equal(1, r.Code);
        Assert.Contains("UC-002 already has proofs", r.Body.ToJsonString());
    }

    private static JsonObject With(JsonObject head, JsonObject tail)
    {
        foreach (var (k, v) in tail) head[k] = v?.DeepClone();
        return head;
    }
}
