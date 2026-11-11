using System.Text.Json.Nodes;
using Lastro;
using Lastro.Claude;

// --replay answers the two AI steps from ../answers (what the CLI walkthrough uses); otherwise Claude does.
AskJson ask = args.Contains("--replay")
    ? (request, _) => Task.FromResult(JsonNode.Parse(File.ReadAllText($"../answers/{request.Step}.json")))
    : ClaudeAsk.Create();

var proof = await Shipping.ProveAsync(ask, Shipping.Fee);
Console.WriteLine(proof.Receipt);
if (!proof.Ok) Console.Error.WriteLine(Feedback.AgentFeedback(proof));
return proof.Ok ? 0 : 1;

public static class Shipping
{
    /// <summary>The decision core under proof.</summary>
    public static Env Fee(Env input)
    {
        var subtotal = input["subtotal"].AsLong;
        var member = input["member"].AsBool;
        return new Env { ["fee"] = member || subtotal >= 100 ? 0 : 15 };
    }

    public static async Task<Proof> ProveAsync(AskJson ask, Func<Env, Env> subject)
    {
        var model = LastroJson.Deserialize<Model>(File.ReadAllText("../model.json"));
        var prose = File.ReadAllText("../prose.md").Trim();

        var lastro = new LastroClient(new LastroOptions
        {
            Model = model,
            Ask = ask,
            RequiredApprovals = 1,
            Rust = RustProver.Locate(), // lastro-prove: $LASTRO_PROVE, PATH or ./target; null when absent
        });

        var intents = await lastro.PlanAsync(prose);                     // AI: extract intents
        intents = lastro.Review(intents, "ana").Answer("D1", 1).ApproveAll().Done();
        if (lastro.Pending(intents).Count > 0) throw new InvalidOperationException("review pending");

        var spec = await lastro.SpecAsync(intents);                      // AI: derive the formal spec, then validate it
        if (!spec.Ok) throw new InvalidOperationException("the spec contradicts itself: answer lastro.ReturnDecisions(spec)");

        return await lastro.ProveAsync(spec, subject);                   // deterministic, in both engines
    }
}
