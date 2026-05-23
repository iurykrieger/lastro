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
	a, _ := compiledSchema()
	b, _ := compiledSchema()
	if a != b {
		t.Fatal("compiledSchema returned different pointers on successive calls")
	}
}
