# Schema Freeze Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce the sequential schema-freeze gate for the harness framework — 8 entity JSON Schema documents, 8 enum data files + meta-schema, 44 worked examples, a README, and a minimal Go validator that proves they all hold together.

**Architecture:** Each schema is a JSON Schema 2020-12 document written as YAML, self-contained (no external `$ref` to a `common.yaml`), with cross-entity references typed as form-only string ids. Enum files are structured data validated by `_meta.yaml`. A small Go program (`cmd/validate-schemas`) loads every schema, registers them with `santhosh-tekuri/jsonschema`, validates each example against its entity schema, and exits zero on success. The validator is intentionally tiny — it is the foundation Phase A entity loaders will reuse.

**Tech Stack:** Go 1.22+, `github.com/santhosh-tekuri/jsonschema/v6`, `sigs.k8s.io/yaml`. All schemas/instances are YAML. No runtime, no CLI beyond the validator, no typed Go structs.

---

## Spec reference

Source spec: [`docs/superpowers/specs/2026-05-22-schema-freeze-design.md`](../specs/2026-05-22-schema-freeze-design.md).

## File map

| Path | Purpose |
|---|---|
| `go.mod` | Module declaration for the validator |
| `go.sum` | Locked deps (`santhosh-tekuri/jsonschema/v6`, `sigs.k8s.io/yaml`, transitive) |
| `cmd/validate-schemas/main.go` | The validator entry point |
| `schemas/README.md` | Cross-reference catalog + conventions |
| `schemas/stack-component.yaml` | JSON Schema for StackComponent |
| `schemas/entry-point.yaml` | JSON Schema for embedded EntryPoint (discriminated union) |
| `schemas/use-case.yaml` | JSON Schema for UseCase (`$ref`s entry-point.yaml) |
| `schemas/fixture.yaml` | JSON Schema for Fixture |
| `schemas/sensor.yaml` | JSON Schema for Sensor |
| `schemas/signal.yaml` | JSON Schema for Signal |
| `schemas/aggregate-signal.yaml` | JSON Schema for AggregateSignal |
| `schemas/validation-policy.yaml` | JSON Schema for ValidationPolicy |
| `schemas/enums/_meta.yaml` | Meta-schema (JSON Schema 2020-12) for enum files |
| `schemas/enums/validation-angles.yaml` | 10 angles |
| `schemas/enums/archetypes.yaml` | 9 archetypes + applicable_angles matrix |
| `schemas/enums/sensor-kinds.yaml` | 2 values (assertion, observational) |
| `schemas/enums/sensor-natures.yaml` | 2 values (computational, inferential) |
| `schemas/enums/signal-output-types.yaml` | 2 values (single-shot, stream) |
| `schemas/enums/fixture-roles.yaml` | 3 values (input, expected-output, expected-side-effect) |
| `schemas/enums/verdicts.yaml` | 3 values (pass, fail, inconclusive) |
| `schemas/enums/termination-reasons.yaml` | 4 values (completed, stopped, timeout, error) |
| `schemas/examples/stack-component/*.yaml` | 6 examples (one per kind) |
| `schemas/examples/entry-point/*.yaml` | 9 examples (one per archetype) |
| `schemas/examples/use-case/*.yaml` | 9 examples (one per archetype_scope) |
| `schemas/examples/fixture/*.yaml` | 3 examples (one per role) |
| `schemas/examples/sensor/*.yaml` | 6 examples (realistic kind × nature × output_type) |
| `schemas/examples/signal/*.yaml` | 3 examples (one per verdict) |
| `schemas/examples/aggregate-signal/*.yaml` | 5 examples (termination combinations) |
| `schemas/examples/validation-policy/*.yaml` | 3 examples (one per scope) |

Total: 62 schema/example/doc files + 3 Go/module files = 65 files.

---

# Phase 1 — Bootstrap

## Task 1: Scaffold directories and go.mod

**Files:**
- Create: `go.mod`
- Create: directory `schemas/`, `schemas/enums/`, `schemas/examples/{stack-component,entry-point,use-case,fixture,sensor,signal,aggregate-signal,validation-policy}/`, `cmd/validate-schemas/`

- [ ] **Step 1: Create the directory tree**

Run:
```bash
mkdir -p cmd/validate-schemas schemas/enums \
  schemas/examples/stack-component schemas/examples/entry-point \
  schemas/examples/use-case schemas/examples/fixture \
  schemas/examples/sensor schemas/examples/signal \
  schemas/examples/aggregate-signal schemas/examples/validation-policy
```

Expected: directories created, no output.

- [ ] **Step 2: Initialize the Go module**

Run:
```bash
go mod init github.com/iurykrieger/lastro
```

Expected: creates `go.mod` with one line declaring the module.

- [ ] **Step 3: Add the two runtime dependencies**

Run:
```bash
go get github.com/santhosh-tekuri/jsonschema/v6
go get sigs.k8s.io/yaml
```

Expected: `go.mod` lists both as `require`; `go.sum` is created.

- [ ] **Step 4: Verify tree**

Run:
```bash
find . -maxdepth 4 -type d -name schemas -o -name cmd -o -name 'examples' -o -name 'enums' | sort
```

Expected output includes `./cmd`, `./schemas`, `./schemas/enums`, and all 8 `./schemas/examples/<entity>` subdirs.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: bootstrap go module for schema-freeze validator"
```

---

## Task 2: Write the validator

**Files:**
- Create: `cmd/validate-schemas/main.go`

- [ ] **Step 1: Write the validator**

Create `cmd/validate-schemas/main.go` with the following content:

```go
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"
)

var entities = []string{
	"stack-component", "entry-point", "use-case", "fixture",
	"sensor", "signal", "aggregate-signal", "validation-policy",
}

var enums = []string{
	"validation-angles", "archetypes", "sensor-kinds", "sensor-natures",
	"signal-output-types", "fixture-roles", "verdicts", "termination-reasons",
}

const baseURL = "https://lastro.dev/harness/schemas/"

func loadYAMLAsAny(path string) (any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	j, err := yaml.YAMLToJSON(b)
	if err != nil {
		return nil, fmt.Errorf("yaml->json %s: %w", path, err)
	}
	var v any
	if err := json.Unmarshal(j, &v); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return v, nil
}

func main() {
	var errs []string
	c := jsonschema.NewCompiler()

	// Register every entity schema by its canonical URL so $refs resolve.
	for _, e := range entities {
		path := filepath.Join("schemas", e+".yaml")
		doc, err := loadYAMLAsAny(path)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if err := c.AddResource(baseURL+e+".yaml", doc); err != nil {
			errs = append(errs, fmt.Sprintf("register %s: %v", path, err))
		}
	}

	// Compile each entity schema — this is the "is itself a valid JSON Schema 2020-12" check.
	schemas := map[string]*jsonschema.Schema{}
	for _, e := range entities {
		sch, err := c.Compile(baseURL + e + ".yaml")
		if err != nil {
			errs = append(errs, fmt.Sprintf("compile %s: %v", e, err))
			continue
		}
		schemas[e] = sch
		fmt.Printf("OK schemas/%s.yaml is a valid JSON Schema 2020-12\n", e)
	}

	// Compile _meta.yaml and validate each enum file against it.
	metaPath := "schemas/enums/_meta.yaml"
	metaDoc, err := loadYAMLAsAny(metaPath)
	if err != nil {
		errs = append(errs, err.Error())
	} else {
		metaURL := baseURL + "enums/_meta.yaml"
		_ = c.AddResource(metaURL, metaDoc)
		metaSch, err := c.Compile(metaURL)
		if err != nil {
			errs = append(errs, fmt.Sprintf("compile _meta: %v", err))
		} else {
			for _, en := range enums {
				p := filepath.Join("schemas", "enums", en+".yaml")
				doc, err := loadYAMLAsAny(p)
				if err != nil {
					errs = append(errs, err.Error())
					continue
				}
				if err := metaSch.Validate(doc); err != nil {
					errs = append(errs, fmt.Sprintf("FAIL %s: %v", p, err))
				} else {
					fmt.Printf("OK %s matches _meta.yaml\n", p)
				}
			}
		}
	}

	// Validate each example against its entity schema.
	for _, e := range entities {
		sch, ok := schemas[e]
		if !ok {
			continue
		}
		dir := filepath.Join("schemas", "examples", e)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		var files []string
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".yaml") {
				files = append(files, path)
			}
			return nil
		})
		sort.Strings(files)
		for _, p := range files {
			doc, err := loadYAMLAsAny(p)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			if err := sch.Validate(doc); err != nil {
				errs = append(errs, fmt.Sprintf("FAIL %s: %v", p, err))
			} else {
				fmt.Printf("OK %s passes %s.yaml\n", p, e)
			}
		}
	}

	if len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "\n"+strings.Join(errs, "\n"))
		os.Exit(1)
	}
	fmt.Println("\nAll schemas, enums, and examples validated.")
}
```

- [ ] **Step 2: Build the validator**

Run:
```bash
go build ./cmd/validate-schemas
```

Expected: a `validate-schemas` binary is produced in the repo root (delete it after — we use `go run` from now on). No compilation errors.

- [ ] **Step 3: Run it against the empty tree**

Run:
```bash
go run ./cmd/validate-schemas
```

Expected: it fails because no schemas exist yet. Output contains errors like `read schemas/stack-component.yaml: open: no such file`. **This is the desired baseline — the validator runs and reports missing artifacts.**

- [ ] **Step 4: Remove the binary**

Run:
```bash
rm -f validate-schemas
```

- [ ] **Step 5: Commit**

```bash
git add cmd/validate-schemas/main.go go.mod go.sum
git commit -m "feat(gate): add minimal Go validator for schema-freeze artifacts"
```

---

## Task 3: Validator smoke shim

The validator currently panics if any schema is missing, which masks per-file errors. Confirm the structure handles an entirely empty tree by skipping cleanly when nothing exists — but for the gate we want the "missing file" error to be loud. No code change; this task just locks in the expected behavior by documenting the smoke output.

**Files:**
- (none — verification only)

- [ ] **Step 1: Run the validator and capture output**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | head -20
```

- [ ] **Step 2: Confirm output contains 8 "read schemas/..." errors**

Expected: stderr lists `read schemas/stack-component.yaml`, `read schemas/entry-point.yaml`, etc. — one per entity. Plus `read schemas/enums/_meta.yaml`. Exit code 1.

If the output is shaped differently (e.g., a panic), revisit Task 2.

- [ ] **Step 3: Nothing to commit**

This task is verification only.

---

# Phase 2 — Enums

The enum files are structured data, not JSON Schemas. They are validated by `_meta.yaml` (built in Task 4). After Task 4 each enum file can be validated immediately.

## Task 4: Meta-schema for enum files

**Files:**
- Create: `schemas/enums/_meta.yaml`

- [ ] **Step 1: Write the meta-schema**

Create `schemas/enums/_meta.yaml`:

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/enums/_meta.yaml"
title: EnumFileMeta
description: |
  Shape every file in schemas/enums/*.yaml must follow. Enum files are
  canonical structured data; this meta-schema is the only JSON Schema that
  lives under enums/.

type: object
required: [schema_version, title, values]
additionalProperties: false

properties:
  schema_version:
    type: string
    pattern: "^\\d+\\.\\d+\\.\\d+$"
  title:
    type: string
    minLength: 1
  description:
    type: string
  values:
    type: array
    minItems: 1
    items:
      type: object
      required: [id, purpose]
      additionalProperties: true
      properties:
        id:
          type: string
          pattern: "^[a-z][a-z0-9-]*$"
          minLength: 1
          maxLength: 128
        purpose:
          type: string
          minLength: 1
```

Note: `additionalProperties: true` at the values-item level lets each enum carry its own extra fields (`applicable_angles`, `lifecycle_notes`, `confidence_default`). The meta locks the shape baseline but does not constrain extensions.

- [ ] **Step 2: Run the validator**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep -i meta
```

Expected: no compile error about `_meta.yaml`. The validator still fails because enum files don't exist yet — that is fine. Look for the absence of a `compile _meta` failure.

- [ ] **Step 3: Commit**

```bash
git add schemas/enums/_meta.yaml
git commit -m "feat(gate): add meta-schema for enum files"
```

---

## Task 5: `validation-angles.yaml`

**Files:**
- Create: `schemas/enums/validation-angles.yaml`

- [ ] **Step 1: Write the enum file**

Create `schemas/enums/validation-angles.yaml`:

```yaml
schema_version: 1.0.0
title: ValidationAngle
description: |
  The ten facets a sensor can validate. Angles are extensible only by
  framework version bump; users control which apply via ValidationPolicy.

values:
  - id: security
    purpose: "Secrets, vulnerable deps, SAST findings, sensitive data in logs"
  - id: build
    purpose: "Compilation and packaging succeed"
  - id: code-structure
    purpose: "Conformance to architectural and lint patterns"
  - id: unit-test
    purpose: "Existence and execution of unit tests"
  - id: e2e-test
    purpose: "End-to-end behavior matches fixtures"
  - id: contracts
    purpose: "API, schema, or SDK contract conformance"
  - id: logs
    purpose: "Log shape, redaction, and semantic correctness"
  - id: metrics
    purpose: "Telemetry emission and shape"
  - id: database
    purpose: "Data writes and migrations match expectations"
  - id: performance
    purpose: "Latency, throughput, and resource ceilings"
```

- [ ] **Step 2: Run the validator**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep validation-angles
```

Expected: `OK schemas/enums/validation-angles.yaml matches _meta.yaml`.

- [ ] **Step 3: Commit**

```bash
git add schemas/enums/validation-angles.yaml
git commit -m "feat(gate): add validation-angles enum"
```

---

## Task 6: `archetypes.yaml`

**Files:**
- Create: `schemas/enums/archetypes.yaml`

- [ ] **Step 1: Write the enum file**

Create `schemas/enums/archetypes.yaml`:

```yaml
schema_version: 1.0.0
title: Archetype
description: |
  Shape of an application's observable surface. Each archetype declares
  which validation angles can produce sensors against it. This file is
  the canonical archetype × angle matrix.

values:
  - id: http-api
    purpose: "HTTP server exposing routes"
    applicable_angles:
      [security, build, code-structure, unit-test, e2e-test,
       contracts, logs, metrics, database, performance]
  - id: event-consumer
    purpose: "Listens on a queue or topic and reacts to messages"
    applicable_angles:
      [security, build, code-structure, unit-test, e2e-test,
       contracts, logs, metrics, database]
  - id: event-producer
    purpose: "Publishes events to a queue or topic"
    applicable_angles:
      [security, build, code-structure, unit-test, contracts, logs, metrics]
  - id: cli
    purpose: "Command-line program"
    applicable_angles:
      [security, build, code-structure, unit-test, contracts, logs]
  - id: sdk
    purpose: "Library distributed for embedding in other applications"
    applicable_angles:
      [security, build, code-structure, unit-test, contracts]
  - id: library
    purpose: "Internal-facing reusable module"
    applicable_angles:
      [security, build, code-structure, unit-test, contracts]
  - id: worker
    purpose: "Background process triggered by cron or signal"
    applicable_angles:
      [security, build, code-structure, unit-test, logs, metrics, database]
  - id: batch-job
    purpose: "Bulk processor with input source and output destination"
    applicable_angles:
      [security, build, code-structure, unit-test, logs, metrics, database, performance]
  - id: static-site
    purpose: "Pre-rendered HTML asset bundle"
    applicable_angles:
      [security, build, code-structure, contracts, performance]
```

- [ ] **Step 2: Run the validator**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep archetypes
```

Expected: `OK schemas/enums/archetypes.yaml matches _meta.yaml`.

- [ ] **Step 3: Commit**

```bash
git add schemas/enums/archetypes.yaml
git commit -m "feat(gate): add archetypes enum with applicable_angles matrix"
```

---

## Task 7: `sensor-kinds.yaml`

**Files:**
- Create: `schemas/enums/sensor-kinds.yaml`

- [ ] **Step 1: Write the enum file**

Create `schemas/enums/sensor-kinds.yaml`:

```yaml
schema_version: 1.0.0
title: SensorKind
description: |
  Lifecycle shape of a sensor. Sensors are either short-lived assertions
  that run steps and emit a verdict, or long-lived observers that watch a
  stream and emit signals until stopped.

values:
  - id: assertion
    purpose: "Runs steps, terminates, emits a verdict"
    lifecycle_notes: "Short-lived; produced by /run-sensor. May still emit a stream of signals (e.g., one per unit test) before its terminal AggregateSignal."
  - id: observational
    purpose: "Spawns a watcher over a stream, emits signals as patterns match, terminates on stop or completion"
    lifecycle_notes: "Long-lived; managed by /start-sensor and /stop-sensor. AggregateSignal is emitted by stop-sensor and reports observation completeness."
```

- [ ] **Step 2: Run the validator**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep sensor-kinds
```

Expected: `OK schemas/enums/sensor-kinds.yaml matches _meta.yaml`.

- [ ] **Step 3: Commit**

```bash
git add schemas/enums/sensor-kinds.yaml
git commit -m "feat(gate): add sensor-kinds enum"
```

---

## Task 8: `sensor-natures.yaml`

**Files:**
- Create: `schemas/enums/sensor-natures.yaml`

- [ ] **Step 1: Write the enum file**

Create `schemas/enums/sensor-natures.yaml`:

```yaml
schema_version: 1.0.0
title: SensorNature
description: |
  Epistemic source of a sensor's verdict. Computational sensors derive
  pass/fail deterministically; inferential sensors rely on LLM judgment.

values:
  - id: computational
    purpose: "Deterministic; pass/fail derives from program output (exit codes, parsed structured data)"
    confidence_default: 1.0
  - id: inferential
    purpose: "Probabilistic; pass/fail derives from LLM judgment over evidence"
    confidence_default: 0.7
```

- [ ] **Step 2: Run the validator**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep sensor-natures
```

Expected: `OK schemas/enums/sensor-natures.yaml matches _meta.yaml`.

- [ ] **Step 3: Commit**

```bash
git add schemas/enums/sensor-natures.yaml
git commit -m "feat(gate): add sensor-natures enum"
```

---

## Task 9: `signal-output-types.yaml`

**Files:**
- Create: `schemas/enums/signal-output-types.yaml`

- [ ] **Step 1: Write the enum file**

Create `schemas/enums/signal-output-types.yaml`:

```yaml
schema_version: 1.0.0
title: SignalOutputType
description: |
  Cardinality of signals a sensor emits before its terminal AggregateSignal.

values:
  - id: single-shot
    purpose: "Exactly one signal emitted, one verdict"
  - id: stream
    purpose: "N signals emitted (e.g., one per test, one per matched log line)"
```

- [ ] **Step 2: Run the validator**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep signal-output-types
```

Expected: `OK schemas/enums/signal-output-types.yaml matches _meta.yaml`.

- [ ] **Step 3: Commit**

```bash
git add schemas/enums/signal-output-types.yaml
git commit -m "feat(gate): add signal-output-types enum"
```

---

## Task 10: `fixture-roles.yaml`

**Files:**
- Create: `schemas/enums/fixture-roles.yaml`

- [ ] **Step 1: Write the enum file**

Create `schemas/enums/fixture-roles.yaml`:

```yaml
schema_version: 1.0.0
title: FixtureRole
description: |
  Role a fixture plays in driving or asserting a use case's behavior.

values:
  - id: input
    purpose: "Drives the behavior (request payload, CLI args, event message)"
  - id: expected-output
    purpose: "Asserted output (response payload, emitted event, written DB row)"
  - id: expected-side-effect
    purpose: "Non-payload effects (log line shape, metric emission)"
```

- [ ] **Step 2: Run the validator**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep fixture-roles
```

Expected: `OK schemas/enums/fixture-roles.yaml matches _meta.yaml`.

- [ ] **Step 3: Commit**

```bash
git add schemas/enums/fixture-roles.yaml
git commit -m "feat(gate): add fixture-roles enum"
```

---

## Task 11: `verdicts.yaml`

**Files:**
- Create: `schemas/enums/verdicts.yaml`

- [ ] **Step 1: Write the enum file**

Create `schemas/enums/verdicts.yaml`:

```yaml
schema_version: 1.0.0
title: Verdict
description: |
  Outcome of a sensor signal or aggregate signal. Used by both Signal.verdict
  and AggregateSignal.verdict.

values:
  - id: pass
    purpose: "Behavior matched the criterion"
  - id: fail
    purpose: "Behavior did not match; heal_hint is required"
  - id: inconclusive
    purpose: "Sensor could not determine pass/fail (e.g., inferential confidence below floor, timeout without coverage)"
```

- [ ] **Step 2: Run the validator**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep verdicts
```

Expected: `OK schemas/enums/verdicts.yaml matches _meta.yaml`.

- [ ] **Step 3: Commit**

```bash
git add schemas/enums/verdicts.yaml
git commit -m "feat(gate): add verdicts enum"
```

---

## Task 12: `termination-reasons.yaml`

**Files:**
- Create: `schemas/enums/termination-reasons.yaml`

- [ ] **Step 1: Write the enum file**

Create `schemas/enums/termination-reasons.yaml`:

```yaml
schema_version: 1.0.0
title: TerminationReason
description: |
  Why a sensor execution ended. Carried on AggregateSignal.

values:
  - id: completed
    purpose: "Sensor finished its scheduled work naturally"
  - id: stopped
    purpose: "Sensor was explicitly stopped (e.g., /stop-sensor invocation)"
  - id: timeout
    purpose: "Sensor exceeded its time budget before completing"
  - id: error
    purpose: "Sensor crashed or could not perform its work"
```

- [ ] **Step 2: Run the validator**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep termination-reasons
```

Expected: `OK schemas/enums/termination-reasons.yaml matches _meta.yaml`.

- [ ] **Step 3: Commit**

```bash
git add schemas/enums/termination-reasons.yaml
git commit -m "feat(gate): add termination-reasons enum"
```

---

# Phase 3 — Entity schemas with happy-path examples

Each task in this phase writes one entity schema and one example, then validates that the example passes the schema. Tasks are ordered by dependency: `entry-point` before `use-case`; `use-case` before any of `fixture`/`sensor` that reference it.

## Task 13: `stack-component.yaml` (schema + first example)

**Files:**
- Create: `schemas/stack-component.yaml`
- Create: `schemas/examples/stack-component/library.yaml`

- [ ] **Step 1: Write the schema**

Create `schemas/stack-component.yaml`:

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/stack-component.yaml"
title: StackComponent
description: |
  A detected library, runtime, framework, datastore, protocol, or tool present
  in the repository. Sensors may only reference StackComponents whose ids
  appear in the detected stack manifest (grounding invariant).

type: object
required: [schema_version, id, kind, name]
additionalProperties: false

properties:
  schema_version:
    type: string
    pattern: "^\\d+\\.\\d+\\.\\d+$"
  id:
    $ref: "#/$defs/Id"
  kind:
    type: string
    description: "StackComponent classification (kind is inline-only; no canonical enum file)."
    enum: [library, runtime, framework, datastore, protocol, tool]
  name:
    type: string
    minLength: 1
  version:
    type: string
  capabilities:
    type: array
    items: { type: string, minLength: 1 }
  detection_evidence:
    type: array
    items: { type: string, minLength: 1 }

$defs:
  Id:
    type: string
    pattern: "^[a-z][a-z0-9-]*$"
    minLength: 1
    maxLength: 128
```

- [ ] **Step 2: Write the happy-path example**

Create `schemas/examples/stack-component/library.yaml`:

```yaml
# StackComponent example — a library kind (Express HTTP framework).
schema_version: 1.0.0
id: express
kind: library
name: express
version: 4.18.x
capabilities:
  - http-routing
  - middleware
  - json-body-parsing
detection_evidence:
  - "package.json:dependencies.express"
```

- [ ] **Step 3: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep stack-component
```

Expected:
```
OK schemas/stack-component.yaml is a valid JSON Schema 2020-12
OK schemas/examples/stack-component/library.yaml passes stack-component.yaml
```

- [ ] **Step 4: Commit**

```bash
git add schemas/stack-component.yaml schemas/examples/stack-component/library.yaml
git commit -m "feat(gate): add stack-component schema with library example"
```

---

## Task 14: `entry-point.yaml` (schema + first example)

EntryPoint is a discriminated union over `archetype`. The schema uses `oneOf` to constrain `spec` per archetype value. No top-level `schema_version` because EntryPoint is embedded inside UseCase.

**Files:**
- Create: `schemas/entry-point.yaml`
- Create: `schemas/examples/entry-point/http-api.yaml`

- [ ] **Step 1: Write the schema**

Create `schemas/entry-point.yaml`:

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/entry-point.yaml"
title: EntryPoint
description: |
  Archetype-typed observable surface identifier embedded inside UseCase.
  Not a standalone entity; has no schema_version. The shape of `spec` is
  a discriminated union over `archetype` (oneOf below).

type: object
required: [id, archetype, spec]
additionalProperties: false

properties:
  id:
    $ref: "#/$defs/Id"
  archetype:
    type: string
    description: "Canonical source: schemas/enums/archetypes.yaml"
    enum: [http-api, event-consumer, event-producer, cli, sdk, library, worker, batch-job, static-site]
  spec:
    type: object

oneOf:
  - properties:
      archetype: { const: http-api }
      spec:
        type: object
        required: [method, path]
        additionalProperties: false
        properties:
          method: { type: string, enum: [GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS] }
          path:   { type: string, minLength: 1 }
  - properties:
      archetype: { const: event-consumer }
      spec:
        type: object
        required: [channel_kind, channel_name]
        additionalProperties: false
        properties:
          channel_kind: { type: string, enum: [queue, topic] }
          channel_name: { type: string, minLength: 1 }
  - properties:
      archetype: { const: event-producer }
      spec:
        type: object
        required: [target_channel_kind, target_channel_name]
        additionalProperties: false
        properties:
          target_channel_kind: { type: string, enum: [queue, topic] }
          target_channel_name: { type: string, minLength: 1 }
  - properties:
      archetype: { const: cli }
      spec:
        type: object
        required: [command]
        additionalProperties: false
        properties:
          command: { type: string, minLength: 1 }
  - properties:
      archetype: { enum: [sdk, library] }
      spec:
        type: object
        required: [exported_symbol]
        additionalProperties: false
        properties:
          exported_symbol: { type: string, minLength: 1 }
  - properties:
      archetype: { const: worker }
      spec:
        type: object
        required: [trigger_kind, schedule_or_signal]
        additionalProperties: false
        properties:
          trigger_kind:       { type: string, enum: [cron, signal] }
          schedule_or_signal: { type: string, minLength: 1 }
  - properties:
      archetype: { const: batch-job }
      spec:
        type: object
        required: [input_source, output_destination]
        additionalProperties: false
        properties:
          input_source:       { type: string, minLength: 1 }
          output_destination: { type: string, minLength: 1 }
  - properties:
      archetype: { const: static-site }
      spec:
        type: object
        required: [route_path]
        additionalProperties: false
        properties:
          route_path: { type: string, minLength: 1 }

$defs:
  Id:
    type: string
    pattern: "^[a-z][a-z0-9-]*$"
    minLength: 1
    maxLength: 128
```

- [ ] **Step 2: Write the http-api example**

Create `schemas/examples/entry-point/http-api.yaml`:

```yaml
# EntryPoint example — http-api archetype.
id: create-order-endpoint
archetype: http-api
spec:
  method: POST
  path: /orders
```

- [ ] **Step 3: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep entry-point
```

Expected:
```
OK schemas/entry-point.yaml is a valid JSON Schema 2020-12
OK schemas/examples/entry-point/http-api.yaml passes entry-point.yaml
```

- [ ] **Step 4: Commit**

```bash
git add schemas/entry-point.yaml schemas/examples/entry-point/http-api.yaml
git commit -m "feat(gate): add entry-point schema with http-api example"
```

---

## Task 15: `fixture.yaml` (schema + first example)

**Files:**
- Create: `schemas/fixture.yaml`
- Create: `schemas/examples/fixture/input.yaml`

- [ ] **Step 1: Write the schema**

Create `schemas/fixture.yaml`:

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/fixture.yaml"
title: Fixture
description: |
  Concrete I/O proof of a use case's behavior. One fixture can be reused
  by multiple sensors across angles (e.g., the same input payload drives
  an e2e-test sensor and a unit-test sensor for the same use case).

type: object
required: [schema_version, id, use_case_id, role, content_type, payload]
additionalProperties: false

properties:
  schema_version:
    type: string
    pattern: "^\\d+\\.\\d+\\.\\d+$"
  id:
    $ref: "#/$defs/Id"
  use_case_id:
    $ref: "#/$defs/Id"
  role:
    type: string
    description: "Canonical source: schemas/enums/fixture-roles.yaml"
    enum: [input, expected-output, expected-side-effect]
  content_type:
    type: string
    minLength: 1
  payload:
    type: string
    description: "Concrete payload as a string (JSON, XML, plain text, etc.). Parsed only at runtime."
  binding:
    type: object
    additionalProperties: false
    properties:
      channel:
        type: string
        enum: [http, cli-args, event, stdout, log-line, db-row]
      selector:
        type: object
        additionalProperties: true
  source_refs:
    type: array
    items:
      type: object
      required: [path]
      additionalProperties: false
      properties:
        path:   { type: string, minLength: 1 }
        symbol: { type: string }
        reason: { type: string }

$defs:
  Id:
    type: string
    pattern: "^[a-z][a-z0-9-]*$"
    minLength: 1
    maxLength: 128
```

- [ ] **Step 2: Write the input example**

Create `schemas/examples/fixture/input.yaml`:

```yaml
# Fixture example — role=input, an HTTP request body driving create-order.
schema_version: 1.0.0
id: order-input-fixture
use_case_id: create-order-use-case
role: input
content_type: application/json
payload: |
  {
    "customer_id": "c-001",
    "items": [
      { "sku": "BOOK-1", "qty": 2 }
    ]
  }
binding:
  channel: http
  selector:
    method: POST
    path: /orders
source_refs:
  - path: src/handlers/orders.ts
    symbol: createOrder
    reason: "reverse-engineered from handler signature"
```

- [ ] **Step 3: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep fixture
```

Expected:
```
OK schemas/fixture.yaml is a valid JSON Schema 2020-12
OK schemas/examples/fixture/input.yaml passes fixture.yaml
```

- [ ] **Step 4: Commit**

```bash
git add schemas/fixture.yaml schemas/examples/fixture/input.yaml
git commit -m "feat(gate): add fixture schema with input example"
```

---

## Task 16: `use-case.yaml` (schema + first example)

UseCase `$ref`s `entry-point.yaml`. The validator registers entry-point.yaml under its canonical URL so this `$ref` resolves.

**Files:**
- Create: `schemas/use-case.yaml`
- Create: `schemas/examples/use-case/http-api.yaml`

- [ ] **Step 1: Write the schema**

Create `schemas/use-case.yaml`:

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/use-case.yaml"
title: UseCase
description: |
  Behavioral specification in given/when/then form. Tech-agnostic text with
  {{fixtures.<id>}} and {{entry_points.<id>}} interpolation. No steps, no
  assertions — sensors validate; use cases describe.

type: object
required: [schema_version, id, title, archetype_scope, entry_points, given, when, then]
additionalProperties: false

properties:
  schema_version:
    type: string
    pattern: "^\\d+\\.\\d+\\.\\d+$"
  id:
    $ref: "#/$defs/Id"
  title:
    type: string
    minLength: 1
  archetype_scope:
    type: array
    minItems: 1
    description: "Which archetypes this use case is valid under. Canonical source: schemas/enums/archetypes.yaml"
    items:
      type: string
      enum: [http-api, event-consumer, event-producer, cli, sdk, library, worker, batch-job, static-site]
  entry_points:
    type: array
    minItems: 1
    items:
      $ref: "https://lastro.dev/harness/schemas/entry-point.yaml"
  given:
    type: array
    minItems: 1
    items: { type: string, minLength: 1 }
  when:
    type: array
    minItems: 1
    items: { type: string, minLength: 1 }
  then:
    type: array
    minItems: 1
    items: { type: string, minLength: 1 }
  source_refs:
    type: array
    items:
      type: object
      required: [path]
      additionalProperties: false
      properties:
        path:   { type: string, minLength: 1 }
        symbol: { type: string }
        reason: { type: string }
  fixture_ids:
    type: array
    items: { $ref: "#/$defs/Id" }

$defs:
  Id:
    type: string
    pattern: "^[a-z][a-z0-9-]*$"
    minLength: 1
    maxLength: 128
```

- [ ] **Step 2: Write the http-api example**

Create `schemas/examples/use-case/http-api.yaml`:

```yaml
# UseCase example — http-api archetype, create-order behavior.
schema_version: 2.0.0
id: create-order-use-case
title: "Client creates an order via HTTP"
archetype_scope: [http-api]

entry_points:
  - id: create-order-endpoint
    archetype: http-api
    spec:
      method: POST
      path: /orders

given:
  - "A request payload matching {{fixtures.order-input-fixture}} is constructed by the client"
when:
  - "The client invokes {{entry_points.create-order-endpoint}}"
then:
  - "The endpoint responds with a payload matching {{fixtures.order-output-fixture}}"

source_refs:
  - path: src/handlers/orders.ts
    symbol: createOrder
    reason: "reverse-engineered to detect this use case"

fixture_ids: [order-input-fixture, order-output-fixture]
```

- [ ] **Step 3: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep use-case
```

Expected:
```
OK schemas/use-case.yaml is a valid JSON Schema 2020-12
OK schemas/examples/use-case/http-api.yaml passes use-case.yaml
```

- [ ] **Step 4: Commit**

```bash
git add schemas/use-case.yaml schemas/examples/use-case/http-api.yaml
git commit -m "feat(gate): add use-case schema with http-api example"
```

---

## Task 17: `sensor.yaml` (schema + first example)

**Files:**
- Create: `schemas/sensor.yaml`
- Create: `schemas/examples/sensor/assertion-computational-single.yaml`

- [ ] **Step 1: Write the schema**

Create `schemas/sensor.yaml`:

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/sensor.yaml"
title: Sensor
description: |
  Grounded validator produced for one (use case × applicable validation
  angle). Top-level `uses:` lists StackComponent ids (grounding invariant).
  Step-level `uses:` lists Fixture ids (binding per step). No sensor-level
  fixture list.

type: object
required: [schema_version, id, use_case_id, angle, kind, nature, output_type, uses, steps]
additionalProperties: false

properties:
  schema_version:
    type: string
    pattern: "^\\d+\\.\\d+\\.\\d+$"
  id:
    $ref: "#/$defs/Id"
  use_case_id:
    $ref: "#/$defs/Id"
  angle:
    type: string
    description: "Canonical source: schemas/enums/validation-angles.yaml"
    enum: [security, build, code-structure, unit-test, e2e-test,
           contracts, logs, metrics, database, performance]
  kind:
    type: string
    description: "Canonical source: schemas/enums/sensor-kinds.yaml"
    enum: [assertion, observational]
  nature:
    type: string
    description: "Canonical source: schemas/enums/sensor-natures.yaml"
    enum: [computational, inferential]
  output_type:
    type: string
    description: "Canonical source: schemas/enums/signal-output-types.yaml"
    enum: [single-shot, stream]
  uses:
    type: array
    description: "StackComponent ids the sensor draws from (grounding invariant)"
    items: { $ref: "#/$defs/Id" }
  depends_on:
    type: array
    description: "Sensor ids that must pass before this one runs"
    items: { $ref: "#/$defs/Id" }
  steps:
    type: array
    minItems: 1
    items: { $ref: "#/$defs/SensorStep" }

$defs:
  Id:
    type: string
    pattern: "^[a-z][a-z0-9-]*$"
    minLength: 1
    maxLength: 128
  SensorStep:
    type: object
    required: [id, run]
    additionalProperties: false
    properties:
      id:   { $ref: "#/$defs/Id" }
      run:  { type: string, minLength: 1 }
      uses:
        type: array
        description: "Fixture ids referenced by this step (must belong to the sensor's use_case_id — runtime invariant)"
        items: { $ref: "#/$defs/Id" }
```

- [ ] **Step 2: Write the example**

Create `schemas/examples/sensor/assertion-computational-single.yaml`:

```yaml
# Sensor example — assertion / computational / single-shot.
# Build sensor for the create-order use case.
schema_version: 1.0.0
id: build-create-order-sensor
use_case_id: create-order-use-case
angle: build
kind: assertion
nature: computational
output_type: single-shot

uses:
  - node
  - express

steps:
  - id: compile
    run: "tsc --noEmit"
```

- [ ] **Step 3: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep sensor
```

Expected:
```
OK schemas/sensor.yaml is a valid JSON Schema 2020-12
OK schemas/examples/sensor/assertion-computational-single.yaml passes sensor.yaml
```

- [ ] **Step 4: Commit**

```bash
git add schemas/sensor.yaml schemas/examples/sensor/assertion-computational-single.yaml
git commit -m "feat(gate): add sensor schema with build-sensor example"
```

---

## Task 18: `signal.yaml` (schema + first example)

`Signal` requires `heal_hint` when `verdict == fail`. Enforced via `if/then`.

**Files:**
- Create: `schemas/signal.yaml`
- Create: `schemas/examples/signal/pass.yaml`

- [ ] **Step 1: Write the schema**

Create `schemas/signal.yaml`:

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/signal.yaml"
title: Signal
description: |
  Semantically rich, structured JSON record emitted by a sensor. Never raw
  logs. Every failing signal carries a heal_hint that the LLM can act on.

type: object
required: [schema_version, sensor_id, use_case_id, angle, emitted_at, verdict, confidence, evidence]
additionalProperties: false

properties:
  schema_version:
    type: string
    pattern: "^\\d+\\.\\d+\\.\\d+$"
  sensor_id:
    $ref: "#/$defs/Id"
  use_case_id:
    $ref: "#/$defs/Id"
  angle:
    type: string
    description: "Canonical source: schemas/enums/validation-angles.yaml"
    enum: [security, build, code-structure, unit-test, e2e-test,
           contracts, logs, metrics, database, performance]
  emitted_at:
    type: string
    format: date-time
  verdict:
    type: string
    description: "Canonical source: schemas/enums/verdicts.yaml"
    enum: [pass, fail, inconclusive]
  confidence:
    type: number
    minimum: 0
    maximum: 1
  evidence:
    type: object
    additionalProperties: true
    properties:
      expected: {}
      actual:   {}
      fixture_id:
        $ref: "#/$defs/Id"
  heal_hint:
    $ref: "#/$defs/HealHint"

allOf:
  - if:
      properties:
        verdict: { const: fail }
    then:
      required: [heal_hint]

$defs:
  Id:
    type: string
    pattern: "^[a-z][a-z0-9-]*$"
    minLength: 1
    maxLength: 128
  HealHint:
    type: object
    required: [summary, rationale]
    additionalProperties: false
    properties:
      summary:
        type: string
        minLength: 1
      suggested_locus:
        type: array
        items:
          type: object
          required: [path]
          additionalProperties: false
          properties:
            path:   { type: string, minLength: 1 }
            symbol: { type: string }
      rationale:
        type: string
        minLength: 1
```

- [ ] **Step 2: Write the pass example**

Create `schemas/examples/signal/pass.yaml`:

```yaml
# Signal example — verdict=pass from a build sensor.
schema_version: 1.0.0
sensor_id: build-create-order-sensor
use_case_id: create-order-use-case
angle: build
emitted_at: "2026-05-22T10:15:00Z"
verdict: pass
confidence: 1.0
evidence:
  expected: "tsc exits 0"
  actual: "tsc exited 0"
```

- [ ] **Step 3: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep "signal"
```

Expected:
```
OK schemas/signal.yaml is a valid JSON Schema 2020-12
OK schemas/examples/signal/pass.yaml passes signal.yaml
```

- [ ] **Step 4: Commit**

```bash
git add schemas/signal.yaml schemas/examples/signal/pass.yaml
git commit -m "feat(gate): add signal schema with pass example"
```

---

## Task 19: `aggregate-signal.yaml` (schema + first example)

Same `heal_hint` requirement on `verdict == fail`. Carries `rollup` and `completeness` blocks.

**Files:**
- Create: `schemas/aggregate-signal.yaml`
- Create: `schemas/examples/aggregate-signal/single-shot-pass.yaml`

- [ ] **Step 1: Write the schema**

Create `schemas/aggregate-signal.yaml`:

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/aggregate-signal.yaml"
title: AggregateSignal
description: |
  Terminal record emitted by every sensor at the end of its run. For
  single-shot sensors, mirrors the sole signal. For stream sensors, rolls
  up counts. For observational sensors, reports observation completeness.

type: object
required: [schema_version, type, sensor_id, use_case_id, angle, started_at, ended_at, termination_reason, verdict, confidence, rollup]
additionalProperties: false

properties:
  schema_version:
    type: string
    pattern: "^\\d+\\.\\d+\\.\\d+$"
  type:
    type: string
    const: aggregate
  sensor_id:
    $ref: "#/$defs/Id"
  use_case_id:
    $ref: "#/$defs/Id"
  angle:
    type: string
    description: "Canonical source: schemas/enums/validation-angles.yaml"
    enum: [security, build, code-structure, unit-test, e2e-test,
           contracts, logs, metrics, database, performance]
  started_at:
    type: string
    format: date-time
  ended_at:
    type: string
    format: date-time
  termination_reason:
    type: string
    description: "Canonical source: schemas/enums/termination-reasons.yaml"
    enum: [completed, stopped, timeout, error]
  verdict:
    type: string
    description: "Canonical source: schemas/enums/verdicts.yaml"
    enum: [pass, fail, inconclusive]
  confidence:
    type: number
    minimum: 0
    maximum: 1
  rollup:
    type: object
    required: [total_signals, pass_count, fail_count, inconclusive_count]
    additionalProperties: false
    properties:
      total_signals:       { type: integer, minimum: 0 }
      pass_count:          { type: integer, minimum: 0 }
      fail_count:          { type: integer, minimum: 0 }
      inconclusive_count:  { type: integer, minimum: 0 }
  completeness:
    type: object
    additionalProperties: false
    properties:
      expected_observations:
        type: array
        items: { type: string, minLength: 1 }
      missing_observations:
        type: array
        items: { type: string, minLength: 1 }
  heal_hint:
    $ref: "#/$defs/HealHint"

allOf:
  - if:
      properties:
        verdict: { const: fail }
    then:
      required: [heal_hint]

$defs:
  Id:
    type: string
    pattern: "^[a-z][a-z0-9-]*$"
    minLength: 1
    maxLength: 128
  HealHint:
    type: object
    required: [summary, rationale]
    additionalProperties: false
    properties:
      summary:
        type: string
        minLength: 1
      suggested_locus:
        type: array
        items:
          type: object
          required: [path]
          additionalProperties: false
          properties:
            path:   { type: string, minLength: 1 }
            symbol: { type: string }
      rationale:
        type: string
        minLength: 1
```

- [ ] **Step 2: Write the example**

Create `schemas/examples/aggregate-signal/single-shot-pass.yaml`:

```yaml
# AggregateSignal example — terminal record from a single-shot build sensor.
schema_version: 1.0.0
type: aggregate
sensor_id: build-create-order-sensor
use_case_id: create-order-use-case
angle: build
started_at: "2026-05-22T10:15:00Z"
ended_at:   "2026-05-22T10:15:08Z"
termination_reason: completed
verdict: pass
confidence: 1.0
rollup:
  total_signals: 1
  pass_count: 1
  fail_count: 0
  inconclusive_count: 0
```

- [ ] **Step 3: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep aggregate-signal
```

Expected:
```
OK schemas/aggregate-signal.yaml is a valid JSON Schema 2020-12
OK schemas/examples/aggregate-signal/single-shot-pass.yaml passes aggregate-signal.yaml
```

- [ ] **Step 4: Commit**

```bash
git add schemas/aggregate-signal.yaml schemas/examples/aggregate-signal/single-shot-pass.yaml
git commit -m "feat(gate): add aggregate-signal schema with single-shot-pass example"
```

---

## Task 20: `validation-policy.yaml` (schema + first example)

**Files:**
- Create: `schemas/validation-policy.yaml`
- Create: `schemas/examples/validation-policy/global.yaml`

- [ ] **Step 1: Write the schema**

Create `schemas/validation-policy.yaml`:

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/validation-policy.yaml"
title: ValidationPolicy
description: |
  Per-archetype declaration of which validation angles are obligatory,
  optional, or disabled. Resolution order: repo overrides org overrides global.

type: object
required: [schema_version, scope, per_archetype]
additionalProperties: false

properties:
  schema_version:
    type: string
    pattern: "^\\d+\\.\\d+\\.\\d+$"
  scope:
    type: string
    enum: [org, global, repo]
  inherits_from:
    $ref: "#/$defs/Id"
  per_archetype:
    type: object
    additionalProperties:
      type: object
      required: [obligatory_angles, optional_angles, disabled_angles]
      additionalProperties: false
      properties:
        obligatory_angles: { $ref: "#/$defs/AngleList" }
        optional_angles:   { $ref: "#/$defs/AngleList" }
        disabled_angles:   { $ref: "#/$defs/AngleList" }
    propertyNames:
      enum: [http-api, event-consumer, event-producer, cli, sdk, library, worker, batch-job, static-site]

$defs:
  Id:
    type: string
    pattern: "^[a-z][a-z0-9-]*$"
    minLength: 1
    maxLength: 128
  AngleList:
    type: array
    items:
      type: string
      enum: [security, build, code-structure, unit-test, e2e-test,
             contracts, logs, metrics, database, performance]
```

- [ ] **Step 2: Write the global example**

Create `schemas/examples/validation-policy/global.yaml`:

```yaml
# ValidationPolicy example — global scope, no inheritance.
schema_version: 1.0.0
scope: global
per_archetype:
  http-api:
    obligatory_angles: [build, security, unit-test, e2e-test, contracts]
    optional_angles:   [performance, metrics, logs]
    disabled_angles:   []
  cli:
    obligatory_angles: [build, security, contracts]
    optional_angles:   [unit-test, logs]
    disabled_angles:   [e2e-test, database]
```

- [ ] **Step 3: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep validation-policy
```

Expected:
```
OK schemas/validation-policy.yaml is a valid JSON Schema 2020-12
OK schemas/examples/validation-policy/global.yaml passes validation-policy.yaml
```

- [ ] **Step 4: Commit**

```bash
git add schemas/validation-policy.yaml schemas/examples/validation-policy/global.yaml
git commit -m "feat(gate): add validation-policy schema with global example"
```

---

# Phase 4 — Exhaustive examples

Each task in this phase fills in the remaining examples for one entity to reach the exhaustive coverage declared in the spec. Tasks are independent of each other and can be parallelized.

## Task 21: stack-component — remaining 5 examples

**Files:**
- Create: `schemas/examples/stack-component/{runtime,framework,datastore,protocol,tool}.yaml`

- [ ] **Step 1: Write `runtime.yaml`**

```yaml
# StackComponent example — runtime kind (Node).
schema_version: 1.0.0
id: node
kind: runtime
name: node
version: 20.x
capabilities:
  - event-loop
  - child-processes
  - native-tls
detection_evidence:
  - "package.json:engines.node"
```

- [ ] **Step 2: Write `framework.yaml`**

```yaml
# StackComponent example — framework kind (NestJS).
schema_version: 1.0.0
id: nestjs
kind: framework
name: "@nestjs/core"
version: 10.x
capabilities:
  - dependency-injection
  - http-routing
  - middleware
detection_evidence:
  - "package.json:dependencies.@nestjs/core"
```

- [ ] **Step 3: Write `datastore.yaml`**

```yaml
# StackComponent example — datastore kind (Postgres).
schema_version: 1.0.0
id: postgres
kind: datastore
name: postgres
version: 16.x
capabilities:
  - transactions
  - row-level-security
  - logical-replication
detection_evidence:
  - "docker-compose.yaml:services.db.image"
  - "package.json:dependencies.pg"
```

- [ ] **Step 4: Write `protocol.yaml`**

```yaml
# StackComponent example — protocol kind (gRPC).
schema_version: 1.0.0
id: grpc
kind: protocol
name: grpc
version: "1.60"
capabilities:
  - streaming
  - http2-transport
  - protobuf-codec
detection_evidence:
  - "package.json:dependencies.@grpc/grpc-js"
```

- [ ] **Step 5: Write `tool.yaml`**

```yaml
# StackComponent example — tool kind (ESLint).
schema_version: 1.0.0
id: eslint
kind: tool
name: eslint
version: 9.x
capabilities:
  - lint
  - autofix
detection_evidence:
  - "package.json:devDependencies.eslint"
  - ".eslintrc.json"
```

- [ ] **Step 6: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep "examples/stack-component"
```

Expected: six `OK ... passes stack-component.yaml` lines.

- [ ] **Step 7: Commit**

```bash
git add schemas/examples/stack-component/
git commit -m "feat(gate): add exhaustive stack-component examples (one per kind)"
```

---

## Task 22: entry-point — remaining 8 examples

**Files:**
- Create: `schemas/examples/entry-point/{event-consumer,event-producer,cli,sdk,library,worker,batch-job,static-site}.yaml`

- [ ] **Step 1: Write `event-consumer.yaml`**

```yaml
# EntryPoint example — event-consumer archetype.
id: on-order-created
archetype: event-consumer
spec:
  channel_kind: queue
  channel_name: orders.created
```

- [ ] **Step 2: Write `event-producer.yaml`**

```yaml
# EntryPoint example — event-producer archetype.
id: emit-payment-processed
archetype: event-producer
spec:
  target_channel_kind: topic
  target_channel_name: payments.processed
```

- [ ] **Step 3: Write `cli.yaml`**

```yaml
# EntryPoint example — cli archetype.
id: harness-detect-cli
archetype: cli
spec:
  command: harness
```

- [ ] **Step 4: Write `sdk.yaml`**

```yaml
# EntryPoint example — sdk archetype.
id: create-client-symbol
archetype: sdk
spec:
  exported_symbol: createClient
```

- [ ] **Step 5: Write `library.yaml`**

```yaml
# EntryPoint example — library archetype.
id: parse-config-symbol
archetype: library
spec:
  exported_symbol: parseConfig
```

- [ ] **Step 6: Write `worker.yaml`**

```yaml
# EntryPoint example — worker archetype.
id: daily-reconciliation-worker
archetype: worker
spec:
  trigger_kind: cron
  schedule_or_signal: "0 2 * * *"
```

- [ ] **Step 7: Write `batch-job.yaml`**

```yaml
# EntryPoint example — batch-job archetype.
id: csv-import-job
archetype: batch-job
spec:
  input_source: "s3://imports/orders/*.csv"
  output_destination: "postgres://app/orders"
```

- [ ] **Step 8: Write `static-site.yaml`**

```yaml
# EntryPoint example — static-site archetype.
id: pricing-route
archetype: static-site
spec:
  route_path: /pricing
```

- [ ] **Step 9: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep "examples/entry-point"
```

Expected: nine `OK ... passes entry-point.yaml` lines.

- [ ] **Step 10: Commit**

```bash
git add schemas/examples/entry-point/
git commit -m "feat(gate): add exhaustive entry-point examples (one per archetype)"
```

---

## Task 23: use-case — remaining 8 examples

**Files:**
- Create: `schemas/examples/use-case/{event-consumer,event-producer,cli,sdk,library,worker,batch-job,static-site}.yaml`

- [ ] **Step 1: Write `event-consumer.yaml`**

```yaml
# UseCase example — event-consumer archetype.
schema_version: 2.0.0
id: record-incoming-order-use-case
title: "Service records an order received via the orders.created queue"
archetype_scope: [event-consumer]

entry_points:
  - id: on-order-created
    archetype: event-consumer
    spec:
      channel_kind: queue
      channel_name: orders.created

given:
  - "An event matching {{fixtures.order-created-event-fixture}} is published to {{entry_points.on-order-created}}"
when:
  - "The handler receives the event"
then:
  - "The order is recorded in the application's storage"

fixture_ids: [order-created-event-fixture]
```

- [ ] **Step 2: Write `event-producer.yaml`**

```yaml
# UseCase example — event-producer archetype.
schema_version: 2.0.0
id: emit-payment-processed-use-case
title: "Service emits payments.processed after charging a card"
archetype_scope: [event-producer]

entry_points:
  - id: emit-payment-processed
    archetype: event-producer
    spec:
      target_channel_kind: topic
      target_channel_name: payments.processed

given:
  - "A successful card charge has occurred matching {{fixtures.charge-success-state-fixture}}"
when:
  - "The service publishes its outbound event"
then:
  - "An event matching {{fixtures.payment-processed-event-fixture}} is published to {{entry_points.emit-payment-processed}}"

fixture_ids: [charge-success-state-fixture, payment-processed-event-fixture]
```

- [ ] **Step 3: Write `cli.yaml`**

```yaml
# UseCase example — cli archetype.
schema_version: 2.0.0
id: harness-detect-use-case
title: "Operator detects stack on a repo via the harness CLI"
archetype_scope: [cli]

entry_points:
  - id: harness-detect-cli
    archetype: cli
    spec:
      command: harness

given:
  - "The user invokes {{entry_points.harness-detect-cli}} with arguments matching {{fixtures.detect-args-fixture}}"
when:
  - "The command completes"
then:
  - "Standard output matches {{fixtures.detect-stdout-fixture}} and the process exits with code 0"

fixture_ids: [detect-args-fixture, detect-stdout-fixture]
```

- [ ] **Step 4: Write `sdk.yaml`**

```yaml
# UseCase example — sdk archetype.
schema_version: 2.0.0
id: create-client-use-case
title: "Consumer instantiates the SDK client and is ready to make calls"
archetype_scope: [sdk]

entry_points:
  - id: create-client-symbol
    archetype: sdk
    spec:
      exported_symbol: createClient

given:
  - "Configuration matching {{fixtures.client-config-fixture}} is available"
when:
  - "The consumer calls {{entry_points.create-client-symbol}}"
then:
  - "An initialized client matching {{fixtures.client-instance-fixture}} is returned"

fixture_ids: [client-config-fixture, client-instance-fixture]
```

- [ ] **Step 5: Write `library.yaml`**

```yaml
# UseCase example — library archetype.
schema_version: 2.0.0
id: parse-config-use-case
title: "Caller parses a configuration document into a typed structure"
archetype_scope: [library]

entry_points:
  - id: parse-config-symbol
    archetype: library
    spec:
      exported_symbol: parseConfig

given:
  - "A configuration document matching {{fixtures.config-input-fixture}}"
when:
  - "The caller invokes {{entry_points.parse-config-symbol}}"
then:
  - "A typed configuration matching {{fixtures.config-parsed-fixture}} is returned"

fixture_ids: [config-input-fixture, config-parsed-fixture]
```

- [ ] **Step 6: Write `worker.yaml`**

```yaml
# UseCase example — worker archetype.
schema_version: 2.0.0
id: daily-reconciliation-use-case
title: "Worker reconciles ledger entries each night at 02:00"
archetype_scope: [worker]

entry_points:
  - id: daily-reconciliation-worker
    archetype: worker
    spec:
      trigger_kind: cron
      schedule_or_signal: "0 2 * * *"

given:
  - "Ledger state matching {{fixtures.ledger-state-fixture}} at 02:00"
when:
  - "The cron trigger fires {{entry_points.daily-reconciliation-worker}}"
then:
  - "Reconciliation results matching {{fixtures.reconciliation-result-fixture}} are persisted"

fixture_ids: [ledger-state-fixture, reconciliation-result-fixture]
```

- [ ] **Step 7: Write `batch-job.yaml`**

```yaml
# UseCase example — batch-job archetype.
schema_version: 2.0.0
id: csv-import-use-case
title: "Job imports orders from S3 CSV files into Postgres"
archetype_scope: [batch-job]

entry_points:
  - id: csv-import-job
    archetype: batch-job
    spec:
      input_source: "s3://imports/orders/*.csv"
      output_destination: "postgres://app/orders"

given:
  - "CSV files matching {{fixtures.orders-csv-fixture}} exist in the input source"
when:
  - "The job runs to completion"
then:
  - "Database rows matching {{fixtures.orders-db-rows-fixture}} exist in the output destination"

fixture_ids: [orders-csv-fixture, orders-db-rows-fixture]
```

- [ ] **Step 8: Write `static-site.yaml`**

```yaml
# UseCase example — static-site archetype.
schema_version: 2.0.0
id: pricing-page-use-case
title: "/pricing renders the published rate card"
archetype_scope: [static-site]

entry_points:
  - id: pricing-route
    archetype: static-site
    spec:
      route_path: /pricing

given:
  - "A rate card matching {{fixtures.rate-card-fixture}} is available at build time"
when:
  - "A client fetches {{entry_points.pricing-route}}"
then:
  - "The response body matches {{fixtures.pricing-html-fixture}}"

fixture_ids: [rate-card-fixture, pricing-html-fixture]
```

- [ ] **Step 9: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep "examples/use-case"
```

Expected: nine `OK ... passes use-case.yaml` lines.

- [ ] **Step 10: Commit**

```bash
git add schemas/examples/use-case/
git commit -m "feat(gate): add exhaustive use-case examples (one per archetype)"
```

---

## Task 24: fixture — remaining 2 examples

**Files:**
- Create: `schemas/examples/fixture/{expected-output,expected-side-effect}.yaml`

- [ ] **Step 1: Write `expected-output.yaml`**

```yaml
# Fixture example — role=expected-output, the response body from create-order.
schema_version: 1.0.0
id: order-output-fixture
use_case_id: create-order-use-case
role: expected-output
content_type: application/json
payload: |
  {
    "order_id": "o-001",
    "status": "accepted",
    "total_cents": 5990
  }
binding:
  channel: http
  selector:
    method: POST
    path: /orders
    status: 201
source_refs:
  - path: src/handlers/orders.ts
    symbol: createOrder
    reason: "response body shape observed via reverse-engineering"
```

- [ ] **Step 2: Write `expected-side-effect.yaml`**

```yaml
# Fixture example — role=expected-side-effect, a log line shape.
schema_version: 1.0.0
id: order-created-log-fixture
use_case_id: create-order-use-case
role: expected-side-effect
content_type: text/plain
payload: |
  {"level":"info","msg":"order created","order_id":"<uuid>","customer_id":"<id>"}
binding:
  channel: log-line
  selector:
    logger: orders
    level: info
source_refs:
  - path: src/handlers/orders.ts
    symbol: createOrder
    reason: "log emitted on successful order creation"
```

- [ ] **Step 3: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep "examples/fixture"
```

Expected: three `OK ... passes fixture.yaml` lines.

- [ ] **Step 4: Commit**

```bash
git add schemas/examples/fixture/
git commit -m "feat(gate): add exhaustive fixture examples (one per role)"
```

---

## Task 25: sensor — remaining 5 examples

**Files:**
- Create: `schemas/examples/sensor/{assertion-computational-stream,assertion-inferential-single,assertion-inferential-stream,observational-computational-stream,observational-inferential-stream}.yaml`

- [ ] **Step 1: Write `assertion-computational-stream.yaml`**

```yaml
# Sensor example — assertion / computational / stream.
# Unit-test runner emits one signal per test, then an AggregateSignal.
schema_version: 1.0.0
id: unit-tests-create-order-sensor
use_case_id: create-order-use-case
angle: unit-test
kind: assertion
nature: computational
output_type: stream

uses:
  - node
  - express

steps:
  - id: run-tests
    run: "jest --json src/handlers/orders.test.ts"
```

- [ ] **Step 2: Write `assertion-inferential-single.yaml`**

```yaml
# Sensor example — assertion / inferential / single-shot.
# Code-structure sensor uses an LLM judge over the diff.
schema_version: 1.0.0
id: code-structure-create-order-sensor
use_case_id: create-order-use-case
angle: code-structure
kind: assertion
nature: inferential
output_type: single-shot

uses:
  - node
  - eslint

steps:
  - id: collect-context
    run: "harness collect-files src/handlers/orders.ts"
  - id: judge
    run: "harness llm-judge --rubric code-structure"
```

- [ ] **Step 3: Write `assertion-inferential-stream.yaml`**

```yaml
# Sensor example — assertion / inferential / stream.
# Contracts sensor judges each OpenAPI endpoint.
schema_version: 1.0.0
id: contracts-create-order-sensor
use_case_id: create-order-use-case
angle: contracts
kind: assertion
nature: inferential
output_type: stream

uses:
  - express
  - grpc

steps:
  - id: extract-endpoints
    run: "harness extract-openapi src/api.yaml"
  - id: judge-each
    run: "harness llm-judge --rubric contract-per-endpoint"
    uses:
      - order-input-fixture
      - order-output-fixture
```

- [ ] **Step 4: Write `observational-computational-stream.yaml`**

```yaml
# Sensor example — observational / computational / stream.
# Log-tail with structured matchers derived from the log library.
schema_version: 1.0.0
id: logs-create-order-sensor
use_case_id: create-order-use-case
angle: logs
kind: observational
nature: computational
output_type: stream

uses:
  - node
  - express

steps:
  - id: start-tail
    run: "harness log-tail --logger orders --level info"
    uses:
      - order-created-log-fixture
```

- [ ] **Step 5: Write `observational-inferential-stream.yaml`**

```yaml
# Sensor example — observational / inferential / stream.
# Log-tail using LLM-derived patterns for semantic correctness.
schema_version: 1.0.0
id: semantic-logs-create-order-sensor
use_case_id: create-order-use-case
angle: logs
kind: observational
nature: inferential
output_type: stream

uses:
  - node

steps:
  - id: derive-patterns
    run: "harness derive-log-patterns --use-case create-order-use-case"
  - id: start-tail
    run: "harness log-tail --semantic"
```

- [ ] **Step 6: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep "examples/sensor"
```

Expected: six `OK ... passes sensor.yaml` lines.

- [ ] **Step 7: Commit**

```bash
git add schemas/examples/sensor/
git commit -m "feat(gate): add exhaustive sensor examples (kind × nature × output_type)"
```

---

## Task 26: signal — remaining 2 examples

**Files:**
- Create: `schemas/examples/signal/{fail,inconclusive}.yaml`

- [ ] **Step 1: Write `fail.yaml`**

```yaml
# Signal example — verdict=fail with required heal_hint.
schema_version: 1.0.0
sensor_id: unit-tests-create-order-sensor
use_case_id: create-order-use-case
angle: unit-test
emitted_at: "2026-05-22T10:16:42Z"
verdict: fail
confidence: 1.0
evidence:
  expected: "createOrder() returns 201 for valid input"
  actual:   "createOrder() returned 500"
  fixture_id: order-input-fixture
heal_hint:
  summary: "createOrder throws on valid input; check the validation branch"
  suggested_locus:
    - path: src/handlers/orders.ts
      symbol: createOrder
  rationale: "Test stack trace points to a thrown TypeError in body validation; the fixture passes the expected schema, so the validator is wrong, not the input."
```

- [ ] **Step 2: Write `inconclusive.yaml`**

```yaml
# Signal example — verdict=inconclusive from an inferential sensor below the confidence floor.
schema_version: 1.0.0
sensor_id: code-structure-create-order-sensor
use_case_id: create-order-use-case
angle: code-structure
emitted_at: "2026-05-22T10:17:15Z"
verdict: inconclusive
confidence: 0.55
evidence:
  expected: "Handler delegates to a service layer"
  actual:   "Handler is mixed with business logic; LLM judgment uncertain"
```

- [ ] **Step 3: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep "examples/signal"
```

Expected: three `OK ... passes signal.yaml` lines.

- [ ] **Step 4: Commit**

```bash
git add schemas/examples/signal/
git commit -m "feat(gate): add exhaustive signal examples (one per verdict)"
```

---

## Task 27: aggregate-signal — remaining 4 examples

**Files:**
- Create: `schemas/examples/aggregate-signal/{stream-pass,stream-fail,observational-pass,observational-fail-missing}.yaml`

- [ ] **Step 1: Write `stream-pass.yaml`**

```yaml
# AggregateSignal example — terminal record from a stream sensor; all tests passed.
schema_version: 1.0.0
type: aggregate
sensor_id: unit-tests-create-order-sensor
use_case_id: create-order-use-case
angle: unit-test
started_at: "2026-05-22T10:16:00Z"
ended_at:   "2026-05-22T10:16:30Z"
termination_reason: completed
verdict: pass
confidence: 1.0
rollup:
  total_signals: 700
  pass_count: 700
  fail_count: 0
  inconclusive_count: 0
```

- [ ] **Step 2: Write `stream-fail.yaml`**

```yaml
# AggregateSignal example — terminal record from a stream sensor; 70/700 failures with heal_hint.
schema_version: 1.0.0
type: aggregate
sensor_id: unit-tests-create-order-sensor
use_case_id: create-order-use-case
angle: unit-test
started_at: "2026-05-22T10:16:00Z"
ended_at:   "2026-05-22T10:16:42Z"
termination_reason: completed
verdict: fail
confidence: 1.0
rollup:
  total_signals: 700
  pass_count: 630
  fail_count: 70
  inconclusive_count: 0
heal_hint:
  summary: "70 of 700 unit tests failed — concentrated in orders/validation"
  suggested_locus:
    - path: src/handlers/orders.ts
      symbol: createOrder
  rationale: "Individual signals carry per-test details; the cluster of failures is in body-validation tests."
```

- [ ] **Step 3: Write `observational-pass.yaml`**

```yaml
# AggregateSignal example — observational sensor stopped cleanly with all expected observations seen.
schema_version: 1.0.0
type: aggregate
sensor_id: logs-create-order-sensor
use_case_id: create-order-use-case
angle: logs
started_at: "2026-05-22T10:18:00Z"
ended_at:   "2026-05-22T10:18:30Z"
termination_reason: stopped
verdict: pass
confidence: 1.0
rollup:
  total_signals: 3
  pass_count: 3
  fail_count: 0
  inconclusive_count: 0
completeness:
  expected_observations:
    - "order-received"
    - "order-validated"
    - "order-persisted"
  missing_observations: []
```

- [ ] **Step 4: Write `observational-fail-missing.yaml`**

```yaml
# AggregateSignal example — observational sensor failed because an expected observation never arrived.
schema_version: 1.0.0
type: aggregate
sensor_id: logs-create-order-sensor
use_case_id: create-order-use-case
angle: logs
started_at: "2026-05-22T10:19:00Z"
ended_at:   "2026-05-22T10:19:30Z"
termination_reason: stopped
verdict: fail
confidence: 1.0
rollup:
  total_signals: 2
  pass_count: 2
  fail_count: 0
  inconclusive_count: 0
completeness:
  expected_observations:
    - "order-received"
    - "order-validated"
    - "order-persisted"
  missing_observations:
    - "order-persisted"
heal_hint:
  summary: "order-persisted log line never emitted; the persistence step is silently failing"
  suggested_locus:
    - path: src/handlers/orders.ts
      symbol: persistOrder
  rationale: "The first two observations were seen; the third did not arrive within the window. The persistence call likely swallows an error."
```

- [ ] **Step 5: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep "examples/aggregate-signal"
```

Expected: five `OK ... passes aggregate-signal.yaml` lines.

- [ ] **Step 6: Commit**

```bash
git add schemas/examples/aggregate-signal/
git commit -m "feat(gate): add exhaustive aggregate-signal examples"
```

---

## Task 28: validation-policy — remaining 2 examples

**Files:**
- Create: `schemas/examples/validation-policy/{org,repo}.yaml`

- [ ] **Step 1: Write `org.yaml`**

```yaml
# ValidationPolicy example — org scope, inherits from global, tightens cli.
schema_version: 1.0.0
scope: org
inherits_from: global-default-policy
per_archetype:
  http-api:
    obligatory_angles: [build, security, unit-test, e2e-test, contracts, logs]
    optional_angles:   [performance, metrics]
    disabled_angles:   []
  cli:
    obligatory_angles: [build, security, contracts, unit-test]
    optional_angles:   [logs]
    disabled_angles:   [e2e-test, database, performance]
```

- [ ] **Step 2: Write `repo.yaml`**

```yaml
# ValidationPolicy example — repo scope, inherits from org, drops performance for http-api.
schema_version: 1.0.0
scope: repo
inherits_from: acme-org-policy
per_archetype:
  http-api:
    obligatory_angles: [build, security, unit-test, e2e-test, contracts, logs]
    optional_angles:   [metrics]
    disabled_angles:   [performance]
```

- [ ] **Step 3: Validate**

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep "examples/validation-policy"
```

Expected: three `OK ... passes validation-policy.yaml` lines.

- [ ] **Step 4: Commit**

```bash
git add schemas/examples/validation-policy/
git commit -m "feat(gate): add exhaustive validation-policy examples (one per scope)"
```

---

# Phase 5 — README and final acceptance

## Task 29: `schemas/README.md`

**Files:**
- Create: `schemas/README.md`

- [ ] **Step 1: Write the README**

Create `schemas/README.md`:

```markdown
# Schemas — Harness Framework Cross-Entity Contract

This directory is the **sequential gate** before Phase A of the harness
framework. It locks the YAML contract that every parallel entity chunk
(E1–E9) reads from.

## 1. Purpose

These files define the *shape* of every artifact the framework deals with:
StackComponents, EntryPoints, UseCases, Fixtures, Sensors, Signals,
AggregateSignals, and ValidationPolicies. They do **not** validate that
referenced ids actually exist — that is a runtime concern.

## 2. Layout

```
schemas/
├── README.md
├── stack-component.yaml          ┐
├── entry-point.yaml              │
├── use-case.yaml                 │
├── fixture.yaml                  │  8 schemas (7 entities + entry-point embedded type)
├── sensor.yaml                   │
├── signal.yaml                   │
├── aggregate-signal.yaml         │
├── validation-policy.yaml        ┘
├── enums/
│   ├── _meta.yaml                ── meta-schema for enum files
│   ├── validation-angles.yaml    ┐
│   ├── archetypes.yaml           │
│   ├── sensor-kinds.yaml         │  8 enums
│   ├── sensor-natures.yaml       │
│   ├── signal-output-types.yaml  │
│   ├── fixture-roles.yaml        │
│   ├── verdicts.yaml             │
│   └── termination-reasons.yaml  ┘
└── examples/
    └── <entity>/<scenario>.yaml  ── 44 worked examples
```

## 3. Conventions

- **Standard:** JSON Schema 2020-12, embedded in YAML.
- **Id form:** lowercase slug, regex `^[a-z][a-z0-9-]*$`, max 128 chars.
- **`schema_version`:** required on every entity (and every enum file).
  Semver. Versions evolve **independently** per file.
- **`additionalProperties: false`** at every object level. Undocumented
  fields are validation errors.
- **Cross-references** are typed as strings matching the id regex. No
  cross-file `$ref` for ids (except where one schema embeds another's
  type — UseCase `$ref`s EntryPoint).
- **Enums** appear inline (`enum: [...]`) in entity schemas; the
  canonical source is the matching file in `enums/`. See §5.
- **No `default:` values** in any schema. The gate freezes shape, not policy.

## 4. Cross-reference catalog

Every cross-entity reference is a string matching `^[a-z][a-z0-9-]*$`.
Schemas validate form only; existence is a **runtime** concern.

| Source | Target | Cardinality |
|---|---|---|
| `Sensor.use_case_id` | UseCase | 1 |
| `Sensor.uses[]` | StackComponent | 0..N |
| `Sensor.depends_on[]` | Sensor | 0..N |
| `Sensor.steps[].uses[]` | Fixture | 0..N (must belong to `Sensor.use_case_id` — runtime invariant) |
| `Signal.sensor_id` | Sensor | 1 |
| `Signal.use_case_id` | UseCase | 1 |
| `Signal.evidence.fixture_id` | Fixture | 0..1 |
| `AggregateSignal.sensor_id` | Sensor | 1 |
| `AggregateSignal.use_case_id` | UseCase | 1 |
| `Fixture.use_case_id` | UseCase | 1 |
| `UseCase.fixture_ids[]` | Fixture | 1..N |
| `UseCase.entry_points[].id` | (local — unique within the UseCase) | — |
| `UseCase.entry_points[].archetype` | enum: archetypes | 1 |
| `UseCase.archetype_scope[]` | enum: archetypes | 1..N |
| `ValidationPolicy.inherits_from` | ValidationPolicy | 0..1 |

## 5. Enums

The eight files under `enums/` carry the canonical lists for the framework's
fixed enums. Each is structured data, not a JSON Schema; they are
validated by `enums/_meta.yaml`.

| File | Purpose |
|---|---|
| `validation-angles.yaml` | The 10 angles a sensor can validate |
| `archetypes.yaml` | The 9 application archetypes + their applicable_angles matrix |
| `sensor-kinds.yaml` | `assertion`, `observational` |
| `sensor-natures.yaml` | `computational`, `inferential` |
| `signal-output-types.yaml` | `single-shot`, `stream` |
| `fixture-roles.yaml` | `input`, `expected-output`, `expected-side-effect` |
| `verdicts.yaml` | `pass`, `fail`, `inconclusive` |
| `termination-reasons.yaml` | `completed`, `stopped`, `timeout`, `error` |

### Adding or removing a value

1. Edit the enum file.
2. Bump its `schema_version` (additive minor; removal major).
3. Mirror the change in any entity schema's inline `enum:` clause.
4. Run `go run ./cmd/validate-schemas`. All examples must still pass.

**Drift contract:** the inline `enum:` lists in entity schemas duplicate
`enums/<x>.yaml#/values[*].id`. The enum file is canonical. Reconciling
inline copies with the canonical source is a Phase A concern (likely a
codegen step or a CI consistency test).

### Sole load-bearing enum metadata

`archetypes.yaml` carries `applicable_angles` per archetype — the
canonical archetype × angle matrix. This is the **only** enum metadata
the framework reads programmatically. All other `purpose` /
`lifecycle_notes` / `confidence_default` fields are informational.

## 6. Examples

`examples/<entity>/<scenario>.yaml` carries 44 worked examples (one or more
per entity, covering each entity's internal dimensions). Each file:

- Opens with 2–3 comment lines describing the scenario
- Uses descriptive slug ids (`create-order-endpoint`, `order-input-fixture`)
- Cross-references ids found in sibling examples (purely for human
  readability — the schema does not check existence)

The pair `observational + single-shot` is excluded from `sensor/`
examples because it is a conceptual contradiction (observational implies
a stream). The schema **does not** enforce this — keeping it permissive
avoids brittle `if/then/else` constructs.

## 7. Validation

The gate ships a Go validator under `cmd/validate-schemas`.

```
go run ./cmd/validate-schemas
```

Behavior:

1. Loads every entity schema and confirms it is itself a valid JSON
   Schema 2020-12.
2. Loads `enums/_meta.yaml` and validates each enum file against it.
3. Walks `examples/<entity>/*.yaml` and validates each against the
   corresponding entity schema.
4. Exits zero on success; non-zero with a list of errors otherwise.

Dependencies: `github.com/santhosh-tekuri/jsonschema/v6` (the standard
2020-12 implementation Phase A loaders will also use) and
`sigs.k8s.io/yaml` for YAML→JSON conversion.

## 8. Versioning policy

Each schema bumps **independently**:

- **Patch bump** (`1.0.0 → 1.0.1`): clarifying description text only.
- **Minor bump** (`1.0.0 → 1.1.0`): adding optional fields.
- **Major bump** (`1.0.0 → 2.0.0`): removing fields, tightening constraints,
  changing required field set, or changing field types.

Enum files follow the same rules: additive value → minor; removal or
rename → major.

Phase B's `framework.lock` will record the version of each schema and
each enum in use. The gate itself does not produce a `framework.lock`.

## 9. What this gate does NOT do

- **Does not generate Go types.** Phase A entity chunks own typed
  structs and per-entity loaders.
- **Does not enforce referential integrity.** A Sensor referencing a
  non-existent UseCase passes the gate's validation. Runtime loaders
  must check existence.
- **Does not provide a CLI.** Only `cmd/validate-schemas` exists; the
  `harness` CLI is Phase B.
- **Does not implement skills or runtime logic.**

## 10. Items deferred to Phase A

- Canonical content-hashing rule for ids (if entity chunks decide to
  derive ids from content hashes — the gate only enforces form).
- Reconciliation mechanism between inline `enum:` clauses and
  `enums/*.yaml` (codegen vs CI consistency test).
- Whether Go loaders treat unknown fields strictly (schema already
  says `additionalProperties: false`, but loaders may add their own
  policy).
- Migration strategy when an enum value is renamed or removed.
```

- [ ] **Step 2: Commit**

```bash
git add schemas/README.md
git commit -m "docs(gate): add cross-entity contract README"
```

---

## Task 30: Final acceptance run

**Files:**
- (none — verification only)

This task confirms the gate is fully assembled and that the validator catches deliberate errors.

- [ ] **Step 1: Run the validator against the complete tree**

Run:
```bash
go run ./cmd/validate-schemas
```

Expected output ends with:
```
All schemas, enums, and examples validated.
```

Exit code 0. Total `OK ...` lines: 8 (entity schemas) + 8 (enum files) + 44 (examples) = **60**.

- [ ] **Step 2: Negative case — break an example deliberately**

Edit `schemas/examples/stack-component/library.yaml`: change `kind: library` to `kind: wat`.

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep -i fail
```

Expected: at least one `FAIL schemas/examples/stack-component/library.yaml: ...` line. Exit code 1.

- [ ] **Step 3: Restore the example**

Revert the edit:
```bash
git checkout -- schemas/examples/stack-component/library.yaml
```

Run validator again:
```bash
go run ./cmd/validate-schemas
```

Expected: exit 0, `All schemas, enums, and examples validated.`

- [ ] **Step 4: Negative case — break an enum deliberately**

Edit `schemas/enums/verdicts.yaml`: remove the `purpose:` field from the `pass` value.

Run:
```bash
go run ./cmd/validate-schemas 2>&1 | grep -i "verdicts"
```

Expected: a `FAIL schemas/enums/verdicts.yaml: ...` line reporting the missing `purpose`. Exit code 1.

- [ ] **Step 5: Restore the enum**

```bash
git checkout -- schemas/enums/verdicts.yaml
```

Run validator:
```bash
go run ./cmd/validate-schemas
```

Expected: exit 0.

- [ ] **Step 6: Final commit and acceptance check**

There should be nothing to commit at this point (the negative cases were reverted). Confirm:

```bash
git status
```

Expected: `nothing to commit, working tree clean`.

Print the final acceptance summary:

```bash
echo "Schema-freeze gate complete."
echo "Files produced:"
find schemas cmd go.mod -type f | sort
```

Expected: 65 paths (1 README + 8 entity schemas + 1 _meta + 8 enums + 44 examples + 1 main.go + go.mod + go.sum).

The gate is complete. Phase A entity chunks (E1–E9) may now proceed in parallel.
