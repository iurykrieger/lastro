package signal

import (
	"bytes"
	"reflect"
	"testing"
)

// TestWriteSignal_RoundTripStable verifies the spec's round-trip
// contract: parse → encode → parse again is *semantically* stable.
// The intermediate byte form may differ between input and output
// (map iteration order, timestamp formatting), but the parsed Signal
// values must compare equal via reflect.DeepEqual.
func TestWriteSignal_RoundTripStable(t *testing.T) {
	f := openTestdata(t, "mixed.jsonl")
	originals, errs := collectSignals(ParseSignals(f))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors parsing fixture: %v", errs)
	}
	if len(originals) != 3 {
		t.Fatalf("fixture should yield 3 signals, got %d", len(originals))
	}

	for i, orig := range originals {
		var buf bytes.Buffer
		if err := WriteSignal(&buf, orig); err != nil {
			t.Errorf("originals[%d]: WriteSignal: %v", i, err)
			continue
		}

		reparsed, errs := collectSignals(ParseSignals(&buf))
		if len(errs) != 0 {
			t.Errorf("originals[%d]: re-parse errors: %v", i, errs)
			continue
		}
		if len(reparsed) != 1 {
			t.Errorf("originals[%d]: expected 1 reparsed signal, got %d", i, len(reparsed))
			continue
		}
		if !reflect.DeepEqual(orig, reparsed[0]) {
			t.Errorf("originals[%d] not DeepEqual after round-trip\n original: %+v\n reparsed: %+v", i, orig, reparsed[0])
		}
	}
}

// TestWriteSignal_WritesNewline confirms the encoder terminates each
// record with a newline so the output is valid JSON Lines.
func TestWriteSignal_WritesNewline(t *testing.T) {
	var buf bytes.Buffer
	sig := validSignal()
	if err := WriteSignal(&buf, sig); err != nil {
		t.Fatalf("WriteSignal: %v", err)
	}
	data := buf.Bytes()
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Errorf("WriteSignal output should end with '\\n', got: %q", data)
	}
}
