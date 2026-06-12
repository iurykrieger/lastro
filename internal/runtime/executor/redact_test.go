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
