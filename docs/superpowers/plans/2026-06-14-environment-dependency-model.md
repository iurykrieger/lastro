# Operational Environment Dependency Model — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make sensor generation discover how an app boots — a dependency model of every piece of software the app needs to execute — so `depends_on` closures, commands, and readiness are discovered rather than hand-authored (issue #52).

**Architecture:** A new `.harness/environment-model.yaml` (a dependency graph: `application` + `dependencies` + `setup` nodes, each carrying a grounded `provided_by` pointer and `depends_on` edges) is produced by a new `/detect-environment` skill (deterministic Go parser → LLM classifier → Go validator/persist). `/create-core-sensors` projects one core sensor per node; the existing resolver + `servicemgr` consume the discovered edges unchanged. Precondition failures aggregate to `inconclusive` via typed observation keys.

**Tech Stack:** Go 1.22, `sigs.k8s.io/yaml`, `github.com/santhosh-tekuri/jsonschema/v6`, embedded schemas (`schemas/schemas.go`), the `lastro` multi-subcommand binary (`cmd/lastro`).

**Spec:** `docs/superpowers/specs/2026-06-14-environment-dependency-model-design.md`

**Out of scope:** #22 (triggering `/heal` on setup-command failures); reconciling conflicting infra facts.

**Module path:** `github.com/iurykrieger/lastro`. Run Go via the toolchain at `/usr/lib/go-1.22/bin` (on PATH as `go`). Run all commands from the repo root unless stated.

---

## Phase boundaries (natural PR/checkpoint points)

1. **Phase 1 — Schema & enums foundation** (no Go deps): the model schema, golden examples, precondition-signals enum.
2. **Phase 2 — `internal/environment/` package**: types, parser, validator (grounding/edge/cycle), persist — all TDD, table-driven.
3. **Phase 3 — `/detect-environment` skill**: `cmd/lastro` subcommand + SKILL.md.
4. **Phase 4 — `/detect-stack` extension**: record `docker`/`compose` as a `StackComponent`.
5. **Phase 5 — `/create-core-sensors` consumption**: SKILL.md rewrite + golden core sensors.
6. **Phase 6 — Typed precondition signals**: type the existing service-error path; add `setup_unavailable`.
7. **Phase 7 — Integration & dogfood**: `examples/nextjs-drizzle-sample` fixture test + self-dogfood no-op.

Each phase ends green (build + tests) and is committable on its own.

---

# Phase 1 — Schema & enums foundation

### Task 1: The environment-model JSON Schema

**Files:**
- Create: `schemas/environment-model.yaml`

The schema is embedded automatically — `schemas/schemas.go` already declares `//go:embed *.yaml enums/*.yaml core-inputs/*.yaml`, so a new top-level `*.yaml` is picked up with no Go change. Examples live under `schemas/examples/` which is intentionally NOT embedded (loaded from disk by tests).

- [ ] **Step 1: Write the schema file**

```yaml
# schemas/environment-model.yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/environment-model.yaml"
title: EnvironmentModel
description: >
  The operational dependency model: every piece of software the application
  depends on to execute. Produced by /detect-environment, consumed by
  /create-core-sensors to emit one core sensor per node. References sensors via
  grounded provided_by pointers; never restates commands, env, or readiness.
type: object
additionalProperties: false
required: [schema_version, application]
properties:
  schema_version:
    type: string
    pattern: "^\\d+\\.\\d+\\.\\d+$"
  application:
    type: object
    additionalProperties: false
    required: [provided_by]
    properties:
      provided_by: { $ref: "#/$defs/ProvidedBy" }
      depends_on:
        type: array
        items: { $ref: "#/$defs/NodeName" }
  dependencies:
    type: object
    propertyNames: { $ref: "#/$defs/NodeName" }
    additionalProperties: { $ref: "#/$defs/Dependency" }
  setup:
    type: array
    items: { $ref: "#/$defs/SetupNode" }
$defs:
  NodeName:
    type: string
    pattern: "^[a-z][a-z0-9-]*$"
    maxLength: 128
  ProvidedBy:
    type: object
    additionalProperties: false
    required: [file, path]
    properties:
      file: { type: string, minLength: 1 }
      path: { type: string, minLength: 1 }
  Dependency:
    type: object
    additionalProperties: false
    required: [type, provided_by]
    properties:
      type:
        type: string
        enum: [datastore, cache, broker]
      provided_by: { $ref: "#/$defs/ProvidedBy" }
      depends_on:
        type: array
        items: { $ref: "#/$defs/NodeName" }
  SetupNode:
    type: object
    additionalProperties: false
    required: [id, type, provided_by]
    properties:
      id: { $ref: "#/$defs/NodeName" }
      type: { type: string, const: setup }
      provided_by: { $ref: "#/$defs/ProvidedBy" }
      depends_on:
        type: array
        items: { $ref: "#/$defs/NodeName" }
      optional: { type: boolean, default: false }
```

- [ ] **Step 2: Commit**

```bash
git add schemas/environment-model.yaml
git commit -m "feat(#52): environment-model JSON schema"
```

### Task 2: Golden examples

**Files:**
- Create: `schemas/examples/environment-model/nextjs-drizzle.yaml`
- Create: `schemas/examples/environment-model/no-op.yaml`

- [ ] **Step 1: Write the reference example (Next.js + Drizzle + Postgres)**

```yaml
# schemas/examples/environment-model/nextjs-drizzle.yaml
schema_version: 1.0.0
application:
  provided_by: { file: package.json, path: "scripts.dev" }
  depends_on: [postgres, migrate]
dependencies:
  postgres:
    type: datastore
    provided_by: { file: docker-compose.yml, path: services.postgres }
    depends_on: []
setup:
  - id: migrate
    type: setup
    provided_by: { file: package.json, path: "scripts.db:migrate" }
    depends_on: [postgres]
    optional: false
```

- [ ] **Step 2: Write the no-op example (bare app, no infra)**

```yaml
# schemas/examples/environment-model/no-op.yaml
schema_version: 1.0.0
application:
  provided_by: { file: package.json, path: "scripts.start" }
  depends_on: []
```

- [ ] **Step 3: Commit**

```bash
git add schemas/examples/environment-model/
git commit -m "feat(#52): golden environment-model examples"
```

### Task 3: precondition-signals enum

**Files:**
- Create: `schemas/enums/precondition-signals.yaml`

This documents the reserved `observation_key` values that aggregate to `inconclusive`. `missing-env` already exists in code (`internal/runtime/executor/envsignal.go`); this enum records the family. It follows the `_meta.yaml` shape used by `verdicts.yaml`/`termination-reasons.yaml`.

- [ ] **Step 1: Write the enum file**

```yaml
# schemas/enums/precondition-signals.yaml
schema_version: 1.0.0
title: PreconditionSignal
description: |
  Reserved observation_key values emitted when a runtime precondition cannot be
  established. All carry verdict: inconclusive (never fail) — the harness could
  not set up the world, so it cannot claim the application is broken. Each
  payload names the unmet piece and carries a human remediation hint.

values:
  - id: missing-env
    purpose: "A required env var (sensor env: EnvSpec) was absent/empty pre-spawn"
  - id: missing-service
    purpose: "A dependency node's bring-up failed (no Docker daemon, image pull failed, port taken)"
  - id: unready-service
    purpose: "Bring-up ran but the readiness probe never passed within its timeout"
  - id: setup-unavailable
    purpose: "A setup node's command is absent or unresolvable"
```

- [ ] **Step 2: Verify it loads under the existing enum embed/test**

Run: `go test ./schemas/... ./internal/enums/... 2>&1 | tail -20`
Expected: PASS (the new file matches `enums/_meta.yaml`; if an enum-coverage test enumerates files, it now includes precondition-signals).

If a test fails because it asserts an exact enum-file list, add `precondition-signals` to that list (search: `grep -rn "termination-reasons" internal/ schemas/`).

- [ ] **Step 3: Commit**

```bash
git add schemas/enums/precondition-signals.yaml
git commit -m "feat(#52): precondition-signals enum (missing/unready/setup)"
```

---

# Phase 2 — `internal/environment/` package

Mirror `internal/stack` exactly: `types.go`, `parse*.go` (deterministic parsers), `load.go` (schema + programmatic validate), `validate.go` (grounding/edge/cycle), `persist.go`. Every file gets a `_test.go` sibling.

### Task 4: Model + facts types

**Files:**
- Create: `internal/environment/types.go`
- Test: `internal/environment/types_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/environment/types_test.go
package environment

import "testing"

func TestProvidedBy_RoundTrip(t *testing.T) {
	m := EnvironmentModel{
		SchemaVersion: "1.0.0",
		Application:   Application{ProvidedBy: ProvidedBy{File: "package.json", Path: "scripts.dev"}, DependsOn: []string{"postgres"}},
		Dependencies: map[string]Dependency{
			"postgres": {Type: "datastore", ProvidedBy: ProvidedBy{File: "docker-compose.yml", Path: "services.postgres"}},
		},
		Setup: []SetupNode{{ID: "migrate", Type: "setup", ProvidedBy: ProvidedBy{File: "package.json", Path: "scripts.db:migrate"}, DependsOn: []string{"postgres"}}},
	}
	if got := m.Dependencies["postgres"].Type; got != DependencyDatastore {
		t.Fatalf("type = %q, want %q", got, DependencyDatastore)
	}
	names := m.NodeNames()
	if len(names) != 3 { // postgres, migrate, application is not a depend-able node
		t.Fatalf("NodeNames = %v, want 3 (application excluded as a target)", names)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/environment/ -run TestProvidedBy_RoundTrip -v`
Expected: FAIL — package/types not defined.

- [ ] **Step 3: Write the types**

```go
// internal/environment/types.go

// Package environment models the operational dependency graph of a project —
// every piece of software the application needs to execute — and the
// deterministic parsers + validators that produce it. It mirrors the
// internal/stack package: deterministic parse/validate/persist; the LLM
// (the /detect-environment skill) only classifies raw facts into the model.
package environment

// DependencyType labels a backing service. It drives the SHAPE of the core
// sensor generation emits (readiness probe + observational vs single-shot),
// never a duplicated command.
type DependencyType string

const (
	DependencyDatastore DependencyType = "datastore"
	DependencyCache     DependencyType = "cache"
	DependencyBroker    DependencyType = "broker"
)

// ProvidedBy is a grounded pointer to where a node's launch command lives. It
// is NOT the resolved command: generation reads the command from this {file,
// path} at generation time, so the command string exists in exactly one
// authored place (package.json / docker-compose.yml).
type ProvidedBy struct {
	File string `json:"file" yaml:"file"`
	Path string `json:"path" yaml:"path"`
}

// Application is the system under test. It has no `type` (it is implicitly the
// run-dev service). depends_on names backing dependencies and setup nodes.
type Application struct {
	ProvidedBy ProvidedBy `json:"provided_by" yaml:"provided_by"`
	DependsOn  []string   `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
}

// Dependency is a long-running backing service (datastore/cache/broker).
type Dependency struct {
	Type       DependencyType `json:"type" yaml:"type"`
	ProvidedBy ProvidedBy     `json:"provided_by" yaml:"provided_by"`
	DependsOn  []string       `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
}

// SetupNode is a run-to-completion task (migrate/seed). Success = exit 0.
type SetupNode struct {
	ID         string     `json:"id" yaml:"id"`
	Type       string     `json:"type" yaml:"type"` // always "setup"
	ProvidedBy ProvidedBy `json:"provided_by" yaml:"provided_by"`
	DependsOn  []string   `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Optional   bool       `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// EnvironmentModel is the full dependency graph persisted to
// .harness/environment-model.yaml.
type EnvironmentModel struct {
	SchemaVersion string                `json:"schema_version" yaml:"schema_version"`
	Application   Application           `json:"application" yaml:"application"`
	Dependencies  map[string]Dependency `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Setup         []SetupNode           `json:"setup,omitempty" yaml:"setup,omitempty"`
}

// NodeNames returns every depend-able node name (dependency keys + setup ids).
// The application is excluded — nothing depends on the app.
func (m EnvironmentModel) NodeNames() []string {
	names := make([]string, 0, len(m.Dependencies)+len(m.Setup))
	for k := range m.Dependencies {
		names = append(names, k)
	}
	for _, s := range m.Setup {
		names = append(names, s.ID)
	}
	return names
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/environment/ -run TestProvidedBy_RoundTrip -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/environment/types.go internal/environment/types_test.go
git commit -m "feat(#52): environment model Go types"
```

### Task 5: RawFacts type + grounding resolver

**Files:**
- Create: `internal/environment/facts.go`
- Test: `internal/environment/facts_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/environment/facts_test.go
package environment

import "testing"

func TestRawFacts_Resolve(t *testing.T) {
	f := RawFacts{
		Scripts:         map[string]string{"dev": "next dev", "db:migrate": "drizzle-kit migrate"},
		ComposeServices: map[string]ComposeService{"postgres": {Image: "postgres:16-alpine"}},
		MakeTargets:     map[string]string{"run": "go run ."},
		ProcfileEntries: map[string]string{"web": "node server.js"},
	}
	cases := []struct {
		p    ProvidedBy
		want string
		ok   bool
	}{
		{ProvidedBy{"package.json", "scripts.dev"}, "next dev", true},
		{ProvidedBy{"package.json", "scripts.db:migrate"}, "drizzle-kit migrate", true},
		{ProvidedBy{"docker-compose.yml", "services.postgres"}, "docker compose up -d postgres", true},
		{ProvidedBy{"Makefile", "run"}, "go run .", true},
		{ProvidedBy{"Procfile", "web"}, "node server.js", true},
		{ProvidedBy{"package.json", "scripts.missing"}, "", false},
		{ProvidedBy{"unknown.txt", "x"}, "", false},
	}
	for _, c := range cases {
		got, ok := f.Resolve(c.p)
		if ok != c.ok || got != c.want {
			t.Errorf("Resolve(%+v) = (%q,%v), want (%q,%v)", c.p, got, ok, c.want, c.ok)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/environment/ -run TestRawFacts_Resolve -v`
Expected: FAIL — RawFacts not defined.

- [ ] **Step 3: Write facts.go**

```go
// internal/environment/facts.go
package environment

import (
	"strings"
)

// ComposeHealthcheck mirrors a compose service healthcheck (captured verbatim
// when declared; generation reads it to derive readiness).
type ComposeHealthcheck struct {
	Test     []string `json:"test,omitempty" yaml:"test,omitempty"`
	Interval string   `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timeout  string   `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Retries  int      `json:"retries,omitempty" yaml:"retries,omitempty"`
}

// ComposeService is a parsed docker-compose service, verbatim.
type ComposeService struct {
	Image       string              `json:"image,omitempty" yaml:"image,omitempty"`
	Ports       []string            `json:"ports,omitempty" yaml:"ports,omitempty"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	DependsOn   []string            `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Healthcheck *ComposeHealthcheck `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
}

// RawFacts is the deterministic parser output: operational facts extracted
// verbatim from infra files, with no interpretation. The classifier (skill)
// turns these into an EnvironmentModel; ValidateGrounding cross-checks every
// provided_by against them.
type RawFacts struct {
	Scripts          map[string]string         `json:"scripts,omitempty" yaml:"scripts,omitempty"`
	MakeTargets      map[string]string         `json:"make_targets,omitempty" yaml:"make_targets,omitempty"`
	ProcfileEntries  map[string]string         `json:"procfile_entries,omitempty" yaml:"procfile_entries,omitempty"`
	ComposeServices  map[string]ComposeService `json:"compose_services,omitempty" yaml:"compose_services,omitempty"`
	ComposeFile      string                    `json:"compose_file,omitempty" yaml:"compose_file,omitempty"`
	EnvKeys          []string                  `json:"env_keys,omitempty" yaml:"env_keys,omitempty"`
	RequiredEnvHints []string                  `json:"required_env_hints,omitempty" yaml:"required_env_hints,omitempty"`
}

// Resolve returns the launch command a provided_by pointer grounds to, and
// whether it resolved. Compose services resolve to a deterministic
// `docker compose up -d <svc>` (the actual image/ports live in the compose
// file, which that command invokes — never duplicated here).
func (f RawFacts) Resolve(p ProvidedBy) (string, bool) {
	switch {
	case isPackageJSON(p.File) && strings.HasPrefix(p.Path, "scripts."):
		name := strings.TrimPrefix(p.Path, "scripts.")
		cmd, ok := f.Scripts[name]
		if !ok {
			return "", false
		}
		return cmd, true
	case isComposeFile(p.File) && strings.HasPrefix(p.Path, "services."):
		name := strings.TrimPrefix(p.Path, "services.")
		if _, ok := f.ComposeServices[name]; !ok {
			return "", false
		}
		return "docker compose up -d " + name, true
	case p.File == "Makefile":
		cmd, ok := f.MakeTargets[p.Path]
		if !ok {
			return "", false
		}
		return cmd, true
	case p.File == "Procfile":
		cmd, ok := f.ProcfileEntries[p.Path]
		if !ok {
			return "", false
		}
		return cmd, true
	}
	return "", false
}

func isPackageJSON(file string) bool { return file == "package.json" }

func isComposeFile(file string) bool {
	switch file {
	case "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
		return true
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/environment/ -run TestRawFacts_Resolve -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/environment/facts.go internal/environment/facts_test.go
git commit -m "feat(#52): RawFacts type + provided_by grounding resolver"
```

### Task 6: package.json + Procfile + Makefile parsers

**Files:**
- Create: `internal/environment/parse_scripts.go`
- Test: `internal/environment/parse_scripts_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/environment/parse_scripts_test.go
package environment

import (
	"path/filepath"
	"testing"
)

func TestParsePackageScripts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{
	  "scripts": { "dev": "next dev", "db:migrate": "drizzle-kit migrate" }
	}`)
	got, err := parsePackageScripts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got["dev"] != "next dev" || got["db:migrate"] != "drizzle-kit migrate" {
		t.Fatalf("scripts = %v", got)
	}
}

func TestParsePackageScripts_Absent(t *testing.T) {
	got, err := parsePackageScripts(t.TempDir())
	if err != nil {
		t.Fatalf("absent package.json must not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}

func TestParseProcfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Procfile"), "web: node server.js\nworker: node worker.js\n")
	got := parseProcfile(dir)
	if got["web"] != "node server.js" || got["worker"] != "node worker.js" {
		t.Fatalf("procfile = %v", got)
	}
}
```

- [ ] **Step 2: Add the shared test helper**

**Files:** Create: `internal/environment/helpers_test.go`

```go
// internal/environment/helpers_test.go
package environment

import (
	"os"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/environment/ -run 'TestParsePackageScripts|TestParseProcfile' -v`
Expected: FAIL — parse functions not defined.

- [ ] **Step 4: Write parse_scripts.go**

```go
// internal/environment/parse_scripts.go
package environment

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// parsePackageScripts reads package.json's "scripts" object. Absent file →
// empty map, no error (graceful degradation).
func parsePackageScripts(repoDir string) (map[string]string, error) {
	b, err := os.ReadFile(filepath.Join(repoDir, "package.json"))
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		return nil, err
	}
	if pkg.Scripts == nil {
		return map[string]string{}, nil
	}
	return pkg.Scripts, nil
}

// parseProcfile reads a Procfile (`name: command` per line). Absent → empty.
func parseProcfile(repoDir string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(filepath.Join(repoDir, "Procfile"))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, cmd, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		out[strings.TrimSpace(name)] = strings.TrimSpace(cmd)
	}
	return out
}

// parseMakeTargets reads `target:` lines from a Makefile. The recipe body is
// not captured — generation invokes `make <target>` (the target name is the
// grounding); we store the target with a "make <name>" placeholder command so
// Resolve has a non-empty value to return.
func parseMakeTargets(repoDir string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(filepath.Join(repoDir, "Makefile"))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "#") {
			continue
		}
		target, _, ok := strings.Cut(line, ":")
		target = strings.TrimSpace(target)
		if !ok || target == "" || strings.ContainsAny(target, " =.") {
			continue
		}
		out[target] = "make " + target
	}
	return out
}
```

> Note: the Makefile test in facts_test.go asserts `MakeTargets["run"] == "go run ."`; that map is built by the *test* directly, not the parser. The parser stores `"make run"`. Both are valid grounding values; `Resolve` returns whatever is in the map.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/environment/ -run 'TestParsePackageScripts|TestParseProcfile' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/environment/parse_scripts.go internal/environment/parse_scripts_test.go internal/environment/helpers_test.go
git commit -m "feat(#52): package.json/Procfile/Makefile parsers"
```

### Task 7: docker-compose + dotenv parsers

**Files:**
- Create: `internal/environment/parse_compose.go`
- Test: `internal/environment/parse_compose_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/environment/parse_compose_test.go
package environment

import (
	"path/filepath"
	"testing"
)

func TestParseCompose(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "docker-compose.yml"), `services:
  postgres:
    image: postgres:16-alpine
    ports: ["5432:5432"]
    environment:
      POSTGRES_USER: reliable
    depends_on: []
volumes:
  pgdata:
`)
	svcs, file, err := parseCompose(dir)
	if err != nil {
		t.Fatal(err)
	}
	if file != "docker-compose.yml" {
		t.Fatalf("file = %q", file)
	}
	pg, ok := svcs["postgres"]
	if !ok || pg.Image != "postgres:16-alpine" || pg.Ports[0] != "5432:5432" {
		t.Fatalf("postgres = %+v ok=%v", pg, ok)
	}
}

func TestParseCompose_Absent(t *testing.T) {
	svcs, file, err := parseCompose(t.TempDir())
	if err != nil || len(svcs) != 0 || file != "" {
		t.Fatalf("absent compose: svcs=%v file=%q err=%v", svcs, file, err)
	}
}

func TestParseDotenvKeys(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env.example"), "# comment\nDATABASE_URL=postgres://x\nNEXTAUTH_SECRET=changeme\n\nEMPTY=\n")
	keys := parseDotenvKeys(dir)
	want := map[string]bool{"DATABASE_URL": true, "NEXTAUTH_SECRET": true, "EMPTY": true}
	if len(keys) != 3 {
		t.Fatalf("keys = %v", keys)
	}
	for _, k := range keys {
		if !want[k] {
			t.Fatalf("unexpected key %q in %v", k, keys)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/environment/ -run 'TestParseCompose|TestParseDotenv' -v`
Expected: FAIL — parse functions not defined.

- [ ] **Step 3: Write parse_compose.go**

```go
// internal/environment/parse_compose.go
package environment

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

var composeCandidates = []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}

// parseCompose reads the first present compose file and returns its services
// verbatim plus the filename used (for provided_by grounding). Absent → empty.
func parseCompose(repoDir string) (map[string]ComposeService, string, error) {
	for _, name := range composeCandidates {
		b, err := os.ReadFile(filepath.Join(repoDir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		var doc struct {
			Services map[string]ComposeService `json:"services"`
		}
		if err := yaml.Unmarshal(b, &doc); err != nil {
			return nil, "", err
		}
		if doc.Services == nil {
			doc.Services = map[string]ComposeService{}
		}
		return doc.Services, name, nil
	}
	return map[string]ComposeService{}, "", nil
}

// parseDotenvKeys returns the key names declared in .env.example (preferred) or
// .env. Values are ignored. Absent → empty.
func parseDotenvKeys(repoDir string) []string {
	for _, name := range []string{".env.example", ".env.local", ".env"} {
		f, err := os.Open(filepath.Join(repoDir, name))
		if err != nil {
			continue
		}
		defer f.Close()
		var keys []string
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, _, ok := strings.Cut(line, "=")
			key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
			if ok && key != "" {
				keys = append(keys, key)
			}
		}
		return keys
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/environment/ -run 'TestParseCompose|TestParseDotenv' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/environment/parse_compose.go internal/environment/parse_compose_test.go
git commit -m "feat(#52): docker-compose + dotenv parsers"
```

### Task 8: top-level Parse orchestrator

**Files:**
- Create: `internal/environment/parse.go`
- Test: `internal/environment/parse_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/environment/parse_test.go
package environment

import (
	"path/filepath"
	"testing"
)

func TestParse_DashboardShape(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"scripts":{"dev":"next dev","db:migrate":"drizzle-kit migrate"}}`)
	writeFile(t, filepath.Join(dir, "docker-compose.yml"), "services:\n  postgres:\n    image: postgres:16-alpine\n")
	writeFile(t, filepath.Join(dir, ".env.example"), "DATABASE_URL=x\n")
	f, err := Parse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.Scripts["dev"] != "next dev" {
		t.Errorf("scripts.dev = %q", f.Scripts["dev"])
	}
	if _, ok := f.ComposeServices["postgres"]; !ok {
		t.Errorf("missing compose postgres: %v", f.ComposeServices)
	}
	if f.ComposeFile != "docker-compose.yml" {
		t.Errorf("compose file = %q", f.ComposeFile)
	}
	if len(f.EnvKeys) != 1 || f.EnvKeys[0] != "DATABASE_URL" {
		t.Errorf("env keys = %v", f.EnvKeys)
	}
}

func TestParse_NoInfra(t *testing.T) {
	f, err := Parse(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Scripts) != 0 || len(f.ComposeServices) != 0 {
		t.Fatalf("want empty facts, got %+v", f)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/environment/ -run TestParse_ -v`
Expected: FAIL — Parse not defined.

- [ ] **Step 3: Write parse.go**

```go
// internal/environment/parse.go
package environment

// Parse runs every deterministic parser over repoDir and assembles RawFacts.
// It never interprets — classification is the skill's job. Missing infra files
// degrade to empty maps, never errors.
func Parse(repoDir string) (RawFacts, error) {
	scripts, err := parsePackageScripts(repoDir)
	if err != nil {
		return RawFacts{}, err
	}
	compose, composeFile, err := parseCompose(repoDir)
	if err != nil {
		return RawFacts{}, err
	}
	return RawFacts{
		Scripts:         scripts,
		MakeTargets:     parseMakeTargets(repoDir),
		ProcfileEntries: parseProcfile(repoDir),
		ComposeServices: compose,
		ComposeFile:     composeFile,
		EnvKeys:         parseDotenvKeys(repoDir),
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/environment/ -run TestParse_ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/environment/parse.go internal/environment/parse_test.go
git commit -m "feat(#52): top-level RawFacts parse orchestrator"
```

### Task 9: model schema load + programmatic validate (edges + cycles)

**Files:**
- Create: `internal/environment/load.go`
- Create: `internal/environment/validate.go`
- Test: `internal/environment/validate_test.go`

The schema URL + compile pattern mirrors `internal/stack/load.go` (santhosh-tekuri/jsonschema + sigs.k8s.io/yaml, reading from `schemas.FS`).

- [ ] **Step 1: Write the failing test**

```go
// internal/environment/validate_test.go
package environment

import (
	"strings"
	"testing"
)

func TestValidate_DanglingEdge(t *testing.T) {
	m := EnvironmentModel{
		SchemaVersion: "1.0.0",
		Application:   Application{ProvidedBy: ProvidedBy{"package.json", "scripts.dev"}, DependsOn: []string{"ghost"}},
	}
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("want dangling-edge error naming ghost, got %v", err)
	}
}

func TestValidate_Cycle(t *testing.T) {
	m := EnvironmentModel{
		SchemaVersion: "1.0.0",
		Application:   Application{ProvidedBy: ProvidedBy{"package.json", "scripts.dev"}},
		Dependencies: map[string]Dependency{
			"a": {Type: "datastore", ProvidedBy: ProvidedBy{"docker-compose.yml", "services.a"}, DependsOn: []string{"b"}},
			"b": {Type: "datastore", ProvidedBy: ProvidedBy{"docker-compose.yml", "services.b"}, DependsOn: []string{"a"}},
		},
	}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want cycle error, got %v", err)
	}
}

func TestValidate_OK(t *testing.T) {
	m := EnvironmentModel{
		SchemaVersion: "1.0.0",
		Application:   Application{ProvidedBy: ProvidedBy{"package.json", "scripts.dev"}, DependsOn: []string{"postgres", "migrate"}},
		Dependencies:  map[string]Dependency{"postgres": {Type: "datastore", ProvidedBy: ProvidedBy{"docker-compose.yml", "services.postgres"}}},
		Setup:         []SetupNode{{ID: "migrate", Type: "setup", ProvidedBy: ProvidedBy{"package.json", "scripts.db:migrate"}, DependsOn: []string{"postgres"}}},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("valid model rejected: %v", err)
	}
}

func TestLoadBytes_SchemaReject(t *testing.T) {
	// type not in enum → schema violation
	_, err := LoadBytes([]byte("schema_version: 1.0.0\napplication:\n  provided_by: {file: package.json, path: scripts.dev}\ndependencies:\n  x: {type: bogus, provided_by: {file: docker-compose.yml, path: services.x}}\n"))
	if err == nil {
		t.Fatal("want schema violation for bogus type")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/environment/ -run 'TestValidate_|TestLoadBytes_' -v`
Expected: FAIL — Validate/LoadBytes not defined.

- [ ] **Step 3: Write validate.go (edges + cycle, Kahn)**

```go
// internal/environment/validate.go
package environment

import (
	"fmt"
	"sort"
)

// Validate enforces model-shape invariants that the JSON Schema cannot:
// unique node names, every depends_on edge resolves to a declared node, and
// the graph is acyclic. (Grounding against RawFacts is ValidateGrounding.)
func (m EnvironmentModel) Validate() error {
	// Build node set + edge map. Application participates as a source node
	// (id "application") but is never a depends_on target.
	const appID = "application"
	edges := map[string][]string{appID: m.Application.DependsOn}
	nodes := map[string]bool{appID: true}

	for name := range m.Dependencies {
		if nodes[name] {
			return fmt.Errorf("environment: duplicate node name %q", name)
		}
		nodes[name] = true
	}
	for _, s := range m.Setup {
		if nodes[s.ID] {
			return fmt.Errorf("environment: duplicate node name %q", s.ID)
		}
		nodes[s.ID] = true
	}
	for name, d := range m.Dependencies {
		edges[name] = d.DependsOn
	}
	for _, s := range m.Setup {
		edges[s.ID] = s.DependsOn
	}

	// Edge integrity: every target must be a declared node (not application).
	for src, targets := range edges {
		for _, tgt := range targets {
			if tgt == appID || !nodes[tgt] {
				return fmt.Errorf("environment: node %q depends_on unknown node %q", src, tgt)
			}
		}
	}

	return acyclic(nodes, edges)
}

// acyclic runs Kahn's algorithm; returns an *fmt.Errorf naming "cycle" when one
// remains. Deterministic ordering via sorted keys.
func acyclic(nodes map[string]bool, edges map[string][]string) error {
	indeg := map[string]int{}
	for n := range nodes {
		indeg[n] = 0
	}
	for _, targets := range edges {
		for _, t := range targets {
			indeg[t]++
		}
	}
	var queue []string
	for n := range nodes {
		if indeg[n] == 0 {
			queue = append(queue, n)
		}
	}
	sort.Strings(queue)
	visited := 0
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		visited++
		next := append([]string{}, edges[n]...)
		sort.Strings(next)
		for _, t := range next {
			indeg[t]--
			if indeg[t] == 0 {
				queue = append(queue, t)
			}
		}
	}
	if visited != len(nodes) {
		return fmt.Errorf("environment: dependency graph has a cycle")
	}
	return nil
}
```

- [ ] **Step 4: Write load.go (mirror internal/stack/load.go)**

```go
// internal/environment/load.go
package environment

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/schemas"
)

const modelSchemaURL = "https://lastro.dev/harness/schemas/environment-model.yaml"

// Load reads, schema-validates, unmarshals, and programmatically validates an
// environment model file.
func Load(path string) (EnvironmentModel, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return EnvironmentModel{}, fmt.Errorf("read %s: %w", path, err)
	}
	m, err := LoadBytes(b)
	if err != nil {
		return EnvironmentModel{}, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// LoadBytes is the in-memory entrypoint (used by Persist's pre-write check).
func LoadBytes(b []byte) (EnvironmentModel, error) {
	sch, err := compileSchema()
	if err != nil {
		return EnvironmentModel{}, err
	}
	if err := validateAgainstSchema(b, sch); err != nil {
		return EnvironmentModel{}, err
	}
	var m EnvironmentModel
	if err := yaml.Unmarshal(b, &m); err != nil {
		return EnvironmentModel{}, fmt.Errorf("unmarshal: %w", err)
	}
	if err := m.Validate(); err != nil {
		return EnvironmentModel{}, err
	}
	return m, nil
}

func compileSchema() (*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()
	b, err := schemas.FS.ReadFile("environment-model.yaml")
	if err != nil {
		return nil, fmt.Errorf("read embedded environment-model.yaml: %w", err)
	}
	j, err := yaml.YAMLToJSON(b)
	if err != nil {
		return nil, fmt.Errorf("yaml->json: %w", err)
	}
	var doc any
	if err := json.Unmarshal(j, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}
	if err := c.AddResource(modelSchemaURL, doc); err != nil {
		return nil, fmt.Errorf("register schema: %w", err)
	}
	return c.Compile(modelSchemaURL)
}

func validateAgainstSchema(b []byte, sch *jsonschema.Schema) error {
	j, err := yaml.YAMLToJSON(b)
	if err != nil {
		return fmt.Errorf("yaml->json: %w", err)
	}
	var doc any
	if err := json.Unmarshal(j, &doc); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	if err := sch.Validate(doc); err != nil {
		return fmt.Errorf("schema validation: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/environment/ -run 'TestValidate_|TestLoadBytes_' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/environment/load.go internal/environment/validate.go internal/environment/validate_test.go
git commit -m "feat(#52): environment model schema load + edge/cycle validation"
```

### Task 10: grounding validation + Persist

**Files:**
- Create: `internal/environment/persist.go`
- Test: `internal/environment/persist_test.go`

Mirror `internal/stack/persist.go`: returns `*persisterror.Error` on validation failure; uses `persisthelp.BumpSchemaVersion` + `persisthelp.AtomicWrite`. Adds `ValidateGrounding` cross-check against RawFacts.

- [ ] **Step 1: Write the failing test**

```go
// internal/environment/persist_test.go
package environment

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/persisterror"
)

const validModelYAML = `schema_version: 1.0.0
application:
  provided_by: {file: package.json, path: scripts.dev}
  depends_on: [postgres, migrate]
dependencies:
  postgres:
    type: datastore
    provided_by: {file: docker-compose.yml, path: services.postgres}
setup:
  - id: migrate
    type: setup
    provided_by: {file: package.json, path: scripts.db:migrate}
    depends_on: [postgres]
`

const factsYAML = `scripts:
  dev: next dev
  db:migrate: drizzle-kit migrate
compose_services:
  postgres:
    image: postgres:16-alpine
compose_file: docker-compose.yml
`

func TestPersist_OK(t *testing.T) {
	dir := t.TempDir()
	if err := Persist([]byte(validModelYAML), []byte(factsYAML), dir); err != nil {
		t.Fatalf("persist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "environment-model.yaml")); err != nil {
		t.Fatalf("model not written: %v", err)
	}
}

func TestPersist_UngroundedProvidedBy(t *testing.T) {
	dir := t.TempDir()
	bad := `schema_version: 1.0.0
application:
  provided_by: {file: package.json, path: scripts.ghost}
`
	err := Persist([]byte(bad), []byte(factsYAML), dir)
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("want *persisterror.Error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "environment-model.yaml")); statErr == nil {
		t.Fatal("ungrounded model must NOT be written")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/environment/ -run TestPersist_ -v`
Expected: FAIL — Persist not defined.

- [ ] **Step 3: Write persist.go**

```go
// internal/environment/persist.go
package environment

import (
	"fmt"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/persisterror"
	"github.com/iurykrieger/lastro/internal/persisthelp"
)

const modelFilename = "environment-model.yaml"

// ValidateGrounding asserts every provided_by pointer resolves to a fact the
// parser actually extracted — the anti-hallucination guard.
func ValidateGrounding(m EnvironmentModel, f RawFacts) error {
	check := func(node string, p ProvidedBy) error {
		if _, ok := f.Resolve(p); !ok {
			return fmt.Errorf("node %q: provided_by {file:%q, path:%q} does not resolve to any parsed fact", node, p.File, p.Path)
		}
		return nil
	}
	if err := check("application", m.Application.ProvidedBy); err != nil {
		return err
	}
	for name, d := range m.Dependencies {
		if err := check(name, d.ProvidedBy); err != nil {
			return err
		}
	}
	for _, s := range m.Setup {
		if err := check(s.ID, s.ProvidedBy); err != nil {
			return err
		}
	}
	return nil
}

// Persist validates an LLM-emitted environment model (schema + edges/cycle +
// grounding against the parser's RawFacts), patch-bumps schema_version, and
// atomically writes it to <harnessDir>/environment-model.yaml. Returns a
// *persisterror.Error on any validation failure; nothing is written on error.
func Persist(modelContent, factsContent []byte, harnessDir string) error {
	model, err := LoadBytes(modelContent)
	if err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "environment-model", Message: err.Error()}
	}

	var facts RawFacts
	if err := yaml.Unmarshal(factsContent, &facts); err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "environment-model", Message: fmt.Sprintf("unmarshal facts: %v", err)}
	}
	if err := ValidateGrounding(model, facts); err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "environment-model", Message: err.Error()}
	}

	targetPath := filepath.Join(harnessDir, modelFilename)
	bumped, err := persisthelp.BumpSchemaVersion(targetPath, model.SchemaVersion)
	if err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "environment-model", Message: fmt.Sprintf("schema_version bump: %v", err)}
	}
	model.SchemaVersion = bumped

	out, err := yaml.Marshal(model)
	if err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "environment-model", Message: fmt.Sprintf("marshal: %v", err)}
	}
	if err := persisthelp.AtomicWrite(targetPath, out); err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "environment-model", Message: fmt.Sprintf("write %s: %v", targetPath, err)}
	}
	return nil
}
```

- [ ] **Step 4: Run tests + full package**

Run: `go test ./internal/environment/ -v`
Expected: PASS (all tasks 4-10).

- [ ] **Step 5: Commit**

```bash
git add internal/environment/persist.go internal/environment/persist_test.go
git commit -m "feat(#52): environment model grounding validation + persist"
```

---

# Phase 3 — `/detect-environment` skill

### Task 11: `cmd/lastro` detect-environment subcommand

**Files:**
- Create: `cmd/lastro/environment.go`
- Modify: `cmd/lastro/main.go` (add the subcommand case + usage line)
- Test: `cmd/lastro/environment_test.go`

The subcommand has two modes: `--mode facts` (run `environment.Parse(cwd)`, print RawFacts YAML to stdout) and `--mode persist --file <model> --facts <raw-facts>` (call `environment.Persist`). Mirrors `persistDetectStack`'s exit codes: 0 ok, 2 validation (JSON error on stdout), 1 script error.

- [ ] **Step 1: Write the failing test**

```go
// cmd/lastro/environment_test.go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectEnvironment_FactsMode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"dev":"next dev"}}`), 0o644)
	var out, errb bytes.Buffer
	code := detectEnvironment([]string{"detect-environment", "--mode", "facts", "--repo", dir}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "next dev") {
		t.Fatalf("facts output missing script: %s", out.String())
	}
}

func TestDetectEnvironment_PersistValidationError(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "m.yaml")
	facts := filepath.Join(dir, "f.yaml")
	os.WriteFile(model, []byte("schema_version: 1.0.0\napplication:\n  provided_by: {file: package.json, path: scripts.ghost}\n"), 0o644)
	os.WriteFile(facts, []byte("scripts:\n  dev: next dev\n"), 0o644)
	var out, errb bytes.Buffer
	code := detectEnvironment([]string{"detect-environment", "--mode", "persist", "--file", model, "--facts", facts, "--harness-dir", filepath.Join(dir, ".harness")}, &out, &errb)
	if code != 2 {
		t.Fatalf("want exit 2 (validation), got %d (stderr=%s)", code, errb.String())
	}
	if !strings.Contains(out.String(), "environment-model") {
		t.Fatalf("want JSON persisterror on stdout, got %s", out.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/lastro/ -run TestDetectEnvironment_ -v`
Expected: FAIL — detectEnvironment not defined.

- [ ] **Step 3: Write cmd/lastro/environment.go**

```go
// cmd/lastro/environment.go
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/environment"
	"github.com/iurykrieger/lastro/internal/persisterror"
)

func detectEnvironment(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("detect-environment", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mode := fs.String("mode", "", "facts | persist")
	repo := fs.String("repo", ".", "Repo root to parse (facts mode)")
	file := fs.String("file", "", "Path to the LLM-emitted environment-model YAML (persist mode)")
	facts := fs.String("facts", "", "Path to the raw-facts YAML from facts mode (persist mode)")
	harnessDir := fs.String("harness-dir", ".harness", "Target .harness directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}

	switch *mode {
	case "facts":
		f, err := environment.Parse(*repo)
		if err != nil {
			fmt.Fprintln(stderr, "parse:", err)
			return 1
		}
		b, err := yaml.Marshal(f)
		if err != nil {
			fmt.Fprintln(stderr, "marshal facts:", err)
			return 1
		}
		_, _ = stdout.Write(b)
		return 0
	case "persist":
		if *file == "" || *facts == "" {
			fmt.Fprintln(stderr, "persist mode requires --file and --facts")
			return 1
		}
		modelContent, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(stderr, "read model:", err)
			return 1
		}
		factsContent, err := os.ReadFile(*facts)
		if err != nil {
			fmt.Fprintln(stderr, "read facts:", err)
			return 1
		}
		if err := environment.Persist(modelContent, factsContent, *harnessDir); err != nil {
			var pe *persisterror.Error
			if errors.As(err, &pe) {
				_ = json.NewEncoder(stdout).Encode(pe)
				return 2
			}
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "detect-environment: invalid --mode %q (want facts or persist)\n", *mode)
		return 1
	}
}
```

- [ ] **Step 4: Wire into the router**

In `cmd/lastro/main.go`, add a usage line after the `detect-stack` line:

```go
		fmt.Fprintf(stderr, "  detect-environment  parse infra facts / validate-persist an environment-model YAML\n")
```

And add the case in the `switch sub` block after `case "detect-stack":`:

```go
	case "detect-environment":
		return detectEnvironment(rest, stdout, stderr)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/lastro/ -run TestDetectEnvironment_ -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 6: Commit**

```bash
git add cmd/lastro/environment.go cmd/lastro/environment_test.go cmd/lastro/main.go
git commit -m "feat(#52): detect-environment subcommand (facts + persist modes)"
```

### Task 12: `/detect-environment` SKILL.md

**Files:**
- Create: `skills/detect-environment/SKILL.md`

Mirror `skills/detect-stack/SKILL.md` structure and stay ≤200 lines (target ~120). The skill is the LLM classifier: run facts mode, classify into the model, run persist mode.

- [ ] **Step 1: Write the skill**

````markdown
---
name: detect-environment
description: Detect how the application boots — every piece of software it depends on to execute — and write .harness/environment-model.yaml. Run /detect-stack first. No argument.
---

# /detect-environment

You are building the **operational dependency model** for the repo at the current
working directory: every piece of software the application needs to *execute*, as a
graph that `/create-core-sensors` turns into core sensors. You classify; the script
parses and validates.

## Prerequisites

- `.harness/stack-manifest.yaml` must exist (run `/detect-stack` first).

## Step 1 — gather raw facts (deterministic)

Run the parser and capture its output (it never interprets — just extracts):

```bash
<plugin-root>/scripts/harness-tools.sh detect-environment --mode facts --repo . > /tmp/raw-facts.yaml
```

`raw-facts.yaml` contains: `scripts` (package.json), `make_targets`, `procfile_entries`,
`compose_services` (image/ports/environment/depends_on/healthcheck), `compose_file`,
and `env_keys`. Read it.

## Step 2 — classify into the dependency model

Write `/tmp/environment-model.yaml` matching `schemas/environment-model.yaml`. The
**only** authored values are classifications and grounded pointers — never resolved
commands, env, or readiness (those live in the sensors generation emits).

- **`application`** — pick the script that *is* the app (`dev`, then `start`).
  `provided_by: {file: package.json, path: "scripts.<name>"}`. Its `depends_on` lists
  every backing dependency and setup node it needs before it can serve.
- **`dependencies.<name>`** — one per backing service in `compose_services`. Set
  `type` to `datastore` | `cache` | `broker`. `provided_by: {file: <compose_file>,
  path: "services.<name>"}`. Carry compose `depends_on` verbatim.
- **`setup[]`** — run-to-completion tasks. Map `db:migrate` → a `setup` node
  `depends_on` the datastore; map `db:seed*` → `setup` nodes with `optional: true`.
  `provided_by: {file: package.json, path: "scripts.<name>"}`.

Infer edges the infra files imply but don't state: the app and migrate both
`depends_on` the datastore; the app `depends_on` migrate.

**Grounding rule:** every `provided_by` must point at a `{file, path}` present in
`raw-facts.yaml` (a real `scripts.*` / `services.*` / Makefile target / Procfile
entry). The validator rejects anything else.

**No infra?** Emit just an `application` node (empty `dependencies`/`setup`). No run
script at all → there is no operational environment; tell the user and stop.

**DO NOT** put commands, env vars, or readiness in this file. Sensors own those.

## Step 3 — validate + persist

```bash
<plugin-root>/scripts/harness-tools.sh detect-environment --mode persist \
  --file /tmp/environment-model.yaml --facts /tmp/raw-facts.yaml --harness-dir .harness
```

- **Exit 0:** written to `.harness/environment-model.yaml`. Done.
- **Exit 2:** JSON `persisterror.Error` on stdout (schema, dangling edge, cycle, or an
  ungrounded `provided_by`). Fix `/tmp/environment-model.yaml` and re-run. **Stop after
  3 attempts** and report.
- **Exit 1:** script error on stderr — report to the user.

> **Plugin users:** `<plugin-root>` is two directories above this skill file.
````

- [ ] **Step 2: Verify line budget**

Run: `wc -l skills/detect-environment/SKILL.md`
Expected: ≤200 (target ~120).

- [ ] **Step 3: Commit**

```bash
git add skills/detect-environment/SKILL.md
git commit -m "feat(#52): /detect-environment skill (classifier)"
```

---

# Phase 4 — `/detect-stack` records container tooling

`/create-core-sensors` will emit a datastore sensor whose top-level `uses:` must
ground in a `StackComponent`. So `/detect-stack` must record `docker`/`compose` as a
component when a compose file exists. This is a skill-text change (detection is
LLM-driven); no Go change is required because `StackComponent` already supports
`kind: tool`.

### Task 13: Teach /detect-stack to record compose tooling

**Files:**
- Modify: `skills/detect-stack/SKILL.md`
- Create: `schemas/examples/stack-component/compose-tool.yaml`

- [ ] **Step 1: Add the example component**

```yaml
# schemas/examples/stack-component/compose-tool.yaml
schema_version: 1.0.0
id: compose
kind: tool
name: docker compose
version: "2"
capabilities: [container-orchestration, service-provisioning]
detection_evidence:
  - file: docker-compose.yml
    path: services
```

- [ ] **Step 2: Add an instruction to detect-stack/SKILL.md**

Under the "What to inspect" list, add a bullet:

```markdown
- Container orchestration: when a `docker-compose.yml` / `compose.yaml` exists, record
  a `kind: tool` component with `id: compose` (or `docker`) and capabilities like
  `container-orchestration` — its evidence is the compose file. Core environment
  sensors that bring up backing services must ground their `uses:` in this component.
```

- [ ] **Step 3: Verify the example validates + line budget**

Run: `go test ./internal/stack/... 2>&1 | tail -5 && wc -l skills/detect-stack/SKILL.md`
Expected: stack tests PASS; SKILL.md ≤200 lines.

- [ ] **Step 4: Commit**

```bash
git add skills/detect-stack/SKILL.md schemas/examples/stack-component/compose-tool.yaml
git commit -m "feat(#52): /detect-stack records docker/compose as a tool component"
```

---

# Phase 5 — `/create-core-sensors` consumes the model

Generation is LLM-driven, so this is a SKILL.md change plus golden core-sensor
examples that show the node→sensor projection. The `lastro create-core-sensors`
validator already enforces `scope: core` and grounding; no Go change is needed.

### Task 14: Golden core sensors derived from the model

**Files:**
- Create: `schemas/examples/sensor/core-datastore-postgres.yaml`
- Create: `schemas/examples/sensor/core-migrate.yaml`

These show: datastore = observational/stream service with a `pg_isready` `key: ready`
matcher; migrate = assertion/single-shot with `env:` declared and `depends_on` the
datastore. (`core-run-dev.yaml` already exists.)

- [ ] **Step 1: Write the datastore sensor**

```yaml
# schemas/examples/sensor/core-datastore-postgres.yaml
schema_version: 1.0.0
id: core-datastore-postgres
scope: core
angle: environment
kind: observational
nature: computational
output_type: stream
uses: [compose]
signal_matches:
  - key: ready
    pattern: "accepting connections"
    verdict: pass
    expected: true
  - key: startup-error
    pattern: "could not|FATAL"
    verdict: fail
    heal_hint:
      summary: "Datastore failed to start"
      rationale: "Inspect docker compose logs for the postgres service."
steps:
  - id: up
    run: "docker compose up -d postgres && until docker compose exec -T postgres pg_isready -U postgres; do sleep 1; done && echo 'accepting connections' && docker compose logs -f postgres"
```

- [ ] **Step 2: Write the migrate sensor**

```yaml
# schemas/examples/sensor/core-migrate.yaml
schema_version: 1.0.0
id: core-migrate
scope: core
angle: environment
kind: assertion
nature: computational
output_type: single-shot
uses: []
depends_on: [core-datastore-postgres]
env:
  DATABASE_URL:
    description: "Connection string drizzle-kit migrate reads"
signal_matches:
  - key: migrate-failed
    pattern: "error|Error|failed"
    verdict: fail
    heal_hint:
      summary: "Migration failed"
      rationale: "A migration step exited non-zero; inspect the migration output."
steps:
  - id: migrate
    run: "npm run db:migrate"
```

- [ ] **Step 3: Verify both validate as core sensors**

Run: `go test ./internal/sensor/... 2>&1 | tail -5`
Expected: PASS (if a test enumerates `schemas/examples/sensor/*.yaml` it now includes these; both are valid core sensors).

- [ ] **Step 4: Commit**

```bash
git add schemas/examples/sensor/core-datastore-postgres.yaml schemas/examples/sensor/core-migrate.yaml
git commit -m "feat(#52): golden datastore + migrate core sensors"
```

### Task 15: Rewrite create-core-sensors/SKILL.md to consume the model

**Files:**
- Modify: `skills/create-core-sensors/SKILL.md`

Add a section (keeping ≤200 lines — synthesize/trim existing prose to make room) directing the skill to read `.harness/environment-model.yaml` and project nodes to sensors.

- [ ] **Step 1: Add the model-consumption section near the top of the skill body**

```markdown
## Ground environment sensors in the dependency model

If `.harness/environment-model.yaml` exists (run `/detect-environment` first), it is
the source of truth for the environment sensors — do NOT infer boot commands. Emit
**one core sensor per node**, resolving each command from the node's `provided_by`
pointer (read the named `package.json` script / compose service):

- **`application`** → `core-run-dev` (observational/stream, `angle: environment`).
  `run` = the resolved app command; `signal_matches` carries a `key: ready` matcher
  for the framework's readiness line; `depends_on` = the node-name edges translated to
  `core-*` sensor ids; declare every key the app reads under `env:`.
- **`dependencies.<svc>`** → `core-datastore-<svc>` (observational/stream). `run` brings
  the service up via the compose file and blocks until ready; `signal_matches` uses a
  `type`-appropriate `key: ready` probe (datastore → `pg_isready`). `uses: [compose]`.
  See `schemas/examples/sensor/core-datastore-postgres.yaml`.
- **`setup[]`** → `core-<id>` (assertion/single-shot). `run` = the resolved command;
  declare required keys under `env:`; `depends_on` the datastore. See
  `schemas/examples/sensor/core-migrate.yaml`.

**Translate edges:** a model `depends_on: [postgres, migrate]` becomes the sensor's
`depends_on: [core-datastore-postgres, core-migrate]`. The resolver orders the closure;
use-case sensors then depend only on `core-run-dev` and inherit the rest transitively.

If the model is absent, fall back to the existing inference flow below.
```

- [ ] **Step 2: Verify line budget**

Run: `wc -l skills/create-core-sensors/SKILL.md`
Expected: ≤200. If over, trim redundant prose from the fallback inference section.

- [ ] **Step 3: Commit**

```bash
git add skills/create-core-sensors/SKILL.md
git commit -m "feat(#52): create-core-sensors projects the environment model to sensors"
```

---

# Phase 6 — Typed precondition signals

`inconclusiveFromServiceError` (`lib/skillruntime/services.go:298`) already produces an
`inconclusive` aggregate when a shared service fails to start / become ready. This
phase makes those failures *typed* (distinguishable observation keys) and adds the
`setup_unavailable` guard. Rollup already orders error-termination → inconclusive
before completeness (`internal/aggregate/rollup.go` Rule 0), so no rollup-verdict
change is needed — only typed evidence.

### Task 16: Type the service-error aggregate (missing/unready)

**Files:**
- Modify: `lib/skillruntime/services.go` (`inconclusiveFromServiceError` + its call sites)
- Test: `lib/skillruntime/services_test.go` (add a case)

- [ ] **Step 1: Write the failing test**

```go
// lib/skillruntime/services_test.go  (add)
package skillruntime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/sensor"
)

func TestInconclusiveFromServiceError_Typed(t *testing.T) {
	s := sensor.Sensor{ID: "e2e", Angle: enums.AngleE2ETest}
	// readiness timeout → unready-service
	got := inconclusiveFromServiceError(s, "core-run-dev", context.DeadlineExceeded)
	if got.Verdict != enums.VerdictInconclusive {
		t.Fatalf("verdict = %q", got.Verdict)
	}
	if k, _ := got.Evidence["observation_key"].(string); k != "unready-service" {
		t.Fatalf("observation_key = %v, want unready-service", got.Evidence["observation_key"])
	}
	// generic start failure → missing-service
	got2 := inconclusiveFromServiceError(s, "core-run-dev", errors.New("docker daemon not running"))
	if k, _ := got2.Evidence["observation_key"].(string); k != "missing-service" {
		t.Fatalf("observation_key = %v, want missing-service", got2.Evidence["observation_key"])
	}
	if !strings.Contains(got2.HealHint.Summary, "core-run-dev") {
		t.Fatalf("heal hint missing service id: %q", got2.HealHint.Summary)
	}
}
```

> Confirm `enums.AngleE2ETest` is the correct identifier (grep `internal/enums` for the e2e-test angle constant; adjust if named differently). Confirm `AggregateSignal.Evidence` exists; if the aggregate type has no `Evidence` map, add an `Evidence map[string]any` field to `aggregate.AggregateSignal` (mirroring `signal.Signal.Evidence`) and a corresponding `evidence` property in `schemas/aggregate-signal.yaml`, then set it here. (Verify first with: `grep -n "Evidence" internal/aggregate/types.go`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./lib/skillruntime/ -run TestInconclusiveFromServiceError_Typed -v`
Expected: FAIL — no `observation_key` / typing.

- [ ] **Step 3: Modify inconclusiveFromServiceError to set a typed key**

Update the function (`lib/skillruntime/services.go`) to classify the error and stamp `observation_key`:

```go
func inconclusiveFromServiceError(s sensor.Sensor, serviceID string, err error) aggregate.AggregateSignal {
	now := time.Now().UTC()
	key := "missing-service"
	if errors.Is(err, context.DeadlineExceeded) {
		key = "unready-service"
	}
	return aggregate.AggregateSignal{
		SchemaVersion:     "1.0.0",
		Type:              aggregate.TypeAggregate,
		SensorID:          s.ID,
		UseCaseID:         s.UseCaseID,
		Angle:             s.Angle,
		StartedAt:         now,
		EndedAt:           now,
		Verdict:           enums.VerdictInconclusive,
		Confidence:        0,
		TerminationReason: enums.TerminationError,
		Evidence:          map[string]any{"observation_key": key, "service": serviceID},
		Rollup:            aggregate.RollupCounts{InconclusiveCount: 1},
		HealHint: &aggregate.HealHint{
			Summary:   fmt.Sprintf("shared service %s could not be established (%s): %v", serviceID, key, err),
			Rationale: fmt.Sprintf("sensor %s attaches to %s; the precondition was not met, so the sensor did not run. This is an environment problem, not an application defect.", s.ID, serviceID),
		},
	}
}
```

Add `"context"` and `"errors"` to the imports if not present.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./lib/skillruntime/ -run TestInconclusiveFromServiceError_Typed -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lib/skillruntime/services.go lib/skillruntime/services_test.go
# include internal/aggregate/types.go + schemas/aggregate-signal.yaml if Evidence was added
git commit -m "feat(#52): type service precondition failures (missing/unready-service)"
```

### Task 17: setup_unavailable guard for unresolvable setup commands

**Files:**
- Modify: `internal/runtime/executor/envsignal.go` (add a typed signal constructor)
- Test: `internal/runtime/executor/envsignal_test.go` (add a case)

A `setup` sensor whose `run` command is missing/unresolvable should emit a typed
`setup-unavailable` signal (inconclusive), reusing the existing `envProblemSignal`
helper that already sets `verdict: inconclusive` + `observation_key`.

- [ ] **Step 1: Write the failing test**

```go
// internal/runtime/executor/envsignal_test.go  (add)
func TestSetupUnavailableSignal(t *testing.T) {
	s := sensor.Sensor{ID: "core-migrate", Angle: enums.AngleEnvironment}
	sig := setupUnavailableSignal(s, "db:migrate", func() time.Time { return time.Unix(0, 0).UTC() })
	if sig.Verdict != enums.VerdictInconclusive {
		t.Fatalf("verdict = %q", sig.Verdict)
	}
	if k, _ := sig.Evidence["observation_key"].(string); k != "setup-unavailable" {
		t.Fatalf("observation_key = %v", sig.Evidence["observation_key"])
	}
}
```

> Verify the environment angle constant name (`grep -n "environment" internal/enums/*.go`); adjust `enums.AngleEnvironment` to match.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime/executor/ -run TestSetupUnavailableSignal -v`
Expected: FAIL — setupUnavailableSignal not defined.

- [ ] **Step 3: Add the constructor to envsignal.go**

```go
// setupUnavailableSignal is emitted when a setup node's command cannot be
// resolved/run. Verdict inconclusive: a missing setup step is an incomplete
// environment, not an application defect.
func setupUnavailableSignal(s sensor.Sensor, ref string, now func() time.Time) signal.Signal {
	return envProblemSignal(s, "setup-unavailable",
		signal.Evidence{"setup": ref},
		"Setup step unavailable: "+ref,
		"The setup command ("+ref+") could not be resolved or executed. Provide the missing script/target — no behavioral conclusion was drawn.",
		now)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/runtime/executor/ -run TestSetupUnavailableSignal -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/executor/envsignal.go internal/runtime/executor/envsignal_test.go
git commit -m "feat(#52): setup-unavailable typed precondition signal"
```

---

# Phase 7 — Integration & dogfood

### Task 18: Offline integration fixture (Next.js + Drizzle)

**Files:**
- Create: `examples/nextjs-drizzle-sample/package.json`
- Create: `examples/nextjs-drizzle-sample/docker-compose.yml`
- Create: `examples/nextjs-drizzle-sample/.env.example`
- Create: `examples/nextjs-drizzle-sample/drizzle.config.ts`
- Test: `internal/environment/integration_test.go`

Capture the dashboard's real infra files (deterministic, offline). The test runs
`Parse` → classifies a known-good model → `Persist` and asserts grounding holds.

- [ ] **Step 1: Write the fixture infra files**

`examples/nextjs-drizzle-sample/package.json`:

```json
{
  "name": "nextjs-drizzle-sample",
  "scripts": {
    "dev": "next dev",
    "build": "next build",
    "start": "next start",
    "db:migrate": "drizzle-kit migrate",
    "db:seed-events": "tsx scripts/seed-events.ts"
  }
}
```

`examples/nextjs-drizzle-sample/docker-compose.yml`:

```yaml
services:
  postgres:
    image: postgres:16-alpine
    ports:
      - "5432:5432"
    environment:
      POSTGRES_USER: reliable
      POSTGRES_PASSWORD: reliable123
      POSTGRES_DB: reliable
volumes:
  pgdata:
```

`examples/nextjs-drizzle-sample/.env.example`:

```
DATABASE_URL=postgresql://hooky:hooky@localhost:5432/hooky
NEXTAUTH_SECRET=dev-secret
NEXTAUTH_URL=http://localhost:3000
```

`examples/nextjs-drizzle-sample/drizzle.config.ts`:

```ts
import "dotenv/config";
import { defineConfig } from "drizzle-kit";
export default defineConfig({ out: "./drizzle", schema: "./src/db/schema", dialect: "postgresql", dbCredentials: { url: process.env.DATABASE_URL! } });
```

- [ ] **Step 2: Write the integration test**

```go
// internal/environment/integration_test.go
package environment

import (
	"os"
	"path/filepath"
	"testing"
)

func fixtureDir(t *testing.T) string {
	t.Helper()
	// repo-root-relative: internal/environment -> ../../examples/...
	d, err := filepath.Abs(filepath.Join("..", "..", "examples", "nextjs-drizzle-sample"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(d); err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	return d
}

func TestIntegration_DashboardChain(t *testing.T) {
	dir := fixtureDir(t)
	facts, err := Parse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Scripts["dev"] != "next dev" || facts.ComposeServices["postgres"].Image != "postgres:16-alpine" {
		t.Fatalf("facts wrong: %+v", facts)
	}

	// The model the classifier should produce.
	model := EnvironmentModel{
		SchemaVersion: "1.0.0",
		Application:   Application{ProvidedBy: ProvidedBy{"package.json", "scripts.dev"}, DependsOn: []string{"postgres", "migrate"}},
		Dependencies:  map[string]Dependency{"postgres": {Type: DependencyDatastore, ProvidedBy: ProvidedBy{"docker-compose.yml", "services.postgres"}}},
		Setup:         []SetupNode{{ID: "migrate", Type: "setup", ProvidedBy: ProvidedBy{"package.json", "scripts.db:migrate"}, DependsOn: []string{"postgres"}}},
	}
	if err := model.Validate(); err != nil {
		t.Fatalf("model invalid: %v", err)
	}
	if err := ValidateGrounding(model, facts); err != nil {
		t.Fatalf("grounding failed: %v", err)
	}

	// Persist round-trips and reloads.
	harness := t.TempDir()
	out, _ := yamlMarshalForTest(t, model)
	factsOut, _ := yamlMarshalForTest(t, facts)
	if err := Persist(out, factsOut, harness); err != nil {
		t.Fatalf("persist: %v", err)
	}
	reloaded, err := Load(filepath.Join(harness, "environment-model.yaml"))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Application.DependsOn) != 2 {
		t.Fatalf("reloaded app deps = %v", reloaded.Application.DependsOn)
	}
}
```

- [ ] **Step 3: Add the marshal test helper**

In `internal/environment/helpers_test.go`, add:

```go
import "sigs.k8s.io/yaml"

func yamlMarshalForTest(t *testing.T, v any) ([]byte, error) {
	t.Helper()
	b, err := yaml.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b, nil
}
```

(Adjust the existing `helpers_test.go` import block to include `sigs.k8s.io/yaml` alongside `os`/`testing`.)

- [ ] **Step 4: Run the integration test**

Run: `go test ./internal/environment/ -run TestIntegration_ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add examples/nextjs-drizzle-sample/ internal/environment/integration_test.go internal/environment/helpers_test.go
git commit -m "test(#52): offline integration fixture + full detect chain"
```

### Task 19: Self-dogfood — no-op model on the harness repo

**Files:**
- Test: `internal/environment/dogfood_test.go`

Run `Parse` on the lastro repo root (CLI archetype, no compose). Assert it yields no
compose services — the no-op path. A classifier would emit just an `application` node;
the test asserts the facts that drive that.

- [ ] **Step 1: Write the test**

```go
// internal/environment/dogfood_test.go
package environment

import (
	"path/filepath"
	"testing"
)

func TestDogfood_NoComposeInHarnessRepo(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	facts, err := Parse(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.ComposeServices) != 0 {
		t.Fatalf("harness repo should have no compose services, got %v", facts.ComposeServices)
	}
	// A model with only an application node + no deps validates (no-op).
	m := EnvironmentModel{
		SchemaVersion: "1.0.0",
		Application:   Application{ProvidedBy: ProvidedBy{"Makefile", "build"}},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("no-op model invalid: %v", err)
	}
}
```

> The harness repo has a `Makefile`; `Parse` should surface a `build` (or similar) target. If `Makefile` has no simple `build:` target, change the pointer to a target the parser actually finds (run `go test -run TestDogfood -v` and read `facts.MakeTargets`).

- [ ] **Step 2: Run the test**

Run: `go test ./internal/environment/ -run TestDogfood_ -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/environment/dogfood_test.go
git commit -m "test(#52): self-dogfood no-op environment model"
```

### Task 20: Full build + test sweep

- [ ] **Step 1: Build everything**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 2: Run the whole suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: all PASS. Investigate any failure (especially enum-coverage tests touched in Task 3 and example-enumeration tests touched in Tasks 2/14).

- [ ] **Step 3: Rebuild the embedded binaries (so skills pick up the new subcommand)**

Run: `make build-all` (the target referenced by `scripts/harness-tools.sh`).
Expected: refreshes `bin/<os>-<arch>/lastro`.

- [ ] **Step 4: Commit the rebuilt binaries if the repo tracks them**

```bash
git add bin/
git commit -m "build(#52): rebuild lastro binary with detect-environment"
```

> Note: `git status` at plan start showed `bin/linux-amd64/lastro` already modified by prior work; coordinate with the user before committing binaries to avoid bundling unrelated changes.

---

## Self-Review

**Spec coverage:**
- §1 dependency model schema → Tasks 1, 2 ✔
- §2 `/detect-environment` pipeline (parser/classifier/validator) → Tasks 4–12 ✔
- §3 `/create-core-sensors` consumption (node→sensor, command resolution, edge translation, env via EnvSpec) → Tasks 14, 15 ✔
- §3 detect-stack records docker/compose → Task 13 ✔
- §4 typed precondition signals + inconclusive → Tasks 3, 16, 17 ✔
- §5 tests: unit per package (Tasks 4–10, 11, 16, 17), offline integration fixture (Task 18), self-dogfood no-op (Task 19) ✔
- #22 held out of scope: no heal-trigger task ✔

**Placeholder scan:** No "TBD"/"implement later"; the few `> Note:` callouts are verification instructions (grep to confirm an enum/field name), each with the exact command to resolve them, not deferred work.

**Type consistency:** `EnvironmentModel`/`Application`/`Dependency`/`SetupNode`/`ProvidedBy`/`RawFacts`/`ComposeService` used identically across Tasks 4–19. `Parse`, `Load`, `LoadBytes`, `Validate`, `ValidateGrounding`, `Persist`, `Resolve` signatures match between definition and call sites. `detectEnvironment` signature matches between Task 11 definition and its test. `inconclusiveFromServiceError`/`setupUnavailableSignal` match the verbatim patterns extracted from `lib/skillruntime/services.go` and `internal/runtime/executor/envsignal.go`.

**Known verification points (resolve during execution, commands given inline):** (a) `enums` angle constant names (`AngleE2ETest`, `AngleEnvironment`); (b) whether `aggregate.AggregateSignal` has an `Evidence` field — if not, Task 16 adds it; (c) whether any test enumerates `schemas/enums/*.yaml` or `schemas/examples/**` and needs the new files registered (Tasks 2, 3, 14).
