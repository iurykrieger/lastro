package fixturebinder

import "testing"

func TestExtensionFor(t *testing.T) {
	cases := []struct {
		contentType string
		want        string
	}{
		{"application/json", ".json"},
		{"application/vnd.api+json", ".json"},
		{"application/yaml", ".yaml"},
		{"text/yaml", ".yaml"},
		{"application/x-yaml", ".yaml"},
		{"application/xml", ".xml"},
		{"text/xml", ".xml"},
		{"application/atom+xml", ".xml"},
		{"application/octet-stream", ".bin"},
		{"image/png", ".bin"},
		{"", ".bin"},
		{"text/plain", ".bin"},
	}
	for _, c := range cases {
		got := extensionFor(c.contentType)
		if got != c.want {
			t.Errorf("extensionFor(%q) = %q, want %q", c.contentType, got, c.want)
		}
	}
}
