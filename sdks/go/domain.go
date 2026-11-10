package lastro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// Values and environments.
//
// LSL values are booleans, numbers and strings. Numbers are float64, as in
// JSON and JavaScript; Normalize turns Go integers into float64 so a subject
// may return Env{"status": 200}.

// Env maps model fields to values: the input or output of one run.
type Env map[string]any

// Normalize converts any Go number to float64 and json.Number to float64; other values are returned as is.
func Normalize(v any) any {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int8:
		return float64(x)
	case int16:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case uint:
		return float64(x)
	case uint8:
		return float64(x)
	case uint16:
		return float64(x)
	case uint32:
		return float64(x)
	case uint64:
		return float64(x)
	case float32:
		return float64(x)
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return x.String()
		}
		return f
	}
	return v
}

// NormalizeEnv returns a copy of e with every value normalized.
func NormalizeEnv(e Env) Env {
	out := make(Env, len(e))
	for k, v := range e {
		out[k] = Normalize(v)
	}
	return out
}

// same is JavaScript's ===: same type and same value.
func same(a, b any) bool {
	a, b = Normalize(a), Normalize(b)
	switch x := a.(type) {
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	}
	return false
}

// ---------------------------------------------------------------------------
// Domains
// ---------------------------------------------------------------------------

// Domain is the set of values a model field can take.
type Domain struct {
	Type     string    `json:"type"` // "bool" | "enum" | "int" | "string"
	Values   []any     `json:"values,omitempty"`
	Min      *float64  `json:"min,omitempty"`
	Max      *float64  `json:"max,omitempty"`
	Examples *[]string `json:"examples,omitempty"`
}

// Bool is the domain {false, true}.
func Bool() Domain { return Domain{Type: "bool"} }

// Enum is a finite set of strings or numbers.
func Enum(values ...any) Domain {
	vs := make([]any, len(values))
	for i, v := range values {
		vs[i] = Normalize(v)
	}
	return Domain{Type: "enum", Values: vs}
}

// Int is the integers from min to max, inclusive.
func Int(min, max int) Domain {
	a, b := float64(min), float64(max)
	return Domain{Type: "int", Min: &a, Max: &b}
}

// UnboundedInt is every integer: never enumerable, so only sampling applies.
func UnboundedInt() Domain { return Domain{Type: "int"} }

// String is any string; the examples bias sampling.
func String(examples ...string) Domain {
	ex := append([]string{}, examples...)
	return Domain{Type: "string", Examples: &ex}
}

// MarshalJSON writes the same keys, in the same order, as the TS builders (D.enum, D.int...).
func (d Domain) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`{"type":`)
	b.WriteString(jsString(d.Type))
	switch d.Type {
	case "enum":
		vs, err := json.Marshal(nonNil(d.Values))
		if err != nil {
			return nil, err
		}
		b.WriteString(`,"values":`)
		b.Write(vs)
	case "int":
		if d.Min != nil {
			b.WriteString(`,"min":` + jsNumber(*d.Min))
		}
		if d.Max != nil {
			b.WriteString(`,"max":` + jsNumber(*d.Max))
		}
	case "string":
		if d.Examples != nil {
			ex, err := json.Marshal(nonNil(*d.Examples))
			if err != nil {
				return nil, err
			}
			b.WriteString(`,"examples":`)
			b.Write(ex)
		}
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// UnmarshalJSON normalizes enum values to float64/string/bool.
func (d *Domain) UnmarshalJSON(data []byte) error {
	type raw Domain
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*d = Domain(r)
	for i, v := range d.Values {
		d.Values[i] = Normalize(v)
	}
	return nil
}

// Size is the number of values, +Inf when infinite.
func (d Domain) Size() float64 {
	switch d.Type {
	case "bool":
		return 2
	case "enum":
		return float64(len(d.Values))
	case "int":
		if d.Min != nil && d.Max != nil {
			return math.Max(0, *d.Max-*d.Min+1)
		}
	}
	return math.Inf(1)
}

// Enumerate lists every value of a finite domain.
func (d Domain) Enumerate() ([]any, error) {
	switch d.Type {
	case "bool":
		return []any{false, true}, nil
	case "enum":
		return append([]any{}, d.Values...), nil
	case "int":
		if d.Min == nil || d.Max == nil {
			return nil, fmt.Errorf("cannot enumerate unbounded int")
		}
		var out []any
		for i := *d.Min; i <= *d.Max; i++ {
			out = append(out, i)
		}
		return out, nil
	}
	return nil, fmt.Errorf("cannot enumerate string domain")
}

// Contains reports whether v belongs to the domain.
func (d Domain) Contains(v any) bool {
	v = Normalize(v)
	switch d.Type {
	case "bool":
		_, ok := v.(bool)
		return ok
	case "enum":
		for _, x := range d.Values {
			if same(x, v) {
				return true
			}
		}
		return false
	case "int":
		n, ok := v.(float64)
		return ok && n == math.Trunc(n) && !math.IsInf(n, 0) && (d.Min == nil || n >= *d.Min) && (d.Max == nil || n <= *d.Max)
	case "string":
		_, ok := v.(string)
		return ok
	}
	return false
}

// ---------------------------------------------------------------------------
// Ordered field tables
// ---------------------------------------------------------------------------

// Field is one named model input or output.
type Field struct {
	Name   string
	Domain Domain
}

// Fields is an ordered table of fields. It reads and writes as a JSON object and keeps its key order.
type Fields []Field

// Get returns the domain of a field.
func (fs Fields) Get(name string) (Domain, bool) {
	for _, f := range fs {
		if f.Name == name {
			return f.Domain, true
		}
	}
	return Domain{}, false
}

// Has reports whether the table declares the field.
func (fs Fields) Has(name string) bool {
	_, ok := fs.Get(name)
	return ok
}

// Names returns the field names in declaration order.
func (fs Fields) Names() []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Name
	}
	return out
}

func (fs Fields) sortedNames() []string {
	names := fs.Names()
	sort.SliceStable(names, func(i, j int) bool { return utf16Less(names[i], names[j]) })
	return names
}

func (fs Fields) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range fs {
		if i > 0 {
			b.WriteByte(',')
		}
		d, err := f.Domain.MarshalJSON()
		if err != nil {
			return nil, err
		}
		b.WriteString(jsString(f.Name))
		b.WriteByte(':')
		b.Write(d)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func (fs *Fields) UnmarshalJSON(data []byte) error {
	return decodeOrderedObject(data, func(key string, raw json.RawMessage) error {
		var d Domain
		if err := json.Unmarshal(raw, &d); err != nil {
			return fmt.Errorf("field %s: %w", key, err)
		}
		*fs = append(*fs, Field{Name: key, Domain: d})
		return nil
	}, func() { *fs = Fields{} })
}

// decodeOrderedObject calls each(key, value) in document order.
func decodeOrderedObject(data []byte, each func(string, json.RawMessage) error, reset func()) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok == nil {
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("expected a JSON object")
	}
	reset()
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		if err := each(kt.(string), raw); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	return err
}

// SpaceSize is the size of the cartesian product of the fields (+Inf when any is infinite).
func SpaceSize(fs Fields) float64 {
	n := 1.0
	for _, f := range fs {
		n *= f.Domain.Size()
	}
	return n
}

// EnumerateSpace lists every point of a finite space. Keys are visited in sorted order and the
// last key varies fastest: the same order as the TS SDK, so counterexamples match.
func EnumerateSpace(fs Fields) ([]Env, error) {
	keys := fs.sortedNames()
	values := make([][]any, len(keys))
	for i, k := range keys {
		d, _ := fs.Get(k)
		vs, err := d.Enumerate()
		if err != nil {
			return nil, err
		}
		if len(vs) == 0 {
			return nil, nil
		}
		values[i] = vs
	}
	idx := make([]int, len(keys))
	var out []Env
	for {
		env := Env{}
		for i, k := range keys {
			env[k] = values[i][idx[i]]
		}
		out = append(out, env)
		i := len(keys) - 1
		for i >= 0 {
			idx[i]++
			if idx[i] < len(values[i]) {
				break
			}
			idx[i] = 0
			i--
		}
		if i < 0 {
			return out, nil
		}
	}
}

// rng is mulberry32, bit for bit the TS SDK's generator: same seed, same samples.
func rng(seed uint32) func() float64 {
	a := seed
	return func() float64 {
		a += 0x6d2b79f5
		t := a
		t = (t ^ (t >> 15)) * (t | 1)
		t ^= t + (t^(t>>7))*(t|61)
		return float64(t^(t>>14)) / 4294967296
	}
}

func sampleDomain(d Domain, next func() float64) any {
	switch d.Type {
	case "bool":
		return next() < 0.5
	case "enum":
		return d.Values[int(math.Floor(next()*float64(len(d.Values))))]
	case "int":
		min, max := -1_000_000.0, 1_000_000.0
		if d.Min != nil {
			min = *d.Min
		}
		if d.Max != nil {
			max = *d.Max
		}
		r := next()
		if r < 0.1 {
			return min
		}
		if r < 0.2 {
			return max
		}
		if r < 0.3 && min <= 0 && max >= 0 {
			return 0.0
		}
		return min + math.Floor(next()*(max-min+1))
	case "string":
		var ex []string
		if d.Examples != nil {
			ex = *d.Examples
		}
		if len(ex) > 0 && next() < 0.7 {
			return ex[int(math.Floor(next()*float64(len(ex))))]
		}
		n := int(math.Floor(next() * 12))
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(97 + int(math.Floor(next()*26)))
		}
		return string(b)
	}
	return nil
}

func sampleSpace(fs Fields, next func() float64) Env {
	env := Env{}
	for _, k := range fs.sortedNames() {
		d, _ := fs.Get(k)
		env[k] = sampleDomain(d, next)
	}
	return env
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
