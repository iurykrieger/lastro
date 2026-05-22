package fixture

import "testing"

func TestParsePayloadJSON(t *testing.T) {
	got, err := parsePayload("application/json", []byte(`{"customer_id":"c-001","qty":2}`))
	if err != nil {
		t.Fatalf("parsePayload: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("parsePayload: got %T, want map[string]any", got)
	}
	if m["customer_id"] != "c-001" {
		t.Errorf("customer_id: got %v, want c-001", m["customer_id"])
	}
}

func TestParsePayloadJSONMalformedIsError(t *testing.T) {
	_, err := parsePayload("application/json", []byte(`{not valid json`))
	if err == nil {
		t.Fatal("parsePayload: expected error on malformed JSON, got nil")
	}
}
