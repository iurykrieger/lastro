using System.Text.Json.Nodes;
using Xunit;
using static Lastro.Tests.Support;

namespace Lastro.Tests;

public class CoreTests
{
    [Fact]
    public void Sha256MatchesTheStandardVectors()
    {
        Assert.Equal("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", Canonical.Sha256Hex("abc"));
        Assert.Equal("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", Canonical.Sha256Hex(""));
    }

    [Fact]
    public void JsJsonFormatsLikeJavaScript()
    {
        Assert.Equal("200", JsJson.Number(200));
        Assert.Equal("0.001", JsJson.Number(0.001));
        Assert.Equal("0.014084507042253521", JsJson.Number(3.0 / 213));
        Assert.Equal("1e-7", JsJson.Number(1e-7));
        Assert.Equal("1e+21", JsJson.Number(1e21));
        Assert.Equal("-5", JsJson.Number(-5));
        Assert.Equal("\"a\\\"b\\n\\u0001é\"", JsJson.Quote("a\"b\n\u0001é"));
        Assert.Equal("{\"a\":1,\"b\":[true,\"x\"]}", JsJson.Canonical(JsonNode.Parse("{\"b\":[true,\"x\"],\"a\":1,\"c\":null}")));
    }

    [Fact]
    public void QuotesMustAnchorToTheProse()
    {
        Assert.Throws<ArtifactException>(() => Artifacts.Create("only the right password", [new("x", IntentKind.Behavior, Provability.Formal, "something not in the prose")]));
        var a = LoginExample.Extract();
        var i1 = a.Intents[0];
        Assert.Equal(i1.Source.Quote, a.Prose[i1.Source.Start..i1.Source.End]);
    }

    [Fact]
    public void FreezeRequiresEveryIntentReviewedAndEveryDecisionAnswered()
    {
        Assert.Throws<ArtifactException>(() => Artifacts.Freeze(LoginExample.Extract()));
        var e = Assert.Throws<ArtifactException>(() => Artifacts.Freeze(Artifacts.Review(LoginExample.Extract()).ApproveAll().Done()));
        Assert.Contains(e.Problems, p => p.StartsWith("D1 unanswered", StringComparison.Ordinal));
        var f = Reviewed();
        Assert.StartsWith("sha256:", f.Hash);
        Assert.True(Artifacts.IsIntact(f));
        var tampered = LastroJson.Clone(f);
        tampered.Intents[0].Statement = "something else";
        Assert.False(Artifacts.IsIntact(tampered));
    }

    [Fact]
    public async Task TheCorrectLoginIsProvedAndBeyondProofIntentsAreReported()
    {
        var a = Reviewed();
        var plan = LoginExample.Derive(a);
        Assert.Empty(Plans.Validate(a, plan));
        var report = await Engine.VerifyAsync(a, plan, new VerifyOptions { Subject = LoginExample.Subject() });
        Assert.True(report.Ok);
        Assert.All(report.Clauses, c => Assert.Equal((ClauseVerdict.Proved, CheckMethod.Exhaustive), (c.Verdict, c.Method)));
        Assert.Equal(2, report.Summary[IntentVerdict.BeyondProof]);
        Assert.Equal(IntentVerdict.ProvedUnderAssumptions, report.Intents.First(i => i.IntentId == "I1").Verdict);
        Assert.All(report.Scenarios, s => Assert.Equal(ScenarioStatus.Passed, s.Status));
    }

    [Fact]
    public async Task EveryPlantedBugIsRejectedWithACounterexample()
    {
        var a = Reviewed();
        var plan = LoginExample.Derive(a);
        foreach (var bug in LoginExample.Bugs)
        {
            var report = await Engine.VerifyAsync(a, plan, new VerifyOptions { Subject = LoginExample.Subject(bug) });
            Assert.False(report.Ok, bug);
            var failed = report.Clauses.Where(c => c.Verdict == ClauseVerdict.Violated).ToList();
            Assert.NotEmpty(failed);
            Assert.All(failed, c => Assert.NotNull(c.Counterexample));
        }
    }

    [Fact]
    public async Task OneWaySpecLetsRefuseEveryonePassFormallyButVacuityExposesIt()
    {
        var oneWay = Reviewed(d1: 1);
        var r1 = await Engine.VerifyAsync(oneWay, LoginExample.Derive(oneWay), new VerifyOptions { Subject = LoginExample.Subject("refuse-everyone") });
        Assert.DoesNotContain(r1.Clauses, c => c.Verdict == ClauseVerdict.Violated);
        Assert.Contains(r1.Clauses, c => c.Verdict == ClauseVerdict.Vacuous);
        Assert.False(r1.Ok);
        var bothWays = Reviewed(d1: 0);
        var r2 = await Engine.VerifyAsync(bothWays, LoginExample.Derive(bothWays), new VerifyOptions { Subject = LoginExample.Subject("refuse-everyone") });
        Assert.Contains(r2.Clauses, c => c.Verdict == ClauseVerdict.Violated);
    }

    [Fact]
    public async Task AnIntentTheAiMisreadAndNobodyFixedShowsUpAsViolated()
    {
        var a = Reviewed(fixI5: false);
        var report = await Engine.VerifyAsync(a, LoginExample.Derive(a), new VerifyOptions { Subject = LoginExample.Subject() });
        Assert.Equal(IntentVerdict.Violated, report.Intents.First(i => i.IntentId == "I5").Verdict);
    }

    [Fact]
    public void RejectedIntentsCannotBeBuiltAndThePlanIsBoundToTheArtifactHash()
    {
        var a = Artifacts.Freeze(Artifacts.Review(LoginExample.Extract()).ApproveAll().Reject("I5", "out of scope").Answer("D1", 0).Answer("D2", 0).Done());
        var plan = LoginExample.Derive(Reviewed());
        Assert.Throws<PlanException>(() => Plans.Validate(a, plan));
        Assert.Empty(Plans.Validate(a, LoginExample.Derive(a)));
    }

    private static (IntentArtifact A, ProofPlan Plan) TinyPlan(List<Clause> clauses, Dictionary<string, Domain>? inputs = null)
    {
        var a = Artifacts.Freeze(Artifacts.Review(Artifacts.Create("x doubles", [new("y = 2x", IntentKind.Behavior, Provability.Formal, "x doubles")])).ApproveAll().Done());
        return (a, new ProofPlan
        {
            ArtifactHash = a.Hash!,
            Model = new Model { Name = "double", Inputs = inputs ?? new() { ["x"] = D.Int(0, 9) }, Outputs = new() { ["y"] = D.Int(-1000, 10_000_000) } },
            Clauses = clauses,
        });
    }

    private static Env Double(Env i) => new() { ["y"] = 2 * i["x"].AsNumber };

    [Fact]
    public async Task WeakAndVacuousClausesAreFlagged()
    {
        var (a, plan) = TinyPlan([
            new() { Id = "C1", IntentIds = ["I1"], Description = "tautology", Ensures = L.Or(L.Ge(L.Out("y"), 0), L.Lt(L.Out("y"), 0)) },
            new() { Id = "C2", IntentIds = ["I1"], Description = "never applies", Requires = L.Gt(L.In("x"), 100), Ensures = L.Eq(L.Out("y"), 0) },
            new() { Id = "C3", IntentIds = ["I1"], Description = "y = 2x", Ensures = L.Eq(L.Out("y"), L.Mul(L.C(2), L.In("x"))) },
        ]);
        var r = await Engine.VerifyAsync(a, plan, new VerifyOptions { Subject = Subjects.From(Double) });
        Assert.Equal(ClauseVerdict.Weak, r.Clauses[0].Verdict);
        Assert.Equal(ClauseVerdict.Vacuous, r.Clauses[1].Verdict);
        Assert.Equal(ClauseVerdict.Proved, r.Clauses[2].Verdict);
    }

    [Fact]
    public async Task LargeDomainsFallBackToSeededSamplingWithAQuantifiedBound()
    {
        var (a, plan) = TinyPlan([new() { Id = "C1", IntentIds = ["I1"], Description = "y = 2x", Ensures = L.Eq(L.Out("y"), L.Mul(L.C(2), L.In("x"))) }], new() { ["x"] = D.Int(0, 1_000_000) });
        var r = await Engine.VerifyAsync(a, plan, new VerifyOptions { Subject = Subjects.From(Double), Samples = 3000 });
        Assert.Equal(ClauseVerdict.Evidenced, r.Clauses[0].Verdict);
        Assert.Equal(0.001, r.Clauses[0].FailureRateUpperBound95);
        var again = await Engine.VerifyAsync(a, plan, new VerifyOptions { Subject = Subjects.From(Double), Samples = 3000 });
        Assert.Equal(Canonical.HashOf(r.Clauses), Canonical.HashOf(again.Clauses));
    }

    [Fact]
    public async Task AProductionTraceCanContradictAModelProof()
    {
        var a = Reviewed();
        var plan = LoginExample.Derive(a);
        var report = await Engine.VerifyAsync(a, plan, new VerifyOptions
        {
            Subject = LoginExample.Subject(),
            Traces = [new Trace { Ref = "req-123", Input = new() { ["sends"] = "email", ["accountFound"] = true, ["password"] = "wrong", ["infra"] = "ok" }, Output = new() { ["status"] = 200, ["token"] = true, ["role"] = "customer" } }],
        });
        var c = report.Clauses.First(x => x.TraceViolation is not null);
        Assert.Contains("trusted piece", c.Note);
    }

    [Fact]
    public void ContradictoryClausesComeBackAsADecisionCard()
    {
        var (_, plan) = TinyPlan([
            new() { Id = "C1", IntentIds = ["I1"], Description = "y is 1 when x=3", Requires = L.Eq(L.In("x"), 3), Ensures = L.Eq(L.Out("y"), 1) },
            new() { Id = "C2", IntentIds = ["I1"], Description = "y is 2 when x=3", Requires = L.Eq(L.In("x"), 3), Ensures = L.Eq(L.Out("y"), 2) },
        ]);
        plan.Model.Outputs = new() { ["y"] = D.Int(0, 5) };
        var cards = Engine.CheckConsistency(plan);
        Assert.Single(cards);
        Assert.Equal(["Relax C1", "Relax C2"], cards[0].Options.Select(o => o.Label));
        Assert.Equal("Clauses C1,C2 cannot all hold for input {\"x\":3}. Which one should give way?", cards[0].Question);
    }

    [Fact]
    public void ExplainRunClassifiesEachClauseForASingleRun()
    {
        var plan = LoginExample.Derive(Reviewed());
        var input = new Env { ["sends"] = "email", ["accountFound"] = true, ["password"] = "right", ["infra"] = "ok" };
        var ex = Engine.ExplainRun(plan, input, LoginExample.Decide(input));
        Assert.DoesNotContain(ex, e => e.Status == ClauseStatus.Fails);
        Assert.Contains(ex, e => e.Status == ClauseStatus.Holds);
        Assert.Contains(ex, e => e.Status == ClauseStatus.IfPartFalse);
    }

    [Fact]
    public void LslValidationRejectsUnknownVariablesAndSmtLibExportIsWellFormed()
    {
        Assert.NotEmpty(Lsl.Validate(L.Eq(L.In("nope"), 1), LoginExample.Model));
        Assert.Empty(Lsl.Validate(L.Eq(L.In("sends"), "email"), LoginExample.Model));
        Assert.NotEmpty(Lsl.Validate(JsonNode.Parse("{\"op\":\"pow\",\"args\":[]}"), LoginExample.Model));
        var plan = LoginExample.Derive(Reviewed());
        var smt = Lsl.ToSmtLib(plan.Model, plan.Clauses[0]);
        Assert.Contains("(assert (not ", smt);
        Assert.Contains("(check-sat)", smt);
        Assert.Equal(smt.Count(c => c == '('), smt.Count(c => c == ')'));
        Assert.Equal("in.sends = \"neither\" ⇒ (out.status = 400) ∧ ¬out.token ∧ (out.role = \"none\")", Lsl.Formal(plan.Clauses[0].Requires, plan.Clauses[0].Ensures, " ⇒ "));
    }

    [Fact]
    public void LslRoundTripsThroughJson()
    {
        var e = L.And(L.OneOf(L.In("sends"), "email", "both"), L.Call("named"), L.Ge(L.Out("n"), -3.5));
        var json = LastroJson.Serialize(e);
        Assert.Equal("{\"op\":\"and\",\"args\":[{\"op\":\"in\",\"args\":[{\"var\":\"in.sends\"},{\"const\":\"email\"},{\"const\":\"both\"}]},{\"call\":\"named\",\"args\":[]},{\"op\":\"ge\",\"args\":[{\"var\":\"out.n\"},{\"const\":-3.5}]}]}", json);
        Assert.Equal(json, LastroJson.Serialize(Lsl.Parse(json)));
    }

    [Fact]
    public void CarryReviewKeepsVerdictsForUnchangedIntentsAndReappliesAmendments()
    {
        var before = ReviewedLogin();
        var fresh = LoginExample.Extract();
        var next = Artifacts.CarryReview(before, fresh);
        Assert.All(next.Intents, i => Assert.NotEqual(IntentStatus.Proposed, i.Status));
        var i5 = next.Intents.First(i => i.Id == "I5");
        Assert.Equal(IntentStatus.Amended, i5.Status);
        Assert.Equal(LoginExample.FixedI5, i5.Statement);
        Assert.Equal([0, 0], next.Decisions.Select(d => d.Chosen ?? -1));

        fresh.Intents[6].Statement = "A database failure gets 503 and no token.";
        var changed = Artifacts.CarryReview(before, fresh);
        Assert.Equal(["I7"], changed.Intents.Where(i => i.Status == IntentStatus.Proposed).Select(i => i.Id));
    }

    private static JsonObject DeriveAnswer(ProofPlan plan) => new()
    {
        ["clauses"] = new JsonArray(plan.Clauses.Select(c => (JsonNode)new JsonObject
        {
            ["id"] = c.Id,
            ["intentIds"] = LastroJson.ToNode(c.IntentIds),
            ["description"] = c.Description,
            ["requires"] = c.Requires is null ? "" : LastroJson.Serialize(c.Requires),
            ["ensures"] = LastroJson.Serialize(c.Ensures),
            ["assumes"] = LastroJson.ToNode(c.Assumes ?? []),
        }).ToArray()),
        ["assumptions"] = LastroJson.ToNode(plan.Assumptions),
        ["scenarios"] = new JsonArray(plan.Scenarios.Select(s => (JsonNode)new JsonObject
        {
            ["id"] = s.Id,
            ["intentIds"] = LastroJson.ToNode(s.IntentIds),
            ["given"] = s.Given,
            ["when"] = s.When,
            ["then"] = s.Then,
            ["inputJson"] = LastroJson.Serialize(s.Input),
            ["expect"] = LastroJson.Serialize(s.Expect),
        }).ToArray()),
        ["beyondProof"] = LastroJson.ToNode(plan.BeyondProof),
    };

    internal static JsonObject ExtractAnswer(IntentArtifact ex) => new()
    {
        ["intents"] = new JsonArray(ex.Intents.Select(i => (JsonNode)new JsonObject
        {
            ["statement"] = i.Statement,
            ["kind"] = Wire.Of(i.Kind),
            ["provability"] = Wire.Of(i.Provability),
            ["beyondProofReason"] = i.BeyondProofReason is { } r ? Wire.Of(r) : "none",
            ["quote"] = i.Source.Quote,
        }).ToArray()),
        ["decisions"] = new JsonArray(ex.Decisions.Select(d => (JsonNode)new JsonObject
        {
            ["intentIndexes"] = new JsonArray(d.IntentIds.Select(x => (JsonNode)JsonValue.Create(int.Parse(x[1..]) - 1)).ToArray()),
            ["question"] = d.Question,
            ["options"] = LastroJson.ToNode(d.Options),
            ["recommended"] = d.Recommended,
        }).ToArray()),
        ["notPromised"] = LastroJson.ToNode(ex.NotPromised),
    };

    internal static JsonObject DeriveAnswerFor(IntentArtifact frozen) => DeriveAnswer(LoginExample.Derive(frozen));

    [Fact]
    public async Task LlmStepsTurnJsonAnswersIntoCheckedLastroObjects()
    {
        var frozen = Artifacts.Freeze(ReviewedLogin());
        var answers = new Dictionary<string, JsonNode?>
        {
            ["extract"] = JsonNode.Parse("""{"intents":[{"statement":"Only the right password gets a token.","kind":"invariant","provability":"formal","beyondProofReason":"none","quote":"only the right password gets a token"}],"decisions":[],"notPromised":[]}"""),
            ["derive"] = DeriveAnswerFor(frozen),
        };
        AskJson ask = (req, _) => Task.FromResult(answers[req.Step]?.DeepClone());

        var extracted = await new LlmIntentExtractor(ask).ExtractAsync(frozen.Prose);
        Assert.Equal("only the right password gets a token", extracted.Intents[0].Source.Quote);

        var plan = await new LlmProofDeriver(ask).DeriveAsync(frozen, LoginExample.Model);
        Plans.Validate(frozen, plan);
        Assert.Equal(Canonical.HashOf(LoginExample.Derive(frozen)), Canonical.HashOf(plan));
        var report = await Engine.VerifyAsync(frozen, plan, new VerifyOptions { Subject = LoginExample.Subject() });
        Assert.True(report.Ok);

        // A derivation that references a variable outside the model is refused.
        answers["derive"]!["clauses"]![0]!["ensures"] = "{\"var\":\"out.cookie\"}";
        var e = await Assert.ThrowsAsync<PlanException>(() => new LlmProofDeriver(ask).DeriveAsync(frozen, LoginExample.Model));
        Assert.Contains("cookie", e.Message);
    }

    private static LastroClient LoginPipeline(int requiredApprovals = 1) => new(new LastroOptions
    {
        Model = LoginExample.Model,
        Extractor = new StaticExtractor(LoginExample.Extract),
        Deriver = new StaticDeriver((a, _) => LoginExample.Derive(a)),
        RequiredApprovals = requiredApprovals,
    });

    [Fact]
    public async Task PipelineNeedsOneApprovalByDefault()
    {
        var lastro = LoginPipeline();
        var a = lastro.Review(await lastro.PlanAsync(LoginExample.Prose), "ana").ApproveAll().Amend("I5", LoginExample.FixedI5).Answer("D1", 0).Answer("D2", 0).Done();
        Assert.Empty(lastro.Pending(a));
    }

    [Fact]
    public async Task APolicyOfTwoApprovalsNeedsTwoDistinctReviewers()
    {
        var lastro = LoginPipeline(2);
        var a = await lastro.PlanAsync(LoginExample.Prose);
        Assert.Equal(2, a.Policy!.RequiredApprovals);
        a = lastro.Review(a, "ana").ApproveAll().Amend("I5", LoginExample.FixedI5).Answer("D1", 0).Answer("D2", 0).Done();
        Assert.Contains(lastro.Pending(a), p => p.Contains("I1 needs 1 more approval"));
        await Assert.ThrowsAsync<ArtifactException>(() => lastro.SpecAsync(a));
        // Ana's amendment reset I5: Bruno must approve the new wording too.
        a = lastro.Review(a, "ana").ApproveAll().Done();
        Assert.Contains(lastro.Pending(a), p => p.Contains("I5 needs 1 more"));
        a = lastro.Review(a, "bruno").ApproveAll().Done();
        Assert.Empty(lastro.Pending(a));
        var spec = await lastro.SpecAsync(a);
        Assert.True(spec.Ok);
        var proof = await lastro.ProveAsync(spec, LoginExample.Subject());
        Assert.True(proof.Ok);
        Assert.Empty(proof.Failures);
        Assert.Contains("login_flow", proof.Receipt);
    }

    [Fact]
    public void ContradictionsGoBackToTheTeamAsUnansweredDecisions()
    {
        var a = Artifacts.Freeze(ReviewedLogin());
        var raised = Artifacts.RaiseDecisions(a, [new DecisionCard
        {
            Id = "X1", IntentIds = ["I5", "I7"], Question = "C5 and C7 cannot both hold. Which gives way?",
            Options = [new() { Label = "Relax C5" }, new() { Label = "Relax C7" }], Recommended = 0, Origin = DecisionOrigin.ConsistencyCheck,
        }]);
        Assert.Null(raised.Hash);
        Assert.Equal("D3", raised.Decisions[^1].Id);
        var e = Assert.Throws<ArtifactException>(() => Artifacts.Freeze(raised));
        Assert.Contains(e.Problems, p => p.StartsWith("D3 unanswered", StringComparison.Ordinal));
    }

    [Fact]
    public void AnsweringAContradictionAddsAnIntentTheTeamMustApprove()
    {
        var raised = Artifacts.RaiseDecisions(Artifacts.Freeze(ReviewedLogin()), [new DecisionCard
        {
            Id = "X1", IntentIds = ["I5", "I7"], Question = "C5 and C7 cannot both hold for both+db-fails. Which gives way?",
            Options = [new() { Label = "Relax C5", Consequence = "the database error wins" }, new() { Label = "Relax C7", Consequence = "the 400 wins" }],
            Recommended = 0, Origin = DecisionOrigin.ConsistencyCheck,
        }]);
        var answered = Artifacts.Review(raised, "ana").Answer("D3", 0).Done();
        var it = answered.Intents[^1];
        Assert.Equal("D3", it.DecisionId);
        Assert.Equal(IntentStatus.Proposed, it.Status);
        Assert.Contains("Relax C5 (the database error wins)", it.Statement);
        var e = Assert.Throws<ArtifactException>(() => Artifacts.Freeze(answered));
        Assert.Contains(e.Problems, p => p.StartsWith("I10 not reviewed", StringComparison.Ordinal));
        Assert.NotNull(Artifacts.Freeze(Artifacts.Review(answered, "bruno").Approve(it.Id).Done()).Hash);
    }

    [Fact]
    public async Task FailuresAreStructuredFeedbackForWhateverWritesTheCode()
    {
        var lastro = LoginPipeline();
        var spec = await lastro.SpecAsync(lastro.Review(ReviewedLogin(), "ana").ApproveAll().Done());
        var proof = await lastro.ProveAsync(spec, LoginExample.Subject("skip-password"));
        Assert.False(proof.Ok);
        var f = proof.Failures.First(x => x.Kind == FailureKind.Clause);
        Assert.Equal("wrong", f.Input["password"].AsString);
        Assert.Matches("counterexample: .*\"password\":\"wrong\"", Feedback.AgentFeedback(proof));
        Assert.Equal("Every proof passes.", Feedback.AgentFeedback(await lastro.ProveAsync(spec, LoginExample.Subject())));
    }

    [Fact]
    public async Task ArtifactsRoundTripThroughTheWireFormat()
    {
        var lastro = LoginPipeline();
        var spec = await lastro.SpecAsync(lastro.Review(ReviewedLogin(), "ana").ApproveAll().Done());
        var proof = await lastro.ProveAsync(spec, LoginExample.Subject("db-as-401"));
        var json = LastroJson.Serialize(proof);
        Assert.Contains("\"verdict\":\"proved-under-assumptions\"", json);
        Assert.Contains("\"proved-under-assumptions\":", json);
        var back = LastroJson.Deserialize<Proof>(json);
        Assert.Equal(json, LastroJson.Serialize(back));
        var specBack = LastroJson.Deserialize<Spec>(LastroJson.Serialize(spec));
        Assert.True(Artifacts.IsIntact(specBack.Artifact));
        Assert.Equal(Canonical.HashOf(spec.Plan), Canonical.HashOf(specBack.Plan));
    }
}
