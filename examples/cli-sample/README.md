# cli-sample

Minimal archetype-`cli` subject used by Track 1 to prove detection
branches downstream sensors by archetype.

## Usage

```
cd examples/cli-sample
go mod tidy
go run . greet --name World
# → Hello, World
```

## What it demonstrates

- Stack detection (archetype=`cli`) — distinct from `http-api`.
- One use case (`uc-greet-by-name`) with one fixture and two sensors
  (`e2e-test` + `unit-test`) — same fixture-reuse property as the
  http-api sample.
