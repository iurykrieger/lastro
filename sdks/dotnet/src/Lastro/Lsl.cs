using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Text.Json.Serialization;

namespace Lastro;

/// <summary>
/// An LSL (Lastro Spec Language) expression: a JSON AST that is interpreted, never <c>eval</c>ed.
/// <c>{"var":"in.x"}</c> | <c>{"const":1}</c> | <c>{"op":"and","args":[...]}</c> | <c>{"call":"def","args":[...]}</c>.
/// Variables are namespaced: <c>in.&lt;input&gt;</c> and <c>out.&lt;output&gt;</c>; def parameters are bare names.
/// </summary>
[JsonConverter(typeof(ExprConverter))]
public abstract record Expr;

public sealed record VarExpr(string Var) : Expr;

public sealed record ConstExpr(Value Const) : Expr;

public sealed record OpExpr(string Op, IReadOnlyList<Expr> Args) : Expr;

public sealed record CallExpr(string Call, IReadOnlyList<Expr> Args) : Expr;

public sealed class ExprConverter : JsonConverter<Expr>
{
    public override Expr Read(ref Utf8JsonReader reader, Type typeToConvert, JsonSerializerOptions options)
    {
        using var doc = JsonDocument.ParseValue(ref reader);
        return Lsl.FromElement(doc.RootElement);
    }

    public override void Write(Utf8JsonWriter writer, Expr value, JsonSerializerOptions options)
    {
        writer.WriteStartObject();
        switch (value)
        {
            case VarExpr v:
                writer.WriteString("var", v.Var);
                break;
            case ConstExpr c:
                writer.WritePropertyName("const");
                JsonSerializer.Serialize(writer, c.Const, options);
                break;
            case OpExpr o:
                writer.WriteString("op", o.Op);
                writer.WritePropertyName("args");
                JsonSerializer.Serialize(writer, o.Args, options);
                break;
            case CallExpr c:
                writer.WriteString("call", c.Call);
                writer.WritePropertyName("args");
                JsonSerializer.Serialize(writer, c.Args, options);
                break;
        }
        writer.WriteEndObject();
    }
}

/// <summary>A named definition shared by clauses (referenced with <c>{"call": name}</c>).</summary>
public sealed class Def
{
    public List<string> Params { get; set; } = [];
    public string? Description { get; set; }
    public Expr Body { get; set; } = L.C(true);
}

/// <summary>Ergonomic LSL builders, as in the TypeScript SDK's <c>L</c>.</summary>
public static class L
{
    public static Expr In(string name) => new VarExpr($"in.{name}");
    public static Expr Out(string name) => new VarExpr($"out.{name}");
    public static Expr V(string name) => new VarExpr(name);
    public static Expr C(Value value) => new ConstExpr(value);
    public static Expr And(params Expr[] args) => new OpExpr("and", args);
    public static Expr Or(params Expr[] args) => new OpExpr("or", args);
    public static Expr Not(Expr a) => new OpExpr("not", [a]);
    public static Expr Implies(Expr a, Expr b) => new OpExpr("implies", [a, b]);
    public static Expr Iff(Expr a, Expr b) => new OpExpr("iff", [a, b]);
    public static Expr Eq(Expr a, Value b) => new OpExpr("eq", [a, C(b)]);
    public static Expr Eq(Expr a, Expr b) => new OpExpr("eq", [a, b]);
    public static Expr Neq(Expr a, Value b) => new OpExpr("neq", [a, C(b)]);
    public static Expr Neq(Expr a, Expr b) => new OpExpr("neq", [a, b]);
    public static Expr Lt(Expr a, Value b) => new OpExpr("lt", [a, C(b)]);
    public static Expr Lt(Expr a, Expr b) => new OpExpr("lt", [a, b]);
    public static Expr Le(Expr a, Value b) => new OpExpr("le", [a, C(b)]);
    public static Expr Le(Expr a, Expr b) => new OpExpr("le", [a, b]);
    public static Expr Gt(Expr a, Value b) => new OpExpr("gt", [a, C(b)]);
    public static Expr Gt(Expr a, Expr b) => new OpExpr("gt", [a, b]);
    public static Expr Ge(Expr a, Value b) => new OpExpr("ge", [a, C(b)]);
    public static Expr Ge(Expr a, Expr b) => new OpExpr("ge", [a, b]);
    public static Expr Add(params Expr[] args) => new OpExpr("add", args);
    public static Expr Sub(Expr a, Expr b) => new OpExpr("sub", [a, b]);
    public static Expr Mul(params Expr[] args) => new OpExpr("mul", args);
    public static Expr Ite(Expr c, Expr a, Expr b) => new OpExpr("ite", [c, a, b]);
    public static Expr OneOf(Expr a, params Value[] values) => new OpExpr("in", [a, .. values.Select(C)]);
    public static Expr Call(string name, params Expr[] args) => new CallExpr(name, args);
}

public sealed class LslException(string message) : Exception(message);

/// <summary>Context of one evaluation: the run's input and output, and the model's defs.</summary>
public sealed record EvalContext(Env Input, Env Output, IReadOnlyDictionary<string, Def>? Defs);

/// <summary>Evaluation, validation, printing and SMT-LIB export of LSL expressions.</summary>
public static class Lsl
{
    private static readonly Dictionary<string, (int Min, int Max)> Arity = new(StringComparer.Ordinal)
    {
        ["and"] = (1, int.MaxValue),
        ["or"] = (1, int.MaxValue),
        ["not"] = (1, 1),
        ["implies"] = (2, 2),
        ["iff"] = (2, 2),
        ["eq"] = (2, 2),
        ["neq"] = (2, 2),
        ["lt"] = (2, 2),
        ["le"] = (2, 2),
        ["gt"] = (2, 2),
        ["ge"] = (2, 2),
        ["add"] = (2, int.MaxValue),
        ["sub"] = (2, 2),
        ["mul"] = (2, int.MaxValue),
        ["ite"] = (3, 3),
        ["in"] = (2, int.MaxValue),
    };

    public static IReadOnlyCollection<string> Ops => Arity.Keys;

    // -----------------------------------------------------------------------
    // Parsing
    // -----------------------------------------------------------------------

    public static Expr Parse(string json)
    {
        using var doc = JsonDocument.Parse(json);
        return FromElement(doc.RootElement);
    }

    internal static Expr FromElement(JsonElement e)
    {
        if (e.ValueKind != JsonValueKind.Object) throw new JsonException("LSL: an expression must be a JSON object");
        if (e.TryGetProperty("const", out var c))
        {
            var v = Value.From(c);
            if (v.Kind == ValueKind.Missing) throw new JsonException("LSL: bad const");
            return new ConstExpr(v);
        }
        if (e.TryGetProperty("var", out var name)) return new VarExpr(name.GetString() ?? throw new JsonException("LSL: var must be a string"));
        var args = e.TryGetProperty("args", out var a) && a.ValueKind == JsonValueKind.Array ? a.EnumerateArray().Select(FromElement).ToList() : [];
        if (e.TryGetProperty("call", out var call)) return new CallExpr(call.GetString() ?? throw new JsonException("LSL: call must be a string"), args);
        if (e.TryGetProperty("op", out var op)) return new OpExpr(op.GetString() ?? throw new JsonException("LSL: op must be a string"), args);
        throw new JsonException("LSL: expected one of var/const/op/call");
    }

    // -----------------------------------------------------------------------
    // Evaluation
    // -----------------------------------------------------------------------

    public static Value Evaluate(Expr e, EvalContext ctx) => Eval(e, ctx, null, 0);

    public static bool Holds(Expr e, EvalContext ctx) => Bool(Evaluate(e, ctx));

    private static Value Eval(Expr e, EvalContext ctx, Env? locals, int depth)
    {
        if (depth > 64) throw new LslException("LSL: max recursion depth exceeded (recursive def?)");
        switch (e)
        {
            case ConstExpr c:
                return c.Const;
            case VarExpr v:
                return Lookup(v.Var, ctx, locals);
            case CallExpr call:
            {
                if (ctx.Defs is null || !ctx.Defs.TryGetValue(call.Call, out var def)) throw new LslException($"LSL: unknown def '{call.Call}'");
                if (def.Params.Count != call.Args.Count)
                    throw new LslException($"LSL: def '{call.Call}' expects {def.Params.Count} args, got {call.Args.Count}");
                var bound = new Env();
                for (var i = 0; i < def.Params.Count; i++) bound[def.Params[i]] = Eval(call.Args[i], ctx, locals, depth + 1);
                return Eval(def.Body, ctx, bound, depth + 1);
            }
            case OpExpr o:
            {
                Value Ev(Expr x) => Eval(x, ctx, locals, depth + 1);
                var a = o.Args;
                return o.Op switch
                {
                    "and" => a.All(x => Bool(Ev(x))),
                    "or" => a.Any(x => Bool(Ev(x))),
                    "not" => !Bool(Ev(a[0])),
                    "implies" => !Bool(Ev(a[0])) || Bool(Ev(a[1])),
                    "iff" => Bool(Ev(a[0])) == Bool(Ev(a[1])),
                    "eq" => Ev(a[0]) == Ev(a[1]),
                    "neq" => Ev(a[0]) != Ev(a[1]),
                    "lt" => Num(Ev(a[0])) < Num(Ev(a[1])),
                    "le" => Num(Ev(a[0])) <= Num(Ev(a[1])),
                    "gt" => Num(Ev(a[0])) > Num(Ev(a[1])),
                    "ge" => Num(Ev(a[0])) >= Num(Ev(a[1])),
                    "add" => a.Aggregate(0d, (s, x) => s + Num(Ev(x))),
                    "sub" => Num(Ev(a[0])) - Num(Ev(a[1])),
                    "mul" => a.Aggregate(1d, (s, x) => s * Num(Ev(x))),
                    "ite" => Bool(Ev(a[0])) ? Ev(a[1]) : Ev(a[2]),
                    "in" => InSet(Ev(a[0]), a.Skip(1).Select(Ev)),
                    _ => throw new LslException($"LSL: unknown op '{o.Op}'"),
                };
            }
            default:
                throw new LslException("LSL: unknown expression");
        }
    }

    private static bool InSet(Value v, IEnumerable<Value> set) => set.Any(x => x == v);

    private static Value Lookup(string name, EvalContext ctx, Env? locals)
    {
        if (locals is not null && locals.TryGetValue(name, out var local)) return local;
        var (ns, field) = SplitVar(name);
        var src = ns == "in" ? ctx.Input : ns == "out" ? ctx.Output : null;
        if (src is null || field is null || !src.TryGetValue(field, out var v)) throw new LslException($"LSL: unbound variable '{name}'");
        return v;
    }

    private static bool Bool(Value v) => v.IsBool ? v.AsBool : throw new LslException($"LSL: expected boolean, got {v}");

    private static double Num(Value v) => v.IsNumber ? v.AsNumber : throw new LslException($"LSL: expected number, got {v}");

    private static (string Ns, string? Field) SplitVar(string name)
    {
        var i = name.IndexOf('.');
        return i < 0 ? (name, null) : (name[..i], name[(i + 1)..]);
    }

    // -----------------------------------------------------------------------
    // Static validation
    // -----------------------------------------------------------------------

    /// <summary>Problems with an expression, given as raw JSON (for example an LLM answer). Empty means well-formed.</summary>
    public static List<string> Validate(JsonNode? e, Model model, IReadOnlyList<string>? locals = null, string path = "expr")
    {
        locals ??= [];
        var errs = new List<string>();
        if (e is not JsonObject o) return [$"{path}: not an object"];
        if (o.ContainsKey("const"))
        {
            var kind = o["const"]?.GetValueKind();
            if (kind is not (JsonValueKind.True or JsonValueKind.False or JsonValueKind.Number or JsonValueKind.String)) errs.Add($"{path}: bad const");
        }
        else if (o.ContainsKey("var"))
        {
            var name = NodeString(o["var"]);
            if (!locals.Contains(name))
            {
                var (ns, field) = SplitVar(name);
                var table = ns == "in" ? model.Inputs : ns == "out" ? model.Outputs : null;
                if (table is null || field is null || !table.ContainsKey(field))
                    errs.Add($"{path}: unknown variable '{name}' (use in.<input> or out.<output>)");
            }
        }
        else if (o.ContainsKey("call"))
        {
            var name = NodeString(o["call"]);
            Def? def = null;
            if (model.Defs is null || !model.Defs.TryGetValue(name, out def)) errs.Add($"{path}: unknown def '{name}'");
            var args = o["args"] as JsonArray ?? [];
            if (def is not null && def.Params.Count != args.Count) errs.Add($"{path}: def '{name}' arity mismatch");
            for (var i = 0; i < args.Count; i++) errs.AddRange(Validate(args[i], model, locals, $"{path}.args[{i}]"));
        }
        else if (o.ContainsKey("op"))
        {
            var op = NodeString(o["op"]);
            var args = o["args"] as JsonArray;
            if (!Arity.TryGetValue(op, out var ar)) errs.Add($"{path}: unknown op '{op}'");
            else if (args is null) errs.Add($"{path}: missing args");
            else
            {
                if (args.Count < ar.Min || args.Count > ar.Max) errs.Add($"{path}: op '{op}' arity {args.Count}");
                for (var i = 0; i < args.Count; i++) errs.AddRange(Validate(args[i], model, locals, $"{path}.args[{i}]"));
            }
        }
        else errs.Add($"{path}: expected one of var/const/op/call");
        return errs;
    }

    /// <summary>Problems with a typed expression. Empty means well-formed for the model.</summary>
    public static List<string> Validate(Expr e, Model model, IReadOnlyList<string>? locals = null, string path = "expr") =>
        Validate(LastroJson.ToNode(e), model, locals, path);

    private static string NodeString(JsonNode? n) =>
        n is JsonValue v && v.GetValueKind() == JsonValueKind.String ? v.GetValue<string>() : n?.ToJsonString() ?? "undefined";

    public static HashSet<string> FreeVars(Expr e, HashSet<string>? acc = null)
    {
        acc ??= new HashSet<string>(StringComparer.Ordinal);
        switch (e)
        {
            case VarExpr v:
                acc.Add(v.Var);
                break;
            case OpExpr o:
                foreach (var x in o.Args) FreeVars(x, acc);
                break;
            case CallExpr c:
                foreach (var x in c.Args) FreeVars(x, acc);
                break;
        }
        return acc;
    }

    // -----------------------------------------------------------------------
    // Pretty printing
    // -----------------------------------------------------------------------

    private static readonly Dictionary<string, string> Infix = new(StringComparer.Ordinal)
    {
        ["and"] = "∧",
        ["or"] = "∨",
        ["implies"] = "⇒",
        ["iff"] = "⇔",
        ["eq"] = "=",
        ["neq"] = "≠",
        ["lt"] = "<",
        ["le"] = "≤",
        ["gt"] = ">",
        ["ge"] = "≥",
        ["add"] = "+",
        ["sub"] = "-",
        ["mul"] = "·",
    };

    /// <summary>Human-readable notation, identical to the TypeScript SDK's <c>show</c>.</summary>
    public static string Show(Expr e)
    {
        switch (e)
        {
            case ConstExpr c:
                return c.Const.ToString();
            case VarExpr v:
                return v.Var;
            case CallExpr call:
                return $"{call.Call}({string.Join(", ", call.Args.Select(Show))})";
            case OpExpr o:
                var a = o.Args;
                if (o.Op == "not") return $"¬{Wrap(a[0])}";
                if (o.Op == "ite") return $"if {Show(a[0])} then {Show(a[1])} else {Show(a[2])}";
                if (o.Op == "in") return $"{Show(a[0])} ∈ {{{string.Join(", ", a.Skip(1).Select(Show))}}}";
                return string.Join($" {(Infix.TryGetValue(o.Op, out var s) ? s : "undefined")} ", a.Select(Wrap));
            default:
                return "?";
        }
    }

    private static string Wrap(Expr e) => e is OpExpr o && o.Op != "not" ? $"({Show(e)})" : Show(e);

    // -----------------------------------------------------------------------
    // SMT-LIB2 export
    // -----------------------------------------------------------------------

    /// <summary>
    /// Compiles <c>requires ⇒ ensures</c> into an SMT-LIB2 script asserting its negation.
    /// <c>unsat</c> means the clause holds for every input/output allowed by the domains
    /// and by <paramref name="implementation"/> (an optional LSL relation).
    /// </summary>
    public static string ToSmtLib(Model model, Expr? requires, Expr ensures, Expr? implementation = null)
    {
        var lines = new List<string> { "(set-logic ALL)" };
        var enums = new Dictionary<string, List<string>>(StringComparer.Ordinal);
        static string Sym(string name) => $"|{name}|";
        void Declare(string prefix, Dictionary<string, Domain> table)
        {
            foreach (var (field, d) in table)
            {
                var name = $"{prefix}.{field}";
                switch (d.Type)
                {
                    case "bool":
                        lines.Add($"(declare-const {Sym(name)} Bool)");
                        break;
                    case "int":
                        lines.Add($"(declare-const {Sym(name)} Int)");
                        if (d.Min is { } min) lines.Add($"(assert (>= {Sym(name)} {SmtInt(min)}))");
                        if (d.Max is { } max) lines.Add($"(assert (<= {Sym(name)} {SmtInt(max)}))");
                        break;
                    case "enum":
                        enums[name] = (d.Values ?? []).Select(v => v.ToPlainString()).ToList();
                        lines.Add($"(declare-const {Sym(name)} Int)");
                        lines.Add($"(assert (and (>= {Sym(name)} 0) (< {Sym(name)} {(d.Values ?? []).Count})))");
                        break;
                    default:
                        lines.Add($"(declare-const {Sym(name)} String)");
                        break;
                }
            }
        }
        Declare("in", model.Inputs);
        Declare("out", model.Outputs);

        (string, string)? EnumIndex(Expr varExpr, Expr other)
        {
            if (varExpr is VarExpr v && enums.TryGetValue(v.Var, out var values) && other is ConstExpr c)
                return (Sym(v.Var), values.IndexOf(c.Const.ToPlainString()).ToString(System.Globalization.CultureInfo.InvariantCulture));
            return null;
        }

        string Smt(Expr e)
        {
            switch (e)
            {
                case ConstExpr c:
                    return c.Const.Kind switch
                    {
                        ValueKind.Bool => c.Const.AsBool ? "true" : "false",
                        ValueKind.Number => SmtInt(c.Const.AsNumber),
                        _ => c.Const.ToString(),
                    };
                case VarExpr v:
                    return Sym(v.Var);
                case CallExpr call:
                    return $"({Sym(call.Call)} {string.Join(" ", call.Args.Select(Smt))})";
                case OpExpr o:
                {
                    var a = o.Args;
                    if (o.Op is "eq" or "neq")
                    {
                        var pair = EnumIndex(a[0], a[1]) ?? EnumIndex(a[1], a[0]);
                        var body = pair is { } p ? $"(= {p.Item1} {p.Item2})" : $"(= {Smt(a[0])} {Smt(a[1])})";
                        return o.Op == "eq" ? body : $"(not {body})";
                    }
                    if (o.Op == "in") return $"(or {string.Join(" ", a.Skip(1).Select(x => Smt(new OpExpr("eq", [a[0], x]))))})";
                    var head = o.Op switch
                    {
                        "implies" => "=>",
                        "iff" => "=",
                        "lt" => "<",
                        "le" => "<=",
                        "gt" => ">",
                        "ge" => ">=",
                        "add" => "+",
                        "sub" => "-",
                        "mul" => "*",
                        _ => o.Op,
                    };
                    return $"({head} {string.Join(" ", a.Select(Smt))})";
                }
                default:
                    return "?";
            }
        }

        foreach (var (name, def) in model.Defs ?? [])
        {
            // Def params are untyped in v0; only Bool params are exported.
            var ps = string.Join(" ", def.Params.Select(p => $"({Sym(p)} Bool)"));
            lines.Add($"(define-fun {Sym(name)} ({ps}) Bool {Smt(def.Body)})");
        }
        if (implementation is not null) lines.Add($"(assert {Smt(implementation)})");
        var goal = requires is not null ? $"(=> {Smt(requires)} {Smt(ensures)})" : Smt(ensures);
        lines.Add($"(assert (not {goal}))");
        lines.Add("(check-sat)");
        lines.Add("(get-model)");
        return string.Join("\n", lines);
    }

    public static string ToSmtLib(Model model, Clause clause, Expr? implementation = null) => ToSmtLib(model, clause.Requires, clause.Ensures, implementation);

    private static string SmtInt(double n) => n < 0 ? $"(- {JsJson.Number(-n)})" : JsJson.Number(n);

    /// <summary>"requires ⇒ ensures" in LSL notation, with the given arrow.</summary>
    public static string Formal(Expr? requires, Expr ensures, string arrow)
    {
        var sb = new StringBuilder();
        if (requires is not null) sb.Append(Show(requires)).Append(arrow);
        sb.Append(Show(ensures));
        return sb.ToString();
    }
}
