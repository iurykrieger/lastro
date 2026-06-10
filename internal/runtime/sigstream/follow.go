// Package sigstream tails a sensor's signals.jsonl file, decoding each
// JSON line into a lightweight Decoded record and delivering it to a
// callback. It is the read side of "attach to a running observational
// service": the writer is the service's watcher (internal/runtime/executor),
// which flushes one JSON signal per line.
package sigstream

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

// Decoded is the subset of a signal line the attach machinery needs.
// Evidence is kept raw so consumers can read matched_line or named groups.
type Decoded struct {
	ObservationKey string
	MatchedLine    string
	Raw            map[string]any
}

// Follow tails the JSONL file at path. For every newly appended line that
// decodes as a JSON object it calls onSignal. Follow returns nil when:
//   - onSignal returns true (satisfied), or
//   - stop is closed, or
//   - ctx is done.
//
// A not-yet-created file is treated as empty and retried every poll interval.
// Passing a nil stop channel is valid: it simply never fires, so cancellation
// relies on ctx.
func Follow(ctx context.Context, path string, poll time.Duration, stop <-chan struct{}, onSignal func(Decoded) (done bool)) error {
	if poll <= 0 {
		poll = 50 * time.Millisecond
	}
	var offset int64

	timer := time.NewTimer(poll)
	defer timer.Stop()
	// Start drained; we Reset before each wait.
	if !timer.Stop() {
		<-timer.C
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stop:
			return nil
		default:
		}

		n, done, err := drain(path, &offset, onSignal)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if n == 0 {
			timer.Reset(poll)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return ctx.Err()
			case <-stop:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return nil
			case <-timer.C:
			}
		}
	}
}

// drain reads any new whole lines from path starting at *offset, advances
// *offset past consumed bytes, and invokes onSignal for each decoded object.
// Returns the count of successfully decoded (delivered) signals and whether
// onSignal asked to stop. *offset is advanced for every complete line
// (valid or malformed) so malformed lines are not re-read on the next pass.
func drain(path string, offset *int64, onSignal func(Decoded) bool) (int, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, err
	}
	defer f.Close()

	if _, err := f.Seek(*offset, io.SeekStart); err != nil {
		return 0, false, err
	}
	r := bufio.NewReader(f)
	count := 0
	for {
		line, err := r.ReadBytes('\n')
		// Complete line ends in '\n'; a partial trailing line leaves *offset unadvanced so it is re-read next pass.
		if len(line) > 0 && line[len(line)-1] == '\n' {
			*offset += int64(len(line))
			if d, ok := decode(line); ok {
				count++
				if onSignal(d) {
					return count, true, nil
				}
			}
		}
		if err != nil {
			break
		}
	}
	return count, false, nil
}

func decode(line []byte) (Decoded, bool) {
	var m map[string]any
	if err := json.Unmarshal(line, &m); err != nil {
		return Decoded{}, false
	}
	d := Decoded{Raw: m}
	if ev, ok := m["evidence"].(map[string]any); ok {
		if k, ok := ev["observation_key"].(string); ok {
			d.ObservationKey = k
		}
		if ml, ok := ev["matched_line"].(string); ok {
			d.MatchedLine = ml
		}
	}
	return d, true
}
