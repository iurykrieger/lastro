package executor

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRawLog_WriteAnnotated_Format(t *testing.T) {
	dir := t.TempDir()
	rl, err := newRawLog(filepath.Join(dir, "raw.log"), fixedNow(t))
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Close()

	rl.WriteAnnotated(1, "stdout", []byte(`{"ok":true}`))
	rl.WriteAnnotated(1, "stderr", []byte("oops"))
	rl.WriteAnnotated(2, "parse-error", []byte("bad json"))

	rl.Close()
	data, err := os.ReadFile(filepath.Join(dir, "raw.log"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		"[2026-05-24T10:00:00.000000000Z step-01 stdout] {\"ok\":true}",
		"[2026-05-24T10:00:00.000000000Z step-01 stderr] oops",
		"[2026-05-24T10:00:00.000000000Z step-02 parse-error] bad json",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("raw.log missing line %q\nfull output:\n%s", want, got)
		}
	}
}

func TestRawLog_ConcurrentWritersDoNotTear(t *testing.T) {
	dir := t.TempDir()
	rl, err := newRawLog(filepath.Join(dir, "raw.log"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Close()

	const writers = 8
	const linesPerWriter = 200
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			payload := strings.Repeat("x", 200)
			for i := 0; i < linesPerWriter; i++ {
				rl.WriteAnnotated(id+1, "stdout", []byte(payload))
			}
		}(w)
	}
	wg.Wait()
	rl.Close()

	data, _ := os.ReadFile(filepath.Join(dir, "raw.log"))
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if got, want := len(lines), writers*linesPerWriter; got != want {
		t.Errorf("line count = %d, want %d", got, want)
	}
	// No line should be torn — every line starts with "[".
	for i, line := range lines {
		if !strings.HasPrefix(line, "[") {
			t.Fatalf("line %d does not start with '[': %q", i, line)
		}
	}
}

func TestRawLog_RedactsRegisteredValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raw.log")
	rl, err := newRawLog(path, fixedNow(t))
	if err != nil {
		t.Fatal(err)
	}
	red := &redactor{}
	red.Add("supersecret")
	rl.red = red
	rl.WriteAnnotated(1, "stdout", []byte("token=supersecret"))
	_ = rl.Close()
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "supersecret") {
		t.Errorf("raw.log leaked the secret: %s", b)
	}
	if !strings.Contains(string(b), "token=***") {
		t.Errorf("raw.log missing masked content: %s", b)
	}
}

// fixedNow returns a Now() that always returns 2026-05-24T10:00:00.000000000Z.
func fixedNow(t *testing.T) func() time.Time {
	t.Helper()
	ts, _ := time.Parse(time.RFC3339Nano, "2026-05-24T10:00:00.000000000Z")
	return func() time.Time { return ts }
}
