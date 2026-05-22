package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"
)

var entities = []string{
	"stack-component", "entry-point", "use-case", "fixture",
	"sensor", "signal", "aggregate-signal", "validation-policy",
	"stack-manifest",
}

var enums = []string{
	"validation-angles", "archetypes", "sensor-kinds", "sensor-natures",
	"signal-output-types", "fixture-roles", "verdicts", "termination-reasons",
}

const baseURL = "https://lastro.dev/harness/schemas/"

func loadYAMLAsAny(path string) (any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	j, err := yaml.YAMLToJSON(b)
	if err != nil {
		return nil, fmt.Errorf("yaml->json %s: %w", path, err)
	}
	var v any
	if err := json.Unmarshal(j, &v); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return v, nil
}

func main() {
	var errs []string
	c := jsonschema.NewCompiler()

	// Register every entity schema by its canonical URL so $refs resolve.
	for _, e := range entities {
		path := filepath.Join("schemas", e+".yaml")
		doc, err := loadYAMLAsAny(path)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if err := c.AddResource(baseURL+e+".yaml", doc); err != nil {
			errs = append(errs, fmt.Sprintf("register %s: %v", path, err))
		}
	}

	// Compile each entity schema — this is the "is itself a valid JSON Schema 2020-12" check.
	schemas := map[string]*jsonschema.Schema{}
	for _, e := range entities {
		sch, err := c.Compile(baseURL + e + ".yaml")
		if err != nil {
			errs = append(errs, fmt.Sprintf("compile %s: %v", e, err))
			continue
		}
		schemas[e] = sch
		fmt.Printf("OK schemas/%s.yaml is a valid JSON Schema 2020-12\n", e)
	}

	// Compile _meta.yaml and validate each enum file against it.
	metaPath := "schemas/enums/_meta.yaml"
	metaDoc, err := loadYAMLAsAny(metaPath)
	if err != nil {
		errs = append(errs, err.Error())
	} else {
		metaURL := baseURL + "enums/_meta.yaml"
		if err := c.AddResource(metaURL, metaDoc); err != nil {
			errs = append(errs, fmt.Sprintf("register %s: %v", metaPath, err))
		}
		metaSch, err := c.Compile(metaURL)
		if err != nil {
			errs = append(errs, fmt.Sprintf("compile _meta: %v", err))
		} else {
			for _, en := range enums {
				p := filepath.Join("schemas", "enums", en+".yaml")
				doc, err := loadYAMLAsAny(p)
				if err != nil {
					errs = append(errs, err.Error())
					continue
				}
				if err := metaSch.Validate(doc); err != nil {
					errs = append(errs, fmt.Sprintf("FAIL %s: %v", p, err))
				} else {
					fmt.Printf("OK %s matches _meta.yaml\n", p)
				}
			}
		}
	}

	// Validate each example against its entity schema.
	for _, e := range entities {
		sch, ok := schemas[e]
		if !ok {
			continue
		}
		dir := filepath.Join("schemas", "examples", e)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		var files []string
		if walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".yaml") {
				files = append(files, path)
			}
			return nil
		}); walkErr != nil {
			errs = append(errs, fmt.Sprintf("walk %s: %v", dir, walkErr))
			continue
		}
		sort.Strings(files)
		for _, p := range files {
			doc, err := loadYAMLAsAny(p)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			if err := sch.Validate(doc); err != nil {
				errs = append(errs, fmt.Sprintf("FAIL %s: %v", p, err))
			} else {
				fmt.Printf("OK %s passes %s.yaml\n", p, e)
			}
		}
	}

	if len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "\n"+strings.Join(errs, "\n"))
		os.Exit(1)
	}
	fmt.Println("\nAll schemas, enums, and examples validated.")
}
