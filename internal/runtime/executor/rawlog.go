package executor

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"time"
)

// rawLog is a line-annotated, mutex-serialized writer for the per-run
// raw.log file. Multiple goroutines (stdout reader, stderr reader) write
// concurrently; the mutex guarantees no line is torn.
type rawLog struct {
	mu  sync.Mutex
	f   *os.File
	w   *bufio.Writer
	now func() time.Time
}

func newRawLog(path string, now func() time.Time) (*rawLog, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("rawlog: open %s: %w", path, err)
	}
	if now == nil {
		now = time.Now
	}
	return &rawLog{
		f:   f,
		w:   bufio.NewWriter(f),
		now: now,
	}, nil
}

// WriteAnnotated writes one annotated line of the form
//
//	[<RFC3339Nano timestamp> step-NN <stream>] <content>\n
//
// stepIdx is 1-based and zero-padded to two digits. stream is one of
// "stdout", "stderr", "parse-error", "exit-nonzero".
func (r *rawLog) WriteAnnotated(stepIdx int, stream string, content []byte) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ts := r.now().UTC().Format(time.RFC3339Nano)
	// Pad to 9 fractional digits for stable golden tests; Go's RFC3339Nano
	// trims trailing zeros, so pad explicitly.
	ts = padNanos(ts)
	fmt.Fprintf(r.w, "[%s step-%02d %s] %s\n", ts, stepIdx, stream, content)
}

// Close flushes the buffer and closes the file. Safe to call multiple times.
func (r *rawLog) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.w.Flush()
	if cerr := r.f.Close(); err == nil {
		err = cerr
	}
	r.f = nil
	return err
}

// padNanos ensures the fractional-second portion of an RFC3339Nano
// timestamp is exactly 9 digits, so golden test outputs are byte-stable.
func padNanos(ts string) string {
	// Find the dot.
	dot := -1
	for i := 0; i < len(ts); i++ {
		if ts[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		// No fractional part — insert ".000000000" before the timezone.
		// Find the timezone suffix start (Z or +/-).
		tz := len(ts)
		for i := len(ts) - 1; i >= 0; i-- {
			if ts[i] == 'Z' || ts[i] == '+' || ts[i] == '-' {
				tz = i
				break
			}
		}
		return ts[:tz] + ".000000000" + ts[tz:]
	}
	// Already has a dot; count fractional digits.
	tz := len(ts)
	for i := dot + 1; i < len(ts); i++ {
		if ts[i] == 'Z' || ts[i] == '+' || (ts[i] == '-' && i > dot+3) {
			tz = i
			break
		}
	}
	fracLen := tz - dot - 1
	if fracLen >= 9 {
		return ts
	}
	pad := ""
	for i := fracLen; i < 9; i++ {
		pad += "0"
	}
	return ts[:tz] + pad + ts[tz:]
}
