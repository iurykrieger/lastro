using System.Text.Json.Nodes;
using Xunit;
using static Lastro.Tests.Support;

namespace Lastro.Tests;

/// <summary>
/// The feature map and the snapshot helpers agree with the TS SDK: same maps, errors and text, same example inputs and
/// contracts (their files are hashed, so the bytes must match).
/// </summary>
public class FeaturesTests
{
    private const string Script = """
        const L = await import(process.argv[1]);
        const cases = JSON.parse(process.argv[2]);
        const maps = cases.map(({ features, removed, changed }) => {
          try {
            const m = L.featureMap(features, removed);
            const affected = L.affectedBy(m, changed);
            return { map: m, affected, text: L.renderFeatureMap(m, affected) };
          } catch (e) {
            return { error: e.message, cycle: e.cycle ?? [] };
          }
        });
        const big = { name: "big", inputs: { a: L.D.int(0, 99), b: L.D.int(0, 99) }, outputs: { y: L.D.enum("x", 2, true) } };
        const small = { name: "small", inputs: { member: L.D.bool(), subtotal: L.D.int(0, 3) }, outputs: { fee: L.D.int(0, 15) } };
        process.stdout.write(JSON.stringify({
          maps,
          examples: { big: L.exampleInputs(big, [{ b: 1, a: 2 }]), small: L.exampleInputs(small, [{ subtotal: 2, member: true }]) },
          contracts: { big: L.modelContract(big), small: L.modelContract(small) },
          diff: L.structuralDiff({ a: "int", b: "string", c: "bool" }, { a: "int", b: "number", d: "string" }),
          path: [L.snapshotPath("P-006", "after", "examples-a b.json", { feature: "UC-001", intent: "P-006.I-03" }), L.snapshotPath("P-006", "before", "data.json")],
        }));
        """;

    private static Surface S(string kind, string name, string loc) => new() { Kind = kind, Name = name, Locator = loc };

    private static readonly (List<Linked> Features, List<string> Removed, List<string> Changed)[] Cases =
    [
        ([
            new("UC-003", "checkout", "Checkout", [S("screen", "Checkout", "/checkout"), S("api", "Pay", "POST /pay")], ["UC-001", "UC-002"]),
            new("UC-001", "login", "Login", [S("route", "Login", "/login")]),
            new("UC-002", "cart", "Cart", null, ["UC-001"]),
            new("UC-004", "receipt", "Receipt", null, ["UC-003"]),
        ], [], ["UC-001"]),
        ([
            new("UC-001", "a", "A", null, ["UC-002"]),
            new("UC-002", "b", "B", null, ["UC-003"]),
            new("UC-003", "c", "C", null, ["UC-002", "UC-009"]),
        ], [], []),
        ([new("UC-001", "a", "A", [S("page", "", "")])], [], []),
        ([new("UC-001", "a", "A", null, ["UC-002"]), new("UC-002", "b", "B")], ["UC-002"], ["UC-002"]),
        ([], [], []),
    ];

    private static string Canon(JsonNode? n) => JsJson.Canonical(n);

    [SkippableFact]
    public void FeatureMapAndSnapshotHelpersMatchTheTsSdk()
    {
        Require(TsBuilt, TsMissing);
        var input = new JsonArray([.. Cases.Select(c => (JsonNode)new JsonObject
        {
            ["features"] = LastroJson.ToNode(c.Features),
            ["removed"] = LastroJson.ToNode(c.Removed),
            ["changed"] = LastroJson.ToNode(c.Changed),
        })]);
        var index = new Uri(Path.Combine(RepoRoot, "sdks/typescript/dist/src/index.js")).AbsoluteUri;
        var (code, stdout, stderr) = Run("node", ["--input-type=module", "-e", Script, index, input.ToJsonString()]);
        Assert.True(code == 0, stderr);
        var ts = JsonNode.Parse(stdout)!;
        for (var k = 0; k < Cases.Length; k++)
        {
            var (features, removed, changed) = Cases[k];
            JsonNode got;
            try
            {
                var m = Features.Build(features, removed);
                var affected = Features.AffectedBy(m, changed);
                got = new JsonObject { ["map"] = LastroJson.ToNode(m), ["affected"] = LastroJson.ToNode(affected), ["text"] = Features.Render(m, affected) };
            }
            catch (FeatureMapException e)
            {
                got = new JsonObject { ["error"] = e.Message, ["cycle"] = LastroJson.ToNode(e.Cycle.ToList()) };
            }
            Assert.Equal(Canon(ts["maps"]![k]), Canon(got));
        }
        var big = new Model { Name = "big", Inputs = { ["a"] = D.Int(0, 99), ["b"] = D.Int(0, 99) }, Outputs = { ["y"] = D.Enum("x", 2, true) } };
        var small = new Model { Name = "small", Inputs = { ["member"] = D.Bool(), ["subtotal"] = D.Int(0, 3) }, Outputs = { ["fee"] = D.Int(0, 15) } };
        foreach (var (name, model, scen) in new[] { ("big", big, """{"b":1,"a":2}"""), ("small", small, """{"subtotal":2,"member":true}""") })
        {
            var (inputs, whole) = Snapshots.ExampleInputs(model, [JsonNode.Parse(scen)!]);
            var want = ts["examples"]![name]!;
            Assert.Equal(want["wholeSpace"]!.GetValue<bool>(), whole);
            // The examples file keeps the key order: compare the exact JSON text.
            Assert.Equal(JsJson.Stringify(want["inputs"]), JsJson.Stringify(new JsonArray([.. inputs.Select(i => i.DeepClone())])));
            Assert.Equal(JsJson.Stringify(ts["contracts"]![name]), JsJson.Stringify(Snapshots.ModelContract(model)));
        }
        var diff = Snapshots.StructuralDiff(new Dictionary<string, string> { ["a"] = "int", ["b"] = "string", ["c"] = "bool" }, new Dictionary<string, string> { ["a"] = "int", ["b"] = "number", ["d"] = "string" });
        Assert.Equal(Canon(ts["diff"]), Canon(LastroJson.ToNode(diff)));
        Assert.Equal(Canon(ts["path"]), Canon(new JsonArray(Snapshots.PathOf("P-006", "after", "examples-a b.json", "UC-001", "P-006.I-03"), Snapshots.PathOf("P-006", "before", "data.json"))));
    }
}
