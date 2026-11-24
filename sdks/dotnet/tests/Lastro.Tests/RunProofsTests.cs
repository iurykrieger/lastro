using System.Net;
using System.Net.Sockets;
using System.Text;
using System.Text.Json.Nodes;
using Xunit;
using static Lastro.Tests.Support;

namespace Lastro.Tests;

/// <summary>
/// Machine, driven and property proofs run as the TS SDK runs them: same verdicts, notes and counterexamples. The
/// driven scenarios and the property go through one HTTP server both SDKs drive.
/// </summary>
public class RunProofsTests
{
    private const string Script = """
        const L = await import(process.argv[1]);
        const { driveRunner } = await import(process.argv[2]);
        const proofs = JSON.parse(process.argv[3]);
        const counter = JSON.parse(process.argv[4]);
        const step = (x) =>
          x.event === "read" && x.pc === "idle" ? { t: x.n, pc: "read" } : x.event === "write" && x.pc === "read" ? { n: x.t + 1, pc: "done" } : null;
        const drivers = { api: { baseUrl: process.argv[5] } };
        const ws = { file: (p) => p, root: process.cwd() };
        const { outcomes } = await L.runProofs(proofs.map((proof) => ({ proof, role: "proposal" })), { models: { counter }, subjects: { counter: step }, drive: driveRunner(ws, drivers) });
        process.stdout.write(JSON.stringify(outcomes));
        """;

    private const string Machine = """{"name":"counter","kind":"machine","actors":2,"state":{"n":{"type":"int","min":0,"max":9}},"local":{"pc":{"type":"enum","values":["idle","read","done"]},"t":{"type":"int","min":0,"max":9}},"initial":{"n":0,"pc":"idle","t":0},"events":{"read":{},"write":{}}}""";

    private static string Done(int a) => $$"""{"op":"ite","args":[{"op":"eq","args":[{"var":"in.pc_{{a}}"},{"const":"done"}]},{"const":1},{"const":0}]}""";

    private static string Pf(string id, string check) =>
        $$"""{"id":"P-001.PF-{{id}}","slug":"pf-{{id}}","name":"x","description":"x","intent":"P-001.I-01","useCase":"UC-001","model":"counter","subject":"counter","check":{{check}},"status":"pending"}""";

    private static HttpListener Serve(out string baseUrl)
    {
        var probe = new TcpListener(IPAddress.Loopback, 0);
        probe.Start();
        var port = ((IPEndPoint)probe.LocalEndpoint).Port;
        probe.Stop();
        baseUrl = $"http://127.0.0.1:{port}";
        var http = new HttpListener();
        http.Prefixes.Add(baseUrl + "/");
        http.Start();
        _ = Task.Run(async () =>
        {
            while (http.IsListening)
            {
                HttpListenerContext ctx;
                try { ctx = await http.GetContextAsync(); } catch { return; }
                var (status, type, body) = ctx.Request.Url!.AbsolutePath switch
                {
                    "/pay" => (200, "application/json", """{"paid":true,"total":10}"""),
                    "/boom" => (500, "text/plain", "boom"),
                    _ => (200, "text/plain", """{"ok":true}"""),
                };
                var bytes = Encoding.UTF8.GetBytes(body);
                ctx.Response.StatusCode = status;
                ctx.Response.ContentType = type;
                await ctx.Response.OutputStream.WriteAsync(bytes);
                ctx.Response.Close();
            }
        });
        return http;
    }

    [SkippableFact]
    public async Task MachineDrivenAndPropertyProofsRunAsInTheTsSdk()
    {
        Require(TsBuilt, TsMissing);
        using var http = Serve(out var baseUrl);
        var proofs = "[" + string.Join(",",
            Pf("01", """{"kind":"machine","clause":"invariant","ensures":{"op":"eq","args":[{"var":"in.n"},{"op":"add","args":[""" + Done(1) + "," + Done(2) + "]}]}}"),
            Pf("02", """{"kind":"machine","clause":"reach","ensures":{"op":"eq","args":[{"var":"in.n"},{"const":2}]}}"""),
            Pf("03", """{"kind":"machine","clause":"step","requires":{"op":"eq","args":[{"var":"in.event"},{"const":"write"}]},"ensures":{"op":"eq","args":[{"var":"out.pc"},{"const":"done"}]}}"""),
            Pf("04", """{"kind":"driven","surface":{"kind":"api","locator":"POST /pay"},"steps":[{"input":{"method":"POST","path":"/pay","body":{"cart":"c1"}},"expect":{"status":200,"body":{"paid":true}}}]}"""),
            Pf("05", """{"kind":"driven","surface":{"kind":"api","locator":"POST /pay"},"steps":[{"input":{"path":"/pay"},"expect":{"status":200}},{"input":{"path":"/pay"},"expect":{"body":{"total":11}}}]}"""),
            Pf("06", """{"kind":"property","surface":{"kind":"api","locator":"/"},"actions":[{"path":"/a"},{"path":"/b"}],"invariant":{"op":"eq","args":[{"var":"out.status"},{"const":200}]}}"""),
            Pf("07", """{"kind":"property","surface":{"kind":"api","locator":"/"},"actions":[{"path":"/a"},{"path":"/boom"}],"invariant":{"op":"eq","args":[{"var":"out.status"},{"const":200}]}}""")) + "]";

        var dist = Path.Combine(RepoRoot, "sdks/typescript/dist/src");
        var (code, stdout, stderr) = Run("node", ["--input-type=module", "-e", Script, new Uri(Path.Combine(dist, "index.js")).AbsoluteUri,
            new Uri(Path.Combine(dist, "evidence.js")).AbsoluteUri, proofs, Machine, baseUrl]);
        Assert.True(code == 0, stderr);

        var entries = LastroJson.FromNode<List<ProofEntry>>(JsonNode.Parse(proofs));
        var m = LastroJson.FromNode<MachineModel>(JsonNode.Parse(Machine));
        Func<Env, Task<Env?>> step = x =>
        {
            var n = x["n"].AsNumber;
            var t = x["t"].AsNumber;
            Env? o = (x["event"].AsString, x["pc"].AsString) switch
            {
                ("read", "idle") => new Env { ["t"] = n, ["pc"] = "read" },
                ("write", "read") => new Env { ["n"] = t + 1, ["pc"] = "done" },
                _ => null,
            };
            return Task.FromResult(o);
        };
        var drivers = new JsonObject { ["api"] = new JsonObject { ["baseUrl"] = baseUrl } };
        var (outcomes, _) = await Proposals.RunProofsAsync(entries.Select(e => (e, "proposal")).ToList(), new RunOptions(
            new Dictionary<string, Subject> { ["counter"] = async x => (await step(x))! }, new Dictionary<string, Model>(),
            Machines: new Dictionary<string, MachineModel> { ["counter"] = m }, Drive: new Drivers(drivers, ".", ".")));

        var ts = JsonNode.Parse(stdout)!.AsArray();
        Assert.Equal(ts.Count, outcomes.Count);
        for (var i = 0; i < ts.Count; i++)
        {
            var (want, got) = (JsJson.Canonical(ts[i]), JsJson.Canonical(LastroJson.ToNode(outcomes[i])));
            Assert.True(want == got, $"outcome {i}:\nTS   {want}\n.NET {got}");
        }
        // A proof's check is written back exactly as the TS SDK writes it.
        var all = JsonNode.Parse(proofs)!.AsArray();
        for (var i = 0; i < entries.Count; i++)
            Assert.Equal(all[i]!["check"]!.ToJsonString(), LastroJson.ToNode(entries[i].Check)!.ToJsonString());
    }
}
