package lastro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// JavaScript-compatible JSON.
//
// Hashes, handoff request ids and prompts must be byte-identical to what the
// TypeScript SDK produces, so the Go SDK cannot rely on encoding/json's output
// (it escapes <, > and &, and formats some numbers differently). Values are
// marshaled with encoding/json, re-read token by token (which keeps key
// order), and written back the way JavaScript's JSON.stringify writes them.

type jnode struct {
	kind byte // 'o' object, 'a' array, 's' string, 'n' number, 'b' bool, 'z' null
	keys []string
	vals []*jnode
	str  string
	num  float64
	b    bool
}

func parseJSONTree(data []byte) (*jnode, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	n, err := readNode(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing data after JSON value")
	}
	return n, nil
}

func readNode(dec *json.Decoder) (*jnode, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			n := &jnode{kind: 'o'}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, _ := kt.(string)
				v, err := readNode(dec)
				if err != nil {
					return nil, err
				}
				n.keys = append(n.keys, k)
				n.vals = append(n.vals, v)
			}
			_, err := dec.Token()
			return n, err
		case '[':
			n := &jnode{kind: 'a'}
			for dec.More() {
				v, err := readNode(dec)
				if err != nil {
					return nil, err
				}
				n.vals = append(n.vals, v)
			}
			_, err := dec.Token()
			return n, err
		}
		return nil, fmt.Errorf("unexpected delimiter %v", t)
	case string:
		return &jnode{kind: 's', str: t}, nil
	case json.Number:
		f, err := strconv.ParseFloat(string(t), 64)
		if err != nil {
			return nil, err
		}
		return &jnode{kind: 'n', num: f}, nil
	case bool:
		return &jnode{kind: 'b', b: t}, nil
	case nil:
		return &jnode{kind: 'z'}, nil
	}
	return nil, fmt.Errorf("unexpected token %v", tok)
}

func (n *jnode) write(sb *strings.Builder, sortKeys bool, indent string, depth int) {
	nl := func(d int) {
		if indent != "" {
			sb.WriteByte('\n')
			sb.WriteString(strings.Repeat(indent, d))
		}
	}
	switch n.kind {
	case 'o':
		if len(n.keys) == 0 {
			sb.WriteString("{}")
			return
		}
		idx := make([]int, len(n.keys))
		for i := range idx {
			idx[i] = i
		}
		if sortKeys {
			sort.SliceStable(idx, func(a, b int) bool { return utf16Less(n.keys[idx[a]], n.keys[idx[b]]) })
		}
		sb.WriteByte('{')
		for j, i := range idx {
			if j > 0 {
				sb.WriteByte(',')
			}
			nl(depth + 1)
			sb.WriteString(jsString(n.keys[i]))
			sb.WriteByte(':')
			if indent != "" {
				sb.WriteByte(' ')
			}
			n.vals[i].write(sb, sortKeys, indent, depth+1)
		}
		nl(depth)
		sb.WriteByte('}')
	case 'a':
		if len(n.vals) == 0 {
			sb.WriteString("[]")
			return
		}
		sb.WriteByte('[')
		for i, v := range n.vals {
			if i > 0 {
				sb.WriteByte(',')
			}
			nl(depth + 1)
			v.write(sb, sortKeys, indent, depth+1)
		}
		nl(depth)
		sb.WriteByte(']')
	case 's':
		sb.WriteString(jsString(n.str))
	case 'n':
		sb.WriteString(jsNumber(n.num))
	case 'b':
		if n.b {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	default:
		sb.WriteString("null")
	}
}

// utf16Less orders strings the way JavaScript's default sort does (by UTF-16 code units).
func utf16Less(a, b string) bool {
	ua, ub := utf16Units(a), utf16Units(b)
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}

func marshalTree(v any) (*jnode, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return parseJSONTree(bytes.TrimSpace(buf.Bytes()))
}

func render(v any, sortKeys bool, indent string) (string, error) {
	n, err := marshalTree(v)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	n.write(&sb, sortKeys, indent, 0)
	return sb.String(), nil
}

// CanonicalJSON is the TS SDK's canonicalJson: compact JSON with object keys sorted.
func CanonicalJSON(v any) (string, error) { return render(v, true, "") }

// HashOf is "sha256:" + SHA-256 of CanonicalJSON(v), the hash both SDKs use for frozen artifacts and plans.
func HashOf(v any) (string, error) {
	s, err := CanonicalJSON(v)
	if err != nil {
		return "", err
	}
	return "sha256:" + sha256Hex(s), nil
}

func mustHash(v any) string {
	h, err := HashOf(v)
	if err != nil {
		panic(err)
	}
	return h
}

// Stringify writes v as JavaScript's JSON.stringify(v) would: keys in field order, JS number format.
func Stringify(v any) string {
	s, err := render(v, false, "")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return s
}

// StringifyIndent is JSON.stringify(v, null, 2).
func StringifyIndent(v any) string {
	s, err := render(v, false, "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return s
}

// jsString quotes s the way JSON.stringify does: only ", \ and control characters are escaped.
func jsString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&sb, `\u%04x`, r)
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// jsNumber formats a float64 like JavaScript's Number.prototype.toString.
func jsNumber(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "null"
	}
	if f == 0 {
		return "0"
	}
	abs := math.Abs(f)
	if abs >= 1e21 || abs < 1e-6 {
		s := strconv.FormatFloat(f, 'e', -1, 64) // 1e+21, 1.5e-07
		mant, exp, _ := strings.Cut(s, "e")
		sign := exp[0]
		digits := strings.TrimLeft(exp[1:], "0")
		return mant + "e" + string(sign) + digits
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
