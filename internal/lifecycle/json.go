package lifecycle

import "encoding/json"

func jsonEncode(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func jsonDecode(b []byte, v any) error {
	return json.Unmarshal(b, v)
}
