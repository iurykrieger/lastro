# E4 — UseCase (Design)

> Source chunk: [`docs/harness-framework/E4-use-case.md`](../../harness-framework/E4-use-case.md)
> Plan sections consumed: [`plan.md §4.1`](../../harness-framework/plan.md), §4.1.1, §4.1.2, §4.2
> Sequential gate consumed: [`docs/harness-framework/00-schema-freeze.md`](../../harness-framework/00-schema-freeze.md)
> Sibling chunks: [`E1`](../../harness-framework/E1-enums.md) (enums), [`E3`](../../harness-framework/E3-entry-point.md) (EntryPoint), [`E5`](../../harness-framework/E5-fixture.md) (Fixture)
> Brainstorm date: 2026-05-22

## 1. Purpose

A `UseCase` is the behavioral spec — `given / when / then` text plus
structured references to entry points and fixtures, with `{{ }}`
interpolation as the bridge between human-readable behavior and
machine-resolvable references.

E4 owns the largest Phase A surface: schema + Go type, loader, template
grammar parser, template resolver (live values), human-label renderer,
cross-reference validator, content-hash id computation. It composes
EntryPoint (E3) and Fixture (E5) by importing their types and the
`FixtureStore` interface, but owns no internals of either.

E4 does **not** own EntryPoint spec parsing, Fixture payload semantics,
the `/detect-use-cases` skill (Phase B), or the runtime executor
(Phase 3).

## 2. Scope

**In:**

- `internal/usecase/` Go package — `UseCase`, `SourceRef` types; YAML
  loader; cross-reference validator; content-hash id computation.
- `internal/usecase/template/` Go subpackage — grammar, parser, AST,
  live resolver, human-label renderer.
- Tests covering every grammar production, every error code, every
  archetype's golden example.
- A small `internal/usecase/internal/fixturestub` for tests, to avoid
  blocking on E5's concrete `FixtureStore` implementation.

**Out:**

- EntryPoint spec parsing (E3 owns).
- Fixture payload generation and `FixtureStore` implementation (E5 owns;
  E4 imports the interface).
- `source_refs` filesystem verification — kept as pure provenance
  metadata, never resolved against the repo.
- `/detect-use-cases` skill (Phase B).
- Runtime sensor execution (Phase 3 — `internal/runtime/`).

## 3. Decisions locked in brainstorming

| # | Decision | Rationale |
|---|---|---|
| D1 | Hand-roll the template parser | Grammar is 4 forms; pulling in a template engine adds surface area that would still need locking down. |
| D2 | Dot-notation jsonpath only, no brackets/wildcards/filters | Sufficient for fixture-payload drilling; trivially walked on a JSON-decoded `map[string]any`. |
| D3 | `given`/`when`/`then` are arrays of strings, one-or-many | Matches plan §4.1; multi-step preconditions are supported. |
| D4 | `source_refs` is pure provenance | Loader does not touch the filesystem; detection time validates code shape, not load time. |
| D5 | E4 owns both the live resolver and the human-label renderer | Both consume the same parsed AST; sharing the parse is the point. |
| D6 | `UseCase.id` is an **author-supplied kebab-case slug** matching the frozen schema's pattern `^[a-z][a-z0-9-]*$` (max 128 chars). Loader validates charset + uniqueness. | The schema-freeze gate locked this contract; the entire harness toolchain reads slug-form ids. Content-hashing was considered (JCS+SHA-256+prefix) but conflicts with the gate. The hashing pattern remains available for entities where author-supplied ids are inappropriate (e.g., Sensor, which may be regenerated and benefit from content-addressing) — but not for UseCase. |
| D7 | `fixture_ids[]` strictly mirrors template refs (both directions) | The declared dependency surface always equals what the text uses; drift caught at load time. |

**Implicit decisions** (follow from the above):

- IDs for entry points, fixtures, and template refs share charset
  `^[a-z][a-z0-9-]*$` (max 128 chars) — kebab-case, dot-free, so the
  parser can split `<id>` from `.<jsonpath>` on a dot boundary.
- `{{entry_points.<id>.<field>}}` accepts only `spec.<key>`; nothing
  else, per the plan §4.1.2 grammar table.
- No escape syntax for literal `{{` — use-case text describing behavior
  has no legitimate reason to need it; revisit only if it surfaces.
- Templates inside `given`/`when`/`then` are kept as **raw text** when
  computing the content hash, so id stability doesn't couple to
  fixture-payload contents.

## 4. Package layout

```
internal/usecase/
├── usecase.go         # UseCase + SourceRef types; YAML tags only
├── loader.go          # YAML bytes → UseCase, runs validate() before returning
├── id.go              # id charset & length validation (slug-form per gate)
├── validate.go        # cross-reference invariants
├── template/
│   ├── grammar.go     # token kinds, charset rules, grammar productions
│   ├── parser.go      # bytes → []Segment (Literal | FixtureRef | EntryPointRef)
│   ├── resolver.go    # Segment + FixtureStore + entry_points → resolved string / live value
│   └── label.go       # Segment → human label
├── internal/
│   └── fixturestub/   # test-only in-memory FixtureStore
└── *_test.go          # sibling tests for each file
```

Two seams that allow parallel work with E3 and E5:

1. `fixture.FixtureStore` is owned by E5. E4 imports the interface only.
   Resolver and validator take `FixtureStore` as a parameter; tests stub
   via `fixturestub`.
2. `entrypoint.EntryPoint` and its polymorphic loader are owned by E3.
   `UseCase.EntryPoints []entrypoint.EntryPoint`. E4's loader delegates
   per-entry parsing to E3.

## 5. Go types

```go
// usecase.go

type UseCase struct {
    SchemaVersion  string                  `yaml:"schema_version"`
    ID             string                  `yaml:"id"`              // required; author-supplied slug, validated
    Title          string                  `yaml:"title"`
    ArchetypeScope []enums.Archetype       `yaml:"archetype_scope"`
    EntryPoints    []entrypoint.EntryPoint `yaml:"entry_points"`
    Given          []string                `yaml:"given"`
    When           []string                `yaml:"when"`
    Then           []string                `yaml:"then"`
    SourceRefs     []SourceRef             `yaml:"source_refs"`
    FixtureIDs     []string                `yaml:"fixture_ids"`

    // parsed template segments, populated by the loader; not serialized.
    givenSegs [][]template.Segment
    whenSegs  [][]template.Segment
    thenSegs  [][]template.Segment
}

type SourceRef struct {
    Path   string `yaml:"path"`
    Symbol string `yaml:"symbol"`
    Reason string `yaml:"reason"`
}
```

The unexported `*Segs` fields cache the parsed AST per behavior line.
The resolver and label renderer walk these directly — parsing happens
exactly once per load.

## 6. ID validation

`UseCase.id` is author-supplied and required (the frozen schema enforces
presence and pattern). The loader validates only:

1. **Charset** — matches `^[a-z][a-z0-9-]*$`.
2. **Length** — between 1 and 128 chars inclusive.
3. **Uniqueness** — within a load context, no other UseCase loaded by
   the same caller has the same id. (Enforced at a higher level if a
   `Registry` is used; the single-document `Load` cannot verify
   cross-file uniqueness.)

These are codified by `USECASE_ID_CHARSET` and `USECASE_ID_TOO_LONG`
(see §10.1).

**Content-hashing deferred.** A reusable pattern — hashable view + JCS
(RFC 8785) + SHA-256 + entity prefix — was considered for UseCase id
during brainstorming. It conflicts with the schema-freeze gate's
slug-form contract and is not implemented here. The same pattern
remains a valid option for Sensor (Phase 2, where generation produces
content-addressable artifacts) and is mentioned in §13 as a follow-up
to revisit at that time.

## 7. Template grammar

**Tokens inside `{{ }}`:**

```
NAMESPACE := "fixtures" | "entry_points"
ID        := [a-z][a-z0-9-]{0,127}       // matches the frozen schema's id pattern
FIELD     := "spec"                      // only legal under entry_points
JSONKEY   := [a-zA-Z_][a-zA-Z0-9_-]*     // hyphens allowed; the parser is already past the id here
DOT       := "."
```

**Productions:**

```
template       := "{{" ws? ref ws? "}}"
ref            := fixtureRef | entryPointRef
fixtureRef     := "fixtures" "." ID ( "." JSONKEY )*
entryPointRef  := "entry_points" "." ID ( "." "spec" "." JSONKEY )?
```

**AST:**

```go
type Segment interface{ isSegment() }

type Literal struct { Text string }

type FixtureRef struct {
    ID       string
    JSONPath []string   // empty == whole payload
    Pos      Position
}

type EntryPointRef struct {
    ID      string
    SpecKey string      // empty == whole entry point
    Pos     Position
}

type Position struct {
    Line   int
    Col    int
    Offset int
}
```

**Parser surface:**

```go
func Parse(text string) ([]Segment, error)
```

`Parse` returns a slice that, when its `Literal` and ref-label
representations are concatenated, reproduces the original input minus
the `{{ }}` brackets.

**Hard rules:**

- No nesting. `{{` inside `{{...}}` is `USECASE_TEMPLATE_PARSE`.
- No escape syntax.
- An unclosed `{{` is an error pointing at the open token.
- Unknown namespace → `USECASE_TEMPLATE_PARSE`.
- `entry_points.<id>` accepts bare (whole entry point) or
  `.spec.<field>` only; anything else is rejected at parse time
  (`USECASE_TEMPLATE_BAD_SPEC_FIELD`).
- Position is carried on every ref for downstream error messages.

## 8. Resolver and label renderer

Both consume the same `[]Segment`.

```go
// template/resolver.go

type Resolver struct {
    Fixtures    fixture.FixtureStore
    EntryPoints map[string]entrypoint.EntryPoint    // built from UseCase at load time
}

func (r *Resolver) Resolve(segs []Segment) (string, error)
func (r *Resolver) ResolveValue(seg Segment) (any, error)
```

Resolution rules:

| Segment | Result |
|---|---|
| `Literal{Text}` | passes through verbatim |
| `FixtureRef{ID, JSONPath: []}` | fixture payload rendered as string via E5's `PayloadAsJSON()` |
| `FixtureRef{ID, JSONPath: [k1, k2, ...]}` | parse payload as JSON, walk keys, return leaf |
| `EntryPointRef{ID, SpecKey: ""}` | `"<archetype>:<id>"` (e.g. `http-api:create_order_endpoint`) |
| `EntryPointRef{ID, SpecKey: "method"}` | typed spec field via E3 accessor |

Resolver does no I/O.

**Resolver-time errors** are runtime concerns, not validator concerns:

- JSONPath miss (key absent, payload not JSON, path crosses a scalar).
- Unknown spec key for the archetype.

These are returned with the ref's `Position` but are not in §10's load-
time error catalogue, because they depend on fixture payload shape that
can change after the use case is validated.

**Label renderer** — format locked here so CLI, reports, and skill
output are consistent:

```go
func RenderLabels(segs []Segment) string
```

| Ref | Label |
|---|---|
| `{{fixtures.fx_order}}` | `[fixture: fx_order]` |
| `{{fixtures.fx_order.user.name}}` | `[fixture: fx_order.user.name]` |
| `{{entry_points.ep_create}}` | `[entry: ep_create]` |
| `{{entry_points.ep_create.spec.method}}` | `[entry: ep_create.spec.method]` |

## 9. Loader pipeline

```
YAML bytes
  ↓ yaml.Unmarshal into UseCase
  ↓ parse every given/when/then line into []Segment (cached on UseCase)
  ↓ validate(uc, FixtureStore)         (§10)
  → return *UseCase, nil
```

`validate` runs every check and returns `errors.Join(...)` of every
`*ValidationError` produced. The loader never returns a partially
constructed `UseCase`.

```go
func Load(data []byte, store fixture.FixtureStore) (*UseCase, error)
```

## 10. Cross-reference validator

Order of checks (later checks may assume earlier ones held):

1. **Schema version** in supported range.
2. **Required fields** present — `title`, `archetype_scope` non-empty,
   ≥1 in each of `given`/`when`/`then`, ≥1 `entry_points[]`, every
   entry has non-empty `id`.
3. **Charset & length** — `UseCase.id`, every entry_point id, and
   every fixture id matches `^[a-z][a-z0-9-]*$` with length 1-128.
   Mismatch → `USECASE_ID_CHARSET`; over-length → `USECASE_ID_TOO_LONG`.
   (Source_ref symbols are language-native identifiers from the user's
   code and are not constrained.)
4. **Uniqueness** — entry_point ids unique within use case;
   `fixture_ids[]` has no duplicates.
5. **Archetype scope** — every `entry_points[i].archetype ∈
   archetype_scope`.
6. **Template parse** — every line in `given`/`when`/`then` parses;
   parse errors surface with line + column.
7. **Template ref structural resolution:**
   - Every `FixtureRef.ID` is in `fixture_ids[]`
     → `USECASE_FIXTURE_USED_UNDECLARED`.
   - Every `EntryPointRef.ID` is one of the use case's
     `entry_points[].id`
     → `USECASE_TEMPLATE_UNKNOWN_ENTRY_POINT`.
8. **Fixture store resolution** — every id in `fixture_ids[]` exists in
   `FixtureStore` → `USECASE_FIXTURE_NOT_IN_STORE`.
9. **Declared-fixture-unused** — every entry in `fixture_ids[]` is
   referenced by at least one template → `USECASE_FIXTURE_DECLARED_UNUSED`.
   (No symmetric check for entry_points; they may be declared without
   being interpolated, since sensors can bind to them by id.)

### 10.1 Error catalogue

Codes are stable contract for tooling and skill prompts:

```
USECASE_SCHEMA_VERSION_UNSUPPORTED
USECASE_REQUIRED_FIELD_MISSING
USECASE_ID_CHARSET
USECASE_ID_TOO_LONG
USECASE_DUPLICATE_ID
USECASE_ARCHETYPE_OUT_OF_SCOPE
USECASE_TEMPLATE_PARSE
USECASE_TEMPLATE_BAD_SPEC_FIELD
USECASE_TEMPLATE_UNKNOWN_ENTRY_POINT
USECASE_FIXTURE_USED_UNDECLARED
USECASE_FIXTURE_NOT_IN_STORE
USECASE_FIXTURE_DECLARED_UNUSED
```

```go
type ValidationError struct {
    Code     string   // stable, machine-readable
    Message  string   // human-readable
    Location Position // line+col within YAML where possible
    Refs     []string // related ids (e.g., the unreferenced fixture_id)
}
```

## 11. Testing strategy

Tests live alongside each file as `*_test.go` siblings. The acceptance
bar from the E4 doc — *every interpolation form, every cross-ref
failure case* — drives the suite shape.

| Layer | File | Coverage |
|---|---|---|
| Unit | `template/parser_test.go` | All 4 grammar forms accepted; bad inputs (nested, unclosed, unknown namespace, malformed spec access, bad ids) rejected with correct position. |
| Unit | `template/resolver_test.go` | Every interpolation form; jsonpath misses; one spec-field case per archetype; unknown spec field is an error. |
| Unit | `template/label_test.go` | Every grammar form renders to the locked label format. |
| Unit | `id_test.go` | Charset & length validation: accepts valid kebab-case slugs of varying lengths; rejects uppercase, leading digit, underscores, dots, hyphenless boundary cases, and lengths 0 / 129+. |
| Unit | `validate_test.go` | One targeted negative per error code; one happy-path per archetype. |
| Integration | `loader_test.go` | Golden examples for http-api, cli, event-consumer at minimum. For each: refs resolve, labels match snapshots, recomputed id matches file. |
| Property | `template/walk_test.go` | For a corpus of valid use cases, both walkers (resolver with stub, label renderer) produce output for every segment without panic. |

**Test stub for `FixtureStore`:** an in-memory map-backed implementation
in `internal/usecase/internal/fixturestub`. Not exported beyond
`usecase/`. Lets E4 tests run independently of E5's completion.

## 12. Deliverable acceptance

Matches the E4 doc, restated:

- `internal/usecase/` loads the three golden examples (`http-api`,
  `cli`, `event-consumer`) without error.
- Every interpolation form has a passing resolver test.
- Every error code in §10.1 has a targeted negative test.
- The human-label renderer's output matches checked-in snapshots.
- ID validation accepts the schema-conformant slugs in the golden
  examples and rejects malformed ids (per §6).
- No package in `internal/usecase/` lands without a sibling `_test.go`.

## 13. Out-of-scope follow-ups

Items surfaced during brainstorming but deferred:

- **Escape syntax for literal `{{`** — punt until a real use case
  demands it. If demand appears, the natural extension is `{{` doubled
  or a backslash escape.
- **Stricter jsonpath** (array indexing, wildcards) — revisit if
  fixture payloads with arrays start needing addressing beyond
  whole-array references.
- **Content-hashing for Sensor ids** — the hashable-view + JCS +
  SHA-256 + entity-prefix pattern was designed in E4's brainstorm but
  rejected for UseCase (slug-form per gate). Revisit when E6 (Sensor)
  begins design: regenerated sensors benefit from content-addressing.
- **Resolver-time error surfacing in `/validate-use-case`** — runtime
  ergonomics for jsonpath misses (e.g., re-pointing the LLM at the
  miss location) is a Phase 3 concern, not E4.
