package fixture

import (
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestPersist_NewFile_WritesUnderFixturesDir(t *testing.T) {
	dir := t.TempDir()
	in := []byte(`schema_version: 1.0.0
id: fx-create-order-request
use_case_id: create-order
role: input
content_type: application/json
payload: |
  {"item":"book"}
binding:
  channel: http
  selector:
    method: POST
    path: /orders
source_refs:
  - path: src/handlers/orders.ts
    symbol: createOrder
`)
	if err := Persist(in, dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	written, err := os.ReadFile(filepath.Join(dir, "fixtures", "fx-create-order-request.yaml"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	// Round-trip through the loader so we exercise the full schema +
	// payload validation against what was written.
	fx, err := LoadFixtureBytes(written)
	if err != nil {
		t.Fatalf("LoadFixtureBytes on written: %v", err)
	}
	if fx.ID != "fx-create-order-request" {
		t.Fatalf("ID=%q", fx.ID)
	}
	// Confirm the payload survived the round-trip.
	if string(fx.Payload) == "" {
		t.Fatal("Payload missing from written file")
	}
}

func TestPersist_BumpsSchemaVersionOnExisting(t *testing.T) {
	dir := t.TempDir()
	fxDir := filepath.Join(dir, "fixtures")
	if err := os.MkdirAll(fxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prior := []byte(`schema_version: 1.0.3
id: fx-x
use_case_id: uc-x
role: input
content_type: application/json
payload: |
  {}
binding:
  channel: http
  selector:
    method: POST
    path: /x
source_refs:
  - path: src/x.ts
    symbol: x
`)
	if err := os.WriteFile(filepath.Join(fxDir, "fx-x.yaml"), prior, 0o644); err != nil {
		t.Fatal(err)
	}
	in := []byte(`schema_version: 1.0.0
id: fx-x
use_case_id: uc-x
role: input
content_type: application/json
payload: |
  {"k":"v"}
binding:
  channel: http
  selector:
    method: POST
    path: /x
source_refs:
  - path: src/x.ts
    symbol: x
`)
	if err := Persist(in, dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	out, _ := os.ReadFile(filepath.Join(fxDir, "fx-x.yaml"))
	var head struct {
		SchemaVersion string `json:"schema_version" yaml:"schema_version"`
	}
	_ = yaml.Unmarshal(out, &head)
	if head.SchemaVersion != "1.0.4" {
		t.Fatalf("SchemaVersion=%q, want 1.0.4 (prior 1.0.3 + 1)", head.SchemaVersion)
	}
}

func TestPersist_RejectsMalformedJSONPayload(t *testing.T) {
	dir := t.TempDir()
	// content_type says JSON, but payload is unparseable.
	in := []byte(`schema_version: 1.0.0
id: fx-bad
use_case_id: uc-x
role: input
content_type: application/json
payload: |
  { broken
binding:
  channel: http
  selector:
    method: POST
    path: /x
source_refs:
  - path: src/x.ts
    symbol: x
`)
	err := Persist(in, dir)
	if err == nil {
		t.Fatal("Persist accepted malformed JSON payload")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "fixtures", "fx-bad.yaml")); statErr == nil {
		t.Fatal("Persist wrote a file despite validation failure")
	}
}
