using System.Diagnostics;
using System.Globalization;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Text.RegularExpressions;

namespace Lastro;

// Driving the application through its surfaces, over the protocols the market already uses (as the TS SDK's drive.ts):
//
//   screen / route  W3C WebDriver with any WebDriver server, or Appium for Android and iOS (appium:* capabilities)
//   api             HTTP, called directly
//   command         a process
//   any kind        Lastro's own JSON lines protocol, only when no standard protocol fits
//
// `.lastro/config.json` names them under `drivers`. Lastro ships no browser, device or Appium server. A driver that
// fails to start or times out (30 s per action by default) fails the scenario with the driver's error, never passes it.

/// <summary>A driver that failed to start, broke or timed out.</summary>
public sealed class DriverException(string message) : Exception(message);

/// <summary>One open surface.</summary>
public interface ISession : IAsyncDisposable
{
    Task<JsonObject> ActAsync(JsonObject action);
    /// <summary>A PNG of the screen (screen and route surfaces), null otherwise.</summary>
    Task<byte[]?> ScreenshotAsync();
}

/// <summary>The result of one driven scenario or property.</summary>
public sealed record DrivenOutcome(string Verdict, bool Passed, ProofCounterexample? Counterexample = null, string? Note = null);

/// <summary>Runs driven scenarios and properties (see <see cref="Drivers"/>).</summary>
public interface IDriveRunner
{
    Task<DrivenOutcome> RunAsync(ProofEntry proof);
}

public static class Drive
{
    /// <summary>A JSON number as a double, whatever CLR type the node was built from.</summary>
    internal static double Num(JsonNode v) => double.Parse(v.ToJsonString(), CultureInfo.InvariantCulture);

    public static readonly TimeSpan DefaultTimeout = TimeSpan.FromSeconds(30);
    private static readonly HttpClient Http = new();

    private static JsonObject? DriverFor(string kind, JsonObject? d) =>
        (d?[kind] ?? (kind == "route" ? d?["screen"] : null)) as JsonObject;

    /// <summary>The protocol a surface kind is driven over: webdriver, appium, http, process, jsonlines or none.</summary>
    public static string ProtocolFor(string kind, JsonObject? drivers)
    {
        var d = DriverFor(kind, drivers);
        if (d is null) return "none";
        if (d["jsonlines"] is JsonValue) return "jsonlines";
        if (kind is "screen" or "route") return d["appium"] is JsonValue ? "appium" : d["webdriver"] is JsonValue ? "webdriver" : "none";
        if (kind == "api") return d["baseUrl"] is JsonValue ? "http" : "none";
        return d["process"] is JsonValue p && p.GetValueKind() == JsonValueKind.True ? "process" : "none";
    }

    private static TimeSpan TimeoutOf(JsonObject? drivers) =>
        drivers?["timeoutMs"] is JsonValue v && v.GetValueKind() == JsonValueKind.Number && Num(v) is var ms && ms > 0 ? TimeSpan.FromMilliseconds(ms) : DefaultTimeout;

    internal static async Task<T> WithTimeout<T>(Task<T> task, TimeSpan ms, string what)
    {
        if (await Task.WhenAny(task, Task.Delay(ms)).ConfigureAwait(false) == task) return await task.ConfigureAwait(false);
        throw new DriverException($"{what} timed out after {(long)ms.TotalMilliseconds} ms");
    }

    private static string Str(JsonNode? n) => n is JsonValue v && v.GetValueKind() == JsonValueKind.String ? v.GetValue<string>() : n?.ToJsonString() ?? "";

    /// <summary>Opens a session on a surface; root is the folder relative commands run in.</summary>
    public static async Task<ISession> OpenAsync(string kind, string locator, JsonObject? drivers, string root)
    {
        var protocol = ProtocolFor(kind, drivers);
        var d = DriverFor(kind, drivers);
        var ms = TimeoutOf(drivers);
        var cwd = d?["cwd"] is JsonValue c ? Path.GetFullPath(Path.Combine(root, c.GetValue<string>())) : root;
        return protocol switch
        {
            "webdriver" or "appium" => await WithTimeout(WebDriver.StartAsync(Str(d![protocol]), d["capabilities"] as JsonObject, d["baseUrl"] is JsonValue b ? b.GetValue<string>() : null, locator, ms), ms, $"starting the {protocol} driver").ConfigureAwait(false),
            "http" => new HttpSession(Str(d!["baseUrl"]), d["headers"] as JsonObject),
            "process" => new ProcessSession(cwd, locator, ms),
            "jsonlines" => await JsonLines.StartAsync(Str(d!["jsonlines"]), cwd, kind, locator, ms).ConfigureAwait(false),
            _ => throw new DriverException($"no driver configured for {kind} surfaces: add one under \"drivers\" in .lastro/config.json"),
        };
    }

    // -----------------------------------------------------------------------
    // W3C WebDriver (and Appium, which speaks the same protocol)
    // -----------------------------------------------------------------------

    private static async Task<JsonNode?> Wd(string server, HttpMethod method, string path, JsonNode? body = null)
    {
        using var req = new HttpRequestMessage(method, server.TrimEnd('/') + path);
        if (body is not null) req.Content = new StringContent(body.ToJsonString(), Encoding.UTF8, "application/json");
        HttpResponseMessage res;
        try { res = await Http.SendAsync(req).ConfigureAwait(false); }
        catch (HttpRequestException e) { throw new DriverException($"WebDriver {method} {path}: {e.Message}"); }
        JsonNode? value = null;
        try { value = JsonNode.Parse(await res.Content.ReadAsStringAsync().ConfigureAwait(false))?["value"]; } catch (JsonException) { }
        if (!res.IsSuccessStatusCode || (value is JsonObject o && o.ContainsKey("error")))
            throw new DriverException($"WebDriver {method} {path}: {(value is JsonObject m && m["message"] is { } msg ? Str(msg) : ((int)res.StatusCode).ToString(CultureInfo.InvariantCulture))}");
        return value;
    }

    private sealed class WebDriver(string server, string session, string? baseUrl, TimeSpan ms) : ISession
    {
        private static readonly Regex HttpUrl = new("^https?:");

        public static async Task<ISession> StartAsync(string server, JsonObject? caps, string? baseUrl, string locator, TimeSpan ms)
        {
            var created = await Wd(server, HttpMethod.Post, "/session", new JsonObject { ["capabilities"] = new JsonObject { ["alwaysMatch"] = caps?.DeepClone() ?? new JsonObject() } }).ConfigureAwait(false);
            var w = new WebDriver(server, $"/session/{Str(created?["sessionId"])}", baseUrl, ms);
            if (baseUrl is not null || HttpUrl.IsMatch(locator))
            {
                try { await Wd(server, HttpMethod.Post, $"{w.session}/url", new JsonObject { ["url"] = w.Url(locator) }).ConfigureAwait(false); }
                catch { await w.DisposeAsync().ConfigureAwait(false); throw; }
            }
            return w;
        }

        private readonly string session = session;

        private string Url(string to) => baseUrl is not null && to.StartsWith('/') ? baseUrl.TrimEnd('/') + to : to;

        private async Task<string> Element(string selector)
        {
            var (by, value) = selector.StartsWith('~') ? ("accessibility id", selector[1..]) : selector.StartsWith("//", StringComparison.Ordinal) ? ("xpath", selector) : ("css selector", selector);
            var el = await Wd(server, HttpMethod.Post, $"{session}/element", new JsonObject { ["using"] = by, ["value"] = value }).ConfigureAwait(false);
            return el is JsonObject o && o.Count > 0 ? Str(o.First().Value) : throw new DriverException($"no element {selector}");
        }

        public Task<JsonObject> ActAsync(JsonObject a) => WithTimeout(Run(a), ms, $"action {JsJson.Canonical(a)}");

        private async Task<JsonObject> Run(JsonObject a)
        {
            if (a["navigate"] is JsonValue nav && nav.GetValueKind() == JsonValueKind.String) await Wd(server, HttpMethod.Post, $"{session}/url", new JsonObject { ["url"] = Url(nav.GetValue<string>()) }).ConfigureAwait(false);
            if (a["click"] is JsonValue click && click.GetValueKind() == JsonValueKind.String) await Wd(server, HttpMethod.Post, $"{session}/element/{await Element(click.GetValue<string>()).ConfigureAwait(false)}/click", new JsonObject()).ConfigureAwait(false);
            if (a["type"] is JsonValue type && type.GetValueKind() == JsonValueKind.String)
                await Wd(server, HttpMethod.Post, $"{session}/element/{await Element(type.GetValue<string>()).ConfigureAwait(false)}/value", new JsonObject { ["text"] = a["text"] is null ? "" : Str(a["text"]) }).ConfigureAwait(false);
            var obs = new JsonObject();
            if (a["read"] is JsonValue read && read.GetValueKind() == JsonValueKind.String)
                obs["text"] = (await Wd(server, HttpMethod.Get, $"{session}/element/{await Element(read.GetValue<string>()).ConfigureAwait(false)}/text").ConfigureAwait(false))?.DeepClone();
            JsonNode? url = null;
            try { url = await Wd(server, HttpMethod.Get, $"{session}/url").ConfigureAwait(false); } catch (DriverException) { }
            obs["url"] = url?.DeepClone();
            return obs;
        }

        public async Task<byte[]?> ScreenshotAsync() =>
            Convert.FromBase64String(Str(await WithTimeout(Wd(server, HttpMethod.Get, $"{session}/screenshot"), ms, "taking a screenshot").ConfigureAwait(false)));

        public async ValueTask DisposeAsync()
        {
            try { await Wd(server, HttpMethod.Delete, session).ConfigureAwait(false); } catch { /* already gone */ }
        }
    }

    // -----------------------------------------------------------------------
    // HTTP and processes
    // -----------------------------------------------------------------------

    private sealed class HttpSession(string baseUrl, JsonObject? headers) : ISession
    {
        public async Task<JsonObject> ActAsync(JsonObject a)
        {
            var method = a["method"] is JsonValue m ? m.GetValue<string>() : "GET";
            var path = a["path"] is JsonValue p ? p.GetValue<string>() : "/";
            using var req = new HttpRequestMessage(new HttpMethod(method), baseUrl.TrimEnd('/') + path);
            if (a["body"] is { } body) req.Content = new StringContent(JsJson.Stringify(body), Encoding.UTF8, "application/json");
            if (req.Content is not null) req.Content.Headers.ContentType = new MediaTypeHeaderValue("application/json");
            foreach (var (k, v) in headers ?? []) req.Headers.TryAddWithoutValidation(k, Str(v));
            foreach (var (k, v) in a["headers"] as JsonObject ?? []) req.Headers.TryAddWithoutValidation(k, Str(v));
            HttpResponseMessage res;
            try { res = await Http.SendAsync(req).ConfigureAwait(false); }
            catch (HttpRequestException e) { throw new DriverException(e.Message); }
            var text = await res.Content.ReadAsStringAsync().ConfigureAwait(false);
            JsonNode? parsed;
            try { parsed = JsonNode.Parse(text); } catch (JsonException) { parsed = JsonValue.Create(text); }
            return new JsonObject { ["status"] = (int)res.StatusCode, ["body"] = parsed };
        }

        public Task<byte[]?> ScreenshotAsync() => Task.FromResult<byte[]?>(null);
        public ValueTask DisposeAsync() => ValueTask.CompletedTask;
    }

    private static ProcessStartInfo Shell(string command, string cwd) => new("sh", ["-c", command])
    {
        WorkingDirectory = cwd, RedirectStandardInput = true, RedirectStandardOutput = true, RedirectStandardError = true, UseShellExecute = false,
    };

    private sealed class ProcessSession(string cwd, string locator, TimeSpan ms) : ISession
    {
        public async Task<JsonObject> ActAsync(JsonObject a)
        {
            var line = a["run"] is JsonValue r && r.GetValueKind() == JsonValueKind.String ? r.GetValue<string>()
                : string.Join(" ", new[] { locator }.Concat((a["args"] as JsonArray ?? []).Select(Str)));
            using var proc = Process.Start(Shell(line, cwd)) ?? throw new DriverException($"could not run {line}");
            if (a["stdin"] is JsonValue si && si.GetValueKind() == JsonValueKind.String) await proc.StandardInput.WriteAsync(si.GetValue<string>()).ConfigureAwait(false);
            proc.StandardInput.Close();
            var stdout = proc.StandardOutput.ReadToEndAsync();
            var stderr = proc.StandardError.ReadToEndAsync();
            using var cts = new CancellationTokenSource(ms);
            try { await proc.WaitForExitAsync(cts.Token).ConfigureAwait(false); }
            catch (OperationCanceledException)
            {
                try { proc.Kill(entireProcessTree: true); } catch { /* gone */ }
                throw new DriverException($"running {line} timed out after {(long)ms.TotalMilliseconds} ms");
            }
            return new JsonObject { ["exitCode"] = proc.ExitCode, ["stdout"] = await stdout.ConfigureAwait(false), ["stderr"] = await stderr.ConfigureAwait(false) };
        }

        public Task<byte[]?> ScreenshotAsync() => Task.FromResult<byte[]?>(null);
        public ValueTask DisposeAsync() => ValueTask.CompletedTask;
    }

    // -----------------------------------------------------------------------
    // Lastro's JSON lines protocol: only when no standard protocol fits
    //   -> {"open": {"kind": "screen", "locator": "/chats"}}   <- {"ok": true}
    //   -> {"act": {...action}}                                <- {"observed": {...}, "screenshot"?: "<base64 PNG>"}
    //   -> {"screenshot": true}                                <- {"screenshot": "<base64 PNG>"}
    //   -> {"close": true}                                     <- {"ok": true}
    // Any answer with "error" fails the step with that error.
    // -----------------------------------------------------------------------

    private sealed class JsonLines(Process proc, TimeSpan ms) : ISession
    {
        private byte[]? last;

        public static async Task<ISession> StartAsync(string command, string cwd, string kind, string locator, TimeSpan ms)
        {
            var info = Shell(command, cwd);
            info.RedirectStandardError = false;
            Process proc;
            try { proc = Process.Start(info) ?? throw new DriverException("the driver could not start"); }
            catch (Exception e) when (e is not DriverException) { throw new DriverException($"the driver could not start: {e.Message}"); }
            var j = new JsonLines(proc, ms);
            try { await j.Ask(new JsonObject { ["open"] = new JsonObject { ["kind"] = kind, ["locator"] = locator } }, "starting the JSON lines driver").ConfigureAwait(false); }
            catch { j.Stop(); throw; }
            return j;
        }

        private async Task<JsonObject> Ask(JsonObject msg, string what)
        {
            if (proc.HasExited) throw new DriverException($"the driver exited with code {proc.ExitCode}");
            try { await proc.StandardInput.WriteLineAsync(msg.ToJsonString()).ConfigureAwait(false); await proc.StandardInput.FlushAsync().ConfigureAwait(false); }
            catch (IOException) { throw new DriverException("the driver exited"); }
            var line = await WithTimeout(proc.StandardOutput.ReadLineAsync(), ms, what).ConfigureAwait(false) ?? throw new DriverException("the driver exited");
            var o = JsonNode.Parse(line) as JsonObject ?? throw new DriverException($"the driver answered with something that is not a JSON object: {line}");
            if (o["error"] is { } err && err.GetValueKind() is not (JsonValueKind.Null or JsonValueKind.False)) throw new DriverException(Str(err));
            return o;
        }

        public async Task<JsonObject> ActAsync(JsonObject a)
        {
            var o = await Ask(new JsonObject { ["act"] = a.DeepClone() }, $"action {JsJson.Canonical(a)}").ConfigureAwait(false);
            last = o["screenshot"] is JsonValue s && s.GetValueKind() == JsonValueKind.String ? Convert.FromBase64String(s.GetValue<string>()) : null;
            return o["observed"]?.DeepClone() as JsonObject ?? [];
        }

        public async Task<byte[]?> ScreenshotAsync()
        {
            if (last is not null) return last;
            var o = await Ask(new JsonObject { ["screenshot"] = true }, "taking a screenshot").ConfigureAwait(false);
            return o["screenshot"] is JsonValue s && s.GetValueKind() == JsonValueKind.String ? Convert.FromBase64String(s.GetValue<string>()) : null;
        }

        public async ValueTask DisposeAsync()
        {
            try { await Ask(new JsonObject { ["close"] = true }, "closing the driver").ConfigureAwait(false); } catch { /* closing anyway */ }
            Stop();
        }

        private void Stop()
        {
            try { if (!proc.HasExited) proc.Kill(entireProcessTree: true); } catch { /* gone */ }
            proc.Dispose();
        }
    }

    // -----------------------------------------------------------------------
    // Matching and screenshots
    // -----------------------------------------------------------------------

    /// <summary>True when every field of <paramref name="expected"/> equals the observed one, recursively.</summary>
    public static bool Matches(JsonNode? expected, JsonNode? observed) => expected switch
    {
        JsonObject e => observed is JsonObject o && e.All(kv => Matches(kv.Value, o[kv.Key])),
        JsonArray e => observed is JsonArray o && o.Count == e.Count && e.Select((x, i) => Matches(x, o[i])).All(x => x),
        null => observed is null || observed.GetValueKind() == JsonValueKind.Null,
        _ => observed is JsonValue && SameValue(expected, observed),
    };

    private static bool SameValue(JsonNode a, JsonNode b)
    {
        var (ka, kb) = (a.GetValueKind(), b.GetValueKind());
        if (ka == JsonValueKind.Number && kb == JsonValueKind.Number) return Drive.Num(a) == Drive.Num(b);
        return ka == kb && (ka is JsonValueKind.True or JsonValueKind.False or JsonValueKind.Null || a.ToJsonString() == b.ToJsonString());
    }

    /// <summary>The share of pixels that differ between two PNGs (1 when their sizes differ or one is unreadable).</summary>
    public static double PixelShare(byte[] a, byte[] b)
    {
        var (x, y) = (Png.Decode(a), Png.Decode(b));
        if (x is null || y is null || x.Width != y.Width || x.Height != y.Height) return 1;
        var n = x.Width * x.Height;
        if (n == 0) return 0;
        var changed = 0;
        for (var i = 0; i < n; i++)
            if (x.Rgba[i * 4] != y.Rgba[i * 4] || x.Rgba[i * 4 + 1] != y.Rgba[i * 4 + 1] || x.Rgba[i * 4 + 2] != y.Rgba[i * 4 + 2] || x.Rgba[i * 4 + 3] != y.Rgba[i * 4 + 3]) changed++;
        return (double)changed / n;
    }
}

/// <summary>Runs driven scenarios and properties through the configured drivers; references are read from the workspace.</summary>
public sealed class Drivers(JsonObject? config, string dir, string root, Func<string, string, Task<ISession>>? open = null) : IDriveRunner
{
    private Task<ISession> Open(string kind, string locator) => open is not null ? open(kind, locator) : Drive.OpenAsync(kind, locator, config, root);

    private static Env EnvOf(JsonObject o)
    {
        var e = new Env();
        foreach (var (k, v) in o)
            if (v is JsonValue jv)
                e[k] = jv.GetValueKind() switch
                {
                    JsonValueKind.String => jv.GetValue<string>(),
                    JsonValueKind.Number => Drive.Num(jv),
                    JsonValueKind.True => true,
                    JsonValueKind.False => false,
                    _ => Value.Missing,
                };
        return e;
    }

    public async Task<DrivenOutcome> RunAsync(ProofEntry pr)
    {
        var c = pr.Check;
        var kind = c.Surface?["kind"]?.GetValue<string>() ?? "";
        var locator = c.Surface?["locator"]?.GetValue<string>() ?? "";
        if (c.Kind == "driven")
        {
            var steps = c.Steps as JsonArray ?? [];
            ISession s;
            try { s = await Open(kind, locator).ConfigureAwait(false); }
            catch (Exception e) { return new DrivenOutcome("violated", false, Note: $"driver error: {e.Message}"); }
            await using (s.ConfigureAwait(false))
            {
                for (var k = 0; k < steps.Count; k++)
                {
                    var step = steps[k] as JsonObject ?? [];
                    var input = step["input"] as JsonObject ?? [];
                    JsonObject obs;
                    try { obs = await s.ActAsync((JsonObject)input.DeepClone()).ConfigureAwait(false); }
                    catch (Exception e) { return new DrivenOutcome("violated", false, Note: $"driver error: {e.Message}"); }
                    var problem = Drive.Matches(step["expect"], obs) ? "" : $"expected {JsJson.Canonical(step["expect"])}, observed {JsJson.Canonical(obs)}";
                    if (step["reference"] is JsonValue rf && kind is "screen" or "route" && File.Exists(Path.Combine(dir, rf.GetValue<string>())))
                    {
                        var shot = await s.ScreenshotAsync().ConfigureAwait(false);
                        var share = shot is null ? 1 : Drive.PixelShare(File.ReadAllBytes(Path.Combine(dir, rf.GetValue<string>())), shot);
                        var tol = c.Tolerance ?? 0;
                        if (share > tol)
                        {
                            var p = $"{(share * 100).ToString("F2", CultureInfo.InvariantCulture)}% of pixels changed, over the {(tol * 100).ToString("F2", CultureInfo.InvariantCulture)}% tolerance";
                            problem = problem.Length > 0 ? $"{problem}; {p}" : p;
                        }
                    }
                    if (problem.Length > 0)
                    {
                        var ce = new JsonObject { ["step"] = k + 1 };
                        foreach (var (key, v) in input) ce[key] = v?.DeepClone();
                        return new DrivenOutcome("violated", false, new ProofCounterexample { Input = ce, Output = obs }, $"step {k + 1}: {problem}");
                    }
                }
            }
            return new DrivenOutcome("evidenced", true);
        }
        // A property: fixed-seed action sequences, the invariant checked after every action.
        var actions = (c.Actions as JsonArray ?? []).OfType<JsonObject>().ToList();
        bool Holds(JsonObject obs)
        {
            try { return Lsl.Holds(c.Invariant!, new EvalContext(new Env(), EnvOf(obs), null)); }
            catch { return false; }
        }
        const int sequences = 100, length = 10;
        var next = Domains.Rng(42);
        var seqs = Enumerable.Range(0, sequences).Select(_ => Enumerable.Range(0, length).Select(_ => actions[(int)Math.Floor(next() * actions.Count)]).ToList()).ToList();
        var count = 0;
        async Task<int> RunSeq(List<JsonObject> seq)
        {
            var s = await Open(kind, locator).ConfigureAwait(false);
            await using (s.ConfigureAwait(false))
            {
                for (var k = 0; k < seq.Count; k++)
                {
                    count++;
                    if (!Holds(await s.ActAsync((JsonObject)seq[k].DeepClone()).ConfigureAwait(false))) return k + 1;
                }
            }
            return 0;
        }
        try
        {
            foreach (var seq in seqs)
            {
                var failAt = await RunSeq(seq).ConfigureAwait(false);
                if (failAt == 0) continue;
                var prefix = seq.Take(failAt).ToList();
                var again = await RunSeq(prefix).ConfigureAwait(false);
                var ce = new ProofCounterexample { Input = new JsonObject { ["prefix"] = JsJson.Canonical(new JsonArray([.. prefix.Select(x => x.DeepClone())])) }, Output = null };
                return again == 0
                    ? new DrivenOutcome("violated", false, ce, "nondeterministic: the shortest failing prefix passed on replay")
                    : new DrivenOutcome("violated", false, ce);
            }
        }
        catch (Exception e)
        {
            return new DrivenOutcome("violated", false, Note: $"driver error: {e.Message}");
        }
        var bound = Math.Min(1, 3.0 / Math.Max(1, count));
        return new DrivenOutcome("evidenced", true, Note: $"{sequences} sequences, {count} actions; failure rate below {JsJson.Number(bound)} (95%)");
    }
}
