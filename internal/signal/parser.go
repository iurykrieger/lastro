package signal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"iter"
)

// maxSignalLineBytes caps the size of a single JSON Lines record. The
// scanner's max-token-size is set to this value; a line exceeding it
// surfaces as a reader-level error (wrapped bufio.ErrTooLong). Signals
// are summary records, not log dumps — 1 MiB is generous.
const maxSignalLineBytes = 1 << 20

// Compile-time signature assertion. Pure type-check; never invokes the
// function. If ParseSignals' signature ever drifts from
// func(io.Reader) iter.Seq2[Signal, error], the build breaks here.
var _ func(io.Reader) iter.Seq2[Signal, error] = ParseSignals

// ParseSignals streams a JSON Lines signal sequence from r and yields
// each record as (Signal, error). Blank lines are skipped silently.
//
// Per-line errors (decode failure, schema-validation failure,
// typed-decode failure) yield (Signal{}, error) and iteration
// continues to the next line — the caller decides abort vs. skip.
//
// Reader-level errors (I/O failure, line exceeds maxSignalLineBytes)
// yield once at the end and terminate the stream.
//
// The reader is not closed; the caller owns its lifetime. Stopping
// iteration early (yield returns false) cleanly exits without
// consuming the remainder of r.
func ParseSignals(r io.Reader) iter.Seq2[Signal, error] {
	return func(yield func(Signal, error) bool) {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64*1024), maxSignalLineBytes)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			sig, err := DecodeLine(line)
			if !yield(sig, err) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			yield(Signal{}, fmt.Errorf("signal: scan: %w", err))
		}
	}
}

// DecodeLine runs the per-line three-phase pipeline used by ParseSignals
// for one JSON Lines record: JSON decode → schema validation → typed
// decode. Returns the zero Signal and a wrapped error on any phase
// failure. Exposed so streaming consumers (e.g., the executor) can
// interleave decoding with their own per-line bookkeeping.
func DecodeLine(line []byte) (Signal, error) {
	var instance any
	if err := json.Unmarshal(line, &instance); err != nil {
		return Signal{}, fmt.Errorf("signal: decode line: %w", err)
	}
	s, err := compiledSchema()
	if err != nil {
		return Signal{}, err
	}
	if err := s.Validate(instance); err != nil {
		return Signal{}, fmt.Errorf("signal: schema: %w", err)
	}
	var sig Signal
	if err := json.Unmarshal(line, &sig); err != nil {
		return Signal{}, fmt.Errorf("signal: decode typed: %w", err)
	}
	return sig, nil
}
