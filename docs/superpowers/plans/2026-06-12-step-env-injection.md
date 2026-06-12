# Declarative Step Env-Var Injection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sensors declare env-var injection (`env:` on steps, `env_file:` in the stack manifest, `${{ env.NAME }}` namespace, declared requirements on core primitives) so secret-needing recipes run reproducibly without manual `set -a; . ./.env` — with pre-spawn `missing_env` diagnostics and secret redaction (issue #49).

**Spec:** `docs/superpowers/specs/2026-06-12-step-env-injection-design.md` — read it first.

**Architecture:** A Go-side env resolver inside `internal/runtime/executor`: `loadEnvView` loads the manifest-declared dotenv file once per run and merges it under the host environment (host wins); `resolveStepEnv` resolves per-step `env:` expressions at spawn time; pre-spawn checks emit a typed `missing-env` signal (verdict `inconclusive`) instead of injecting empty strings; a `redactor` masks every injected value in raw.log/signals.jsonl at the pump choke point.

**Tech Stack:** Go 1.24, `github.com/joho/godotenv` (new dep), existing `template`/`executor`/`sensor`/`stack` packages.

**⚠️ Dirty working tree:** `lib/skillruntime/boot.go`, `bin/linux-amd64/lastro`, and untracked `harness` carry pre-existing uncommitted changes. NEVER `git add -A`. Stage only the files each task names. Before Task 11 (which edits `boot.go`), run `git diff lib/skillruntime/boot.go` — if pre-existing hunks unrelated to this plan exist, surface them to the user before committing that file.

**Spec deviation (intentional):** The spec puts concrete var names (e.g. `NEXTAUTH_SECRET`) in `schemas/core-inputs/*.yaml`. Those files are embedded, stack-agnostic generation floors — concrete names would be wrong for non-NextAuth stacks. Instead: core-inputs gain an `env_guidance` string telling the generator what to declare; the **generated** sensor carries the concrete, runtime-enforced `env:` block (sensor-level declaration added in Task 3). The fail-fast acceptance behavior is unchanged.

---

### Task 1: Add godotenv dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Fetch the dependency**

```bash
cd /home/iury/Workspace/iurykrieger/lastro && go get github.com/joho/godotenv@v1.5.1
```

- [ ] **Step 2: Verify the module builds**

Run: `go build ./...`
Expected: clean exit 0.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "build: add godotenv for manifest-declared dotenv loading

- Enable sensors to read the same .env the application loads
- Mature, widely-used parser instead of a bespoke dotenv reader"
```

---

### Task 2: `env` template namespace (`${{ env.NAME }}`)

**Files:**
- Modify: `internal/usecase/template/grammar.go`
- Modify: `internal/usecase/template/parser.go`
- Modify: `internal/usecase/template/compile.go`
- Modify: `internal/runtime/executor/compose.go` (resolveWithValue arm)
- Test: `internal/usecase/template/parser_test.go`, `internal/usecase/template/compile_test.go`

- [ ] **Step 1: Write the failing parser tests**

Append to `internal/usecase/template/parser_test.go` (match the file's existing table/assert style):

```go
func TestParse_EnvRef(t *testing.T) {
	segs, err := Parse("${{ env.NEXTAUTH_SECRET }}")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(segs) != 1 {
		t.Fatalf("segments = %d, want 1", len(segs))
	}
	ref, ok := segs[0].(EnvRef)
	if !ok {
		t.Fatalf("segment type = %T, want EnvRef", segs[0])
	}
	if ref.Name != "NEXTAUTH_SECRET" {
		t.Errorf("name = %q, want NEXTAUTH_SECRET", ref.Name)
	}
}

func TestParse_EnvRefRejectsBadNames(t *testing.T) {
	for _, in := range []string{
		"${{ env.lower }}",       // lowercase not an env var name
		"${{ env.NAME.more }}",   // no further keys
		"${{ env. }}",            // empty name
	} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q): expected error, got nil", in)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/usecase/template/ -run TestParse_Env -v`
Expected: FAIL — `unknown namespace: env` / undefined `EnvRef`.

- [ ] **Step 3: Implement the grammar node**

Append to `internal/usecase/template/grammar.go` (and add `envRef := "env" "." ENVNAME` plus `ENVNAME := [A-Z_][A-Z0-9_]{0,127}` to the package doc-comment grammar recap):

```go
// EnvRef is `${{ env.<NAME> }}` — an ambient environment variable resolved
// from the harness host environment merged with the manifest-declared
// env_file (host wins). Compiles to a quoted "${NAME}" shell lookup; the
// executor injects the merged view into the child env and refuses to
// spawn when a referenced name is unset or empty (missing_env).
type EnvRef struct {
	Name string
	Pos  Position
}

func (EnvRef) isSegment() {}
```

- [ ] **Step 4: Implement the parser arm**

In `internal/usecase/template/parser.go`, add to the `switch ns` in `parseRef` (after `case "steps":`):

```go
	case "env":
		return p.parseEnvTail(nsStart)
```

Append the tail parser and token reader:

```go
// parseEnvTail reads `${{ env.<NAME> }}` where NAME is a POSIX-style
// exported variable name: ^[A-Z_][A-Z0-9_]{0,127}$.
func (p *parser) parseEnvTail(refPos Position) (Segment, error) {
	name, ok := p.readEnvVarName()
	if !ok {
		return nil, &ParseError{Pos: p.here(), Msg: "expected env var name (UPPER_SNAKE_CASE)"}
	}
	if p.peekByte('.') {
		return nil, &ParseError{Pos: p.here(), Msg: "env.<NAME> takes no further keys"}
	}
	if err := p.expectClose(); err != nil {
		return nil, err
	}
	return EnvRef{Name: name, Pos: refPos}, nil
}

// readEnvVarName reads ENVNAME matching ^[A-Z_][A-Z0-9_]{0,127}$.
func (p *parser) readEnvVarName() (string, bool) {
	if p.pos >= len(p.input) {
		return "", false
	}
	c := p.input[p.pos]
	if !((c >= 'A' && c <= 'Z') || c == '_') {
		return "", false
	}
	start := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			p.advance(1)
			continue
		}
		break
	}
	name := p.input[start:p.pos]
	if len(name) > 128 {
		return "", false
	}
	return name, true
}
```

- [ ] **Step 5: Write the failing compile test**

Append to `internal/usecase/template/compile_test.go`:

```go
func TestCompile_EnvRef(t *testing.T) {
	segs, err := Parse(`echo "t=${{ env.MY_TOKEN }}" "${{ env.MY_TOKEN }}"`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, refs, err := Compile(segs)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	want := `echo "t="${MY_TOKEN}"" ""${MY_TOKEN}""`
	if out != want {
		t.Errorf("compiled = %q, want %q", out, want)
	}
	if len(refs.Env) != 1 || refs.Env[0] != "MY_TOKEN" {
		t.Errorf("refs.Env = %v, want [MY_TOKEN] (deduped)", refs.Env)
	}
}
```

- [ ] **Step 6: Run to verify it fails, then implement Compile**

Run: `go test ./internal/usecase/template/ -run TestCompile_EnvRef -v` → FAIL (`refs.Env` undefined).

In `internal/usecase/template/compile.go`: add `Env []string // env var names, first-seen order, deduped` to `Refs`, and a case in the `Compile` switch (before `case EntryPointRef:`):

```go
		case EnvRef:
			b.WriteString(`"${` + v.Name + `}"`)
			if !seen["e:"+v.Name] {
				seen["e:"+v.Name] = true
				refs.Env = append(refs.Env, v.Name)
			}
```

Also update the doc comment's env-name list to mention the plain `"${NAME}"` form for env refs.

- [ ] **Step 7: Guard the other Segment type-switches**

New segment types must not fall into silent/confusing `default` arms. Find every switch:

Run: `grep -rn "template.EntryPointRef\|case EntryPointRef" internal/ lib/ cmd/ --include="*.go" | grep -v _test`

For each site that switches on `Segment` outside the template package, add an explicit `EnvRef` arm. Known site — `internal/runtime/executor/compose.go` `resolveWithValue` (add before the `case template.InputRef:` arm):

```go
		case template.EnvRef:
			return "", fmt.Errorf("env.* is not valid inside a with-value; declare it under the step's env: map instead")
```

If the grep surfaces switches in `internal/usecase/template/resolver.go` or use-case validation, add an arm there erroring with `"env.* is not valid in use-case text"` (use-case text stays tech-agnostic). Sites whose `default` already returns a typed error may keep the default, but prefer the explicit message.

- [ ] **Step 8: Run the package tests**

Run: `go test ./internal/usecase/template/ ./internal/runtime/executor/ ./internal/usecase/...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/usecase/template/ internal/runtime/executor/compose.go
git commit -m "feat(#49): add env.* template namespace for ambient variables

- Let run scripts reference host/.env configuration as \${{ env.NAME }}
- Compile to a safely-quoted shell lookup, consistent with every namespace
- Collected refs feed the executor's pre-spawn missing_env check"
```

---

### Task 3: Sensor schema + Go types (`env:` on steps and sensors)

**Files:**
- Modify: `schemas/sensor.yaml`
- Modify: `internal/sensor/types.go`
- Modify: `internal/sensor/baseline.go` (`ValidateInputReferences` env scan)
- Modify: `schemas/examples/sensor/core-provision-auth.yaml`, `schemas/examples/sensor/uc-authenticated-consumer.yaml`
- Test: `internal/sensor/loader_test.go`, `internal/sensor/baseline_test.go`

- [ ] **Step 1: Write the failing loader test**

Append to `internal/sensor/loader_test.go` (use the file's existing `LoadSensorBytes` style):

```go
func TestLoadSensor_EnvDeclarations(t *testing.T) {
	raw := []byte(`
schema_version: 1.0.0
id: env-demo
scope: core
angle: environment
kind: assertion
nature: computational
output_type: single-shot
uses: [node]
env:
  NEXTAUTH_SECRET:
    description: "session signing secret"
  OPTIONAL_FLAG:
    required: false
steps:
  - id: mint
    run: echo ok
    env:
      DATABASE_URL: ${{ env.DATABASE_URL }}
      NODE_ENV: test
`)
	s, err := LoadSensorBytes(raw)
	if err != nil {
		t.Fatalf("LoadSensorBytes: %v", err)
	}
	if !s.Env["NEXTAUTH_SECRET"].IsRequired() {
		t.Error("NEXTAUTH_SECRET should default to required")
	}
	if s.Env["OPTIONAL_FLAG"].IsRequired() {
		t.Error("OPTIONAL_FLAG declared required: false")
	}
	if got := s.Steps[0].Env["NODE_ENV"]; got != "test" {
		t.Errorf("step env NODE_ENV = %q, want test", got)
	}
}

func TestLoadSensor_EnvRejectsBadNames(t *testing.T) {
	raw := []byte(`
schema_version: 1.0.0
id: env-bad
scope: core
angle: environment
kind: assertion
nature: computational
output_type: single-shot
uses: [node]
steps:
  - id: only
    run: echo ok
    env:
      lower_case: nope
`)
	if _, err := LoadSensorBytes(raw); err == nil {
		t.Fatal("expected schema rejection of lowercase env name")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/sensor/ -run TestLoadSensor_Env -v`
Expected: FAIL (unknown field `env` → schema `additionalProperties: false` violation, and `IsRequired` undefined).

- [ ] **Step 3: Extend `schemas/sensor.yaml`**

Add a sensor-level property (after `outputs:`):

```yaml
  env:
    type: object
    description: |
      Ambient environment variables this sensor's recipes read from the
      process environment (secrets, connection strings — not declared
      inputs). Before spawning any step the runtime verifies every
      required name is present and non-empty in the merged host+env_file
      view and emits a typed missing-env signal (verdict inconclusive)
      otherwise. `required` defaults to true.
    propertyNames: { pattern: "^[A-Z_][A-Z0-9_]*$" }
    additionalProperties:
      type: object
      additionalProperties: false
      properties:
        required:    { type: boolean, default: true }
        description: { type: string }
```

Add a step-level property inside `$defs.SensorStep.properties` (after `with:`):

```yaml
      env:
        type: object
        description: |
          Environment variables injected into this step's process (and, on
          a uses-step, into every inner step of the expansion). Values
          support the full ${{ }} surface; values resolved from refs are
          redacted in raw.log/signals.jsonl, inline literals are not.
        propertyNames: { pattern: "^[A-Z_][A-Z0-9_]*$" }
        additionalProperties: { type: string }
```

`env:` is valid on both run- and uses-steps — the existing `oneOf` only constrains `with:`, no change needed there.

- [ ] **Step 4: Extend the Go types**

In `internal/sensor/types.go`, add to `Sensor` (after `Outputs`):

```go
	// Env declares ambient environment variables the sensor's recipes read
	// (secrets, connection strings). The runtime enforces required entries
	// pre-spawn; on a composed primitive a consumer step's env: injection
	// also satisfies the requirement.
	Env map[string]EnvSpec `json:"env,omitempty"`
```

Add to `Step` (after `With`):

```go
	// Env is the per-step environment injection map (issue #49). Values
	// support ${{ }} interpolation and are resolved by the executor at
	// spawn time, never inlined into script text.
	Env map[string]string `json:"env,omitempty"`
```

Append the spec type:

```go
// EnvSpec declares one ambient environment variable a sensor's recipes
// read. Required is a pointer so the zero value distinguishes "unset"
// (defaults to true) from an explicit false.
type EnvSpec struct {
	Required    *bool  `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

// IsRequired reports whether the variable must be present and non-empty
// in the merged host+env_file view before any step spawns.
func (e EnvSpec) IsRequired() bool { return e.Required == nil || *e.Required }
```

- [ ] **Step 5: Run the loader tests**

Run: `go test ./internal/sensor/ -run TestLoadSensor_Env -v`
Expected: PASS.

- [ ] **Step 6: Failing test — inputs referenced only in step env count as referenced**

Append to `internal/sensor/baseline_test.go`:

```go
func TestValidateInputReferences_EnvValueCounts(t *testing.T) {
	s := Sensor{
		Scope:  enums.ScopeCore,
		Inputs: map[string]InputSpec{"persona": {Default: "default", HasDefault: true}},
		Steps: []Step{{
			ID:  "mint",
			Run: "echo ok",
			Env: map[string]string{"PERSONA": "${{ inputs.persona }}"},
		}},
	}
	if err := ValidateInputReferences(s); err != nil {
		t.Errorf("input referenced in step env should count: %v", err)
	}
}
```

Run: `go test ./internal/sensor/ -run TestValidateInputReferences_EnvValueCounts -v` → FAIL.

Fix in `internal/sensor/baseline.go` `ValidateInputReferences` — extend the blob loop:

```go
	for _, st := range s.Steps {
		blob.WriteString(st.Run)
		blob.WriteByte('\n')
		for _, v := range st.With {
			blob.WriteString(v)
			blob.WriteByte('\n')
		}
		for _, v := range st.Env {
			blob.WriteString(v)
			blob.WriteByte('\n')
		}
	}
```

Run again → PASS.

- [ ] **Step 7: Update the golden examples**

`schemas/examples/sensor/core-provision-auth.yaml` — add after the `outputs:` block:

```yaml
env:
  NEXTAUTH_SECRET:
    description: "Session-signing secret the mint recipe encodes JWTs with; must equal the value the dev server loaded so encode/decode agree"
```

`schemas/examples/sensor/uc-authenticated-consumer.yaml` — Read the file; on the step with `uses: provision-auth`, add (aligned with that step's `with:`):

```yaml
    env:
      NEXTAUTH_SECRET: ${{ env.NEXTAUTH_SECRET }}
```

- [ ] **Step 8: Run sensor + schemas tests**

Run: `go test ./internal/sensor/ ./schemas/...`
Expected: PASS (the schemas package validates golden examples against the schema, so this proves the new fields validate).

- [ ] **Step 9: Commit**

```bash
git add schemas/sensor.yaml schemas/examples/sensor/ internal/sensor/
git commit -m "feat(#49): declarative env on sensors and steps

- Steps declare the variables their process needs alongside uses:/with:
- Core primitives declare required ambient vars so consumers fail fast
- Inputs bound only through step env still satisfy the faithfulness check"
```

---

### Task 4: `env_file:` in the stack manifest + /detect-stack guidance

**Files:**
- Modify: `schemas/stack-manifest.yaml`
- Modify: `internal/stack/types.go`
- Modify: `schemas/examples/stack-manifest/http-api.yaml`
- Modify: `skills/detect-stack/SKILL.md`
- Test: `internal/stack/stack_test.go`

- [ ] **Step 1: Write the failing round-trip test**

Append to `internal/stack/stack_test.go` (reuse a valid manifest literal from an existing test in that file, adding the `env_file` line):

```go
func TestLoadBytes_EnvFileRoundTrips(t *testing.T) {
	// Take any valid manifest YAML already used in this file and add:
	//   env_file: .env.local
	// then assert:
	m, err := LoadBytes(validManifestWithEnvFile) // build inline as in sibling tests
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if m.EnvFile != ".env.local" {
		t.Errorf("EnvFile = %q, want .env.local", m.EnvFile)
	}
}
```

(Adapt the literal construction to the file's existing pattern — there are valid-manifest constants/helpers in `stack_test.go`; copy one and insert `env_file: .env.local`.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/stack/ -run TestLoadBytes_EnvFile -v`
Expected: FAIL (schema rejects unknown property / `EnvFile` undefined).

- [ ] **Step 3: Implement schema + type**

`schemas/stack-manifest.yaml` — add to `properties:` (after `archetype:`):

```yaml
  env_file:
    type: string
    minLength: 1
    description: |
      Optional project-root-relative path to the dotenv file the
      application itself loads (e.g. ".env", ".env.local"). The runtime
      loads it for every sensor step, host environment winning on
      conflicts, so recipes see the same configuration the app does.
```

`internal/stack/types.go` — add to `StackManifest` (after `Archetype`):

```go
	// EnvFile is the project-root-relative dotenv path the application
	// loads (optional). Recorded by /detect-stack; the runtime injects its
	// values into every step's process env, host environment winning.
	EnvFile string `json:"env_file,omitempty" yaml:"env_file,omitempty"`
```

`schemas/examples/stack-manifest/http-api.yaml` — add `env_file: .env` after the `archetype:` line.

- [ ] **Step 4: Run stack + schemas tests**

Run: `go test ./internal/stack/ ./schemas/...`
Expected: PASS (Persist round-trips the field automatically via marshal).

- [ ] **Step 5: Update the detect-stack skill**

In `skills/detect-stack/SKILL.md`, under "## What to emit", add a bullet after the `components` bullet:

```markdown
- `env_file` (optional) — when the repo has a dotenv file the app itself
  loads (`.env`, `.env.local`), record its project-root-relative path
  (prefer `.env.local` when both exist, matching Next.js precedence). The
  runtime injects these values into every sensor step's process, with the
  host environment winning on conflicts — this is what lets secret-reading
  recipes run without a manual `set -a; . ./.env`.
```

Verify the skill stays within budget: `wc -l skills/detect-stack/SKILL.md` → must be ≤ 200 (it is ~61 + 6).

- [ ] **Step 6: Commit**

```bash
git add schemas/stack-manifest.yaml schemas/examples/stack-manifest/ internal/stack/ skills/detect-stack/SKILL.md
git commit -m "feat(#49): record the app's dotenv file in the stack manifest

- /detect-stack captures env_file so runs reproduce the app's own config
- Optional field; existing manifests stay valid"
```

---

### Task 5: Redactor

**Files:**
- Create: `internal/runtime/executor/redact.go`
- Test: `internal/runtime/executor/redact_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/runtime/executor/redact_test.go`:

```go
package executor

import (
	"bytes"
	"testing"
)

func TestRedactor_MasksRegisteredValues(t *testing.T) {
	r := &redactor{}
	r.Add("supersecret")
	got := r.Apply([]byte(`token=supersecret rest`))
	want := []byte(`token=*** rest`)
	if !bytes.Equal(got, want) {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}

func TestRedactor_SkipsShortValues(t *testing.T) {
	r := &redactor{}
	r.Add("dev") // < minRedactLen: masking "dev" would corrupt unrelated output
	got := r.Apply([]byte("development mode"))
	if !bytes.Equal(got, []byte("development mode")) {
		t.Errorf("short value was masked: %q", got)
	}
}

func TestRedactor_LongestFirstAndMultiOccurrence(t *testing.T) {
	r := &redactor{}
	r.Add("secret")
	r.Add("secret-extended") // must mask before its prefix
	got := r.Apply([]byte("a=secret-extended b=secret c=secret"))
	want := []byte("a=*** b=*** c=***")
	if !bytes.Equal(got, want) {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}

func TestRedactor_NilSafe(t *testing.T) {
	var r *redactor
	r.Add("whatever")
	if got := r.Apply([]byte("x")); !bytes.Equal(got, []byte("x")) {
		t.Errorf("nil redactor mutated input: %q", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/runtime/executor/ -run TestRedactor -v`
Expected: FAIL — `redactor` undefined.

- [ ] **Step 3: Implement**

Create `internal/runtime/executor/redact.go`:

```go
package executor

import (
	"bytes"
	"sort"
	"sync"
)

// minRedactLen guards against registering trivial values ("1", "dev")
// whose masking would corrupt unrelated output.
const minRedactLen = 4

// redactedPlaceholder replaces each registered value occurrence.
const redactedPlaceholder = "***"

// redactor masks registered secret values in every byte stream the run
// persists (raw.log, signals.jsonl) and in the pump choke point before
// lines reach signal matching — so matched_line evidence and aggregate
// heal hints never carry a secret. Values are matched exactly (no
// encodings); registration sources are the env_file ambient view and
// ref-derived step env: values. All methods are nil-receiver-safe and
// goroutine-safe (stdout/stderr pumps write concurrently).
type redactor struct {
	mu     sync.RWMutex
	values []string // longest-first so an extended secret masks before its prefix
}

// Add registers one secret value. Short values and duplicates are ignored.
func (r *redactor) Add(v string) {
	if r == nil || len(v) < minRedactLen {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.values {
		if existing == v {
			return
		}
	}
	r.values = append(r.values, v)
	sort.Slice(r.values, func(i, j int) bool { return len(r.values[i]) > len(r.values[j]) })
}

// Apply returns b with every registered value replaced by the placeholder.
func (r *redactor) Apply(b []byte) []byte {
	if r == nil {
		return b
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, v := range r.values {
		b = bytes.ReplaceAll(b, []byte(v), []byte(redactedPlaceholder))
	}
	return b
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/runtime/executor/ -run TestRedactor -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/executor/redact.go internal/runtime/executor/redact_test.go
git commit -m "feat(#49): exact-match redactor for injected secret values

- Mask every value the env feature injects before it reaches any sink
- Longest-first ordering so overlapping secrets mask fully
- Length floor prevents trivial strings from corrupting output"
```

---

### Task 6: envView — dotenv loading with host-wins precedence

**Files:**
- Create: `internal/runtime/executor/envview.go`
- Test: `internal/runtime/executor/envview_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/runtime/executor/envview_test.go`:

```go
package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadEnvView_FileFillsGaps(t *testing.T) {
	p := writeEnvFile(t, "FROM_FILE_ONLY=filevalue\n")
	v, missing, err := loadEnvView(p)
	if err != nil || missing {
		t.Fatalf("loadEnvView: missing=%v err=%v", missing, err)
	}
	if got, ok := v.lookup("FROM_FILE_ONLY"); !ok || got != "filevalue" {
		t.Errorf("lookup = %q,%v; want filevalue,true", got, ok)
	}
	if v.source != p {
		t.Errorf("source = %q, want %q", v.source, p)
	}
}

func TestLoadEnvView_HostWins(t *testing.T) {
	t.Setenv("ENVVIEW_CLASH", "fromhost")
	p := writeEnvFile(t, "ENVVIEW_CLASH=fromfile\n")
	v, _, err := loadEnvView(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, shadowed := v.ambient["ENVVIEW_CLASH"]; shadowed {
		t.Error("host-set var must not enter the ambient map")
	}
	if got, _ := v.lookup("ENVVIEW_CLASH"); got != "fromhost" {
		t.Errorf("lookup = %q, want fromhost", got)
	}
}

func TestLoadEnvView_AbsentFileDegrades(t *testing.T) {
	v, missing, err := loadEnvView(filepath.Join(t.TempDir(), "no-such.env"))
	if err != nil {
		t.Fatalf("absent file must not error: %v", err)
	}
	if !missing {
		t.Error("missing flag should be true")
	}
	if _, ok := v.lookup("ANYTHING_AT_ALL_XYZ"); ok {
		t.Error("empty view resolved a name")
	}
}

func TestLoadEnvView_NoPathDeclared(t *testing.T) {
	v, missing, err := loadEnvView("")
	if err != nil || missing {
		t.Fatalf("empty path: missing=%v err=%v", missing, err)
	}
	if v.source != "" {
		t.Errorf("source = %q, want empty", v.source)
	}
}

func TestLoadEnvView_UnparseableErrors(t *testing.T) {
	p := writeEnvFile(t, "not a dotenv line at all\n\"unterminated")
	if _, _, err := loadEnvView(p); err == nil {
		t.Error("unparseable env_file must error")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/runtime/executor/ -run TestLoadEnvView -v`
Expected: FAIL — `loadEnvView` undefined.

- [ ] **Step 3: Implement**

Create `internal/runtime/executor/envview.go`:

```go
package executor

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// envView is the merged ambient environment a sensor run exposes to its
// steps: the manifest-declared env_file's values with the harness host
// environment winning on conflict (standard dotenv semantics — the file
// fills gaps, CI stays in control). The zero value behaves as "no
// env_file declared".
type envView struct {
	ambient map[string]string // env_file values NOT shadowed by the host env
	source  string            // env_file path for diagnostics; "" when none declared
}

// loadEnvView loads the dotenv file at path. path=="" means no env_file
// is declared. A declared-but-absent file degrades to an empty view with
// fileMissing=true (the requirement checks still catch real gaps); an
// unparseable file is an error the run surfaces as an inconclusive
// env-file-invalid signal.
func loadEnvView(path string) (view envView, fileMissing bool, err error) {
	view = envView{ambient: map[string]string{}}
	if path == "" {
		return view, false, nil
	}
	view.source = path
	raw, err := godotenv.Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return view, true, nil
		}
		return envView{}, false, fmt.Errorf("executor: parse env_file %s: %w", path, err)
	}
	for k, v := range raw {
		if _, shadowed := os.LookupEnv(k); shadowed {
			continue
		}
		view.ambient[k] = v
	}
	return view, false, nil
}

// lookup resolves name from the host environment first, then the ambient
// env_file values.
func (v envView) lookup(name string) (string, bool) {
	if val, ok := os.LookupEnv(name); ok {
		return val, true
	}
	val, ok := v.ambient[name]
	return val, ok
}
```

If `TestLoadEnvView_UnparseableErrors` fails because godotenv tolerates that content, change the test content to a value godotenv rejects (check its docs/tests — e.g. `export\n`); the behavior under test is "parse error propagates", not a specific syntax.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/runtime/executor/ -run TestLoadEnvView -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/executor/envview.go internal/runtime/executor/envview_test.go
git commit -m "feat(#49): merged host+env_file ambient view

- Steps see the same configuration the application loads from .env
- Host environment wins so CI keeps control of every variable
- Absent file degrades gracefully; unparseable file is a typed error"
```

---

### Task 7: Wire the redactor into raw.log, signals.jsonl, and the pumps

**Files:**
- Modify: `internal/runtime/executor/rawlog.go`
- Modify: `internal/runtime/executor/signals.go`
- Modify: `internal/runtime/executor/step.go` (stepArgs + pump calls)
- Modify: `internal/runtime/executor/compose.go` (topStepArgs + pass-through)
- Modify: `internal/runtime/executor/executor.go` (Run creates the redactor)
- Test: `internal/runtime/executor/rawlog_test.go`

- [ ] **Step 1: Write the failing rawlog test**

Append to `internal/runtime/executor/rawlog_test.go` (mirror the construction pattern of the existing tests in that file):

```go
func TestRawLog_RedactsRegisteredValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raw.log")
	rl, err := newRawLog(path, fixedNow) // reuse the file's existing fixed clock helper name
	if err != nil {
		t.Fatal(err)
	}
	red := &redactor{}
	red.Add("supersecret")
	rl.red = red
	rl.WriteAnnotated(1, "stdout", []byte("token=supersecret"))
	_ = rl.Close()
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "supersecret") {
		t.Errorf("raw.log leaked the secret: %s", b)
	}
	if !strings.Contains(string(b), "token=***") {
		t.Errorf("raw.log missing masked content: %s", b)
	}
}
```

(Check `rawlog_test.go` for the actual fixed-clock helper name and imports; adapt.)

- [ ] **Step 2: Run to verify failure, then implement the sinks**

Run: `go test ./internal/runtime/executor/ -run TestRawLog_Redacts -v` → FAIL (`rl.red` undefined).

`rawlog.go` — add the field to `rawLog`:

```go
	// red masks registered secret values before any line is persisted.
	// Set by the Run that constructs this rawLog; nil-safe.
	red *redactor
```

In `WriteAnnotated`, before the `fmt.Fprintf`:

```go
	content = r.red.Apply(content)
```

`signals.go` — add the same field to `jsonlWriter`:

```go
	// red masks registered secret values in persisted signal lines. nil-safe.
	red *redactor
```

In `WriteLine`, first line of the locked section:

```go
	b = j.red.Apply(b)
```

- [ ] **Step 3: Redact at the pump choke point**

In `signals.go`, change both pump signatures to accept the redactor:

```go
func pumpStdout(r io.Reader, stepIdx int, rl *rawLog, signalsJSONL *jsonlWriter, obs *signalConfig, red *redactor) (pumpOutput, error)
func pumpStderr(r io.Reader, stepIdx int, rl *rawLog, signalsJSONL *jsonlWriter, obs *signalConfig, red *redactor) (pumpOutput, error)
```

In each, immediately after `lineCopy := append([]byte(nil), ...)`:

```go
		// Redact before ANYTHING sees the line — raw.log, signal matching,
		// JSON decode — so matched_line evidence and aggregates are clean.
		lineCopy = red.Apply(lineCopy)
```

- [ ] **Step 4: Plumb through stepArgs/topStepArgs and Run**

`step.go` — add to `stepArgs`:

```go
	// Redactor masks injected secret values in every persisted byte. May be nil.
	Redactor *redactor
```

Update the two pump goroutine calls in `runStep` to pass `a.Redactor` as the final argument.

`compose.go` — add `Redactor *redactor` to `topStepArgs`; in `execRunStep` and in `execUsesStep`'s inner `runStep` call, set `Redactor: a.Redactor` in the constructed `stepArgs`.

`executor.go` `Run` — after creating `rl` and `sw`:

```go
	red := &redactor{}
	rl.red = red
	sw.red = red
```

and set `Redactor: red` in the `topStepArgs` literal inside the steps loop.

- [ ] **Step 5: Run the executor suite**

Run: `go test ./internal/runtime/executor/`
Expected: PASS (existing tests unaffected — empty redactor is a no-op).

- [ ] **Step 6: Commit**

```bash
git add internal/runtime/executor/
git commit -m "feat(#49): redact injected values at every persistence sink

- Lines are masked at the pump before matching, so evidence and heal
  hints never carry a secret
- raw.log and signals.jsonl writers double as a belt-and-braces layer"
```

---

### Task 8: Ambient injection — Options.EnvFile flows into every step's process

**Files:**
- Modify: `internal/runtime/executor/executor.go`
- Modify: `internal/runtime/executor/step.go`
- Modify: `internal/runtime/executor/compose.go`
- Test: `internal/runtime/executor/run_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/runtime/executor/run_test.go` (follow `TestRunAssertion_PassSingleStep`'s construction exactly — `emptyStore{}`, `fixedExecNow`):

```go
func TestRun_EnvFileValueReachesStepProcess(t *testing.T) {
	repo := t.TempDir()
	envFile := filepath.Join(repo, ".env")
	if err := os.WriteFile(envFile, []byte("HARNESS_T8_TOKEN=fromenvfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "env-inject", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		SignalMatches: []sensor.SignalMatch{
			{Key: "seen", Pattern: "token=fromenvfile", Verdict: enums.VerdictPass},
		},
		Steps: []sensor.Step{{ID: "only", Run: `echo "token=$HARNESS_T8_TOKEN"`}},
	}
	ex := New(Options{
		RepoRoot: repo, EnvFile: envFile,
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})
	agg, err := ex.Run(context.Background(), s, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass (env_file value did not reach the child)", agg.Verdict)
	}
}

func TestRun_HostWinsOverEnvFile(t *testing.T) {
	t.Setenv("HARNESS_T8_CLASH", "fromhost")
	repo := t.TempDir()
	envFile := filepath.Join(repo, ".env")
	if err := os.WriteFile(envFile, []byte("HARNESS_T8_CLASH=fromfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "env-clash", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		SignalMatches: []sensor.SignalMatch{
			{Key: "host-won", Pattern: "clash=fromhost", Verdict: enums.VerdictPass},
		},
		Steps: []sensor.Step{{ID: "only", Run: `echo "clash=$HARNESS_T8_CLASH"`}},
	}
	ex := New(Options{
		RepoRoot: repo, EnvFile: envFile,
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})
	agg, err := ex.Run(context.Background(), s, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass (host value should win)", agg.Verdict)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/runtime/executor/ -run "TestRun_EnvFile|TestRun_HostWins" -v`
Expected: FAIL — `Options.EnvFile` undefined.

- [ ] **Step 3: Implement**

`executor.go` — add to `Options` (after `RepoRoot`):

```go
	// EnvFile is the resolved path of the manifest-declared dotenv file
	// (stack-manifest env_file joined to the repo root). Empty means no
	// env_file is declared; steps then see only the host environment.
	EnvFile string
```

In `Run`, right after the `red := &redactor{}` block from Task 7:

```go
	view, envFileMissing, viewErr := loadEnvView(e.opts.EnvFile)
	if viewErr != nil {
		// Surfaced as an inconclusive env-file-invalid signal in Task 9;
		// for now propagate.
		return aggregate.AggregateSignal{}, viewErr
	}
	// Every env_file-sourced value is a secret-by-injection (spec: mask
	// all injected values); host-only vars are untouched status quo.
	for _, v := range view.ambient {
		red.Add(v)
	}
	if envFileMissing {
		rl.WriteAnnotated(0, "env-file-missing", []byte(e.opts.EnvFile))
	}
```

Add `EnvView: view` to the `topStepArgs` literal in the steps loop.

`compose.go` — add to `topStepArgs`:

```go
	EnvView envView // merged host+env_file ambient view, loaded once per run
```

Pass `EnvView: a.EnvView` in `execRunStep`'s `stepArgs` literal and in `execUsesStep`'s inner `runStep` `stepArgs` literal.

`step.go` — add to `stepArgs`:

```go
	// EnvView is the merged host+env_file ambient view. Zero value = no
	// env_file declared.
	EnvView envView
```

In `runStep` env construction, right after `env := os.Environ()`:

```go
	// Ambient env_file values first: pre-filtered to names the host does
	// not set, so the host always wins, and later injections override.
	for k, v := range a.EnvView.ambient {
		env = append(env, k+"="+v)
	}
```

- [ ] **Step 4: Run the executor suite**

Run: `go test ./internal/runtime/executor/`
Expected: PASS, including the two new tests.

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/executor/
git commit -m "feat(#49): inject the app's env_file into every step process

- Recipes see the same configuration the dev server loaded
- Host environment always wins, keeping CI in control
- Every file-sourced value is registered for redaction"
```

---

### Task 9: Pre-spawn missing_env — typed signals, inconclusive aggregates

**Files:**
- Create: `internal/runtime/executor/envsignal.go`
- Modify: `internal/runtime/executor/errors.go` (or envsignal.go) — `MissingEnvError`
- Modify: `internal/runtime/executor/step.go` (refs.Env check)
- Modify: `internal/runtime/executor/compose.go` (execRunStep catch)
- Modify: `internal/runtime/executor/executor.go` (sensor-level preflight + env-file-invalid path)
- Test: `internal/runtime/executor/run_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `run_test.go`:

```go
func TestRun_MissingEnvRefAggregatesInconclusive(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "env-missing-ref", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:  []string{"fake-stack"},
		Steps: []sensor.Step{{ID: "only", Run: `echo "t=${{ env.HARNESS_T9_ABSENT }}"`}},
	}
	ex := New(Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})
	runDir := t.TempDir()
	agg, err := ex.Run(context.Background(), s, runDir, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictInconclusive {
		t.Errorf("verdict = %q, want inconclusive", agg.Verdict)
	}
	b, _ := os.ReadFile(filepath.Join(runDir, "signals.jsonl"))
	if !strings.Contains(string(b), "missing-env") || !strings.Contains(string(b), "HARNESS_T9_ABSENT") {
		t.Errorf("signals.jsonl missing the typed missing-env record: %s", b)
	}
}

func TestRun_SensorDeclaredRequiredEnvBlocksAllSteps(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	marker := filepath.Join(t.TempDir(), "ran")
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "env-declared", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:  []string{"fake-stack"},
		Env:   map[string]sensor.EnvSpec{"HARNESS_T9_REQ": {Description: "needed"}},
		Steps: []sensor.Step{{ID: "only", Run: "touch " + marker}},
	}
	ex := New(Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})
	agg, err := ex.Run(context.Background(), s, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictInconclusive {
		t.Errorf("verdict = %q, want inconclusive", agg.Verdict)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("step ran despite missing required env (must be pre-spawn)")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/runtime/executor/ -run "TestRun_MissingEnvRef|TestRun_SensorDeclared" -v`
Expected: FAIL (first: spawn proceeds with empty var → pass/no missing-env signal; second: verdict pass).

- [ ] **Step 3: Implement the error and signal types**

Create `internal/runtime/executor/envsignal.go`:

```go
package executor

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/signal"
)

// MissingEnvError reports ambient environment variables a step requires
// (via ${{ env.* }} refs, its env: map, or a composed primitive's
// declared env) that the merged host+env_file view does not provide.
// Pre-spawn: the step's process is never started, so no recipe ever sees
// a silent empty value.
type MissingEnvError struct {
	Step    int
	Names   []string
	EnvFile string // "" when no env_file is declared
}

func (e *MissingEnvError) Error() string {
	return "executor: missing required env var(s): " + strings.Join(e.Names, ", ")
}

// missingEnvSignal synthesizes the typed pre-spawn signal for absent
// ambient env vars. Verdict inconclusive: the application is not proven
// broken, the environment is incomplete (mirrors provision-auth's
// auth-not-provisioned contract).
func missingEnvSignal(s sensor.Sensor, names []string, envFile string, now func() time.Time) signal.Signal {
	sources := "host environment"
	if envFile != "" {
		sources += " and " + envFile
	}
	return envProblemSignal(s, "missing-env",
		signal.Evidence{"missing": strings.Join(names, ","), "sources": sources},
		"Missing required env var(s): "+strings.Join(names, ", "),
		"The step needs ambient configuration the harness could not find in the "+sources+
			". Export the variable(s) before invoking the harness, or add them to the project's env_file"+
			" — no step process was spawned.",
		now)
}

// envFileInvalidSignal surfaces an unparseable manifest-declared env_file.
func envFileInvalidSignal(s sensor.Sensor, envFile, parseErr string, now func() time.Time) signal.Signal {
	return envProblemSignal(s, "env-file-invalid",
		signal.Evidence{"env_file": envFile, "error": parseErr},
		"Declared env_file could not be parsed: "+envFile,
		"The stack manifest binds an env_file the dotenv parser rejects. Fix the file's syntax"+
			" (or correct the manifest's env_file path) — no step was run.",
		now)
}

func envProblemSignal(s sensor.Sensor, key string, ev signal.Evidence, summary, rationale string, now func() time.Time) signal.Signal {
	ev["observation_key"] = key
	return signal.Signal{
		SchemaVersion: observationSignalSchemaVersion,
		SensorID:      s.ID,
		UseCaseID:     s.UseCaseID,
		Angle:         s.Angle,
		EmittedAt:     now(),
		Verdict:       enums.VerdictInconclusive,
		Confidence:    1,
		Evidence:      ev,
		HealHint:      &signal.HealHint{Summary: summary, Rationale: rationale},
	}
}

// writeSignal best-effort persists one synthesized signal to signals.jsonl.
func writeSignal(sw *jsonlWriter, sig signal.Signal) {
	if b, err := json.Marshal(sig); err == nil {
		_ = sw.WriteLine(b)
	}
}

// missingRequiredEnv returns the names of required declared env vars that
// are unset or empty in the merged view, sorted.
func missingRequiredEnv(decl map[string]sensor.EnvSpec, view envView) []string {
	var missing []string
	for name, spec := range decl {
		if !spec.IsRequired() {
			continue
		}
		if v, ok := view.lookup(name); !ok || v == "" {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}
```

- [ ] **Step 4: Check run-script env refs in runStep**

In `step.go` `runStep`, immediately after the `template.Compile` call:

```go
	// Pre-spawn ambient floor: every ${{ env.NAME }} the run script
	// references must be present and non-empty, or the step never spawns.
	var missingEnv []string
	for _, name := range refs.Env {
		if v, ok := a.EnvView.lookup(name); !ok || v == "" {
			missingEnv = append(missingEnv, name)
		}
	}
	if len(missingEnv) > 0 {
		sort.Strings(missingEnv)
		return stepOutcome{}, &MissingEnvError{Step: a.StepIdx, Names: missingEnv, EnvFile: a.EnvView.source}
	}
```

(Add `"sort"` to step.go imports.)

- [ ] **Step 5: Catch MissingEnvError in both step paths**

`compose.go` `execRunStep` — after `outcome, err := runStep(...)`:

```go
	var me *MissingEnvError
	if errors.As(err, &me) {
		sig := missingEnvSignal(a.Sensor, me.Names, me.EnvFile, e.opts.Now)
		writeSignal(a.SignalsW, sig)
		return topStepResult{
			Signals:    []signal.Signal{sig},
			Outputs:    map[string]string{},
			TermReason: enums.TerminationError,
			StepErr:    me,
		}
	}
```

`execUsesStep` inner loop — after `outcome, runErr := runStep(...)` and the `innerOutputs[inner.ID] = ...` line, add the identical catch (returning `res` with the signal appended):

```go
		var me *MissingEnvError
		if errors.As(runErr, &me) {
			sig := missingEnvSignal(a.Sensor, me.Names, me.EnvFile, e.opts.Now)
			writeSignal(a.SignalsW, sig)
			res.Signals = append(res.Signals, sig)
			res.TermReason = enums.TerminationError
			res.StepErr = me
			return res
		}
```

- [ ] **Step 6: Sensor-level preflight in Run**

In `executor.go` `Run`, replace the Task 8 temporary `return ... viewErr` and wrap the steps loop:

```go
	view, envFileMissing, viewErr := loadEnvView(e.opts.EnvFile)
	if viewErr == nil {
		for _, v := range view.ambient {
			red.Add(v)
		}
	}
	if envFileMissing {
		rl.WriteAnnotated(0, "env-file-missing", []byte(e.opts.EnvFile))
	}
```

and after the `globalIdx := 0` declaration, gate the loop:

```go
	// Environment preflight: an unparseable env_file or a missing
	// sensor-declared required var blocks every step (pre-spawn) and
	// rolls up inconclusive — the app is not proven broken.
	if viewErr != nil {
		sig := envFileInvalidSignal(s, e.opts.EnvFile, viewErr.Error(), e.opts.Now)
		writeSignal(sw, sig)
		allSignals = append(allSignals, toAggregateSignals([]signal.Signal{sig})...)
		termReason = enums.TerminationError
		stepErr = viewErr
	} else if missing := missingRequiredEnv(s.Env, view); len(missing) > 0 {
		sig := missingEnvSignal(s, missing, view.source, e.opts.Now)
		writeSignal(sw, sig)
		allSignals = append(allSignals, toAggregateSignals([]signal.Signal{sig})...)
		termReason = enums.TerminationError
		stepErr = &MissingEnvError{Names: missing, EnvFile: view.source}
	} else {
		for _, step := range s.Steps {
			// ... existing loop body unchanged ...
		}
	}
```

- [ ] **Step 7: Run the executor suite**

Run: `go test ./internal/runtime/executor/`
Expected: PASS, including both new tests. If the inconclusive assertion fails, inspect `aggregate.Rollup`'s handling of `TerminationError` + inconclusive signals (the #45/#46 "Rule 0" path in `internal/aggregate`) before changing anything — the expected behavior is: explicit fail wins, else inconclusive.

- [ ] **Step 8: Commit**

```bash
git add internal/runtime/executor/
git commit -m "feat(#49): pre-spawn missing_env diagnostics

- A step needing an absent variable never spawns with a silent empty value
- Typed missing-env / env-file-invalid signals carry remediation hints
- Sensors aggregate inconclusive: the environment, not the app, is at fault"
```

---

### Task 10: Per-step `env:` resolution, injection, and composed-primitive flow

**Files:**
- Create: `internal/runtime/executor/stepenv.go`
- Test: `internal/runtime/executor/stepenv_test.go`
- Modify: `internal/runtime/executor/step.go`
- Modify: `internal/runtime/executor/compose.go`

- [ ] **Step 1: Write the failing unit tests for resolveStepEnv**

Create `internal/runtime/executor/stepenv_test.go`:

```go
package executor

import (
	"testing"
)

func TestResolveStepEnv_LiteralAndRefs(t *testing.T) {
	view := envView{ambient: map[string]string{"AMBIENT_SECRET": "shhh-value"}}
	stepOut := map[string]string{stepOutEnvName("auth", "header"): "Cookie: tok=1"}
	resolved, refDerived, missing, err := resolveStepEnv(
		map[string]string{
			"NODE_ENV":  "test",
			"SECRET":    "${{ env.AMBIENT_SECRET }}",
			"AUTH_HDR":  "${{ steps.auth.outputs.header }}",
			"COMPOSITE": "pre-${{ env.AMBIENT_SECRET }}-post",
		},
		view, nil, stepOut, nil)
	if err != nil || len(missing) != 0 {
		t.Fatalf("err=%v missing=%v", err, missing)
	}
	if resolved["NODE_ENV"] != "test" || refDerived["NODE_ENV"] {
		t.Errorf("literal: %q refDerived=%v (want test, false)", resolved["NODE_ENV"], refDerived["NODE_ENV"])
	}
	if resolved["SECRET"] != "shhh-value" || !refDerived["SECRET"] {
		t.Errorf("env ref: %q refDerived=%v (want shhh-value, true)", resolved["SECRET"], refDerived["SECRET"])
	}
	if resolved["AUTH_HDR"] != "Cookie: tok=1" {
		t.Errorf("step output ref: %q", resolved["AUTH_HDR"])
	}
	if resolved["COMPOSITE"] != "pre-shhh-value-post" {
		t.Errorf("composite: %q", resolved["COMPOSITE"])
	}
}

func TestResolveStepEnv_MissingAndEmptyCollect(t *testing.T) {
	view := envView{ambient: map[string]string{"EMPTY_ONE": ""}}
	resolved, _, missing, err := resolveStepEnv(
		map[string]string{
			"A": "${{ env.TOTALLY_ABSENT_XYZ }}",
			"B": "${{ env.EMPTY_ONE }}",
		},
		view, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 2 || missing[0] != "EMPTY_ONE" || missing[1] != "TOTALLY_ABSENT_XYZ" {
		t.Errorf("missing = %v, want [EMPTY_ONE TOTALLY_ABSENT_XYZ]", missing)
	}
	if _, ok := resolved["A"]; ok {
		t.Error("partially-resolved entry must not be injected")
	}
}

func TestResolveStepEnv_EntryPointRejected(t *testing.T) {
	_, _, _, err := resolveStepEnv(
		map[string]string{"X": "${{ entry_points.create-thing }}"},
		envView{}, nil, nil, nil)
	if err == nil {
		t.Error("entry_points.* must be rejected in env values")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/runtime/executor/ -run TestResolveStepEnv -v`
Expected: FAIL — `resolveStepEnv` undefined.

- [ ] **Step 3: Implement resolveStepEnv**

Create `internal/runtime/executor/stepenv.go`:

```go
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// resolveStepEnv resolves a step's declared env: map into concrete
// NAME=value pairs at spawn time. Values support the full ${{ }} surface:
// literals pass through verbatim; env.* resolves from the ambient view
// (unset-or-empty names are collected into missing — never injected as
// empty strings); inputs.* / steps.* resolve from the compiled env maps;
// fixtures.* resolves to the bound payload path. refDerived marks names
// whose value contains at least one resolved ref — secrets-by-construction
// that must be registered for redaction (inline literals are repo-visible
// and exempt). Structural problems (unbound input/fixture, entry_points)
// are errors, not missing-env.
func resolveStepEnv(
	stepEnv map[string]string,
	view envView,
	inputEnv, stepOutEnv map[string]string,
	fixturePaths map[string]string,
) (resolved map[string]string, refDerived map[string]bool, missing []string, err error) {
	resolved = map[string]string{}
	refDerived = map[string]bool{}
	for name, raw := range stepEnv {
		segs, perr := template.Parse(raw)
		if perr != nil {
			return nil, nil, nil, fmt.Errorf("env %q: %w", name, perr)
		}
		var b strings.Builder
		sawRef := false
		miss := false
		for _, seg := range segs {
			switch v := seg.(type) {
			case template.Literal:
				b.WriteString(v.Text)
			case template.EnvRef:
				val, ok := view.lookup(v.Name)
				if !ok || val == "" {
					missing = append(missing, v.Name)
					miss = true
					continue
				}
				b.WriteString(val)
				sawRef = true
			case template.InputRef:
				val, ok := inputEnv[inputEnvName(v.Name)]
				if !ok {
					return nil, nil, nil, fmt.Errorf("env %q: input %q is not bound in this context", name, v.Name)
				}
				b.WriteString(val)
				sawRef = true
			case template.StepOutputRef:
				b.WriteString(stepOutEnv[stepOutEnvName(v.StepID, v.Name)])
				sawRef = true
			case template.FixtureRef:
				if len(v.JSONPath) > 0 {
					return nil, nil, nil, fmt.Errorf("env %q: fixture jsonpath drilling is not supported", name)
				}
				p, ok := fixturePaths[v.ID]
				if !ok {
					return nil, nil, nil, fmt.Errorf("env %q: fixture %q was not bound", name, v.ID)
				}
				b.WriteString(p)
				sawRef = true
			case template.EntryPointRef:
				return nil, nil, nil, fmt.Errorf("env %q: entry_points.* is not valid in env values", name)
			default:
				return nil, nil, nil, fmt.Errorf("env %q: unsupported segment %T", name, seg)
			}
		}
		if miss {
			continue // never inject a partial value
		}
		resolved[name] = b.String()
		refDerived[name] = sawRef
	}
	sort.Strings(missing)
	return resolved, refDerived, missing, nil
}
```

Run: `go test ./internal/runtime/executor/ -run TestResolveStepEnv -v` → PASS.

- [ ] **Step 4: Wire into runStep**

In `step.go`:

a) Add to `stepArgs`:

```go
	// ExtraEnv carries already-resolved env entries a consumer uses-step
	// declared; injected into every inner step of the expansion, after
	// the ambient view and before the step's own env:.
	ExtraEnv map[string]string
```

b) Extend fixture binding to cover env values — replace the `binder.Bind(refs.Fixtures, ...)` call region:

```go
	envFixtures, err := fixturebinder.CollectFixtureRefs("", a.Step.Env)
	if err != nil {
		return stepOutcome{}, &TemplateError{Step: a.StepIdx, Cause: err}
	}
	binding, err := binder.Bind(mergeKeys(refs.Fixtures, envFixtures), a.UseCase, a.Store)
```

c) After the Task 9 refs.Env check, resolve the step's own env (merging both missing lists; final form of the check):

```go
	stepEnv, refDerived, missing2, err := resolveStepEnv(a.Step.Env, a.EnvView, a.InputEnv, a.StepOutEnv, binding.Files)
	if err != nil {
		return stepOutcome{}, &TemplateError{Step: a.StepIdx, Cause: err}
	}
	missingEnv = mergeKeys(missingEnv, missing2)
	if len(missingEnv) > 0 {
		sort.Strings(missingEnv)
		return stepOutcome{}, &MissingEnvError{Step: a.StepIdx, Names: missingEnv, EnvFile: a.EnvView.source}
	}
	for name, val := range stepEnv {
		if refDerived[name] {
			a.Redactor.Add(val)
		}
	}
```

NOTE: this moves the Task 9 `if len(missingEnv) > 0` return BELOW the resolveStepEnv call (one merged check). The fixture `binder.Bind` must run before `resolveStepEnv` (it needs `binding.Files`), so order is: refs.Env collection → bind → resolveStepEnv → merged missing check.

d) In the env construction, after the `a.StepOutEnv` loop and before the `outTag` logic:

```go
	for k, v := range a.ExtraEnv {
		env = append(env, k + "=" + v)
	}
	for k, v := range stepEnv {
		env = append(env, k + "=" + v)
	}
```

(Step's own env appended last → wins over consumer ExtraEnv → wins over ambient/host. POSIX exec gives the last duplicate to the child shell.)

- [ ] **Step 5: Wire the composed-primitive flow in execUsesStep**

In `compose.go` `execUsesStep`:

a) Collect env-value fixture refs alongside with-value refs — after the existing `ids, err := fixturebinder.CollectFixtureRefs("", a.Step.With)` block:

```go
	envIDs, err := fixturebinder.CollectFixtureRefs("", a.Step.Env)
	if err != nil {
		return topStepResult{TermReason: enums.TerminationError, StepErr: fmt.Errorf("executor: collect fixture refs in env: %w", err)}
	}
	ids = mergeKeys(ids, envIDs)
```

b) After `inputEnv, err := buildInputEnv(...)` succeeds, resolve the consumer step's env and enforce the primitive's declared requirements:

```go
	// Consumer-declared env: resolved once, injected into every inner step
	// of the expansion (issue #49) — this is how a use-case sensor hands
	// NEXTAUTH_SECRET to provision-auth's recipe. inputs.* is not valid at
	// the consumer level (mirrors with-value semantics), hence nil.
	consumerEnv, refDerived, consumerMissing, err := resolveStepEnv(a.Step.Env, a.EnvView, nil, a.StepOutEnv, fixturePaths)
	if err != nil {
		return topStepResult{TermReason: enums.TerminationError, StepErr: fmt.Errorf("executor: step %q: %w", a.Step.ID, err)}
	}
	// Primitive-declared ambient requirements: satisfied by the merged
	// host+env_file view or by the consumer's own env: injection.
	for name, spec := range prim.Env {
		if !spec.IsRequired() {
			continue
		}
		if v, ok := consumerEnv[name]; ok && v != "" {
			continue
		}
		if v, ok := a.EnvView.lookup(name); ok && v != "" {
			continue
		}
		consumerMissing = append(consumerMissing, name)
	}
	if len(consumerMissing) > 0 {
		consumerMissing = mergeKeys(consumerMissing, nil) // dedupe
		sort.Strings(consumerMissing)
		me := &MissingEnvError{Names: consumerMissing, EnvFile: a.EnvView.source}
		sig := missingEnvSignal(a.Sensor, consumerMissing, a.EnvView.source, e.opts.Now)
		writeSignal(a.SignalsW, sig)
		return topStepResult{
			Signals:    []signal.Signal{sig},
			Outputs:    map[string]string{},
			TermReason: enums.TerminationError,
			StepErr:    me,
		}
	}
	for name, val := range consumerEnv {
		if refDerived[name] {
			a.Redactor.Add(val)
		}
	}
```

(Add `"sort"` to compose.go imports.)

c) Pass `ExtraEnv: consumerEnv` in the inner `runStep`'s `stepArgs` literal.

- [ ] **Step 6: Write the failing composition test**

Append to `internal/runtime/executor/compose_test.go` (follow that file's existing primitive-composition test setup — there are tests wiring `SensorLookup` to a core primitive; copy the harness of one):

```go
func TestExecUsesStep_PrimitiveDeclaredEnvMissingIsInconclusive(t *testing.T) {
	// Arrange: a core primitive declaring a required env var nobody provides.
	prim := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "needs-secret", Scope: enums.ScopeCore,
		Angle: enums.AngleEnvironment, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Env:  map[string]sensor.EnvSpec{"HARNESS_T10_SECRET": {Description: "required"}},
		Steps: []sensor.Step{{ID: "inner", Run: "echo never-runs"}},
	}
	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer", UseCaseID: "fake-uc",
		Angle: enums.AngleE2E, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:  []string{"fake-stack"},
		Steps: []sensor.Step{{ID: "auth", Uses: "needs-secret"}},
	}
	// Act: run the consumer through an executor whose SensorLookup returns prim
	// (reuse the construction pattern of the sibling composition tests).
	// Assert: aggregate verdict == enums.VerdictInconclusive and signals.jsonl
	// contains "missing-env" and "HARNESS_T10_SECRET".
	_ = prim
	_ = consumer
	t.Fatal("wire this test using the composition harness in this file, then remove this line")
}

func TestExecUsesStep_ConsumerEnvSatisfiesPrimitiveRequirement(t *testing.T) {
	// Same primitive as above, but the consumer's uses-step declares
	//   Env: map[string]string{"HARNESS_T10_SECRET": "${{ env.HARNESS_T10_AMBIENT }}"}
	// and the Options.EnvFile dotenv provides HARNESS_T10_AMBIENT=long-secret-value.
	// The inner step runs `echo "got=$HARNESS_T10_SECRET"`.
	// Assert: aggregate verdict pass, raw.log does NOT contain
	// "long-secret-value" and DOES contain "got=***".
	t.Fatal("wire this test using the composition harness in this file, then remove this line")
}
```

These two stubs intentionally fail; implement them fully against the file's existing helpers (the harness for composition tests already exists there — match its `New(Options{...SensorLookup...})` wiring), then remove the `t.Fatal` lines. The assertions named in the comments are the contract.

- [ ] **Step 7: Run the executor suite**

Run: `go test ./internal/runtime/executor/`
Expected: PASS, all tests including the two new composition tests fully implemented.

- [ ] **Step 8: Commit**

```bash
git add internal/runtime/executor/
git commit -m "feat(#49): per-step env resolution and composed-primitive injection

- Consumer uses-steps hand secrets to a primitive's whole expansion
- A consumer env: entry satisfies the primitive's declared requirement
- Ref-derived values are redacted; inline literals stay readable
- Partial values are never injected: any miss blocks the spawn"
```

---

### Task 11: Boot wiring — manifest env_file reaches the executor

**Files:**
- Modify: `lib/skillruntime/boot.go`
- Test: `lib/skillruntime/boot_test.go`

⚠️ Run `git diff lib/skillruntime/boot.go` first. The working tree has pre-existing uncommitted changes to this file. If hunks unrelated to env_file exist, STOP and ask the user whether to include or exclude them before committing.

- [ ] **Step 1: Write the failing test**

Append to `lib/skillruntime/boot_test.go` (mirror the existing test's `.harness` fixture-building helpers; if it builds a temp `.harness` dir, extend that helper):

```go
func TestBootLifecycle_InvalidManifestErrors(t *testing.T) {
	repo := t.TempDir()
	harness := filepath.Join(repo, ".harness")
	if err := os.MkdirAll(harness, 0o755); err != nil {
		t.Fatal(err)
	}
	// Present-but-invalid manifest must surface, not be silently ignored.
	if err := os.WriteFile(filepath.Join(harness, "stack-manifest.yaml"), []byte("archetype: [not, a, string]"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BootLifecycle(repo); err == nil {
		t.Error("invalid stack manifest should fail boot")
	}
}

func TestBootLifecycle_ManifestAbsentStillBoots(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".harness"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := BootLifecycle(repo); err != nil {
		t.Errorf("boot without a manifest must keep working: %v", err)
	}
}
```

- [ ] **Step 2: Run to verify the first fails**

Run: `go test ./lib/skillruntime/ -run TestBootLifecycle_ -v`
Expected: `InvalidManifestErrors` FAILS (boot ignores the manifest today); `ManifestAbsentStillBoots` may already pass.

- [ ] **Step 3: Implement**

In `lib/skillruntime/boot.go`, add `"github.com/iurykrieger/lastro/internal/stack"` to imports. In `BootLifecycle`, after the use-cases load and before `exec := executor.New(...)`:

```go
	// Optional stack manifest: absence is normal before /detect-stack; a
	// present-but-invalid manifest is an error. When it binds an env_file,
	// every sensor step gets the merged host+dotenv view (issue #49).
	var envFile string
	manifestPath := filepath.Join(harnessDir, "stack-manifest.yaml")
	if _, statErr := os.Stat(manifestPath); statErr == nil {
		m, err := stack.Load(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("skillruntime: load stack manifest: %w", err)
		}
		if m.EnvFile != "" {
			envFile = filepath.Join(repoRoot, m.EnvFile)
		}
	}
```

Add `EnvFile: envFile,` to the `executor.Options` literal (after `RepoRoot`).

- [ ] **Step 4: Run the tests**

Run: `go test ./lib/skillruntime/`
Expected: PASS.

- [ ] **Step 5: Commit (only the env_file hunks — see the warning above)**

```bash
git add -p lib/skillruntime/boot.go   # stage ONLY the env_file hunks
git add lib/skillruntime/boot_test.go
git commit -m "feat(#49): bind the manifest's env_file at skill boot

- /run-sensor and /validate-use-case now reproduce the app's env without
  any manual export, closing the CI false-negative gap"
```

---

### Task 12: Generation guidance — baselines and skills

**Files:**
- Modify: `schemas/core-input-baseline.yaml`
- Modify: `internal/sensor/baseline.go` (EnvGuidance field)
- Modify: `schemas/core-inputs/provision-auth.yaml`, `schemas/core-inputs/database.yaml`
- Modify: `skills/create-core-sensors/SKILL.md`, `skills/create-sensors/SKILL.md`
- Test: `internal/sensor/baseline_test.go`

- [ ] **Step 1: Schema + type + baseline files**

`schemas/core-input-baseline.yaml` — add to `properties:` (after `primitive:`):

```yaml
  env_guidance:
    type: string
    minLength: 1
    description: |
      Generator guidance: which ambient env vars (secrets, connection
      strings) this primitive's recipes typically read. The generated
      sensor MUST declare each concrete var in its top-level env: block
      so the runtime fails fast with missing_env instead of a
      recipe-specific failure.
```

`internal/sensor/baseline.go` — add to `CoreInputBaseline` (after `Primitive`):

```go
	// EnvGuidance tells the generating skill which ambient vars this
	// primitive's recipes typically read; the generated sensor declares
	// the concrete names in its top-level env: block.
	EnvGuidance string `json:"env_guidance,omitempty"`
```

`schemas/core-inputs/provision-auth.yaml` — add after the `primitive:` line:

```yaml
env_guidance: "Credential recipes read app secrets from the process environment (a session-signing secret such as NEXTAUTH_SECRET; the datastore connection string such as DATABASE_URL for api-key seeding). Declare every var the generated recipe reads in the sensor's top-level env: block — the signing secret MUST be the same value the dev server loaded, and a missing var must surface as missing_env, not mint-failed."
```

`schemas/core-inputs/database.yaml` — add after the `angle:` line:

```yaml
env_guidance: "Query recipes read the datastore connection string from the process environment (e.g. DATABASE_URL); declare it in the generated sensor's top-level env: block so an absent value fails fast as missing_env."
```

- [ ] **Step 2: Test the round-trip**

Append to `internal/sensor/baseline_test.go`:

```go
func TestLoadBaselines_EnvGuidanceParsed(t *testing.T) {
	bl, err := LoadBaselines()
	if err != nil {
		t.Fatal(err)
	}
	if bl["provision-auth"].EnvGuidance == "" {
		t.Error("provision-auth baseline should carry env_guidance")
	}
	if bl["database"].EnvGuidance == "" {
		t.Error("database baseline should carry env_guidance")
	}
}
```

Run: `go test ./internal/sensor/ -run TestLoadBaselines_EnvGuidance -v` → PASS (FAIL first if Step 1 yaml not yet written — do test-first if convenient; the YAML is data, not logic, so either order is acceptable here).

- [ ] **Step 3: Update create-core-sensors skill**

In `skills/create-core-sensors/SKILL.md`, add one bullet to the "Rules:" list under "Parameterized primitives (composable)" (after the `depends_on:` bullet):

```markdown
- Declare top-level `env:` naming every ambient var a recipe reads from the
  process environment (secrets, connection strings — see the floor's
  `env_guidance`). The runtime injects the manifest's `env_file` into every
  step and fails fast with `missing_env` when a required var is absent or
  empty — never let a recipe diagnose a missing secret itself.
```

Then enforce the size budget: run `wc -l skills/create-core-sensors/SKILL.md`. If over 200 lines, apply this exact trim — replace the two-line "Coverage check" body:

```markdown
After writing all sensors, list `.harness/sensors/core/` and confirm that each
expected primitive is present. Emit any missing primitive before finishing.
```

with the single line:

```markdown
List `.harness/sensors/core/` and emit any missing expected primitive before finishing.
```

and re-check `wc -l` ≤ 200.

- [ ] **Step 4: Update create-sensors skill**

Read `skills/create-sensors/SKILL.md`. After the section describing uses-step composition (`uses:` + `with:` binding — locate the paragraph that explains binding primitive inputs), add:

```markdown
**Ambient env:** the runtime injects the manifest's `env_file` into every
step automatically — do NOT add `env:` maps that merely restate `.env`
contents. Declare a step-level `env:` only to rename a var for a recipe
(`SECRET: ${{ env.NEXTAUTH_SECRET }}`), to forward a prior step's output,
or to satisfy a composed primitive's declared `env:` requirement from a
non-default source. Values resolved from refs are redacted in all logs.
```

Run `wc -l skills/create-sensors/SKILL.md` — if the addition pushes it past 200 lines, condense the least essential nearby prose (keep the budget) and note the trim in the commit body.

- [ ] **Step 5: Run sensor + schemas tests**

Run: `go test ./internal/sensor/ ./schemas/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add schemas/core-input-baseline.yaml schemas/core-inputs/ internal/sensor/ skills/create-core-sensors/SKILL.md skills/create-sensors/SKILL.md
git commit -m "feat(#49): teach generators to declare ambient env requirements

- Baselines carry env_guidance so generated primitives fail fast with
  missing_env instead of recipe-specific errors
- Use-case sensors add env: only for overrides; env_file covers the rest"
```

---

### Task 13: End-to-end integration tests + full verification

**Files:**
- Test: `internal/runtime/executor/run_test.go`

- [ ] **Step 1: Write the acceptance-shaped integration test**

Append to `run_test.go` — the full chain: env_file + `${{ env.* }}` in a run script + redaction in raw.log AND signals.jsonl:

```go
func TestRun_EnvEndToEnd_InjectionRedactionAndEvidence(t *testing.T) {
	repo := t.TempDir()
	envFile := filepath.Join(repo, ".env")
	const secret = "nextauth-secret-value-1234"
	if err := os.WriteFile(envFile, []byte("HARNESS_T13_SECRET="+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "env-e2e", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		SignalMatches: []sensor.SignalMatch{
			{Key: "minted", Pattern: "minted=(?P<tok>\\S+)", Verdict: enums.VerdictPass},
		},
		Steps: []sensor.Step{{
			ID:  "mint",
			Run: `echo "minted=$INJECTED"`,
			Env: map[string]string{"INJECTED": "${{ env.HARNESS_T13_SECRET }}"},
		}},
	}
	ex := New(Options{
		RepoRoot: repo, EnvFile: envFile,
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})
	runDir := t.TempDir()
	agg, err := ex.Run(context.Background(), s, runDir, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Fatalf("verdict = %q, want pass", agg.Verdict)
	}
	for _, f := range []string{"raw.log", "signals.jsonl"} {
		b, _ := os.ReadFile(filepath.Join(runDir, f))
		if strings.Contains(string(b), secret) {
			t.Errorf("%s leaked the secret", f)
		}
		if !strings.Contains(string(b), redactedPlaceholder) {
			t.Errorf("%s carries no masked content: %s", f, b)
		}
	}
	// Aggregate evidence must be clean too: the matched_line evidence came
	// from a pre-redacted pump line.
	rawAgg, _ := json.Marshal(agg)
	if strings.Contains(string(rawAgg), secret) {
		t.Error("aggregate evidence leaked the secret")
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/runtime/executor/ -run TestRun_EnvEndToEnd -v`
Expected: PASS (everything was built in Tasks 5–10; failure here means a wiring gap — debug before proceeding).

- [ ] **Step 3: Full repo verification**

```bash
go build ./... && go vet ./... && go test ./...
```

Expected: all PASS. Fix anything that surfaces.

- [ ] **Step 4: Acceptance checklist against the spec**

Verify each, citing the test that proves it:

1. Step `env:` merged into the step's process — `TestRun_EnvEndToEnd_InjectionRedactionAndEvidence`, `TestExecUsesStep_ConsumerEnvSatisfiesPrimitiveRequirement`.
2. `${{ env.NAME }}` resolves from host and/or env_file; undeclared required → typed error, not empty — `TestRun_MissingEnvRefAggregatesInconclusive`, `TestRun_HostWinsOverEnvFile`.
3. No manual export needed end-to-end — boot wiring (`TestBootLifecycle_*`) + `Options.EnvFile` tests. (Real-repo NextAuth validation happens on the next dogfood run of `/run-sensor`; note it in the PR.)
4. Secrets never in raw.log / signals.jsonl / aggregate evidence — `TestRun_EnvEndToEnd_InjectionRedactionAndEvidence`.

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/executor/run_test.go
git commit -m "test(#49): end-to-end proof for declarative env injection

- A secret travels .env -> step env -> recipe and never reaches any log
- Missing variables grade inconclusive with actionable evidence"
```

- [ ] **Step 6: Finish the branch**

Use the superpowers:finishing-a-development-branch flow (merge vs PR per user preference). Reference issue #49 in the PR body with the acceptance checklist from Step 4.
