using System.Text.Json;
using System.Text.Json.Nodes;
using System.Text.Json.Serialization;

namespace Lastro;

/// <summary>One request to an LLM for a single JSON value matching <see cref="Schema"/>.</summary>
public sealed record LlmRequest(string Step, string System, string User, JsonObject Schema)
{
    /// <summary>The request as JSON, in the key order the TypeScript SDK uses (step, system, user, schema).</summary>
    public JsonObject ToJson() => new()
    {
        ["step"] = Step,
        ["system"] = System,
        ["user"] = User,
        ["schema"] = Schema.DeepClone(),
    };

    /// <summary>A stable id for this request: the same in every Lastro SDK (used by the CLI's handoff files).</summary>
    public string Id => $"{Step}-{Canonical.Sha256Hex(JsJson.Stringify(ToJson()))[..12]}";
}

/// <summary>
/// Any LLM transport: the Anthropic API (see Lastro.Claude), the calling agent's own model (CLI handoff),
/// a test double. Nothing it returns is trusted as is: quotes are anchored and plans are validated as LSL.
/// </summary>
public delegate Task<JsonNode?> AskJson(LlmRequest request, CancellationToken cancellationToken = default);

/// <summary>prose → proposed intents (non-deterministic; a human reviews them).</summary>
public interface IIntentExtractor
{
    Task<IntentArtifact> ExtractAsync(string prose, string? context = null, CancellationToken cancellationToken = default);
}

/// <summary>approved intents + model → proof plan (non-deterministic; checked by <see cref="Plans.Validate"/>).</summary>
public interface IProofDeriver
{
    Task<ProofPlan> DeriveAsync(IntentArtifact artifact, Model model, CancellationToken cancellationToken = default);
}

/// <summary>Deterministic stand-ins for tests, examples and offline use.</summary>
public sealed class StaticExtractor(Func<string, IntentArtifact> build) : IIntentExtractor
{
    public Task<IntentArtifact> ExtractAsync(string prose, string? context = null, CancellationToken cancellationToken = default) => Task.FromResult(build(prose));
}

public sealed class StaticDeriver(Func<IntentArtifact, Model, ProofPlan> build) : IProofDeriver
{
    public Task<ProofPlan> DeriveAsync(IntentArtifact artifact, Model model, CancellationToken cancellationToken = default) => Task.FromResult(build(artifact, model));
}

/// <summary>A gap the proofs cannot see, found by the LLM gap review. Evidence for humans, never proof.</summary>
public sealed class Gap
{
    public string IntentId { get; set; } = "";
    [JsonPropertyName("gap")]
    public string Description { get; set; } = "";
    public string Suggestion { get; set; } = "";
    public string Severity { get; set; } = "low";
}

/// <summary>Prompts, schemas and the deterministic conversions shared by every transport.</summary>
public static partial class LlmPrompts
{
    public static JsonObject ExtractSchema => (JsonObject)JsonNode.Parse(ExtractSchemaJson)!;
    public static JsonObject DeriveSchema => (JsonObject)JsonNode.Parse(DeriveSchemaJson)!;
    public static JsonObject GapReviewSchema => (JsonObject)JsonNode.Parse(GapReviewSchemaJson)!;

    public static string ExtractUserPrompt(string prose, string? context = null) =>
        $"{(string.IsNullOrEmpty(context) ? "" : $"<context>\n{context}\n</context>\n\n")}<prose>\n{prose}\n</prose>\n\nIntents are numbered from 0 in the order you list them; decisions reference them by index.";

    /// <summary>Turns an extraction answer into an artifact. Throws <see cref="ArtifactException"/> on any problem.</summary>
    public static IntentArtifact ArtifactFromExtraction(string prose, JsonNode? answer)
    {
        var o = answer as JsonObject ?? throw new ArtifactException(["extraction answer is not a JSON object"]);
        var problems = new List<string>();
        var drafts = new List<IntentDraft>();
        foreach (var (n, i) in (o["intents"] as JsonArray ?? []).Select((n, i) => (n, i)))
        {
            var where = $"intents[{i}]";
            var kind = Enum<IntentKind>(n?["kind"], where, problems);
            var prov = Enum<Provability>(n?["provability"], where, problems);
            var reasonText = Str(n?["beyondProofReason"]);
            BeyondProofReason? reason = null;
            if (!string.IsNullOrEmpty(reasonText) && reasonText != "none")
            {
                if (KebabEnumConverter<BeyondProofReason>.TryParse(reasonText, out var r)) reason = r;
                else problems.Add($"{where}: unknown beyondProofReason '{reasonText}'");
            }
            drafts.Add(new IntentDraft(Str(n?["statement"]), kind, prov, Str(n?["quote"]), reason));
        }
        var decisions = new List<DecisionDraft>();
        foreach (var d in o["decisions"] as JsonArray ?? [])
        {
            var idx = (d?["intentIndexes"] as JsonArray ?? []).Select(x => $"I{(int)(double.Parse(x!.ToJsonString(), System.Globalization.CultureInfo.InvariantCulture)) + 1}").ToList();
            var opts = (d?["options"] as JsonArray ?? []).Select(x => new DecisionOption { Label = Str(x?["label"]), Consequence = Str(x?["consequence"]) }).ToList();
            decisions.Add(new DecisionDraft(idx, Str(d?["question"]), opts, (int)(d?["recommended"]?.GetValue<double>() ?? 0)));
        }
        if (problems.Count > 0) throw new ArtifactException(problems);
        var notPromised = (o["notPromised"] as JsonArray ?? []).Select(x => Str(x)).ToList();
        return Artifacts.Create(prose, drafts, decisions, notPromised);
    }

    public static string DeriveUserPrompt(IntentArtifact artifact, Model model)
    {
        if (artifact.Hash is null) throw new InvalidOperationException("approve() the artifact before deriving proofs");
        var intents = new JsonArray();
        foreach (var i in Artifacts.ApprovedIntents(artifact))
        {
            var it = new JsonObject
            {
                ["id"] = i.Id,
                ["statement"] = i.Statement,
                ["kind"] = Wire.Of(i.Kind),
                ["provability"] = Wire.Of(i.Provability),
            };
            if (i.BeyondProofReason is { } r) it["beyondProofReason"] = Wire.Of(r);
            intents.Add(it);
        }
        var decisions = new JsonArray();
        foreach (var d in artifact.Decisions)
        {
            var it = new JsonObject { ["question"] = d.Question };
            var pick = d.Chosen ?? d.Recommended;
            if (pick >= 0 && pick < d.Options.Count) it["answer"] = d.Options[pick].Label;
            decisions.Add(it);
        }
        var payload = new JsonObject
        {
            ["model"] = LastroJson.ToNode(model),
            ["approvedIntents"] = intents,
            ["decisions"] = decisions,
            ["notPromised"] = LastroJson.ToNode(artifact.NotPromised),
        };
        return JsJson.Stringify(payload, indent: 2);
    }

    /// <summary>Parses and validates the deriver's answer. Throws with every problem found.</summary>
    public static ProofPlan PlanFromDerivation(IntentArtifact artifact, Model model, JsonNode? answer)
    {
        var o = answer as JsonObject ?? throw new PlanException(["derivation answer is not a JSON object"]);
        Expr ParseExpr(string where, JsonNode? s)
        {
            JsonNode? e;
            if (s is JsonValue v && v.GetValueKind() == JsonValueKind.String)
            {
                try
                {
                    e = JsonNode.Parse(v.GetValue<string>());
                }
                catch (JsonException)
                {
                    throw new PlanException([$"{where}: not valid JSON: {v.GetValue<string>()}"]);
                }
            }
            else e = s;
            var errs = Lsl.Validate(e, model, null, where);
            if (errs.Count > 0) throw new PlanException(errs);
            return LastroJson.FromNode<Expr>(e);
        }
        var clauses = new List<Clause>();
        foreach (var c in o["clauses"] as JsonArray ?? [])
        {
            var id = Str(c?["id"]);
            var req = c?["requires"];
            var hasRequires = req is not null && !(req is JsonValue rv && rv.GetValueKind() == JsonValueKind.String && string.IsNullOrWhiteSpace(rv.GetValue<string>()));
            var assumes = (c?["assumes"] as JsonArray ?? []).Select(x => Str(x)).ToList();
            clauses.Add(new Clause
            {
                Id = id,
                IntentIds = Strings(c?["intentIds"]),
                Description = Str(c?["description"]),
                Requires = hasRequires ? ParseExpr($"{id}.requires", req) : null,
                Ensures = ParseExpr($"{id}.ensures", c?["ensures"]),
                Assumes = assumes.Count > 0 ? assumes : null,
            });
        }
        var scenarios = new List<Scenario>();
        foreach (var s in o["scenarios"] as JsonArray ?? [])
        {
            var id = Str(s?["id"]);
            var inputNode = s?["inputJson"] is JsonValue iv && iv.GetValueKind() == JsonValueKind.String ? JsonNode.Parse(iv.GetValue<string>()) : s?["inputJson"];
            scenarios.Add(new Scenario
            {
                Id = id,
                IntentIds = Strings(s?["intentIds"]),
                Given = Str(s?["given"]),
                When = Str(s?["when"]),
                Then = Str(s?["then"]),
                Input = LastroJson.FromNode<Env>(inputNode),
                Expect = ParseExpr($"{id}.expect", s?["expect"]),
            });
        }
        return new ProofPlan
        {
            ArtifactHash = artifact.Hash!,
            Model = model,
            Clauses = clauses,
            Assumptions = o["assumptions"] is JsonArray a ? LastroJson.FromNode<List<Assumption>>(a) : [],
            Scenarios = scenarios,
            BeyondProof = o["beyondProof"] is JsonArray b ? LastroJson.FromNode<List<BeyondProofItem>>(b) : [],
        };
    }

    public static string GapReviewUserPrompt(IntentArtifact artifact, ProofPlan plan, string source)
    {
        var payload = new JsonObject
        {
            ["approvedIntents"] = new JsonArray(Artifacts.ApprovedIntents(artifact).Select(i => (JsonNode)new JsonObject { ["id"] = i.Id, ["statement"] = i.Statement }).ToArray()),
            ["clauses"] = new JsonArray(plan.Clauses.Select(c => (JsonNode)new JsonObject
            {
                ["id"] = c.Id,
                ["intentIds"] = LastroJson.ToNode(c.IntentIds),
                ["description"] = c.Description,
                ["formal"] = Lsl.Formal(c.Requires, c.Ensures, " => "),
            }).ToArray()),
            ["trustedPieces"] = new JsonArray(plan.Assumptions.Select(a => (JsonNode)new JsonObject { ["id"] = a.Id, ["piece"] = a.Piece, ["contract"] = a.Contract }).ToArray()),
            ["beyondProof"] = LastroJson.ToNode(plan.BeyondProof),
            ["code"] = source,
        };
        return JsJson.Stringify(payload, indent: 2);
    }

    private static string Str(JsonNode? n) =>
        n is JsonValue v && v.GetValueKind() == JsonValueKind.String ? v.GetValue<string>() : n is null ? "" : n.ToJsonString();

    private static List<string> Strings(JsonNode? n) => (n as JsonArray ?? []).Select(Str).ToList();

    private static T Enum<T>(JsonNode? n, string where, List<string> problems) where T : struct, System.Enum
    {
        var s = Str(n);
        if (KebabEnumConverter<T>.TryParse(s, out var v)) return v;
        problems.Add($"{where}: unknown {typeof(T).Name} '{s}'");
        return default;
    }
}

public sealed class LlmIntentExtractor(AskJson ask) : IIntentExtractor
{
    public async Task<IntentArtifact> ExtractAsync(string prose, string? context = null, CancellationToken cancellationToken = default)
    {
        var answer = await ask(new LlmRequest("extract", LlmPrompts.ExtractSystem, LlmPrompts.ExtractUserPrompt(prose, context), LlmPrompts.ExtractSchema), cancellationToken).ConfigureAwait(false);
        return LlmPrompts.ArtifactFromExtraction(prose, answer);
    }
}

public sealed class LlmProofDeriver(AskJson ask) : IProofDeriver
{
    public async Task<ProofPlan> DeriveAsync(IntentArtifact artifact, Model model, CancellationToken cancellationToken = default)
    {
        var answer = await ask(new LlmRequest("derive", LlmPrompts.DeriveSystem, LlmPrompts.DeriveUserPrompt(artifact, model), LlmPrompts.DeriveSchema), cancellationToken).ConfigureAwait(false);
        return LlmPrompts.PlanFromDerivation(artifact, model, answer);
    }
}

/// <summary>LLM gap review: what the proofs cannot see. Evidence for humans, never proof.</summary>
public sealed class LlmGapReviewer(AskJson ask)
{
    public async Task<List<Gap>> ReviewAsync(IntentArtifact artifact, ProofPlan plan, string source, CancellationToken cancellationToken = default)
    {
        var answer = await ask(new LlmRequest("gap-review", LlmPrompts.GapReviewSystem, LlmPrompts.GapReviewUserPrompt(artifact, plan, source), LlmPrompts.GapReviewSchema), cancellationToken).ConfigureAwait(false);
        return answer?["gaps"] is JsonArray gaps ? LastroJson.FromNode<List<Gap>>(gaps) : [];
    }
}
