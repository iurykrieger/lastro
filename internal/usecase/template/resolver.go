package template

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/fixture"
)

// ResolveError is returned when a ref cannot be resolved at runtime.
// These errors are distinct from ParseError — parse runs at load time;
// resolve runs at sensor-execution time.
type ResolveError struct {
	Pos Position
	Msg string
}

func (e *ResolveError) Error() string {
	return fmt.Sprintf("template resolve error at line %d col %d: %s", e.Pos.Line, e.Pos.Col, e.Msg)
}

// Resolver evaluates segments against live fixtures and entry points.
type Resolver struct {
	Fixtures    fixture.FixtureStore
	EntryPoints map[string]entrypoint.EntryPoint
}

// Resolve walks segments and produces the fully-rendered string.
func (r *Resolver) Resolve(segs []Segment) (string, error) {
	var b strings.Builder
	for _, s := range segs {
		switch v := s.(type) {
		case Literal:
			b.WriteString(v.Text)
		case FixtureRef:
			val, err := r.resolveFixture(v)
			if err != nil {
				return "", err
			}
			b.WriteString(stringify(val))
		case EntryPointRef:
			val, err := r.resolveEntryPoint(v)
			if err != nil {
				return "", err
			}
			b.WriteString(stringify(val))
		case EnvRef:
			return "", fmt.Errorf("env.* is not valid in use-case text")
		default:
			return "", fmt.Errorf("unknown segment type %T", s)
		}
	}
	return b.String(), nil
}

// ResolveValue resolves a single ref segment to its underlying Go value
// (typed where possible). Literals return their Text. This is the API
// sensors use when they need the raw typed value rather than a string.
func (r *Resolver) ResolveValue(seg Segment) (any, error) {
	switch v := seg.(type) {
	case Literal:
		return v.Text, nil
	case FixtureRef:
		return r.resolveFixture(v)
	case EntryPointRef:
		return r.resolveEntryPoint(v)
	case EnvRef:
		return nil, fmt.Errorf("env.* is not valid in use-case text")
	default:
		return nil, fmt.Errorf("unknown segment type %T", seg)
	}
}

func (r *Resolver) resolveFixture(ref FixtureRef) (any, error) {
	fx, ok := r.Fixtures.LookupFixture(ref.ID)
	if !ok {
		return nil, &ResolveError{Pos: ref.Pos, Msg: "unknown fixture: " + ref.ID}
	}
	if len(ref.JSONPath) == 0 {
		// Bare ref: return the structured Parsed value if available; otherwise
		// fall back to the raw payload as a string. Both paths produce the
		// same human-meaningful render for stringify().
		if fx.Parsed != nil {
			return fx.Parsed, nil
		}
		return string(fx.Payload), nil
	}
	// Drilled ref: structured payload required.
	if fx.Parsed == nil {
		return nil, &ResolveError{
			Pos: ref.Pos,
			Msg: "fixture " + ref.ID + " has no parsed payload; cannot drill " + strings.Join(ref.JSONPath, "."),
		}
	}
	return walkJSON(fx.Parsed, ref.JSONPath, ref.Pos)
}

func (r *Resolver) resolveEntryPoint(ref EntryPointRef) (any, error) {
	ep, ok := r.EntryPoints[ref.ID]
	if !ok {
		return nil, &ResolveError{Pos: ref.Pos, Msg: "unknown entry_point: " + ref.ID}
	}
	if ref.SpecKey == "" {
		return ep.Label(), nil
	}
	v, ok := ep.SpecField(ref.SpecKey)
	if !ok {
		return nil, &ResolveError{Pos: ref.Pos, Msg: "entry_point " + ref.ID + " has no spec field: " + ref.SpecKey}
	}
	return v, nil
}

func walkJSON(v any, path []string, pos Position) (any, error) {
	for i, key := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, &ResolveError{
				Pos: pos,
				Msg: fmt.Sprintf("jsonpath %q crosses non-object at segment %d", strings.Join(path, "."), i),
			}
		}
		next, ok := m[key]
		if !ok {
			return nil, &ResolveError{Pos: pos, Msg: fmt.Sprintf("jsonpath key %q not found", key)}
		}
		v = next
	}
	return v, nil
}

// stringify renders a resolved value for string concatenation. Strings
// pass through naturally; maps and slices serialize as JSON; primitives
// stringify via json.Marshal for canonical form.
func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return "null"
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprintf("%v", x)
		}
		return string(b)
	}
}
