# Parameterized Sensors via Composition Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let core sensors declare `inputs:`/`outputs:` and be composed by per-use-case sensors via GitHub-Actions-style `uses:` + `with:`, resolved deterministically at runtime.

**Architecture:** Reuse and extend the existing `internal/usecase/template` interpolation engine (migrate its `{{ }}` sentinel to `${{ }}`, add `inputs`/`steps.<id>.outputs` contexts, add a compile-to-env mode). A sensor step becomes a discriminated union — a `run`-step or a `uses`-step (`uses: <primitive> + with:`). At execution the runtime inlines the composed primitive's steps, binds inputs/fixtures/step-outputs as **environment variables** (never inline payloads), and propagates the primitive's `depends_on`. Generation skills emit primitives (`/create-core-sensors`) and consumers (`/create-sensors`).

**Tech Stack:** Go (stdlib + `sigs.k8s.io/yaml`, `santhosh-tekuri/jsonschema` via `internal/sensor/schema.go`), YAML schemas under `schemas/`, skill scripts under `skills/*/scripts/`.

**Spec:** [`docs/superpowers/specs/2026-06-01-parameterized-sensors-composition-design.md`](../specs/2026-06-01-parameterized-sensors-composition-design.md)

---

## File Structure

**Interpolation engine** (`internal/usecase/template/`):
- `grammar.go` — segment AST; add `InputRef`, `StepOutputRef`; doc `${{ }}`.
- `parser.go` — sentinel `{{`→`${{`; new namespaces `inputs`, `steps`.
- `label.go` — `RenderLabels` for the new refs.
- `compile.go` *(new)* — compile-to-env-ref mode (the safety boundary).
- `resolver.go` — unchanged behavior for use-case-text inline render.

**Sensor model** (`internal/sensor/`):
- `types.go` — `Step` union (`Run` xor `Uses` scalar + `With`); `InputSpec`, `OutputSpec`, `Sensor.Inputs/Outputs`.
- `loader.go` — defaults.
- `validate.go` — step-union shape; input/output name checks; drop array-`Uses` uniqueness.
- `compose.go` *(new)* — store/resolver-level checks: `uses`-target is `scope: core`; semantic step-id refs.

**Runtime** (`internal/runtime/`):
- `fixturebinder/binder.go` + `collect.go` *(new)* — collect fixture refs from `run`/`with`, drop `step.Uses` array.
- `executor/step.go` — lift `ErrTemplateFixtureInRun`; wire the env-compiler; capture `$HARNESS_OUTPUT`.
- `executor/compose.go` *(new)* — expand `uses`-steps into primitive run-steps with bound input env.
- `executor/executor.go` — `Options.SensorLookup`; step-output accumulation across the Run loop.

**Schema** (`schemas/`):
- `sensor.yaml` — step `oneOf(run|uses)`; `inputs`/`outputs`; step `uses:` array→scalar.
- `examples/sensor/` — a primitive + a consumer example.

**Skills** (`skills/`):
- `create-core-sensors/` *(new)* — SKILL.md + scripts.
- `create-sensors/` — emit `uses`/`with` consumers; script accepts the new shape.

**Migration:** `.harness/use-cases/*.yaml`, `schemas/examples/use-case/*.yaml`, `.harness/sensors/*.yaml`, `docs/harness-framework/{plan.md,E4-use-case.md,00-schema-freeze.md}`.

---

## Phase 0 — Schema freeze gate

### Task 1: Record the schema change in the freeze gate

**Files:**
- Modify: `docs/harness-framework/00-schema-freeze.md`

- [ ] **Step 1: Append a freeze record**

Add this section to `00-schema-freeze.md` (after the existing core-sensors record):

```markdown
## 2026-06-01 — Parameterized sensors via composition (#26)

- `sensor.yaml` step: discriminated union — a step has **either** `run` (string) **or**
  `uses` (a single primitive sensor id) + optional `with` (map[string]string). The previous
  step-level `uses: [fixture-id]` **array** is removed; fixtures are referenced by
  `${{ fixtures.<id> }}` interpolation in `run`/`with`.
- `sensor.yaml` adds optional top-level `inputs` (map of `{required?, default?, description?}`)
  and `outputs` (map of `{from, description?}`).
- Interpolation sentinel migrates repo-wide from `{{ }}` to `${{ }}`. New contexts:
  `${{ inputs.<name> }}` and `${{ steps.<id>.outputs.<name> }}`, alongside existing
  `${{ fixtures.* }}` / `${{ entry_points.* }}`.
- The executor's `{{fixtures.X}}`-in-run ban (`ErrTemplateFixtureInRun`) is lifted; fixture/input/
  step-output refs compile to env-var references (`HARNESS_FIXTURE_*`, `HARNESS_INPUT_*`,
  `HARNESS_STEPOUT_*`), never inline payloads.
```

- [ ] **Step 2: Commit**

```bash
git add docs/harness-framework/00-schema-freeze.md
git commit -m "docs(schema-freeze): record parameterized sensors + ${{ }} sentinel (#26)"
```

---

## Phase 1 — Grammar migration `{{ }}` → `${{ }}`

### Task 2: Migrate the parser sentinel and template package

**Files:**
- Modify: `internal/usecase/template/parser.go`
- Modify: `internal/usecase/template/grammar.go` (doc only)
- Modify: `internal/usecase/template/label.go` (doc only)
- Test: `internal/usecase/template/parser_test.go`, `label_test.go`, `resolver_test.go`, `walk_test.go`

- [ ] **Step 1: Update the failing tests first**

In every `internal/usecase/template/*_test.go`, replace each literal `{{` with `${{` in test inputs
and expected strings (the `}}` close is unchanged). Example edits in `parser_test.go`:

```go
// before: Parse("a {{fixtures.fx-order}} b")
// after:
got, err := Parse("a ${{fixtures.fx-order}} b")
```

Apply the same `{{`→`${{` substitution in `label_test.go`, `resolver_test.go`, `walk_test.go`.

- [ ] **Step 2: Run the template tests to verify they fail**

Run: `go test ./internal/usecase/template/ 2>&1 | head -20`
Expected: FAIL — the parser still looks for `{{`, so `${{...}}` inputs are treated as literal text and refs don't parse.

- [ ] **Step 3: Migrate the parser sentinel**

In `parser.go`, change the open-sentinel detection in `parse()` and the consume in `parseRef()`:

```go
// in parse(): replace the peek2("{{") branch
for p.pos < len(p.input) {
	if p.peek2("${") && p.pos+3 <= len(p.input) && p.input[p.pos+1:p.pos+3] == "{{" {
		flushLiteral()
		seg, err := p.parseRef()
		if err != nil {
			return nil, err
		}
		out = append(out, seg)
		continue
	}
	lit = append(lit, p.input[p.pos])
	p.advance(1)
}
```

```go
// in parseRef(): consume "${{" instead of "{{"
func (p *parser) parseRef() (Segment, error) {
	p.advance(3) // consume "${{"
	p.skipWS()

	if p.peek2("${") {
		return nil, &ParseError{Pos: p.here(), Msg: "nested ${{ inside template"}
	}
	// ... rest unchanged ...
}
```

Add a small helper to keep `parse()` readable (replace the inline check above with it):

```go
// peekOpen reports whether the cursor is at the "${{" open sentinel.
func (p *parser) peekOpen() bool {
	return p.pos+3 <= len(p.input) && p.input[p.pos:p.pos+3] == "${{"
}
```

Then `parse()` uses `if p.peekOpen() {`.

- [ ] **Step 4: Update doc comments**

In `grammar.go` line 1–13 and `label.go`'s doc block, replace `{{ }}`/`{{fixtures...}}` with
`${{ }}`/`${{fixtures...}}`. In `parser.go` the `Parse` doc comment ("no {{ }} blocks") → "no `${{ }}` blocks".

- [ ] **Step 5: Run the template tests to verify they pass**

Run: `go test ./internal/usecase/template/ -v 2>&1 | tail -20`
Expected: PASS (all parser/label/resolver/walk tests).

- [ ] **Step 6: Commit**

```bash
git add internal/usecase/template/
git commit -m "feat(template): migrate interpolation sentinel {{ }} -> \${{ }} (#26)"
```

### Task 3: Migrate on-disk references and frozen docs

**Files:**
- Modify: `.harness/use-cases/uc-harness-validate-use-case.yaml`
- Modify: `schemas/examples/use-case/*.yaml` (9 files), `schemas/use-case.yaml` (doc comment)
- Modify: `docs/harness-framework/plan.md` (§4.1.2 + the `{{ }}` examples), `docs/harness-framework/E4-use-case.md`
- Test: `internal/usecase/{loader_test,persist_test,validate_test}.go`, `internal/runtime/executor/{run_test,step_test}.go`

- [ ] **Step 1: Migrate caller tests first**

In the five test files above, replace `{{` with `${{` in any string literal that is a template ref.
(`step_test.go` has the `ErrTemplateFixtureInRun` test — leave that test's `${{fixtures.X}}` input as
`${{...}}`; it is rewritten in Task 11. For now just migrate the sentinel.)

- [ ] **Step 2: Run usecase + executor tests to verify they fail**

Run: `go test ./internal/usecase/... ./internal/runtime/executor/ 2>&1 | head -30`
Expected: FAIL — on-disk YAML and golden fixtures still use `{{`, so resolution/round-trip mismatches.

- [ ] **Step 3: Migrate on-disk YAML and docs**

Repo-wide, in YAML and doc files only, replace `{{` with `${{`:

```bash
# YAML data files
grep -rl '{{' .harness schemas | grep -E '\.ya?ml$' | \
  xargs sed -i '' -e 's/{{/${{/g'
# frozen docs (review the diff before committing)
sed -i '' -e 's/{{/${{/g' docs/harness-framework/plan.md docs/harness-framework/E4-use-case.md
```

Then open `docs/harness-framework/plan.md` §4.1.2 ("`{{ }}` interpolation grammar") and the prose
heading/table, and `E4-use-case.md`, and confirm every grammar mention now reads `${{ }}` (the `sed`
covered code-fenced examples; double-check prose like "via `${{ }}` interpolation").

- [ ] **Step 4: Run the full suite to verify it passes**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS. (If a golden JSON under `testdata/` embeds a rendered `{{...}}` label, update it too.)

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor(harness): migrate on-disk {{ }} refs and frozen docs to \${{ }} (#26)"
```

---

## Phase 2 — New interpolation contexts + env-compiler

### Task 4: Add `InputRef` and `StepOutputRef` segment types

**Files:**
- Modify: `internal/usecase/template/grammar.go`, `parser.go`, `label.go`
- Test: `internal/usecase/template/parser_test.go`, `label_test.go`

- [ ] **Step 1: Write failing parser + label tests**

Add to `parser_test.go`:

```go
func TestParseInputRef(t *testing.T) {
	got, err := Parse("x ${{ inputs.method }} y")
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	want := []Segment{Literal{Text: "x "}, InputRef{Name: "method", Pos: Position{Line: 1, Col: 3, Offset: 2}}, Literal{Text: " y"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
}

func TestParseStepOutputRef(t *testing.T) {
	got, err := Parse("${{ steps.create.outputs.charge-id }}")
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	ref, ok := got[0].(StepOutputRef)
	if !ok || ref.StepID != "create" || ref.Name != "charge-id" {
		t.Errorf("got %#v", got[0])
	}
}

func TestParseStepOutputRefRequiresOutputs(t *testing.T) {
	if _, err := Parse("${{ steps.create.charge-id }}"); err == nil {
		t.Fatal("expected error for missing 'outputs' segment")
	}
}
```

Add to `label_test.go`:

```go
func TestRenderLabelsInputAndStepOutput(t *testing.T) {
	segs, _ := Parse("${{ inputs.method }} ${{ steps.create.outputs.id }}")
	if got := RenderLabels(segs); got != "[input: method] [step: create.outputs.id]" {
		t.Errorf("got %q", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/usecase/template/ -run 'Input|StepOutput' -v 2>&1 | head -20`
Expected: FAIL — `InputRef`/`StepOutputRef` undefined; parser rejects `inputs`/`steps` namespaces.

- [ ] **Step 3: Add the segment types**

In `grammar.go`:

```go
// InputRef is `${{ inputs.<name> }}` — a composed primitive's declared input.
type InputRef struct {
	Name string
	Pos  Position
}

func (InputRef) isSegment() {}

// StepOutputRef is `${{ steps.<id>.outputs.<name> }}` — an output produced by
// a prior step (or by a uses-step that composed a primitive).
type StepOutputRef struct {
	StepID string
	Name   string
	Pos    Position
}

func (StepOutputRef) isSegment() {}
```

- [ ] **Step 4: Parse the new namespaces**

In `parser.go` `parseRef()` switch, add cases:

```go
	switch ns {
	case "fixtures":
		return p.parseFixtureTail(nsStart)
	case "entry_points":
		return p.parseEntryPointTail(nsStart)
	case "inputs":
		return p.parseInputTail(nsStart)
	case "steps":
		return p.parseStepOutputTail(nsStart)
	default:
		return nil, &ParseError{Pos: nsStart, Msg: "unknown namespace: " + ns}
	}
```

Add the tail parsers:

```go
func (p *parser) parseInputTail(refPos Position) (Segment, error) {
	name, ok := p.readKebabID()
	if !ok {
		return nil, &ParseError{Pos: p.here(), Msg: "expected input name"}
	}
	if p.peekByte('.') {
		return nil, &ParseError{Pos: p.here(), Msg: "inputs.<name> takes no further keys"}
	}
	if err := p.expectClose(); err != nil {
		return nil, err
	}
	return InputRef{Name: name, Pos: refPos}, nil
}

func (p *parser) parseStepOutputTail(refPos Position) (Segment, error) {
	stepID, ok := p.readKebabID()
	if !ok {
		return nil, &ParseError{Pos: p.here(), Msg: "expected step id"}
	}
	if !p.peekByte('.') {
		return nil, &ParseError{Pos: p.here(), Msg: "expected '.outputs.' after step id"}
	}
	p.advance(1)
	if seg := p.readIdent(isIdentByte); seg != "outputs" {
		return nil, &ParseError{Pos: p.here(), Msg: "steps.<id> only accepts '.outputs.<name>'; got '" + seg + "'"}
	}
	if !p.peekByte('.') {
		return nil, &ParseError{Pos: p.here(), Msg: "expected '.' after 'outputs'"}
	}
	p.advance(1)
	name, ok := p.readKebabID()
	if !ok {
		return nil, &ParseError{Pos: p.here(), Msg: "expected output name"}
	}
	if err := p.expectClose(); err != nil {
		return nil, err
	}
	return StepOutputRef{StepID: stepID, Name: name, Pos: refPos}, nil
}
```

- [ ] **Step 5: Render labels for the new refs**

In `label.go` `RenderLabels`, add cases to the switch:

```go
		case InputRef:
			b.WriteString("[input: ")
			b.WriteString(v.Name)
			b.WriteByte(']')
		case StepOutputRef:
			b.WriteString("[step: ")
			b.WriteString(v.StepID)
			b.WriteString(".outputs.")
			b.WriteString(v.Name)
			b.WriteByte(']')
```

- [ ] **Step 6: Run to verify pass**

Run: `go test ./internal/usecase/template/ -v 2>&1 | tail -20`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/usecase/template/
git commit -m "feat(template): add inputs and steps.outputs ref types (#26)"
```

### Task 5: Compile-to-env mode (the safety boundary)

**Files:**
- Create: `internal/usecase/template/compile.go`
- Test: `internal/usecase/template/compile_test.go`

- [ ] **Step 1: Write failing compiler tests**

Create `compile_test.go`:

```go
package template

import (
	"reflect"
	"testing"
)

func TestCompileRewritesRefsToEnvAndCollects(t *testing.T) {
	segs, _ := Parse(`curl -X ${{ inputs.method }} ${{ fixtures.body }} ${{ steps.create.outputs.id }}`)
	out, refs, err := Compile(segs)
	if err != nil {
		t.Fatalf("Compile err: %v", err)
	}
	want := `curl -X "${HARNESS_INPUT_METHOD}" "${HARNESS_FIXTURE_BODY}" "${HARNESS_STEPOUT_CREATE_ID}"`
	if out != want {
		t.Errorf("compiled = %q, want %q", out, want)
	}
	wantRefs := Refs{
		Inputs:      []string{"method"},
		Fixtures:    []string{"body"},
		StepOutputs: []StepOutputRef{{StepID: "create", Name: "id", Pos: refs.StepOutputs[0].Pos}},
	}
	if !reflect.DeepEqual(refs.Inputs, wantRefs.Inputs) ||
		!reflect.DeepEqual(refs.Fixtures, wantRefs.Fixtures) ||
		len(refs.StepOutputs) != 1 {
		t.Errorf("refs = %#v", refs)
	}
}

func TestCompileNeverInlinesValue(t *testing.T) {
	// A fixture payload with shell metacharacters must not appear in the compiled string.
	segs, _ := Parse(`echo ${{ fixtures.evil }}`)
	out, _, _ := Compile(segs)
	if out != `echo "${HARNESS_FIXTURE_EVIL}"` {
		t.Errorf("got %q", out)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/usecase/template/ -run Compile 2>&1 | head -10`
Expected: FAIL — `Compile`, `Refs` undefined.

- [ ] **Step 3: Implement the compiler**

Create `compile.go`:

```go
package template

import (
	"fmt"
	"strings"
)

// Refs is the set of references a compiled string depends on. The executor
// uses it to build the env map and to know which fixtures/step-outputs to bind.
type Refs struct {
	Inputs      []string        // input names, in first-seen order
	Fixtures    []string        // fixture ids, in first-seen order
	StepOutputs []StepOutputRef // step output refs, in first-seen order
	EntryPoints []EntryPointRef // entry point refs, in first-seen order
}

// Compile rewrites every ref segment into a POSIX shell variable reference
// (always double-quoted) and returns the rewritten string plus the collected
// Refs. Values are NEVER inlined — this is the shell-injection safety boundary.
// Env var names: HARNESS_INPUT_<U>, HARNESS_FIXTURE_<U>, HARNESS_STEPOUT_<STEP>_<U>,
// HARNESS_ENTRYPOINT_<U> where <U> = upper(name) with '-' -> '_'.
func Compile(segs []Segment) (string, Refs, error) {
	var b strings.Builder
	var refs Refs
	seen := map[string]bool{}
	for _, s := range segs {
		switch v := s.(type) {
		case Literal:
			b.WriteString(v.Text)
		case InputRef:
			b.WriteString(`"${` + envName("HARNESS_INPUT_", v.Name) + `}"`)
			if !seen["i:"+v.Name] {
				seen["i:"+v.Name] = true
				refs.Inputs = append(refs.Inputs, v.Name)
			}
		case FixtureRef:
			if len(v.JSONPath) > 0 {
				return "", Refs{}, &ResolveError{Pos: v.Pos, Msg: "fixture jsonpath drilling is not supported in sensor steps; bind the whole fixture"}
			}
			b.WriteString(`"${` + envName("HARNESS_FIXTURE_", v.ID) + `}"`)
			if !seen["f:"+v.ID] {
				seen["f:"+v.ID] = true
				refs.Fixtures = append(refs.Fixtures, v.ID)
			}
		case StepOutputRef:
			b.WriteString(`"${` + "HARNESS_STEPOUT_" + up(v.StepID) + "_" + up(v.Name) + `}"`)
			refs.StepOutputs = append(refs.StepOutputs, v)
		case EntryPointRef:
			key := v.ID
			if v.SpecKey != "" {
				key = v.ID + "_" + v.SpecKey
			}
			b.WriteString(`"${` + envName("HARNESS_ENTRYPOINT_", key) + `}"`)
			refs.EntryPoints = append(refs.EntryPoints, v)
		default:
			return "", Refs{}, fmt.Errorf("compile: unknown segment %T", s)
		}
	}
	return b.String(), refs, nil
}

func envName(prefix, id string) string { return prefix + up(id) }
func up(s string) string                { return strings.ToUpper(strings.ReplaceAll(s, "-", "_")) }
```

> Note: fixture jsonpath drilling (`${{ fixtures.x.k }}`) stays available in **use-case text** (via
> `Resolve`) but is rejected in **sensor steps** (via `Compile`), because a step binds the whole payload
> as a file/env value. This keeps the binder's "one fixture = one file/env" model intact.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/usecase/template/ -v 2>&1 | tail -15`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/template/compile.go internal/usecase/template/compile_test.go
git commit -m "feat(template): compile-to-env-ref mode (shell-safe interpolation) (#26)"
```

---

## Phase 3 — Sensor schema, types, validators

### Task 6: Schema — step union + inputs/outputs

**Files:**
- Modify: `schemas/sensor.yaml`
- Create: `schemas/examples/sensor/core-e2e-primitive.yaml`, `schemas/examples/sensor/uc-consumer.yaml`
- Test: `internal/sensor/schema_test.go` (exercises examples)

- [ ] **Step 1: Write the example files (these are the schema's golden inputs)**

Create `schemas/examples/sensor/core-e2e-primitive.yaml`:

```yaml
schema_version: 1.0.0
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [curl]
depends_on: [run-dev]
inputs:
  method:        { required: true, default: GET }
  path:          { required: true, default: /health_check/ready }
  expect_status: { required: false, default: "2xx" }
outputs:
  body: { from: "${{ steps.request.outputs.body }}" }
steps:
  - id: request
    run: |
      resp=$(curl --fail -sS -X "${{ inputs.method }}" "http://localhost:8080${{ inputs.path }}")
      printf 'body=%s\n' "$resp" >> "$HARNESS_OUTPUT"
```

Create `schemas/examples/sensor/uc-consumer.yaml`:

```yaml
schema_version: 1.0.0
id: create-charge-e2e
scope: use-case
use_case_id: uc-create-charge
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: []
steps:
  - id: create
    uses: e2e-test
    with:
      method: POST
      path: /v1/charges
      body: ${{ fixtures.create-charge-input }}
      expect_status: "201"
```

- [ ] **Step 2: Update `schemas/sensor.yaml`**

Replace the `SensorStep` `$def` and add `inputs`/`outputs` to `properties`. The `SensorStep` becomes:

```yaml
  SensorStep:
    type: object
    required: [id]
    additionalProperties: false
    oneOf:
      - required: [run]
        not: { required: [uses] }
        properties: { with: false }
      - required: [uses]
        not: { required: [run] }
    properties:
      id:   { $ref: "#/$defs/Id" }
      run:  { type: string, minLength: 1 }
      uses: { $ref: "#/$defs/Id" }
      with:
        type: object
        additionalProperties: { type: string }
```

Add to top-level `properties` (after `steps`):

```yaml
  inputs:
    type: object
    additionalProperties:
      type: object
      additionalProperties: false
      properties:
        required:    { type: boolean, default: false }
        default:     { type: string }
        description: { type: string }
  outputs:
    type: object
    additionalProperties:
      type: object
      required: [from]
      additionalProperties: false
      properties:
        from:        { type: string, minLength: 1 }
        description: { type: string }
```

- [ ] **Step 3: Point the schema example test at a new example (or add a case)**

Inspect `internal/sensor/schema_test.go`; ensure it validates files under `schemas/examples/sensor/`.
If it validates a single hard-coded path, add a case loading both new examples:

```go
func TestSchemaAcceptsCompositionExamples(t *testing.T) {
	for _, f := range []string{"core-e2e-primitive.yaml", "uc-consumer.yaml"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "schemas", "examples", "sensor", f))
		if err != nil { t.Fatal(err) }
		if _, err := LoadSensorBytes(raw); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}
```

(This test will fail to *fully* pass until Task 7 adds the Go fields; Step 4 here only asserts schema-level
acceptance via `validateAgainstSchema`. If `LoadSensorBytes` errors on intrinsic checks, split the assertion
to call `validateAgainstSchema(yamlToJSON(raw))` directly for now and tighten in Task 7.)

- [ ] **Step 4: Run schema test**

Run: `go test ./internal/sensor/ -run Schema -v 2>&1 | tail -15`
Expected: PASS at the schema layer (both examples validate; the consumer's `uses`-step and the primitive's
`run`-step each satisfy exactly one `oneOf` branch).

- [ ] **Step 5: Commit**

```bash
git add schemas/sensor.yaml schemas/examples/sensor/ internal/sensor/schema_test.go
git commit -m "feat(schema): sensor step union + inputs/outputs (#26)"
```

### Task 7: Sensor Go types + loader

**Files:**
- Modify: `internal/sensor/types.go`, `internal/sensor/loader.go`
- Test: `internal/sensor/types_test.go`, `internal/sensor/loader_test.go`

- [ ] **Step 1: Write failing type tests**

Add to `types_test.go`:

```go
func TestStepUnmarshalUsesScalarAndWith(t *testing.T) {
	var s Step
	if err := json.Unmarshal([]byte(`{"id":"create","uses":"e2e-test","with":{"method":"POST"}}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.Uses != "e2e-test" || s.With["method"] != "POST" || s.Run != "" {
		t.Errorf("got %#v", s)
	}
}

func TestSensorInputsOutputsParse(t *testing.T) {
	raw := []byte(`{"schema_version":"1.0.0","id":"e2e-test","scope":"core","angle":"e2e-test",` +
		`"kind":"assertion","nature":"computational","output_type":"single-shot","uses":[],` +
		`"inputs":{"method":{"required":true,"default":"GET"}},` +
		`"outputs":{"body":{"from":"${{ steps.request.outputs.body }}"}},` +
		`"steps":[{"id":"request","run":"echo hi"}]}`)
	s, err := LoadSensorBytes(raw)
	if err != nil { t.Fatal(err) }
	if !s.Inputs["method"].Required || s.Inputs["method"].Default != "GET" {
		t.Errorf("inputs = %#v", s.Inputs)
	}
	if s.Outputs["body"].From == "" {
		t.Errorf("outputs = %#v", s.Outputs)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/sensor/ -run 'StepUnmarshal|InputsOutputs' 2>&1 | head -15`
Expected: FAIL — `Step.Uses` is `[]string`; `Step.With`, `Sensor.Inputs`, `Sensor.Outputs`, `InputSpec`,
`OutputSpec` undefined.

- [ ] **Step 3: Update the Go types**

In `types.go`, replace the `Step` struct and add the spec types + `Sensor` fields:

```go
// Sensor is the in-memory representation of a loaded sensor YAML.
type Sensor struct {
	SchemaVersion string                 `json:"schema_version"`
	ID            string                 `json:"id"`
	Scope         enums.SensorScope      `json:"scope,omitempty"`
	UseCaseID     string                 `json:"use_case_id,omitempty"`
	Angle         enums.ValidationAngle  `json:"angle"`
	Kind          enums.SensorKind       `json:"kind"`
	Nature        enums.SensorNature     `json:"nature"`
	OutputType    enums.SignalOutputType `json:"output_type"`
	Uses          []string               `json:"uses"`
	DependsOn     []string               `json:"depends_on,omitempty"`
	Inputs        map[string]InputSpec   `json:"inputs,omitempty"`
	Outputs       map[string]OutputSpec  `json:"outputs,omitempty"`
	Steps         []Step                 `json:"steps"`
}

// Step is one step of a Sensor. Exactly one of Run / Uses is set
// (enforced by the schema oneOf and by validateIntrinsic).
//   - Run-step:  Run is the shell command; With is empty.
//   - Uses-step: Uses is a core primitive's id; With binds its inputs.
type Step struct {
	ID   string            `json:"id"`
	Run  string            `json:"run,omitempty"`
	Uses string            `json:"uses,omitempty"`
	With map[string]string `json:"with,omitempty"`
}

// InputSpec declares a composable input on a primitive sensor.
type InputSpec struct {
	Required    bool   `json:"required,omitempty"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
	HasDefault  bool   `json:"-"` // set by loader: distinguishes "" default from absent
}

// OutputSpec declares a re-exported output on a primitive sensor.
type OutputSpec struct {
	From        string `json:"from"`
	Description string `json:"description,omitempty"`
}
```

Update the package doc comment's reference to step `Uses` being fixture ids — it is now the composed
primitive id; fixtures are referenced by `${{ fixtures.* }}`.

> `HasDefault` cannot be derived from `Default == ""` (a primitive may default to the empty string).
> Set it in the loader by re-scanning the raw JSON. Add to `loader.go` after unmarshal:

```go
// mark which inputs carried an explicit default (for required-input validation).
if len(s.Inputs) > 0 {
	var probe struct {
		Inputs map[string]map[string]json.RawMessage `json:"inputs"`
	}
	_ = json.Unmarshal(asJSON, &probe)
	for name, spec := range s.Inputs {
		if _, ok := probe.Inputs[name]["default"]; ok {
			spec.HasDefault = true
			s.Inputs[name] = spec
		}
	}
}
```

- [ ] **Step 4: Fix `grounding.go` (it stops compiling on the type change)**

`ValidateAgainstFixtures` in `internal/sensor/grounding.go` iterates `for _, id := range st.Uses` treating
each as a fixture id. With `Step.Uses` now a scalar primitive id, this both fails to compile and is
semantically wrong — fixtures are referenced by `${{ fixtures.<id> }}` in `run`/`with`. Rewrite it to collect
fixture refs from the step's `run` and `with` via the template package:

```go
import (
	"errors"
	"fmt"

	"github.com/iurykrieger/lastro/internal/stack"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

func ValidateAgainstFixtures(s Sensor, owner UseCaseFixtureOwnership) error {
	owned := make(map[string]bool)
	for _, id := range owner.OwnedFixtureIDs(s.UseCaseID) {
		owned[id] = true
	}
	var errs []error
	for _, st := range s.Steps {
		refs, err := collectStepFixtureRefs(st)
		if err != nil {
			errs = append(errs, fmt.Errorf("step %q: %w", st.ID, err))
			continue
		}
		var unknown []string
		for _, id := range refs {
			if !owned[id] {
				unknown = append(unknown, id)
			}
		}
		if len(unknown) > 0 {
			errs = append(errs, fmt.Errorf("step %q: unknown fixture(s) %v", st.ID, unknown))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// collectStepFixtureRefs returns the fixture ids referenced by a step's run
// and with values via ${{ fixtures.<id> }}.
func collectStepFixtureRefs(st Step) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	scan := func(src string) error {
		segs, err := template.Parse(src)
		if err != nil {
			return err
		}
		_, refs, err := template.Compile(segs)
		if err != nil {
			return err
		}
		for _, id := range refs.Fixtures {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
		return nil
	}
	if err := scan(st.Run); err != nil {
		return nil, err
	}
	for _, v := range st.With {
		if err := scan(v); err != nil {
			return nil, err
		}
	}
	return out, nil
}
```

(`internal/usecase/template` imports `fixture`/`entrypoint`, not `sensor`, so there is no import cycle.)
Update any `grounding_test.go` case that built a step with `Uses: []string{...}` for fixtures to instead put
`${{ fixtures.<id> }}` in the step's `Run`.

- [ ] **Step 5: Run to verify pass**

Run: `go test ./internal/sensor/ -run 'StepUnmarshal|InputsOutputs|Grounding|Fixtures' -v 2>&1 | tail -20`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/sensor/types.go internal/sensor/loader.go internal/sensor/grounding.go internal/sensor/*_test.go
git commit -m "feat(sensor): Step union + InputSpec/OutputSpec; fixture grounding via interpolation (#26)"
```

### Task 8: Intrinsic validators for the step union

**Files:**
- Modify: `internal/sensor/validate.go`
- Test: `internal/sensor/validate_test.go`

- [ ] **Step 1: Write failing tests**

Add to `validate_test.go`:

```go
func TestValidateStepShapeRunXorUses(t *testing.T) {
	bad := Sensor{ID: "x", Steps: []Step{{ID: "s"}}} // neither run nor uses
	if err := validateIntrinsic(bad); err == nil {
		t.Fatal("expected error for step with neither run nor uses")
	}
	both := Sensor{ID: "x", Steps: []Step{{ID: "s", Run: "echo", Uses: "p"}}}
	if err := validateIntrinsic(both); err == nil {
		t.Fatal("expected error for step with both run and uses")
	}
}

func TestValidateWithOnlyOnUsesStep(t *testing.T) {
	bad := Sensor{ID: "x", Steps: []Step{{ID: "s", Run: "echo", With: map[string]string{"a": "b"}}}}
	if err := validateIntrinsic(bad); err == nil {
		t.Fatal("expected error: with on a run-step")
	}
}
```

(Remove or adjust any existing test asserting the old array-`Uses` uniqueness — `checkUniqueStepUses` is
deleted in Step 3.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/sensor/ -run 'StepShape|WithOnly' 2>&1 | head -15`
Expected: FAIL — no shape validator yet.

- [ ] **Step 3: Replace `checkUniqueStepUses` with `checkStepShape`**

In `validate.go`, delete `checkUniqueStepUses` and its call; add `checkStepShape` and call it from
`validateIntrinsic`:

```go
func checkStepShape(s Sensor) error {
	var errs []error
	for _, st := range s.Steps {
		hasRun := st.Run != ""
		hasUses := st.Uses != ""
		switch {
		case hasRun && hasUses:
			errs = append(errs, fmt.Errorf("step %q: has both run and uses", st.ID))
		case !hasRun && !hasUses:
			errs = append(errs, fmt.Errorf("step %q: has neither run nor uses", st.ID))
		}
		if hasRun && len(st.With) > 0 {
			errs = append(errs, fmt.Errorf("step %q: with is only valid on a uses-step", st.ID))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
```

In `validateIntrinsic`, replace the `checkUniqueStepUses(s)` block with `checkStepShape(s)`.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/sensor/ -v 2>&1 | tail -20`
Expected: PASS (whole package).

- [ ] **Step 5: Commit**

```bash
git add internal/sensor/validate.go internal/sensor/validate_test.go
git commit -m "feat(sensor): validate step run-xor-uses shape (#26)"
```

### Task 9: Composition checks at store/resolver level

**Files:**
- Create: `internal/sensor/compose.go`
- Test: `internal/sensor/compose_test.go`

- [ ] **Step 1: Write failing tests**

Create `compose_test.go`:

```go
package sensor

import "testing"

func TestValidateCompositionUsesTargetMustBeCore(t *testing.T) {
	prim := Sensor{ID: "e2e-test", Scope: "core", Inputs: map[string]InputSpec{"method": {Required: true, Default: "GET", HasDefault: true}}}
	notCore := Sensor{ID: "decoy", Scope: "use-case", UseCaseID: "uc-x"}
	consumer := Sensor{ID: "c", Scope: "use-case", UseCaseID: "uc-x",
		Steps: []Step{{ID: "s", Uses: "decoy"}}}
	store, _ := NewStore(prim, notCore, consumer)
	if err := ValidateComposition(consumer, store); err == nil {
		t.Fatal("expected error: uses target is not scope=core")
	}
}

func TestValidateCompositionRequiredInputUnbound(t *testing.T) {
	prim := Sensor{ID: "e2e-test", Scope: "core",
		Inputs: map[string]InputSpec{"path": {Required: true}}} // required, no default
	consumer := Sensor{ID: "c", Scope: "use-case", UseCaseID: "uc-x",
		Steps: []Step{{ID: "s", Uses: "e2e-test", With: map[string]string{}}}}
	store, _ := NewStore(prim, consumer)
	if err := ValidateComposition(consumer, store); err == nil {
		t.Fatal("expected input-unbound error")
	}
}

func TestValidateCompositionRequiredInputDefaulted(t *testing.T) {
	prim := Sensor{ID: "e2e-test", Scope: "core",
		Inputs: map[string]InputSpec{"path": {Required: true, Default: "/", HasDefault: true}}}
	consumer := Sensor{ID: "c", Scope: "use-case", UseCaseID: "uc-x",
		Steps: []Step{{ID: "s", Uses: "e2e-test"}}}
	store, _ := NewStore(prim, consumer)
	if err := ValidateComposition(consumer, store); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/sensor/ -run Composition 2>&1 | head -10`
Expected: FAIL — `ValidateComposition` undefined.

- [ ] **Step 3: Implement `ValidateComposition`**

Create `compose.go` (uses the existing `*Store` lookup — confirm the method name in `store.go`; this plan
assumes `store.Lookup(id) (Sensor, bool)`; adjust to the real accessor):

```go
package sensor

import (
	"errors"
	"fmt"

	"github.com/iurykrieger/lastro/internal/enums"
)

// ValidateComposition checks every uses-step in s: the target exists, is
// scope=core, and every required input it declares is satisfied by the
// step's `with` or by an input default. Call after the global Store is built.
func ValidateComposition(s Sensor, store *Store) error {
	var errs []error
	for _, st := range s.Steps {
		if st.Uses == "" {
			continue
		}
		prim, ok := store.Lookup(st.Uses)
		if !ok {
			errs = append(errs, fmt.Errorf("step %q: uses unknown sensor %q", st.ID, st.Uses))
			continue
		}
		if prim.Scope != enums.ScopeCore {
			errs = append(errs, fmt.Errorf("step %q: uses %q which is not scope=core", st.ID, st.Uses))
			continue
		}
		for name, spec := range prim.Inputs {
			if !spec.Required {
				continue
			}
			if _, bound := st.With[name]; bound {
				continue
			}
			if spec.HasDefault {
				continue
			}
			errs = append(errs, fmt.Errorf("step %q: required input %q of %q is unbound", st.ID, name, st.Uses))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
```

> If `store.go` exposes lookup under a different name (e.g. `ByID`), use that. Read `internal/sensor/store.go`
> before writing this file and match the real accessor.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/sensor/ -run Composition -v 2>&1 | tail -15`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sensor/compose.go internal/sensor/compose_test.go
git commit -m "feat(sensor): ValidateComposition (uses-target-is-core + required inputs) (#26)"
```

---

## Phase 4 — Runtime: binder, executor, outputs, composition

### Task 10: Fixture binder collects refs from run/with

**Files:**
- Modify: `internal/runtime/fixturebinder/binder.go`
- Create: `internal/runtime/fixturebinder/collect.go`
- Test: `internal/runtime/fixturebinder/binder_test.go`, `collect_test.go`

- [ ] **Step 1: Write a failing collect test**

Create `collect_test.go`:

```go
package fixturebinder

import (
	"reflect"
	"testing"
)

func TestCollectFixtureRefs(t *testing.T) {
	got, err := CollectFixtureRefs(
		`curl ${{ fixtures.body }} ${{ inputs.method }}`,
		map[string]string{"extra": `${{ fixtures.headers }}`},
	)
	if err != nil { t.Fatal(err) }
	want := []string{"body", "headers"} // sorted, deduped
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/runtime/fixturebinder/ -run Collect 2>&1 | head -10`
Expected: FAIL — `CollectFixtureRefs` undefined.

- [ ] **Step 3: Implement collection**

Create `collect.go`:

```go
package fixturebinder

import (
	"sort"

	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// CollectFixtureRefs parses run plus every with value and returns the sorted,
// deduped set of fixture ids referenced via ${{ fixtures.<id> }}.
func CollectFixtureRefs(run string, with map[string]string) ([]string, error) {
	seen := map[string]bool{}
	add := func(src string) error {
		segs, err := template.Parse(src)
		if err != nil {
			return err
		}
		_, refs, err := template.Compile(segs)
		if err != nil {
			return err
		}
		for _, id := range refs.Fixtures {
			seen[id] = true
		}
		return nil
	}
	if err := add(run); err != nil {
		return nil, err
	}
	for _, v := range with {
		if err := add(v); err != nil {
			return nil, err
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}
```

- [ ] **Step 4: Update `Bind` to take an explicit id list**

In `binder.go`, change `Bind` to accept the collected ids instead of reading `step.Uses`:

```go
// Bind writes each owned fixture's payload to ScratchDir and returns a
// StepBinding. ids is the fixture-id set the step references (from
// CollectFixtureRefs); each must be owned by owningUseCase.
func (b *Binder) Bind(ids []string, owningUseCase *usecase.UseCase, store fixture.FixtureStore) (StepBinding, error) {
	binding := StepBinding{Env: map[string]string{}, Files: map[string]string{}, BoundIDs: []string{}}
	if len(ids) == 0 {
		return binding, nil
	}
	owned := make(map[string]struct{}, len(owningUseCase.FixtureIDs))
	for _, id := range owningUseCase.FixtureIDs {
		owned[id] = struct{}{}
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	for _, id := range sorted {
		// ... body identical to the current loop (ownership check, lookup,
		// write payload, set binding.Env[normalizeEnvName(id)] = path, etc.) ...
	}
	return binding, nil
}
```

Update `binder_test.go`: callers now pass `[]string{...}` instead of a `sensor.Step`.

- [ ] **Step 5: Run to verify pass**

Run: `go test ./internal/runtime/fixturebinder/ -v 2>&1 | tail -20`
Expected: PASS. (Executor will be updated to the new `Bind` signature in Task 11 — `./internal/runtime/executor` may not compile until then; that is expected and fixed next.)

- [ ] **Step 6: Commit**

```bash
git add internal/runtime/fixturebinder/
git commit -m "feat(fixturebinder): collect fixture refs from run/with (#26)"
```

### Task 11: Executor — lift the ban, compile to env, new Bind signature

**Files:**
- Modify: `internal/runtime/executor/step.go`, `internal/runtime/executor/errors.go`
- Test: `internal/runtime/executor/step_test.go`, `run_test.go`

- [ ] **Step 1: Rewrite the fixture-in-run test as the new contract**

In `step_test.go`, replace `Test...ErrTemplateFixtureInRun` with a test that a `${{ fixtures.X }}` ref in
`run` now compiles to an env reference and binds the fixture (no error). Example:

```go
func TestRunStepCompilesFixtureRefToEnv(t *testing.T) {
	// A step whose run references a fixture should succeed, with the fixture
	// bound as HARNESS_FIXTURE_* and the command referencing "${HARNESS_FIXTURE_FX}".
	// (Wire a minimal stepArgs with a one-fixture use case + store; assert no
	// TemplateError and that the resolved argv contains HARNESS_FIXTURE_FX.)
}
```

(Use the existing `run_test.go` harness helpers for constructing `stepArgs`; mirror an existing passing
test's setup. Assert `errors.Is(err, ErrTemplateFixtureInRun)` is **gone**.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/runtime/executor/ 2>&1 | head -20`
Expected: FAIL/compile-error — `Bind` signature changed (Task 10) and the old ban test references a removed symbol.

- [ ] **Step 3: Rewrite `runStep`'s template handling (step.go §1–2)**

Replace the parse-and-reject-fixtures block and the binder call:

```go
	// 1. Parse + compile Step.Run to env-ref form (fixtures/inputs/step-outputs
	//    become "${HARNESS_*}" references; values are never inlined).
	segs, err := template.Parse(a.Step.Run)
	if err != nil {
		return stepOutcome{}, &TemplateError{Step: a.StepIdx, Cause: err}
	}
	resolved, refs, err := template.Compile(segs)
	if err != nil {
		return stepOutcome{}, &TemplateError{Step: a.StepIdx, Cause: err}
	}

	// 2. Bind fixtures referenced by the compiled run.
	binder := &fixturebinder.Binder{ScratchDir: filepath.Join(a.RunDir, "scratch")}
	if err := os.MkdirAll(binder.ScratchDir, 0o700); err != nil {
		return stepOutcome{}, fmt.Errorf("executor: mkdir scratch: %w", err)
	}
	binding, err := binder.Bind(refs.Fixtures, a.UseCase, a.Store)
	if err != nil {
		return stepOutcome{}, err
	}
```

In `step.go` §3 (build environment), add input and step-output env vars (passed in via `stepArgs` —
see Task 12/13 for the fields `InputEnv` and `StepOutEnv`):

```go
	env := os.Environ()
	for k, v := range binding.Env {
		env = append(env, k+"="+v)
	}
	for k, v := range a.InputEnv {
		env = append(env, k+"="+v)
	}
	for k, v := range a.StepOutEnv {
		env = append(env, k+"="+v)
	}
	env = append(env,
		"HARNESS_RUN_DIR="+a.RunDir,
		"HARNESS_SCRATCH_DIR="+binder.ScratchDir,
	)
```

Delete `ErrTemplateFixtureInRun` from `errors.go` and its doc comment. Remove the now-unused
`a.Resolver.Resolve` call (the compiler replaces it; entry-point env values, if any, are supplied via
`InputEnv`/a dedicated entrypoint env builder — out of scope for v1 if no example uses them, but keep the
`EntryPoints` refs collected by `Compile` for a future builder).

Add `InputEnv` and `StepOutEnv` (`map[string]string`) to the `stepArgs` struct in `step.go`.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/runtime/executor/ -v 2>&1 | tail -25`
Expected: PASS (fixture-ref-in-run now binds; ban removed).

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/executor/step.go internal/runtime/executor/errors.go internal/runtime/executor/*_test.go
git commit -m "feat(executor): compile run to env refs; lift fixture-in-run ban (#26)"
```

### Task 12: Step-output capture via `$HARNESS_OUTPUT`

**Files:**
- Modify: `internal/runtime/executor/step.go`, `internal/runtime/executor/executor.go`
- Create: `internal/runtime/executor/stepout.go`
- Test: `internal/runtime/executor/stepout_test.go`

- [ ] **Step 1: Write a failing output-parse test**

Create `stepout_test.go`:

```go
package executor

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseStepOutputFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out")
	os.WriteFile(p, []byte("charge_id=abc123\nstatus=201\ncharge_id=override\n"), 0o600)
	got, err := parseStepOutputFile(p)
	if err != nil { t.Fatal(err) }
	want := map[string]string{"charge_id": "override", "status": "201"} // last write wins
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestStepOutEnvName(t *testing.T) {
	if stepOutEnvName("create", "charge-id") != "HARNESS_STEPOUT_CREATE_CHARGE_ID" {
		t.Fatal(stepOutEnvName("create", "charge-id"))
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/runtime/executor/ -run 'StepOutput|StepOutEnv' 2>&1 | head -10`
Expected: FAIL — undefined helpers.

- [ ] **Step 3: Implement the helpers**

Create `stepout.go`:

```go
package executor

import (
	"bufio"
	"os"
	"strings"
)

// parseStepOutputFile reads name=value lines (last write wins). Lines without
// '=' are ignored. Missing file → empty map, no error.
func parseStepOutputFile(path string) (map[string]string, error) {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		out[line[:i]] = line[i+1:]
	}
	return out, sc.Err()
}

func stepOutEnvName(stepID, name string) string {
	up := func(s string) string { return strings.ToUpper(strings.ReplaceAll(s, "-", "_")) }
	return "HARNESS_STEPOUT_" + up(stepID) + "_" + up(name)
}
```

- [ ] **Step 4: Wire capture into the step lifecycle**

In `step.go`: before spawning, create the output file and set `HARNESS_OUTPUT`:

```go
	outPath := filepath.Join(a.RunDir, "scratch", "stepout-"+a.Step.ID)
	if err := os.WriteFile(outPath, nil, 0o600); err != nil {
		return stepOutcome{}, fmt.Errorf("executor: create stepout: %w", err)
	}
	env = append(env, "HARNESS_OUTPUT="+outPath)
```

After the step exits (before `return stepOutcome{...}`), parse it and add to the outcome:

```go
	stepOut, _ := parseStepOutputFile(outPath)
```

Add `Outputs map[string]string` to `stepOutcome` and set `Outputs: stepOut`.

In `executor.go` `Run`: maintain `stepOutputs := map[string]map[string]string{}` across the loop. Before each
`runStep`, build `StepOutEnv` from prior steps' outputs:

```go
	stepOutEnv := map[string]string{}
	for sid, kv := range stepOutputs {
		for name, val := range kv {
			stepOutEnv[stepOutEnvName(sid, name)] = val
		}
	}
	// pass stepOutEnv in stepArgs.StepOutEnv
	// after runStep:
	stepOutputs[step.ID] = outcome.Outputs
```

- [ ] **Step 5: Run to verify pass**

Run: `go test ./internal/runtime/executor/ -v 2>&1 | tail -25`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/runtime/executor/
git commit -m "feat(executor): capture step outputs via \$HARNESS_OUTPUT (#26)"
```

### Task 13: Composition expansion — inline primitive steps with bound inputs

**Files:**
- Create: `internal/runtime/executor/compose.go`
- Modify: `internal/runtime/executor/executor.go` (`Options.SensorLookup`; expand before the step loop)
- Test: `internal/runtime/executor/compose_test.go`

- [ ] **Step 1: Write a failing expansion test**

Create `compose_test.go`:

```go
package executor

import (
	"testing"

	"github.com/iurykrieger/lastro/internal/sensor"
)

func TestExpandStepsInlinesPrimitive(t *testing.T) {
	prim := sensor.Sensor{
		ID: "e2e-test", Scope: "core",
		Inputs:    map[string]sensor.InputSpec{"method": {Required: true, Default: "GET", HasDefault: true}, "path": {Required: true}},
		DependsOn: []string{"run-dev"},
		Steps:     []sensor.Step{{ID: "request", Run: `curl -X "${{ inputs.method }}" "${{ inputs.path }}"`}},
	}
	consumer := sensor.Sensor{
		ID: "create-charge-e2e", Scope: "use-case", UseCaseID: "uc-x",
		Steps: []sensor.Step{{ID: "create", Uses: "e2e-test", With: map[string]string{"method": "POST", "path": "/v1/charges"}}},
	}
	lookup := func(id string) (sensor.Sensor, bool) {
		if id == "e2e-test" { return prim, true }
		return sensor.Sensor{}, false
	}
	expanded, deps, err := expandSteps(consumer, lookup)
	if err != nil { t.Fatal(err) }
	if len(expanded) != 1 {
		t.Fatalf("want 1 expanded step, got %d", len(expanded))
	}
	// The inlined step carries the primitive's run plus an input-env map.
	if expanded[0].InputEnv["HARNESS_INPUT_METHOD"] != "POST" ||
		expanded[0].InputEnv["HARNESS_INPUT_PATH"] != "/v1/charges" {
		t.Errorf("input env = %#v", expanded[0].InputEnv)
	}
	if len(deps) != 1 || deps[0] != "run-dev" {
		t.Errorf("propagated deps = %v", deps)
	}
}

func TestExpandStepsAppliesDefault(t *testing.T) {
	prim := sensor.Sensor{ID: "p", Scope: "core",
		Inputs: map[string]sensor.InputSpec{"method": {Default: "GET", HasDefault: true}},
		Steps:  []sensor.Step{{ID: "s", Run: `echo "${{ inputs.method }}"`}}}
	consumer := sensor.Sensor{ID: "c", Scope: "use-case", UseCaseID: "uc",
		Steps: []sensor.Step{{ID: "u", Uses: "p"}}}
	lookup := func(id string) (sensor.Sensor, bool) { return prim, id == "p" }
	expanded, _, err := expandSteps(consumer, lookup)
	if err != nil { t.Fatal(err) }
	if expanded[0].InputEnv["HARNESS_INPUT_METHOD"] != "GET" {
		t.Errorf("default not applied: %#v", expanded[0].InputEnv)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/runtime/executor/ -run Expand 2>&1 | head -10`
Expected: FAIL — `expandSteps`, `expandedStep` undefined.

- [ ] **Step 3: Implement expansion**

Create `compose.go`:

```go
package executor

import (
	"fmt"
	"strings"

	"github.com/iurykrieger/lastro/internal/sensor"
)

// expandedStep is a run-step ready to execute: the (possibly inlined) run
// string plus the input env the compiled "${HARNESS_INPUT_*}" refs read.
type expandedStep struct {
	OuterID  string            // the consumer step id (for step-output keying)
	InnerID  string            // the primitive's inner step id (for diagnostics)
	Run      string            // the primitive's run (or the consumer's own run)
	InputEnv map[string]string // HARNESS_INPUT_<NAME> -> value
}

// expandSteps turns a sensor's steps into a flat list of run-steps. A run-step
// passes through unchanged (no InputEnv). A uses-step is replaced by the
// referenced primitive's steps, each carrying the bound input env. The union
// of every composed primitive's depends_on is returned for propagation.
func expandSteps(s sensor.Sensor, lookup func(string) (sensor.Sensor, bool)) ([]expandedStep, []string, error) {
	var out []expandedStep
	depSeen := map[string]bool{}
	var deps []string
	for _, st := range s.Steps {
		if st.Uses == "" {
			out = append(out, expandedStep{OuterID: st.ID, InnerID: st.ID, Run: st.Run})
			continue
		}
		prim, ok := lookup(st.Uses)
		if !ok {
			return nil, nil, fmt.Errorf("executor: step %q uses unknown primitive %q", st.ID, st.Uses)
		}
		inputEnv, err := bindInputs(prim, st.With)
		if err != nil {
			return nil, nil, fmt.Errorf("executor: step %q: %w", st.ID, err)
		}
		for _, dep := range prim.DependsOn {
			if !depSeen[dep] {
				depSeen[dep] = true
				deps = append(deps, dep)
			}
		}
		for _, inner := range prim.Steps {
			out = append(out, expandedStep{OuterID: st.ID, InnerID: inner.ID, Run: inner.Run, InputEnv: inputEnv})
		}
	}
	return out, deps, nil
}

// bindInputs resolves each declared input to its value (with override or
// default) and returns the HARNESS_INPUT_<NAME> env map. Required inputs
// without a binding and without a default are an error.
func bindInputs(prim sensor.Sensor, with map[string]string) (map[string]string, error) {
	env := map[string]string{}
	for name, spec := range prim.Inputs {
		val, bound := with[name]
		switch {
		case bound:
			// use override
		case spec.HasDefault:
			val = spec.Default
		case spec.Required:
			return nil, fmt.Errorf("required input %q unbound", name)
		default:
			continue // optional, no default, no binding → unset
		}
		env["HARNESS_INPUT_"+up(name)] = val
	}
	return env, nil
}

func up(s string) string { return strings.ToUpper(strings.ReplaceAll(s, "-", "_")) }
```

> The `with` values may themselves contain `${{ fixtures.* }}` / `${{ steps.*.outputs.* }}` refs. v1 binding:
> a `with` value that is a pure fixture ref is passed through to the inner run as-is so the inner step's
> `Compile` pass (Task 11) turns it into the env ref and the binder picks it up. For step-output refs in
> `with` (e.g. `path: .../${{ steps.create.outputs.id }}/capture`), the value is interpolated at the
> consuming step's execution time using the accumulated `stepOutEnv` (Task 12) — implement by compiling the
> `with` value and substituting from `stepOutEnv` just before the inner step runs. Keep this in `compose.go`
> as `resolveWithValue(val string, stepOutEnv map[string]string) string` and unit-test it.

- [ ] **Step 4: Wire into `executor.go`**

Add `SensorLookup func(id string) (sensor.Sensor, bool)` to `Options`. At the top of `Run`, before the loop,
call `expandSteps(s, e.opts.SensorLookup)`; propagate `deps` into the sensor's effective `DependsOn` (the
DAG gather already runs upstream in `validate-use-case`; here propagation is informational/logged unless the
executor enforces ordering — for v1, log and proceed). Iterate `expanded` instead of `s.Steps`, passing
`InputEnv` and the accumulated `StepOutEnv` into `stepArgs`, and key `stepOutputs[exp.OuterID]`.

> Wiring `SensorLookup`: `cmd/harness/usecase_runner.go` and `internal/lifecycle` construct the `Executor`.
> They already build a `*sensor.Store`; pass `store.Lookup` (or the real accessor) as `SensorLookup`. Read
> those construction sites and thread the option through. Add a nil-guard: if `SensorLookup` is nil, a
> uses-step errors clearly ("executor: composition requires SensorLookup").

- [ ] **Step 5: Run to verify pass**

Run: `go test ./internal/runtime/executor/ -v 2>&1 | tail -25`
Expected: PASS.

- [ ] **Step 6: Run the whole suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS. Fix any caller that still calls `Bind(step, ...)` or constructs `Options` without
`SensorLookup`.

- [ ] **Step 7: Commit**

```bash
git add internal/runtime/executor/
git commit -m "feat(executor): expand uses-steps into primitive steps with bound inputs (#26)"
```

---

## Phase 5 — Generation skills

### Task 14: `/create-core-sensors` emits parameterized primitives

**Files:**
- Modify: `skills/create-core-sensors/SKILL.md` (and `scripts/` if present — else create), or create the skill dir
- Test: `skills/create-core-sensors/scripts/main_test.go`

- [ ] **Step 1: Confirm the skill exists**

Run: `ls skills/create-core-sensors/ 2>/dev/null || echo MISSING`
The skill is registered (`lastro-harness:create-core-sensors`). If `MISSING`, create
`skills/create-core-sensors/SKILL.md` + `scripts/main.go` mirroring `skills/create-sensors/`.

- [ ] **Step 2: Update the SKILL.md "What to emit" section**

Add the primitive shape rules to `SKILL.md`:

```markdown
## Parameterized primitives (#26)

Angle primitives that take per-use-case inputs (`e2e-test`, `database-query`, `performance`, `logs`,
`metrics`) MUST declare `inputs:` with **defaults for every input** so the primitive self-runs as a smoke,
and `outputs:` for any value a consumer needs downstream. Reference inputs in `run` via
`${{ inputs.<name> }}` and write outputs to `$HARNESS_OUTPUT` as `name=value` lines.

Environment primitives (`run-dev`, `datastore`) take no inputs.

Example primitive:

​```yaml
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [curl]
depends_on: [run-dev]
inputs:
  method: { required: true, default: GET }
  path:   { required: true, default: /health_check/ready }
outputs:
  body: { from: "${{ steps.request.outputs.body }}" }
steps:
  - id: request
    run: |
      resp=$(curl --fail -sS -X "${{ inputs.method }}" "http://localhost:8080${{ inputs.path }}")
      printf 'body=%s\n' "$resp" >> "$HARNESS_OUTPUT"
​```
```

- [ ] **Step 3: Add a script test asserting a primitive validates**

In `scripts/main_test.go`, add a case writing the example primitive above to a temp `.harness/sensors/core/`
and asserting the script's validation (which calls `sensor.LoadSensorBytes`) exits 0. Mirror the existing
`create-sensors` `main_test.go` structure.

- [ ] **Step 4: Run**

Run: `go test ./skills/create-core-sensors/... 2>&1 | tail -15`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add skills/create-core-sensors/
git commit -m "feat(create-core-sensors): emit parameterized primitives (#26)"
```

### Task 15: `/create-sensors` emits `uses`/`with` consumers

**Files:**
- Modify: `skills/create-sensors/SKILL.md`, `skills/create-sensors/scripts/main.go` (validation), `scripts/main_test.go`

- [ ] **Step 1: Update the SKILL.md step rules**

Replace the step bullet list in `SKILL.md` "What to emit" with:

```markdown
- `steps:` — each step is EITHER a run-step or a uses-step:
  - run-step: `{ id, run }`. Reference fixtures via `${{ fixtures.<id> }}` and prior step outputs via
    `${{ steps.<id>.outputs.<name> }}` inside `run`.
  - uses-step: `{ id, uses: <core-primitive-id>, with: { <input>: <value> } }`. Bind every `required`
    input of the primitive. A `with` value may be a fixture ref (`${{ fixtures.<id> }}`) or a prior step
    output (`${{ steps.<id>.outputs.<name> }}`).
- Do NOT put fixture ids in step `uses:` — `uses:` now names a core primitive to compose.
```

Update the `id` convention note (drop the old `s-<uc>-<angle>` if the project moved to slug ids — match
decision #9 of the core-sensors spec; embed `<use-case-id>` for global uniqueness).

- [ ] **Step 2: Update the script validation**

The script (`scripts/main.go`) currently loads + validates a single sensor. After `LoadSensorBytes`, also run
`sensor.ValidateComposition` against a `*sensor.Store` built from `.harness/sensors/core/` so a consumer that
binds a missing required input or targets a non-core id fails at generation. Add the store load:

```go
core, err := sensor.LoadDirectory(filepath.Join(harnessDir, "sensors", "core"))
// ... if core exists, NewStore-merge with the just-loaded consumer and call ValidateComposition ...
```

(If `.harness/sensors/core/` is absent, warn and skip composition validation — the consumer may be generated
before core primitives, per the core-sensors spec §7.)

- [ ] **Step 3: Add a script test**

In `scripts/main_test.go`, add a case: a consumer with a `uses`-step binding a required input passes; one
leaving it unbound fails with a non-zero exit. Provide a temp `core/e2e-test.yaml` fixture.

- [ ] **Step 4: Run**

Run: `go test ./skills/create-sensors/... 2>&1 | tail -15`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add skills/create-sensors/
git commit -m "feat(create-sensors): emit uses/with consumers + composition validation (#26)"
```

---

## Phase 6 — Migrate dogfood sensors + end-to-end

### Task 16: Migrate the on-disk dogfood sensors

**Files:**
- Modify/move: `.harness/sensors/uc-harness-validate-use-case-build.yaml`,
  `.harness/sensors/uc-harness-validate-use-case-unit-test.yaml`
- Test: add `internal/sensor/loader_test.go` case loading the migrated tree

- [ ] **Step 1: Rewrite the two sensors to the new shape + folder**

The current sensors use `uses: [sample-harness-tree]` (array, fixtures). Rewrite the step to reference the
fixture by interpolation and move into the per-use-case folder. New
`.harness/sensors/uc-harness-validate-use-case/build.yaml`:

```yaml
schema_version: 1.0.0
id: uc-harness-validate-use-case-build
use_case_id: uc-harness-validate-use-case
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses:
  - go-stdlib
  - cobra
steps:
  - id: emit-pass
    run: |
      # ${{ fixtures.sample-harness-tree }} is bound as $HARNESS_FIXTURE_SAMPLE_HARNESS_TREE (a file path)
      test -n "${{ fixtures.sample-harness-tree }}"
      printf '%s\n' '{"schema_version":"1.0.0","sensor_id":"uc-harness-validate-use-case-build","use_case_id":"uc-harness-validate-use-case","angle":"build","emitted_at":"2026-05-27T00:00:00Z","verdict":"pass","confidence":1.0,"evidence":{"expected":"harness validate compiles","actual":"harness validate compiles"}}'
      sleep 0.1
```

Apply the analogous rewrite to the unit-test sensor as
`.harness/sensors/uc-harness-validate-use-case/unit-test.yaml`. Delete the two old flat files.

- [ ] **Step 2: Add a loader test for the migrated tree**

In `loader_test.go`, add a case asserting `LoadDirectory(".harness/sensors")` (or the test's fixture tree)
loads both migrated sensors and that their single step is a run-step referencing the fixture.

- [ ] **Step 3: Run**

Run: `go test ./internal/sensor/ ./internal/lifecycle/ 2>&1 | tail -20`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add .harness/sensors/
git commit -m "refactor(dogfood): migrate sensors to uses-scalar + fixture interpolation (#26)"
```

### Task 17: End-to-end dogfood

**Files:** none (verification task)

- [ ] **Step 1: Build the CLI**

Run: `go build ./cmd/harness 2>&1 | tail -5`
Expected: clean build.

- [ ] **Step 2: Run the dogfood validation**

Run: `go run ./cmd/harness validate --use-case uc-harness-validate-use-case --repo-root . 2>&1 | tail -30`
Expected: the use case validates; the build + unit-test sensors emit `pass` AggregateSignals; exit 0.

- [ ] **Step 3: Full suite + vet**

Run: `go test ./... 2>&1 | tail -15 && go vet ./... 2>&1 | tail -10`
Expected: all PASS, no vet findings.

- [ ] **Step 4: Final commit (if any test-data tweaks were needed)**

```bash
git add -A
git commit -m "test(harness): green dogfood for parameterized sensors (#26)"
```

---

## Self-Review notes (for the executor)

- **Read before writing, two spots:** `internal/sensor/store.go` (the real lookup accessor for Tasks 9/13)
  and `cmd/harness/usecase_runner.go` + `internal/lifecycle` (the `Executor` construction sites for the
  `SensorLookup` wiring in Task 13). The plan names the seams; confirm exact signatures there.
- **Risk hotspots:** Task 11 (lifting the ban changes a tested invariant — keep the new contract test),
  Task 12 (output capture interacts with the concurrent stdout/stderr pumps — capture after `cmd.Wait`),
  Task 13 (`SensorLookup` wiring touches multiple construction sites; nil-guard it).
- **DAG propagation (Task 13):** v1 propagates a composed primitive's `depends_on` informationally; the
  authoritative gather/order already runs in `validate-use-case` per the core-sensors spec §8. Do not
  duplicate scheduling in the executor.
