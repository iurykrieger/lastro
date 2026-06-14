// internal/environment/parse_compose.go
package environment

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

var composeCandidates = []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}

// parseCompose reads the first present compose file and returns its services
// verbatim plus the filename used (for provided_by grounding). Absent → empty.
func parseCompose(repoDir string) (map[string]ComposeService, string, error) {
	for _, name := range composeCandidates {
		b, err := os.ReadFile(filepath.Join(repoDir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		var doc struct {
			Services map[string]ComposeService `json:"services"`
		}
		if err := yaml.Unmarshal(b, &doc); err != nil {
			return nil, "", err
		}
		if doc.Services == nil {
			doc.Services = map[string]ComposeService{}
		}
		return doc.Services, name, nil
	}
	return map[string]ComposeService{}, "", nil
}

// parseDotenvKeys returns the key names declared in .env.example (preferred) or
// .env. Values are ignored. Absent → empty.
func parseDotenvKeys(repoDir string) []string {
	for _, name := range []string{".env.example", ".env.local", ".env"} {
		f, err := os.Open(filepath.Join(repoDir, name))
		if err != nil {
			continue
		}
		defer f.Close()
		var keys []string
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, _, ok := strings.Cut(line, "=")
			key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
			if ok && key != "" {
				keys = append(keys, key)
			}
		}
		return keys
	}
	return nil
}
