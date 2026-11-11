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
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	lastro "github.com/iurykrieger/lastro/sdks/go"
	"github.com/iurykrieger/lastro/sdks/go/claude"
)

const usage = `lastro-go <command> [options]

  init [--model <model.json>] [--subject <module>[#export]] [--approvals N] [--submitter-cannot-approve]
                                                start a .lastro/ workspace (or add a model to it)
  propose <prose-file> [--section <heading>] [--as <submitter>] [--proposal <P>]
                                                create a proposal and extract its intents against the use cases;
                                                --proposal re-extracts an existing one and keeps its review
  status [<P>]                                  proposals, or one proposal's intents and pending items
  review <P> --as <name> <action> ...           approve <I..> | approve-all | amend <I> <text> | reject <I> <note>
                                                answer <D> <option> | comment <I|PF|UC|prose> <text> | reply <T> <text>
                                                resolve <T> | rewrite <UC> <after> | recombine <UC> [instructions]
  derive <P>                                    derive the pending proofs of the approved intents
  prove [<P>] --cmd "<command>" | --table rows.json [--prover path/to/lastro-prove] [--stream]
                                                run its proofs and every use case's proofs (regression);
                                                writes the receipt next to the proposal
  feedback <P>                                  the last run's failures, as a brief for whoever writes the code
  apply <P>                                     attach the proofs to their use cases and sync .lastro/usecases/
  regress --cmd "<command>" | --table rows.json run every use case's proofs alone

  <P> is a proposal id (P-001, or 1) or slug; without it, the latest open proposal.
  The code under proof speaks JSON lines (one input object per line in, one output object per line out).
  --dir <path>          workspace (default .lastro)
  --llm anthropic|handoff`

func main() {
	argv := os.Args[1:]
	code, body := runWith(context.Background(), argv, func(event obj) { os.Stdout.WriteString(lastro.Stringify(event) + "\n") })
	if parse(argv).flags["stream"] == "true" {
		os.Stdout.WriteString(lastro.Stringify(append(obj{{"event", "result"}}, body...)) + "\n")
	} else {
		os.Stdout.WriteString(lastro.StringifyIndent(body) + "\n")
	}
	os.Exit(code)
}

// streamProgressEvery matches the TS CLI: one progress event every 100 runs of the code under proof.
const streamProgressEvery = 100

// emitter receives the `prove --stream` events (start, progress, clause, scenario, crosscheck), in the
// TS CLI's order and shape. The CLI prints the closing {"event":"result",...} itself.
type emitter func(event obj)

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

// booleanFlags never take a value.
var booleanFlags = map[string]bool{"stream": true, "submitter-cannot-approve": true}

func parse(argv []string) args {
	a := args{flags: map[string]string{}}
	for i := 0; i < len(argv); i++ {
		s := argv[i]
		if strings.HasPrefix(s, "--") {
			k := s[2:]
			if i+1 >= len(argv) || strings.HasPrefix(argv[i+1], "--") || booleanFlags[k] {
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

// ---------------------------------------------------------------------------
// Workspace (same files as the TS CLI's lastro/workspace)
// ---------------------------------------------------------------------------

// legacyFiles are the intent-artifact/v0 workspace files, which proposal/v1 refuses.
var legacyFiles = []string{"intents.json", "spec.json", "proof.json"}

type config struct {
	models     map[string]lastro.Model
	modelOrder []string
	// subjects keeps the TS CLI's {module, export} entries as written.
	subjects     json.RawMessage
	subjectNames []string
	review       lastro.ProposalPolicy
}

type workspace struct{ dir string }

func (w workspace) file(parts ...string) string {
	return filepath.Join(append([]string{w.dir}, parts...)...)
}

func (w workspace) exists() bool {
	_, err := os.Stat(w.file("config.json"))
	return err == nil
}

func (w workspace) legacy() []string {
	out := []string{}
	for _, f := range legacyFiles {
		if _, err := os.Stat(w.file(f)); err == nil {
			out = append(out, f)
		}
	}
	return out
}

func (w workspace) config() (config, error) {
	if !w.exists() {
		return config{}, fail(2, kv{"error", fmt.Sprintf("no workspace at %s: run `lastro-go init --model model.json`", w.dir)})
	}
	var raw struct {
		Models   json.RawMessage `json:"models"`
		Model    *lastro.Model   `json:"model"`
		Subjects json.RawMessage `json:"subjects"`
		Review   *struct {
			Approvals           *int  `json:"approvals"`
			SubmitterCanApprove *bool `json:"submitterCanApprove"`
		} `json:"review"`
		RequiredApprovals *int `json:"requiredApprovals"`
	}
	if err := readJSON(w.file("config.json"), &raw); err != nil {
		return config{}, err
	}
	c := config{models: map[string]lastro.Model{}, subjects: raw.Subjects, review: lastro.ReadPolicy(raw.Review, raw.RequiredApprovals)}
	if len(raw.Models) > 0 {
		if err := json.Unmarshal(raw.Models, &c.models); err != nil {
			return config{}, err
		}
		c.modelOrder = lastro.ObjectKeys(raw.Models)
	}
	if raw.Model != nil && c.models[raw.Model.Name].Name == "" {
		c.models[raw.Model.Name] = *raw.Model // the v0 `model` key is still read
		c.modelOrder = append(c.modelOrder, raw.Model.Name)
	}
	if len(c.subjects) == 0 || string(c.subjects) == "null" {
		c.subjects = json.RawMessage("{}")
	}
	c.subjectNames = lastro.ObjectKeys(c.subjects)
	return c, nil
}

func (w workspace) writeConfig(c config) error {
	models := obj{}
	for _, n := range c.modelOrder {
		models = append(models, kv{n, c.models[n]})
	}
	return writeJSON(w.file("config.json"), obj{{"models", models}, {"subjects", c.subjects}, {"review", c.review}})
}

func listJSON(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := []string{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func (w workspace) useCases() ([]lastro.UseCase, error) {
	out := []lastro.UseCase{}
	for _, f := range listJSON(w.file("usecases")) {
		var u lastro.UseCase
		if err := readJSON(w.file("usecases", f), &u); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out = append(out, u)
	}
	return out, nil
}

func (w workspace) writeUseCase(u lastro.UseCase) error {
	return writeJSON(w.file("usecases", lastro.FileNameOf(u.ID, u.Slug)), u)
}

func (w workspace) removeUseCase(u lastro.UseCase) {
	_ = os.Remove(w.file("usecases", lastro.FileNameOf(u.ID, u.Slug)))
}

func (w workspace) proposals() ([]lastro.Proposal, error) {
	out := []lastro.Proposal{}
	for _, f := range listJSON(w.file("proposals")) {
		var p lastro.Proposal
		if err := readJSON(w.file("proposals", f), &p); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out = append(out, p)
	}
	return out, nil
}

var proposalFile = regexp.MustCompile(`^(P-\d+)-`)

func (w workspace) nextProposalID() string {
	ids := []string{}
	entries, _ := os.ReadDir(w.file("proposals"))
	for _, e := range entries {
		if m := proposalFile.FindStringSubmatch(e.Name()); m != nil {
			ids = append(ids, m[1])
		}
	}
	return lastro.NextID("P", ids)
}

// proposal finds a proposal by id ("P-001", "1") or slug; without one, the latest that is not applied or closed.
func (w workspace) proposal(ref string) (lastro.Proposal, error) {
	all, err := w.proposals()
	if err != nil {
		return lastro.Proposal{}, err
	}
	if ref == "" {
		for i := len(all) - 1; i >= 0; i-- {
			if all[i].Status != "applied" && all[i].Status != "closed" {
				return all[i], nil
			}
		}
		return lastro.Proposal{}, fail(2, kv{"error", "no open proposal: run `lastro-go propose <prose-file>`"})
	}
	id := ref
	if n, err := strconv.Atoi(ref); err == nil && n >= 0 {
		id = fmt.Sprintf("P-%03d", n)
	}
	for _, p := range all {
		if p.ID == id || p.Slug == ref {
			return p, nil
		}
	}
	return lastro.Proposal{}, fail(2, kv{"error", "no proposal " + ref})
}

func (w workspace) proposalPath(p lastro.Proposal, ext string) string {
	return w.file("proposals", p.ID+"-"+p.Slug+ext)
}

func (w workspace) writeProposal(p lastro.Proposal) error {
	return writeJSON(w.proposalPath(p, ".json"), p)
}

// appendRun adds one line to <id>-<slug>.runs.jsonl: every prove run, with its time and commit.
func (w workspace) appendRun(p lastro.Proposal, run lastro.ProposalRun) error {
	path := w.proposalPath(p, ".runs.jsonl")
	n := 1
	if b, err := os.ReadFile(path); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.TrimSpace(l) != "" {
				n++
			}
		}
	}
	line := obj{{"run", n}, {"at", time.Now().UTC().Format("2006-01-02T15:04:05.000Z")}}
	root := filepath.Dir(w.dir)
	if out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output(); err == nil {
		line = append(line, kv{"commit", strings.TrimSpace(string(out))})
		if st, err := exec.Command("git", "-C", root, "status", "--porcelain").Output(); err == nil && strings.TrimSpace(string(st)) != "" {
			line = append(line, kv{"dirty", true})
		}
	}
	line = append(line, kv{"ok", run.OK}, kv{"outcomes", nonNil(run.Outcomes)}, kv{"regressions", nonNil(run.Regressions)},
		kv{"replacedStillPass", nonNil(run.ReplacedStillPass)}, kv{"unproved", nonNil(run.Unproved)})
	if run.EnginesAgree != nil {
		line = append(line, kv{"enginesAgree", *run.EnginesAgree})
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(lastro.Stringify(line) + "\n")
	return err
}

// ---------------------------------------------------------------------------
// LLM
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Output
// ---------------------------------------------------------------------------

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// section is the body of the Markdown section whose heading is title, up to the next heading of the same or a higher level.
func section(markdown, title string) (string, error) {
	lines := strings.Split(markdown, "\n")
	heading := regexp.MustCompile(`^(#{1,6})\s`)
	norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	start := -1
	for i, l := range lines {
		if heading.MatchString(l) && norm(strings.TrimLeft(l, "#")) == norm(title) {
			start = i
			break
		}
	}
	if start < 0 {
		return "", fail(2, kv{"error", fmt.Sprintf("no section %q", title)})
	}
	level := len(heading.FindStringSubmatch(lines[start])[1])
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if m := heading.FindStringSubmatch(lines[i]); m != nil && len(m[1]) <= level {
			end = i
			break
		}
	}
	return strings.Join(lines[start+1:end], "\n"), nil
}

// summary is one proposal as the CLI prints it, key for key the TS CLI's summary().
func summary(p lastro.Proposal, useCases []lastro.UseCase) []kv {
	described := map[string]string{}
	for _, u := range useCases {
		for _, x := range u.Proofs {
			described[x.ID] = x.Description
		}
	}
	pending := lastro.PendingProposal(p)
	changes := []obj{}
	for _, c := range p.UseCases {
		o := obj{{"useCase", c.UseCase}, {"slug", c.Slug}, {"change", c.Change}, {"name", c.Name}}
		if c.Stale != nil && *c.Stale {
			o = append(o, kv{"stale", true})
		}
		changes = append(changes, append(o, kv{"intents", nonNil(c.Intents)}))
	}
	intents := []obj{}
	for _, i := range p.Intents {
		replaces := []obj{}
		for _, r := range i.Replaces {
			var d any
			if s, ok := described[r.Proof]; ok {
				d = s
			}
			replaces = append(replaces, obj{{"proof", r.Proof}, {"reason", r.Reason}, {"description", d}})
		}
		intents = append(intents, obj{{"id", i.ID}, {"change", i.Change}, {"useCase", i.UseCase}, {"status", i.Status}, {"approvals", nonNil(i.Approvals)},
			{"name", i.Name}, {"description", i.Description}, {"provability", i.Provability}, {"replaces", replaces}})
	}
	decisions := []obj{}
	for _, d := range p.Decisions {
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
	threads := []obj{}
	for _, t := range p.Threads {
		if !t.Resolved {
			threads = append(threads, obj{{"id", t.ID}, {"on", t.On}, {"by", t.By}, {"comments", nonNil(t.Comments)}})
		}
	}
	proofs := []obj{}
	for _, x := range p.Proofs {
		o := obj{{"id", x.ID}, {"intent", x.Intent}, {"useCase", x.UseCase}, {"kind", x.Check.Kind}, {"description", x.Description}, {"status", x.Status}}
		if len(x.Replaces) > 0 {
			o = append(o, kv{"replaces", x.Replaces})
		}
		proofs = append(proofs, o)
	}
	return []kv{
		{"proposal", obj{{"id", p.ID}, {"slug", p.Slug}, {"name", p.Name}, {"status", p.Status}, {"submittedBy", p.SubmittedBy}, {"policy", p.Policy}, {"approved", p.Hash != ""}}},
		{"useCases", changes}, {"intents", intents}, {"decisions", decisions}, {"threads", threads}, {"supersedes", nonNil(p.Supersedes)},
		{"proofs", proofs}, {"pending", nonNil(pending)}, {"ready", len(pending) == 0},
	}
}

func runBody(run lastro.ProposalRun) []kv {
	failures := []lastro.ProofOutcome{}
	for _, x := range run.Outcomes {
		if x.Role != "superseded" && !x.Passed {
			failures = append(failures, x)
		}
	}
	var agree any
	if run.EnginesAgree != nil {
		agree = *run.EnginesAgree
	}
	return []kv{{"ok", run.OK}, {"enginesAgree", agree}, {"failures", failures}, {"regressions", nonNil(run.Regressions)},
		{"replacedStillPass", nonNil(run.ReplacedStillPass)}, {"unproved", nonNil(run.Unproved)}}
}

func run(ctx context.Context, argv []string) (int, obj) { return runWith(ctx, argv, nil) }

func runWith(ctx context.Context, argv []string, emit emitter) (int, obj) {
	code, body, err := dispatch(ctx, parse(argv), emit)
	if err != nil {
		var e *exit
		var pe *lastro.ProposalError
		switch {
		case errors.As(err, &e):
			return e.code, e.body
		case errors.As(err, &pe):
			return 1, obj{{"status", "refused"}, {"problems", pe.Problems}}
		}
		return 2, obj{{"error", err.Error()}}
	}
	return code, body
}

// subjects is the code under proof, by subject name: one program (--cmd, JSON lines) or one recorded table (--table).
func subjects(ctx context.Context, a args, cfg config, emit emitter) (map[string]lastro.Subject, func(), error) {
	names := cfg.subjectNames
	if len(names) > 1 {
		return nil, nil, fail(2, kv{"error", fmt.Sprintf("the workspace declares %d subjects (%s); lastro-go proves exactly one per run", len(names), strings.Join(names, ", "))})
	}
	name := "default"
	if len(names) == 1 {
		name = names[0]
	} else if len(cfg.modelOrder) > 0 {
		name = cfg.modelOrder[0]
	}
	var subject lastro.Subject
	stop := func() {}
	switch {
	case a.flags["table"] != "":
		var rows []lastro.Row
		if err := readJSON(a.flags["table"], &rows); err != nil {
			return nil, nil, err
		}
		subject = lastro.TableSubject(rows)
	case a.flags["cmd"] != "":
		s, halt, err := lastro.ProcessSubject(ctx, "sh", "-c", a.flags["cmd"])
		if err != nil {
			return nil, nil, err
		}
		subject, stop = s, func() { _ = halt() }
	default:
		return nil, nil, fail(2, kv{"error", `the code under proof is missing: --cmd "<command>" (JSON lines) or --table rows.json`})
	}
	if emit != nil {
		inner, runs := subject, 0
		subject = func(in lastro.Env) (lastro.Env, error) {
			runs++
			if runs%streamProgressEvery == 0 {
				emit(obj{{"event", "progress"}, {"runs", runs}})
			}
			return inner(in)
		}
	}
	return map[string]lastro.Subject{name: subject}, stop, nil
}

func rustProver(a args) lastro.RustProver {
	if p := a.flags["prover"]; p != "" {
		return &lastro.ExecProver{Path: p}
	}
	if p := lastro.FindRustProver(); p != nil {
		return p
	}
	return nil
}

func writeReceipt(ws workspace, p lastro.Proposal, useCases []lastro.UseCase) (string, error) {
	path := ws.proposalPath(p, ".receipt.md")
	return path, os.WriteFile(path, []byte(lastro.RenderProposalReceipt(p, useCases)+"\n"), 0o644)
}

func setup(cfg config) lastro.ProofSetup {
	subjects := cfg.subjectNames
	if len(subjects) == 0 {
		subjects = cfg.modelOrder
	}
	return lastro.ProofSetup{Models: cfg.models, ModelOrder: cfg.modelOrder, Subjects: subjects}
}

func dispatch(ctx context.Context, a args, emit emitter) (int, obj, error) {
	dir := a.flags["dir"]
	if dir == "" {
		dir = ".lastro"
	}
	dir, _ = filepath.Abs(dir)
	ws := workspace{dir}
	cmd, rest := "", []string{}
	if len(a.pos) > 0 {
		cmd, rest = a.pos[0], a.pos[1:]
	}
	arg := func(xs []string, i int) string {
		if i < len(xs) {
			return xs[i]
		}
		return ""
	}
	if cmd != "" && cmd != "init" {
		if legacy := ws.legacy(); len(legacy) > 0 {
			return 0, nil, fail(2, kv{"error", fmt.Sprintf("%s holds %s from the old workspace format, which this version does not read. Delete them and run `lastro-go propose <prose-file>` again.", ws.dir, strings.Join(legacy, ", "))}, kv{"legacy", legacy})
		}
	}
	if cmd == "plan" || cmd == "spec" {
		return 0, nil, fail(2, kv{"error", fmt.Sprintf("`lastro-go %s` was replaced: use `lastro-go propose`, `lastro-go review` and `lastro-go derive`", cmd)}, kv{"usage", usage})
	}

	switch cmd {
	case "init":
		// Without --model the workspace starts empty: proposals can be written and reviewed, and a bootstrap brings models later.
		hasModel := a.flags["model"] != "" && a.flags["model"] != "true"
		if s := a.flags["subject"]; s != "" && s != "true" && !hasModel {
			return 0, nil, fail(2, kv{"error", "--subject needs --model <model.json>"})
		}
		var model lastro.Model
		if hasModel {
			if err := readJSON(a.flags["model"], &model); err != nil {
				return 0, nil, err
			}
		}
		cfg := config{models: map[string]lastro.Model{}, subjects: json.RawMessage("{}"), review: lastro.DefaultPolicy}
		if ws.exists() {
			var err error
			if cfg, err = ws.config(); err != nil {
				return 0, nil, err
			}
		}
		if hasModel {
			if _, ok := cfg.models[model.Name]; !ok {
				cfg.modelOrder = append(cfg.modelOrder, model.Name)
			}
			cfg.models[model.Name] = model
		}
		if s := a.flags["subject"]; hasModel && s != "" && s != "true" {
			module, exp, _ := strings.Cut(s, "#")
			ref := obj{{"module", module}}
			if exp != "" {
				ref = append(ref, kv{"export", exp})
			}
			name := a.flags["subject-name"]
			if name == "" {
				name = model.Name
			}
			subjects := obj{}
			var existing map[string]json.RawMessage
			_ = json.Unmarshal(cfg.subjects, &existing)
			replaced := false
			for _, k := range cfg.subjectNames {
				if k == name {
					subjects, replaced = append(subjects, kv{k, ref}), true
				} else {
					subjects = append(subjects, kv{k, existing[k]})
				}
			}
			if !replaced {
				subjects = append(subjects, kv{name, ref})
			}
			b, _ := json.Marshal(subjects)
			cfg.subjects, cfg.subjectNames = b, lastro.ObjectKeys(b)
		}
		if s, ok := a.flags["approvals"]; ok {
			n, err := strconv.Atoi(s)
			if err != nil {
				return 0, nil, fail(2, kv{"error", "--approvals needs a number"})
			}
			cfg.review.Approvals = max(1, n)
		}
		if a.flags["submitter-cannot-approve"] == "true" {
			cfg.review.SubmitterCanApprove = false
		}
		if err := ws.writeConfig(cfg); err != nil {
			return 0, nil, err
		}
		return 0, obj{{"status", "initialized"}, {"dir", ws.dir}, {"models", nonNil(cfg.modelOrder)}, {"subjects", nonNil(cfg.subjectNames)}, {"review", cfg.review}}, nil

	case "propose":
		cfg, err := ws.config()
		if err != nil {
			return 0, nil, err
		}
		if len(rest) == 0 {
			return 0, nil, fail(2, kv{"error", "propose needs a prose file"})
		}
		proseFile, _ := filepath.Abs(rest[0])
		b, err := os.ReadFile(proseFile)
		if err != nil {
			return 0, nil, err
		}
		text := string(b)
		if s := a.flags["section"]; s != "" {
			if text, err = section(text, s); err != nil {
				return 0, nil, err
			}
		}
		prose := strings.TrimSpace(text)
		useCases, err := ws.useCases()
		if err != nil {
			return 0, nil, err
		}
		// A prose file already in .lastro/proposals/ named <id>-<slug>.md names its proposal: re-extract it if it exists.
		inProposals := filepath.Dir(proseFile) == ws.file("proposals")
		var named []string
		if inProposals {
			named = regexp.MustCompile(`^(P-\d+)-([a-z0-9]+(?:-[a-z0-9]+)*)\.md$`).FindStringSubmatch(filepath.Base(proseFile))
		}
		var previous *lastro.Proposal
		if ref := a.flags["proposal"]; ref != "" {
			p, err := ws.proposal(ref)
			if err != nil {
				return 0, nil, err
			}
			previous = &p
		} else if named != nil {
			all, err := ws.proposals()
			if err != nil {
				return 0, nil, err
			}
			for i := range all {
				if all[i].ID == named[1] {
					previous = &all[i]
				}
			}
		}
		id := ws.nextProposalID()
		if previous != nil {
			id = previous.ID
		} else if named != nil {
			id = named[1]
		}
		submittedBy := a.flags["as"]
		if submittedBy == "" && previous != nil {
			submittedBy = previous.SubmittedBy
		}
		if submittedBy == "" {
			submittedBy = os.Getenv("LASTRO_USER")
		}
		if submittedBy == "" {
			submittedBy = "submitter"
		}
		ask, err := pickAsk(a, ws.dir)
		if err != nil {
			return 0, nil, err
		}
		all, err := ws.proposals()
		if err != nil {
			return 0, nil, err
		}
		p, err := lastro.ExtractProposal(ctx, ask, lastro.ProposeInput{ID: id, Prose: prose, SubmittedBy: submittedBy, UseCases: useCases, Policy: cfg.review, Reserved: lastro.ReservedUseCaseIDs(all, id)})
		if err != nil {
			return 0, nil, err
		}
		if previous != nil {
			p = lastro.CarryProposalReview(*previous, p)
		}
		if previous != nil {
			p.Slug = previous.Slug // slugs are stable
		} else if named != nil {
			p.Slug = named[2]
		}
		if err := ws.writeProposal(p); err != nil {
			return 0, nil, err
		}
		if md := ws.proposalPath(p, ".md"); md != proseFile {
			if err := os.WriteFile(md, []byte(prose+"\n"), 0o644); err != nil {
				return 0, nil, err
			}
			if inProposals && named == nil {
				_ = os.Remove(proseFile) // a draft in .lastro/proposals/ becomes <id>-<slug>.md
			}
		}
		s := summary(p, useCases)
		code := 1
		if len(lastro.PendingProposal(p)) == 0 {
			code = 0
		}
		return code, append(obj{{"status", "proposed"}, {"file", ws.proposalPath(p, ".json")}}, s...), nil

	case "status":
		useCases, err := ws.useCases()
		if err != nil {
			return 0, nil, err
		}
		ref := arg(rest, 0)
		if ref == "" {
			ref = a.flags["proposal"]
		}
		if ref == "" {
			all, err := ws.proposals()
			if err != nil {
				return 0, nil, err
			}
			list, open := []obj{}, 0
			for _, p := range all {
				list = append(list, obj{{"id", p.ID}, {"slug", p.Slug}, {"name", p.Name}, {"status", p.Status}, {"pending", len(lastro.PendingProposal(p))}})
				if p.Status != "applied" && p.Status != "closed" {
					open++
				}
			}
			proofs, unproved := 0, []string{}
			for _, u := range useCases {
				proofs += len(u.Proofs)
				if len(u.Proofs) == 0 {
					unproved = append(unproved, u.ID)
				}
			}
			code := 0
			if open > 0 {
				code = 1
			}
			return code, obj{{"status", "status"}, {"useCases", len(useCases)}, {"proofs", proofs}, {"unproved", unproved}, {"proposals", list}}, nil
		}
		p, err := ws.proposal(ref)
		if err != nil {
			return 0, nil, err
		}
		code := 1
		if len(lastro.PendingProposal(p)) == 0 {
			code = 0
		}
		return code, append(obj{{"status", "status"}}, summary(p, useCases)...), nil

	case "review":
		who := a.flags["as"]
		if who == "" || who == "true" {
			return 0, nil, fail(2, kv{"error", "review needs --as <name>"})
		}
		if len(rest) < 2 {
			return 0, nil, fail(2, kv{"error", "review needs a proposal and an action"}, kv{"usage", usage})
		}
		ref, action, xs := rest[0], rest[1], rest[2:]
		useCases, err := ws.useCases()
		if err != nil {
			return 0, nil, err
		}
		before, err := ws.proposal(ref)
		if err != nil {
			return 0, nil, err
		}
		r := lastro.ReviewProposal(before, who)
		tail := strings.Join(xs[min(1, len(xs)):], " ")
		switch action {
		case "approve":
			r.Approve(xs...)
		case "approve-all":
			r.ApproveAll()
		case "amend":
			r.Amend(arg(xs, 0), tail)
		case "reject":
			if tail == "" {
				tail = "rejected"
			}
			r.Reject(arg(xs, 0), tail)
		case "answer":
			n, err := strconv.Atoi(arg(xs, 1))
			if err != nil {
				return 0, nil, fail(2, kv{"error", "answer needs <decision> <option-index>"})
			}
			r.Answer(arg(xs, 0), n)
		case "comment":
			r.Comment(arg(xs, 0), tail)
		case "reply":
			r.Reply(arg(xs, 0), tail)
		case "resolve":
			r.Resolve(arg(xs, 0))
		case "rewrite":
			r.RewriteAfter(arg(xs, 0), tail, "")
		case "recombine":
			current, err := r.Done()
			if err != nil {
				return 0, nil, err
			}
			ask, err := pickAsk(a, ws.dir)
			if err != nil {
				return 0, nil, err
			}
			out, err := lastro.RecombineAfter(ctx, ask, current, arg(xs, 0), tail)
			if err != nil {
				return 0, nil, err
			}
			r.RewriteAfter(arg(xs, 0), out.After, out.Name)
		default:
			return 0, nil, fail(2, kv{"error", "unknown review action " + action}, kv{"usage", usage})
		}
		p, err := r.Done()
		if err != nil {
			return 0, nil, err
		}
		// Fully reviewed: approve it, bound to the current use cases.
		if len(lastro.PendingProposal(p)) == 0 && p.Hash == "" {
			if lastro.BaselineOf(useCases) != p.Baseline {
				if err := ws.writeProposal(p); err != nil {
					return 0, nil, err
				}
				return 0, nil, fail(1, kv{"status", "stale"}, kv{"error", fmt.Sprintf("the use cases changed since %s was extracted: run `lastro-go propose <prose-file> --proposal %s` again", p.ID, p.ID)})
			}
			if p, err = lastro.ApproveProposal(p, useCases); err != nil {
				return 0, nil, err
			}
		}
		if err := ws.writeProposal(p); err != nil {
			return 0, nil, err
		}
		return 0, append(obj{{"status", "reviewed"}, {"reviewer", who}}, summary(p, useCases)...), nil

	case "derive":
		cfg, err := ws.config()
		if err != nil {
			return 0, nil, err
		}
		useCases, err := ws.useCases()
		if err != nil {
			return 0, nil, err
		}
		p, err := ws.proposal(arg(rest, 0))
		if err != nil {
			return 0, nil, err
		}
		if p.Hash == "" {
			return 0, nil, fail(1, kv{"status", "review-pending"}, kv{"pending", nonNil(lastro.PendingProposal(p))})
		}
		ask, err := pickAsk(a, ws.dir)
		if err != nil {
			return 0, nil, err
		}
		next, err := lastro.DeriveProofs(ctx, ask, p, useCases, setup(cfg), lastro.VerifyOptions{})
		if err != nil {
			return 0, nil, err
		}
		if err := ws.writeProposal(next); err != nil {
			return 0, nil, err
		}
		if next.Hash == "" {
			return 1, append(obj{{"status", "back-to-review"}, {"note", fmt.Sprintf("Derivation raised decisions. Answer them with `lastro-go review %s --as <name> answer <D> <option>`, review the intents they reopened, and run `lastro-go derive %s` again.", next.ID, next.ID)}}, summary(next, useCases)...), nil
		}
		return 0, append(obj{{"status", "derived"}}, summary(next, useCases)...), nil

	case "prove":
		cfg, err := ws.config()
		if err != nil {
			return 0, nil, err
		}
		useCases, err := ws.useCases()
		if err != nil {
			return 0, nil, err
		}
		p, err := ws.proposal(arg(rest, 0))
		if err != nil {
			return 0, nil, err
		}
		stream := emit
		if a.flags["stream"] != "true" {
			stream = nil
		}
		subs, stop, err := subjects(ctx, a, cfg, stream)
		if err != nil {
			return 0, nil, err
		}
		defer stop()
		if stream != nil {
			ids := []string{}
			for _, x := range p.Proofs {
				ids = append(ids, x.ID)
			}
			stream(obj{{"event", "start"}, {"proposal", p.ID}, {"proofs", ids}})
		}
		proposal, run, err := lastro.ProveProposal(ctx, p, useCases, lastro.RunOptions{Subjects: subs, Models: cfg.models, Rust: rustProver(a)})
		if err != nil {
			return 0, nil, err
		}
		if stream != nil {
			for _, o := range run.Outcomes {
				ev := obj{{"event", "proof"}, {"proof", o.Proof}, {"role", o.Role}, {"verdict", o.Verdict}, {"passed", o.Passed}}
				if o.Counterexample != nil {
					ev = append(ev, kv{"counterexample", o.Counterexample})
				}
				stream(ev)
			}
			if run.EnginesAgree != nil {
				stream(obj{{"event", "crosscheck"}, {"enginesAgree", *run.EnginesAgree}})
			}
		}
		if err := ws.writeProposal(proposal); err != nil {
			return 0, nil, err
		}
		if err := ws.appendRun(proposal, run); err != nil {
			return 0, nil, err
		}
		receipt, err := writeReceipt(ws, proposal, useCases)
		if err != nil {
			return 0, nil, err
		}
		status, code := "proved", 0
		if !run.OK {
			status, code = "not-proved", 1
		}
		body := append(obj{{"status", status}, {"proposal", proposal.ID}}, runBody(run)...)
		return code, append(body, kv{"receipt", receipt}, kv{"feedback", lastro.ProposalFeedback(proposal, useCases)}), nil

	case "feedback":
		useCases, err := ws.useCases()
		if err != nil {
			return 0, nil, err
		}
		p, err := ws.proposal(arg(rest, 0))
		if err != nil {
			return 0, nil, err
		}
		code := 1
		if p.LastRun != nil && p.LastRun.OK {
			code = 0
		}
		return code, obj{{"status", "feedback"}, {"proposal", p.ID}, {"feedback", lastro.ProposalFeedback(p, useCases)}}, nil

	case "apply":
		useCases, err := ws.useCases()
		if err != nil {
			return 0, nil, err
		}
		p, err := ws.proposal(arg(rest, 0))
		if err != nil {
			return 0, nil, err
		}
		applied, err := lastro.ApplyProposal(p, useCases)
		if err != nil {
			return 0, nil, err
		}
		for _, id := range applied.Removed {
			for _, u := range useCases {
				if u.ID == id {
					ws.removeUseCase(u)
				}
			}
		}
		for _, id := range applied.Written {
			for _, u := range applied.UseCases {
				if u.ID == id {
					if err := ws.writeUseCase(u); err != nil {
						return 0, nil, err
					}
				}
			}
		}
		if err := ws.writeProposal(applied.Proposal); err != nil {
			return 0, nil, err
		}
		receipt, err := writeReceipt(ws, applied.Proposal, useCases)
		if err != nil {
			return 0, nil, err
		}
		unproved := []string{}
		if p.LastRun != nil {
			unproved = nonNil(p.LastRun.Unproved)
		}
		return 0, obj{{"status", "applied"}, {"proposal", p.ID}, {"written", nonNil(applied.Written)}, {"removed", nonNil(applied.Removed)},
			{"baseline", applied.Baseline}, {"unproved", unproved}, {"receipt", receipt}}, nil

	case "regress":
		cfg, err := ws.config()
		if err != nil {
			return 0, nil, err
		}
		useCases, err := ws.useCases()
		if err != nil {
			return 0, nil, err
		}
		subs, stop, err := subjects(ctx, a, cfg, nil)
		if err != nil {
			return 0, nil, err
		}
		defer stop()
		run, err := lastro.Regress(ctx, useCases, lastro.RunOptions{Subjects: subs, Models: cfg.models, Rust: rustProver(a)})
		if err != nil {
			return 0, nil, err
		}
		status, code := "no-regressions", 0
		if !run.OK {
			status, code = "regressions", 1
		}
		return code, append(obj{{"status", status}}, runBody(run)...), nil
	}
	errText := "no command"
	if cmd != "" {
		errText = "unknown command " + cmd
	}
	return 0, nil, fail(2, kv{"error", errText}, kv{"usage", usage})
}
