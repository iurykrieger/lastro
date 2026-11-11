namespace Lastro;

/// <summary>
/// A value domain for a model input or output: <c>bool</c>, <c>enum</c>, <c>int</c> (optionally bounded)
/// or <c>string</c>. A model whose inputs are all finite can be verified by exhaustive enumeration,
/// which is a complete proof over the declared domain.
/// </summary>
public sealed class Domain
{
    /// <summary>"bool", "enum", "int" or "string".</summary>
    public string Type { get; set; } = "bool";
    public List<Value>? Values { get; set; }
    public double? Min { get; set; }
    public double? Max { get; set; }
    public List<string>? Examples { get; set; }
}

/// <summary>Domain builders, as in the TypeScript SDK (<c>D.bool()</c>, <c>D.enum(...)</c>...).</summary>
public static class D
{
    public static Domain Bool() => new() { Type = "bool" };
    public static Domain Enum(params Value[] values) => new() { Type = "enum", Values = [.. values] };
    public static Domain Int(double? min = null, double? max = null) => new() { Type = "int", Min = min, Max = max };
    public static Domain String(params string[] examples) => new() { Type = "string", Examples = [.. examples] };
}

/// <summary>Enumeration, sampling and membership over domains. Deterministic: same seed, same samples.</summary>
public static class Domains
{
    public static double Size(Domain d) => d.Type switch
    {
        "bool" => 2,
        "enum" => d.Values?.Count ?? 0,
        "int" => d.Min is { } min && d.Max is { } max ? Math.Max(0, max - min + 1) : double.PositiveInfinity,
        _ => double.PositiveInfinity,
    };

    public static double SpaceSize(IReadOnlyDictionary<string, Domain> table) => table.Values.Aggregate(1d, (n, d) => n * Size(d));

    public static IEnumerable<Value> Enumerate(Domain d)
    {
        switch (d.Type)
        {
            case "bool":
                yield return false;
                yield return true;
                yield break;
            case "enum":
                foreach (var v in d.Values ?? []) yield return v;
                yield break;
            case "int":
                if (d.Min is not { } min || d.Max is not { } max) throw new InvalidOperationException("cannot enumerate unbounded int");
                for (var i = min; i <= max; i++) yield return i;
                yield break;
            default:
                throw new InvalidOperationException($"cannot enumerate {d.Type} domain");
        }
    }

    /// <summary>Cartesian product of finite domains, keys in ordinal order, last key varying fastest.</summary>
    public static IEnumerable<Env> EnumerateSpace(IReadOnlyDictionary<string, Domain> table)
    {
        var keys = table.Keys.OrderBy(k => k, StringComparer.Ordinal).ToArray();
        var values = keys.Select(k => Enumerate(table[k]).ToArray()).ToArray();
        if (values.Any(v => v.Length == 0)) yield break;
        var idx = new int[keys.Length];
        while (true)
        {
            var env = new Env();
            for (var i = 0; i < keys.Length; i++) env[keys[i]] = values[i][idx[i]];
            yield return env;
            var j = keys.Length - 1;
            while (j >= 0)
            {
                idx[j]++;
                if (idx[j] < values[j].Length) break;
                idx[j] = 0;
                j--;
            }
            if (j < 0) yield break;
        }
    }

    /// <summary>Deterministic PRNG (mulberry32), bit-identical to the TypeScript engine.</summary>
    public static Func<double> Rng(long seed)
    {
        var a = unchecked((uint)seed);
        return () =>
        {
            unchecked
            {
                a += 0x6d2b79f5;
                var t = a;
                t = (t ^ (t >> 15)) * (t | 1);
                t ^= t + (t ^ (t >> 7)) * (t | 61);
                return (t ^ (t >> 14)) / 4294967296.0;
            }
        };
    }

    public static Value Sample(Domain d, Func<double> next)
    {
        T Pick<T>(IReadOnlyList<T> xs) => xs[(int)Math.Floor(next() * xs.Count)];
        switch (d.Type)
        {
            case "bool":
                return next() < 0.5;
            case "enum":
                return Pick(d.Values ?? []);
            case "int":
            {
                var min = d.Min ?? -1_000_000;
                var max = d.Max ?? 1_000_000;
                // Bias toward boundaries, where bugs live.
                var r = next();
                if (r < 0.1) return min;
                if (r < 0.2) return max;
                if (r < 0.3 && min <= 0 && max >= 0) return 0;
                return min + Math.Floor(next() * (max - min + 1));
            }
            default:
            {
                var ex = d.Examples ?? [];
                if (ex.Count > 0 && next() < 0.7) return Pick(ex);
                var len = (int)Math.Floor(next() * 12);
                var chars = new char[len];
                for (var i = 0; i < len; i++) chars[i] = (char)(97 + (int)Math.Floor(next() * 26));
                return new string(chars);
            }
        }
    }

    public static Env SampleSpace(IReadOnlyDictionary<string, Domain> table, Func<double> next)
    {
        var env = new Env();
        foreach (var k in table.Keys.OrderBy(k => k, StringComparer.Ordinal)) env[k] = Sample(table[k], next);
        return env;
    }

    public static bool Contains(Domain d, Value v) => d.Type switch
    {
        "bool" => v.IsBool,
        "enum" => (d.Values ?? []).Contains(v),
        "int" => v.IsNumber && Math.Floor(v.AsNumber) == v.AsNumber && !double.IsInfinity(v.AsNumber)
                 && (d.Min is not { } min || v.AsNumber >= min) && (d.Max is not { } max || v.AsNumber <= max),
        "string" => v.IsString,
        _ => false,
    };
}
