# Lastro Go SDK

The Go SDK of Lastro: prose → reviewable intents → derived proofs → a deterministic check of what the code
actually does. It is the same pipeline as the TypeScript SDK (`sdks/typescript`), with the same data model,
hashes and `.lastro/` workspace, so the two interoperate:

- an artifact approved by one SDK is intact for the other, and both compute the same plan hash;
- both engines produce the same verification report, field for field, on the reference login and every planted bug
  (`crosscheck_test.go` runs the TS SDK and compares);
- `lastro-go` and the TS `lastro` CLI share a workspace: one can plan, the other review, derive and prove
  (`cmd/lastro-go/main_test.go`);
- the Rust prover (`crates/lastro-core`) checks the same tables through its native `lastro-prove` binary.

Lastro's only AI steps are extracting intents from prose and deriving the proofs. It never writes the
implementation: any coding agent or person does, and `Prove` judges it.

```sh
go get github.com/iurykrieger/lastro/sdks/go
```

## Library

```go
import (
	lastro "github.com/iurykrieger/lastro/sdks/go"
	"github.com/iurykrieger/lastro/sdks/go/claude" // or any lastro.AskJSON transport
)

l := lastro.New(lastro.Options{
	Model:             model,                         // the vocabulary of the proofs
	Ask:               claude.Ask(claude.Options{}),  // the two AI steps
	RequiredApprovals: 2,                             // your team's policy; default 1
	Rust:              lastro.FindRustProver(),       // optional independent prover (lastro-prove)
})

intents, _ := l.Plan(ctx, prose, nil)                          // AI: extract reviewable intents
intents, _ = l.Review(intents, "ana").ApproveAll().Amend("I5", "...").Done()
intents, _ = l.Review(intents, "bruno").ApproveAll().Done()
l.Pending(intents)                                             // [] once the policy is met

spec, _ := l.Spec(ctx, intents)            // AI: derive clauses; then validated, checked for contradictions
if !spec.OK {
	intents = l.ReturnDecisions(spec)      // contradictions come back as decisions;
}                                          // each answer becomes an intent to approve

proof, _ := l.Prove(ctx, spec, decideLogin, nil) // deterministic: every input, plus the Rust prover if given
proof.OK                                         // gate a merge on it
proof.Receipt                                    // Markdown for the pull request
lastro.AgentFeedback(proof.Failures, proof.SpecGaps) // failures with counterexamples, for the coding agent
```

A subject is any `func(lastro.Env) (lastro.Env, error)`. Code in another language can be proved too:
`lastro.ProcessSubject` runs a program that reads one JSON input per line and writes one JSON output per line,
and `lastro.TableSubject` answers from a recorded input/output table.

`examples/login` is the TS SDK's worked example (a store login with six planted bugs) ported to Go.

## CLI

```sh
go install github.com/iurykrieger/lastro/sdks/go/cmd/lastro-go@latest

lastro-go init --model model.json --approvals 1
lastro-go plan prose.md                         # extract intents
lastro-go review --as ana approve-all           # approve | amend | reject | answer
lastro-go spec                                  # derive the formal spec; contradictions become decisions
lastro-go prove --cmd "./decide-login"          # JSON lines in, JSON lines out; or --table rows.json
lastro-go feedback                              # the failures as a brief for the coding agent
```

Every command prints one JSON object. Exit codes: 0 ok, 1 not ok, 2 error, 3 an LLM answer is needed. With
`ANTHROPIC_API_KEY` set the AI steps run on the Anthropic API; otherwise (`--llm handoff`) the request is
written to `.lastro/llm/` for the calling agent to answer with its own model. Request ids match the TS CLI's.

## Development

```sh
cargo build --release -p lastro-core   # lastro-prove, for the Rust cross-check
npm install && npm run build -w lastro # the TS SDK, for the TS cross-check
cd sdks/go && go test ./...
go generate ./...                      # refresh prompts.go from the TS SDK after changing its prompts
```

The cross-check tests skip (and say so) when `lastro-prove` or the built TS SDK is missing; CI runs them all.
