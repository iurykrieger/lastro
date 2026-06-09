package sigstream

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	for _, l := range lines {
		if _, err := f.WriteString(l + "\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

func TestFollow_DeliversExistingSignalsThenStopsOnDone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "signals.jsonl")
	writeLines(t,
		path,
		`{"schema_version":"1.0.0","sensor_id":"run-dev","angle":"environment","emitted_at":"2026-06-09T00:00:00Z","verdict":"pass","confidence":1,"evidence":{"observation_key":"ready","matched_line":"ready - started server"}}`,
	)

	var seen []string
	err := Follow(context.Background(), path, 10*time.Millisecond, nil, func(s Decoded) (done bool) {
		seen = append(seen, s.ObservationKey)
		return s.ObservationKey == "ready" // satisfied on ready
	})
	if err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if len(seen) != 1 || seen[0] != "ready" {
		t.Fatalf("seen = %v, want [ready]", seen)
	}
}

func TestFollow_StopsWhenStopChannelClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "signals.jsonl") // never created
	stop := make(chan struct{})
	close(stop)
	if err := Follow(context.Background(), path, 10*time.Millisecond, stop, func(Decoded) bool { return false }); err != nil {
		t.Fatalf("Follow on closed stop: %v", err)
	}
}

func TestFollow_PicksUpAppendedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "signals.jsonl")
	writeLines(t, path, `{"evidence":{"observation_key":"log-line","matched_line":"booting"}}`)

	go func() {
		time.Sleep(30 * time.Millisecond)
		writeLines(t, path, `{"evidence":{"observation_key":"ready","matched_line":"ready - started"}}`)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var keys []string
	err := Follow(ctx, path, 10*time.Millisecond, nil, func(s Decoded) bool {
		keys = append(keys, s.ObservationKey)
		return s.ObservationKey == "ready"
	})
	if err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if len(keys) != 2 || keys[1] != "ready" {
		t.Fatalf("keys = %v, want [log-line ready]", keys)
	}
}
