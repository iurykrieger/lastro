package fixture

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/clbanning/mxj/v2"
	"sigs.k8s.io/yaml"
)

// parsePayload eagerly parses raw payload bytes based on content_type.
// Returns a parsed Go value (typically map[string]any or []any for
// structured types) or nil for unstructured / unknown content types.
//
// Returns an error only when content_type is recognized as structured
// but the bytes don't parse.
func parsePayload(contentType string, payload []byte) (any, error) {
	switch {
	case isJSONContentType(contentType):
		var v any
		if err := json.Unmarshal(payload, &v); err != nil {
			return nil, fmt.Errorf("payload: invalid JSON for content_type %q: %w", contentType, err)
		}
		return v, nil
	case isYAMLContentType(contentType):
		var v any
		if err := yaml.Unmarshal(payload, &v); err != nil {
			return nil, fmt.Errorf("payload: invalid YAML for content_type %q: %w", contentType, err)
		}
		return v, nil
	case isXMLContentType(contentType):
		m, err := mxj.NewMapXml(payload)
		if err != nil {
			return nil, fmt.Errorf("payload: invalid XML for content_type %q: %w", contentType, err)
		}
		return map[string]any(m), nil
	}
	return nil, nil
}

func isJSONContentType(ct string) bool {
	return ct == "application/json" || hasMediaTypeSuffix(ct, "+json")
}

func isYAMLContentType(ct string) bool {
	switch ct {
	case "application/yaml", "text/yaml", "application/x-yaml":
		return true
	}
	return false
}

func isXMLContentType(ct string) bool {
	switch ct {
	case "application/xml", "text/xml":
		return true
	}
	return hasMediaTypeSuffix(ct, "+xml")
}

// hasMediaTypeSuffix reports whether ct is of the form "type/subtype+suffix".
// Matches RFC 6839 structured-syntax-suffix conventions (e.g.,
// application/vnd.api+json, application/atom+xml). YAML has no analogous
// suffix convention; isYAMLContentType stays exact-match.
func hasMediaTypeSuffix(ct, suffix string) bool {
	if strings.IndexByte(ct, '/') < 0 {
		return false
	}
	return strings.HasSuffix(ct, suffix)
}
