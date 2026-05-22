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
