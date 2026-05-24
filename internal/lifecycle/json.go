package lifecycle

import "encoding/json"

func jsonEncode(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}
