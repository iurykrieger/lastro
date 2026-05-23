package entrypoint

import "testing"

func TestCompiledSchemaIsAvailable(t *testing.T) {
	s, err := compiledSchema()
	if err != nil {
		t.Fatalf("compiledSchema: %v", err)
	}
	if s == nil {
		t.Fatal("compiledSchema: returned nil schema")
	}
}

func TestCompiledSchemaIsCached(t *testing.T) {
	a, err := compiledSchema()
	if err != nil {
		t.Fatalf("compiledSchema (first call): %v", err)
	}
	b, err := compiledSchema()
	if err != nil {
		t.Fatalf("compiledSchema (second call): %v", err)
	}
	if a != b {
		t.Fatal("compiledSchema returned different pointers on successive calls")
	}
}
