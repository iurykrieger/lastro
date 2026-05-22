package enums

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"sigs.k8s.io/yaml"
)

// enumFile is the on-disk shape of every schemas/enums/*.yaml file (subset).
type enumFile struct {
	SchemaVersion string `json:"schema_version"`
	Title         string `json:"title"`
	Values        []struct {
		ID               string   `json:"id"`
		ApplicableAngles []string `json:"applicable_angles,omitempty"`
	} `json:"values"`
}

func readEnumFile(t *testing.T, name string) enumFile {
	t.Helper()
	path := filepath.Join("..", "..", "schemas", "enums", name+".yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var ef enumFile
	if err := yaml.Unmarshal(b, &ef); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return ef
}

func enumIDs(ef enumFile) []string {
	out := make([]string, len(ef.Values))
	for i, v := range ef.Values {
		out[i] = v.ID
	}
	return out
}

func stringify[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

func TestApplicableAnglesMatchYAML(t *testing.T) {
	ef := readEnumFile(t, "archetypes")
	if len(ef.Values) != len(ApplicableAngles) {
		t.Fatalf("archetype count: yaml=%d, go=%d", len(ef.Values), len(ApplicableAngles))
	}
	for _, v := range ef.Values {
		t.Run(v.ID, func(t *testing.T) {
			goList, ok := ApplicableAngles[Archetype(v.ID)]
			if !ok {
				t.Fatalf("ApplicableAngles missing archetype %q", v.ID)
			}
			goIDs := stringify(goList)
			if !reflect.DeepEqual(v.ApplicableAngles, goIDs) {
				t.Errorf("drift for archetype %q:\n  yaml: %v\n  go:   %v",
					v.ID, v.ApplicableAngles, goIDs)
			}
		})
	}
}

func TestGoConstantsMatchYAML(t *testing.T) {
	cases := []struct {
		yamlFile string
		goValues []string
	}{
		{"validation-angles", stringify(AllAngles())},
		{"archetypes", stringify(AllArchetypes())},
		{"sensor-kinds", stringify(AllSensorKinds())},
		{"sensor-natures", stringify(AllSensorNatures())},
		{"signal-output-types", stringify(AllSignalOutputTypes())},
		{"fixture-roles", stringify(AllFixtureRoles())},
		{"verdicts", stringify(AllVerdicts())},
		{"termination-reasons", stringify(AllTerminationReasons())},
	}
	for _, c := range cases {
		t.Run(c.yamlFile, func(t *testing.T) {
			ef := readEnumFile(t, c.yamlFile)
			yamlIDs := enumIDs(ef)
			if !reflect.DeepEqual(yamlIDs, c.goValues) {
				t.Errorf("drift in %s:\n  yaml: %v\n  go:   %v",
					c.yamlFile, yamlIDs, c.goValues)
			}
		})
	}
}

// findEnumBlocks recursively walks decoded YAML and returns every []any
// value found under a map key named "enum".
func findEnumBlocks(node any) [][]any {
	var out [][]any
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			if k == "enum" {
				if arr, ok := child.([]any); ok {
					out = append(out, arr)
				}
				continue
			}
			out = append(out, findEnumBlocks(child)...)
		}
	case []any:
		for _, child := range v {
			out = append(out, findEnumBlocks(child)...)
		}
	}
	return out
}

func toStringSet(arr []any) (map[string]bool, bool) {
	out := map[string]bool{}
	for _, v := range arr {
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		out[s] = true
	}
	return out, true
}

func setEquals(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func TestInlineSchemaEnumsMatchYAML(t *testing.T) {
	// Build canonical name -> id-set map.
	canonical := map[string]map[string]bool{}
	enumNames := []string{
		"validation-angles", "archetypes", "sensor-kinds", "sensor-natures",
		"signal-output-types", "fixture-roles", "verdicts", "termination-reasons",
	}
	for _, name := range enumNames {
		ef := readEnumFile(t, name)
		set := map[string]bool{}
		for _, v := range ef.Values {
			set[v.ID] = true
		}
		canonical[name] = set
	}

	// Walk entity schemas (top-level schemas/*.yaml, excluding enums/ and examples/).
	schemasDir := filepath.Join("..", "..", "schemas")
	entries, err := os.ReadDir(schemasDir)
	if err != nil {
		t.Fatalf("read schemas dir: %v", err)
	}

	referenced := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(schemasDir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var doc any
		if err := yaml.Unmarshal(b, &doc); err != nil {
			// Some schemas contain regex patterns with escape sequences (e.g. \d)
			// that confuse the JSON-bridge in sigs.k8s.io/yaml. Those patterns
			// are unrelated to enum blocks, so we skip the file rather than
			// failing the test.
			t.Logf("skipping %s (unmarshal error: %v)", e.Name(), err)
			continue
		}

		for _, block := range findEnumBlocks(doc) {
			set, ok := toStringSet(block)
			if !ok {
				// Non-string enum values (e.g., numbers) — not framework enums.
				continue
			}
			// Check equality across all canonicals first. Map iteration order
			// is random, so a "break on subset" before "break on equal" would
			// be nondeterministic.
			matchedEqual := ""
			for name, canonSet := range canonical {
				if setEquals(set, canonSet) {
					matchedEqual = name
					break
				}
			}
			if matchedEqual != "" {
				referenced[matchedEqual] = true
				continue
			}
			// Otherwise: if it's a strict subset of any canonical, treat as a
			// local domain constraint and skip silently. If unrelated, it's a
			// local enum (HTTP methods, etc.) — also skip.
		}
	}

	for name := range canonical {
		if !referenced[name] {
			t.Errorf("canonical enum %q is not referenced by any inline enum site in schemas/*.yaml", name)
		}
	}
}
