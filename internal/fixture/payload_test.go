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

func TestParsePayloadYAML(t *testing.T) {
	cases := []string{"application/yaml", "text/yaml", "application/x-yaml"}
	for _, ct := range cases {
		t.Run(ct, func(t *testing.T) {
			got, err := parsePayload(ct, []byte("customer_id: c-001\nqty: 2\n"))
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
		})
	}
}

func TestParsePayloadYAMLMalformedIsError(t *testing.T) {
	_, err := parsePayload("application/yaml", []byte("not: : valid: yaml: :"))
	if err == nil {
		t.Fatal("parsePayload: expected error on malformed YAML, got nil")
	}
}

func TestParsePayloadXML(t *testing.T) {
	cases := []string{"application/xml", "text/xml"}
	for _, ct := range cases {
		t.Run(ct, func(t *testing.T) {
			got, err := parsePayload(ct, []byte(`<order><customer_id>c-001</customer_id><qty>2</qty></order>`))
			if err != nil {
				t.Fatalf("parsePayload: %v", err)
			}
			m, ok := got.(map[string]any)
			if !ok {
				t.Fatalf("parsePayload: got %T, want map[string]any", got)
			}
			// mxj wraps the document under its root element name.
			order, ok := m["order"].(map[string]any)
			if !ok {
				t.Fatalf("expected nested order map, got %T (%+v)", m["order"], m)
			}
			if order["customer_id"] != "c-001" {
				t.Errorf("order.customer_id: got %v, want c-001", order["customer_id"])
			}
		})
	}
}

func TestParsePayloadXMLMalformedIsError(t *testing.T) {
	_, err := parsePayload("application/xml", []byte(`<unclosed`))
	if err == nil {
		t.Fatal("parsePayload: expected error on malformed XML, got nil")
	}
}

func TestParsePayloadUnknownContentTypeReturnsNil(t *testing.T) {
	cases := []string{"text/plain", "application/octet-stream", "", "image/png"}
	for _, ct := range cases {
		t.Run(ct, func(t *testing.T) {
			got, err := parsePayload(ct, []byte("anything goes here"))
			if err != nil {
				t.Fatalf("parsePayload(%q): unexpected error %v", ct, err)
			}
			if got != nil {
				t.Errorf("parsePayload(%q): got %v, want nil", ct, got)
			}
		})
	}
}

func TestParsePayloadJSONSuffixMatch(t *testing.T) {
	// application/vnd.api+json should dispatch to JSON.
	got, err := parsePayload("application/vnd.api+json", []byte(`{"ok":true}`))
	if err != nil {
		t.Fatalf("parsePayload: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok || m["ok"] != true {
		t.Errorf("expected JSON parse via +json suffix; got %v (%T)", got, got)
	}
}

func TestParsePayloadXMLSuffixMatch(t *testing.T) {
	got, err := parsePayload("application/atom+xml", []byte(`<a><b>1</b></a>`))
	if err != nil {
		t.Fatalf("parsePayload: %v", err)
	}
	if _, ok := got.(map[string]any); !ok {
		t.Errorf("expected XML parse via +xml suffix; got %T", got)
	}
}
