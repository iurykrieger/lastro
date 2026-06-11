# E10 — Branch inventory, journeys, and use-case coverage

> Extends: [`E4-use-case.md`](E4-use-case.md) (UseCase entity), [`B4-detection-generation.md`](B4-detection-generation.md) (detection skills)

Use-case detection today is purely inferential: the LLM reads the code and
invents scenarios. Nothing measures whether the detected use cases actually
cover the application's behavior. E10 adds a deterministic **branch
inventory engine** that parses the application source, extracts every logic
branch (if / else-if / else, switch and select cases, catch/ternary in
heuristic languages), and a **coverage metric** that scores the detected use
cases against that inventory. Use cases gain a **journey** grouping (one
folder per journey, each holding success / failure / alternative variations)
and a **covers** list binding each use case to the branch ids it exercises.

Determinism beats prediction (framework rule 2): the engine, the inventory
ids, and the coverage math are deterministic Go. The LLM's only inferential
job is condensing branches into human-meaningful journeys and scenario text.

## Entities

### Branch (new)

One decision point in the application source.

- `id` — stable: `br-<12 hex of sha256(file|kind|condition|ordinal)>`.
  Line numbers are excluded so ids survive unrelated edits; `ordinal`
  disambiguates identical conditions in the same file.
- `file` — repo-relative path.
- `line` — 1-based line at scan time (informational, not identity).
- `kind` — `schemas/enums/branch-kinds.yaml`: `if`, `else-if`, `else`,
  `case`, `default`, `catch`, `ternary`.
- `condition` — normalized condition text (single-spaced); empty for `else`.
- `enclosing` — enclosing function/method name when the analyzer knows it.

### BranchInventory (new)

`.harness/branch-inventory.yaml` — the engine's output. Header
(`schema_version`, `scanned_at` omitted by design — output must be
byte-stable), `source_root`, per-file `precision` (`ast` for Go,
`heuristic` for regex-scanned languages), and the flat `branches` list.

### UseCase (extended, stays schema_version 2.0.x — additive optional fields)

- `journey` — id of the journey this use case belongs to. Persisted path
  becomes `use-cases/<journey>/<id>.yaml`; absent → legacy flat path.
- `variation` — `schemas/enums/use-case-variations.yaml`: `success`,
  `failure`, `alternative`. A complete journey has at least one `success`
  and one `failure` use case.
- `covers` — list of Branch ids this use case's scenario exercises.
  Persist validates every id against the on-disk inventory
  (new persist error kind `unknown_branch_ref`).

### CoverageReport (new)

Computed, never hand-authored. Overall: `total_branches`,
`covered_branches`, `coverage_percent` (1 decimal). Per journey: use-case
count by variation + distinct covered count. `uncovered` lists every branch
with no covering use case (file, line, kind, condition) — this is the
actionable remainder the detection skill iterates on.

## Engine architecture (`internal/detect/branchscan`)

- `Analyzer` interface: `Supports(path) bool; Scan(relPath string, src []byte) ([]Branch, error)`.
- **Go analyzer** — stdlib `go/parser` + `go/ast`. Exact: `*ast.IfStmt`
  (else-if chains flattened, trailing else emitted), `*ast.SwitchStmt`,
  `*ast.TypeSwitchStmt`, `*ast.SelectStmt` clauses. Skips `_test.go`.
- **Heuristic analyzer** — line-regex scanner for `.js .jsx .ts .tsx .mjs
  .cjs .py .rb .php .java .cs`: `if (...)` / `elif` / `else`, `case X:`,
  `default:`, `catch`, ternaries. Marked `precision: heuristic` so
  consumers know counts are approximate.
- **Walker** — repo-relative walk skipping dot-dirs, `vendor/`,
  `node_modules/`, `dist/`, `build/`, `.harness/`, and test files.
- Future: tree-sitter analyzers per language would upgrade heuristic →
  ast precision, but CGO breaks the pure-Go five-platform cross-compile in
  the Makefile, so v1 stays stdlib-only (rule 5 trade-off, documented here).

## CLI (`lastro`, consumed by skills via harness-tools.sh)

- `scan-branches --src <dir> --harness-dir <dir>` — writes
  `branch-inventory.yaml`, prints `{"total": N, "files": M}` JSON.
- `coverage --harness-dir <dir>` — reads inventory + all use cases
  (foldered and flat), writes `coverage.yaml`, prints the report as JSON.
  Exit 0 always (the metric is information, not a gate; policy gating can
  come later).

## Skill flow (`/detect-use-cases`, rewritten)

1. `scan-branches` (deterministic) → read the inventory.
2. Group branches into journeys by entry point / feature (inferential).
3. Per journey emit fixtures + use cases: one `success`, one `failure`,
   `alternative` per remaining meaningful branch cluster; each use case
   lists the branch ids it `covers`.
4. `coverage` (deterministic) → report the percentage; iterate on the
   `uncovered` list until covered or explicitly justified as unreachable.

`/create-sensors` resolves `use-cases/<journey>/<id>.yaml` as well as flat
paths; sensor design per angle is unchanged (see `angles.md`).

## Compatibility

- Flat `use-cases/<id>.yaml` files without journey/variation/covers remain
  valid (all three fields optional; loaders accept both layouts).
- No schema_version major bump: additive optional properties.
- Sensors and runtime are untouched except use-case path resolution.

## Acceptance

- `scan-branches` on `examples/http-api-sample` finds the handler branches
  with `precision: ast` and byte-identical output across runs.
- A use case persisted with `journey: orders` lands in
  `use-cases/orders/`, loads through `cmd/harness` loaders, and resolves
  from sensor persist.
- `coverage` over the sample reports a deterministic percentage and lists
  the uncovered branches.
- A `covers` id absent from the inventory fails persist with
  `unknown_branch_ref`.
