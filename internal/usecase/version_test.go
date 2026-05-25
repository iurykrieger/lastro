package usecase

import (
	"bytes"
	"os"
	"testing"

	"github.com/iurykrieger/lastro/internal/usecase/internal/fixturestub"
)

func TestLoad_AcceptsPatchBumpedVersion(t *testing.T) {
	raw, err := os.ReadFile("../../schemas/examples/use-case/http-api.yaml")
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	bumped := bytes.Replace(raw, []byte("schema_version: 2.0.0"), []byte("schema_version: 2.0.5"), 1)

	// Build a fixture store that includes anything the example use-case
	// references. The example uses order-input-fixture and order-output-fixture.
	store := fixturestub.New(map[string]string{
		"order-input-fixture":  `{"customer_id":"c-001"}`,
		"order-output-fixture": `{"order_id":"o-1"}`,
	})
	_, err = Load(bumped, store)
	// We expect Load to succeed since 2.0.5 is within the supported 2.0.x range.
	if err != nil {
		t.Fatalf("Load rejected patch-bumped version: %v", err)
	}
}

func TestLoad_RejectsDifferentMajor(t *testing.T) {
	raw, err := os.ReadFile("../../schemas/examples/use-case/http-api.yaml")
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	bumped := bytes.Replace(raw, []byte("schema_version: 2.0.0"), []byte("schema_version: 3.0.0"), 1)

	store := fixturestub.New(map[string]string{
		"order-input-fixture":  `{"customer_id":"c-001"}`,
		"order-output-fixture": `{"order_id":"o-1"}`,
	})
	_, err = Load(bumped, store)
	if err == nil {
		t.Fatal("Load accepted incompatible major version 3.0.0")
	}
	if !hasCode(err, "USECASE_SCHEMA_VERSION_UNSUPPORTED") {
		t.Fatalf("expected USECASE_SCHEMA_VERSION_UNSUPPORTED error, got: %v", err)
	}
}
