package aggregate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGoldenFilesRoundTrip(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", e.Name()))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			parsed, err := ParseAggregate(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("first parse: %v", err)
			}
			out, err := json.Marshal(parsed)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			reparsed, err := ParseAggregate(bytes.NewReader(out))
			if err != nil {
				t.Fatalf("second parse: %v\n  json: %s", err, out)
			}
			if !reflect.DeepEqual(parsed, reparsed) {
				t.Errorf("round-trip mismatch for %s", e.Name())
			}
		})
	}
}
