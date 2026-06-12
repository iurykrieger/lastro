package executor

import (
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// envView is the merged ambient environment a sensor run exposes to its
// steps: the manifest-declared env_file's values with the harness host
// environment winning on conflict (standard dotenv semantics — the file
// fills gaps, CI stays in control). The zero value behaves as "no
// env_file declared".
type envView struct {
	ambient map[string]string // env_file values NOT shadowed by the host env
	source  string            // env_file path for diagnostics; "" when none declared
}

// loadEnvView loads the dotenv file at path. path=="" means no env_file
// is declared. A declared-but-absent file degrades to an empty view with
// fileMissing=true (the requirement checks still catch real gaps); an
// unparseable file is an error the run surfaces as an inconclusive
// env-file-invalid signal.
func loadEnvView(path string) (view envView, fileMissing bool, err error) {
	if path == "" {
		return envView{}, false, nil
	}
	view = envView{ambient: map[string]string{}}
	view.source = path
	raw, err := godotenv.Read(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return view, true, nil
		}
		return envView{}, false, fmt.Errorf("executor: parse env_file %s: %w", path, err)
	}
	for k, v := range raw {
		if _, shadowed := os.LookupEnv(k); shadowed {
			continue
		}
		view.ambient[k] = v
	}
	return view, false, nil
}

// lookup resolves name from the host environment first, then the ambient
// env_file values.
func (v envView) lookup(name string) (string, bool) {
	if val, ok := os.LookupEnv(name); ok {
		return val, true
	}
	val, ok := v.ambient[name]
	return val, ok
}
