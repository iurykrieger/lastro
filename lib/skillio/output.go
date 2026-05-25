package skillio

import (
	"encoding/json"
	"io"
)

// EmitJSON writes a single JSON object to w followed by a newline.
// Used for stdout streams: signals, terminal AggregateSignal, UseCaseVerdict.
func EmitJSON(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err = w.Write([]byte{'\n'})
	return err
}

// EmitError writes a ScriptError envelope to w (typically stderr) as a
// single-line JSON object followed by a newline. Best-effort: encoding
// errors are dropped because the caller is already exiting.
func EmitError(w io.Writer, code, message string, details map[string]any) {
	_ = EmitJSON(w, NewScriptError(code, message, details))
}
