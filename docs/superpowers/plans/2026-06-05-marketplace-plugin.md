# Marketplace Plugin — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Package the harness framework as a self-contained Claude Code marketplace plugin that ships pre-compiled `harness-tools` binaries for five platforms so users need no Go installation.

**Architecture:** A single `cmd/harness-tools` binary consolidates all 10 skill-script subcommands by calling the same `internal/` packages the existing scripts use. Helpers shared between `validate-use-case` and `heal` (policy loading, worst-verdict computation, signal replay) move to `lib/skillruntime`. Existing `skills/<name>/scripts/` remain unchanged for `go run`-based development use. All 10 `SKILL.md` files are updated to invoke the binary via a platform-detection wrapper at `scripts/harness-tools.sh`.

**Tech Stack:** Go 1.24, Cobra (existing), `lib/skillio`, `lib/skillruntime`, `internal/` packages, GNU Make, GitHub Actions.

---

## File Map

### New files
| Path | Responsibility |
|---|---|
| `lib/skillruntime/policy.go` | `LoadPolicies` helper shared by validate-use-case and heal |
| `lib/skillruntime/verdicts.go` | `WorstExitCode`, `WorstAggregateVerdict` shared by validate-use-case and heal |
| `lib/skillruntime/replay.go` | `ReplaySignals` used by run-sensor subcommand |
| `cmd/harness-tools/main.go` | Entry point, `run(args) int` router, usage |
| `cmd/harness-tools/main_test.go` | Smoke tests for router |
| `cmd/harness-tools/persist.go` | `detect-stack`, `detect-use-cases`, `create-sensors`, `create-core-sensors` |
| `cmd/harness-tools/lifecycle.go` | `run-sensor`, `start-sensor`, `stop-sensor`, `tail-sensor-signals` |
| `cmd/harness-tools/validate.go` | `validate-use-case` + private types |
| `cmd/harness-tools/heal.go` | `heal` + private state/envelope types |
| `scripts/harness-tools.sh` | Platform detection wrapper invoked by SKILL.md files |
| `Makefile` | `build-all` and per-platform targets |
| `.github/workflows/release.yml` | Build binaries on tag push, commit to `bin/` |
| `bin/.gitkeep` | Placeholder so `bin/` directory is tracked |

### Modified files
| Path | Change |
|---|---|
| `skills/detect-stack/SKILL.md` | Replace `go run` invocation with wrapper |
| `skills/detect-use-cases/SKILL.md` | Replace `go run` invocation with wrapper |
| `skills/create-sensors/SKILL.md` | Replace `go run` invocation with wrapper |
| `skills/create-core-sensors/SKILL.md` | Replace `go run` invocation with wrapper |
| `skills/run-sensor/SKILL.md` | Replace `go run` invocation with wrapper |
| `skills/start-sensor/SKILL.md` | Replace `go run` invocation with wrapper |
| `skills/stop-sensor/SKILL.md` | Replace `go run` invocation with wrapper |
| `skills/tail-sensor-signals/SKILL.md` | Replace `go run` invocation with wrapper |
| `skills/validate-use-case/SKILL.md` | Replace `go run` invocation with wrapper |
| `skills/heal/SKILL.md` | Replace `go run` invocation with wrapper |

---

## Task 1: Add shared helpers to `lib/skillruntime`

**Files:**
- Create: `lib/skillruntime/policy.go`
- Create: `lib/skillruntime/verdicts.go`
- Create: `lib/skillruntime/replay.go`

- [ ] **Step 1: Create `lib/skillruntime/policy.go`**

```go
package skillruntime

import (
	"os"
	"path/filepath"

	"github.com/iurykrieger/lastro/internal/policy"
)

// LoadPolicies resolves the effective validation policy for a use case.
// Both files are optional; missing or malformed yields an empty policy.
func LoadPolicies(policyDir, useCaseID string) *policy.EffectivePolicy {
	global := loadPolicyFile(filepath.Join(policyDir, "global.yaml"))
	local := loadPolicyFile(filepath.Join(policyDir, "local", useCaseID+".yaml"))
	return policy.Resolve(global, local)
}

func loadPolicyFile(path string) *policy.ValidationPolicy {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	p, err := policy.Load(f)
	if err != nil {
		return nil
	}
	return p
}
```

- [ ] **Step 2: Create `lib/skillruntime/verdicts.go`**

```go
package skillruntime

import (
	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/lib/skillio"
)

// WorstExitCode returns the exit code for ucVerdict but promotes to
// ExitFail if any AggregateSignal has verdict=fail. Prevents a vacuous
// policy (no obligatory angles) from hiding real sensor failures.
func WorstExitCode(ucVerdict enums.Verdict, aggs []aggregate.AggregateSignal) int {
	code := skillio.ExitCodeForVerdict(ucVerdict)
	for _, a := range aggs {
		if a.Verdict == enums.VerdictFail {
			return skillio.ExitFail
		}
		if a.Verdict == enums.VerdictInconclusive && code == skillio.ExitPass {
			code = skillio.ExitInconclusive
		}
	}
	return code
}

// WorstAggregateVerdict returns the worst verdict across all AggregateSignals.
func WorstAggregateVerdict(aggs []aggregate.AggregateSignal) enums.Verdict {
	worst := enums.VerdictPass
	for _, a := range aggs {
		switch a.Verdict {
		case enums.VerdictFail:
			return enums.VerdictFail
		case enums.VerdictInconclusive:
			worst = enums.VerdictInconclusive
		}
	}
	return worst
}
```

- [ ] **Step 3: Create `lib/skillruntime/replay.go`**

```go
package skillruntime

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
)

// ReplaySignals streams the most recent signals.jsonl for sensorID to w.
// Best-effort: a missing file is not an error (single-shot sensors emit
// zero streamed signals before producing the terminal aggregate).
// Run IDs are ULIDs and sort lexically by creation time, so the
// alphabetically-greatest subdirectory of runtimeRoot/sensorID is the
// most recent run.
func ReplaySignals(runtimeRoot, sensorID string, w io.Writer) error {
	sensorDir := filepath.Join(runtimeRoot, sensorID)
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
	f, err := os.Open(filepath.Join(sensorDir, latest, "signals.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if _, err := w.Write(sc.Bytes()); err != nil {
			return err
		}
		if _, err := w.Write([]byte{'\n'}); err != nil {
			return err
		}
	}
	return sc.Err()
}
```

- [ ] **Step 4: Verify the new files compile**

```bash
go build ./lib/skillruntime/...
```
Expected: no output, exit 0.

- [ ] **Step 5: Commit**

```bash
git add lib/skillruntime/policy.go lib/skillruntime/verdicts.go lib/skillruntime/replay.go
git commit -m "feat: add shared policy, verdict, and replay helpers to lib/skillruntime"
```

---

## Task 2: Create `cmd/harness-tools/main.go`

**Files:**
- Create: `cmd/harness-tools/main.go`

- [ ] **Step 1: Create the router**

```go
// Command harness-tools is the single pre-compiled binary backing every
// harness skill script. Each subcommand matches the name of a skill
// (detect-stack, detect-use-cases, etc.) and accepts the same flags and
// positional arguments as the corresponding go-run-able script under
// skills/<name>/scripts/.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}

// run is the testable entry point.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintf(stderr, "usage: harness-tools <subcommand> [args...]\n")
		fmt.Fprintf(stderr, "subcommands:\n")
		fmt.Fprintf(stderr, "  detect-stack        validate and persist a stack-manifest YAML\n")
		fmt.Fprintf(stderr, "  detect-use-cases    validate and persist a use-case or fixture YAML\n")
		fmt.Fprintf(stderr, "  create-sensors      validate and persist a sensor YAML\n")
		fmt.Fprintf(stderr, "  create-core-sensors validate and persist a core sensor YAML\n")
		fmt.Fprintf(stderr, "  run-sensor          run an assertion sensor\n")
		fmt.Fprintf(stderr, "  start-sensor        start an observational sensor\n")
		fmt.Fprintf(stderr, "  stop-sensor         stop an in-flight observational sensor\n")
		fmt.Fprintf(stderr, "  tail-sensor-signals tail a sensor's signals.jsonl\n")
		fmt.Fprintf(stderr, "  validate-use-case   run all sensors for a use case\n")
		fmt.Fprintf(stderr, "  heal                apply an edit plan and re-validate\n")
		return 1
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, "cwd:", err)
		return 1
	}

	sub, rest := args[1], args[1:]
	switch sub {
	case "detect-stack":
		return persistDetectStack(rest, stdout, stderr)
	case "detect-use-cases":
		return persistDetectUseCases(rest, stdout, stderr)
	case "create-sensors":
		return persistCreateSensors(rest, stdout, stderr)
	case "create-core-sensors":
		return persistCreateCoreSensors(rest, stdout, stderr)
	case "run-sensor":
		return runRunSensor(rest, stdin, stdout, stderr, cwd)
	case "start-sensor":
		return runStartSensor(rest, stdin, stdout, stderr, cwd)
	case "stop-sensor":
		return runStopSensor(rest, stdin, stdout, stderr, cwd)
	case "tail-sensor-signals":
		return runTailSensorSignals(rest, stdin, stdout, stderr, cwd)
	case "validate-use-case":
		return runValidateUseCase(rest, stdin, stdout, stderr, cwd)
	case "heal":
		return runHeal(rest, stdin, stdout, stderr, cwd)
	default:
		fmt.Fprintf(stderr, "harness-tools: unknown subcommand %q\n", sub)
		return 1
	}
}
```

- [ ] **Step 2: Verify the file compiles in isolation (missing subcommand functions will produce expected errors)**

```bash
go build ./cmd/harness-tools/ 2>&1 | head -5
```
Expected: undefined function errors for `persistDetectStack` etc. — that's fine for now.

---

## Task 3: Create `cmd/harness-tools/persist.go`

**Files:**
- Create: `cmd/harness-tools/persist.go`

- [ ] **Step 1: Create the four persist subcommand handlers**

```go
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/persisterror"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/stack"
	"github.com/iurykrieger/lastro/internal/usecase"
)

func persistDetectStack(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("detect-stack", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "Path to the LLM-emitted stack-manifest YAML")
	harnessDir := fs.String("harness-dir", ".harness", "Target .harness directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	if *file == "" {
		fmt.Fprintln(stderr, "missing required --file")
		return 1
	}
	content, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(stderr, "read input:", err)
		return 1
	}
	if err := stack.Persist(content, *harnessDir); err != nil {
		var pe *persisterror.Error
		if errors.As(err, &pe) {
			_ = json.NewEncoder(stdout).Encode(pe)
			return 2
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func persistDetectUseCases(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("detect-use-cases", flag.ContinueOnError)
	fs.SetOutput(stderr)
	entityType := fs.String("type", "", "Entity type: fixture | use-case")
	file := fs.String("file", "", "Path to the LLM-emitted YAML")
	harnessDir := fs.String("harness-dir", ".harness", "Target .harness directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	if *file == "" {
		fmt.Fprintln(stderr, "missing required --file")
		return 1
	}
	content, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(stderr, "read input:", err)
		return 1
	}
	var persistErr error
	switch *entityType {
	case "fixture":
		persistErr = fixture.Persist(content, *harnessDir)
	case "use-case":
		persistErr = usecase.Persist(content, *harnessDir)
	default:
		fmt.Fprintf(stderr, "invalid --type %q (want fixture or use-case)\n", *entityType)
		return 1
	}
	if persistErr != nil {
		var pe *persisterror.Error
		if errors.As(persistErr, &pe) {
			_ = json.NewEncoder(stdout).Encode(pe)
			return 2
		}
		fmt.Fprintln(stderr, persistErr)
		return 1
	}
	return 0
}

func persistCreateSensors(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("create-sensors", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "Path to the LLM-emitted sensor YAML")
	harnessDir := fs.String("harness-dir", ".harness", "Target .harness directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	if *file == "" {
		fmt.Fprintln(stderr, "missing required --file")
		return 1
	}
	content, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(stderr, "read input:", err)
		return 1
	}
	s, err := sensor.LoadSensorBytes(content)
	if err != nil {
		pe := &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "sensor", Message: err.Error()}
		_ = json.NewEncoder(stdout).Encode(pe)
		return 2
	}
	coreDir := filepath.Join(*harnessDir, "sensors", "core")
	if info, statErr := os.Stat(coreDir); statErr == nil && info.IsDir() {
		coreStore, loadErr := sensor.LoadDirectory(coreDir)
		if loadErr != nil {
			fmt.Fprintln(stderr, "composition validation: load core sensors:", loadErr)
			return 1
		}
		store, storeErr := sensor.NewStore(append(coreStore.All(), s)...)
		if storeErr != nil {
			fmt.Fprintln(stderr, "composition validation: build store:", storeErr)
			return 1
		}
		if compErr := sensor.ValidateComposition(s, store); compErr != nil {
			pe := &persisterror.Error{
				Kind: persisterror.SchemaViolation, EntityType: "sensor", EntityID: s.ID,
				Message: "composition validation: " + compErr.Error(),
			}
			_ = json.NewEncoder(stdout).Encode(pe)
			return 2
		}
	} else {
		fmt.Fprintln(stderr, "warning: .harness/sensors/core/ not found; skipping composition validation")
	}
	if err := sensor.Persist(content, *harnessDir); err != nil {
		var pe *persisterror.Error
		if errors.As(err, &pe) {
			_ = json.NewEncoder(stdout).Encode(pe)
			return 2
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func persistCreateCoreSensors(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("create-core-sensors", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "Path to the LLM-emitted sensor YAML")
	harnessDir := fs.String("harness-dir", ".harness", "Target .harness directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	if *file == "" {
		fmt.Fprintln(stderr, "missing required --file")
		return 1
	}
	content, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(stderr, "read input:", err)
		return 1
	}
	s, err := sensor.LoadSensorBytes(content)
	if err != nil {
		pe := &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "sensor", Message: err.Error()}
		_ = json.NewEncoder(stdout).Encode(pe)
		return 2
	}
	if s.Scope != enums.ScopeCore {
		pe := &persisterror.Error{
			Kind: persisterror.SchemaViolation, EntityType: "sensor", EntityID: s.ID,
			Message: fmt.Sprintf("scope must be %q for a core sensor, got %q", enums.ScopeCore, s.Scope),
		}
		_ = json.NewEncoder(stdout).Encode(pe)
		return 2
	}
	if s.UseCaseID != "" {
		pe := &persisterror.Error{
			Kind: persisterror.SchemaViolation, EntityType: "sensor", EntityID: s.ID,
			Message: fmt.Sprintf("use_case_id must be empty for a core sensor, got %q", s.UseCaseID),
		}
		_ = json.NewEncoder(stdout).Encode(pe)
		return 2
	}
	if err := sensor.Persist(content, *harnessDir); err != nil {
		var pe *persisterror.Error
		if errors.As(err, &pe) {
			_ = json.NewEncoder(stdout).Encode(pe)
			return 2
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
```

- [ ] **Step 2: Verify the package compiles (excluding the missing lifecycle.go and validate.go)**

```bash
go build ./cmd/harness-tools/ 2>&1 | grep -v "undefined:"
```
Expected: only "undefined:" lines for `runRunSensor` etc. — no syntax or import errors.

---

## Task 4: Create `cmd/harness-tools/lifecycle.go`

**Files:**
- Create: `cmd/harness-tools/lifecycle.go`

- [ ] **Step 1: Create the four lifecycle subcommand handlers**

```go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/lifecycle"
	"github.com/iurykrieger/lastro/lib/skillio"
	"github.com/iurykrieger/lastro/lib/skillruntime"
)

func runRunSensor(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
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
	if s.Kind == enums.KindObservational {
		skillio.EmitError(stderr, "wrong-kind", "use /start-sensor for observational sensors", map[string]any{"sensor_id": sensorID})
		return skillio.ExitScriptError
	}

	agg, err := b.Lifecycle.RunSensor(context.Background(), sensorID, nil)
	if err != nil {
		if errors.Is(err, lifecycle.ErrSensorNotFound) {
			skillio.EmitError(stderr, "sensor-not-found", err.Error(), map[string]any{"sensor_id": sensorID})
			return skillio.ExitScriptError
		}
		skillio.EmitError(stderr, "run-failed", err.Error(), map[string]any{"sensor_id": sensorID})
		return skillio.ExitScriptError
	}
	if err := skillruntime.ReplaySignals(b.RuntimeRoot, sensorID, stdout); err != nil {
		skillio.EmitError(stderr, "replay-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	if err := skillio.EmitJSON(stdout, agg); err != nil {
		skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	return skillio.ExitCodeForVerdict(agg.Verdict)
}

func runStartSensor(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
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

	if skillruntime.IsWatchMode(args) {
		if err := skillruntime.RunWatcherMode(b, args); err != nil {
			skillio.EmitError(stderr, "watch-failed", err.Error(), nil)
			return skillio.ExitScriptError
		}
		return skillio.ExitPass
	}

	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected sensor-id as first argument", nil)
		return skillio.ExitScriptError
	}
	sensorID := args[1]
	s, ok := b.Sensors.LookupSensor(sensorID)
	if !ok {
		skillio.EmitError(stderr, "sensor-not-found", fmt.Sprintf("no sensor %q", sensorID), map[string]any{"sensor_id": sensorID})
		return skillio.ExitScriptError
	}
	if s.Kind != enums.KindObservational {
		skillio.EmitError(stderr, "wrong-kind", "use /run-sensor for assertion sensors", map[string]any{"sensor_id": sensorID})
		return skillio.ExitScriptError
	}
	h, err := skillruntime.StartDetachedSensor(b, repoRoot, sensorID, nil)
	if err != nil {
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

func runStopSensor(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
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

func runTailSensorSignals(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected '<sensor-id>:<run-id>' as first argument", nil)
		return skillio.ExitScriptError
	}
	handle := args[1]
	fs := flag.NewFlagSet("tail-sensor-signals", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	follow := fs.Bool("follow", false, "tail new signals as they arrive")
	since := fs.Int("since", 0, "start emitting at line N (1-indexed; 0 = from the beginning)")
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
	harnessDir := skillio.HarnessDir(repoRoot)
	signalsPath := filepath.Join(harnessDir, "runtime", sensorID, runID, "signals.jsonl")
	if *follow {
		if err := tailFollowLoop(signalsPath, *since, sensorID, runID, harnessDir, stdout); err != nil {
			skillio.EmitError(stderr, "follow-failed", err.Error(), map[string]any{"path": signalsPath})
			return skillio.ExitScriptError
		}
		return skillio.ExitPass
	}
	if _, err := tailSnapshot(signalsPath, *since, stdout); err != nil {
		skillio.EmitError(stderr, "read-failed", err.Error(), map[string]any{"path": signalsPath})
		return skillio.ExitScriptError
	}
	return skillio.ExitPass
}

// tailSnapshot reads signals.jsonl skipping the first `since` lines.
// Returns the number of lines emitted.
func tailSnapshot(path string, since int, stdout io.Writer) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	idx, emitted := 0, 0
	for sc.Scan() {
		idx++
		if since > 0 && idx < since {
			continue
		}
		if _, err := stdout.Write(sc.Bytes()); err != nil {
			return emitted, err
		}
		if _, err := stdout.Write([]byte{'\n'}); err != nil {
			return emitted, err
		}
		emitted++
	}
	return emitted, sc.Err()
}

// tailFollowLoop tails signalsPath: emits existing lines (after `since`),
// then polls every 200ms until the sensor leaves running_sensors.json AND
// no new bytes have arrived for 1 s.
func tailFollowLoop(signalsPath string, since int, sensorID, runID, harnessDir string, stdout io.Writer) error {
	if _, err := tailSnapshot(signalsPath, since, stdout); err != nil {
		return err
	}
	f, err := os.Open(signalsPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	const pollInterval = 200 * time.Millisecond
	const quietWindow = 1 * time.Second
	registryPath := filepath.Join(harnessDir, "runtime", "running_sensors.json")
	lastEmittedAt := time.Now()
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
			continue
		}
		if err != io.EOF {
			return err
		}
		alive := tailIsInRegistry(registryPath, sensorID, runID)
		if !alive && time.Since(lastEmittedAt) > quietWindow {
			return nil
		}
		time.Sleep(pollInterval)
	}
}

func tailIsInRegistry(path, sensorID, runID string) bool {
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

- [ ] **Step 2: Verify lifecycle.go compiles (only validate.go and heal.go still missing)**

```bash
go build ./cmd/harness-tools/ 2>&1 | grep "undefined:"
```
Expected: only `runValidateUseCase` and `runHeal` undefined.

---

## Task 5: Create `cmd/harness-tools/validate.go`

**Files:**
- Create: `cmd/harness-tools/validate.go`

- [ ] **Step 1: Create the validate-use-case handler and its private types**

```go
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	aggregator "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/lib/skillio"
	"github.com/iurykrieger/lastro/lib/skillruntime"
	"github.com/oklog/ulid/v2"
)

type persistedVerdict struct {
	UseCaseVerdict aggregator.UseCaseVerdict `json:"use_case_verdict"`
	UseCaseRunID   string                    `json:"use_case_run_id"`
	SensorRuns     []sensorRun               `json:"sensor_runs"`
}

type sensorRun struct {
	SensorID string `json:"sensor_id"`
	Verdict  string `json:"verdict"`
}

func runValidateUseCase(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected use-case-id as first argument", nil)
		return skillio.ExitScriptError
	}
	useCaseID := args[1]

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

	uc, ok := b.UseCases[useCaseID]
	if !ok {
		skillio.EmitError(stderr, "use-case-not-found", fmt.Sprintf("no use case %q", useCaseID), map[string]any{"use_case_id": useCaseID})
		return skillio.ExitScriptError
	}
	if len(uc.ArchetypeScope) == 0 {
		skillio.EmitError(stderr, "no-archetype", "use case has empty archetype_scope", map[string]any{"use_case_id": useCaseID})
		return skillio.ExitScriptError
	}
	archetype := uc.ArchetypeScope[0]

	sensors := b.Sensors.GatherForUseCase(useCaseID)
	pol := skillruntime.LoadPolicies(filepath.Join(skillio.HarnessDir(repoRoot), "policy"), useCaseID)

	runner := skillruntime.SensorRunner(func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		return b.Lifecycle.RunSensor(ctx, s.ID, nil)
	})
	aggs, err := skillruntime.RunAll(context.Background(), sensors, runner, runtime.NumCPU())
	if err != nil {
		skillio.EmitError(stderr, "scheduler-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	for _, a := range aggs {
		if err := skillio.EmitJSON(stdout, a); err != nil {
			skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
			return skillio.ExitScriptError
		}
	}

	verdict, err := aggregator.UseCase(uc, archetype, aggs, sensors, pol)
	if err != nil {
		skillio.EmitError(stderr, "aggregate-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	ucRunID := validateNewULID()
	pv := persistedVerdict{
		UseCaseVerdict: verdict,
		UseCaseRunID:   ucRunID,
		SensorRuns:     make([]sensorRun, 0, len(aggs)),
	}
	for _, a := range aggs {
		pv.SensorRuns = append(pv.SensorRuns, sensorRun{SensorID: a.SensorID, Verdict: string(a.Verdict)})
	}
	verdictPath := filepath.Join(b.RuntimeRoot, "use-cases", useCaseID, ucRunID, "verdict.json")
	if err := validateWriteVerdict(verdictPath, pv); err != nil {
		skillio.EmitError(stderr, "persist-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	if err := skillio.EmitJSON(stdout, pv); err != nil {
		skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	return skillruntime.WorstExitCode(verdict.Verdict, aggs)
}

func validateWriteVerdict(path string, v persistedVerdict) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func validateNewULID() string {
	ms := ulid.Timestamp(time.Now())
	id, _ := ulid.New(ms, rand.Reader)
	return id.String()
}
```

- [ ] **Step 2: Verify validate.go compiles (only heal.go still missing)**

```bash
go build ./cmd/harness-tools/ 2>&1 | grep "undefined:"
```
Expected: only `runHeal` undefined.

---

## Task 6: Create `cmd/harness-tools/heal.go`

**Files:**
- Create: `cmd/harness-tools/heal.go`

- [ ] **Step 1: Create the heal handler with its private types**

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	aggregator "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
	"github.com/iurykrieger/lastro/internal/runtime/healloop"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/lib/skillio"
	"github.com/iurykrieger/lastro/lib/skillruntime"
)

const healDefaultMaxIterations = 3

type healState struct {
	UseCaseID     string        `json:"use_case_id"`
	Iteration     int           `json:"iteration"`
	MaxIterations int           `json:"max_iterations"`
	History       []healAttempt `json:"history"`
}

type healAttempt struct {
	AppliedAt time.Time `json:"applied_at"`
	Rationale string    `json:"rationale"`
	Verdict   string    `json:"verdict"`
}

type healEnvelope struct {
	Status        string                    `json:"status"`
	Iteration     int                       `json:"iteration"`
	MaxIterations int                       `json:"max_iterations"`
	Verdict       aggregator.UseCaseVerdict `json:"verdict"`
	AppliedFiles  []string                  `json:"applied_files"`
	Rationale     string                    `json:"rationale,omitempty"`
}

func runHeal(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected use-case-id as first argument", nil)
		return skillio.ExitScriptError
	}
	useCaseID := args[1]

	plan, err := healDecodePlan(stdin)
	if err != nil {
		skillio.EmitError(stderr, "bad-edit-plan", err.Error(), nil)
		return skillio.ExitScriptError
	}
	if err := healValidatePaths(plan); err != nil {
		skillio.EmitError(stderr, "bad-edit-plan", err.Error(), nil)
		return skillio.ExitScriptError
	}

	repoRoot, err := skillio.FindRepoRoot(cwd)
	if err != nil {
		skillio.EmitError(stderr, "repo-root-not-found", err.Error(), nil)
		return skillio.ExitScriptError
	}

	statePath := filepath.Join(skillio.HarnessDir(repoRoot), "runtime", "heal-state.json")
	state := healLoadState(statePath, useCaseID)
	if state.Iteration >= state.MaxIterations {
		skillio.EmitError(stderr, "heal-exhausted", "heal iteration cap reached", map[string]any{
			"iteration": state.Iteration, "max_iterations": state.MaxIterations, "use_case_id": useCaseID,
		})
		return skillio.ExitScriptError
	}

	b, err := skillruntime.BootLifecycle(repoRoot)
	if err != nil {
		skillio.EmitError(stderr, "boot-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	defer func() { _ = b.Cleanup() }()

	uc, ok := b.UseCases[useCaseID]
	if !ok {
		skillio.EmitError(stderr, "use-case-not-found", fmt.Sprintf("no use case %q", useCaseID), map[string]any{"use_case_id": useCaseID})
		return skillio.ExitScriptError
	}
	if len(uc.ArchetypeScope) == 0 {
		skillio.EmitError(stderr, "no-archetype", "use case has empty archetype_scope", nil)
		return skillio.ExitScriptError
	}
	archetype := uc.ArchetypeScope[0]
	sensors := b.Sensors.ForUseCase(useCaseID)
	pol := skillruntime.LoadPolicies(filepath.Join(skillio.HarnessDir(repoRoot), "policy"), useCaseID)

	snap, err := healSnapshot(repoRoot, plan)
	if err != nil {
		skillio.EmitError(stderr, "snapshot-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	if err := healApplyPlan(repoRoot, plan); err != nil {
		_ = healRestore(snap)
		skillio.EmitError(stderr, "apply-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	healRunner := skillruntime.SensorRunner(func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		return b.Lifecycle.RunSensor(ctx, s.ID, nil)
	})
	aggs, err := skillruntime.RunAll(context.Background(), sensors, healRunner, runtime.NumCPU())
	if err != nil {
		_ = healRestore(snap)
		skillio.EmitError(stderr, "scheduler-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	for _, a := range aggs {
		_ = skillio.EmitJSON(stdout, a)
	}

	verdict, aggErr := aggregator.UseCase(uc, archetype, aggs, sensors, pol)
	if aggErr != nil {
		_ = healRestore(snap)
		skillio.EmitError(stderr, "aggregate-failed", aggErr.Error(), nil)
		return skillio.ExitScriptError
	}

	envelope := healEnvelope{
		Iteration: state.Iteration + 1, MaxIterations: state.MaxIterations,
		Verdict: verdict, Rationale: plan.Rationale,
	}
	for _, f := range plan.Files {
		envelope.AppliedFiles = append(envelope.AppliedFiles, f.Path)
	}

	worst := skillruntime.WorstAggregateVerdict(aggs)
	healed := verdict.Verdict == enums.VerdictPass && worst == enums.VerdictPass
	if healed {
		envelope.Status = "healed"
		state.Iteration = 0
		state.History = nil
	} else {
		envelope.Status = "reverted"
		envelope.AppliedFiles = nil
		if err := healRestore(snap); err != nil {
			skillio.EmitError(stderr, "revert-failed", err.Error(), nil)
			return skillio.ExitScriptError
		}
		state.Iteration++
		state.History = append(state.History, healAttempt{
			AppliedAt: time.Now().UTC(), Rationale: plan.Rationale, Verdict: string(worst),
		})
	}
	state.UseCaseID = useCaseID
	if err := healSaveState(statePath, state); err != nil {
		skillio.EmitError(stderr, "persist-state-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	_ = skillio.EmitJSON(stdout, envelope)

	if healed {
		return skillio.ExitPass
	}
	if worst == enums.VerdictInconclusive {
		return skillio.ExitInconclusive
	}
	return skillio.ExitFail
}

func healDecodePlan(r io.Reader) (healloop.EditPlan, error) {
	if r == nil {
		return healloop.EditPlan{}, errors.New("stdin is nil")
	}
	var plan healloop.EditPlan
	if err := json.NewDecoder(r).Decode(&plan); err != nil {
		return healloop.EditPlan{}, err
	}
	if len(plan.Files) == 0 {
		return healloop.EditPlan{}, errors.New("edit plan has no files")
	}
	for _, f := range plan.Files {
		switch f.Op {
		case healloop.OpWrite, healloop.OpDelete:
		case "":
			return healloop.EditPlan{}, fmt.Errorf("edit file %q has no op", f.Path)
		default:
			return healloop.EditPlan{}, fmt.Errorf("edit file %q has unknown op %q", f.Path, f.Op)
		}
	}
	return plan, nil
}

func healValidatePaths(plan healloop.EditPlan) error {
	for _, f := range plan.Files {
		if f.Path == "" {
			return errors.New("edit file has empty path")
		}
		if filepath.IsAbs(f.Path) {
			return fmt.Errorf("edit file %q must be repo-root-relative", f.Path)
		}
		clean := filepath.ToSlash(filepath.Clean(f.Path))
		if strings.HasPrefix(clean, "../") || clean == ".." {
			return fmt.Errorf("edit file %q escapes repo root", f.Path)
		}
	}
	return nil
}

type healFileSnapshot struct {
	Path    string
	Existed bool
	Bytes   []byte
}

func healSnapshot(repoRoot string, plan healloop.EditPlan) ([]healFileSnapshot, error) {
	snaps := make([]healFileSnapshot, 0, len(plan.Files))
	for _, f := range plan.Files {
		abs := filepath.Join(repoRoot, f.Path)
		bs, err := os.ReadFile(abs)
		if err == nil {
			snaps = append(snaps, healFileSnapshot{Path: abs, Existed: true, Bytes: bs})
		} else if os.IsNotExist(err) {
			snaps = append(snaps, healFileSnapshot{Path: abs})
		} else {
			return nil, fmt.Errorf("snapshot %s: %w", f.Path, err)
		}
	}
	return snaps, nil
}

func healApplyPlan(repoRoot string, plan healloop.EditPlan) error {
	for _, f := range plan.Files {
		abs := filepath.Join(repoRoot, f.Path)
		switch f.Op {
		case healloop.OpWrite:
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(abs, []byte(f.Content), 0o644); err != nil {
				return err
			}
		case healloop.OpDelete:
			if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func healRestore(snaps []healFileSnapshot) error {
	var first error
	for _, s := range snaps {
		if s.Existed {
			if err := os.WriteFile(s.Path, s.Bytes, 0o644); err != nil && first == nil {
				first = err
			}
			continue
		}
		if err := os.Remove(s.Path); err != nil && !os.IsNotExist(err) && first == nil {
			first = err
		}
	}
	return first
}

func healLoadState(path, useCaseID string) healState {
	bs, err := os.ReadFile(path)
	if err != nil {
		return healState{UseCaseID: useCaseID, MaxIterations: healDefaultMaxIterations}
	}
	var s healState
	if err := json.Unmarshal(bs, &s); err != nil {
		return healState{UseCaseID: useCaseID, MaxIterations: healDefaultMaxIterations}
	}
	if s.MaxIterations == 0 {
		s.MaxIterations = healDefaultMaxIterations
	}
	if s.UseCaseID == "" {
		s.UseCaseID = useCaseID
	}
	return s
}

func healSaveState(path string, s healState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	bs, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bs, 0o644)
}
```

- [ ] **Step 2: Full build check**

```bash
go build ./cmd/harness-tools/
```
Expected: exit 0, binary at `cmd/harness-tools/harness-tools` (or similar). No output means success.

- [ ] **Step 3: Commit**

```bash
git add cmd/harness-tools/
git commit -m "feat: add cmd/harness-tools unified binary with all 10 subcommands"
```

---

## Task 7: Tests for `cmd/harness-tools`

**Files:**
- Create: `cmd/harness-tools/main_test.go`

The router is the one unit testable in isolation (it doesn't need a real `.harness/` directory). Test the no-args and unknown-subcommand paths.

- [ ] **Step 1: Write the tests**

```go
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_NoArgs(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"harness-tools"}, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "usage") {
		t.Fatalf("expected usage in stderr, got: %q", stderr.String())
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"harness-tools", "does-not-exist"}, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown subcommand") {
		t.Fatalf("expected 'unknown subcommand' in stderr, got: %q", stderr.String())
	}
}
```

- [ ] **Step 2: Run the tests**

```bash
go test ./cmd/harness-tools/ -run TestRun -v
```
Expected:
```
--- PASS: TestRun_NoArgs (0.00s)
--- PASS: TestRun_UnknownSubcommand (0.00s)
PASS
```

- [ ] **Step 3: Commit**

```bash
git add cmd/harness-tools/main_test.go
git commit -m "test: add smoke tests for harness-tools router"
```

---

## Task 8: Create `scripts/harness-tools.sh`

**Files:**
- Create: `scripts/harness-tools.sh`

- [ ] **Step 1: Create the platform detection wrapper**

```bash
#!/usr/bin/env bash
# harness-tools.sh — platform-aware launcher for the harness-tools binary.
# Usage: harness-tools.sh <subcommand> [args...]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_DIR="$(dirname "$SCRIPT_DIR")"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)        ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
esac
case "$OS" in
  darwin|linux) BIN="${PLUGIN_DIR}/bin/${OS}-${ARCH}/harness-tools" ;;
  msys*|mingw*|cygwin*|windows*) BIN="${PLUGIN_DIR}/bin/windows-amd64/harness-tools.exe" ;;
  *)
    echo "harness-tools.sh: unsupported platform ${OS}-${ARCH}" >&2
    exit 1
    ;;
esac

if [[ ! -x "$BIN" ]]; then
  echo "harness-tools.sh: binary not found at ${BIN}" >&2
  echo "Build with: make build-all  (from ${PLUGIN_DIR})" >&2
  exit 1
fi
exec "$BIN" "$@"
```

- [ ] **Step 2: Make it executable**

```bash
chmod +x scripts/harness-tools.sh
```

- [ ] **Step 3: Smoke test (requires a local build first — skip if binaries not yet built)**

```bash
# Only run this after Task 9 Step 2 (make build-all)
./scripts/harness-tools.sh 2>&1 || true
```
Expected: prints usage and exits 1.

- [ ] **Step 4: Commit**

```bash
git add scripts/harness-tools.sh
git commit -m "feat: add platform detection wrapper script"
```

---

## Task 9: Create `Makefile`

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Create the Makefile**

```makefile
BINARY  := harness-tools
OUTDIR  := bin
GOFLAGS := -trimpath -ldflags="-s -w"
PKG     := ./cmd/harness-tools/

PLATFORMS := darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 windows-amd64

.PHONY: build-all test clean $(PLATFORMS)

build-all: $(PLATFORMS)

darwin-arm64:
	GOOS=darwin  GOARCH=arm64 go build $(GOFLAGS) -o $(OUTDIR)/darwin-arm64/$(BINARY)      $(PKG)

darwin-amd64:
	GOOS=darwin  GOARCH=amd64 go build $(GOFLAGS) -o $(OUTDIR)/darwin-amd64/$(BINARY)      $(PKG)

linux-amd64:
	GOOS=linux   GOARCH=amd64 go build $(GOFLAGS) -o $(OUTDIR)/linux-amd64/$(BINARY)       $(PKG)

linux-arm64:
	GOOS=linux   GOARCH=arm64 go build $(GOFLAGS) -o $(OUTDIR)/linux-arm64/$(BINARY)       $(PKG)

windows-amd64:
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -o $(OUTDIR)/windows-amd64/$(BINARY).exe $(PKG)

test:
	go test ./...

clean:
	rm -rf $(OUTDIR)/darwin-* $(OUTDIR)/linux-* $(OUTDIR)/windows-*
```

- [ ] **Step 2: Create `bin/.gitkeep` so the directory is tracked**

```bash
mkdir -p bin
touch bin/.gitkeep
```

- [ ] **Step 3: Build all platforms**

```bash
make build-all
```
Expected: 5 binaries created under `bin/`. Each `ls -lh bin/*/harness-tools*` should show ~5–10 MB per file.

- [ ] **Step 4: Test the wrapper smoke test (from Task 8 Step 3)**

```bash
./scripts/harness-tools.sh 2>&1 || true
```
Expected: usage printed, exit 1.

- [ ] **Step 5: Commit**

```bash
git add Makefile bin/
git commit -m "chore: add Makefile and initial harness-tools binaries for all platforms"
```

---

## Task 10: Update all 10 SKILL.md files

Each `SKILL.md` currently instructs Claude to run `go run ./skills/<name>/scripts/ [flags]`. Replace every such invocation with an equivalent call via `scripts/harness-tools.sh`.

**Convention for SKILL.md invocations:**

The skill files are loaded from the plugin's `skills/<name>/SKILL.md`. The wrapper is at `<plugin-root>/scripts/harness-tools.sh`. The SKILL.md instructs Claude using the path:

```
<plugin-root>/scripts/harness-tools.sh <subcommand> [flags]
```

where `<plugin-root>` is the directory containing `bin/`, `scripts/`, and `skills/` — i.e., two levels up from the skill file itself (`../../`).

The instruction block in each skill should read:

```
Run (replace `<plugin-root>` with the directory two levels above this skill file,
e.g. `~/.claude/plugins/lastro-harness/`):
```bash
<plugin-root>/scripts/harness-tools.sh <subcommand> [flags]
```
```

- [ ] **Step 1: Update `skills/detect-stack/SKILL.md`**

Find every occurrence of:
```
go run ./skills/detect-stack/scripts/ --file
```
Replace with:
```
<plugin-root>/scripts/harness-tools.sh detect-stack --file
```
And add a note at the top of the "How to write" section:
```
> **Plugin users:** `<plugin-root>` is the directory two levels above this skill file.
> Typical path: `~/.claude/plugins/lastro-harness/`.
```

- [ ] **Step 2: Update `skills/detect-use-cases/SKILL.md`**

Replace every `go run ./skills/detect-use-cases/scripts/` with `<plugin-root>/scripts/harness-tools.sh detect-use-cases`.
Add the same plugin-root note.

- [ ] **Step 3: Update `skills/create-sensors/SKILL.md`**

Replace `go run ./skills/create-sensors/scripts/` with `<plugin-root>/scripts/harness-tools.sh create-sensors`.
Add the plugin-root note.

- [ ] **Step 4: Update `skills/create-core-sensors/SKILL.md`**

Replace `go run ./skills/create-core-sensors/scripts/` with `<plugin-root>/scripts/harness-tools.sh create-core-sensors`.
Add the plugin-root note.

- [ ] **Step 5: Update `skills/run-sensor/SKILL.md`**

Replace `go run ./skills/run-sensor/scripts/` with `<plugin-root>/scripts/harness-tools.sh run-sensor`.
Add the plugin-root note.

- [ ] **Step 6: Update `skills/start-sensor/SKILL.md`**

Replace `go run ./skills/start-sensor/scripts/` with `<plugin-root>/scripts/harness-tools.sh start-sensor`.
Add the plugin-root note.

- [ ] **Step 7: Update `skills/stop-sensor/SKILL.md`**

Replace `go run ./skills/stop-sensor/scripts/` with `<plugin-root>/scripts/harness-tools.sh stop-sensor`.
Add the plugin-root note.

- [ ] **Step 8: Update `skills/tail-sensor-signals/SKILL.md`**

Replace `go run ./skills/tail-sensor-signals/scripts/` with `<plugin-root>/scripts/harness-tools.sh tail-sensor-signals`.
Add the plugin-root note.

- [ ] **Step 9: Update `skills/validate-use-case/SKILL.md`**

Replace `go run ./skills/validate-use-case/scripts/` with `<plugin-root>/scripts/harness-tools.sh validate-use-case`.
Add the plugin-root note.

- [ ] **Step 10: Update `skills/heal/SKILL.md`**

Replace `go run ./skills/heal/scripts/` with `<plugin-root>/scripts/harness-tools.sh heal`.
Add the plugin-root note.

- [ ] **Step 11: Commit**

```bash
git add skills/
git commit -m "feat: update SKILL.md files to invoke harness-tools binary via wrapper"
```

---

## Task 11: Create `.github/workflows/release.yml`

**Files:**
- Create: `.github/workflows/release.yml`

On every pushed tag matching `v*`, this workflow builds the binaries, commits them to the tag's branch (or main), and pushes.

- [ ] **Step 1: Create the workflow**

```yaml
name: Release binaries

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  build-and-commit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          # Fetch the full history so we can push back to main
          fetch-depth: 0
          token: ${{ secrets.GITHUB_TOKEN }}

      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
          cache: true

      - name: Build all platforms
        run: make build-all

      - name: Commit binaries to main
        run: |
          git config user.name  "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git fetch origin main
          git checkout main
          git add bin/
          git diff --cached --quiet && echo "no binary changes" && exit 0
          git commit -m "chore: update harness-tools binaries for ${{ github.ref_name }}"
          git push origin main
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add release workflow to build and commit binaries on tag push"
```

---

## Task 12: Verify `go test ./...` passes

- [ ] **Step 1: Run the full test suite**

```bash
go test ./...
```
Expected: all packages pass. No regressions from the new `lib/skillruntime` files or `cmd/harness-tools/`.

- [ ] **Step 2: Verify all 10 skills reference the wrapper correctly**

```bash
grep -r "harness-tools.sh" skills/
```
Expected: 10 matches, one per SKILL.md.

- [ ] **Step 3: Verify no SKILL.md still references `go run ./skills`**

```bash
grep -r "go run ./skills" skills/
```
Expected: no output.

- [ ] **Step 4: Final commit**

```bash
git add -A
git status  # review any unstaged files
git commit -m "chore: verify all tests pass and skill invocations updated"
```
