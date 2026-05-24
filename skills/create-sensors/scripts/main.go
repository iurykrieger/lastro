// Command create-sensors-script is invoked by the /create-sensors slash
// command. Reads an LLM-emitted sensor YAML from --file and hands it to
// internal/sensor.Persist. On success: exit 0, nothing on stdout. On
// validation failure: exit 2 with a JSON persisterror.Error on stdout.
// On script-level failure (bad args, missing file): exit 1 with a
// message on stderr.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/iurykrieger/lastro/internal/persisterror"
	"github.com/iurykrieger/lastro/internal/sensor"
)

func main() {
	file := flag.String("file", "", "Path to the LLM-emitted sensor YAML")
	harnessDir := flag.String("harness-dir", ".harness", "Target .harness directory")
	flag.Parse()
	if *file == "" {
		fmt.Fprintln(os.Stderr, "missing required --file")
		os.Exit(1)
	}
	content, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read input:", err)
		os.Exit(1)
	}
	if err := sensor.Persist(content, *harnessDir); err != nil {
		var pe *persisterror.Error
		if errors.As(err, &pe) {
			_ = json.NewEncoder(os.Stdout).Encode(pe)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
