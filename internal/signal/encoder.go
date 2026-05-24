package signal

import (
	"encoding/json"
	"fmt"
	"io"
)

// WriteSignal encodes one Signal as a single JSON Lines record
// (terminated by a newline) and writes it to w. Used by the round-trip
// test and by any future caller that needs to produce JSONL from typed
// Signals (e.g., golden-file generation, sensor-emitter test harnesses).
//
// HTML escaping is disabled because signals are not HTML contexts;
// "<", ">", and "&" should round-trip without &lt;-style escapes.
//
// WriteSignal does not validate sig before encoding. Callers that
// construct a Signal in Go (rather than having obtained it from
// ParseSignals) should call Validate first if schema conformance is
// required — for example, a Signal with a nil Evidence will encode as
// JSON null and fail re-parsing.
func WriteSignal(w io.Writer, sig Signal) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(sig); err != nil {
		return fmt.Errorf("signal: encode: %w", err)
	}
	return nil
}
