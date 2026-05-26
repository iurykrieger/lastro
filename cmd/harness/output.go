package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// CLISchemaVersion is the wire-format version of harness CLI output.
// Bump on breaking schema changes. CI tooling reads this to gate
// parsing logic.
const CLISchemaVersion = "1.0.0"

// RunResult is the top-level structure printed to stdout for every
// CLI invocation that produces structured output. Subcommands populate
// Result with their own shape (see §6.2 for validate / heal).
type RunResult struct {
	CLISchemaVersion string    `json:"cli_schema_version"`
	RunID            string    `json:"run_id"`
	Command          string    `json:"command"`
	Args             []string  `json:"args"`
	StartedAt        time.Time `json:"started_at"`
	EndedAt          time.Time `json:"ended_at"`
	DurationMs       int64     `json:"duration_ms"`
	HarnessVersion   string    `json:"harness_version"`
	Result           any       `json:"result"`
}

// Render writes the result to w in the requested format. Format must
// be "json" or "text"; anything else falls back to text.
func Render(w io.Writer, r *RunResult, format string, renderText func(io.Writer, *RunResult) error) error {
	switch format {
	case "json":
		return renderJSON(w, r)
	default:
		if renderText == nil {
			return fmt.Errorf("output: no text renderer registered for command %q", r.Command)
		}
		return renderText(w, r)
	}
}

func renderJSON(w io.Writer, r *RunResult) error {
	// Indent for human-friendliness; CI tooling will still parse it.
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return fmt.Errorf("output: encode json: %w", err)
	}
	return nil
}
