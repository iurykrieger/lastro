package fixture

import (
	"encoding/json"
	"fmt"
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
	}
	return nil, nil
}

func isJSONContentType(ct string) bool {
	return ct == "application/json"
}
