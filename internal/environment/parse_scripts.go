// internal/environment/parse_scripts.go
package environment

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// parsePackageScripts reads package.json's "scripts" object. Absent file →
// empty map, no error (graceful degradation).
func parsePackageScripts(repoDir string) (map[string]string, error) {
	b, err := os.ReadFile(filepath.Join(repoDir, "package.json"))
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		return nil, err
	}
	if pkg.Scripts == nil {
		return map[string]string{}, nil
	}
	return pkg.Scripts, nil
}

// parseProcfile reads a Procfile (`name: command` per line). Absent → empty.
func parseProcfile(repoDir string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(filepath.Join(repoDir, "Procfile"))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, cmd, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		out[strings.TrimSpace(name)] = strings.TrimSpace(cmd)
	}
	return out
}

// parseMakeTargets reads `target:` lines from a Makefile. The recipe body is
// not captured — generation invokes `make <target>` (the target name is the
// grounding); we store the target with a "make <name>" placeholder command so
// Resolve has a non-empty value to return.
func parseMakeTargets(repoDir string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(filepath.Join(repoDir, "Makefile"))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "#") {
			continue
		}
		target, _, ok := strings.Cut(line, ":")
		target = strings.TrimSpace(target)
		if !ok || target == "" || strings.ContainsAny(target, " =.") {
			continue
		}
		out[target] = "make " + target
	}
	return out
}
