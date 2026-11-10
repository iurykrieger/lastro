// Command lastro-go is the Go SDK as a command line: the same commands, JSON
// output, exit codes and .lastro/ workspace as the TypeScript CLI (`lastro`),
// so a workspace started with one can be continued with the other.
//
// Exit codes: 0 done/ok · 1 not ok (pending review, contradictions, failing
// proofs) · 2 usage or error · 3 an LLM answer is needed (see "handoff").
//
// The two AI steps (extracting intents, deriving proofs) run on:
//
//	--llm anthropic  the Anthropic API (ANTHROPIC_API_KEY)
//	--llm handoff    the calling agent's own model: lastro-go writes the request
//	                 (instructions, data, JSON Schema) to a file and exits 3; the
//	                 agent writes its JSON answer to the named path and runs the
//	                 same command again.
//
// The default is anthropic when ANTHROPIC_API_KEY is set, handoff otherwise.
// Request ids match the TS CLI's, so either CLI finds the other's answers.
//
// The code under proof is any program: `prove --cmd "<command>"` starts it and
// speaks JSON lines (one input object per line in, one output object per line
// out), and `prove --table rows.json` checks a recorded input/output table.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	lastro "github.com/iurykrieger/lastro/sdks/go"
	"github.com/iurykrieger/lastro/sdks/go/claude"
)

const usage = `lastro-go <command> [options]

  init --model <model.json> [--approvals N]     start a .lastro/ workspace
  plan <prose-file>                             extract intents (keeps earlier review)
  status                                        intents, approvals, pending items
  review --as <name> <action> ...               approve <I1 I2..> | approve-all | amend <I> <text>
                                                reject <I> <note> | answer <D> <option-index>
  spec                                          derive the formal spec; contradictions come back as decisions
  prove --cmd "<command>" | --table rows.json [--traces file.json] [--prover path/to/lastro-prove]
                                                check the code against the spec; writes proof.json, receipt.md
  feedback                                      the last proof's failures, as a brief for whoever writes the code

  --dir <path>          workspace (default .lastro)
  --llm anthropic|handoff`

func main() {
	code, body := run(context.Background(), os.Args[1:])
	os.Stdout.WriteString(lastro.StringifyIndent(body) + "\n")
	os.Exit(code)
}

// obj is a JSON object that keeps its key order, like the TS CLI's object literals.
type obj []kv

type kv struct {
	k string
	v any
}

func (o obj) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, p := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		v, err := json.Marshal(p.v)
		if err != nil {
			return nil, err
		}
		k, _ := json.Marshal(p.k)
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// exit carries a result out of the command switch.
type exit struct {
	code int
	body obj
}

func (e *exit) Error() string { return fmt.Sprint(e.body) }

func fail(code int, body ...kv) error { return &exit{code, obj(body)} }

type args struct {
	pos   []string
	flags map[string]string
}

func parse(argv []string) args {
	a := args{flags: map[string]string{}}
	for i := 0; i < len(argv); i++ {
		s := argv[i]
		if strings.HasPrefix(s, "--") {
			k := s[2:]
			if i+1 >= len(argv) || strings.HasPrefix(argv[i+1], "--") {
				a.flags[k] = "true"
			} else {
				a.flags[k] = argv[i+1]
				i++
			}
		} else {
			a.pos = append(a.pos, s)
		}
	}
	return a
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// writeJSON writes JSON.stringify(v, null, 2) + "\n", as the TS CLI does.
func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(lastro.StringifyIndent(v)+"\n"), 0o644)
}

type config struct {
	Model             lastro.Model `json:"model"`
	RequiredApprovals int          `json:"requiredApprovals"`
}

type workspace struct{ dir string }

func (w workspace) file(name string) string { return filepath.Join(w.dir, name) }

func (w workspace) config() (config, error) {
	var c config
	if _, err := os.Stat(w.file("config.json")); err != nil {
		return c, fail(2, kv{"error", fmt.Sprintf("no workspace at %s: run `lastro-go init --model model.json`", w.dir)})
	}
	return c, readJSON(w.file("config.json"), &c)
}

// handoffAsk writes a deterministic request file per question and reads the answer next to it.
func handoffAsk(dir string) lastro.AskJSON {
	return func(_ context.Context, req lastro.AskRequest) (json.RawMessage, error) {
		id := lastro.HandoffID(req)
		answer := filepath.Join(dir, "llm", id+".answer.json")
		if b, err := os.ReadFile(answer); err == nil {
			return json.RawMessage(b), nil
		}
		request := filepath.Join(dir, "llm", id+".request.json")
		err := writeJSON(request, obj{
			{"instructions", fmt.Sprintf(`Act as the system described in "system". Read "user". Write ONLY a JSON value matching "schema" to %s, then run the same lastro command again.`, answer)},
			{"step", req.Step}, {"system", req.System}, {"user", req.User}, {"schema", req.Schema},
		})
		if err != nil {
			return nil, err
		}
		return nil, fail(3, kv{"status", "needs-llm"}, kv{"step", req.Step}, kv{"request", request}, kv{"answer", answer})
	}
}

func pickAsk(a args, dir string) (lastro.AskJSON, error) {
	mode := a.flags["llm"]
	if mode == "" {
		mode = "handoff"
		if os.Getenv("ANTHROPIC_API_KEY") != "" {
			mode = "anthropic"
		}
	}
	switch mode {
	case "anthropic":
		return claude.Ask(claude.Options{}), nil
	case "handoff":
		return handoffAsk(dir), nil
	}
	return nil, fail(2, kv{"error", "unknown --llm " + mode})
}

func summary(a lastro.IntentArtifact, pending []string) []kv {
	intents := []obj{}
	for _, i := range a.Intents {
		o := obj{{"id", i.ID}, {"status", i.Status}, {"approvals", nonNil(i.Approvals)}, {"statement", i.Statement}}
		if i.DecisionID != "" {
			o = append(o, kv{"decisionId", i.DecisionID})
		}
		intents = append(intents, o)
	}
	decisions := []obj{}
	for _, d := range a.Decisions {
		labels := []string{}
		for _, o := range d.Options {
			labels = append(labels, o.Label)
		}
		var chosen any
		if d.Chosen != nil {
			chosen = *d.Chosen
		}
		decisions = append(decisions, obj{{"id", d.ID}, {"origin", d.Origin}, {"question", d.Question}, {"options", labels}, {"chosen", chosen}, {"recommended", d.Recommended}})
	}
	return []kv{{"intents", intents}, {"decisions", decisions}, {"pending", nonNil(pending)}, {"ready", len(pending) == 0}}
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func exitIf(pending []string) int {
	if len(pending) > 0 {
		return 1
	}
	return 0
}

func run(ctx context.Context, argv []string) (int, obj) {
	code, body, err := dispatch(ctx, parse(argv))
	if err != nil {
		var e *exit
		if errors.As(err, &e) {
			return e.code, e.body
		}
		return 2, obj{{"error", err.Error()}}
	}
	return code, body
}

func dispatch(ctx context.Context, a args) (int, obj, error) {
	dir := a.flags["dir"]
	if dir == "" {
		dir = ".lastro"
	}
	dir, _ = filepath.Abs(dir)
	ws := workspace{dir}
	intentsPath := ws.file("intents.json")
	cmd, rest := "", []string{}
	if len(a.pos) > 0 {
		cmd, rest = a.pos[0], a.pos[1:]
	}
	loadIntents := func() (lastro.IntentArtifact, error) {
		var art lastro.IntentArtifact
		return art, readJSON(intentsPath, &art)
	}

	switch cmd {
	case "init":
		if a.flags["model"] == "" {
			return 0, nil, fail(2, kv{"error", "init needs --model <model.json>"})
		}
		var model lastro.Model
		if err := readJSON(a.flags["model"], &model); err != nil {
			return 0, nil, err
		}
		approvals := 1
		if s, ok := a.flags["approvals"]; ok {
			n, err := strconv.Atoi(s)
			if err != nil {
				return 0, nil, fail(2, kv{"error", "--approvals needs a number"})
			}
			approvals = n
		}
		if err := writeJSON(ws.file("config.json"), obj{{"model", model}, {"requiredApprovals", approvals}}); err != nil {
			return 0, nil, err
		}
		return 0, obj{{"status", "initialized"}, {"dir", ws.dir}, {"model", model.Name}, {"requiredApprovals", approvals}}, nil

	case "plan":
		cfg, err := ws.config()
		if err != nil {
			return 0, nil, err
		}
		if len(rest) == 0 {
			return 0, nil, fail(2, kv{"error", "plan needs a prose file"})
		}
		b, err := os.ReadFile(rest[0])
		if err != nil {
			return 0, nil, err
		}
		ask, err := pickAsk(a, ws.dir)
		if err != nil {
			return 0, nil, err
		}
		l := lastro.New(lastro.Options{Model: cfg.Model, RequiredApprovals: cfg.RequiredApprovals, Ask: ask})
		var previous *lastro.IntentArtifact
		if prev, err := loadIntents(); err == nil {
			previous = &prev
		}
		art, err := l.Plan(ctx, strings.TrimSpace(string(b)), previous)
		if err != nil {
			return 0, nil, err
		}
		if err := writeJSON(intentsPath, art); err != nil {
			return 0, nil, err
		}
		pending := l.Pending(art)
		return exitIf(pending), append(obj{{"status", "planned"}}, summary(art, pending)...), nil

	case "status":
		cfg, err := ws.config()
		if err != nil {
			return 0, nil, err
		}
		art, err := loadIntents()
		if err != nil {
			return 0, nil, err
		}
		pending := lastro.New(lastro.Options{Model: cfg.Model, RequiredApprovals: cfg.RequiredApprovals}).Pending(art)
		return exitIf(pending), append(obj{{"status", "status"}}, summary(art, pending)...), nil

	case "review":
		cfg, err := ws.config()
		if err != nil {
			return 0, nil, err
		}
		who := a.flags["as"]
		if who == "" || who == "true" {
			return 0, nil, fail(2, kv{"error", "review needs --as <name>"})
		}
		art, err := loadIntents()
		if err != nil {
			return 0, nil, err
		}
		l := lastro.New(lastro.Options{Model: cfg.Model, RequiredApprovals: cfg.RequiredApprovals})
		r := l.Review(art, who)
		action, xs := "", []string{}
		if len(rest) > 0 {
			action, xs = rest[0], rest[1:]
		}
		arg := func(i int) string {
			if i < len(xs) {
				return xs[i]
			}
			return ""
		}
		switch action {
		case "approve":
			r.Approve(xs...)
		case "approve-all":
			r.ApproveAll()
		case "amend":
			r.Amend(arg(0), strings.Join(xs[min(1, len(xs)):], " "))
		case "reject":
			note := strings.Join(xs[min(1, len(xs)):], " ")
			if note == "" {
				note = "rejected"
			}
			r.Reject(arg(0), note)
		case "answer":
			n, err := strconv.Atoi(arg(1))
			if err != nil {
				return 0, nil, fail(2, kv{"error", "answer needs <decision> <option-index>"})
			}
			r.Answer(arg(0), n)
		default:
			return 0, nil, fail(2, kv{"error", "unknown review action " + action})
		}
		art, err = r.Done()
		if err != nil {
			return 0, nil, err
		}
		if err := writeJSON(intentsPath, art); err != nil {
			return 0, nil, err
		}
		return 0, append(obj{{"status", "reviewed"}, {"reviewer", who}}, summary(art, l.Pending(art))...), nil

	case "spec":
		cfg, err := ws.config()
		if err != nil {
			return 0, nil, err
		}
		ask, err := pickAsk(a, ws.dir)
		if err != nil {
			return 0, nil, err
		}
		l := lastro.New(lastro.Options{Model: cfg.Model, RequiredApprovals: cfg.RequiredApprovals, Ask: ask})
		art, err := loadIntents()
		if err != nil {
			return 0, nil, err
		}
		if pending := l.Pending(art); len(pending) > 0 {
			return 0, nil, fail(1, kv{"status", "review-pending"}, kv{"pending", pending})
		}
		spec, err := l.Spec(ctx, art)
		if err != nil {
			return 0, nil, err
		}
		if !spec.OK {
			back := l.ReturnDecisions(spec)
			if err := writeJSON(intentsPath, back); err != nil {
				return 0, nil, err
			}
			return 1, append(obj{{"status", "contradictions"}, {"note", "The spec contradicts itself. Answer the new decisions with `lastro-go review --as <name> answer <D> <option>`, approve the intents they create, and run `lastro-go spec` again."}}, summary(back, l.Pending(back))...), nil
		}
		if err := writeJSON(ws.file("spec.json"), spec); err != nil {
			return 0, nil, err
		}
		clauses := []obj{}
		for _, c := range spec.Plan.Clauses {
			clauses = append(clauses, obj{{"id", c.ID}, {"intentIds", c.IntentIDs}, {"description", c.Description}})
		}
		return 0, obj{{"status", "spec"}, {"artifactHash", spec.Artifact.Hash}, {"clauses", clauses}, {"beyondProof", nonNil(spec.Plan.BeyondProof)}}, nil

	case "prove":
		cfg, err := ws.config()
		if err != nil {
			return 0, nil, err
		}
		var spec lastro.Spec
		if err := readJSON(ws.file("spec.json"), &spec); err != nil {
			return 0, nil, fail(2, kv{"error", "no spec yet: run `lastro-go spec`"})
		}
		var subject lastro.Subject
		switch {
		case a.flags["table"] != "":
			var rows []lastro.Row
			if err := readJSON(a.flags["table"], &rows); err != nil {
				return 0, nil, err
			}
			subject = lastro.TableSubject(rows)
		case a.flags["cmd"] != "" || len(rest) > 0:
			command := a.flags["cmd"]
			if command == "" {
				command = strings.Join(rest, " ")
			}
			s, stop, err := lastro.ProcessSubject(ctx, "sh", "-c", command)
			if err != nil {
				return 0, nil, err
			}
			defer stop()
			subject = s
		default:
			return 0, nil, fail(2, kv{"error", `prove needs the code under proof: --cmd "<command>" (JSON lines) or --table rows.json`})
		}
		var traces []lastro.Trace
		if t := a.flags["traces"]; t != "" {
			if err := readJSON(t, &traces); err != nil {
				return 0, nil, err
			}
		}
		opts := lastro.Options{Model: cfg.Model, RequiredApprovals: cfg.RequiredApprovals}
		if p := a.flags["prover"]; p != "" {
			opts.Rust = &lastro.ExecProver{Path: p}
		} else if p := lastro.FindRustProver(); p != nil {
			opts.Rust = p
		}
		proof, err := lastro.New(opts).Prove(ctx, spec, subject, traces)
		if err != nil {
			return 0, nil, err
		}
		if err := writeJSON(ws.file("proof.json"), proof); err != nil {
			return 0, nil, err
		}
		if err := os.WriteFile(ws.file("receipt.md"), []byte(proof.Receipt+"\n"), 0o644); err != nil {
			return 0, nil, err
		}
		status, code := "proved", 0
		if !proof.OK {
			status, code = "not-proved", 1
		}
		var agree any
		if proof.EnginesAgree != nil {
			agree = *proof.EnginesAgree
		}
		return code, obj{{"status", status}, {"ok", proof.OK}, {"enginesAgree", agree}, {"failures", proof.Failures}, {"specGaps", proof.SpecGaps},
			{"receipt", ws.file("receipt.md")}, {"feedback", lastro.AgentFeedback(proof.Failures, proof.SpecGaps)}}, nil

	case "feedback":
		var proof lastro.Proof
		if err := readJSON(ws.file("proof.json"), &proof); err != nil {
			return 0, nil, err
		}
		code := 0
		if len(proof.Failures) > 0 {
			code = 1
		}
		return code, obj{{"status", "feedback"}, {"feedback", lastro.AgentFeedback(proof.Failures, proof.SpecGaps)}}, nil
	}
	errText := "no command"
	if cmd != "" {
		errText = "unknown command " + cmd
	}
	return 0, nil, fail(2, kv{"error", errText}, kv{"usage", usage})
}
