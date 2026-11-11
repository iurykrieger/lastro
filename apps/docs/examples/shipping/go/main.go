// Command shipping is the decision core, speaking JSON lines: one input per line in, one output per line out.
package main

import (
	"bufio"
	"encoding/json"
	"os"
)

type input struct {
	Subtotal int  `json:"subtotal"`
	Member   bool `json:"member"`
}

func fee(in input) int {
	if in.Member || in.Subtotal >= 100 {
		return 0
	}
	return 15
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		var in input
		if err := json.Unmarshal(sc.Bytes(), &in); err != nil {
			os.Exit(1)
		}
		out.Encode(map[string]int{"fee": fee(in)})
	}
}
