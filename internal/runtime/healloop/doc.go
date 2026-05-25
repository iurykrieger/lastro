// Package healloop owns the iteration loop that turns a failing
// UseCaseVerdict into either a healed use case, an exhausted attempt, or an
// abandoned attempt — without ever leaving the working tree in a partially
// edited state.
//
// The loop is pure orchestration: it composes Phase A entities
// (HealHint, AggregateSignal) with B1 (per-use-case aggregator) and B2
// (lifecycle) and delegates the LLM call itself to an LLMClient interface
// satisfied by the /heal skill scripts at B5.
//
// See docs/superpowers/specs/2026-05-25-b3-heal-loop-design.md for the
// design and docs/harness-framework/B3-heal-loop.md for the source chunk.
package healloop
