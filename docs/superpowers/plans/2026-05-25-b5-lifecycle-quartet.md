# B5 Sub-PR 1 — Lifecycle Quartet Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the first B5 sub-PR — four skill wrappers (`/run-sensor`, `/start-sensor`, `/stop-sensor`, `/tail-sensor-signals`) plus two shared libraries (`lib/skillio`, `lib/skillruntime`) that bootstrap a `*lifecycle.Lifecycle` from `.harness/`.

**Architecture:** Each skill is a Go `main` package under `skills/<name>/scripts/`, invoked via `go run`. Scripts use `lib/skillio` for stdout/stderr/exit conventions and `lib/skillruntime.BootLifecycle` to load `.harness/` artifacts and construct a configured `*lifecycle.Lifecycle`. Three skills wrap one Lifecycle method each (`RunSensor`, `StartSensor`, `StopSensor`); the fourth is a pure file reader over `<runDir>/signals.jsonl` with polling for follow-mode.

**Tech Stack:** Go 1.24, stdlib `flag`, existing internal packages (`internal/lifecycle`, `internal/runtime/executor`, `internal/sensor`, `internal/usecase`, `internal/fixture`, `internal/entrypoint`, `internal/usecase/template`, `internal/enums`), `github.com/oklog/ulid/v2` (already in go.mod via B2).

**Source spec:** [`docs/superpowers/specs/2026-05-25-b5-skill-wrappers-design.md`](../specs/2026-05-25-b5-skill-wrappers-design.md) §11.1 (acceptance), §8.1–8.4 (flows), §5–7 (layout + contracts).

---

## File structure

```
lib/skillio/
├── errors.go          typed error envelope + exit-code constants
├── errors_test.go
├── output.go          JSONL stdout + structured stderr emit helpers
├── output_test.go
├── repo.go            find repo root from cwd (walks up for .harness/ or go.mod)
└── repo_test.go

lib/skillruntime/
├── handles.go         parse/format "<sensor-id>:<run-id>" ↔ (sensorID, runID)
├── handles_test.go
├── boot.go            BootLifecycle(repoRoot) → *Booted
└── boot_test.go

skills/run-sensor/
├── skill.md
├── scripts/main.go
└── scripts/main_test.go

skills/start-sensor/
├── skill.md
├── scripts/main.go
└── scripts/main_test.go

skills/stop-sensor/
├── skill.md
├── scripts/main.go
└── scripts/main_test.go

skills/tail-sensor-signals/
├── skill.md
├── scripts/main.go
└── scripts/main_test.go
```

**File responsibilities:**

- `lib/skillio/errors.go` — defines `ScriptError` struct + JSON-encodes to stderr; exposes `Exit*` constants (`ExitPass=0`, `ExitFail=1`, `ExitInconclusive=2`, `ExitScriptError=3`); maps `enums.Verdict` → exit code.
- `lib/skillio/output.go` — `EmitJSON(w io.Writer, v any)` writes one JSON object + `\n`; `EmitError(w io.Writer, code, msg string, details map[string]any)` writes the structured error envelope.
- `lib/skillio/repo.go` — `FindRepoRoot(cwd string) (string, error)`: walks parents looking for `.harness/` then `go.mod`; returns absolute path. `HarnessDir(repoRoot)` returns `<repoRoot>/.harness`.
- `lib/skillruntime/handles.go` — `ParseHandle(s string) (sensorID, runID string, err error)`; `FormatHandle(sensorID, runID string) string`. Validates ULID shape (26 chars, base32) on both halves.
- `lib/skillruntime/boot.go` — `BootLifecycle(repoRoot string) (*Booted, error)`: loads sensors/use-cases/fixtures from `.harness/`, wires `*executor.Executor`, returns `*Booted` carrying `*lifecycle.Lifecycle` + `*sensor.Store` + `map[string]*usecase.UseCase` + `*fixture.Store` + `Cleanup func() error`.
- `skills/<name>/skill.md` — LLM-facing prompt under 200 lines (CLAUDE.md rule 4); describes the skill's purpose, args, and what to expect on stdout/stderr.
- `skills/<name>/scripts/main.go` — `main()` calls `os.Exit(run(...))`; `run(args, stdin, stdout, stderr, cwd) int` is the testable entry.

---

## Task 0: Branch setup

**Files:** none (git only)

- [ ] **Step 1:** Fetch latest main

Run: `git fetch origin main`
Expected: `From <repo>… branch main -> FETCH_HEAD`

- [ ] **Step 2:** Create the sub-PR branch from fresh main

Run: `git checkout -b feat/b5-lifecycle-wrappers origin/main`
Expected: `Switched to a new branch 'feat/b5-lifecycle-wrappers'`

- [ ] **Step 3:** Verify clean state

Run: `git status`
Expected: `On branch feat/b5-lifecycle-wrappers … working tree clean`

---

## Task 1: `lib/skillio/errors.go`

**Files:**
- Create: `lib/skillio/errors.go`
- Test: `lib/skillio/errors_test.go`

- [ ] **Step 1:** Write the failing test

Create `lib/skillio/errors_test.go`:

```go
package skillio

import (
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestExitCodeForVerdict(t *testing.T) {
	cases := []struct {
		v    enums.Verdict
		want int
	}{
		{enums.VerdictPass, ExitPass},
		{enums.VerdictFail, ExitFail},
		{enums.VerdictInconclusive, ExitInconclusive},
		{enums.VerdictWarn, ExitFail}, // warn is treated as fail for exit-code purposes
	}
	for _, c := range cases {
		if got := ExitCodeForVerdict(c.v); got != c.want {
			t.Errorf("ExitCodeForVerdict(%q) = %d, want %d", c.v, got, c.want)
		}
	}
}

func TestScriptError_Code(t *testing.T) {
	err := NewScriptError("bad-handle", "handle malformed", map[string]any{"input": "abc"})
	if err.Code != "bad-handle" {
		t.Errorf("Code = %q, want bad-handle", err.Code)
	}
	if err.Message != "handle malformed" {
		t.Errorf("Message = %q, want 'handle malformed'", err.Message)
	}
	if err.Details["input"] != "abc" {
		t.Errorf("Details[input] = %v, want abc", err.Details["input"])
	}
}
```

- [ ] **Step 2:** Run test to verify it fails

Run: `go test ./lib/skillio/...`
Expected: FAIL with "undefined: ExitPass" / "undefined: ScriptError"

- [ ] **Step 3:** Write minimal implementation

Create `lib/skillio/errors.go`:

```go
// Package skillio holds the conventions every harness skill script shares:
// exit codes, structured stderr errors, JSONL stdout helpers, and repo-root
// discovery. Both B4 (detection/generation) and B5 (execution) skills import
// this package; runtime logic lives elsewhere.
package skillio

import "github.com/iurykrieger/lastro/internal/enums"

// Exit-code conventions, uniform across every harness skill script.
const (
	ExitPass         = 0
	ExitFail         = 1
	ExitInconclusive = 2
	ExitScriptError  = 3
)

// ExitCodeForVerdict maps an enums.Verdict to the script's exit code.
// Verdicts outside the pass/inconclusive set map to ExitFail.
func ExitCodeForVerdict(v enums.Verdict) int {
	switch v {
	case enums.VerdictPass:
		return ExitPass
	case enums.VerdictInconclusive:
		return ExitInconclusive
	default:
		return ExitFail
	}
}

// ScriptError is the structured envelope written to stderr on
// script-level failures (bad argv, missing file, unparseable input).
// Runtime failures (failing sensor, malformed YAML inside a fixture)
// surface as terminal AggregateSignals on stdout instead.
type ScriptError struct {
	Code    string         `json:"code"`
	Message string         `json:"error"`
	Details map[string]any `json:"details,omitempty"`
}

// NewScriptError constructs a ScriptError. Details may be nil.
func NewScriptError(code, message string, details map[string]any) *ScriptError {
	return &ScriptError{Code: code, Message: message, Details: details}
}
```

- [ ] **Step 4:** Run tests to verify they pass

Run: `go test ./lib/skillio/...`
Expected: PASS, `ok  github.com/iurykrieger/lastro/lib/skillio`

- [ ] **Step 5:** Commit

```bash
git add lib/skillio/errors.go lib/skillio/errors_test.go
git commit -m "feat(skillio): exit-code constants and ScriptError envelope"
```

---

## Task 2: `lib/skillio/output.go`

**Files:**
- Create: `lib/skillio/output.go`
- Test: `lib/skillio/output_test.go`

- [ ] **Step 1:** Write the failing test

Create `lib/skillio/output_test.go`:

```go
package skillio

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEmitJSON_AppendsNewline(t *testing.T) {
	var buf bytes.Buffer
	if err := EmitJSON(&buf, map[string]string{"hello": "world"}); err != nil {
		t.Fatalf("EmitJSON: %v", err)
	}
	got := buf.String()
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("output missing trailing newline: %q", got)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &decoded); err != nil {
		t.Fatalf("output not valid JSON: %v (%q)", err, got)
	}
	if decoded["hello"] != "world" {
		t.Errorf("decoded[hello] = %q, want world", decoded["hello"])
	}
}

func TestEmitError_StructuredEnvelope(t *testing.T) {
	var buf bytes.Buffer
	EmitError(&buf, "bad-handle", "handle malformed", map[string]any{"input": "abc"})
	var decoded ScriptError
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &decoded); err != nil {
		t.Fatalf("output not valid JSON: %v (%q)", err, buf.String())
	}
	if decoded.Code != "bad-handle" {
		t.Errorf("Code = %q, want bad-handle", decoded.Code)
	}
	if decoded.Details["input"] != "abc" {
		t.Errorf("Details[input] = %v, want abc", decoded.Details["input"])
	}
}
```

- [ ] **Step 2:** Run test to verify it fails

Run: `go test ./lib/skillio/...`
Expected: FAIL with "undefined: EmitJSON" / "undefined: EmitError"

- [ ] **Step 3:** Write minimal implementation

Create `lib/skillio/output.go`:

```go
package skillio

import (
	"encoding/json"
	"io"
)

// EmitJSON writes a single JSON object to w followed by a newline.
// Used for stdout streams: signals, terminal AggregateSignal, UseCaseVerdict.
func EmitJSON(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err = w.Write([]byte{'\n'})
	return err
}

// EmitError writes a ScriptError envelope to w (typically stderr) as a
// single-line JSON object followed by a newline. Best-effort: encoding
// errors are dropped because the caller is already exiting.
func EmitError(w io.Writer, code, message string, details map[string]any) {
	_ = EmitJSON(w, NewScriptError(code, message, details))
}
```

- [ ] **Step 4:** Run tests to verify they pass

Run: `go test ./lib/skillio/...`
Expected: PASS

- [ ] **Step 5:** Commit

```bash
git add lib/skillio/output.go lib/skillio/output_test.go
git commit -m "feat(skillio): JSONL stdout and structured stderr emit helpers"
```

---

## Task 3: `lib/skillio/repo.go`

**Files:**
- Create: `lib/skillio/repo.go`
- Test: `lib/skillio/repo_test.go`

- [ ] **Step 1:** Write the failing test

Create `lib/skillio/repo_test.go`:

```go
package skillio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRepoRoot_FindsHarnessDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".harness"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := FindRepoRoot(nested)
	if err != nil {
		t.Fatalf("FindRepoRoot: %v", err)
	}
	// resolve symlinks because t.TempDir returns /tmp paths that may differ
	wantAbs, _ := filepath.EvalSymlinks(root)
	gotAbs, _ := filepath.EvalSymlinks(got)
	if gotAbs != wantAbs {
		t.Errorf("FindRepoRoot = %q, want %q", gotAbs, wantAbs)
	}
}

func TestFindRepoRoot_FallsBackToGoMod(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := FindRepoRoot(nested)
	if err != nil {
		t.Fatalf("FindRepoRoot: %v", err)
	}
	wantAbs, _ := filepath.EvalSymlinks(root)
	gotAbs, _ := filepath.EvalSymlinks(got)
	if gotAbs != wantAbs {
		t.Errorf("FindRepoRoot = %q, want %q", gotAbs, wantAbs)
	}
}

func TestFindRepoRoot_NoMarker(t *testing.T) {
	root := t.TempDir()
	_, err := FindRepoRoot(root)
	if err == nil {
		t.Errorf("expected error when no .harness/ or go.mod present")
	}
}

func TestHarnessDir(t *testing.T) {
	got := HarnessDir("/repo")
	want := filepath.Join("/repo", ".harness")
	if got != want {
		t.Errorf("HarnessDir = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2:** Run test to verify it fails

Run: `go test ./lib/skillio/...`
Expected: FAIL with "undefined: FindRepoRoot"

- [ ] **Step 3:** Write minimal implementation

Create `lib/skillio/repo.go`:

```go
package skillio

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindRepoRoot walks up from cwd looking for the nearest ancestor that
// contains a .harness/ directory; if none is found, falls back to the
// nearest ancestor containing a go.mod file. Returns the absolute path
// or an error if neither marker is found before reaching the filesystem
// root.
func FindRepoRoot(cwd string) (string, error) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("skillio: abs(%q): %w", cwd, err)
	}

	// Pass 1: look for .harness/
	for dir := abs; ; {
		if info, err := os.Stat(filepath.Join(dir, ".harness")); err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Pass 2: look for go.mod
	for dir := abs; ; {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("skillio: no .harness/ or go.mod found in %q or any parent", cwd)
}

// HarnessDir returns the absolute path to the .harness/ directory inside
// the given repo root.
func HarnessDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".harness")
}
```

- [ ] **Step 4:** Run tests to verify they pass

Run: `go test ./lib/skillio/...`
Expected: PASS, four tests

- [ ] **Step 5:** Commit

```bash
git add lib/skillio/repo.go lib/skillio/repo_test.go
git commit -m "feat(skillio): repo-root discovery via .harness/ then go.mod"
```

---

## Task 4: `lib/skillruntime/handles.go`

**Files:**
- Create: `lib/skillruntime/handles.go`
- Test: `lib/skillruntime/handles_test.go`

- [ ] **Step 1:** Write the failing test

Create `lib/skillruntime/handles_test.go`:

```go
package skillruntime

import (
	"strings"
	"testing"
)

const (
	validULID1 = "01HMG12RX9N6Z8WJ3D6PNHVQXC"
	validULID2 = "01HMG12RXATAFM4N0F0X5Y4SGE"
)

func TestParseHandle_OK(t *testing.T) {
	sensorID, runID, err := ParseHandle(validULID1 + ":" + validULID2)
	if err != nil {
		t.Fatalf("ParseHandle: %v", err)
	}
	if sensorID != validULID1 {
		t.Errorf("sensorID = %q, want %q", sensorID, validULID1)
	}
	if runID != validULID2 {
		t.Errorf("runID = %q, want %q", runID, validULID2)
	}
}

func TestParseHandle_NoColon(t *testing.T) {
	_, _, err := ParseHandle(validULID1)
	if err == nil || !strings.Contains(err.Error(), "missing ':'") {
		t.Errorf("expected missing-colon error, got %v", err)
	}
}

func TestParseHandle_WrongLength(t *testing.T) {
	_, _, err := ParseHandle("short:" + validULID2)
	if err == nil {
		t.Errorf("expected error on short sensor id")
	}
}

func TestParseHandle_NonULIDChars(t *testing.T) {
	bad := strings.Repeat("?", 26)
	_, _, err := ParseHandle(bad + ":" + validULID2)
	if err == nil {
		t.Errorf("expected error on non-ULID chars")
	}
}

func TestFormatHandle(t *testing.T) {
	got := FormatHandle(validULID1, validULID2)
	want := validULID1 + ":" + validULID2
	if got != want {
		t.Errorf("FormatHandle = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2:** Run test to verify it fails

Run: `go test ./lib/skillruntime/...`
Expected: FAIL with "undefined: ParseHandle"

- [ ] **Step 3:** Write minimal implementation

Create `lib/skillruntime/handles.go`:

```go
// Package skillruntime bootstraps a configured *lifecycle.Lifecycle from
// .harness/ for B5's skill scripts to use, and exposes helpers for the
// "<sensor-id>:<run-id>" handle format the skills hand to the user.
package skillruntime

import (
	"fmt"
	"strings"
)

// ulidLen is the canonical text length of a ULID per
// github.com/oklog/ulid/v2 (Crockford base32, 26 chars).
const ulidLen = 26

// ParseHandle splits a "<sensor-id>:<run-id>" string into its components
// and validates that each half is a ULID-shaped 26-char base32 token.
func ParseHandle(s string) (sensorID, runID string, err error) {
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return "", "", fmt.Errorf("skillruntime: handle missing ':' separator: %q", s)
	}
	sensorID, runID = s[:i], s[i+1:]
	if err := validateULIDShape(sensorID); err != nil {
		return "", "", fmt.Errorf("skillruntime: sensor id: %w", err)
	}
	if err := validateULIDShape(runID); err != nil {
		return "", "", fmt.Errorf("skillruntime: run id: %w", err)
	}
	return sensorID, runID, nil
}

// FormatHandle composes "<sensor-id>:<run-id>".
func FormatHandle(sensorID, runID string) string {
	return sensorID + ":" + runID
}

// validateULIDShape checks length and Crockford base32 alphabet.
// We do NOT parse the timestamp; B2's lifecycle uses oklog/ulid for that
// when it matters. This is a fast structural check at the boundary.
func validateULIDShape(s string) error {
	if len(s) != ulidLen {
		return fmt.Errorf("expected %d chars, got %d (%q)", ulidLen, len(s), s)
	}
	for _, r := range s {
		if !isCrockfordBase32(r) {
			return fmt.Errorf("non-base32 char %q in %q", r, s)
		}
	}
	return nil
}

func isCrockfordBase32(r rune) bool {
	switch {
	case r >= '0' && r <= '9':
		return true
	case r >= 'A' && r <= 'Z' && r != 'I' && r != 'L' && r != 'O' && r != 'U':
		return true
	case r >= 'a' && r <= 'z' && r != 'i' && r != 'l' && r != 'o' && r != 'u':
		return true
	}
	return false
}
```

- [ ] **Step 4:** Run tests to verify they pass

Run: `go test ./lib/skillruntime/...`
Expected: PASS, five tests

- [ ] **Step 5:** Commit

```bash
git add lib/skillruntime/handles.go lib/skillruntime/handles_test.go
git commit -m "feat(skillruntime): parse and format <sensor-id>:<run-id> handles"
```

---

## Task 5: `lib/skillruntime/boot.go`

**Files:**
- Create: `lib/skillruntime/boot.go`
- Test: `lib/skillruntime/boot_test.go`
- Reference: `internal/lifecycle/lifecycle_test.go:101` (newTestLifecycle), `internal/lifecycle/lifecycle.go:44` (Options)

This task wires the full bootstrap chain. The function must:
1. Load all sensors via `sensor.LoadDirectory(.harness/sensors)`
2. Load all fixtures via `fixture.LoadDirectory(.harness/fixtures)`
3. Walk `.harness/use-cases/*.yaml`, call `usecase.Load(data, fixtureStore)` per file, collect into `map[string]*usecase.UseCase`
4. Build `map[string]entrypoint.EntryPoint` by iterating use-case EntryPoints
5. Construct `*template.Resolver{Fixtures: fixtureStore, EntryPoints: entryPoints}`
6. Construct `*executor.Executor` with stores, resolver, and a `UseCaseLookup` closure
7. Construct `*lifecycle.Lifecycle` with `RuntimeRoot = HarnessDir/runtime`
8. Return all of it in a `*Booted` struct

- [ ] **Step 1:** Write the failing test

Create `lib/skillruntime/boot_test.go`:

```go
package skillruntime

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBootLifecycle_EmptyHarness verifies the function does not blow up
// on an .harness/ tree containing empty sensors/, fixtures/, and
// use-cases/ directories. This is the minimum smoke test.
func TestBootLifecycle_EmptyHarness(t *testing.T) {
	repo := t.TempDir()
	for _, sub := range []string{"sensors", "fixtures", "use-cases", "runtime"} {
		if err := os.MkdirAll(filepath.Join(repo, ".harness", sub), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	b, err := BootLifecycle(repo)
	if err != nil {
		t.Fatalf("BootLifecycle: %v", err)
	}
	defer func() {
		if cerr := b.Cleanup(); cerr != nil {
			t.Errorf("Cleanup: %v", cerr)
		}
	}()

	if b.Lifecycle == nil {
		t.Errorf("Lifecycle is nil")
	}
	if b.Sensors == nil {
		t.Errorf("Sensors is nil")
	}
	if b.Fixtures == nil {
		t.Errorf("Fixtures is nil")
	}
	if b.UseCases == nil {
		t.Errorf("UseCases is nil")
	}
}

func TestBootLifecycle_MissingHarness(t *testing.T) {
	repo := t.TempDir()
	_, err := BootLifecycle(repo)
	if err == nil {
		t.Errorf("expected error on missing .harness/")
	}
}
```

- [ ] **Step 2:** Run test to verify it fails

Run: `go test ./lib/skillruntime/...`
Expected: FAIL with "undefined: BootLifecycle"

- [ ] **Step 3:** Write minimal implementation

Create `lib/skillruntime/boot.go`:

```go
package skillruntime

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/lifecycle"
	"github.com/iurykrieger/lastro/internal/runtime/executor"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// HarnessVersion is the version string recorded in Handle entries by the
// constructed Lifecycle. Hardcoded for B5 sub-PR 1; replace with a
// build-time variable later.
const HarnessVersion = "0.1.0-b5"

// Booted is the bundle of objects every B5 skill script gets from
// BootLifecycle. The script keeps a reference to *Booted for the duration
// of the invocation and calls Cleanup before exit.
type Booted struct {
	Lifecycle   *lifecycle.Lifecycle
	Sensors     *sensor.Store
	Fixtures    *fixture.Store
	UseCases    map[string]*usecase.UseCase
	RuntimeRoot string // <repoRoot>/.harness/runtime — used by skills that need to walk run dirs
	Cleanup     func() error
}

// BootLifecycle loads .harness/{sensors,fixtures,use-cases} from disk and
// returns a configured *lifecycle.Lifecycle ready for RunSensor /
// StartSensor / StopSensor calls. Returns an error if .harness/ is
// missing or any sub-store fails to load.
func BootLifecycle(repoRoot string) (*Booted, error) {
	harnessDir := filepath.Join(repoRoot, ".harness")
	if info, err := os.Stat(harnessDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("skillruntime: .harness/ not found at %s", harnessDir)
	}

	sensorStore, err := loadSensors(filepath.Join(harnessDir, "sensors"))
	if err != nil {
		return nil, fmt.Errorf("skillruntime: load sensors: %w", err)
	}

	fixtureStore, err := loadFixtures(filepath.Join(harnessDir, "fixtures"))
	if err != nil {
		return nil, fmt.Errorf("skillruntime: load fixtures: %w", err)
	}

	useCases, err := loadUseCases(filepath.Join(harnessDir, "use-cases"), fixtureStore)
	if err != nil {
		return nil, fmt.Errorf("skillruntime: load use cases: %w", err)
	}

	entryPoints := collectEntryPoints(useCases)

	resolver := &template.Resolver{
		Fixtures:    fixtureStore,
		EntryPoints: entryPoints,
	}

	exec := executor.New(executor.Options{
		RepoRoot:     repoRoot,
		Resolver:     resolver,
		FixtureStore: fixtureStore,
		UseCaseLookup: func(sensorID string) (*usecase.UseCase, bool) {
			s, ok := sensorStore.LookupSensor(sensorID)
			if !ok {
				return nil, false
			}
			uc, ok := useCases[s.UseCaseID]
			return uc, ok
		},
	})

	runtimeRoot := filepath.Join(harnessDir, "runtime")
	if err := os.MkdirAll(runtimeRoot, 0o755); err != nil {
		return nil, fmt.Errorf("skillruntime: mkdir runtime: %w", err)
	}

	lc := lifecycle.New(lifecycle.Options{
		Sensors:     lifecycle.WrapSensorStore(sensorStore),
		Executor:    exec,
		RuntimeRoot: runtimeRoot,
		Version:     HarnessVersion,
	})

	return &Booted{
		Lifecycle:   lc,
		Sensors:     sensorStore,
		Fixtures:    fixtureStore,
		UseCases:    useCases,
		RuntimeRoot: runtimeRoot,
		Cleanup:     func() error { return nil },
	}, nil
}

// loadSensors returns an empty store if the directory is empty;
// sensor.LoadDirectory must tolerate that or we wrap.
func loadSensors(dir string) (*sensor.Store, error) {
	if !dirHasYAML(dir) {
		return sensor.NewStore()
	}
	return sensor.LoadDirectory(dir)
}

func loadFixtures(dir string) (*fixture.Store, error) {
	if !dirHasYAML(dir) {
		return fixture.NewStore()
	}
	return fixture.LoadDirectory(dir)
}

func loadUseCases(dir string, fixtures fixture.FixtureStore) (map[string]*usecase.UseCase, error) {
	out := map[string]*usecase.UseCase{}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return out, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		uc, err := usecase.Load(data, fixtures)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", e.Name(), err)
		}
		out[uc.ID] = uc
	}
	return out, nil
}

func collectEntryPoints(useCases map[string]*usecase.UseCase) map[string]entrypoint.EntryPoint {
	out := map[string]entrypoint.EntryPoint{}
	for _, uc := range useCases {
		for _, ep := range uc.EntryPoints {
			out[ep.ID] = ep
		}
	}
	return out
}

func dirHasYAML(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4:** Run tests to verify they pass

Run: `go test ./lib/skillruntime/...`
Expected: PASS, both tests

- [ ] **Step 5:** Verify the loader signatures match (read-only check, no edits)

Run: `grep -n "^func " internal/sensor/store.go internal/sensor/loader.go internal/fixture/store.go internal/fixture/loader.go internal/usecase/loader.go`
Expected output (key lines):
- `internal/sensor/store.go: func NewStore(sensors ...Sensor) (*Store, error)`
- `internal/sensor/loader.go: func LoadDirectory(path string) (*Store, error)`
- `internal/fixture/store.go: func NewStore(fixtures ...Fixture) (*Store, error)`
- `internal/fixture/loader.go: func LoadDirectory(path string) (*Store, error)`
- `internal/usecase/loader.go: func Load(data []byte, store fixture.FixtureStore) (*UseCase, error)`

If any signature differs from what `boot.go` uses, fix `boot.go` accordingly and re-run tests.

- [ ] **Step 6:** Commit

```bash
git add lib/skillruntime/boot.go lib/skillruntime/boot_test.go
git commit -m "feat(skillruntime): BootLifecycle wires .harness/ into *lifecycle.Lifecycle"
```

---

## Task 6: `skills/run-sensor/skill.md`

**Files:**
- Create: `skills/run-sensor/skill.md`

- [ ] **Step 1:** Write the skill markdown

Create `skills/run-sensor/skill.md`:

```markdown
---
name: run-sensor
description: Run a kind:assertion harness sensor synchronously. Returns the streamed signals followed by the terminal AggregateSignal. Exit non-zero on verdict=fail.
---

# /run-sensor

Synchronously run an assertion sensor and emit its signals + terminal
`AggregateSignal` as JSONL on stdout.

## Usage

```
/run-sensor <sensor-id>
```

`<sensor-id>` is the `id` field of a sensor YAML under `.harness/sensors/`.

## Output

- **stdout** — JSONL stream. One JSON object per line. Streamed individual
  `Signal`s first, then one terminal `AggregateSignal` as the final line.
- **stderr** — empty on success; a `ScriptError` envelope on script-level
  failure (sensor id not found, kind mismatch, runtime error).

## Exit codes

| Code | Meaning |
|---|---|
| 0 | `AggregateSignal.verdict == pass` |
| 1 | `AggregateSignal.verdict == fail` |
| 2 | `AggregateSignal.verdict == inconclusive` |
| 3 | Script-level error (bad argv, missing sensor, wrong kind, etc.) |

## Constraints

- The sensor's `kind` MUST be `assertion`. If it is `observational`, exit
  code 3 with `{"code":"wrong-kind","hint":"use /start-sensor"}` on stderr.
- The script blocks until the sensor terminates. For long-running
  observational sensors, use `/start-sensor` instead.

## Implementation

Wraps `lifecycle.RunSensor`. After the call returns, replays the
per-run `signals.jsonl` to stdout, then emits the terminal aggregate.

## Examples

Pass:

```
$ /run-sensor 01HMG12RX9N6Z8WJ3D6PNHVQXC
{"schema_version":"1.0.0","sensor_id":"01HMG…","verdict":"pass",…}
{"type":"aggregate","sensor_id":"01HMG…","verdict":"pass",…}
$ echo $?
0
```

Fail (with heal hint):

```
$ /run-sensor 01HMG12RXATAFM4N0F0X5Y4SGE
{"schema_version":"1.0.0","verdict":"fail",…,"heal_hint":{"summary":"…"}}
{"type":"aggregate","verdict":"fail","heal_hint":{…}}
$ echo $?
1
```
```

- [ ] **Step 2:** Verify the skill file is under 200 lines

Run: `wc -l skills/run-sensor/skill.md`
Expected: line count well under 100 (CLAUDE.md rule 4 synthesis trigger)

- [ ] **Step 3:** Commit

```bash
git add skills/run-sensor/skill.md
git commit -m "docs(skills): /run-sensor skill markdown"
```

---

## Task 7: `skills/run-sensor/scripts/main.go`

**Files:**
- Create: `skills/run-sensor/scripts/main.go`
- Test: `skills/run-sensor/scripts/main_test.go`
- Reference: `internal/lifecycle/lifecycle_test.go` (pattern for building fakesensor + assertion test)

- [ ] **Step 1:** Write the failing test

Create `skills/run-sensor/scripts/main_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var fakeSensorBin string

func TestMain(m *testing.M) {
	bin, err := buildFakeSensor()
	if err != nil {
		panic("build fakesensor: " + err.Error())
	}
	fakeSensorBin = bin
	defer os.Remove(bin)
	os.Exit(m.Run())
}

func buildFakeSensor() (string, error) {
	// fakesensor lives at <repo>/internal/testutil/fakesensor/main.go.
	// The skill script tests run from skills/run-sensor/scripts/, so the
	// relative path is ../../../internal/testutil/fakesensor/main.go.
	dir, err := os.MkdirTemp("", "fakesensor-runsensor-")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "fakesensor")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "../../../internal/testutil/fakesensor/main.go")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out, nil
}

// setupHarness writes a minimal .harness/ layout with one sensor.
// Returns the repo root.
func setupHarness(t *testing.T, sensorYAML string) string {
	t.Helper()
	root := t.TempDir()
	for _, sub := range []string{"sensors", "fixtures", "use-cases", "runtime"} {
		if err := os.MkdirAll(filepath.Join(root, ".harness", sub), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".harness", "sensors", "s.yaml"), []byte(sensorYAML), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return root
}

func TestRun_PassingAssertion(t *testing.T) {
	sensorYAML := `schema_version: 1.0.0
id: test-pass
use_case_id: test-uc
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses: [fake]
steps:
  - id: only
    run: "` + fakeSensorBin + ` signal pass"
`
	root := setupHarness(t, sensorYAML)
	var stdout, stderr bytes.Buffer
	code := run([]string{"run-sensor", "test-pass"}, nil, &stdout, &stderr, root)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, stderr.String())
	}
	// last line should be the terminal aggregate
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) == 0 {
		t.Fatalf("no stdout")
	}
	var agg map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &agg); err != nil {
		t.Fatalf("terminal line not JSON: %v (%q)", err, lines[len(lines)-1])
	}
	if agg["verdict"] != "pass" {
		t.Errorf("verdict = %v, want pass", agg["verdict"])
	}
}

func TestRun_SensorNotFound(t *testing.T) {
	root := setupHarness(t, "")
	// overwrite with empty
	os.Remove(filepath.Join(root, ".harness", "sensors", "s.yaml"))
	var stdout, stderr bytes.Buffer
	code := run([]string{"run-sensor", "no-such"}, nil, &stdout, &stderr, root)
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "sensor-not-found") && !strings.Contains(stderr.String(), "no-such") {
		t.Errorf("stderr missing context: %q", stderr.String())
	}
}

func TestRun_BadArgv(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"run-sensor"}, nil, &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}
```

- [ ] **Step 2:** Run test to verify it fails

Run: `go test ./skills/run-sensor/scripts/...`
Expected: FAIL with "undefined: run" or "no Go files"

- [ ] **Step 3:** Write minimal implementation

Create `skills/run-sensor/scripts/main.go`:

```go
// Command run-sensor is the Go binary backing the /run-sensor skill.
// See skills/run-sensor/skill.md for the contract.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/lifecycle"
	"github.com/iurykrieger/lastro/lib/skillio"
	"github.com/iurykrieger/lastro/lib/skillruntime"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		skillio.EmitError(os.Stderr, "cwd-failed", err.Error(), nil)
		os.Exit(skillio.ExitScriptError)
	}
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr, cwd))
}

// run is the testable entry point. args is the full os.Args slice; cwd
// is the working directory used for repo-root discovery.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected sensor-id as first argument", nil)
		return skillio.ExitScriptError
	}
	sensorID := args[1]

	repoRoot, err := skillio.FindRepoRoot(cwd)
	if err != nil {
		skillio.EmitError(stderr, "repo-root-not-found", err.Error(), nil)
		return skillio.ExitScriptError
	}

	b, err := skillruntime.BootLifecycle(repoRoot)
	if err != nil {
		skillio.EmitError(stderr, "boot-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	defer func() { _ = b.Cleanup() }()

	s, ok := b.Sensors.LookupSensor(sensorID)
	if !ok {
		skillio.EmitError(stderr, "sensor-not-found", fmt.Sprintf("no sensor %q in .harness/sensors/", sensorID), map[string]any{"sensor_id": sensorID})
		return skillio.ExitScriptError
	}
	if s.Kind == enums.KindObservational {
		skillio.EmitError(stderr, "wrong-kind", "use /start-sensor for observational sensors", map[string]any{"sensor_id": sensorID, "kind": string(s.Kind)})
		return skillio.ExitScriptError
	}

	ctx := context.Background()
	agg, err := b.Lifecycle.RunSensor(ctx, sensorID, nil)
	if err != nil {
		if errors.Is(err, lifecycle.ErrSensorNotFound) {
			skillio.EmitError(stderr, "sensor-not-found", err.Error(), map[string]any{"sensor_id": sensorID})
			return skillio.ExitScriptError
		}
		skillio.EmitError(stderr, "run-failed", err.Error(), map[string]any{"sensor_id": sensorID})
		return skillio.ExitScriptError
	}

	if err := replaySignals(b, sensorID, stdout); err != nil {
		skillio.EmitError(stderr, "replay-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	if err := skillio.EmitJSON(stdout, agg); err != nil {
		skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	return skillio.ExitCodeForVerdict(agg.Verdict)
}

// replaySignals streams the most recent per-run signals.jsonl to stdout.
// Best effort: missing file is not an error (single-shot sensors may
// emit zero streamed signals). The terminal aggregate is appended by
// the caller.
//
// Run-dir discovery: <RuntimeRoot>/<sensor-id>/<run-id>/signals.jsonl.
// ULID run ids sort lexically by time, so the alphabetically-greatest
// subdir is the most recent run — which is the one RunSensor just
// completed (RunSensor blocks until the run finishes).
func replaySignals(b *skillruntime.Booted, sensorID string, stdout io.Writer) error {
	sensorDir := filepath.Join(b.RuntimeRoot, sensorID)
	entries, err := os.ReadDir(sensorDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var latest string
	for _, e := range entries {
		if e.IsDir() && (latest == "" || e.Name() > latest) {
			latest = e.Name()
		}
	}
	if latest == "" {
		return nil
	}
	signalsPath := filepath.Join(sensorDir, latest, "signals.jsonl")
	f, err := os.Open(signalsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // allow long lines
	for scanner.Scan() {
		if _, err := stdout.Write(scanner.Bytes()); err != nil {
			return err
		}
		if _, err := stdout.Write([]byte{'\n'}); err != nil {
			return err
		}
	}
	return scanner.Err()
}
```

- [ ] **Step 4:** Run tests to verify they pass

Run: `go test ./skills/run-sensor/scripts/... -count=1`
Expected: PASS, three tests

- [ ] **Step 5:** Race test

Run: `go test -race ./skills/run-sensor/scripts/... -count=1`
Expected: PASS, no race warnings

- [ ] **Step 6:** Commit

```bash
git add skills/run-sensor/scripts/main.go skills/run-sensor/scripts/main_test.go
git commit -m "feat(skills/run-sensor): scripts/main wrapping lifecycle.RunSensor"
```

---

## Task 8: `skills/start-sensor/skill.md`

**Files:**
- Create: `skills/start-sensor/skill.md`

- [ ] **Step 1:** Write the skill markdown

Create `skills/start-sensor/skill.md`:

```markdown
---
name: start-sensor
description: Spawn a kind:observational harness sensor as a long-running watcher and return its handle. Pair with /stop-sensor.
---

# /start-sensor

Spawn an observational sensor's watcher process and return immediately
with the handle the caller passes to `/stop-sensor` later.

## Usage

```
/start-sensor <sensor-id>
```

`<sensor-id>` is the `id` field of a sensor YAML under `.harness/sensors/`.
The sensor's `kind` MUST be `observational`.

## Output

- **stdout** — single-line JSON object:
  `{"handle":"<sensor-id>:<run-id>","run_dir":"<path>","pid":<int>}`
- **stderr** — empty on success; `ScriptError` envelope on failure.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Watcher spawned; handle written to stdout |
| 3 | Script-level error (sensor not found, wrong kind, spawn failed) |

The watcher process is detached from this skill's process — exiting the
skill does not kill the watcher. Use `/stop-sensor <handle>` to terminate.

## Constraints

- The sensor's `kind` MUST be `observational`. For `assertion` sensors,
  use `/run-sensor`.
- Expected observations are read from the sensor's YAML
  (`expected_observations` field; see B4). If absent, the script passes
  `nil` and the watcher reports `completeness` as best-effort.

## Examples

```
$ /start-sensor 01HMG12RX9N6Z8WJ3D6PNHVQXC
{"handle":"01HMG12RX9N6Z8WJ3D6PNHVQXC:01HMG12S4ABCDEFGH...","run_dir":"…","pid":12345}
$ echo $?
0
```
```

- [ ] **Step 2:** Commit

```bash
git add skills/start-sensor/skill.md
git commit -m "docs(skills): /start-sensor skill markdown"
```

---

## Task 9: `skills/start-sensor/scripts/main.go`

**Files:**
- Create: `skills/start-sensor/scripts/main.go`
- Test: `skills/start-sensor/scripts/main_test.go`

- [ ] **Step 1:** Write the failing test

Create `skills/start-sensor/scripts/main_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var fakeSensorBin string

func TestMain(m *testing.M) {
	bin, err := buildFakeSensor()
	if err != nil {
		panic("build fakesensor: " + err.Error())
	}
	fakeSensorBin = bin
	defer os.Remove(bin)
	os.Exit(m.Run())
}

func buildFakeSensor() (string, error) {
	dir, err := os.MkdirTemp("", "fakesensor-startsensor-")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "fakesensor")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "../../../internal/testutil/fakesensor/main.go")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out, nil
}

func setupHarness(t *testing.T, sensorYAML string) string {
	t.Helper()
	root := t.TempDir()
	for _, sub := range []string{"sensors", "fixtures", "use-cases", "runtime"} {
		if err := os.MkdirAll(filepath.Join(root, ".harness", sub), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".harness", "sensors", "s.yaml"), []byte(sensorYAML), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return root
}

func TestRun_RejectsAssertionKind(t *testing.T) {
	sensorYAML := `schema_version: 1.0.0
id: test-assertion
use_case_id: test-uc
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses: [fake]
steps:
  - id: only
    run: "` + fakeSensorBin + ` signal pass"
`
	root := setupHarness(t, sensorYAML)
	var stdout, stderr bytes.Buffer
	code := run([]string{"start-sensor", "test-assertion"}, nil, &stdout, &stderr, root)
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "wrong-kind") {
		t.Errorf("stderr missing wrong-kind: %q", stderr.String())
	}
}

func TestRun_ObservationalSpawnsAndEmitsHandle(t *testing.T) {
	// fakesensor "spawn observational" emits 1 signal then waits for stop.
	sensorYAML := `schema_version: 1.0.0
id: test-obs
use_case_id: test-uc
angle: logs
kind: observational
nature: computational
output_type: stream
uses: [fake]
steps:
  - id: watch
    run: "` + fakeSensorBin + ` observe-wait"
`
	root := setupHarness(t, sensorYAML)
	var stdout, stderr bytes.Buffer
	code := run([]string{"start-sensor", "test-obs"}, nil, &stdout, &stderr, root)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &out); err != nil {
		t.Fatalf("stdout not JSON: %v (%q)", err, stdout.String())
	}
	handle, _ := out["handle"].(string)
	if !strings.Contains(handle, ":") {
		t.Errorf("handle missing colon: %q", handle)
	}
	// Cleanup: the test must terminate the spawned watcher to avoid leaked processes.
	// We use the PID from the output to kill it.
	if pid, ok := out["pid"].(float64); ok {
		_ = killPID(int(pid))
	}
}
```

> NOTE: `killPID` is a small platform helper. Add it inline:

```go
func killPID(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
```

> NOTE: the `observe-wait` subcommand of fakesensor may not exist yet. Step 2 verifies whether it does; if not, Step 3 documents the workaround.

- [ ] **Step 2:** Verify `fakesensor` supports the `observe-wait` subcommand

Run: `grep -n "observe-wait\|wait_for_stop" internal/testutil/fakesensor/main.go`
Expected: at least one match showing the subcommand wiring.

If no match: the `observational` test path needs a different fakesensor subcommand (e.g., `observe` that emits N signals over time and exits). Read `internal/testutil/fakesensor/main.go`'s `main()` switch statement and pick an existing observational mode. Update the test's `run:` line accordingly.

- [ ] **Step 3:** Run test to verify it fails

Run: `go test ./skills/start-sensor/scripts/... -count=1`
Expected: FAIL with "undefined: run"

- [ ] **Step 4:** Write minimal implementation

Create `skills/start-sensor/scripts/main.go`:

```go
// Command start-sensor backs the /start-sensor skill.
// See skills/start-sensor/skill.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/lifecycle"
	"github.com/iurykrieger/lastro/lib/skillio"
	"github.com/iurykrieger/lastro/lib/skillruntime"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		skillio.EmitError(os.Stderr, "cwd-failed", err.Error(), nil)
		os.Exit(skillio.ExitScriptError)
	}
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr, cwd))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected sensor-id as first argument", nil)
		return skillio.ExitScriptError
	}
	sensorID := args[1]

	repoRoot, err := skillio.FindRepoRoot(cwd)
	if err != nil {
		skillio.EmitError(stderr, "repo-root-not-found", err.Error(), nil)
		return skillio.ExitScriptError
	}

	b, err := skillruntime.BootLifecycle(repoRoot)
	if err != nil {
		skillio.EmitError(stderr, "boot-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	defer func() { _ = b.Cleanup() }()

	s, ok := b.Sensors.LookupSensor(sensorID)
	if !ok {
		skillio.EmitError(stderr, "sensor-not-found", fmt.Sprintf("no sensor %q", sensorID), map[string]any{"sensor_id": sensorID})
		return skillio.ExitScriptError
	}
	if s.Kind != enums.KindObservational {
		skillio.EmitError(stderr, "wrong-kind", "use /run-sensor for assertion sensors", map[string]any{"sensor_id": sensorID, "kind": string(s.Kind)})
		return skillio.ExitScriptError
	}

	// F3: ExpectedObservations is currently not a field on Sensor; pass nil.
	// When B4 adds it, populate from s here.
	var expectedObs []string

	h, err := b.Lifecycle.StartSensor(context.Background(), sensorID, expectedObs)
	if err != nil {
		if errors.Is(err, lifecycle.ErrAssertionSensor) {
			skillio.EmitError(stderr, "wrong-kind", err.Error(), nil)
			return skillio.ExitScriptError
		}
		skillio.EmitError(stderr, "start-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	out := map[string]any{
		"handle":  skillruntime.FormatHandle(h.SensorID, h.RunID),
		"run_dir": h.RunDir,
		"pid":     h.PID,
	}
	if err := skillio.EmitJSON(stdout, out); err != nil {
		skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	return skillio.ExitPass
}
```

- [ ] **Step 5:** Run tests to verify they pass

Run: `go test ./skills/start-sensor/scripts/... -count=1`
Expected: PASS, two tests.

If `TestRun_ObservationalSpawnsAndEmitsHandle` fails because fakesensor lacks the right subcommand, either extend fakesensor (preferred — file follow-up) or adjust the test's `run:` line to use an existing observational mode that emits at least one signal then waits.

- [ ] **Step 6:** Race test

Run: `go test -race ./skills/start-sensor/scripts/... -count=1`
Expected: PASS

- [ ] **Step 7:** Commit

```bash
git add skills/start-sensor/scripts/main.go skills/start-sensor/scripts/main_test.go
git commit -m "feat(skills/start-sensor): scripts/main wrapping lifecycle.StartSensor"
```

---

## Task 10: `skills/stop-sensor/skill.md`

**Files:**
- Create: `skills/stop-sensor/skill.md`

- [ ] **Step 1:** Write the skill markdown

Create `skills/stop-sensor/skill.md`:

```markdown
---
name: stop-sensor
description: Terminate a running observational sensor identified by its handle. Emits the terminal AggregateSignal.
---

# /stop-sensor

Stop a previously-started observational sensor and emit its terminal
`AggregateSignal`.

## Usage

```
/stop-sensor <sensor-id>:<run-id>
```

The handle is the value `/start-sensor` printed under `"handle"`. Both
halves are ULIDs (26-char Crockford base32).

## Output

- **stdout** — single-line JSON: the terminal `AggregateSignal`.
- **stderr** — empty on success; `ScriptError` envelope on failure.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Sensor terminated cleanly with `verdict == pass` |
| 1 | `verdict == fail` (e.g., expected observations missing) |
| 2 | `verdict == inconclusive` (timeout, partial completion) |
| 3 | Script-level error (bad handle, sensor not found) |

## Constraints

- Works whether the watcher is still running (sends SIGTERM, waits for
  aggregate.json) or already terminated (reads aggregate.json from disk).
- The handle must be well-formed `<26-char>:<26-char>`. Malformed handles
  exit 3 with `{"code":"bad-handle"}`.

## Examples

```
$ /stop-sensor 01HMG12RX9N6Z8WJ3D6PNHVQXC:01HMG12S4ABCDEFGH...
{"type":"aggregate","verdict":"pass",…}
$ echo $?
0
```
```

- [ ] **Step 2:** Commit

```bash
git add skills/stop-sensor/skill.md
git commit -m "docs(skills): /stop-sensor skill markdown"
```

---

## Task 11: `skills/stop-sensor/scripts/main.go`

**Files:**
- Create: `skills/stop-sensor/scripts/main.go`
- Test: `skills/stop-sensor/scripts/main_test.go`

- [ ] **Step 1:** Write the failing test

Create `skills/stop-sensor/scripts/main_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_BadHandle_NoColon(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"stop-sensor", "no-colon-here"}, nil, &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "bad-handle") {
		t.Errorf("stderr missing bad-handle: %q", stderr.String())
	}
}

func TestRun_BadHandle_WrongLength(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"stop-sensor", "short:short"}, nil, &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}

func TestRun_BadArgv(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"stop-sensor"}, nil, &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}
```

> NOTE: a positive-path test (terminate a real running sensor) would require coordinating with `/start-sensor` from another goroutine in the test. That's the integration smoke test in Task 15 — we cover happy-path there. The unit test covers the argv and handle-parsing branches.

- [ ] **Step 2:** Run test to verify it fails

Run: `go test ./skills/stop-sensor/scripts/... -count=1`
Expected: FAIL with "undefined: run"

- [ ] **Step 3:** Write minimal implementation

Create `skills/stop-sensor/scripts/main.go`:

```go
// Command stop-sensor backs the /stop-sensor skill.
// See skills/stop-sensor/skill.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/iurykrieger/lastro/internal/lifecycle"
	"github.com/iurykrieger/lastro/lib/skillio"
	"github.com/iurykrieger/lastro/lib/skillruntime"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		skillio.EmitError(os.Stderr, "cwd-failed", err.Error(), nil)
		os.Exit(skillio.ExitScriptError)
	}
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr, cwd))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected '<sensor-id>:<run-id>' as first argument", nil)
		return skillio.ExitScriptError
	}
	handle := args[1]

	sensorID, runID, err := skillruntime.ParseHandle(handle)
	if err != nil {
		skillio.EmitError(stderr, "bad-handle", err.Error(), map[string]any{"input": handle})
		return skillio.ExitScriptError
	}

	repoRoot, err := skillio.FindRepoRoot(cwd)
	if err != nil {
		skillio.EmitError(stderr, "repo-root-not-found", err.Error(), nil)
		return skillio.ExitScriptError
	}

	b, err := skillruntime.BootLifecycle(repoRoot)
	if err != nil {
		skillio.EmitError(stderr, "boot-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	defer func() { _ = b.Cleanup() }()

	h, err := b.Lifecycle.LoadHandle(sensorID, runID)
	if err != nil {
		if errors.Is(err, lifecycle.ErrSensorNotFound) {
			skillio.EmitError(stderr, "sensor-not-found", fmt.Sprintf("no in-flight handle for %s:%s", sensorID, runID), map[string]any{"handle": handle})
			return skillio.ExitScriptError
		}
		skillio.EmitError(stderr, "load-handle-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	agg, err := b.Lifecycle.StopSensor(context.Background(), h)
	if err != nil {
		skillio.EmitError(stderr, "stop-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	if err := skillio.EmitJSON(stdout, agg); err != nil {
		skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	return skillio.ExitCodeForVerdict(agg.Verdict)
}
```

- [ ] **Step 4:** Run tests to verify they pass

Run: `go test ./skills/stop-sensor/scripts/... -count=1`
Expected: PASS, three tests

- [ ] **Step 5:** Commit

```bash
git add skills/stop-sensor/scripts/main.go skills/stop-sensor/scripts/main_test.go
git commit -m "feat(skills/stop-sensor): scripts/main wrapping lifecycle.StopSensor"
```

---

## Task 12: `skills/tail-sensor-signals/skill.md`

**Files:**
- Create: `skills/tail-sensor-signals/skill.md`

- [ ] **Step 1:** Write the skill markdown

Create `skills/tail-sensor-signals/skill.md`:

```markdown
---
name: tail-sensor-signals
description: Stream signals from a running or completed observational sensor. Supports --follow and --since=<n> for resumption.
---

# /tail-sensor-signals

Read the per-run `signals.jsonl` for a sensor and emit it to stdout.
Pure file reader — no runtime API call. Use this between `/start-sensor`
and `/stop-sensor` to watch what an observational sensor is observing.

## Usage

```
/tail-sensor-signals <sensor-id>:<run-id> [--follow] [--since=<n>]
```

- `--follow`: block and stream new lines as the watcher emits them. Exits
  when the sensor leaves `.harness/runtime/running_sensors.json` AND no
  new bytes arrive for 1 second.
- `--since=<n>`: start at signal number `n` (1-indexed). Lets the LLM
  resume after a previous tail without re-reading.

## Output

- **stdout** — each `signals.jsonl` line, one per line.
- **stderr** — empty on success; `ScriptError` envelope on failure.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Graceful EOF (snapshot done, or follow exited cleanly) |
| 3 | Bad handle, unreadable file, or other script error |

This skill does not opine on the sensor's verdict — that's `/stop-sensor`'s job.

## Examples

Snapshot:

```
$ /tail-sensor-signals 01HMG…:01HMG…
{"verdict":"pass",…}
{"verdict":"pass",…}
$ echo $?
0
```

Follow:

```
$ /tail-sensor-signals 01HMG…:01HMG… --follow
{"verdict":"pass",…}      ← existing line
{"verdict":"pass",…}      ← new line as watcher emits it
^C    ← or wait for /stop-sensor from a sibling process
$ echo $?
0
```

Resume:

```
$ /tail-sensor-signals 01HMG…:01HMG… --since=3
{"verdict":"pass",…}      ← 3rd line onward
```
```

- [ ] **Step 2:** Commit

```bash
git add skills/tail-sensor-signals/skill.md
git commit -m "docs(skills): /tail-sensor-signals skill markdown"
```

---

## Task 13: `skills/tail-sensor-signals/scripts/main.go` — snapshot mode

**Files:**
- Create: `skills/tail-sensor-signals/scripts/main.go`
- Test: `skills/tail-sensor-signals/scripts/main_test.go`

- [ ] **Step 1:** Write the failing test (snapshot mode only)

Create `skills/tail-sensor-signals/scripts/main_test.go`:

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validSensorID = "01HMG12RX9N6Z8WJ3D6PNHVQXC"
const validRunID = "01HMG12RXATAFM4N0F0X5Y4SGE"

func setupSignalsFile(t *testing.T, lines []string) string {
	t.Helper()
	root := t.TempDir()
	runDir := filepath.Join(root, ".harness", "runtime", validSensorID, validRunID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "signals.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return root
}

func TestRun_Snapshot_AllLines(t *testing.T) {
	root := setupSignalsFile(t, []string{`{"verdict":"pass","n":1}`, `{"verdict":"pass","n":2}`, `{"verdict":"pass","n":3}`})
	var stdout, stderr bytes.Buffer
	code := run([]string{"tail-sensor-signals", validSensorID + ":" + validRunID}, nil, &stdout, &stderr, root)
	if code != 0 {
		t.Fatalf("exit = %d; stderr=%s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("got %d lines, want 3 (%q)", len(lines), stdout.String())
	}
}

func TestRun_Snapshot_Since(t *testing.T) {
	root := setupSignalsFile(t, []string{`{"n":1}`, `{"n":2}`, `{"n":3}`, `{"n":4}`})
	var stdout, stderr bytes.Buffer
	code := run([]string{"tail-sensor-signals", validSensorID + ":" + validRunID, "--since=3"}, nil, &stdout, &stderr, root)
	if code != 0 {
		t.Fatalf("exit = %d; stderr=%s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("got %d lines, want 2 (lines 3 and 4); stdout=%q", len(lines), stdout.String())
	}
	if !strings.Contains(stdout.String(), `"n":3`) || !strings.Contains(stdout.String(), `"n":4`) {
		t.Errorf("stdout missing expected lines: %q", stdout.String())
	}
}

func TestRun_BadHandle(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"tail-sensor-signals", "garbage"}, nil, &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
}

func TestRun_MissingFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".harness"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"tail-sensor-signals", validSensorID + ":" + validRunID}, nil, &stdout, &stderr, root)
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
}
```

- [ ] **Step 2:** Run test to verify it fails

Run: `go test ./skills/tail-sensor-signals/scripts/... -count=1`
Expected: FAIL with "undefined: run"

- [ ] **Step 3:** Write minimal implementation (snapshot only)

Create `skills/tail-sensor-signals/scripts/main.go`:

```go
// Command tail-sensor-signals backs the /tail-sensor-signals skill.
// See skills/tail-sensor-signals/skill.md.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/iurykrieger/lastro/lib/skillio"
	"github.com/iurykrieger/lastro/lib/skillruntime"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		skillio.EmitError(os.Stderr, "cwd-failed", err.Error(), nil)
		os.Exit(skillio.ExitScriptError)
	}
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr, cwd))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected '<sensor-id>:<run-id>' as first argument", nil)
		return skillio.ExitScriptError
	}
	handle := args[1]

	fs := flag.NewFlagSet("tail-sensor-signals", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // suppress default error spam; we handle errors ourselves
	follow := fs.Bool("follow", false, "tail new signals as they arrive")
	since := fs.Int("since", 0, "skip the first N lines (1-indexed; 0 = no skip)")
	if err := fs.Parse(args[2:]); err != nil {
		skillio.EmitError(stderr, "bad-argv", err.Error(), nil)
		return skillio.ExitScriptError
	}

	sensorID, runID, err := skillruntime.ParseHandle(handle)
	if err != nil {
		skillio.EmitError(stderr, "bad-handle", err.Error(), map[string]any{"input": handle})
		return skillio.ExitScriptError
	}

	repoRoot, err := skillio.FindRepoRoot(cwd)
	if err != nil {
		skillio.EmitError(stderr, "repo-root-not-found", err.Error(), nil)
		return skillio.ExitScriptError
	}

	signalsPath := filepath.Join(skillio.HarnessDir(repoRoot), "runtime", sensorID, runID, "signals.jsonl")

	if *follow {
		// Implemented in Task 14.
		skillio.EmitError(stderr, "not-implemented", "--follow is implemented in the next task", nil)
		return skillio.ExitScriptError
	}

	emitted, err := snapshot(signalsPath, *since, stdout)
	if err != nil {
		skillio.EmitError(stderr, "read-failed", err.Error(), map[string]any{"path": signalsPath})
		return skillio.ExitScriptError
	}
	_ = emitted
	return skillio.ExitPass
}

// snapshot reads signals.jsonl from the beginning, skipping the first
// `since` lines if since > 0, and writes the rest to stdout (each
// followed by '\n'). Returns the number of lines emitted.
func snapshot(path string, since int, stdout io.Writer) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	idx := 0
	emitted := 0
	for scanner.Scan() {
		idx++
		if idx <= since {
			continue
		}
		if _, err := stdout.Write(scanner.Bytes()); err != nil {
			return emitted, err
		}
		if _, err := stdout.Write([]byte{'\n'}); err != nil {
			return emitted, err
		}
		emitted++
	}
	return emitted, scanner.Err()
}
```

- [ ] **Step 4:** Run tests to verify they pass (the `--follow` test isn't here yet)

Run: `go test ./skills/tail-sensor-signals/scripts/... -count=1`
Expected: PASS, four tests

- [ ] **Step 5:** Commit

```bash
git add skills/tail-sensor-signals/scripts/main.go skills/tail-sensor-signals/scripts/main_test.go
git commit -m "feat(skills/tail-sensor-signals): snapshot mode + --since"
```

---

## Task 14: `skills/tail-sensor-signals/scripts/main.go` — follow mode

**Files:**
- Modify: `skills/tail-sensor-signals/scripts/main.go`
- Modify: `skills/tail-sensor-signals/scripts/main_test.go`

- [ ] **Step 1:** Append the failing follow-mode test

Add to `skills/tail-sensor-signals/scripts/main_test.go`:

```go
func TestRun_Follow_ExitsWhenSensorTerminates(t *testing.T) {
	root := setupSignalsFile(t, []string{`{"n":1}`, `{"n":2}`})
	// No entry in running_sensors.json — follow should drain and exit.
	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"tail-sensor-signals", validSensorID + ":" + validRunID, "--follow"}, nil, &stdout, &stderr, root)
	}()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit = %d, want 0; stderr=%s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), `"n":1`) || !strings.Contains(stdout.String(), `"n":2`) {
			t.Errorf("stdout missing existing lines: %q", stdout.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("follow did not exit within 5s; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRun_Follow_PicksUpNewLines(t *testing.T) {
	root := setupSignalsFile(t, []string{`{"n":1}`})
	runDir := filepath.Join(root, ".harness", "runtime", validSensorID, validRunID)

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"tail-sensor-signals", validSensorID + ":" + validRunID, "--follow"}, nil, &stdout, &stderr, root)
	}()

	// Give the follower a moment to read the existing line, then append.
	time.Sleep(400 * time.Millisecond)
	f, err := os.OpenFile(filepath.Join(runDir, "signals.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	_, _ = f.WriteString(`{"n":2}` + "\n")
	_ = f.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("follow did not exit within 5s; stdout=%q", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"n":2`) {
		t.Errorf("stdout missing appended line: %q", stdout.String())
	}
}
```

Add to the existing imports at the top of the test file:

```go
import (
	// existing imports …
	"time"
)
```

- [ ] **Step 2:** Run tests — the new ones should fail with "not-implemented"

Run: `go test ./skills/tail-sensor-signals/scripts/... -count=1 -run Follow`
Expected: FAIL with stderr containing `not-implemented`

- [ ] **Step 3:** Replace the `--follow` branch in `main.go`

Edit `skills/tail-sensor-signals/scripts/main.go`. Replace the block:

```go
	if *follow {
		// Implemented in Task 14.
		skillio.EmitError(stderr, "not-implemented", "--follow is implemented in the next task", nil)
		return skillio.ExitScriptError
	}
```

with:

```go
	if *follow {
		if err := follow_(signalsPath, *since, sensorID, runID, skillio.HarnessDir(repoRoot), stdout); err != nil {
			skillio.EmitError(stderr, "follow-failed", err.Error(), map[string]any{"path": signalsPath})
			return skillio.ExitScriptError
		}
		return skillio.ExitPass
	}
```

And append the `follow_` function (named with trailing underscore to avoid clashing with the `follow` flag variable):

```go
import (
	// existing imports …
	"encoding/json"
	"time"
)

// follow_ tails signalsPath: emits existing lines (after `since`), then
// polls for new bytes every 200ms until the sensor leaves
// running_sensors.json AND no new bytes have arrived for 1s.
func follow_(signalsPath string, since int, sensorID, runID, harnessDir string, stdout io.Writer) error {
	// First emit the existing snapshot.
	emitted, err := snapshot(signalsPath, since, stdout)
	if err != nil {
		return err
	}
	_ = emitted // line count tracking lives in this function from here on

	// Reopen for incremental reads.
	f, err := os.Open(signalsPath)
	if err != nil {
		return err
	}
	defer f.Close()
	// Seek to end of what snapshot already emitted.
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	const pollInterval = 200 * time.Millisecond
	const quietWindow = 1 * time.Second
	registryPath := filepath.Join(harnessDir, "runtime", "running_sensors.json")
	var lastEmittedAt time.Time
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if _, werr := stdout.Write([]byte(line)); werr != nil {
				return werr
			}
			lastEmittedAt = time.Now()
		}
		if err == nil {
			// Read another line on the next iteration.
			continue
		}
		if err != io.EOF {
			return err
		}
		// EOF reached: decide whether to keep waiting.
		alive := isSensorInRegistry(registryPath, sensorID, runID)
		if !alive && time.Since(lastEmittedAt) > quietWindow {
			return nil
		}
		time.Sleep(pollInterval)
	}
}

// isSensorInRegistry returns true if running_sensors.json contains an
// entry for (sensorID, runID). Missing file or parse error → false.
func isSensorInRegistry(path, sensorID, runID string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc struct {
		Entries []struct {
			SensorID string `json:"sensor_id"`
			RunID    string `json:"run_id"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return false
	}
	for _, e := range doc.Entries {
		if e.SensorID == sensorID && e.RunID == runID {
			return true
		}
	}
	return false
}
```

> NOTE on `lastEmittedAt`: when the loop starts and `lastEmittedAt` is the zero value, `time.Since(lastEmittedAt) > quietWindow` is trivially true. The check still gates on `alive` being false. If the registry file does not exist at the very start (the common case in tests), the function exits as soon as it drains the file — which is the desired "no sensor running, just print what's there" behavior.

- [ ] **Step 4:** Run all tests

Run: `go test ./skills/tail-sensor-signals/scripts/... -count=1`
Expected: PASS, six tests

- [ ] **Step 5:** Race test

Run: `go test -race ./skills/tail-sensor-signals/scripts/... -count=1`
Expected: PASS

- [ ] **Step 6:** Commit

```bash
git add skills/tail-sensor-signals/scripts/main.go skills/tail-sensor-signals/scripts/main_test.go
git commit -m "feat(skills/tail-sensor-signals): --follow with registry-aware termination"
```

---

## Task 15: End-to-end acceptance smoke test

**Files:**
- Create: `skills/integration_test.go`

This test asserts the four skills cooperate: `/run-sensor` for an assertion sensor; `/start-sensor` + `/tail-sensor-signals --follow` + `/stop-sensor` for an observational sensor.

- [ ] **Step 1:** Write the integration test

Create `skills/integration_test.go`:

```go
//go:build integration

// Run with: go test -tags=integration ./skills/...
package skills_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// repoRoot walks up from the test working directory to find the lastro
// repo root (the dir containing go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), ".."))
}

func goRun(t *testing.T, ctx context.Context, scriptDir string, args ...string) (string, string, int) {
	t.Helper()
	full := append([]string{"run", "./" + scriptDir}, args...)
	cmd := exec.CommandContext(ctx, "go", full...)
	cmd.Dir = repoRoot(t)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("go run failed (not ExitError): %v; stderr=%s", err, stderr.String())
	}
	return stdout.String(), stderr.String(), code
}

func TestSkills_RunSensor_PassingAssertion(t *testing.T) {
	// Assumes the user has set up a known-good .harness/ on disk for this
	// integration test, OR we create one inline. For B5 sub-PR 1, we
	// create one inline because no dogfood examples exist yet.
	t.Skip("Integration test stub. Populate with a t.TempDir() harness layout including a known-passing assertion sensor + use case + fixture. Build fakesensor binary, write sensor YAML referencing it. Run /run-sensor and assert exit=0, stdout last line is aggregate with verdict=pass. Wire into CI with -tags=integration only.")
}

func TestSkills_StartTailStop_Observational(t *testing.T) {
	t.Skip("Integration test stub. Populate with a t.TempDir() harness layout for a known-good observational sensor. Spawn /start-sensor, capture handle. In parallel: /tail-sensor-signals --follow and /stop-sensor handle. Assert tail exits within 1.5s of stop and emitted at least 1 signal.")
}

// Compile-time guards that the test file participates in `go build -tags=integration`.
var (
	_ = json.Marshal
	_ = strings.TrimSpace
	_ = time.Now
)
```

- [ ] **Step 2:** Verify the file compiles under the integration tag

Run: `go test -tags=integration -run nothing ./skills/...`
Expected: `ok` (no tests run; we only need the file to compile)

- [ ] **Step 3:** Commit the stub

```bash
git add skills/integration_test.go
git commit -m "test(skills): integration smoke test stub (build-tagged)"
```

> NOTE: the integration tests are intentionally stubs in this sub-PR. Filling them in requires real .harness/ fixtures and dogfood material; that comes in B7. The unit tests in Tasks 7/9/11/13/14 already cover the per-script acceptance criteria from spec §11.1. Wiring the integration body into CI is a follow-up for B7.

---

## Task 16: Open the pull request

**Files:** none (git only)

- [ ] **Step 1:** Run the full test suite

Run: `go test -race ./...`
Expected: PASS across all packages, no race warnings.

- [ ] **Step 2:** Push the branch

Run: `git push -u origin feat/b5-lifecycle-wrappers`
Expected: `* [new branch] feat/b5-lifecycle-wrappers -> feat/b5-lifecycle-wrappers`

- [ ] **Step 3:** Open the PR (use `gh.exe` at the absolute path noted in memory)

Run:

```
"/c/Program Files/GitHub CLI/gh.exe" pr create --title "B5 sub-PR 1: lifecycle quartet skill wrappers" --body "$(cat <<'EOF'
## Summary

- Introduces `lib/skillio/` and `lib/skillruntime/` shared by every harness skill (B4 + B5)
- Implements four B5 skills: `/run-sensor`, `/start-sensor`, `/stop-sensor`, `/tail-sensor-signals`
- Each skill is a `go run`-invoked Go binary under `skills/<name>/scripts/`
- Wraps the B2 `internal/lifecycle` API surface; no new runtime mechanics

## Spec reference

[B5 design doc §11.1 — Sub-PR 1 acceptance criteria](docs/superpowers/specs/2026-05-25-b5-skill-wrappers-design.md#111-sub-pr-1--lifecycle-quartet)

## Test plan

- [x] `go test -race ./...` clean
- [x] `lib/skillio` unit tests cover exit codes, error envelope, repo discovery
- [x] `lib/skillruntime` unit tests cover handle parsing + BootLifecycle smoke
- [x] Each skill's `scripts/main_test.go` exercises happy path + argv/handle error paths
- [ ] Integration tests (`-tags=integration`) are stubs; filled in B7 against real `.harness/` dogfood material

## Out of scope (follow-ups)

- `/validate-use-case` — B5 sub-PR 2
- `/heal` + post-tool-use hook — B5 sub-PR 3 (blocked on B3)
- `Sensor.ExpectedObservations` field propagation — B4 follow-up (F3 in design doc §13)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR URL printed to stdout.

- [ ] **Step 4:** Report the PR URL

The PR URL is the deliverable for this plan. Paste it in the next message to close out.

---

## Spec-coverage check

| Spec requirement | Where covered |
|---|---|
| §5 layout — `lib/skillio/{repo,output,errors}.go` | Tasks 1–3 |
| §5 layout — `lib/skillruntime/{boot,handles}.go` | Tasks 4–5 |
| §6 transport contract — exit codes, JSONL stdout, structured stderr | Tasks 1–2, exercised in every skill test |
| §7 `.harness/` directories — read by `BootLifecycle` | Task 5 |
| §8.1 `/run-sensor` flow | Tasks 6–7 |
| §8.2 `/start-sensor` flow | Tasks 8–9 |
| §8.3 `/stop-sensor` flow | Tasks 10–11 |
| §8.4 `/tail-sensor-signals` flow | Tasks 12–14 |
| §11.1 deliverable acceptance — passing assertion, failing assertion, wrong-kind, start/stop round-trip, dead-handle recovery, tail follow, tail since, malformed handle | Tasks 7, 9, 11, 13, 14; integration smoke stubbed in Task 15 |
| §12 test strategy — unit + race + golden | Each task; `-race` in Task 16 |
