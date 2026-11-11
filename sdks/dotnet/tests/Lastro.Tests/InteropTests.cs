using System.Text.Json.Nodes;
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

    private static string Workspace()
    {
        var dir = Path.Combine(Path.GetTempPath(), "lastro-dotnet-" + Guid.NewGuid().ToString("N")[..8]);
        Directory.CreateDirectory(dir);
        File.WriteAllText(Path.Combine(dir, "model.json"), LastroJson.Serialize(LoginExample.Model));
        File.WriteAllText(Path.Combine(dir, "prose.md"), LoginExample.Prose);
        return dir;
    }

    private static async Task<(int Code, JsonObject Body)> DotNet(string dir, params string[] args) =>
        await Cli.Cli.RunAsync([.. args, "--dir", Path.Combine(dir, ".lastro"), "--llm", "handoff"]);

    private static (int Code, JsonObject Body) Node(string dir, params string[] args)
    {
        var (code, stdout, stderr) = Run("node", [Path.Combine(RepoRoot, "sdks/typescript/dist/src/cli.js"), .. args, "--dir", Path.Combine(dir, ".lastro"), "--llm", "handoff"]);
        return (code, JsonNode.Parse(stdout) as JsonObject ?? throw new InvalidOperationException(stderr));
    }

    private static void Answer(JsonObject body, JsonNode answer) => File.WriteAllText(body["answer"]!.GetValue<string>(), answer.ToJsonString());

    [Fact]
    public async Task CliRunsPlanReviewSpecProveAnsweringLlmStepsByHandoff()
    {
        var dir = Workspace();
        Assert.Equal(0, (await DotNet(dir, "init", "--model", Path.Combine(dir, "model.json"))).Code);

        var r = await DotNet(dir, "plan", Path.Combine(dir, "prose.md"));
        Assert.Equal(3, r.Code);
        Answer(r.Body, CoreTests.ExtractAnswer(LoginExample.Extract()));
        r = await DotNet(dir, "plan", Path.Combine(dir, "prose.md"));
        Assert.Equal(1, r.Code);

        Assert.Equal("review-pending", (await DotNet(dir, "spec")).Body["status"]!.GetValue<string>());
        await DotNet(dir, "review", "--as", "ana", "approve-all");
        await DotNet(dir, "review", "--as", "ana", "amend", "I5", LoginExample.FixedI5);
        await DotNet(dir, "review", "--as", "ana", "answer", "D1", "0");
        await DotNet(dir, "review", "--as", "ana", "answer", "D2", "0");
        Assert.Equal(0, (await DotNet(dir, "status")).Code);

        r = await DotNet(dir, "spec");
        Assert.Equal(3, r.Code);
        var approved = Artifacts.Approve(LastroJson.Deserialize<IntentArtifact>(File.ReadAllText(Path.Combine(dir, ".lastro", "intents.json"))));
        Answer(r.Body, CoreTests.DeriveAnswerFor(approved));
        r = await DotNet(dir, "spec");
        Assert.True(r.Code == 0, r.Body.ToJsonString());

        var asm = typeof(LoginExample).Assembly.Location;
        r = await DotNet(dir, "prove", asm, "--export", "Lastro.Tests.LoginExample.Reference");
        Assert.True(r.Code == 0, r.Body.ToJsonString());
        r = await DotNet(dir, "prove", asm, "--export", "Lastro.Tests.LoginExample.SkipPassword");
        Assert.Equal(1, r.Code);
        Assert.Contains("counterexample", r.Body["feedback"]!.GetValue<string>());
        Assert.Equal(1, (await DotNet(dir, "feedback")).Code);
        Assert.True(File.Exists(Path.Combine(dir, ".lastro", "receipt.md")));
    }

    [SkippableFact]
    public async Task OneWorkspaceSharedByTheDotNetAndTypeScriptClis()
    {
        Require(TsBuilt, TsMissing);
        var dir = Workspace();

        // .NET starts the workspace and asks for the extraction.
        Assert.Equal(0, (await DotNet(dir, "init", "--model", Path.Combine(dir, "model.json"), "--approvals", "2")).Code);
        var r = await DotNet(dir, "plan", Path.Combine(dir, "prose.md"));
        Assert.Equal(3, r.Code);
        Answer(r.Body, CoreTests.ExtractAnswer(LoginExample.Extract()));

        // TypeScript picks up the same handoff answer (same request id) and plans.
        var n = Node(dir, "plan", Path.Combine(dir, "prose.md"));
        Assert.True(n.Code == 1, n.Body.ToJsonString());
        Assert.Equal(9, n.Body["intents"]!.AsArray().Count);

        // Two reviewers, one per SDK, under the policy of two approvals.
        await DotNet(dir, "review", "--as", "ana", "approve-all");
        await DotNet(dir, "review", "--as", "ana", "amend", "I5", LoginExample.FixedI5);
        await DotNet(dir, "review", "--as", "ana", "answer", "D1", "0");
        await DotNet(dir, "review", "--as", "ana", "answer", "D2", "0");
        Assert.Equal(1, Node(dir, "status").Code);
        Assert.Equal(0, Node(dir, "review", "--as", "bruno", "approve-all").Code);
        Assert.Equal(0, (await DotNet(dir, "status")).Code);

        // TypeScript derives the spec (handoff answered by the .NET test), .NET proves against it.
        n = Node(dir, "spec");
        Assert.Equal(3, n.Code);
        var approved = Artifacts.Approve(LastroJson.Deserialize<IntentArtifact>(File.ReadAllText(Path.Combine(dir, ".lastro", "intents.json"))));
        Assert.Equal(new LlmRequest("derive", LlmPrompts.DeriveSystem, LlmPrompts.DeriveUserPrompt(approved, LoginExample.Model), LlmPrompts.DeriveSchema).Id,
            Path.GetFileName(n.Body["answer"]!.GetValue<string>()).Replace(".answer.json", ""));
        Answer(n.Body, CoreTests.DeriveAnswerFor(approved));
        n = Node(dir, "spec");
        Assert.True(n.Code == 0, n.Body.ToJsonString());
        Assert.Equal(approved.Hash, n.Body["artifactHash"]!.GetValue<string>());

        r = await DotNet(dir, "prove", typeof(LoginExample).Assembly.Location, "--export", "Lastro.Tests.LoginExample.SkipPassword");
        Assert.Equal(1, r.Code);
        var feedback = Node(dir, "feedback");
        Assert.Equal(1, feedback.Code);
        Assert.Equal(r.Body["feedback"]!.GetValue<string>(), feedback.Body["feedback"]!.GetValue<string>());

        // prove --stream: both CLIs print the same events for the same bug (progress counts and the Rust check aside).
        var events = new List<JsonObject>();
        var streamed = await Cli.Cli.RunAsync(["prove", "--stream", typeof(LoginExample).Assembly.Location, "--export", "Lastro.Tests.LoginExample.SkipPassword",
            "--dir", Path.Combine(dir, ".lastro")], events.Add);
        Assert.Equal(1, streamed.Code);
        var dotnetResult = new JsonObject { ["event"] = "result" };
        foreach (var (k, v) in streamed.Body) dotnetResult[k] = v?.DeepClone();
        events.Add(dotnetResult);
        var login = Path.Combine(RepoRoot, "sdks/typescript/dist/examples/login/login.js");
        File.WriteAllText(Path.Combine(dir, "impl.mjs"), $"import {{ makeLogin }} from {JsonValue.Create(login).ToJsonString()};\nexport const buggy = makeLogin(\"skip-password\");\n");
        var (code, stdout, stderr) = Run("node", [Path.Combine(RepoRoot, "sdks/typescript/dist/src/cli.js"), "prove", Path.Combine(dir, "impl.mjs"), "--export", "buggy", "--stream", "--dir", Path.Combine(dir, ".lastro")]);
        Assert.True(code == 1, stderr);
        static List<string> Comparable(IEnumerable<JsonObject> evs) => evs
            .Where(e => e["event"]!.GetValue<string>() is not ("progress" or "crosscheck"))
            .Select(e =>
            {
                var c = (JsonObject)e.DeepClone();
                if (c["event"]!.GetValue<string>() == "result") c.Remove("enginesAgree");
                return JsJson.Stringify(c);
            })
            .ToList();
        var ts = Comparable(stdout.Split('\n', StringSplitOptions.RemoveEmptyEntries).Select(l => (JsonObject)JsonNode.Parse(l)!));
        Assert.StartsWith("{\"event\":\"start\",\"model\":\"login_flow\"", ts[0]);
        Assert.Equal(ts, Comparable(events));
    }
}
