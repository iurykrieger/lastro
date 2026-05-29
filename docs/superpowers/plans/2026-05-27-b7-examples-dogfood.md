# B7 — Examples & Dogfood Self-Validation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the two-track integration evidence that the harness framework works end-to-end — three sample subject repos under `examples/` exercised by Go integration tests asserting plan §11's 7 acceptance criteria (Track 1), plus a dogfood gate that runs `/validate-use-case` against the framework's own committed `.harness/` (Track 2).

**Architecture:** A small `examples/validator/` Go package exposes `ValidateAll(ctx, target, skills) (*Report, error)`. It enumerates `.harness/use-cases/*.yaml` under `target`, shells out to a pre-built `/validate-use-case` skill binary once per use case (with `cmd.Dir = target`), parses the JSONL output, and aggregates into a `Report` written to `<target>/.harness/reports/<run-id>/report.json`. Both test tracks consume this primitive against different targets — synthetic samples under `examples/<sample>/` (Track 1) and the framework repo root (Track 2). The `/heal` skill is invoked once from a temp-dir copy of the broken sample with a committed `editplan.json` as stdin.

**Tech Stack:** Go 1.24.2, `sigs.k8s.io/yaml`, `github.com/oklog/ulid/v2`, `github.com/spf13/cobra` (cli-sample only). Build tags: `integration` (Track 1), `dogfood` (Track 2), untagged (validator unit tests).

**Source spec:** [`docs/superpowers/specs/2026-05-27-b7-examples-dogfood-design.md`](../specs/2026-05-27-b7-examples-dogfood-design.md)

---

## File Structure

| Path | Responsibility | Phase |
|---|---|---|
| `examples/validator/doc.go` | Package doc + version constant | 1 |
| `examples/validator/report.go` | `Report`, `UseCaseResult`, `SensorRunSummary`, `Summary` types + `AllPassed`/`Failed` methods | 1 |
| `examples/validator/copydir.go` | `CopyDir(src, dst)` recursive copy helper (for heal test) | 1 |
| `examples/validator/skill_binaries.go` | `SkillBinaries` + `NewSkillBinaries(workDir, frameworkRoot)` | 1 |
| `examples/validator/validator.go` | `ValidateAll` orchestrator + use-case enumeration + skill exec + JSONL parsing + report.json write | 1 |
| `examples/validator/*_test.go` | Unit tests with a fake skill binary built from `testdata/fakeskill/` | 1 |
| `examples/validator/testdata/fakeskill/main.go` | Pure-Go stub skill binary used by validator unit tests | 1 |
| `examples/validator/testdata/sample/...` | Tiny synthetic `.harness/` used to exercise enumeration/parsing | 1 |
| `examples/http-api-sample/{main,handlers,store}.go` + `go.mod` + `README.md` | Passing http-api subject (1 GET + 1 POST) | 2 |
| `examples/http-api-sample/.harness/{stack-manifest,use-cases,fixtures,sensors}/*.yaml` | Hand-curated detection artifacts for the passing sample | 2 |
| `examples/http-api-sample-broken/...` | Sibling sample with the seed bug + `heal-fixture/editplan.json` | 3 |
| `examples/cli-sample/...` | Minimal Cobra CLI subject (one `greet` subcommand) | 4 |
| `examples/integration_test.go` | Track 1 TestMain + criteria 1-5, 7 (build tag `integration`) | 5 |
| `examples/heal_test.go` | Track 1 criterion 6 (build tag `integration`) | 5 |
| `examples/dogfood_test.go` | Track 2 (build tag `dogfood`) | 6 |
| `.harness/{stack-manifest,use-cases,fixtures,sensors}/*.yaml` | Framework's own detection artifacts (produced manually) | 6 |
| `.gitignore` | Add `.harness/runtime/`, `.harness/reports/` exclusions | 7 |
| `examples/README.md` | Map of samples + how to run each track | 7 |
| `.github/workflows/ci.yml` (or equivalent) | Add integration + dogfood jobs | 7 |

**Key APIs consumed (do not redefine):**
- `internal/stack.Load(path string) (StackManifest, error)`
- `internal/fixture.LoadDirectory(path string) (*Store, error)` and `internal/fixture.LoadFixture(path string) (Fixture, error)`
- `internal/usecase.UnmarshalOnly(data []byte) (*UseCase, error)` — for id-only enumeration
- `internal/usecase.Load(data []byte, store fixture.FixtureStore) (*UseCase, error)` — for full validation
- `internal/usecase/template.Parse(input string) ([]Segment, error)`
- `internal/sensor.LoadDirectory(path string) (*Store, error)` and `internal/sensor.LoadSensor(path string) (Sensor, error)`
- `internal/sensor.Grounding(...)` — see `internal/sensor/grounding.go` for the actual signature
- `internal/policy.Resolve(...)` — see `internal/policy/resolve.go`
- `internal/aggregate.HealHint` (type alias for `signalstub.HealHint`)
- `github.com/oklog/ulid/v2.Make().String()` — for run ids

---

## Phase 0 — Preflight

### Task 0.1: Verify branch and scaffold the `examples/` tree

**Files:**
- Create: `examples/.gitkeep` (placeholder; removed once real files land)

- [ ] **Step 1:** Confirm current branch has main merged in.

```bash
git branch --show-current
git log -1 --oneline origin/main..HEAD
```

Expected: current branch shows a merge commit from `main` (you should see the 99-file merge already landed; if not, run `git fetch origin && git pull origin main` and resolve before proceeding).

- [ ] **Step 2:** Create the `examples/` directory.

```bash
mkdir -p examples
touch examples/.gitkeep
```

- [ ] **Step 3:** Commit the scaffold.

```bash
git add examples/.gitkeep
git commit -m "chore(b7): scaffold examples/ directory"
```

---

## Phase 1 — Validator package (TDD)

The validator is the linchpin. Build it first against a synthetic fake skill so the package is verified before any sample exists.

### Task 1.1: `examples/validator/doc.go` + `report.go` with TDD

**Files:**
- Create: `examples/validator/doc.go`
- Create: `examples/validator/report.go`
- Create: `examples/validator/report_test.go`

- [ ] **Step 1: Write the failing test for `AllPassed` and `Failed` helpers**

Create `examples/validator/report_test.go`:

```go
package validator

import "testing"

func TestReportAllPassed(t *testing.T) {
    r := &Report{Summary: Summary{Total: 3, Passed: 3}}
    if !r.AllPassed() {
        t.Fatalf("AllPassed: want true when Passed==Total>0, got false")
    }

    r2 := &Report{Summary: Summary{Total: 3, Passed: 2, Failed: 1}}
    if r2.AllPassed() {
        t.Fatalf("AllPassed: want false when any failed")
    }

    r3 := &Report{Summary: Summary{Total: 0}}
    if r3.AllPassed() {
        t.Fatalf("AllPassed: want false when Total==0 (empty report is not a passing report)")
    }
}

func TestReportFailed(t *testing.T) {
    r := &Report{UseCases: []UseCaseResult{
        {UseCaseID: "a", Verdict: "pass"},
        {UseCaseID: "b", Verdict: "fail"},
        {UseCaseID: "c", Verdict: "inconclusive"},
        {UseCaseID: "d", Verdict: "fail"},
    }}
    failed := r.Failed()
    if len(failed) != 2 {
        t.Fatalf("Failed: want 2, got %d", len(failed))
    }
    if failed[0].UseCaseID != "b" || failed[1].UseCaseID != "d" {
        t.Fatalf("Failed: want [b,d], got %+v", failed)
    }
}

func TestReportFailedEmpty(t *testing.T) {
    r := &Report{UseCases: []UseCaseResult{{UseCaseID: "a", Verdict: "pass"}}}
    failed := r.Failed()
    if failed == nil {
        t.Fatalf("Failed: must return non-nil empty slice, got nil")
    }
    if len(failed) != 0 {
        t.Fatalf("Failed: want empty, got %+v", failed)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./examples/validator/ -run TestReport
```

Expected: compile failure (`undefined: Report`, `undefined: Summary`, etc.).

- [ ] **Step 3: Write `doc.go` and `report.go`**

`examples/validator/doc.go`:

```go
// Package validator is the shared B7 primitive driving the framework's
// /validate-use-case skill against a target directory and aggregating
// the per-use-case verdicts into a Report.
//
// Both B7 test tracks consume this package:
//   - Track 1 (-tags=integration) drives ValidateAll against samples
//     under examples/<sample>/.
//   - Track 2 (-tags=dogfood) drives ValidateAll against the framework
//     repo root.
//
// The package never invokes the LLM-driven detection skills. It only
// shells out to /validate-use-case (and /heal in the heal test).
package validator

// ReportSchemaVersion is the schema_version written into report.json.
// Bump on any breaking change to the Report shape.
const ReportSchemaVersion = 1
```

`examples/validator/report.go`:

```go
package validator

import (
    "time"

    "github.com/iurykrieger/lastro/internal/aggregate"
)

// Report is the structured artifact written by ValidateAll to
// <target>/.harness/reports/<run-id>/report.json.
type Report struct {
    SchemaVersion int             `json:"schema_version"`
    RunID         string          `json:"run_id"`
    Target        string          `json:"target"`
    StartedAt     time.Time       `json:"started_at"`
    EndedAt       time.Time       `json:"ended_at"`
    UseCases      []UseCaseResult `json:"use_cases"`
    Summary       Summary         `json:"summary"`
}

// UseCaseResult is one entry in Report.UseCases. Verdict mirrors the
// persistedVerdict envelope from the /validate-use-case skill.
type UseCaseResult struct {
    UseCaseID  string              `json:"use_case_id"`
    Verdict    string              `json:"verdict"` // pass | fail | inconclusive
    SensorRuns []SensorRunSummary  `json:"sensor_runs"`
    HealHint   *aggregate.HealHint `json:"heal_hint,omitempty"`
    Stdout     string              `json:"-"` // raw JSONL, retained for test debugging
}

// SensorRunSummary mirrors persistedVerdict.sensor_runs entries.
type SensorRunSummary struct {
    SensorID string `json:"sensor_id"`
    Verdict  string `json:"verdict"`
}

// Summary aggregates use-case verdicts in one Report.
type Summary struct {
    Total        int `json:"total"`
    Passed       int `json:"passed"`
    Failed       int `json:"failed"`
    Inconclusive int `json:"inconclusive"`
}

// AllPassed reports whether every use case in the report had verdict=pass.
// An empty report (Total==0) is NOT considered passing — that means no
// use cases were detected, which is itself a regression.
func (r *Report) AllPassed() bool {
    return r.Summary.Total > 0 && r.Summary.Passed == r.Summary.Total
}

// Failed returns the subset of UseCases with verdict=fail. Returns a
// non-nil empty slice when none failed.
func (r *Report) Failed() []UseCaseResult {
    out := make([]UseCaseResult, 0)
    for _, uc := range r.UseCases {
        if uc.Verdict == "fail" {
            out = append(out, uc)
        }
    }
    return out
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./examples/validator/ -run TestReport
```

Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add examples/validator/doc.go examples/validator/report.go examples/validator/report_test.go
git commit -m "feat(examples/validator): add Report type with AllPassed/Failed helpers"
```

---

### Task 1.2: `copydir.go` helper (TDD)

**Files:**
- Create: `examples/validator/copydir.go`
- Create: `examples/validator/copydir_test.go`

- [ ] **Step 1: Write the failing test**

`examples/validator/copydir_test.go`:

```go
package validator

import (
    "os"
    "path/filepath"
    "testing"
)

func TestCopyDirCopiesFiles(t *testing.T) {
    src := t.TempDir()
    dst := t.TempDir()

    if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
        t.Fatal(err)
    }
    if err := os.WriteFile(filepath.Join(src, "top.txt"), []byte("top"), 0o644); err != nil {
        t.Fatal(err)
    }
    if err := os.WriteFile(filepath.Join(src, "sub", "nested.txt"), []byte("nested"), 0o644); err != nil {
        t.Fatal(err)
    }

    if err := CopyDir(src, dst); err != nil {
        t.Fatalf("CopyDir: %v", err)
    }

    top, err := os.ReadFile(filepath.Join(dst, "top.txt"))
    if err != nil {
        t.Fatalf("read top: %v", err)
    }
    if string(top) != "top" {
        t.Fatalf("top: want %q, got %q", "top", string(top))
    }

    nested, err := os.ReadFile(filepath.Join(dst, "sub", "nested.txt"))
    if err != nil {
        t.Fatalf("read nested: %v", err)
    }
    if string(nested) != "nested" {
        t.Fatalf("nested: want %q, got %q", "nested", string(nested))
    }
}

func TestCopyDirPreservesFileMode(t *testing.T) {
    src := t.TempDir()
    dst := t.TempDir()
    exe := filepath.Join(src, "run.sh")
    if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
        t.Fatal(err)
    }
    if err := CopyDir(src, dst); err != nil {
        t.Fatal(err)
    }
    info, err := os.Stat(filepath.Join(dst, "run.sh"))
    if err != nil {
        t.Fatal(err)
    }
    // Allow OSes that ignore exec bit (Windows). Just check it copied.
    if info.Size() == 0 {
        t.Fatalf("copied file is empty")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./examples/validator/ -run TestCopyDir
```

Expected: compile failure (`undefined: CopyDir`).

- [ ] **Step 3: Write `copydir.go`**

```go
package validator

import (
    "io"
    "io/fs"
    "os"
    "path/filepath"
)

// CopyDir recursively copies the contents of src into dst.
// dst is created (with src's mode for the root) if it does not exist.
// Symlinks are dereferenced and written as regular files.
func CopyDir(src, dst string) error {
    return filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
        if err != nil {
            return err
        }
        rel, err := filepath.Rel(src, path)
        if err != nil {
            return err
        }
        target := filepath.Join(dst, rel)
        if info.IsDir() {
            return os.MkdirAll(target, info.Mode())
        }
        return copyFile(path, target, info.Mode())
    })
}

func copyFile(src, dst string, mode os.FileMode) error {
    if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
        return err
    }
    in, err := os.Open(src)
    if err != nil {
        return err
    }
    defer in.Close()
    out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
    if err != nil {
        return err
    }
    defer out.Close()
    _, err = io.Copy(out, in)
    return err
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./examples/validator/ -run TestCopyDir
```

Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add examples/validator/copydir.go examples/validator/copydir_test.go
git commit -m "feat(examples/validator): add CopyDir helper for heal test sandbox"
```

---

### Task 1.3: `skill_binaries.go` — builds skill binaries on demand

**Files:**
- Create: `examples/validator/skill_binaries.go`
- Create: `examples/validator/skill_binaries_test.go`

- [ ] **Step 1: Write the failing test**

```go
package validator

import (
    "os"
    "path/filepath"
    "runtime"
    "testing"
)

// frameworkRootForTest walks up from this file to find the lastro module root.
func frameworkRootForTest(t *testing.T) string {
    t.Helper()
    _, thisFile, _, _ := runtime.Caller(0)
    // examples/validator/skill_binaries_test.go -> ../..
    return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func TestNewSkillBinariesBuildsBothBinaries(t *testing.T) {
    if testing.Short() {
        t.Skip("requires go build")
    }
    work := t.TempDir()
    sb, err := NewSkillBinaries(work, frameworkRootForTest(t))
    if err != nil {
        t.Fatalf("NewSkillBinaries: %v", err)
    }
    for _, p := range []string{sb.ValidateUseCase, sb.Heal} {
        info, err := os.Stat(p)
        if err != nil {
            t.Fatalf("stat %s: %v", p, err)
        }
        if info.Size() == 0 {
            t.Fatalf("%s is empty", p)
        }
    }
}

func TestNewSkillBinariesRejectsEmptyArgs(t *testing.T) {
    if _, err := NewSkillBinaries("", "/some/root"); err == nil {
        t.Fatalf("want error for empty workDir")
    }
    if _, err := NewSkillBinaries(t.TempDir(), ""); err == nil {
        t.Fatalf("want error for empty frameworkRoot")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./examples/validator/ -run TestNewSkillBinaries
```

Expected: compile failure (`undefined: SkillBinaries`, `undefined: NewSkillBinaries`).

- [ ] **Step 3: Write `skill_binaries.go`**

```go
package validator

import (
    "errors"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
)

// SkillBinaries holds absolute paths to skill binaries built by
// NewSkillBinaries. Tests share one instance per process (typically
// constructed in TestMain).
type SkillBinaries struct {
    ValidateUseCase string
    Heal            string
}

// NewSkillBinaries runs `go build` for the validate-use-case and heal
// skills, writing the binaries into workDir. frameworkRoot must be the
// lastro module root (where ./skills/... resolves). workDir must exist
// or be creatable.
func NewSkillBinaries(workDir, frameworkRoot string) (*SkillBinaries, error) {
    if workDir == "" {
        return nil, errors.New("workDir is required")
    }
    if frameworkRoot == "" {
        return nil, errors.New("frameworkRoot is required")
    }
    if err := os.MkdirAll(workDir, 0o755); err != nil {
        return nil, fmt.Errorf("mkdir workDir: %w", err)
    }

    sb := &SkillBinaries{}
    builds := []struct {
        name string
        pkg  string
        out  *string
    }{
        {"validate-use-case", "./skills/validate-use-case", &sb.ValidateUseCase},
        {"heal", "./skills/heal", &sb.Heal},
    }
    for _, b := range builds {
        exeName := b.name
        if runtime.GOOS == "windows" {
            exeName += ".exe"
        }
        outPath := filepath.Join(workDir, exeName)
        cmd := exec.Command("go", "build", "-o", outPath, b.pkg)
        cmd.Dir = frameworkRoot
        if out, err := cmd.CombinedOutput(); err != nil {
            return nil, fmt.Errorf("go build %s: %w\n%s", b.pkg, err, out)
        }
        *b.out = outPath
    }
    return sb, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./examples/validator/ -run TestNewSkillBinaries -v
```

Expected: both tests pass. The build may take ~5-15s on first run.

- [ ] **Step 5: Commit**

```bash
git add examples/validator/skill_binaries.go examples/validator/skill_binaries_test.go
git commit -m "feat(examples/validator): build skill binaries on demand for tests"
```

---

### Task 1.4: `validator.go` — `ValidateAll` parsing + aggregation against a fake skill

The unit test for `ValidateAll` uses a hand-built fake skill binary in `testdata/fakeskill/` that emits canned JSONL. This decouples the validator unit tests from the real skill implementation.

**Files:**
- Create: `examples/validator/testdata/fakeskill/main.go`
- Create: `examples/validator/testdata/sample/.harness/use-cases/uc-alpha.yaml`
- Create: `examples/validator/testdata/sample/.harness/use-cases/uc-bravo.yaml`
- Create: `examples/validator/validator.go`
- Create: `examples/validator/validator_test.go`

- [ ] **Step 1: Write the fake skill binary**

`examples/validator/testdata/fakeskill/main.go`:

```go
// fakeskill emits canned /validate-use-case JSONL for ValidateAll unit
// tests. It treats argv[1] as the use case id and selects a scripted
// response from the FAKESKILL_RESPONSES env var (JSON map: id -> exit
// code + stdout lines).
package main

import (
    "encoding/json"
    "fmt"
    "io"
    "os"
)

type Scripted struct {
    Exit  int      `json:"exit"`
    Lines []string `json:"lines"`
}

func main() {
    if len(os.Args) < 2 {
        fmt.Fprintln(os.Stderr, `{"code":"bad-argv"}`)
        os.Exit(3)
    }
    raw := os.Getenv("FAKESKILL_RESPONSES")
    if raw == "" {
        fmt.Fprintln(os.Stderr, `{"code":"no-script"}`)
        os.Exit(3)
    }
    var script map[string]Scripted
    if err := json.Unmarshal([]byte(raw), &script); err != nil {
        fmt.Fprintln(os.Stderr, `{"code":"bad-script","details":"`+err.Error()+`"}`)
        os.Exit(3)
    }
    resp, ok := script[os.Args[1]]
    if !ok {
        fmt.Fprintln(os.Stderr, `{"code":"unknown-id"}`)
        os.Exit(3)
    }
    for _, line := range resp.Lines {
        if _, err := io.WriteString(os.Stdout, line+"\n"); err != nil {
            os.Exit(3)
        }
    }
    os.Exit(resp.Exit)
}
```

- [ ] **Step 2: Write the tiny synthetic use-case YAMLs the fake skill responds to**

`examples/validator/testdata/sample/.harness/use-cases/uc-alpha.yaml`:

```yaml
schema_version: 2.0.0
id: uc-alpha
title: alpha
archetype_scope: [cli]
entry_points:
  - id: ep-alpha
    archetype: cli
    spec:
      command: alpha
given: ["x"]
when: ["y"]
then: ["z"]
fixture_ids: []
```

`examples/validator/testdata/sample/.harness/use-cases/uc-bravo.yaml`: same shape with `id: uc-bravo`, `title: bravo`, `command: bravo`.

> **Note:** The fake skill never actually parses these — it just keys on argv[1]. They exist so `enumerateUseCases` finds them. If the use-case loader rejects this minimal shape, copy the full structure from `schemas/examples/use-case/cli.yaml` and trim later.

- [ ] **Step 3: Write the failing validator test**

`examples/validator/validator_test.go`:

```go
package validator

import (
    "context"
    "encoding/json"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "testing"
)

// buildFakeSkill builds the testdata/fakeskill stub into a temp dir
// and returns the absolute path to the binary.
func buildFakeSkill(t *testing.T) string {
    t.Helper()
    out := filepath.Join(t.TempDir(), "fakeskill")
    if runtime.GOOS == "windows" {
        out += ".exe"
    }
    cmd := exec.Command("go", "build", "-o", out, "./testdata/fakeskill")
    cmd.Dir = "."
    if b, err := cmd.CombinedOutput(); err != nil {
        t.Fatalf("build fakeskill: %v\n%s", err, b)
    }
    return out
}

func TestValidateAllAggregatesVerdicts(t *testing.T) {
    if testing.Short() {
        t.Skip("requires go build")
    }
    fake := buildFakeSkill(t)
    sample, err := filepath.Abs("testdata/sample")
    if err != nil {
        t.Fatal(err)
    }

    script := map[string]struct {
        Exit  int      `json:"exit"`
        Lines []string `json:"lines"`
    }{
        "uc-alpha": {
            Exit: 0,
            Lines: []string{
                `{"sensor_id":"alpha-e2e","verdict":"pass"}`,
                `{"use_case_verdict":{"verdict":"pass","confidence":1.0},"use_case_run_id":"01HX","sensor_runs":[{"sensor_id":"alpha-e2e","verdict":"pass"}]}`,
            },
        },
        "uc-bravo": {
            Exit: 1,
            Lines: []string{
                `{"sensor_id":"bravo-e2e","verdict":"fail","heal_hint":{"summary":"bravo failed"}}`,
                `{"use_case_verdict":{"verdict":"fail","confidence":0.9},"use_case_run_id":"01HY","sensor_runs":[{"sensor_id":"bravo-e2e","verdict":"fail"}]}`,
            },
        },
    }
    raw, _ := json.Marshal(script)
    t.Setenv("FAKESKILL_RESPONSES", string(raw))

    skills := &SkillBinaries{ValidateUseCase: fake}
    report, err := ValidateAll(context.Background(), sample, skills)
    if err != nil {
        t.Fatalf("ValidateAll: %v", err)
    }

    if report.Summary.Total != 2 {
        t.Fatalf("Total: want 2, got %d", report.Summary.Total)
    }
    if report.Summary.Passed != 1 || report.Summary.Failed != 1 {
        t.Fatalf("Summary: want 1 pass + 1 fail, got %+v", report.Summary)
    }
    if len(report.Failed()) != 1 || report.Failed()[0].UseCaseID != "uc-bravo" {
        t.Fatalf("Failed: want [uc-bravo], got %+v", report.Failed())
    }
    if report.Failed()[0].HealHint == nil {
        t.Fatalf("Failed[0].HealHint: want non-nil")
    }

    // report.json must exist under .harness/reports/<run-id>/
    reportPath := filepath.Join(sample, ".harness", "reports", report.RunID, "report.json")
    if _, err := os.Stat(reportPath); err != nil {
        t.Fatalf("report.json missing at %s: %v", reportPath, err)
    }
    t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(sample, ".harness", "reports")) })
}

func TestValidateAllPropagatesScriptError(t *testing.T) {
    if testing.Short() {
        t.Skip("requires go build")
    }
    fake := buildFakeSkill(t)
    sample, _ := filepath.Abs("testdata/sample")

    script := map[string]struct {
        Exit  int      `json:"exit"`
        Lines []string `json:"lines"`
    }{
        "uc-alpha": {Exit: 3, Lines: []string{`{"code":"boom"}`}},
        "uc-bravo": {Exit: 0, Lines: []string{`{"use_case_verdict":{"verdict":"pass","confidence":1.0},"use_case_run_id":"x","sensor_runs":[]}`}},
    }
    raw, _ := json.Marshal(script)
    t.Setenv("FAKESKILL_RESPONSES", string(raw))

    skills := &SkillBinaries{ValidateUseCase: fake}
    if _, err := ValidateAll(context.Background(), sample, skills); err == nil {
        t.Fatalf("ValidateAll: want error on skill exit 3, got nil")
    }
    t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(sample, ".harness", "reports")) })
}
```

- [ ] **Step 4: Run test to verify it fails**

```bash
go test ./examples/validator/ -run TestValidateAll
```

Expected: compile failure (`undefined: ValidateAll`).

- [ ] **Step 5: Write `validator.go`**

```go
package validator

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "sort"
    "strings"
    "time"

    "github.com/iurykrieger/lastro/internal/aggregate"
    "github.com/iurykrieger/lastro/internal/usecase"
    "github.com/oklog/ulid/v2"
)

// ValidateAll enumerates use cases under target/.harness/use-cases,
// invokes /validate-use-case once per id (with cmd.Dir=target), parses
// the JSONL output, and aggregates into a Report. Writes the Report to
// <target>/.harness/reports/<run-id>/report.json.
//
// Skill exit codes 0/1/2 all produce a UseCaseResult. Exit code 3 is a
// script error and returns a non-nil error.
func ValidateAll(ctx context.Context, target string, skills *SkillBinaries) (*Report, error) {
    if skills == nil || skills.ValidateUseCase == "" {
        return nil, errors.New("SkillBinaries.ValidateUseCase is required")
    }
    abs, err := filepath.Abs(target)
    if err != nil {
        return nil, fmt.Errorf("resolve target: %w", err)
    }

    ids, err := enumerateUseCases(abs)
    if err != nil {
        return nil, fmt.Errorf("enumerate use cases: %w", err)
    }
    sort.Strings(ids)

    r := &Report{
        SchemaVersion: ReportSchemaVersion,
        RunID:         ulid.Make().String(),
        Target:        abs,
        StartedAt:     time.Now().UTC(),
        UseCases:      make([]UseCaseResult, 0, len(ids)),
    }

    for _, id := range ids {
        res, err := runOne(ctx, abs, skills.ValidateUseCase, id)
        if err != nil {
            return nil, fmt.Errorf("run use case %q: %w", id, err)
        }
        r.UseCases = append(r.UseCases, res)
        r.Summary.Total++
        switch res.Verdict {
        case "pass":
            r.Summary.Passed++
        case "fail":
            r.Summary.Failed++
        case "inconclusive":
            r.Summary.Inconclusive++
        }
    }

    r.EndedAt = time.Now().UTC()
    if err := writeReport(abs, r); err != nil {
        return nil, fmt.Errorf("write report: %w", err)
    }
    return r, nil
}

func enumerateUseCases(target string) ([]string, error) {
    dir := filepath.Join(target, ".harness", "use-cases")
    entries, err := os.ReadDir(dir)
    if err != nil {
        return nil, err
    }
    var ids []string
    for _, e := range entries {
        if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
            continue
        }
        data, err := os.ReadFile(filepath.Join(dir, e.Name()))
        if err != nil {
            return nil, fmt.Errorf("read %s: %w", e.Name(), err)
        }
        uc, err := usecase.UnmarshalOnly(data)
        if err != nil {
            return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
        }
        ids = append(ids, uc.ID)
    }
    return ids, nil
}

// persistedVerdict mirrors the envelope emitted as the final stdout line
// by /validate-use-case.
type persistedVerdict struct {
    UseCaseVerdict struct {
        Verdict    string  `json:"verdict"`
        Confidence float64 `json:"confidence"`
    } `json:"use_case_verdict"`
    UseCaseRunID string `json:"use_case_run_id"`
    SensorRuns   []struct {
        SensorID string `json:"sensor_id"`
        Verdict  string `json:"verdict"`
    } `json:"sensor_runs"`
}

// sensorEnvelope mirrors a per-sensor AggregateSignal line on the JSONL
// stream. We capture the first non-nil HealHint to attach to UseCaseResult.
type sensorEnvelope struct {
    SensorID string              `json:"sensor_id"`
    Verdict  string              `json:"verdict"`
    HealHint *aggregate.HealHint `json:"heal_hint,omitempty"`
}

func runOne(ctx context.Context, target, binPath, ucID string) (UseCaseResult, error) {
    cmd := exec.CommandContext(ctx, binPath, ucID)
    cmd.Dir = target
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    runErr := cmd.Run()

    // Exit 3 = script error. Everything else (0/1/2) is verdict-derived
    // and parsable from stdout.
    if ee, ok := runErr.(*exec.ExitError); ok {
        if ee.ExitCode() == 3 {
            return UseCaseResult{}, fmt.Errorf("skill script error (exit 3) for %q: %s", ucID, stderr.String())
        }
    } else if runErr != nil {
        return UseCaseResult{}, fmt.Errorf("exec skill: %v: stderr=%s", runErr, stderr.String())
    }

    result := UseCaseResult{UseCaseID: ucID, Stdout: stdout.String()}

    var firstHint *aggregate.HealHint
    var pv *persistedVerdict
    for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
        if line == "" {
            continue
        }
        var asPV persistedVerdict
        if err := json.Unmarshal([]byte(line), &asPV); err == nil && asPV.UseCaseVerdict.Verdict != "" {
            pv = &asPV
            continue
        }
        var se sensorEnvelope
        if err := json.Unmarshal([]byte(line), &se); err == nil && se.SensorID != "" {
            if firstHint == nil && se.HealHint != nil {
                firstHint = se.HealHint
            }
        }
    }

    if pv == nil {
        return UseCaseResult{}, fmt.Errorf("no persisted verdict in skill stdout for %q: %s", ucID, stdout.String())
    }

    result.Verdict = pv.UseCaseVerdict.Verdict
    for _, sr := range pv.SensorRuns {
        result.SensorRuns = append(result.SensorRuns, SensorRunSummary{
            SensorID: sr.SensorID,
            Verdict:  sr.Verdict,
        })
    }
    result.HealHint = firstHint
    return result, nil
}

func writeReport(target string, r *Report) error {
    dir := filepath.Join(target, ".harness", "reports", r.RunID)
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return err
    }
    b, err := json.MarshalIndent(r, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(filepath.Join(dir, "report.json"), b, 0o644)
}
```

- [ ] **Step 6: Run test to verify it passes**

```bash
go test ./examples/validator/ -v
```

Expected: ALL tests in the package pass (Report tests, CopyDir tests, SkillBinaries tests, ValidateAll tests).

- [ ] **Step 7: Verify `usecase.UnmarshalOnly` accepts the testdata YAML**

If `TestValidateAllAggregatesVerdicts` fails with a parse error in `enumerateUseCases`, the testdata YAML is too minimal for `UnmarshalOnly`. In that case, copy the full structure from `schemas/examples/use-case/cli.yaml`, set the ids to `uc-alpha`/`uc-bravo`, and re-run.

- [ ] **Step 8: Commit**

```bash
git add examples/validator/validator.go examples/validator/validator_test.go examples/validator/testdata/
git commit -m "feat(examples/validator): implement ValidateAll with JSONL parsing + report.json"
```

---

## Phase 2 — `examples/http-api-sample/` (canonical passing sample)

### Task 2.1: Sample Go source (`go.mod`, `main.go`, `handlers.go`, `store.go`, `README.md`)

**Files:**
- Create: `examples/http-api-sample/go.mod`
- Create: `examples/http-api-sample/main.go`
- Create: `examples/http-api-sample/handlers.go`
- Create: `examples/http-api-sample/store.go`
- Create: `examples/http-api-sample/README.md`

- [ ] **Step 1: Write `go.mod`**

```
module example.com/http-api-sample

go 1.24
```

- [ ] **Step 2: Write `store.go`**

```go
package main

import (
    "fmt"
    "sync"
)

type OrderInput struct {
    Item string `json:"item"`
}

type Order struct {
    ID   string `json:"id"`
    Item string `json:"item"`
}

type Store struct {
    mu     sync.Mutex
    orders map[string]Order
    next   int
}

func NewStore() *Store { return &Store{orders: make(map[string]Order)} }

func (s *Store) Create(in OrderInput) Order {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.next++
    id := fmt.Sprintf("%d", s.next)
    o := Order{ID: id, Item: in.Item}
    s.orders[id] = o
    return o
}

func (s *Store) Get(id string) (Order, bool) {
    s.mu.Lock()
    defer s.mu.Unlock()
    o, ok := s.orders[id]
    return o, ok
}
```

- [ ] **Step 3: Write `handlers.go`**

```go
package main

import (
    "encoding/json"
    "net/http"
)

func GetOrderHandler(s *Store) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        id := r.PathValue("id")
        order, ok := s.Get(id)
        if !ok {
            http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(order)
    })
}

func CreateOrderHandler(s *Store) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        var body OrderInput
        if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
            http.Error(w, `{"error":"bad json"}`, http.StatusBadRequest)
            return
        }
        if body.Item == "" {
            http.Error(w, `{"error":"missing required field: item"}`, http.StatusBadRequest)
            return
        }
        order := s.Create(body)
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusCreated)
        _ = json.NewEncoder(w).Encode(order)
    })
}
```

- [ ] **Step 4: Write `main.go`**

```go
package main

import (
    "log"
    "net/http"
)

func main() {
    s := NewStore()
    s.Create(OrderInput{Item: "widget"}) // seed id=1

    mux := http.NewServeMux()
    mux.Handle("GET /orders/{id}", GetOrderHandler(s))
    mux.Handle("POST /orders", CreateOrderHandler(s))

    log.Println("listening on :8080")
    log.Fatal(http.ListenAndServe(":8080", mux))
}
```

- [ ] **Step 5: Write `README.md`**

```markdown
# http-api-sample

Minimal Go HTTP API used by the harness framework's Track 1 integration
tests as the canonical archetype-`http-api` subject.

## Endpoints

- `GET /orders/{id}` — returns the order if known, 404 otherwise.
- `POST /orders` — creates an order with body `{"item": "..."}`. Returns
  400 when `item` is missing.

## Running standalone

    cd examples/http-api-sample
    go run .

## What it demonstrates

- Stack detection (archetype=`http-api`) → see `.harness/stack-manifest.yaml`.
- Three use cases (get-order, create-order-success, create-order-bad-input).
- Six sensors across two angles (`e2e-test`, `unit-test`), with one
  fixture (`valid_order_payload`) reused across both angles of the
  create-order-success use case (plan §11.7).
```

- [ ] **Step 6: Verify the sample compiles**

```bash
cd examples/http-api-sample && go build ./... && cd ../..
```

Expected: no output (clean build).

- [ ] **Step 7: Commit**

```bash
git add examples/http-api-sample/go.mod examples/http-api-sample/main.go examples/http-api-sample/handlers.go examples/http-api-sample/store.go examples/http-api-sample/README.md
git commit -m "feat(examples/http-api-sample): scaffold passing HTTP API subject"
```

---

### Task 2.2: `http-api-sample/.harness/stack-manifest.yaml` + fixtures

**Files:**
- Create: `examples/http-api-sample/.harness/stack-manifest.yaml`
- Create: `examples/http-api-sample/.harness/fixtures/existing-order-id.yaml`
- Create: `examples/http-api-sample/.harness/fixtures/valid-order-payload.yaml`
- Create: `examples/http-api-sample/.harness/fixtures/invalid-order-payload.yaml`

> **Schema reference:** `schemas/examples/stack-manifest/http-api.yaml` and `schemas/examples/fixture/input.yaml`. If a field below trips validation, copy the field from the golden example.

- [ ] **Step 1: Write `stack-manifest.yaml`**

```yaml
# http-api-sample stack manifest
schema_version: 1.0.0
archetype: http-api
applicable_angles:
  - build
  - unit-test
  - e2e-test
  - contracts
components:
  - schema_version: 1.0.0
    id: go-stdlib
    kind: language-runtime
    name: go
    version: "1.24"
    capabilities:
      - http-server
      - json-encoding
    detection_evidence:
      - file: go.mod
        path: go
        value: "1.24"
  - schema_version: 1.0.0
    id: net-http
    kind: framework
    name: net/http
    version: stdlib
    capabilities:
      - http-routing
      - request-handling
    detection_evidence:
      - file: main.go
        path: imports
        value: net/http
```

- [ ] **Step 2: Write the three fixture YAMLs**

`existing-order-id.yaml`:

```yaml
schema_version: 1.0.0
id: existing-order-id
use_case_id: uc-get-order
role: input
content_type: text/plain
payload: "1"
binding:
  channel: http
  selector:
    method: GET
    path: /orders/{id}
source_refs:
  - path: main.go
    symbol: NewStore
    reason: "seeded order id 1 in main()"
```

`valid-order-payload.yaml`:

```yaml
schema_version: 1.0.0
id: valid-order-payload
use_case_id: uc-create-order-success
role: input
content_type: application/json
payload: |
  {"item":"gizmo"}
binding:
  channel: http
  selector:
    method: POST
    path: /orders
source_refs:
  - path: handlers.go
    symbol: CreateOrderHandler
    reason: "matches OrderInput struct"
```

`invalid-order-payload.yaml`:

```yaml
schema_version: 1.0.0
id: invalid-order-payload
use_case_id: uc-create-order-bad-input
role: input
content_type: application/json
payload: |
  {}
binding:
  channel: http
  selector:
    method: POST
    path: /orders
source_refs:
  - path: handlers.go
    symbol: CreateOrderHandler
    reason: "exercises missing-field branch"
```

- [ ] **Step 3: Verify load**

Write a one-shot check in a scratch test file to confirm everything loads:

```bash
go test -count=1 -run TestStackManifestLoads_HttpApi ./examples/validator/ 2>&1 || true
```

Add this gate test to `examples/validator/validator_test.go` (or create `harness_smoke_test.go` in the same package):

```go
func TestStackManifestLoads_HttpApi(t *testing.T) {
    _, err := stack.Load("../http-api-sample/.harness/stack-manifest.yaml")
    if err != nil {
        t.Fatalf("load: %v", err)
    }
}

func TestFixturesLoad_HttpApi(t *testing.T) {
    _, err := fixture.LoadDirectory("../http-api-sample/.harness/fixtures")
    if err != nil {
        t.Fatalf("load fixtures: %v", err)
    }
}
```

(Add imports: `"github.com/iurykrieger/lastro/internal/stack"` and `"github.com/iurykrieger/lastro/internal/fixture"`.)

- [ ] **Step 4: Run gate test**

```bash
go test -run 'TestStackManifestLoads_HttpApi|TestFixturesLoad_HttpApi' ./examples/validator/ -v
```

Expected: PASS. If it fails, the YAML deviates from the schema — adjust against the golden example in `schemas/examples/`.

- [ ] **Step 5: Commit**

```bash
git add examples/http-api-sample/.harness/stack-manifest.yaml examples/http-api-sample/.harness/fixtures/ examples/validator/validator_test.go
git commit -m "feat(examples/http-api-sample): stack manifest + 3 fixtures"
```

---

### Task 2.3: `http-api-sample/.harness/use-cases/*.yaml` (3 use cases)

**Files:**
- Create: `examples/http-api-sample/.harness/use-cases/uc-get-order.yaml`
- Create: `examples/http-api-sample/.harness/use-cases/uc-create-order-success.yaml`
- Create: `examples/http-api-sample/.harness/use-cases/uc-create-order-bad-input.yaml`

> **Schema reference:** `schemas/examples/use-case/http-api.yaml`. Each use case has `schema_version: 2.0.0`, `archetype_scope: [http-api]`, embedded `entry_points`, lists of `given/when/then` strings, and `fixture_ids`.

- [ ] **Step 1: Write `uc-get-order.yaml`**

```yaml
schema_version: 2.0.0
id: uc-get-order
title: "Client fetches an existing order"
archetype_scope: [http-api]

entry_points:
  - id: get-order-endpoint
    archetype: http-api
    spec:
      method: GET
      path: /orders/{id}

given:
  - "An order with id {{fixtures.existing-order-id}} exists in the store"
when:
  - "The client invokes {{entry_points.get-order-endpoint}} with the order id"
then:
  - "The endpoint responds with status 200"
  - "The response body is JSON describing the order"

source_refs:
  - path: handlers.go
    symbol: GetOrderHandler
    reason: "endpoint detected from mux registration"

fixture_ids: [existing-order-id]
```

- [ ] **Step 2: Write `uc-create-order-success.yaml`**

```yaml
schema_version: 2.0.0
id: uc-create-order-success
title: "Client creates an order with a valid payload"
archetype_scope: [http-api]

entry_points:
  - id: create-order-endpoint
    archetype: http-api
    spec:
      method: POST
      path: /orders

given:
  - "The client has a valid order payload {{fixtures.valid-order-payload}}"
when:
  - "The client invokes {{entry_points.create-order-endpoint}} with the payload"
then:
  - "The endpoint responds with status 201"
  - "The response body contains the created order with an id"

source_refs:
  - path: handlers.go
    symbol: CreateOrderHandler
    reason: "endpoint detected from mux registration"

fixture_ids: [valid-order-payload]
```

- [ ] **Step 3: Write `uc-create-order-bad-input.yaml`**

```yaml
schema_version: 2.0.0
id: uc-create-order-bad-input
title: "Client receives 400 when posting an invalid order payload"
archetype_scope: [http-api]

entry_points:
  - id: create-order-endpoint
    archetype: http-api
    spec:
      method: POST
      path: /orders

given:
  - "The client has an invalid order payload {{fixtures.invalid-order-payload}}"
when:
  - "The client invokes {{entry_points.create-order-endpoint}} with the payload"
then:
  - "The endpoint responds with status 400"
  - "The response body contains an error envelope"

source_refs:
  - path: handlers.go
    symbol: CreateOrderHandler
    reason: "endpoint detected from mux registration; validation branch exercised"

fixture_ids: [invalid-order-payload]
```

- [ ] **Step 4: Add a gate test**

In `examples/validator/validator_test.go`:

```go
func TestUseCasesLoad_HttpApi(t *testing.T) {
    fs, err := fixture.LoadDirectory("../http-api-sample/.harness/fixtures")
    if err != nil {
        t.Fatalf("fixtures: %v", err)
    }
    entries, err := os.ReadDir("../http-api-sample/.harness/use-cases")
    if err != nil {
        t.Fatal(err)
    }
    for _, e := range entries {
        data, err := os.ReadFile(filepath.Join("../http-api-sample/.harness/use-cases", e.Name()))
        if err != nil {
            t.Fatal(err)
        }
        if _, err := usecase.Load(data, fs); err != nil {
            t.Fatalf("load %s: %v", e.Name(), err)
        }
    }
}
```

(Add import `"github.com/iurykrieger/lastro/internal/usecase"`.)

- [ ] **Step 5: Run gate test**

```bash
go test -run TestUseCasesLoad_HttpApi ./examples/validator/ -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add examples/http-api-sample/.harness/use-cases/ examples/validator/validator_test.go
git commit -m "feat(examples/http-api-sample): three use cases (get, create, bad-input)"
```

---

### Task 2.4: `http-api-sample/.harness/sensors/*.yaml` (6 sensors)

**Files (6 sensors = 3 use cases × 2 angles):**
- `sensors/uc-get-order-e2e-test.yaml`
- `sensors/uc-get-order-unit-test.yaml`
- `sensors/uc-create-order-success-e2e-test.yaml`
- `sensors/uc-create-order-success-unit-test.yaml`
- `sensors/uc-create-order-bad-input-e2e-test.yaml`
- `sensors/uc-create-order-bad-input-unit-test.yaml`

> **Schema reference:** `schemas/examples/sensor/assertion-computational-single.yaml`. Top-level `uses:` references stack-component ids from `stack-manifest.yaml`. Step-level `uses:` references fixture ids owned by the sensor's use case (this is the fixture-reuse hook).
>
> **Sensor body strategy:** the sensors describe what to execute; the executor in `internal/runtime/executor` runs the steps. For the passing sample, a sensor that always succeeds is enough — what we're testing is that the framework correctly identifies pass vs fail, not that the sample's binary actually responds to HTTP. The simplest passing step is `run: "true"` (the Unix true command; on Windows use `run: "cmd /c exit 0"` — but since CI is Linux, plain `true` is fine; document this).

- [ ] **Step 1: Write `uc-get-order-e2e-test.yaml`**

```yaml
schema_version: 1.0.0
id: uc-get-order-e2e-test
use_case_id: uc-get-order
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot

uses:
  - go-stdlib
  - net-http

steps:
  - id: probe
    uses: [existing-order-id]
    run: "true"
```

- [ ] **Step 2: Write `uc-get-order-unit-test.yaml`**

```yaml
schema_version: 1.0.0
id: uc-get-order-unit-test
use_case_id: uc-get-order
angle: unit-test
kind: assertion
nature: computational
output_type: single-shot

uses:
  - go-stdlib

steps:
  - id: probe
    uses: [existing-order-id]
    run: "true"
```

- [ ] **Step 3: Write `uc-create-order-success-e2e-test.yaml`**

```yaml
schema_version: 1.0.0
id: uc-create-order-success-e2e-test
use_case_id: uc-create-order-success
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot

uses:
  - go-stdlib
  - net-http

steps:
  - id: probe
    uses: [valid-order-payload]
    run: "true"
```

- [ ] **Step 4: Write `uc-create-order-success-unit-test.yaml`**

```yaml
schema_version: 1.0.0
id: uc-create-order-success-unit-test
use_case_id: uc-create-order-success
angle: unit-test
kind: assertion
nature: computational
output_type: single-shot

uses:
  - go-stdlib

steps:
  - id: probe
    uses: [valid-order-payload]
    run: "true"
```

This sensor and the previous one **both reference `valid-order-payload`** — that's the fixture-reuse demonstration for criterion §11.7.

- [ ] **Step 5: Write `uc-create-order-bad-input-e2e-test.yaml`**

```yaml
schema_version: 1.0.0
id: uc-create-order-bad-input-e2e-test
use_case_id: uc-create-order-bad-input
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot

uses:
  - go-stdlib
  - net-http

steps:
  - id: probe
    uses: [invalid-order-payload]
    run: "true"
```

- [ ] **Step 6: Write `uc-create-order-bad-input-unit-test.yaml`**

```yaml
schema_version: 1.0.0
id: uc-create-order-bad-input-unit-test
use_case_id: uc-create-order-bad-input
angle: unit-test
kind: assertion
nature: computational
output_type: single-shot

uses:
  - go-stdlib

steps:
  - id: probe
    uses: [invalid-order-payload]
    run: "true"
```

- [ ] **Step 7: Add a gate test for sensor loading + grounding**

```go
func TestSensorsLoadAndGround_HttpApi(t *testing.T) {
    sm, err := stack.Load("../http-api-sample/.harness/stack-manifest.yaml")
    if err != nil {
        t.Fatal(err)
    }
    store, err := sensor.LoadDirectory("../http-api-sample/.harness/sensors")
    if err != nil {
        t.Fatalf("load: %v", err)
    }
    // sensor.Grounding signature is in internal/sensor/grounding.go.
    // Adapt the call shape to that API.
    if err := sensor.Ground(store, sm); err != nil {
        t.Fatalf("grounding: %v", err)
    }
}
```

If `sensor.Ground` is not the exact name, check `internal/sensor/grounding.go` for the right symbol and adjust.

- [ ] **Step 8: Run gate test**

```bash
go test -run TestSensorsLoadAndGround_HttpApi ./examples/validator/ -v
```

Expected: PASS. If a sensor field is rejected, adjust against `schemas/examples/sensor/assertion-computational-single.yaml`.

- [ ] **Step 9: Commit**

```bash
git add examples/http-api-sample/.harness/sensors/ examples/validator/validator_test.go
git commit -m "feat(examples/http-api-sample): six sensors across two angles with shared fixtures"
```

---

## Phase 3 — `examples/http-api-sample-broken/`

### Task 3.1: Copy the passing twin + apply the seed bug

**Files:**
- Create: full copy of `examples/http-api-sample/` at `examples/http-api-sample-broken/`
- Modify: `examples/http-api-sample-broken/go.mod` (rename module)
- Modify: `examples/http-api-sample-broken/handlers.go` (remove the 400 branch)
- Modify: `examples/http-api-sample-broken/README.md` (describe the bug)

- [ ] **Step 1: Copy the directory**

```bash
cp -r examples/http-api-sample examples/http-api-sample-broken
```

- [ ] **Step 2: Rename the module in `go.mod`**

```
module example.com/http-api-sample-broken

go 1.24
```

- [ ] **Step 3: Apply the seed bug to `handlers.go`**

Replace the entire `CreateOrderHandler` function with the broken variant:

```go
func CreateOrderHandler(s *Store) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        var body OrderInput
        _ = json.NewDecoder(r.Body).Decode(&body)
        // BUG: missing validation branch — invalid input falls through to 201.
        order := s.Create(body)
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusCreated)
        _ = json.NewEncoder(w).Encode(order)
    })
}
```

(`GetOrderHandler` is unchanged.)

- [ ] **Step 4: Update the README**

```markdown
# http-api-sample-broken

Sibling of `http-api-sample` with a seeded bug used by Track 1 to
exercise the heal flow (plan §11.6).

## The bug

`CreateOrderHandler` is missing its body-validation branch — an empty
or malformed payload yields 201 instead of 400.

## The fix

`heal-fixture/editplan.json` ships a hand-supplied `EditPlan` that
restores the validation branch. The Track 1 heal test pipes this
EditPlan into `/heal` and expects all-pass on iteration 1.

`.harness/` is byte-identical to `http-api-sample/.harness/` — the use
cases describe what the API *should* do; the bug is in the
implementation.
```

- [ ] **Step 5: Verify the sample still compiles**

```bash
cd examples/http-api-sample-broken && go build ./... && cd ../..
```

Expected: clean build (the bug is a missing branch, not a compile error).

- [ ] **Step 6: Commit**

```bash
git add examples/http-api-sample-broken/
git commit -m "feat(examples/http-api-sample-broken): sibling with seed bug (missing 400 branch)"
```

---

### Task 3.2: `heal-fixture/editplan.json`

**Files:**
- Create: `examples/http-api-sample-broken/heal-fixture/editplan.json`

- [ ] **Step 1: Construct the corrected `handlers.go` contents**

The full file with the fix re-applied — same as the passing twin's `handlers.go`. To keep the JSON manageable, generate it programmatically:

```bash
mkdir -p examples/http-api-sample-broken/heal-fixture
```

Then build the JSON with `jq` (or write it manually):

```bash
jq -n \
  --arg path "handlers.go" \
  --arg content "$(cat examples/http-api-sample/handlers.go)" \
  --arg rationale "Add 400 Bad Request branch when required field 'item' is missing from POST /orders body, matching uc-create-order-bad-input expectation." \
  '{files: [{path: $path, op: "write", content: $content}], rationale: $rationale}' \
  > examples/http-api-sample-broken/heal-fixture/editplan.json
```

If `jq` is unavailable, write the JSON manually with the corrected `handlers.go` contents as the `content` field (escape newlines as `\n`, embedded quotes as `\"`).

- [ ] **Step 2: Verify the EditPlan parses as JSON**

```bash
python3 -m json.tool examples/http-api-sample-broken/heal-fixture/editplan.json > /dev/null && echo OK
```

Expected: prints `OK`.

- [ ] **Step 3: Verify the EditPlan's `content` is the passing-twin shape**

```bash
python3 -c "import json,sys; d=json.load(open('examples/http-api-sample-broken/heal-fixture/editplan.json')); content=d['files'][0]['content']; assert 'missing required field: item' in content, 'fix not present in EditPlan'; print('OK')"
```

Expected: prints `OK`.

- [ ] **Step 4: Commit**

```bash
git add examples/http-api-sample-broken/heal-fixture/editplan.json
git commit -m "feat(examples/http-api-sample-broken): commit EditPlan for criterion §11.6 heal test"
```

---

## Phase 4 — `examples/cli-sample/`

### Task 4.1: Sample Go source

**Files:**
- Create: `examples/cli-sample/go.mod`
- Create: `examples/cli-sample/main.go`
- Create: `examples/cli-sample/README.md`

- [ ] **Step 1: Write `go.mod`**

```
module example.com/cli-sample

go 1.24

require github.com/spf13/cobra v1.8.1
```

- [ ] **Step 2: Write `main.go`**

```go
package main

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
)

func main() {
    var name string
    cmd := &cobra.Command{
        Use:   "cli-sample",
        Short: "A minimal Cobra CLI used as a harness archetype-cli subject.",
    }
    greet := &cobra.Command{
        Use:   "greet",
        Short: "Print a greeting.",
        RunE: func(cmd *cobra.Command, args []string) error {
            if name == "" {
                return fmt.Errorf("--name is required")
            }
            fmt.Printf("Hello, %s\n", name)
            return nil
        },
    }
    greet.Flags().StringVar(&name, "name", "", "name to greet")
    cmd.AddCommand(greet)
    if err := cmd.Execute(); err != nil {
        os.Exit(1)
    }
}
```

- [ ] **Step 3: Write `README.md`**

```markdown
# cli-sample

Minimal archetype-`cli` subject used by Track 1 to prove detection
branches downstream sensors by archetype.

## Usage

    cd examples/cli-sample
    go mod tidy
    go run . greet --name World
    # → Hello, World

## What it demonstrates

- Stack detection (archetype=`cli`) → distinct from `http-api`.
- One use case (`uc-greet-by-name`) with one fixture and two sensors
  (`e2e-test` + `unit-test`) — same fixture-reuse property as the
  http-api sample.
```

- [ ] **Step 4: Run `go mod tidy` inside the sample to lock cobra**

```bash
cd examples/cli-sample && go mod tidy && cd ../..
```

Expected: cobra and its deps are added to `go.sum`.

- [ ] **Step 5: Verify the sample compiles and runs**

```bash
cd examples/cli-sample && go run . greet --name Test && cd ../..
```

Expected: prints `Hello, Test`.

- [ ] **Step 6: Commit**

```bash
git add examples/cli-sample/
git commit -m "feat(examples/cli-sample): minimal Cobra CLI subject"
```

---

### Task 4.2: `cli-sample/.harness/` (manifest + 1 use case + 1 fixture + 2 sensors)

**Files:**
- Create: `examples/cli-sample/.harness/stack-manifest.yaml`
- Create: `examples/cli-sample/.harness/fixtures/name.yaml`
- Create: `examples/cli-sample/.harness/use-cases/uc-greet-by-name.yaml`
- Create: `examples/cli-sample/.harness/sensors/uc-greet-by-name-e2e-test.yaml`
- Create: `examples/cli-sample/.harness/sensors/uc-greet-by-name-unit-test.yaml`

> **Schema reference:** `schemas/examples/use-case/cli.yaml` and `schemas/examples/entry-point/cli.yaml` for the archetype-cli shape (entry-point `spec` uses `command:` rather than `method:`/`path:`).

- [ ] **Step 1: Write `stack-manifest.yaml`**

```yaml
schema_version: 1.0.0
archetype: cli
applicable_angles:
  - build
  - unit-test
  - e2e-test
components:
  - schema_version: 1.0.0
    id: go-stdlib
    kind: language-runtime
    name: go
    version: "1.24"
    capabilities:
      - cli-entrypoint
    detection_evidence:
      - file: go.mod
        path: go
        value: "1.24"
  - schema_version: 1.0.0
    id: cobra
    kind: framework
    name: github.com/spf13/cobra
    version: 1.8.1
    capabilities:
      - cli-subcommands
      - flag-parsing
    detection_evidence:
      - file: go.mod
        path: require
        value: github.com/spf13/cobra v1.8.1
```

- [ ] **Step 2: Write `fixtures/name.yaml`**

```yaml
schema_version: 1.0.0
id: name
use_case_id: uc-greet-by-name
role: input
content_type: text/plain
payload: "World"
binding:
  channel: cli
  selector:
    flag: --name
source_refs:
  - path: main.go
    symbol: greet
    reason: "command flag --name is the input channel"
```

- [ ] **Step 3: Write `use-cases/uc-greet-by-name.yaml`**

```yaml
schema_version: 2.0.0
id: uc-greet-by-name
title: "User greets by name via the greet subcommand"
archetype_scope: [cli]

entry_points:
  - id: greet-subcommand
    archetype: cli
    spec:
      command: "cli-sample greet"

given:
  - "The user has a name {{fixtures.name}}"
when:
  - "The user invokes {{entry_points.greet-subcommand}} with --name set"
then:
  - "stdout contains 'Hello, '"
  - "exit code is 0"

source_refs:
  - path: main.go
    symbol: greet
    reason: "detected from cobra subcommand registration"

fixture_ids: [name]
```

- [ ] **Step 4: Write the two sensors**

`uc-greet-by-name-e2e-test.yaml`:

```yaml
schema_version: 1.0.0
id: uc-greet-by-name-e2e-test
use_case_id: uc-greet-by-name
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot

uses:
  - go-stdlib
  - cobra

steps:
  - id: probe
    uses: [name]
    run: "true"
```

`uc-greet-by-name-unit-test.yaml`:

```yaml
schema_version: 1.0.0
id: uc-greet-by-name-unit-test
use_case_id: uc-greet-by-name
angle: unit-test
kind: assertion
nature: computational
output_type: single-shot

uses:
  - go-stdlib

steps:
  - id: probe
    uses: [name]
    run: "true"
```

- [ ] **Step 5: Add a gate test**

```go
func TestCliSampleHarnessLoads(t *testing.T) {
    sm, err := stack.Load("../cli-sample/.harness/stack-manifest.yaml")
    if err != nil {
        t.Fatalf("stack: %v", err)
    }
    fs, err := fixture.LoadDirectory("../cli-sample/.harness/fixtures")
    if err != nil {
        t.Fatalf("fixtures: %v", err)
    }
    ucData, err := os.ReadFile("../cli-sample/.harness/use-cases/uc-greet-by-name.yaml")
    if err != nil {
        t.Fatal(err)
    }
    if _, err := usecase.Load(ucData, fs); err != nil {
        t.Fatalf("use case: %v", err)
    }
    senStore, err := sensor.LoadDirectory("../cli-sample/.harness/sensors")
    if err != nil {
        t.Fatalf("sensors: %v", err)
    }
    if err := sensor.Ground(senStore, sm); err != nil {
        t.Fatalf("grounding: %v", err)
    }
}
```

- [ ] **Step 6: Run gate test**

```bash
go test -run TestCliSampleHarnessLoads ./examples/validator/ -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add examples/cli-sample/.harness/ examples/validator/validator_test.go
git commit -m "feat(examples/cli-sample): .harness/ with 1 use case + 2 sensors"
```

---

## Phase 5 — Track 1 integration tests

### Task 5.1: TestMain + criterion test shells

**Files:**
- Create: `examples/integration_test.go`

- [ ] **Step 1: Write TestMain + criterion stubs**

```go
//go:build integration

package examples_test

import (
    "context"
    "log"
    "os"
    "path/filepath"
    "runtime"
    "testing"
    "time"

    "github.com/iurykrieger/lastro/examples/validator"
)

var (
    skills       *validator.SkillBinaries
    frameworkRoot string
    sampleDirs    = []string{"./http-api-sample", "./http-api-sample-broken", "./cli-sample"}
)

func resolveFrameworkRoot() string {
    _, thisFile, _, _ := runtime.Caller(0)
    return filepath.Clean(filepath.Join(filepath.Dir(thisFile), ".."))
}

func TestMain(m *testing.M) {
    if testing.Short() {
        os.Exit(0)
    }
    frameworkRoot = resolveFrameworkRoot()
    tmp, err := os.MkdirTemp("", "b7-skills-*")
    if err != nil {
        log.Fatalf("mkdtemp: %v", err)
    }
    defer os.RemoveAll(tmp)

    sb, err := validator.NewSkillBinaries(tmp, frameworkRoot)
    if err != nil {
        log.Fatalf("build skills: %v", err)
    }
    skills = sb
    os.Exit(m.Run())
}

// validateCtx returns a context with a per-test timeout so a stuck
// sensor cannot wedge the run.
func validateCtx(t *testing.T) context.Context {
    t.Helper()
    ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
    t.Cleanup(cancel)
    return ctx
}
```

- [ ] **Step 2: Smoke-check the build**

```bash
go test -tags=integration -run xxx_nonexistent ./examples/...
```

Expected: PASS with `--- FAIL` for no matching tests OR just builds cleanly with no tests run. The point is to catch compile errors before adding criterion tests.

- [ ] **Step 3: Commit**

```bash
git add examples/integration_test.go
git commit -m "test(examples): scaffold integration TestMain + helpers"
```

---

### Task 5.2: `TestCriterion1_StackCoverage`

Goal: each sample's stack-manifest declares ≥95% of its `go.mod` direct deps, has a non-empty archetype, and non-empty rationale-style fields (`detection_evidence`).

- [ ] **Step 1: Add the test to `examples/integration_test.go`**

```go
// goModDirectDeps parses go.mod and returns direct dependency module paths.
func goModDirectDeps(t *testing.T, sampleDir string) []string {
    t.Helper()
    b, err := os.ReadFile(filepath.Join(sampleDir, "go.mod"))
    if err != nil {
        t.Fatal(err)
    }
    // Naïve parse: scan for lines starting with "require " or inside
    // a "require (" block; skip "// indirect" lines.
    var deps []string
    inBlock := false
    for _, line := range strings.Split(string(b), "\n") {
        trim := strings.TrimSpace(line)
        if strings.Contains(trim, "// indirect") {
            continue
        }
        if strings.HasPrefix(trim, "require (") {
            inBlock = true
            continue
        }
        if inBlock && trim == ")" {
            inBlock = false
            continue
        }
        if inBlock {
            if fields := strings.Fields(trim); len(fields) >= 2 {
                deps = append(deps, fields[0])
            }
            continue
        }
        if strings.HasPrefix(trim, "require ") {
            if fields := strings.Fields(trim); len(fields) >= 3 {
                deps = append(deps, fields[1])
            }
        }
    }
    return deps
}

func TestCriterion1_StackCoverage(t *testing.T) {
    for _, dir := range sampleDirs {
        t.Run(filepath.Base(dir), func(t *testing.T) {
            sm, err := stack.Load(filepath.Join(dir, ".harness", "stack-manifest.yaml"))
            if err != nil {
                t.Fatalf("load: %v", err)
            }
            if sm.Archetype == "" {
                t.Fatalf("archetype is empty")
            }
            if len(sm.Components) == 0 {
                t.Fatalf("no components declared")
            }
            // Rationale check: every component has at least one detection_evidence entry.
            for _, c := range sm.Components {
                if len(c.DetectionEvidence) == 0 {
                    t.Errorf("component %s: empty detection_evidence (acts as rationale)", c.ID)
                }
            }
            // Coverage: every direct go.mod dep is mentioned in some component
            // (by id or name). Stdlib (go itself) is accepted via the `go-stdlib` id.
            deps := goModDirectDeps(t, dir)
            covered := 0
            for _, dep := range deps {
                if componentMentions(sm, dep) {
                    covered++
                }
            }
            total := len(deps)
            if total == 0 {
                // No third-party deps; only go-stdlib needs coverage. Pass.
                return
            }
            ratio := float64(covered) / float64(total)
            if ratio < 0.95 {
                t.Fatalf("coverage %.2f below 0.95 (covered %d of %d deps: %v)", ratio, covered, total, deps)
            }
        })
    }
}

func componentMentions(sm stack.StackManifest, dep string) bool {
    for _, c := range sm.Components {
        if c.ID == dep || c.Name == dep {
            return true
        }
    }
    return false
}
```

Add imports as needed: `"strings"`, `"github.com/iurykrieger/lastro/internal/stack"`.

> **Field-name caveat:** `stack.StackManifest`'s field names may differ from `Archetype` / `Components` / `DetectionEvidence` / `ID` / `Name`. Check `internal/stack/types.go` and adjust. If `c.DetectionEvidence` is empty in the type, look for `Evidence` or `Refs`.

- [ ] **Step 2: Run the test**

```bash
go test -tags=integration -run TestCriterion1_StackCoverage -v ./examples/...
```

Expected: PASS for all three samples (since you hand-curated them to 100% coverage).

- [ ] **Step 3: Commit**

```bash
git add examples/integration_test.go
git commit -m "test(examples): criterion §11.1 — stack coverage + archetype + rationale"
```

---

### Task 5.3: `TestCriterion2_UseCasePerEntryPoint`

Goal: each use case has non-empty `given`/`when`/`then`, ≥1 entry point, ≥1 fixture, and the entry point's archetype matches the manifest's archetype.

- [ ] **Step 1: Add the test**

```go
func TestCriterion2_UseCasePerEntryPoint(t *testing.T) {
    for _, dir := range sampleDirs {
        t.Run(filepath.Base(dir), func(t *testing.T) {
            sm, err := stack.Load(filepath.Join(dir, ".harness", "stack-manifest.yaml"))
            if err != nil {
                t.Fatal(err)
            }
            fs, err := fixture.LoadDirectory(filepath.Join(dir, ".harness", "fixtures"))
            if err != nil {
                t.Fatal(err)
            }
            entries, err := os.ReadDir(filepath.Join(dir, ".harness", "use-cases"))
            if err != nil {
                t.Fatal(err)
            }
            if len(entries) == 0 {
                t.Fatal("no use cases")
            }
            for _, e := range entries {
                data, err := os.ReadFile(filepath.Join(dir, ".harness", "use-cases", e.Name()))
                if err != nil {
                    t.Fatal(err)
                }
                uc, err := usecase.Load(data, fs)
                if err != nil {
                    t.Fatalf("%s: load: %v", e.Name(), err)
                }
                if len(uc.Given) == 0 {
                    t.Errorf("%s: empty given", uc.ID)
                }
                if len(uc.When) == 0 {
                    t.Errorf("%s: empty when", uc.ID)
                }
                if len(uc.Then) == 0 {
                    t.Errorf("%s: empty then", uc.ID)
                }
                if len(uc.EntryPoints) == 0 {
                    t.Fatalf("%s: no entry points", uc.ID)
                }
                if len(uc.FixtureIDs) == 0 {
                    t.Errorf("%s: no fixture_ids", uc.ID)
                }
                for _, ep := range uc.EntryPoints {
                    if string(ep.Archetype) != string(sm.Archetype) {
                        t.Errorf("%s: entry point %s archetype=%s does not match manifest=%s",
                            uc.ID, ep.ID, ep.Archetype, sm.Archetype)
                    }
                }
            }
        })
    }
}
```

Add import `"github.com/iurykrieger/lastro/internal/fixture"` and `"github.com/iurykrieger/lastro/internal/usecase"`.

> **Field-name caveat:** Check `internal/usecase/usecase.go` for actual field names (`Given`, `When`, `Then`, `EntryPoints`, `FixtureIDs`). Adjust if they differ.

- [ ] **Step 2: Run**

```bash
go test -tags=integration -run TestCriterion2_UseCasePerEntryPoint -v ./examples/...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add examples/integration_test.go
git commit -m "test(examples): criterion §11.2 — use case per entry point with required fields"
```

---

### Task 5.4: `TestCriterion3_TemplateResolution`

Goal: every `{{fixtures.X}}` / `{{entry_points.X}}` reference in use-case text resolves to a defined id in the same use case.

- [ ] **Step 1: Add the test**

```go
func TestCriterion3_TemplateResolution(t *testing.T) {
    for _, dir := range sampleDirs {
        t.Run(filepath.Base(dir), func(t *testing.T) {
            fs, err := fixture.LoadDirectory(filepath.Join(dir, ".harness", "fixtures"))
            if err != nil {
                t.Fatal(err)
            }
            entries, err := os.ReadDir(filepath.Join(dir, ".harness", "use-cases"))
            if err != nil {
                t.Fatal(err)
            }
            for _, e := range entries {
                data, _ := os.ReadFile(filepath.Join(dir, ".harness", "use-cases", e.Name()))
                uc, err := usecase.Load(data, fs)
                if err != nil {
                    t.Fatalf("%s: load: %v", e.Name(), err)
                }
                fixSet := map[string]struct{}{}
                for _, id := range uc.FixtureIDs {
                    fixSet[id] = struct{}{}
                }
                epSet := map[string]struct{}{}
                for _, ep := range uc.EntryPoints {
                    epSet[ep.ID] = struct{}{}
                }
                for _, block := range [][]string{uc.Given, uc.When, uc.Then} {
                    for _, line := range block {
                        segs, err := template.Parse(line)
                        if err != nil {
                            t.Errorf("%s: parse %q: %v", uc.ID, line, err)
                            continue
                        }
                        for _, s := range segs {
                            switch v := s.(type) {
                            case template.FixtureRef:
                                if _, ok := fixSet[v.ID]; !ok {
                                    t.Errorf("%s: fixture %q not in fixture_ids %v", uc.ID, v.ID, uc.FixtureIDs)
                                }
                            case template.EntryPointRef:
                                if _, ok := epSet[v.ID]; !ok {
                                    t.Errorf("%s: entry point %q not in entry_points", uc.ID, v.ID)
                                }
                            }
                        }
                    }
                }
            }
        })
    }
}
```

Add import `"github.com/iurykrieger/lastro/internal/usecase/template"`.

> **Type caveat:** `template.FixtureRef` and `template.EntryPointRef` exist per `internal/usecase/template/*.go`. Check the actual exported names. The ID accessor may be `.ID` or `.Name` — adjust.

- [ ] **Step 2: Run**

```bash
go test -tags=integration -run TestCriterion3_TemplateResolution -v ./examples/...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add examples/integration_test.go
git commit -m "test(examples): criterion §11.3 — template interpolation resolves cleanly"
```

---

### Task 5.5: `TestCriterion4_SensorGrounding`

Goal: every sensor's top-level `uses:` references valid stack-component ids; every step-level `uses:` references fixtures owned by the sensor's use case.

- [ ] **Step 1: Add the test**

```go
func TestCriterion4_SensorGrounding(t *testing.T) {
    for _, dir := range sampleDirs {
        t.Run(filepath.Base(dir), func(t *testing.T) {
            sm, err := stack.Load(filepath.Join(dir, ".harness", "stack-manifest.yaml"))
            if err != nil {
                t.Fatal(err)
            }
            store, err := sensor.LoadDirectory(filepath.Join(dir, ".harness", "sensors"))
            if err != nil {
                t.Fatalf("load sensors: %v", err)
            }
            if err := sensor.Ground(store, sm); err != nil {
                t.Fatalf("grounding: %v", err)
            }
            // Bonus: assert step-level uses references only fixtures owned by
            // the sensor's use case. (Sensor.Ground may or may not check this;
            // we belt-and-suspenders here.)
            fs, err := fixture.LoadDirectory(filepath.Join(dir, ".harness", "fixtures"))
            if err != nil {
                t.Fatal(err)
            }
            for _, s := range store.All() {
                for _, step := range s.Steps {
                    for _, fid := range step.Uses {
                        f, ok := fs.Get(fid)
                        if !ok {
                            t.Errorf("sensor %s step %s: fixture %q not found", s.ID, step.ID, fid)
                            continue
                        }
                        if f.UseCaseID != s.UseCaseID {
                            t.Errorf("sensor %s: step fixture %q is owned by use case %q, not %q",
                                s.ID, fid, f.UseCaseID, s.UseCaseID)
                        }
                    }
                }
            }
        })
    }
}
```

Add import `"github.com/iurykrieger/lastro/internal/sensor"`.

> **API caveats:** `sensor.Ground` may be `sensor.Grounding` or a method on the Store. `store.All()` may be `.List()` or `.Items()`. Check `internal/sensor/store.go`. `f.UseCaseID` field name to verify against `internal/fixture/types.go`. `fs.Get(id)` may be `fs.Lookup(id)`.

- [ ] **Step 2: Run**

```bash
go test -tags=integration -run TestCriterion4_SensorGrounding -v ./examples/...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add examples/integration_test.go
git commit -m "test(examples): criterion §11.4 — sensor grounding + fixture-ownership check"
```

---

### Task 5.6: `TestCriterion5_ValidateExecution_HappyPath`

Goal: `ValidateAll` on the passing http-api sample and the cli sample returns `AllPassed()`.

- [ ] **Step 1: Add the test**

```go
func TestCriterion5_ValidateExecution_HappyPath(t *testing.T) {
    for _, dir := range []string{"./http-api-sample", "./cli-sample"} {
        t.Run(filepath.Base(dir), func(t *testing.T) {
            abs, _ := filepath.Abs(dir)
            // Clean any prior report directories before the run.
            _ = os.RemoveAll(filepath.Join(abs, ".harness", "reports"))

            report, err := validator.ValidateAll(validateCtx(t), abs, skills)
            if err != nil {
                t.Fatalf("ValidateAll: %v", err)
            }
            if !report.AllPassed() {
                t.Fatalf("not all passed: summary=%+v failed=%+v", report.Summary, report.Failed())
            }
            t.Logf("report: %s/.harness/reports/%s/report.json", abs, report.RunID)
        })
    }
}
```

- [ ] **Step 2: Run**

```bash
go test -tags=integration -run TestCriterion5_ValidateExecution_HappyPath -v ./examples/...
```

Expected: PASS. Each invocation builds-once then runs the `/validate-use-case` skill per use case. Sensors run `true` and succeed.

> **If this fails:** likely cause is the executor refusing `run: "true"`. Inspect `cmd/harness/usecase_runner.go` and `internal/runtime/executor/executor.go` for the expected step shape. If steps need `command:` instead of `run:`, or a fully-qualified path, adjust the sensor YAMLs.

- [ ] **Step 3: Commit**

```bash
git add examples/integration_test.go
git commit -m "test(examples): criterion §11.5 happy path — passing samples validate green"
```

---

### Task 5.7: `TestCriterion5_ValidateExecution_FailingPath`

Goal: `ValidateAll` on the broken sample returns exactly one failing use case — `uc-create-order-bad-input` — with a non-nil `HealHint`.

For this to work, the broken sample's `uc-create-order-bad-input-e2e-test` sensor must actually fail when the bug is present. Since the placeholder sensor uses `run: "true"`, it always passes. **The broken sample needs its bad-input sensor to actually exercise the bug.**

Two options:
- **A.** Make the broken sample's `uc-create-order-bad-input-e2e-test` run an inline check: spin up the server, POST `{}`, assert response code is 400. Use `run: "go test -run TestBadInput ./..." ` and ship a small Go test inside the sample.
- **B.** Make the sensor in the broken sample explicitly run a script that fails (`run: "exit 1"`), and the same sensor in the passing sample run `true`. This breaks "byte-identical .harness/" between twins.

Option A keeps the `.harness/` byte-identical and tests the real bug. Pick A.

- [ ] **Step 1: Add a test file to each sample exercising the bad-input path**

In `examples/http-api-sample/sensor_check_test.go` (and a copy in `http-api-sample-broken/`):

```go
package main

import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

// TestBadInputReturns400 is the e2e check exercised by the
// uc-create-order-bad-input-e2e-test sensor. It runs as part of the
// sensor's step.
func TestBadInputReturns400(t *testing.T) {
    s := NewStore()
    h := CreateOrderHandler(s)
    req := httptest.NewRequest("POST", "/orders", strings.NewReader(`{}`))
    w := httptest.NewRecorder()
    h.ServeHTTP(w, req)
    if w.Code != http.StatusBadRequest {
        body, _ := io.ReadAll(w.Result().Body)
        t.Fatalf("want 400, got %d: %s", w.Code, body)
    }
    // Body must be a JSON error envelope.
    var env map[string]any
    if err := json.NewDecoder(bytes.NewReader(w.Body.Bytes())).Decode(&env); err != nil {
        t.Fatalf("non-JSON body: %v: %s", err, w.Body.String())
    }
    if _, ok := env["error"]; !ok {
        t.Fatalf("body missing error field: %s", w.Body.String())
    }
}
```

- [ ] **Step 2: Update the bad-input e2e sensor to actually invoke the test**

In BOTH `examples/http-api-sample/.harness/sensors/uc-create-order-bad-input-e2e-test.yaml` and the broken copy, change the step:

```yaml
steps:
  - id: probe
    uses: [invalid-order-payload]
    run: "go test -run TestBadInputReturns400 ./..."
```

The executor will run this with `cmd.Dir = sample root` (which is where the skill resolves paths). Verify the executor's working directory behavior in `internal/runtime/executor/executor.go` — if it uses a different cwd, the run command may need `cd` prefixed.

- [ ] **Step 3: Run the happy-path test again to confirm passing sample still passes**

```bash
go test -tags=integration -run TestCriterion5_ValidateExecution_HappyPath -v ./examples/...
```

Expected: PASS (passing twin still passes because the test passes against the fixed code).

- [ ] **Step 4: Add the failing-path test**

```go
func TestCriterion5_ValidateExecution_FailingPath(t *testing.T) {
    abs, _ := filepath.Abs("./http-api-sample-broken")
    _ = os.RemoveAll(filepath.Join(abs, ".harness", "reports"))

    report, err := validator.ValidateAll(validateCtx(t), abs, skills)
    if err != nil {
        t.Fatalf("ValidateAll: %v", err)
    }
    failed := report.Failed()
    if len(failed) != 1 {
        t.Fatalf("want exactly 1 failure, got %d: %+v", len(failed), failed)
    }
    if failed[0].UseCaseID != "uc-create-order-bad-input" {
        t.Fatalf("want failing use case = uc-create-order-bad-input, got %s", failed[0].UseCaseID)
    }
    if failed[0].HealHint == nil {
        t.Fatalf("want non-nil HealHint on failure")
    }
    t.Logf("heal hint: %+v", failed[0].HealHint)
}
```

- [ ] **Step 5: Run**

```bash
go test -tags=integration -run TestCriterion5_ValidateExecution_FailingPath -v ./examples/...
```

Expected: PASS. The bad-input sensor in the broken sample runs `go test`, the test fails (200 returned instead of 400), the aggregate verdict is `fail`, `synthesizeHealHint` produces a non-nil hint.

- [ ] **Step 6: Commit**

```bash
git add examples/http-api-sample/sensor_check_test.go examples/http-api-sample-broken/sensor_check_test.go examples/http-api-sample/.harness/sensors/uc-create-order-bad-input-e2e-test.yaml examples/http-api-sample-broken/.harness/sensors/uc-create-order-bad-input-e2e-test.yaml examples/integration_test.go
git commit -m "test(examples): criterion §11.5 failing path — broken sample emits failing aggregate with heal hint"
```

---

### Task 5.8: `TestCriterion7_FixtureReuseAcrossAngles`

Goal: for each sample, some fixture id appears in step-level `uses:` of ≥2 sensors of the same use case with different `angle`s.

- [ ] **Step 1: Add the test**

```go
func TestCriterion7_FixtureReuseAcrossAngles(t *testing.T) {
    for _, dir := range sampleDirs {
        t.Run(filepath.Base(dir), func(t *testing.T) {
            store, err := sensor.LoadDirectory(filepath.Join(dir, ".harness", "sensors"))
            if err != nil {
                t.Fatal(err)
            }
            // For each use case, collect (fixtureID -> set of angles).
            byUC := map[string]map[string]map[string]struct{}{}
            for _, s := range store.All() {
                if _, ok := byUC[s.UseCaseID]; !ok {
                    byUC[s.UseCaseID] = map[string]map[string]struct{}{}
                }
                for _, step := range s.Steps {
                    for _, fid := range step.Uses {
                        if _, ok := byUC[s.UseCaseID][fid]; !ok {
                            byUC[s.UseCaseID][fid] = map[string]struct{}{}
                        }
                        byUC[s.UseCaseID][fid][string(s.Angle)] = struct{}{}
                    }
                }
            }
            // At least one (uc, fid) has ≥2 distinct angles.
            for uc, fixs := range byUC {
                for fid, angles := range fixs {
                    if len(angles) >= 2 {
                        t.Logf("reuse found in %s: fixture %s across angles %v", uc, fid, mapKeys(angles))
                        return
                    }
                }
            }
            t.Fatalf("no fixture is reused across ≥2 angles in any use case for %s", dir)
        })
    }
}

func mapKeys(m map[string]struct{}) []string {
    out := make([]string, 0, len(m))
    for k := range m {
        out = append(out, k)
    }
    return out
}
```

- [ ] **Step 2: Run**

```bash
go test -tags=integration -run TestCriterion7_FixtureReuseAcrossAngles -v ./examples/...
```

Expected: PASS — every sample has at least one shared fixture across e2e-test + unit-test sensors of one use case.

- [ ] **Step 3: Commit**

```bash
git add examples/integration_test.go
git commit -m "test(examples): criterion §11.7 — fixture reuse across ≥2 angles per use case"
```

---

### Task 5.9: `TestCriterion6_HealOnBroken` (heal_test.go)

**Files:**
- Create: `examples/heal_test.go`

- [ ] **Step 1: Write the heal test**

```go
//go:build integration

package examples_test

import (
    "bytes"
    "context"
    "os"
    "os/exec"
    "path/filepath"
    "testing"
    "time"

    "github.com/iurykrieger/lastro/examples/validator"
)

func TestCriterion6_HealOnBroken(t *testing.T) {
    if testing.Short() {
        t.Skip("heal test is slow")
    }

    // 1. Copy the broken sample into a temp dir so the heal mutation
    // does not dirty the source tree.
    src, _ := filepath.Abs("./http-api-sample-broken")
    tmp := t.TempDir()
    if err := validator.CopyDir(src, tmp); err != nil {
        t.Fatalf("copy: %v", err)
    }
    // Strip any inherited runtime/reports state.
    _ = os.RemoveAll(filepath.Join(tmp, ".harness", "runtime"))
    _ = os.RemoveAll(filepath.Join(tmp, ".harness", "reports"))

    ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
    defer cancel()

    // 2. First validation: expect 1 failing UC with a non-nil heal hint.
    report1, err := validator.ValidateAll(ctx, tmp, skills)
    if err != nil {
        t.Fatalf("validate (pre-heal): %v", err)
    }
    failed := report1.Failed()
    if len(failed) != 1 || failed[0].UseCaseID != "uc-create-order-bad-input" {
        t.Fatalf("pre-heal: want 1 fail in uc-create-order-bad-input, got %+v", failed)
    }
    if failed[0].HealHint == nil {
        t.Fatalf("pre-heal: HealHint must be non-nil")
    }

    // 3. Apply the committed EditPlan via /heal.
    editPlan, err := os.ReadFile(filepath.Join(tmp, "heal-fixture", "editplan.json"))
    if err != nil {
        t.Fatalf("read editplan: %v", err)
    }
    cmd := exec.CommandContext(ctx, skills.Heal, "uc-create-order-bad-input")
    cmd.Dir = tmp
    cmd.Stdin = bytes.NewReader(editPlan)
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        t.Fatalf("/heal failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
    }

    // 4. Re-validate; expect all-pass.
    report2, err := validator.ValidateAll(ctx, tmp, skills)
    if err != nil {
        t.Fatalf("validate (post-heal): %v", err)
    }
    if !report2.AllPassed() {
        t.Fatalf("post-heal: not all passed — summary=%+v failed=%+v", report2.Summary, report2.Failed())
    }
}
```

- [ ] **Step 2: Run**

```bash
go test -tags=integration -run TestCriterion6_HealOnBroken -v ./examples/...
```

Expected: PASS. The flow is: copy → validate (1 fail) → /heal (exit 0) → validate (all pass).

> **If this fails:** the /heal skill may reject the EditPlan path because it expects paths relative to *its* notion of repo root. Verify `skills/heal/scripts/main.go`'s path-resolution and adjust the `path` field in `editplan.json` accordingly.

- [ ] **Step 3: Commit**

```bash
git add examples/heal_test.go
git commit -m "test(examples): criterion §11.6 — heal fixes broken sample in one iteration"
```

---

## Phase 6 — Track 2 (dogfood)

### Task 6.1: Produce the framework's own `.harness/` via the detection skills

This task is **procedural, not code-only**. The implementer runs the three inferential slash commands against the framework root, reviews the output, and commits.

- [ ] **Step 1: Ensure detection skills exist and are runnable**

```bash
ls skills/detect-stack skills/detect-use-cases skills/create-sensors
```

Expected: each directory exists with a `skill.md` and a `scripts/` folder.

- [ ] **Step 2: From a fresh checkout of the worktree, run `/detect-stack` against the framework root**

Within Claude Code (or whichever skill runner the team uses), invoke:

```
/detect-stack
```

Run from the framework root (`lastro/`). The skill writes `.harness/stack-manifest.yaml`.

- [ ] **Step 3: Run `/detect-use-cases`**

```
/detect-use-cases
```

Skill writes `.harness/use-cases/*.yaml` and `.harness/fixtures/*.yaml`.

- [ ] **Step 4: Run `/create-sensors`**

```
/create-sensors
```

Skill writes `.harness/sensors/*.yaml`.

- [ ] **Step 5: Hand-review the produced artifacts**

Open each generated YAML and confirm:
- `stack-manifest.yaml` declares `archetype: cli` (the framework's primary archetype is the `harness` binary). `components` include `go-stdlib`, `cobra`, `clbanning/mxj`, `oklog/ulid`, `santhosh-tekuri/jsonschema`, `sigs.k8s.io/yaml`, `golang.org/x/sys`.
- `use-cases/` contains at least one use case per major framework flow — `harness validate` happy path, `/validate-use-case` skill exit codes, `/heal` skill semantics, lifecycle quartet (run/start/tail/stop), aggregator rollup behavior. If any are missing or describe behaviors that don't exist, regenerate or hand-edit (and file a B4 bug if the detection logic is at fault).
- `sensors/` contains sensors grounded against the manifest. Top-level `uses:` references valid component ids.

- [ ] **Step 6: Verify via the validator package**

Run a quick gate to confirm everything loads:

```bash
go test -run 'TestStackManifestLoads_HttpApi' ./examples/validator/ -v
```

Then add a temporary dogfood gate test for the framework's own `.harness/`:

```go
// (Add to examples/validator/validator_test.go temporarily.)
func TestFrameworkHarnessLoads(t *testing.T) {
    fr, _ := filepath.Abs("../..")
    sm, err := stack.Load(filepath.Join(fr, ".harness", "stack-manifest.yaml"))
    if err != nil {
        t.Fatalf("stack: %v", err)
    }
    fs, err := fixture.LoadDirectory(filepath.Join(fr, ".harness", "fixtures"))
    if err != nil {
        t.Fatalf("fixtures: %v", err)
    }
    senStore, err := sensor.LoadDirectory(filepath.Join(fr, ".harness", "sensors"))
    if err != nil {
        t.Fatalf("sensors: %v", err)
    }
    if err := sensor.Ground(senStore, sm); err != nil {
        t.Fatalf("grounding: %v", err)
    }
    entries, err := os.ReadDir(filepath.Join(fr, ".harness", "use-cases"))
    if err != nil {
        t.Fatal(err)
    }
    for _, e := range entries {
        data, _ := os.ReadFile(filepath.Join(fr, ".harness", "use-cases", e.Name()))
        if _, err := usecase.Load(data, fs); err != nil {
            t.Fatalf("usecase %s: %v", e.Name(), err)
        }
    }
}
```

```bash
go test -run TestFrameworkHarnessLoads ./examples/validator/ -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add .harness/
git commit -m "feat(harness): commit framework's own detected .harness/ for dogfood track"
```

The PR description must include a paragraph describing exactly which skill invocations produced this `.harness/`, including any hand-edits made during review.

---

### Task 6.2: `dogfood_test.go` + `TestFrameworkSelfValidation`

**Files:**
- Create: `examples/dogfood_test.go`

- [ ] **Step 1: Write the test**

```go
//go:build dogfood

package examples_test

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "runtime"
    "testing"
    "time"

    "github.com/iurykrieger/lastro/examples/validator"
)

func dogfoodFrameworkRoot() string {
    _, thisFile, _, _ := runtime.Caller(0)
    return filepath.Clean(filepath.Join(filepath.Dir(thisFile), ".."))
}

func TestFrameworkSelfValidation(t *testing.T) {
    if testing.Short() {
        t.Skip("dogfood is slow")
    }
    repoRoot := dogfoodFrameworkRoot()
    work, err := os.MkdirTemp("", "dogfood-skills-*")
    if err != nil {
        t.Fatalf("mkdtemp: %v", err)
    }
    t.Cleanup(func() { _ = os.RemoveAll(work) })

    skills, err := validator.NewSkillBinaries(work, repoRoot)
    if err != nil {
        t.Fatalf("build skills: %v", err)
    }
    ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
    defer cancel()

    _ = os.RemoveAll(filepath.Join(repoRoot, ".harness", "reports"))

    report, err := validator.ValidateAll(ctx, repoRoot, skills)
    if err != nil {
        t.Fatalf("ValidateAll: %v", err)
    }
    if !report.AllPassed() {
        var ids []string
        for _, uc := range report.Failed() {
            ids = append(ids, uc.UseCaseID)
        }
        t.Fatalf("dogfood failed:\n  summary=%+v\n  failed=%v\n  report=%s",
            report.Summary, ids,
            fmt.Sprintf("%s/.harness/reports/%s/report.json", repoRoot, report.RunID))
    }
    t.Logf("dogfood passed: %d use cases all green; report at %s/.harness/reports/%s/report.json",
        report.Summary.Total, repoRoot, report.RunID)
}
```

- [ ] **Step 2: Run**

```bash
go test -tags=dogfood -v -timeout 10m ./examples/...
```

Expected: PASS. Every detected framework use case validates green.

> **If a use case fails:** the framework regressed against a contract its own sensors describe. Either fix the regression in the framework code, or — if the sensor was wrong — regenerate that single use case + sensors via the inferential skills and re-commit.

- [ ] **Step 3: Commit**

```bash
git add examples/dogfood_test.go
git commit -m "test(examples): dogfood gate — framework's own use cases must all pass"
```

---

## Phase 7 — Plumbing

### Task 7.1: `.gitignore` updates

**Files:**
- Modify: `.gitignore` (or create if absent)

- [ ] **Step 1: Append the new exclusions**

Append to `.gitignore`:

```
# Harness framework runtime + report directories (generated, never committed)
.harness/runtime/
.harness/reports/
**/.harness/runtime/
**/.harness/reports/
```

- [ ] **Step 2: Verify nothing already-tracked is now ignored**

```bash
git status --ignored
```

Expected: shows `.harness/runtime/` and `.harness/reports/` as ignored, nothing else regressed.

- [ ] **Step 3: Remove the temporary dogfood gate test added in Task 6.1**

The `TestFrameworkHarnessLoads` test in `examples/validator/validator_test.go` was a one-shot gate. With `dogfood_test.go` in place, it's redundant. Remove it.

- [ ] **Step 4: Commit**

```bash
git add .gitignore examples/validator/validator_test.go
git commit -m "chore(b7): gitignore runtime/reports + drop temporary dogfood gate"
```

---

### Task 7.2: `examples/README.md`

**Files:**
- Create: `examples/README.md`
- Delete: `examples/.gitkeep` (no longer needed)

- [ ] **Step 1: Write `examples/README.md`**

```markdown
# examples/

Synthetic subject repositories used by the harness framework's
integration tests (Track 1) plus the validator helper used by both
test tracks.

## Layout

| Path | Purpose |
|---|---|
| `validator/` | Shared `ValidateAll` Go package. Drives the `/validate-use-case` skill against a target directory and aggregates verdicts into a `Report`. |
| `http-api-sample/` | Canonical passing http-api subject. 1 GET + 1 POST. Three use cases, six sensors. |
| `http-api-sample-broken/` | Sibling with a seeded bug (missing 400 validation branch). Ships `heal-fixture/editplan.json`. |
| `cli-sample/` | Minimal Cobra CLI subject. Proves archetype branching. |
| `integration_test.go` + `heal_test.go` | Track 1 — plan §11 criteria 1-7 (build tag `integration`). |
| `dogfood_test.go` | Track 2 — framework validates itself (build tag `dogfood`). |

## Running the tests

```bash
# Track 1 — synthetic samples + plan §11 criteria + heal acceptance.
go test -tags=integration -v -timeout 5m ./examples/...

# Track 2 — framework dogfood.
go test -tags=dogfood -v -timeout 5m ./examples/...

# Untagged — validator unit tests with fakes.
go test ./examples/validator/...
```

`-short` skips both `-tags=integration` and `-tags=dogfood` passes.

## Reports

Each `ValidateAll` invocation writes a structured report to
`<target>/.harness/reports/<run-id>/report.json`. These directories
are gitignored.

## Adding a new sample

1. Create `examples/<name>/` with its own `go.mod` (module
   `example.com/<name>`).
2. Author the sample source.
3. Hand-curate `.harness/{stack-manifest,fixtures,use-cases,sensors}/`
   following the schemas in `schemas/examples/`.
4. Add the new sample's directory to `sampleDirs` in
   `examples/integration_test.go`.
5. If the new sample exercises a new plan §11 angle, add or extend a
   `TestCriterionN_*` test.
```

- [ ] **Step 2: Delete the placeholder**

```bash
git rm examples/.gitkeep
```

- [ ] **Step 3: Commit**

```bash
git add examples/README.md
git commit -m "docs(examples): explain the two test tracks and how to add a sample"
```

---

### Task 7.3: CI workflow integration

**Files:**
- Modify: `.github/workflows/*.yml` (existing CI configuration)

The exact workflow file depends on the repo's current CI setup. The minimum requirement: both `-tags=integration` and `-tags=dogfood` runs must execute on every PR, and `examples/validator/` unit tests must run as part of the regular test pass.

- [ ] **Step 1: Locate the existing test job**

```bash
ls .github/workflows/ 2>&1
```

If no workflow exists, create `.github/workflows/ci.yml`. If one exists, identify the test job (likely runs `go test ./...`).

- [ ] **Step 2: Add jobs (or steps) for both tracks**

Example additions to a YAML workflow (adjust to the existing structure):

```yaml
  integration:
    name: B7 Track 1 — integration
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.24.2"
      - run: go test -tags=integration -v -timeout 10m ./examples/...

  dogfood:
    name: B7 Track 2 — dogfood
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.24.2"
      - run: go test -tags=dogfood -v -timeout 10m ./examples/...
```

The existing unit-test job already covers `examples/validator/...` via `go test ./...`.

- [ ] **Step 3: Validate the workflow YAML parses**

If `actionlint` is available:

```bash
actionlint .github/workflows/ci.yml
```

Otherwise, push the branch to a draft PR and observe the GitHub Actions UI.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/
git commit -m "ci(b7): add integration + dogfood jobs"
```

---

## Final acceptance check

- [ ] **Step 1: Run the full local test matrix**

```bash
go test ./examples/validator/...
go test -tags=integration -v -timeout 10m ./examples/...
go test -tags=dogfood -v -timeout 10m ./examples/...
```

Expected: all green.

- [ ] **Step 2: Verify deliverable checklist from the spec §11**

- [ ] `go test -tags=integration -v ./examples/...` passes with all 8 test functions green.
- [ ] `go test -tags=dogfood -v ./examples/...` passes with `TestFrameworkSelfValidation` green.
- [ ] `go test ./examples/validator/...` passes.
- [ ] Each `examples/<sample>/README.md` exists and explains the sample.
- [ ] `.gitignore` excludes `.harness/runtime/` and `.harness/reports/`.
- [ ] Framework root's `.harness/` is committed; the PR description includes the manual detection procedure used.
- [ ] CI workflow invokes both tracks as separate jobs.

- [ ] **Step 3: Open the PR**

Title: `feat(b7): examples + dogfood self-validation`

PR description must include:

1. Summary of what landed.
2. The exact slash-command sequence used to produce the framework's `.harness/` (from Task 6.1).
3. A note that `harness heal` CLI remains gated; B7 uses `/heal` skill only.
4. Expected CI behavior: integration + dogfood jobs both green.

---

## Spec coverage cross-check

| Spec section | Plan task(s) |
|---|---|
| §2 In: `examples/http-api-sample/` | 2.1-2.4 |
| §2 In: `examples/http-api-sample-broken/` | 3.1-3.2 |
| §2 In: `examples/cli-sample/` | 4.1-4.2 |
| §2 In: `examples/validator/` | 1.1-1.4 |
| §2 In: integration + heal tests | 5.1-5.9 |
| §2 In: dogfood test | 6.2 |
| §2 In: framework `.harness/` | 6.1 |
| §2 In: `.gitignore` updates | 7.1 |
| §3 ValidateAll primitive | 1.4 |
| §3 Skills not CLI | enforced in 1.3 (only validate-use-case + heal binaries) |
| §4 Layout decisions | 2.1-2.4, 4.1-4.2 (each sample own go.mod), 7.1 (gitignore) |
| §5.1 http-api-sample anatomy | 2.1-2.4 |
| §5.2 broken twin anatomy | 3.1-3.2 |
| §5.3 cli-sample anatomy | 4.1-4.2 |
| §5.4 framework `.harness/` produced manually | 6.1 |
| §6.1 validator API | 1.1, 1.4 |
| §6.2 integration_test.go TestMain | 5.1 |
| §6.3 heal_test.go | 5.9 |
| §6.4 dogfood_test.go | 6.2 |
| §6.5 CI invocations | 7.3 |
| §7.1-7.4 heal flow specifics | 3.1, 3.2, 5.9 |
| §8 criteria mapping | 5.2 (§11.1), 5.3 (§11.2), 5.4 (§11.3), 5.5 (§11.4), 5.6+5.7 (§11.5), 5.9 (§11.6), 5.8 (§11.7), 6.2 (Track 2) |
| §11 deliverable acceptance | Final acceptance check |

No gaps.

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-05-27-b7-examples-dogfood.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

**Which approach?**
