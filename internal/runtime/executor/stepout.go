package executor

import (
	"bufio"
	"os"
	"strings"
)

// parseStepOutputFile reads name=value lines (last write wins). Lines without
// '=' are ignored. Missing file → empty map, no error.
func parseStepOutputFile(path string) (map[string]string, error) {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		out[line[:i]] = line[i+1:]
	}
	return out, sc.Err()
}

func stepOutEnvName(stepID, name string) string {
	up := func(s string) string { return strings.ToUpper(strings.ReplaceAll(s, "-", "_")) }
	return "HARNESS_STEPOUT_" + up(stepID) + "_" + up(name)
}
