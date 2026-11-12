# Fix what the use-case proofs found

## Contradictions with clauses that always apply

The consistency check treats a clause with no if-part as applying to every input, in the TypeScript, .NET, Go and
Rust engines alike: for each input it looks for an output that satisfies every clause whose if-part holds, and every
clause with no if-part. So a clause that says y is true for every input and a clause that says y is false when x
holds come back as a contradiction for x = true, in every SDK.

## Coverage of the Rust prover

The Rust prover drops out-of-domain rows and counts missing inputs and nondeterministic rows (an input that appears
with two different outputs). It calls coverage complete when every input of the space has a row and no row is
nondeterministic or out of domain; an input repeated with the same output still counts as covered once.

## Streamed proof runs

lastro prove --stream prints one compact JSON object per line instead of the single result: start (the proposal and
the ids of the proofs it runs), progress every 100 runs of the code under proof, one proof event per proof, the
proposal's own and every use case's regression proof alike (proof, role, verdict, passed, and the counterexample when
there is one), crosscheck (enginesAgree) when the Rust prover ran, and result with the body prove prints without
--stream. Exit codes and the files written are the same as without it. The Go and .NET tests compare their stream
with the TypeScript CLI's on one workspace.
