package fixture

import (
	"path/filepath"
	"strings"
	"testing"
)

func loadErr(t *testing.T, name string) error {
	t.Helper()
	p := filepath.Join("testdata", "invalid", name)
	_, err := LoadFixture(p)
	if err == nil {
		t.Fatalf("LoadFixture(%s): expected error, got nil", p)
	}
	return err
}

func TestLoadFixtureMissingPayloadIsSchemaError(t *testing.T) {
	err := loadErr(t, "missing-payload.yaml")
	if !strings.Contains(err.Error(), "payload") {
		t.Errorf("error %q should mention 'payload'", err.Error())
	}
}

func TestLoadFixtureInvalidRoleIsSchemaError(t *testing.T) {
	err := loadErr(t, "invalid-role.yaml")
	if !strings.Contains(err.Error(), "schema validation") {
		t.Errorf("error %q should mention 'schema validation'", err.Error())
	}
}

func TestLoadFixtureInvalidChannelIsSchemaError(t *testing.T) {
	err := loadErr(t, "invalid-channel.yaml")
	if !strings.Contains(err.Error(), "schema validation") {
		t.Errorf("error %q should mention 'schema validation'", err.Error())
	}
}

func TestLoadFixtureMalformedJSONIsParseError(t *testing.T) {
	err := loadErr(t, "malformed-json.yaml")
	if !strings.Contains(err.Error(), "parse payload") {
		t.Errorf("error %q should mention 'parse payload'", err.Error())
	}
}

func TestLoadFixtureMalformedYAMLIsParseError(t *testing.T) {
	err := loadErr(t, "malformed-yaml.yaml")
	if !strings.Contains(err.Error(), "parse payload") {
		t.Errorf("error %q should mention 'parse payload'", err.Error())
	}
}

func TestLoadFixtureMalformedXMLIsParseError(t *testing.T) {
	err := loadErr(t, "malformed-xml.yaml")
	if !strings.Contains(err.Error(), "parse payload") {
		t.Errorf("error %q should mention 'parse payload'", err.Error())
	}
}

func TestLoadFixtureUnknownContentTypeLoadsCleanly(t *testing.T) {
	p := filepath.Join("testdata", "invalid", "unknown-content-type.yaml")
	fx, err := LoadFixture(p)
	if err != nil {
		t.Fatalf("LoadFixture(%s): unexpected error %v", p, err)
	}
	if fx.Parsed != nil {
		t.Errorf("Parsed: got %v, want nil for unknown content type", fx.Parsed)
	}
}

func TestLoadDirectoryDuplicateIDsReportBothFiles(t *testing.T) {
	p := filepath.Join("testdata", "duplicate-id")
	_, err := LoadDirectory(p)
	if err == nil {
		t.Fatal("LoadDirectory: expected duplicate-id error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "duplicate id") {
		t.Errorf("error %q should mention 'duplicate id'", msg)
	}
	if !strings.Contains(msg, "a.yaml") || !strings.Contains(msg, "b.yaml") {
		t.Errorf("error %q should mention both files (a.yaml and b.yaml)", msg)
	}
}
