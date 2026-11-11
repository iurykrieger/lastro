package lastro

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// Subjects that are not Go functions: a recorded table, or any program that
// speaks JSON lines. With them, Lastro proves code in any language.

// TableSubject answers from recorded rows (for example the output of Tabulate, or a table
// another tool produced). An input with no row is an error.
func TableSubject(rows []Row) Subject {
	byKey := map[string]Env{}
	for _, r := range rows {
		byKey[Stringify(NormalizeEnv(r.Input))] = r.Output
	}
	return func(in Env) (Env, error) {
		out, ok := byKey[Stringify(NormalizeEnv(in))]
		if !ok {
			return nil, fmt.Errorf("no row for input %s", Stringify(in))
		}
		return out, nil
	}
}

// ProcessSubject starts a program once and talks to it in JSON lines: Lastro writes one input
// object per line on its stdin and reads one output object per line from its stdout. Call stop
// when done. The program's stderr is passed through.
//
//	while read -r line; do decide "$line"; done   # one JSON object in, one JSON object out
func ProcessSubject(ctx context.Context, name string, args ...string) (subject Subject, stop func() error, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	reader := bufio.NewReaderSize(stdout, 1<<16)
	var mu sync.Mutex
	subject = func(in Env) (Env, error) {
		mu.Lock()
		defer mu.Unlock()
		line, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		if _, err := stdin.Write(append(line, '\n')); err != nil {
			return nil, fmt.Errorf("subject process: %w", err)
		}
		reply, err := reader.ReadBytes('\n')
		if err != nil && (err != io.EOF || len(reply) == 0) {
			return nil, fmt.Errorf("subject process gave no answer for %s: %w", Stringify(in), err)
		}
		var out Env
		if err := json.Unmarshal(reply, &out); err != nil {
			return nil, fmt.Errorf("subject process answered %q for %s: not a JSON object", string(reply), Stringify(in))
		}
		return NormalizeEnv(out), nil
	}
	stop = func() error {
		_ = stdin.Close()
		return cmd.Wait()
	}
	return subject, stop, nil
}
