// Command go-library is the quickstart as Go library calls: plan, review, spec, prove.
// Run it with Claude: ANTHROPIC_API_KEY=... go run .
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	lastro "github.com/iurykrieger/lastro/sdks/go"
	"github.com/iurykrieger/lastro/sdks/go/claude"
)

// shippingFee is the decision core under proof.
func shippingFee(in lastro.Env) (lastro.Env, error) {
	subtotal, _ := in["subtotal"].(float64) // numbers arrive as float64, like JSON
	member, _ := in["member"].(bool)
	if member || subtotal >= 100 {
		return lastro.Env{"fee": 0}, nil
	}
	return lastro.Env{"fee": 15}, nil
}

func proveShipping(ctx context.Context, ask lastro.AskJSON, subject lastro.Subject) (lastro.Proof, error) {
	var model lastro.Model
	raw, err := os.ReadFile("../model.json")
	if err != nil {
		return lastro.Proof{}, err
	}
	if err := json.Unmarshal(raw, &model); err != nil {
		return lastro.Proof{}, err
	}
	prose, err := os.ReadFile("../prose.md")
	if err != nil {
		return lastro.Proof{}, err
	}

	opts := lastro.Options{Model: model, Ask: ask, RequiredApprovals: 1}
	if p := lastro.FindRustProver(); p != nil { // lastro-prove: $LASTRO_PROVE, PATH or ./target
		opts.Rust = p
	}
	l := lastro.New(opts)

	intents, err := l.Plan(ctx, string(prose), nil) // AI: extract intents
	if err != nil {
		return lastro.Proof{}, err
	}
	intents, err = l.Review(intents, "ana").Answer("D1", 1).ApproveAll().Done()
	if err != nil {
		return lastro.Proof{}, err
	}
	if pending := l.Pending(intents); len(pending) > 0 {
		return lastro.Proof{}, fmt.Errorf("review pending: %v", pending)
	}

	spec, err := l.Spec(ctx, intents) // AI: derive the formal spec, then validate it
	if err != nil {
		return lastro.Proof{}, err
	}
	if !spec.OK {
		return lastro.Proof{}, fmt.Errorf("the spec contradicts itself: answer l.ReturnDecisions(spec)")
	}
	return l.Prove(ctx, spec, subject, nil) // deterministic, in both engines
}

func main() {
	proof, err := proveShipping(context.Background(), claude.Ask(claude.Options{}), shippingFee)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(proof.Receipt)
	if !proof.OK {
		fmt.Fprintln(os.Stderr, lastro.AgentFeedback(proof.Failures, proof.SpecGaps))
		os.Exit(1)
	}
}
