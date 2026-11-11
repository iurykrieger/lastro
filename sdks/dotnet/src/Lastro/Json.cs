using System.Globalization;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Text.Json.Serialization;

namespace Lastro;

/// <summary>
/// JSON plumbing shared by every Lastro artifact. The wire format is the one the
/// TypeScript SDK and the <c>lastro</c> CLI use (camelCase, kebab-case enums, absent
/// instead of null), so a <c>.lastro/</c> workspace written by one SDK is read by the other.
/// </summary>
public static class LastroJson
{
    /// <summary>Serializer options for every Lastro type.</summary>
    public static readonly JsonSerializerOptions Options = Create(indented: false);

    /// <summary>Same as <see cref="Options"/>, indented with two spaces (the CLI's file format).</summary>
    public static readonly JsonSerializerOptions Indented = Create(indented: true);

    private static JsonSerializerOptions Create(bool indented)
    {
        var o = new JsonSerializerOptions
        {
            PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
            DictionaryKeyPolicy = null,
            DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
            WriteIndented = indented,
            Encoder = System.Text.Encodings.Web.JavaScriptEncoder.UnsafeRelaxedJsonEscaping,
            NumberHandling = JsonNumberHandling.Strict,
        };
        o.MakeReadOnly(populateMissingResolver: true);
        return o;
    }

    public static string Serialize<T>(T value, bool indented = false) =>
        JsonSerializer.Serialize(value, indented ? Indented : Options);

    public static T Deserialize<T>(string json) =>
        JsonSerializer.Deserialize<T>(json, Options) ?? throw new JsonException($"null {typeof(T).Name}");

    public static JsonNode? ToNode<T>(T value) => JsonSerializer.SerializeToNode(value, Options);

    public static T FromNode<T>(JsonNode? node) =>
        node is null ? throw new JsonException($"null {typeof(T).Name}") : node.Deserialize<T>(Options) ?? throw new JsonException($"null {typeof(T).Name}");

    /// <summary>Deep copy through JSON (the equivalent of <c>structuredClone</c>).</summary>
    public static T Clone<T>(T value) => Deserialize<T>(Serialize(value));
}

/// <summary>
/// <c>JSON.stringify</c>, byte for byte: same string escaping, same number formatting,
/// same key order. Used wherever the TypeScript SDK hashes or prints JSON, so both
/// SDKs agree on hashes, handoff ids and messages.
/// </summary>
public static class JsJson
{
    /// <summary>Compact (or indented with <paramref name="indent"/> spaces) stringify, keeping key order.</summary>
    public static string Stringify(JsonNode? node, int indent = 0)
    {
        var sb = new StringBuilder();
        Write(sb, node, indent, 0, sortKeys: false);
        return sb.ToString();
    }

    public static string Stringify<T>(T value, int indent = 0) => Stringify(LastroJson.ToNode(value), indent);

    /// <summary>Canonical JSON: keys sorted by UTF-16 code unit, nulls dropped from objects (the TS <c>canonicalJson</c>).</summary>
    public static string Canonical(JsonNode? node)
    {
        var sb = new StringBuilder();
        Write(sb, node, 0, 0, sortKeys: true);
        return sb.ToString();
    }

    private static void Write(StringBuilder sb, JsonNode? node, int indent, int depth, bool sortKeys)
    {
        switch (node)
        {
            case null:
                sb.Append("null");
                return;
            case JsonObject obj:
            {
                IEnumerable<KeyValuePair<string, JsonNode?>> props = obj;
                if (sortKeys) props = props.Where(p => p.Value is not null).OrderBy(p => p.Key, StringComparer.Ordinal);
                var list = props.ToList();
                if (list.Count == 0)
                {
                    sb.Append("{}");
                    return;
                }
                sb.Append('{');
                for (var i = 0; i < list.Count; i++)
                {
                    if (i > 0) sb.Append(',');
                    NewLine(sb, indent, depth + 1);
                    Quote(sb, list[i].Key);
                    sb.Append(':');
                    if (indent > 0) sb.Append(' ');
                    Write(sb, list[i].Value, indent, depth + 1, sortKeys);
                }
                NewLine(sb, indent, depth);
                sb.Append('}');
                return;
            }
            case JsonArray arr:
            {
                if (arr.Count == 0)
                {
                    sb.Append("[]");
                    return;
                }
                sb.Append('[');
                for (var i = 0; i < arr.Count; i++)
                {
                    if (i > 0) sb.Append(',');
                    NewLine(sb, indent, depth + 1);
                    Write(sb, arr[i], indent, depth + 1, sortKeys);
                }
                NewLine(sb, indent, depth);
                sb.Append(']');
                return;
            }
            case JsonValue v:
                switch (v.GetValueKind())
                {
                    case JsonValueKind.String:
                        Quote(sb, v.GetValue<string>());
                        return;
                    case JsonValueKind.Number:
                        sb.Append(Number(double.Parse(v.ToJsonString(), NumberStyles.Float, CultureInfo.InvariantCulture)));
                        return;
                    case JsonValueKind.True:
                        sb.Append("true");
                        return;
                    case JsonValueKind.False:
                        sb.Append("false");
                        return;
                    default:
                        sb.Append("null");
                        return;
                }
        }
    }

    private static void NewLine(StringBuilder sb, int indent, int depth)
    {
        if (indent <= 0) return;
        sb.Append('\n').Append(' ', indent * depth);
    }

    /// <summary>A JavaScript string literal as JSON.stringify writes it.</summary>
    public static string Quote(string s)
    {
        var sb = new StringBuilder(s.Length + 2);
        Quote(sb, s);
        return sb.ToString();
    }

    private static void Quote(StringBuilder sb, string s)
    {
        sb.Append('"');
        for (var i = 0; i < s.Length; i++)
        {
            var c = s[i];
            switch (c)
            {
                case '"': sb.Append("\\\""); break;
                case '\\': sb.Append("\\\\"); break;
                case '\b': sb.Append("\\b"); break;
                case '\f': sb.Append("\\f"); break;
                case '\n': sb.Append("\\n"); break;
                case '\r': sb.Append("\\r"); break;
                case '\t': sb.Append("\\t"); break;
                default:
                    if (c < 0x20) sb.Append("\\u").Append(((int)c).ToString("x4", CultureInfo.InvariantCulture));
                    else if (char.IsHighSurrogate(c) && i + 1 < s.Length && char.IsLowSurrogate(s[i + 1]))
                    {
                        sb.Append(c).Append(s[i + 1]);
                        i++;
                    }
                    else if (char.IsSurrogate(c)) sb.Append("\\u").Append(((int)c).ToString("x4", CultureInfo.InvariantCulture));
                    else sb.Append(c);
                    break;
            }
        }
        sb.Append('"');
    }

    /// <summary>A number as JavaScript's <c>String(n)</c> writes it.</summary>
    public static string Number(double n)
    {
        if (double.IsNaN(n) || double.IsInfinity(n)) return "null";
        if (n == 0) return "0";
        if (Math.Floor(n) == n && Math.Abs(n) < 1e21) return n.ToString("F0", CultureInfo.InvariantCulture);
        var r = n.ToString("R", CultureInfo.InvariantCulture);
        var e = r.IndexOf('E');
        if (e < 0)
        {
            // .NET writes 1E-07 for small numbers; JS switches to exponent below 1e-6.
            return r;
        }
        var mantissa = r[..e];
        var exp = int.Parse(r[(e + 1)..], CultureInfo.InvariantCulture);
        if (exp >= -6 && exp < 21) return n.ToString("0.#############################", CultureInfo.InvariantCulture);
        return $"{mantissa}e{(exp > 0 ? "+" : "-")}{Math.Abs(exp)}";
    }
}

/// <summary>Content hashes over canonical JSON (the TS <c>hashOf</c>).</summary>
public static class Canonical
{
    public static string Json<T>(T value) => JsJson.Canonical(LastroJson.ToNode(value));

    public static string HashOf<T>(T value) => "sha256:" + Sha256Hex(Json(value));

    public static string Sha256Hex(string text) => Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(text))).ToLowerInvariant();
}

/// <summary>Maps PascalCase enum members to the kebab-case strings of the wire format.</summary>
public sealed class KebabEnumConverter<T> : JsonConverter<T> where T : struct, Enum
{
    private static readonly Dictionary<T, string> ToWire = Enum.GetValues<T>().ToDictionary(v => v, v => Kebab(v.ToString()));
    private static readonly Dictionary<string, T> FromWire = ToWire.ToDictionary(kv => kv.Value, kv => kv.Key);

    public static string Kebab(string name)
    {
        var sb = new StringBuilder();
        for (var i = 0; i < name.Length; i++)
        {
            if (char.IsUpper(name[i]) && i > 0) sb.Append('-');
            sb.Append(char.ToLowerInvariant(name[i]));
        }
        return sb.ToString();
    }

    public static string Wire(T value) => ToWire[value];

    public static T Parse(string s) =>
        FromWire.TryGetValue(s, out var v) ? v : throw new JsonException($"unknown {typeof(T).Name} '{s}' (expected one of {string.Join(", ", FromWire.Keys)})");

    public static bool TryParse(string s, out T value) => FromWire.TryGetValue(s, out value);

    public override T Read(ref Utf8JsonReader reader, Type typeToConvert, JsonSerializerOptions options) => Parse(reader.GetString() ?? "");

    public override void Write(Utf8JsonWriter writer, T value, JsonSerializerOptions options) => writer.WriteStringValue(ToWire[value]);

    public override T ReadAsPropertyName(ref Utf8JsonReader reader, Type typeToConvert, JsonSerializerOptions options) => Parse(reader.GetString() ?? "");

    public override void WriteAsPropertyName(Utf8JsonWriter writer, T value, JsonSerializerOptions options) => writer.WritePropertyName(ToWire[value]);
}

/// <summary>Wire names of enum values ("proved-under-assumptions", "not-run", ...).</summary>
public static class Wire
{
    public static string Of<T>(T value) where T : struct, Enum => KebabEnumConverter<T>.Wire(value);
}
