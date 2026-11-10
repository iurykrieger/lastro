package lastro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// LSL, the Lastro Spec Language: a JSON expression AST shared by every engine.
// It is never eval'ed; this file interprets it with the same semantics as the
// TS SDK (sdks/typescript/src/spec/expr.ts) and the Rust prover.
//
// Variables are namespaced: in.<name> for model inputs, out.<name> for
// outputs. Inside a def body, parameters are referenced by bare name.

// Expr is one LSL node: {"var"}, {"const"}, {"op","args"} or {"call","args"}.
type Expr struct {
	Kind  string // "var" | "const" | "op" | "call"
	Name  string // variable name, operator or def name
	Value any    // the constant, for "const"
	Args  []Expr
	// bad is set when the JSON did not describe a valid node; ValidateExpr reports it.
	bad     string
	argsBad bool
}

// The operators and their arity [min, max] (max -1 = unbounded).
var opArity = map[string][2]int{
	"and": {1, -1}, "or": {1, -1}, "not": {1, 1}, "implies": {2, 2}, "iff": {2, 2},
	"eq": {2, 2}, "neq": {2, 2}, "lt": {2, 2}, "le": {2, 2}, "gt": {2, 2}, "ge": {2, 2},
	"add": {2, -1}, "sub": {2, 2}, "mul": {2, -1}, "ite": {3, 3}, "in": {2, -1},
}

// Ops lists the LSL operators in the TS SDK's order.
var Ops = []string{"and", "or", "not", "implies", "iff", "eq", "neq", "lt", "le", "gt", "ge", "add", "sub", "mul", "ite", "in"}

// ---------------------------------------------------------------------------
// Builders
// ---------------------------------------------------------------------------

func lit(v any) Expr {
	if e, ok := v.(Expr); ok {
		return e
	}
	return C(v)
}

// In is the model input in.<name>.
func In(name string) Expr { return Expr{Kind: "var", Name: "in." + name} }

// Out is the model output out.<name>.
func Out(name string) Expr { return Expr{Kind: "var", Name: "out." + name} }

// V is a bare variable (a def parameter).
func V(name string) Expr { return Expr{Kind: "var", Name: name} }

// C is a constant: a bool, a number or a string.
func C(v any) Expr { return Expr{Kind: "const", Value: Normalize(v)} }

func op(name string, args ...Expr) Expr { return Expr{Kind: "op", Name: name, Args: args} }

func And(args ...Expr) Expr     { return op("and", args...) }
func Or(args ...Expr) Expr      { return op("or", args...) }
func Not(a Expr) Expr           { return op("not", a) }
func Implies(a, b Expr) Expr    { return op("implies", a, b) }
func Iff(a, b Expr) Expr        { return op("iff", a, b) }
func Eq(a, b any) Expr          { return op("eq", lit(a), lit(b)) }
func Neq(a, b any) Expr         { return op("neq", lit(a), lit(b)) }
func Lt(a, b any) Expr          { return op("lt", lit(a), lit(b)) }
func Le(a, b any) Expr          { return op("le", lit(a), lit(b)) }
func Gt(a, b any) Expr          { return op("gt", lit(a), lit(b)) }
func Ge(a, b any) Expr          { return op("ge", lit(a), lit(b)) }
func Add(args ...any) Expr      { return op("add", lits(args)...) }
func Sub(a, b any) Expr         { return op("sub", lit(a), lit(b)) }
func Mul(args ...any) Expr      { return op("mul", lits(args)...) }
func Ite(c Expr, a, b any) Expr { return op("ite", c, lit(a), lit(b)) }

// OneOf is a ∈ {values...}.
func OneOf(a Expr, values ...any) Expr { return op("in", append([]Expr{a}, lits(values)...)...) }

// Call applies a def of the model.
func Call(name string, args ...Expr) Expr { return Expr{Kind: "call", Name: name, Args: args} }

func lits(vs []any) []Expr {
	out := make([]Expr, len(vs))
	for i, v := range vs {
		out[i] = lit(v)
	}
	return out
}

// ---------------------------------------------------------------------------
// JSON
// ---------------------------------------------------------------------------

func (e Expr) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	switch e.Kind {
	case "var":
		b.WriteString(`{"var":` + jsString(e.Name) + `}`)
	case "const":
		v, err := json.Marshal(e.Value)
		if err != nil {
			return nil, err
		}
		b.WriteString(`{"const":`)
		b.Write(v)
		b.WriteByte('}')
	case "op", "call":
		b.WriteString(`{"` + e.Kind + `":` + jsString(e.Name) + `,"args":`)
		args, err := json.Marshal(nonNil(e.Args))
		if err != nil {
			return nil, err
		}
		b.Write(args)
		b.WriteByte('}')
	default:
		return nil, fmt.Errorf("LSL: cannot serialize an invalid expression (%s)", e.bad)
	}
	return b.Bytes(), nil
}

func (e *Expr) UnmarshalJSON(data []byte) error {
	var raw any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	*e = exprFromAny(raw)
	return nil
}

// exprFromAny converts a decoded JSON value, recording (not failing on) malformed nodes
// so ValidateExpr can report every problem with its path, as the TS SDK does.
func exprFromAny(raw any) Expr {
	o, ok := raw.(map[string]any)
	if !ok {
		return Expr{bad: "not an object"}
	}
	args := func() ([]Expr, bool) {
		a, ok := o["args"].([]any)
		if !ok {
			return nil, false
		}
		out := make([]Expr, len(a))
		for i, x := range a {
			out[i] = exprFromAny(x)
		}
		return out, true
	}
	if c, has := o["const"]; has {
		c = Normalize(c)
		switch c.(type) {
		case bool, float64, string:
			return Expr{Kind: "const", Value: c}
		}
		return Expr{Kind: "const", Value: c, bad: "bad const"}
	}
	if v, has := o["var"]; has {
		return Expr{Kind: "var", Name: fmt.Sprint(v)}
	}
	if c, has := o["call"]; has {
		a, _ := args()
		return Expr{Kind: "call", Name: fmt.Sprint(c), Args: a}
	}
	if op, has := o["op"]; has {
		a, ok := args()
		name, _ := op.(string)
		return Expr{Kind: "op", Name: name, Args: a, argsBad: !ok}
	}
	return Expr{bad: "expected one of var/const/op/call"}
}

// ParseExpr reads an LSL expression from JSON text.
func ParseExpr(text string) (Expr, error) {
	var e Expr
	if err := json.Unmarshal([]byte(text), &e); err != nil {
		return Expr{}, err
	}
	return e, nil
}

// ---------------------------------------------------------------------------
// Evaluation
// ---------------------------------------------------------------------------

// LslError is an evaluation error (unbound variable, wrong type...).
type LslError struct{ Msg string }

func (e *LslError) Error() string { return e.Msg }

func lslErr(format string, a ...any) error { return &LslError{Msg: fmt.Sprintf(format, a...)} }

// EvalContext is what an expression is evaluated against.
type EvalContext struct {
	Input  Env
	Output Env
	Defs   Defs
}

// Evaluate computes the value of e.
func Evaluate(e Expr, ctx EvalContext) (any, error) { return evaluate(e, ctx, nil, 0) }

// Holds evaluates e and requires a boolean.
func Holds(e Expr, ctx EvalContext) (bool, error) {
	v, err := Evaluate(e, ctx)
	if err != nil {
		return false, err
	}
	return asBool(v)
}

func evaluate(e Expr, ctx EvalContext, locals Env, depth int) (any, error) {
	if depth > 64 {
		return nil, lslErr("LSL: max recursion depth exceeded (recursive def?)")
	}
	switch e.Kind {
	case "const":
		return Normalize(e.Value), nil
	case "var":
		return lookup(e.Name, ctx, locals)
	case "call":
		def, ok := ctx.Defs.Get(e.Name)
		if !ok {
			return nil, lslErr("LSL: unknown def '%s'", e.Name)
		}
		if len(def.Params) != len(e.Args) {
			return nil, lslErr("LSL: def '%s' expects %d args, got %d", e.Name, len(def.Params), len(e.Args))
		}
		bound := Env{}
		for i, p := range def.Params {
			v, err := evaluate(e.Args[i], ctx, locals, depth+1)
			if err != nil {
				return nil, err
			}
			bound[p] = v
		}
		return evaluate(def.Body, ctx, bound, depth+1)
	case "op":
	default:
		return nil, lslErr("LSL: invalid expression")
	}
	ev := func(x Expr) (any, error) { return evaluate(x, ctx, locals, depth+1) }
	evB := func(x Expr) (bool, error) {
		v, err := ev(x)
		if err != nil {
			return false, err
		}
		return asBool(v)
	}
	evN := func(x Expr) (float64, error) {
		v, err := ev(x)
		if err != nil {
			return 0, err
		}
		return asNum(v)
	}
	a := e.Args
	need := func(n int) error {
		if len(a) < n {
			return lslErr("LSL: op '%s' needs %d args", e.Name, n)
		}
		return nil
	}
	switch e.Name {
	case "and":
		for _, x := range a {
			b, err := evB(x)
			if err != nil || !b {
				return false, err
			}
		}
		return true, nil
	case "or":
		for _, x := range a {
			b, err := evB(x)
			if err != nil || b {
				return b, err
			}
		}
		return false, nil
	case "not":
		if err := need(1); err != nil {
			return nil, err
		}
		b, err := evB(a[0])
		return !b, err
	case "implies", "iff":
		if err := need(2); err != nil {
			return nil, err
		}
		p, err := evB(a[0])
		if err != nil {
			return nil, err
		}
		if e.Name == "implies" && !p {
			return true, nil
		}
		q, err := evB(a[1])
		if err != nil {
			return nil, err
		}
		if e.Name == "implies" {
			return q, nil
		}
		return p == q, nil
	case "eq", "neq":
		if err := need(2); err != nil {
			return nil, err
		}
		x, err := ev(a[0])
		if err != nil {
			return nil, err
		}
		y, err := ev(a[1])
		if err != nil {
			return nil, err
		}
		return same(x, y) == (e.Name == "eq"), nil
	case "lt", "le", "gt", "ge", "sub":
		if err := need(2); err != nil {
			return nil, err
		}
		x, err := evN(a[0])
		if err != nil {
			return nil, err
		}
		y, err := evN(a[1])
		if err != nil {
			return nil, err
		}
		switch e.Name {
		case "lt":
			return x < y, nil
		case "le":
			return x <= y, nil
		case "gt":
			return x > y, nil
		case "ge":
			return x >= y, nil
		}
		return x - y, nil
	case "add", "mul":
		acc := 0.0
		if e.Name == "mul" {
			acc = 1
		}
		for _, x := range a {
			n, err := evN(x)
			if err != nil {
				return nil, err
			}
			if e.Name == "add" {
				acc += n
			} else {
				acc *= n
			}
		}
		return acc, nil
	case "ite":
		if err := need(3); err != nil {
			return nil, err
		}
		c, err := evB(a[0])
		if err != nil {
			return nil, err
		}
		if c {
			return ev(a[1])
		}
		return ev(a[2])
	case "in":
		if err := need(1); err != nil {
			return nil, err
		}
		v, err := ev(a[0])
		if err != nil {
			return nil, err
		}
		for _, x := range a[1:] {
			y, err := ev(x)
			if err != nil {
				return nil, err
			}
			if same(y, v) {
				return true, nil
			}
		}
		return false, nil
	}
	return nil, lslErr("LSL: unknown op '%s'", e.Name)
}

func lookup(name string, ctx EvalContext, locals Env) (any, error) {
	if v, ok := locals[name]; ok {
		return v, nil
	}
	ns, field, hasField := strings.Cut(name, ".")
	var src Env
	switch ns {
	case "in":
		src = ctx.Input
	case "out":
		src = ctx.Output
	}
	if src != nil && hasField {
		if v, ok := src[field]; ok {
			return Normalize(v), nil
		}
	}
	return nil, lslErr("LSL: unbound variable '%s'", name)
}

func asBool(v any) (bool, error) {
	b, ok := v.(bool)
	if !ok {
		return false, lslErr("LSL: expected boolean, got %s", Stringify(v))
	}
	return b, nil
}

func asNum(v any) (float64, error) {
	n, ok := Normalize(v).(float64)
	if !ok {
		return 0, lslErr("LSL: expected number, got %s", Stringify(v))
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Static validation
// ---------------------------------------------------------------------------

// ValidateExpr returns the problems that make e ill-formed for the model; empty means well-formed.
func ValidateExpr(e Expr, model Model, locals []string, path string) []string {
	var errs []string
	if e.bad != "" && e.Kind != "const" {
		return []string{path + ": " + e.bad}
	}
	switch e.Kind {
	case "const":
		if e.bad != "" {
			errs = append(errs, path+": bad const")
		}
	case "var":
		if !contains(locals, e.Name) {
			ns, field, hasField := strings.Cut(e.Name, ".")
			var table Fields
			switch ns {
			case "in":
				table = model.Inputs
			case "out":
				table = model.Outputs
			}
			if (ns != "in" && ns != "out") || !hasField || !table.Has(field) {
				errs = append(errs, fmt.Sprintf("%s: unknown variable '%s' (use in.<input> or out.<output>)", path, e.Name))
			}
		}
	case "call":
		def, ok := model.Defs.Get(e.Name)
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: unknown def '%s'", path, e.Name))
		}
		if ok && len(def.Params) != len(e.Args) {
			errs = append(errs, fmt.Sprintf("%s: def '%s' arity mismatch", path, e.Name))
		}
		for i, x := range e.Args {
			errs = append(errs, ValidateExpr(x, model, locals, fmt.Sprintf("%s.args[%d]", path, i))...)
		}
	case "op":
		ar, known := opArity[e.Name]
		switch {
		case !known:
			errs = append(errs, fmt.Sprintf("%s: unknown op '%s'", path, e.Name))
		case e.argsBad:
			errs = append(errs, path+": missing args")
		default:
			if len(e.Args) < ar[0] || (ar[1] >= 0 && len(e.Args) > ar[1]) {
				errs = append(errs, fmt.Sprintf("%s: op '%s' arity %d", path, e.Name, len(e.Args)))
			}
			for i, x := range e.Args {
				errs = append(errs, ValidateExpr(x, model, locals, fmt.Sprintf("%s.args[%d]", path, i))...)
			}
		}
	default:
		errs = append(errs, path+": expected one of var/const/op/call")
	}
	return errs
}

// FreeVars lists the variables an expression mentions, sorted.
func FreeVars(e Expr) []string {
	set := map[string]bool{}
	var walk func(Expr)
	walk = func(x Expr) {
		if x.Kind == "var" {
			set[x.Name] = true
		}
		for _, a := range x.Args {
			walk(a)
		}
	}
	walk(e)
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Pretty printing
// ---------------------------------------------------------------------------

var infix = map[string]string{
	"and": "∧", "or": "∨", "implies": "⇒", "iff": "⇔", "eq": "=", "neq": "≠",
	"lt": "<", "le": "≤", "gt": ">", "ge": "≥", "add": "+", "sub": "-", "mul": "·",
}

// Show renders e in the notation of the TS SDK's show(): in.password = "right" ⇒ out.token.
func Show(e Expr) string {
	switch e.Kind {
	case "const":
		return Stringify(e.Value)
	case "var":
		return e.Name
	case "call":
		parts := make([]string, len(e.Args))
		for i, a := range e.Args {
			parts[i] = Show(a)
		}
		return e.Name + "(" + strings.Join(parts, ", ") + ")"
	}
	a := e.Args
	switch e.Name {
	case "not":
		return "¬" + wrap(a[0])
	case "ite":
		return "if " + Show(a[0]) + " then " + Show(a[1]) + " else " + Show(a[2])
	case "in":
		parts := make([]string, len(a)-1)
		for i, x := range a[1:] {
			parts[i] = Show(x)
		}
		return Show(a[0]) + " ∈ {" + strings.Join(parts, ", ") + "}"
	}
	parts := make([]string, len(a))
	for i, x := range a {
		parts[i] = wrap(x)
	}
	return strings.Join(parts, " "+infix[e.Name]+" ")
}

func wrap(e Expr) string {
	if e.Kind == "op" && e.Name != "not" {
		return "(" + Show(e) + ")"
	}
	return Show(e)
}

// ---------------------------------------------------------------------------
// SMT-LIB2 export
// ---------------------------------------------------------------------------

// ToSmtLib compiles requires ⇒ ensures into an SMT-LIB2 script asserting its negation; `unsat`
// means the clause holds for every input/output the domains (and the optional implementation
// relation) allow. Same output as the TS SDK's toSmtLib.
func ToSmtLib(model Model, requires *Expr, ensures Expr, implementation *Expr) string {
	lines := []string{"(set-logic ALL)"}
	enums := map[string][]string{}
	sym := func(name string) string { return "|" + name + "|" }
	declare := func(prefix string, table Fields) {
		for _, f := range table {
			name := prefix + "." + f.Name
			d := f.Domain
			switch d.Type {
			case "bool":
				lines = append(lines, fmt.Sprintf("(declare-const %s Bool)", sym(name)))
			case "int":
				lines = append(lines, fmt.Sprintf("(declare-const %s Int)", sym(name)))
				if d.Min != nil {
					lines = append(lines, fmt.Sprintf("(assert (>= %s %s))", sym(name), smtInt(*d.Min)))
				}
				if d.Max != nil {
					lines = append(lines, fmt.Sprintf("(assert (<= %s %s))", sym(name), smtInt(*d.Max)))
				}
			case "enum":
				vs := make([]string, len(d.Values))
				for i, v := range d.Values {
					vs[i] = jsValueString(v)
				}
				enums[name] = vs
				lines = append(lines, fmt.Sprintf("(declare-const %s Int)", sym(name)))
				lines = append(lines, fmt.Sprintf("(assert (and (>= %s 0) (< %s %d)))", sym(name), sym(name), len(d.Values)))
			default:
				lines = append(lines, fmt.Sprintf("(declare-const %s String)", sym(name)))
			}
		}
	}
	declare("in", model.Inputs)
	declare("out", model.Outputs)

	var smt func(e Expr) string
	enumIndex := func(v, other Expr) (string, string, bool) {
		if v.Kind == "var" && other.Kind == "const" {
			if vals, ok := enums[v.Name]; ok {
				idx := -1
				for i, x := range vals {
					if x == jsValueString(other.Value) {
						idx = i
						break
					}
				}
				return sym(v.Name), fmt.Sprint(idx), true
			}
		}
		return "", "", false
	}
	head := map[string]string{"and": "and", "or": "or", "not": "not", "implies": "=>", "iff": "=", "lt": "<", "le": "<=", "gt": ">", "ge": ">=", "add": "+", "sub": "-", "mul": "*", "ite": "ite"}
	smt = func(e Expr) string {
		switch e.Kind {
		case "const":
			switch v := Normalize(e.Value).(type) {
			case bool:
				if v {
					return "true"
				}
				return "false"
			case float64:
				return smtInt(v)
			default:
				return Stringify(v)
			}
		case "var":
			return sym(e.Name)
		case "call":
			parts := []string{sym(e.Name)}
			for _, a := range e.Args {
				parts = append(parts, smt(a))
			}
			return "(" + strings.Join(parts, " ") + ")"
		}
		a := e.Args
		if e.Name == "eq" || e.Name == "neq" {
			var body string
			if s, i, ok := enumIndex(a[0], a[1]); ok {
				body = "(= " + s + " " + i + ")"
			} else if s, i, ok := enumIndex(a[1], a[0]); ok {
				body = "(= " + s + " " + i + ")"
			} else {
				body = "(= " + smt(a[0]) + " " + smt(a[1]) + ")"
			}
			if e.Name == "eq" {
				return body
			}
			return "(not " + body + ")"
		}
		if e.Name == "in" {
			parts := []string{}
			for _, x := range a[1:] {
				parts = append(parts, smt(op("eq", a[0], x)))
			}
			return "(or " + strings.Join(parts, " ") + ")"
		}
		parts := []string{head[e.Name]}
		for _, x := range a {
			parts = append(parts, smt(x))
		}
		return "(" + strings.Join(parts, " ") + ")"
	}
	for _, d := range model.Defs {
		params := make([]string, len(d.Params))
		for i, p := range d.Params {
			params[i] = "(" + sym(p) + " Bool)"
		}
		lines = append(lines, fmt.Sprintf("(define-fun %s (%s) Bool %s)", sym(d.Name), strings.Join(params, " "), smt(d.Body)))
	}
	if implementation != nil {
		lines = append(lines, "(assert "+smt(*implementation)+")")
	}
	goal := smt(ensures)
	if requires != nil {
		goal = "(=> " + smt(*requires) + " " + smt(ensures) + ")"
	}
	lines = append(lines, "(assert (not "+goal+"))", "(check-sat)", "(get-model)")
	return strings.Join(lines, "\n")
}

func smtInt(n float64) string {
	if n < 0 {
		return "(- " + jsNumber(-n) + ")"
	}
	return jsNumber(n)
}

// jsValueString is JavaScript's String(v).
func jsValueString(v any) string {
	switch x := Normalize(v).(type) {
	case float64:
		if math.IsNaN(x) {
			return "NaN"
		}
		return jsNumber(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case string:
		return x
	}
	return fmt.Sprint(v)
}
