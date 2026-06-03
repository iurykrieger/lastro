package main

import (
	"io"
	"log/slog"
	"os"
)

// newLogger builds the slog handler for one CLI invocation. format
// selects JSON vs text; quiet raises the floor to Error; verbose drops
// it to Debug. Logs go to stderr; stdout is reserved for output.
func newLogger(format string, quiet, verbose bool, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stderr
	}
	level := slog.LevelInfo
	switch {
	case quiet:
		level = slog.LevelError
	case verbose:
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}

	var h slog.Handler
	switch format {
	case "json":
		h = slog.NewJSONHandler(w, opts)
	default:
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(h)
}
