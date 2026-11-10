using System.Globalization;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Text.Json.Serialization;

namespace Lastro;

public enum ValueKind
{
    /// <summary>No value (a missing output field, or JSON null). Never in a valid domain.</summary>
    Missing = 0,
    Bool,
    Number,
    String,
}

/// <summary>
/// A runtime value: boolean, number or string, with JavaScript <c>===</c> equality.
/// Numbers are doubles, as in JSON, the TypeScript engine and the Rust prover.
/// </summary>
[JsonConverter(typeof(ValueConverter))]
public readonly struct Value : IEquatable<Value>
{
    private readonly bool _bool;
    private readonly double _number;
    private readonly string? _string;

    public ValueKind Kind { get; }

    private Value(ValueKind kind, bool b = false, double n = 0, string? s = null)
    {
        Kind = kind;
        _bool = b;
        _number = n;
        _string = s;
    }

    public static readonly Value Missing = default;

    public static Value Of(bool b) => new(ValueKind.Bool, b: b);
    public static Value Of(double n) => new(ValueKind.Number, n: n);
    public static Value Of(string s) => new(ValueKind.String, s: s);

    /// <summary>Converts a CLR primitive (bool, any integer or floating type, string, JsonElement, Value).</summary>
    public static Value From(object? o) => o switch
    {
        null => Missing,
        Value v => v,
        bool b => Of(b),
        string s => Of(s),
        double d => Of(d),
        float f => Of(f),
        decimal m => Of((double)m),
        int i => Of(i),
        long l => Of(l),
        short s => Of(s),
        byte b => Of(b),
        uint u => Of(u),
        ulong u => Of(u),
        JsonElement e => FromElement(e),
        JsonNode n => FromElement(JsonSerializer.SerializeToElement(n)),
        _ => throw new ArgumentException($"not a Lastro value: {o.GetType().Name}"),
    };

    private static Value FromElement(JsonElement e) => e.ValueKind switch
    {
        JsonValueKind.True => Of(true),
        JsonValueKind.False => Of(false),
        JsonValueKind.Number => Of(e.GetDouble()),
        JsonValueKind.String => Of(e.GetString()!),
        JsonValueKind.Null or JsonValueKind.Undefined => Missing,
        _ => throw new JsonException($"not a Lastro value: {e.GetRawText()}"),
    };

    public static implicit operator Value(bool b) => Of(b);
    public static implicit operator Value(int n) => Of(n);
    public static implicit operator Value(long n) => Of(n);
    public static implicit operator Value(double n) => Of(n);
    public static implicit operator Value(string s) => Of(s);

    public bool IsBool => Kind == ValueKind.Bool;
    public bool IsNumber => Kind == ValueKind.Number;
    public bool IsString => Kind == ValueKind.String;

    public bool AsBool => Kind == ValueKind.Bool ? _bool : throw new InvalidOperationException($"expected boolean, got {this}");
    public double AsNumber => Kind == ValueKind.Number ? _number : throw new InvalidOperationException($"expected number, got {this}");
    public long AsLong => (long)AsNumber;
    public int AsInt => (int)AsNumber;
    public string AsString => Kind == ValueKind.String ? _string! : throw new InvalidOperationException($"expected string, got {this}");

    /// <summary>The CLR value: bool, string, or a number (long when integral, double otherwise).</summary>
    public object? ToObject() => Kind switch
    {
        ValueKind.Bool => _bool,
        ValueKind.String => _string,
        ValueKind.Number => Math.Floor(_number) == _number && Math.Abs(_number) < 9e15 ? (long)_number : _number,
        _ => null,
    };

    public JsonNode? ToNode() => Kind switch
    {
        ValueKind.Bool => JsonValue.Create(_bool),
        ValueKind.Number => JsonValue.Create(_number),
        ValueKind.String => JsonValue.Create(_string),
        _ => null,
    };

    public bool Equals(Value other) => Kind == other.Kind && Kind switch
    {
        ValueKind.Bool => _bool == other._bool,
        ValueKind.Number => _number == other._number,
        ValueKind.String => string.Equals(_string, other._string, StringComparison.Ordinal),
        _ => true,
    };

    public override bool Equals(object? obj) => obj is Value v && Equals(v);

    public override int GetHashCode() => Kind switch
    {
        ValueKind.Bool => _bool.GetHashCode(),
        ValueKind.Number => _number.GetHashCode(),
        ValueKind.String => StringComparer.Ordinal.GetHashCode(_string!),
        _ => 0,
    };

    public static bool operator ==(Value a, Value b) => a.Equals(b);
    public static bool operator !=(Value a, Value b) => !a.Equals(b);

    /// <summary>As <c>JSON.stringify</c> writes it.</summary>
    public override string ToString() => Kind switch
    {
        ValueKind.Bool => _bool ? "true" : "false",
        ValueKind.Number => JsJson.Number(_number),
        ValueKind.String => JsJson.Quote(_string!),
        _ => "undefined",
    };

    /// <summary>As JavaScript's <c>String(v)</c> writes it.</summary>
    public string ToPlainString() => Kind == ValueKind.String ? _string! : ToString();
}

public sealed class ValueConverter : JsonConverter<Value>
{
    public override bool HandleNull => true;

    public override Value Read(ref Utf8JsonReader reader, Type typeToConvert, JsonSerializerOptions options) => reader.TokenType switch
    {
        JsonTokenType.True => Value.Of(true),
        JsonTokenType.False => Value.Of(false),
        JsonTokenType.Number => Value.Of(reader.GetDouble()),
        JsonTokenType.String => Value.Of(reader.GetString()!),
        JsonTokenType.Null => Value.Missing,
        _ => throw new JsonException("expected a boolean, number or string"),
    };

    public override void Write(Utf8JsonWriter writer, Value value, JsonSerializerOptions options)
    {
        switch (value.Kind)
        {
            case ValueKind.Bool:
                writer.WriteBooleanValue(value.AsBool);
                break;
            case ValueKind.Number:
                var n = value.AsNumber;
                if (Math.Floor(n) == n && Math.Abs(n) < 9e15) writer.WriteNumberValue((long)n);
                else writer.WriteRawValue(JsJson.Number(n));
                break;
            case ValueKind.String:
                writer.WriteStringValue(value.AsString);
                break;
            default:
                writer.WriteNullValue();
                break;
        }
    }
}

/// <summary>Named values: a program's input or output.</summary>
public sealed class Env : Dictionary<string, Value>
{
    public Env() : base(StringComparer.Ordinal) { }

    public Env(IEnumerable<KeyValuePair<string, Value>> items) : base(StringComparer.Ordinal)
    {
        foreach (var kv in items) this[kv.Key] = kv.Value;
    }

    /// <summary>Builds an Env from CLR primitives, e.g. <c>Env.From(new() { ["sends"] = "email" })</c>.</summary>
    public static Env From(IEnumerable<KeyValuePair<string, object?>> items) =>
        new(items.Select(kv => new KeyValuePair<string, Value>(kv.Key, Value.From(kv.Value))));

    public Value Get(string key) => TryGetValue(key, out var v) ? v : Value.Missing;

    public Env Copy() => new(this);

    /// <summary>As <c>JSON.stringify</c> writes it (insertion order).</summary>
    public override string ToString() => JsJson.Stringify(LastroJson.ToNode(this));

    public Dictionary<string, object?> ToObjects() => this.ToDictionary(kv => kv.Key, kv => kv.Value.ToObject(), StringComparer.Ordinal);
}

internal static class Inv
{
    public static string F(double d, int digits) => d.ToString("F" + digits, CultureInfo.InvariantCulture);
}
