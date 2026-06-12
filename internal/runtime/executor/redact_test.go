package executor

import (
	"bytes"
	"testing"
)

func TestRedactor_MasksRegisteredValues(t *testing.T) {
	r := &redactor{}
	r.Add("supersecret")
	got := r.Apply([]byte(`token=supersecret rest`))
	want := []byte(`token=*** rest`)
	if !bytes.Equal(got, want) {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}

func TestRedactor_SkipsShortValues(t *testing.T) {
	r := &redactor{}
	r.Add("dev") // < minRedactLen: masking "dev" would corrupt unrelated output
	got := r.Apply([]byte("development mode"))
	if !bytes.Equal(got, []byte("development mode")) {
		t.Errorf("short value was masked: %q", got)
	}
}

func TestRedactor_LongestFirstAndMultiOccurrence(t *testing.T) {
	r := &redactor{}
	r.Add("secret")
	r.Add("secret-extended") // must mask before its prefix
	got := r.Apply([]byte("a=secret-extended b=secret c=secret"))
	want := []byte("a=*** b=*** c=***")
	if !bytes.Equal(got, want) {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}

func TestRedactor_NilSafe(t *testing.T) {
	var r *redactor
	r.Add("whatever")
	if got := r.Apply([]byte("x")); !bytes.Equal(got, []byte("x")) {
		t.Errorf("nil redactor mutated input: %q", got)
	}
}

func TestRedactor_DeduplicatesValues(t *testing.T) {
	r := &redactor{}
	r.Add("samevalue")
	r.Add("samevalue")
	got := r.Apply([]byte("x=samevalue"))
	if !bytes.Equal(got, []byte("x=***")) {
		t.Errorf("Apply = %q, want x=***", got)
	}
	r.mu.RLock()
	n := len(r.values)
	r.mu.RUnlock()
	if n != 1 {
		t.Errorf("registered %d values, want 1 (deduped)", n)
	}
}

func TestRedactor_BoundaryLengthMasked(t *testing.T) {
	r := &redactor{}
	r.Add("abcd") // exactly minRedactLen
	if got := r.Apply([]byte("v=abcd")); !bytes.Equal(got, []byte("v=***")) {
		t.Errorf("Apply = %q, want v=***", got)
	}
}
