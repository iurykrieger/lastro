package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestEvidenceRefStringRendersFileColonPath(t *testing.T) {
	ev := EvidenceRef{File: "package.json", Path: "dependencies.express"}
	got := ev.String()
	want := "package.json:dependencies.express"
	if got != want {
		t.Errorf("EvidenceRef.String() = %q, want %q", got, want)
	}
}

func TestEvidenceRefStringIgnoresValue(t *testing.T) {
	ev := EvidenceRef{File: "package.json", Path: "dependencies.express", Value: "^4.18.0"}
	got := ev.String()
	want := "package.json:dependencies.express"
	if got != want {
		t.Errorf("EvidenceRef.String() with value = %q, want %q", got, want)
	}
}

func TestSchemaVersionConstant(t *testing.T) {
	if SchemaVersion != "1.0.0" {
		t.Errorf("SchemaVersion = %q, want %q", SchemaVersion, "1.0.0")
	}
}

func TestStackComponentValidateRejectsBadID(t *testing.T) {
	cases := []struct {
		name   string
		id     string
		substr string
	}{
		{"empty", "", "id"},
		{"uppercase", "Express", "id"},
		{"underscore", "express_v4", "id"},
		{"leading dash", "-express", "id"},
		{"leading digit", "4express", "id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validComponent()
			c.ID = tc.id
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected error for id=%q", tc.id)
			}
			if !strings.Contains(err.Error(), tc.substr) {
				t.Errorf("error %q does not mention %q", err.Error(), tc.substr)
			}
		})
	}
}

func TestStackComponentValidateRejectsMissingFields(t *testing.T) {
	base := validComponent()

	type mutate func(*StackComponent)
	cases := []struct {
		name   string
		m      mutate
		substr string
	}{
		{"missing kind", func(c *StackComponent) { c.Kind = "" }, "kind"},
		{"missing name", func(c *StackComponent) { c.Name = "" }, "name"},
		{"missing version", func(c *StackComponent) { c.Version = "" }, "version"},
		{"empty capabilities", func(c *StackComponent) { c.Capabilities = nil }, "capabilities"},
		{"blank capability entry", func(c *StackComponent) { c.Capabilities = []string{""} }, "capabilities"},
		{"empty evidence", func(c *StackComponent) { c.DetectionEvidence = nil }, "detection_evidence"},
		{"evidence missing file", func(c *StackComponent) {
			c.DetectionEvidence = []EvidenceRef{{Path: "x"}}
		}, "file"},
		{"evidence missing path", func(c *StackComponent) {
			c.DetectionEvidence = []EvidenceRef{{File: "x"}}
		}, "path"},
		{"unknown kind", func(c *StackComponent) { c.Kind = "service" }, "kind"},
		{"wrong schema_version", func(c *StackComponent) { c.SchemaVersion = "0.1.0" }, "schema_version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.m(&c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected error for case %q", tc.name)
			}
			if !strings.Contains(err.Error(), tc.substr) {
				t.Errorf("error %q does not mention %q", err.Error(), tc.substr)
			}
		})
	}
}

func TestStackComponentValidateAggregates(t *testing.T) {
	c := validComponent()
	c.ID = ""
	c.Kind = ""
	c.Name = ""
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{"id", "kind", "name"} {
		if !strings.Contains(msg, want) {
			t.Errorf("aggregated error %q missing %q", msg, want)
		}
	}
}

func TestStackComponentValidateAcceptsValid(t *testing.T) {
	if err := validComponent().Validate(); err != nil {
		t.Errorf("validComponent().Validate() = %v, want nil", err)
	}
}

// validComponent returns a known-good StackComponent that individual
// tests mutate one field at a time.
func validComponent() StackComponent {
	return StackComponent{
		SchemaVersion: SchemaVersion,
		ID:            "express",
		Kind:          "framework",
		Name:          "express",
		Version:       "4.18.2",
		Capabilities:  []string{"http-routing"},
		DetectionEvidence: []EvidenceRef{
			{File: "package.json", Path: "dependencies.express", Value: "^4.18.2"},
		},
	}
}

func TestStackManifestValidateAcceptsValid(t *testing.T) {
	if err := validManifest().Validate(); err != nil {
		t.Errorf("validManifest().Validate() = %v, want nil", err)
	}
}

func TestStackManifestValidateRejectsTopLevelProblems(t *testing.T) {
	cases := []struct {
		name   string
		m      func(*StackManifest)
		substr string
	}{
		{"wrong schema_version", func(m *StackManifest) { m.SchemaVersion = "9.9.9" }, "schema_version"},
		{"missing archetype", func(m *StackManifest) { m.Archetype = "" }, "archetype"},
		{"unknown archetype", func(m *StackManifest) { m.Archetype = "monolith" }, "archetype"},
		{"empty components", func(m *StackManifest) { m.Components = nil }, "components"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validManifest()
			tc.m(&m)
			err := m.Validate()
			if err == nil {
				t.Fatalf("expected error for %q", tc.name)
			}
			if !strings.Contains(err.Error(), tc.substr) {
				t.Errorf("error %q does not mention %q", err.Error(), tc.substr)
			}
		})
	}
}

func TestStackManifestValidatePrefixesComponentErrorsWithID(t *testing.T) {
	m := validManifest()
	m.Components[0].Name = "" // break the first (named "express")
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "components[express]") {
		t.Errorf("expected error to mention components[express], got %q", err.Error())
	}
}

func TestStackManifestValidatePrefixesByIndexWhenIDMissing(t *testing.T) {
	m := validManifest()
	m.Components[0].ID = ""
	m.Components[0].Name = ""
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "components[0]") {
		t.Errorf("expected error to mention components[0], got %q", err.Error())
	}
}

// validManifest returns a known-good StackManifest with two components.
func validManifest() StackManifest {
	return StackManifest{
		SchemaVersion: SchemaVersion,
		Archetype:     "http-api",
		ApplicableAngles: []enums.ValidationAngle{
			enums.AngleSecurity, enums.AngleBuild, enums.AngleCodeStructure,
			enums.AngleUnitTest, enums.AngleE2ETest, enums.AngleContracts,
			enums.AngleLogs, enums.AngleMetrics, enums.AngleDatabase,
			enums.AnglePerformance,
		},
		Components: []StackComponent{
			validComponent(),
			{
				SchemaVersion: SchemaVersion,
				ID:            "postgres",
				Kind:          "datastore",
				Name:          "postgres",
				Version:       "16",
				Capabilities:  []string{"transactions"},
				DetectionEvidence: []EvidenceRef{
					{File: "docker-compose.yaml", Path: "services.db.image"},
				},
			},
		},
	}
}

func TestLoad_PopulatesApplicableAngles(t *testing.T) {
	m, err := Load(repoPath(t, "schemas/examples/stack-manifest/http-api.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.ApplicableAngles) == 0 {
		t.Fatal("ApplicableAngles is empty after Load")
	}
	wantFirst := enums.AngleSecurity
	if m.ApplicableAngles[0] != wantFirst {
		t.Fatalf("ApplicableAngles[0]=%q, want %q", m.ApplicableAngles[0], wantFirst)
	}
}

func TestLoadGoldenManifestRoundTrips(t *testing.T) {
	path := repoPath(t, "schemas/examples/stack-manifest/http-api.yaml")
	first, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tmp := filepath.Join(t.TempDir(), "round-trip.yaml")
	writeYAML(t, tmp, first)
	second, err := Load(tmp)
	if err != nil {
		t.Fatalf("Load (round-trip): %v", err)
	}
	if first.SchemaVersion != second.SchemaVersion ||
		first.Archetype != second.Archetype ||
		len(first.Components) != len(second.Components) {
		t.Fatalf("round-trip mismatch: %+v vs %+v", first, second)
	}
	for i := range first.Components {
		if !componentsEqual(first.Components[i], second.Components[i]) {
			t.Errorf("component[%d] mismatch", i)
		}
	}
}

func TestLoadAllGoldenStackComponents(t *testing.T) {
	dir := repoPath(t, "schemas/examples/stack-component")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no example files found")
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			_, err := LoadComponent(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Errorf("LoadComponent(%s): %v", e.Name(), err)
			}
		})
	}
}

func TestLoadRejectsDuplicateIDs(t *testing.T) {
	yaml := `schema_version: 1.0.0
archetype: http-api
applicable_angles: [security, build, code-structure, unit-test, e2e-test, contracts, logs, metrics, database, performance]
components:
  - schema_version: 1.0.0
    id: express
    kind: framework
    name: express
    version: "4.18.2"
    capabilities: [http-routing]
    detection_evidence:
      - {file: package.json, path: dependencies.express}
  - schema_version: 1.0.0
    id: express
    kind: library
    name: express-also
    version: "1.0.0"
    capabilities: [http-routing]
    detection_evidence:
      - {file: package.json, path: dependencies.express}
`
	path := writeTempYAML(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected duplicate-id error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "express") {
		t.Errorf("error %q does not name the duplicate id", msg)
	}
	if !strings.Contains(msg, "0") || !strings.Contains(msg, "1") {
		t.Errorf("error %q does not name both occurrences", msg)
	}
}

func TestLoadRejectsBadJSONSchemaShape(t *testing.T) {
	// Missing required top-level field "archetype" — caught by JSON Schema
	// before the Go validator runs.
	yaml := `schema_version: 1.0.0
components:
  - schema_version: 1.0.0
    id: express
    kind: framework
    name: express
    version: "4.18.2"
    capabilities: [http-routing]
    detection_evidence:
      - {file: package.json, path: dependencies.express}
`
	path := writeTempYAML(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected JSON-Schema error")
	}
	if !strings.Contains(err.Error(), "archetype") {
		t.Errorf("error %q should mention archetype", err.Error())
	}
}

func TestLoadComponentRejectsBadJSONSchemaShape(t *testing.T) {
	// detection_evidence using the old string shape — should be rejected
	// by JSON Schema (items must be objects).
	yaml := `schema_version: 1.0.0
id: express
kind: framework
name: express
version: "4.18.2"
capabilities: [http-routing]
detection_evidence:
  - "package.json:dependencies.express"
`
	path := writeTempYAML(t, yaml)
	_, err := LoadComponent(path)
	if err == nil {
		t.Fatal("expected JSON-Schema error for string-form evidence")
	}
}

// --- test helpers ---

func repoPath(t *testing.T, rel string) string {
	t.Helper()
	// Tests run with CWD = the package directory; ../../ reaches the repo root.
	return filepath.Join("..", "..", rel)
}

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fixture.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}

func writeYAML(t *testing.T, path string, m StackManifest) {
	t.Helper()
	b, err := yamlMarshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func componentsEqual(a, b StackComponent) bool {
	if a.SchemaVersion != b.SchemaVersion || a.ID != b.ID || a.Kind != b.Kind ||
		a.Name != b.Name || a.Version != b.Version {
		return false
	}
	if len(a.Capabilities) != len(b.Capabilities) {
		return false
	}
	for i := range a.Capabilities {
		if a.Capabilities[i] != b.Capabilities[i] {
			return false
		}
	}
	if len(a.DetectionEvidence) != len(b.DetectionEvidence) {
		return false
	}
	for i := range a.DetectionEvidence {
		if a.DetectionEvidence[i] != b.DetectionEvidence[i] {
			return false
		}
	}
	return true
}

func TestByIDReturnsPresentComponent(t *testing.T) {
	m := loadGolden(t)
	c, ok := m.ByID("nestjs")
	if !ok {
		t.Fatal("ByID(nestjs) = _, false; want true")
	}
	if c.ID != "nestjs" {
		t.Errorf("ByID returned wrong component: %+v", c)
	}
}

func TestByIDReturnsZeroForAbsent(t *testing.T) {
	m := loadGolden(t)
	c, ok := m.ByID("never-installed")
	if ok {
		t.Errorf("ByID(never-installed) = %+v, true; want zero, false", c)
	}
	if c.ID != "" || c.Kind != "" || c.Name != "" || c.Version != "" {
		t.Errorf("ByID returned non-zero for absent: %+v", c)
	}
}

func TestHasCapability(t *testing.T) {
	m := loadGolden(t)
	if !m.HasCapability("http-routing") {
		t.Error("HasCapability(http-routing) = false; want true")
	}
	if m.HasCapability("graphql-subscriptions") {
		t.Error("HasCapability(graphql-subscriptions) = true; want false")
	}
}

func TestComponentsWithCapabilityPreservesOrder(t *testing.T) {
	m := loadGolden(t)
	got := m.ComponentsWithCapability("http-routing")
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].ID != "nestjs" {
		t.Errorf("got[0].ID = %q, want %q", got[0].ID, "nestjs")
	}
}

func TestComponentsWithCapabilityReturnsEmptyNotNil(t *testing.T) {
	m := loadGolden(t)
	got := m.ComponentsWithCapability("never-declared")
	if got == nil {
		t.Error("got nil; want empty (non-nil) slice")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func loadGolden(t *testing.T) StackManifest {
	t.Helper()
	m, err := Load(repoPath(t, "schemas/examples/stack-manifest/http-api.yaml"))
	if err != nil {
		t.Fatalf("Load golden: %v", err)
	}
	return m
}

func TestLintCapabilitiesWarnsOnUnknown(t *testing.T) {
	m := loadGolden(t)
	known := []string{"http-routing", "middleware", "dependency-injection", "transactions"}
	got := m.LintCapabilities(known)

	// Golden manifest declares: nestjs={dependency-injection, http-routing, middleware},
	// postgres={transactions, row-level-security}.
	// 'row-level-security' is the one unknown capability.
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1; warnings = %+v", len(got), got)
	}
	w := got[0]
	if w.ComponentID != "postgres" {
		t.Errorf("ComponentID = %q, want %q", w.ComponentID, "postgres")
	}
	if w.Capability != "row-level-security" {
		t.Errorf("Capability = %q, want %q", w.Capability, "row-level-security")
	}
	if w.Message == "" {
		t.Error("Message: empty; want a human-readable string")
	}
}

func TestLintCapabilitiesReturnsEmptyWhenAllKnown(t *testing.T) {
	m := loadGolden(t)
	known := []string{
		"http-routing", "middleware", "dependency-injection",
		"transactions", "row-level-security",
	}
	got := m.LintCapabilities(known)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0; warnings = %+v", len(got), got)
	}
}

func TestLoadBytes_RoundTripsExample(t *testing.T) {
	b, err := os.ReadFile("../../schemas/examples/stack-manifest/http-api.yaml")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	m, err := LoadBytes(b)
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if m.Archetype != enums.ArchetypeHTTPAPI {
		t.Fatalf("Archetype=%q, want http-api", m.Archetype)
	}
}

func TestLoadBytes_EnvFileRoundTrips(t *testing.T) {
	// Manifest WITH env_file: field must round-trip through LoadBytes.
	withEnvFile := `schema_version: 1.0.0
archetype: http-api
env_file: .env.local
applicable_angles: [security, build, code-structure, unit-test, e2e-test, contracts, logs, metrics, database, performance]
components:
  - schema_version: 1.0.0
    id: express
    kind: framework
    name: express
    version: "4.18.2"
    capabilities: [http-routing]
    detection_evidence:
      - {file: package.json, path: dependencies.express}
`
	m, err := LoadBytes([]byte(withEnvFile))
	if err != nil {
		t.Fatalf("LoadBytes with env_file: %v", err)
	}
	if m.EnvFile != ".env.local" {
		t.Errorf("EnvFile = %q, want %q", m.EnvFile, ".env.local")
	}

	// Manifest WITHOUT env_file must still load; EnvFile must be empty string.
	withoutEnvFile := `schema_version: 1.0.0
archetype: http-api
applicable_angles: [security, build, code-structure, unit-test, e2e-test, contracts, logs, metrics, database, performance]
components:
  - schema_version: 1.0.0
    id: express
    kind: framework
    name: express
    version: "4.18.2"
    capabilities: [http-routing]
    detection_evidence:
      - {file: package.json, path: dependencies.express}
`
	m2, err := LoadBytes([]byte(withoutEnvFile))
	if err != nil {
		t.Fatalf("LoadBytes without env_file: %v", err)
	}
	if m2.EnvFile != "" {
		t.Errorf("EnvFile = %q, want empty string when not set", m2.EnvFile)
	}
}
