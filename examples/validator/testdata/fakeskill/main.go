// fakeskill emits canned /validate-use-case JSONL for ValidateAll unit
// tests. It treats argv[1] as the use case id and selects a scripted
// response from the FAKESKILL_RESPONSES env var (JSON map: id -> exit
// code + stdout lines).
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type Scripted struct {
	Exit  int      `json:"exit"`
	Lines []string `json:"lines"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, `{"code":"bad-argv"}`)
		os.Exit(3)
	}
	raw := os.Getenv("FAKESKILL_RESPONSES")
	if raw == "" {
		fmt.Fprintln(os.Stderr, `{"code":"no-script"}`)
		os.Exit(3)
	}
	var script map[string]Scripted
	if err := json.Unmarshal([]byte(raw), &script); err != nil {
		fmt.Fprintln(os.Stderr, `{"code":"bad-script","details":"`+err.Error()+`"}`)
		os.Exit(3)
	}
	resp, ok := script[os.Args[1]]
	if !ok {
		fmt.Fprintln(os.Stderr, `{"code":"unknown-id"}`)
		os.Exit(3)
	}
	for _, line := range resp.Lines {
		if _, err := io.WriteString(os.Stdout, line+"\n"); err != nil {
			os.Exit(3)
		}
	}
	os.Exit(resp.Exit)
}
