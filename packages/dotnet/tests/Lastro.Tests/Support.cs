using System.Diagnostics;
using Xunit;

namespace Lastro.Tests;

internal static class Support
{
    public static string RepoRoot
    {
        get
        {
            for (var d = new DirectoryInfo(AppContext.BaseDirectory); d is not null; d = d.Parent)
                if (File.Exists(Path.Combine(d.FullName, "Cargo.toml")) && Directory.Exists(Path.Combine(d.FullName, "packages"))) return d.FullName;
            throw new InvalidOperationException("repository root not found");
        }
    }

    /// <summary>In CI (LASTRO_REQUIRE_CROSSCHECK=1) a missing engine fails the test instead of skipping it.</summary>
    public static void Require(bool available, string reason)
    {
        if (available) return;
        if (Environment.GetEnvironmentVariable("LASTRO_REQUIRE_CROSSCHECK") == "1") Assert.Fail(reason);
        Skip.If(true, reason);
    }

    public static string? Which(string exe) =>
        (Environment.GetEnvironmentVariable("PATH") ?? "").Split(Path.PathSeparator, StringSplitOptions.RemoveEmptyEntries)
        .Select(d => Path.Combine(d, exe)).FirstOrDefault(File.Exists);

    public static bool TsBuilt => File.Exists(Path.Combine(RepoRoot, "packages/sdk/dist/examples/login/login.js")) && Which("node") is not null;

    public const string TsMissing = "the TypeScript SDK is not built (run `npm install && npx tsc -p packages/sdk`) or node is not on PATH";

    public static (int Code, string Stdout, string Stderr) Run(string exe, IEnumerable<string> args, string? cwd = null)
    {
        var psi = new ProcessStartInfo(exe) { RedirectStandardOutput = true, RedirectStandardError = true, UseShellExecute = false, WorkingDirectory = cwd ?? RepoRoot };
        foreach (var a in args) psi.ArgumentList.Add(a);
        // Keep the CLIs on handoff even where an API key is set.
        psi.Environment.Remove("ANTHROPIC_API_KEY");
        using var p = Process.Start(psi)!;
        var stdout = p.StandardOutput.ReadToEndAsync();
        var stderr = p.StandardError.ReadToEndAsync();
        p.WaitForExit();
        return (p.ExitCode, stdout.Result, stderr.Result);
    }

    public static IntentArtifact Reviewed(int d1 = 0, bool fixI5 = true)
    {
        var r = Artifacts.Review(LoginExample.Extract()).ApproveAll().Answer("D1", d1).Answer("D2", 0);
        if (fixI5) r = r.Amend("I5", LoginExample.FixedI5);
        return Artifacts.Freeze(r.Done());
    }

    public static IntentArtifact ReviewedLogin()
    {
        var a = LoginExample.Extract();
        foreach (var it in a.Intents) it.Status = IntentStatus.Approved;
        var i5 = a.Intents.First(i => i.Id == "I5");
        i5.OriginalStatement = i5.Statement;
        i5.Statement = LoginExample.FixedI5;
        i5.Status = IntentStatus.Amended;
        foreach (var d in a.Decisions) d.Chosen = d.Recommended;
        return a;
    }
}
