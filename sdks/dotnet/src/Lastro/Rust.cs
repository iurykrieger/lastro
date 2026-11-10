using System.Diagnostics;
using System.Text.Json.Nodes;

namespace Lastro;

public sealed class RustClauseProof
{
    public string ClauseId { get; set; } = "";
    public ClauseVerdict Verdict { get; set; }
    public string Evidence { get; set; } = "";
    public int Checked { get; set; }
    public Counterexample? Counterexample { get; set; }
    public double? FailureRateUpperBound95 { get; set; }
    public MutantStats Mutants { get; set; } = new();
    public string? Error { get; set; }
}

public sealed class RustCoverage
{
    public double? Space { get; set; }
    public int Rows { get; set; }
    public bool Complete { get; set; }
    public int Missing { get; set; }
    public Env? FirstMissing { get; set; }
    public int OutOfDomain { get; set; }
    public int Nondeterministic { get; set; }
}

public sealed class RustContradiction
{
    public Env Input { get; set; } = new();
    public List<string> Clauses { get; set; } = [];
}

/// <summary>The independent Rust prover's verdicts (<c>crates/lastro-core</c>).</summary>
public sealed class RustProofReport
{
    public string Engine { get; set; } = "";
    public string Model { get; set; } = "";
    public RustCoverage Coverage { get; set; } = new();
    public List<RustClauseProof> Clauses { get; set; } = [];
    public List<RustContradiction> Contradictions { get; set; } = [];
    public string TableDigest { get; set; } = "";
    public bool Ok { get; set; }
}

/// <summary>One input/output pair of the implementation.</summary>
public sealed class Row
{
    public Env Input { get; set; } = new();
    public Env Output { get; set; } = new();
}

/// <summary>
/// A second, independent implementation of the LSL semantics. It enumerates the declared input space itself
/// and only says <c>proved</c> when the table covers every point of it.
/// </summary>
public interface IRustProver
{
    RustProofReport Prove(ProofPlan plan, IReadOnlyList<Row> table);
}

/// <summary>Runs the native <c>lastro-prove</c> binary (JSON on stdin, JSON on stdout).</summary>
public sealed class RustProver(string executable) : IRustProver
{
    public string Executable { get; } = executable;

    /// <summary>
    /// Finds <c>lastro-prove</c>: the <c>LASTRO_PROVE</c> environment variable, then <c>PATH</c>, then
    /// <c>target/release</c> or <c>target/debug</c> of an enclosing Cargo workspace. Null when none is found.
    /// </summary>
    public static RustProver? Locate(string? startDirectory = null)
    {
        var exe = OperatingSystem.IsWindows() ? "lastro-prove.exe" : "lastro-prove";
        var env = Environment.GetEnvironmentVariable("LASTRO_PROVE");
        if (!string.IsNullOrEmpty(env) && File.Exists(env)) return new RustProver(env);
        foreach (var dir in (Environment.GetEnvironmentVariable("PATH") ?? "").Split(Path.PathSeparator, StringSplitOptions.RemoveEmptyEntries))
        {
            var p = Path.Combine(dir, exe);
            if (File.Exists(p)) return new RustProver(p);
        }
        for (var d = new DirectoryInfo(startDirectory ?? AppContext.BaseDirectory); d is not null; d = d.Parent)
        {
            if (!File.Exists(Path.Combine(d.FullName, "Cargo.toml"))) continue;
            foreach (var profile in new[] { "release", "debug" })
            {
                var p = Path.Combine(d.FullName, "target", profile, exe);
                if (File.Exists(p)) return new RustProver(p);
            }
        }
        return null;
    }

    /// <summary>The request the prover reads: model (with defs), clauses and table.</summary>
    public static string Request(ProofPlan plan, IReadOnlyList<Row> table)
    {
        var model = LastroJson.ToNode(plan.Model)!.AsObject();
        model["defs"] ??= new JsonObject();
        var req = new JsonObject
        {
            ["model"] = model,
            ["clauses"] = LastroJson.ToNode(plan.Clauses),
            ["table"] = LastroJson.ToNode(table),
        };
        return req.ToJsonString(LastroJson.Options);
    }

    public RustProofReport Prove(ProofPlan plan, IReadOnlyList<Row> table)
    {
        var psi = new ProcessStartInfo(Executable)
        {
            RedirectStandardInput = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            UseShellExecute = false,
        };
        using var p = Process.Start(psi) ?? throw new InvalidOperationException($"could not start {Executable}");
        var stdout = p.StandardOutput.ReadToEndAsync();
        var stderr = p.StandardError.ReadToEndAsync();
        p.StandardInput.Write(Request(plan, table));
        p.StandardInput.Close();
        p.WaitForExit();
        var json = stdout.GetAwaiter().GetResult();
        var node = JsonNode.Parse(json) as JsonObject ?? throw new InvalidOperationException($"lastro-core: no report ({stderr.GetAwaiter().GetResult()})");
        if (node["error"] is { } err) throw new InvalidOperationException($"lastro-core: {err}");
        return LastroJson.FromNode<RustProofReport>(node);
    }

    /// <summary>Runs the implementation over the whole (finite) input space: the table the Rust prover checks.</summary>
    public static async Task<List<Row>> TabulateAsync(Model model, Subject subject, double limit = 100_000)
    {
        var size = Domains.SpaceSize(model.Inputs);
        if (size > limit) throw new InvalidOperationException($"input space has {JsJson.Number(size)} points (limit {JsJson.Number(limit)}); use sampling instead");
        var rows = new List<Row>();
        foreach (var input in Domains.EnumerateSpace(model.Inputs))
            rows.Add(new Row { Input = input, Output = await subject(input.Copy()).ConfigureAwait(false) });
        return rows;
    }
}
