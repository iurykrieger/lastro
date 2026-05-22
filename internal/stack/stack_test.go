package stack

import (
	"strings"
	"testing"
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
