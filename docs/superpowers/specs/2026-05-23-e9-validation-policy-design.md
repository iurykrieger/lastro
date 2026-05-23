# E9 — ValidationPolicy (Design)

> Source chunk: [`docs/harness-framework/E9-validation-policy.md`](../../harness-framework/E9-validation-policy.md)
> Sequential gate consumed: [`docs/harness-framework/00-schema-freeze.md`](../../harness-framework/00-schema-freeze.md)
> Depends on: [`docs/superpowers/specs/2026-05-22-e1-enums-design.md`](2026-05-22-e1-enums-design.md)
> Brainstorm date: 2026-05-23

## 1. Purpose

A `ValidationPolicy` declares, per archetype, which validation angles are
**obligatory**, **optional**, or **disabled**. Two policies compose to
produce the policy a repo actually runs under: a shared upstream `global`
default and a per-repo `local` override. The merged result — the
**effective policy** — is what sensor generation reads to decide which
sensors to synthesize and what runtime gating compares verdicts against.

E9 owns the Go types, loader, validator, deterministic resolution function,
lookup API, and serializer for the effective view. It is a pure library:
no filesystem awareness, no environment lookups, no network. The caller
(Phase B bootstrap) is responsible for locating policy files and handing
them to E9.

## 2. Scope

**In:**

- Go package `internal/policy/`:
  - Source type `ValidationPolicy` (mirrors the YAML).
  - Resolved type `EffectivePolicy` (per-angle map representation).
  - `Load(io.Reader) (*ValidationPolicy, error)` — strict YAML loader and validator.
  - `Resolve(global, local *ValidationPolicy) *EffectivePolicy` — pure merge.
  - `(*EffectivePolicy).AnglesFor(Archetype) (obligatory, optional []ValidationAngle)`.
  - `(*EffectivePolicy).Status(Archetype, ValidationAngle) AngleStatus`.
  - `(*EffectivePolicy).MarshalYAML() ([]byte, error)` — audit dump.
- Sibling `*_test.go` files with golden fixtures under
  `internal/policy/testdata/`.
- A required amendment to `schemas/validation-policy.yaml` to match the
  two-scope model (see §11).

**Out:**

- Where policies physically live. Filesystem paths, env vars, registry
  lookups, defaults — Phase B / CLI bootstrap concerns.
- Resolving any `inherits_from`-style identifier. The two-scope model
  drops that field; multi-level upstream composition is delegated to
  callers (they can pre-merge a chain into a single `global` before
  calling `Resolve`).
- Runtime gating logic that turns the effective policy into a use-case
  verdict. Phase B aggregator owns this.
- Inferential confidence floor (plan §10.3). A separate config concern.
- Parsing an effective dump back into an `EffectivePolicy`. The serialized
  form is a human/audit artifact, not a re-ingestable source.

## 3. Decisions

Four design questions were resolved during brainstorming on 2026-05-23.
A fifth — scope cardinality — surfaced during the design walkthrough
and is recorded here too.

### 3.1 Two scopes, not three (new)

The frozen schema lists `scope: [org, global, repo]`. The design collapses
`org` and `global` into a single `global` scope: from E9's vantage point
they play the same role (a shared upstream policy that the local repo
overrides). Multi-level upstream chains, if needed, are composed by the
caller before calling `Resolve`.

- **Final scopes:** `global`, `local`.
- **Implication:** schema amendment required. See §11.

### 3.2 Resolution granularity: per-angle override

When local overrides global for an archetype, the override applies
**per `(archetype, angle)` pair**, not per archetype block. Local can flip
a single angle's status without restating the block; angles local doesn't
mention inherit from global.

- **Mental model:** for each `(archetype, angle)` pair in E1's
  `applicable_angles` matrix, the most-specific source that mentions the
  angle wins (local > global). Sources that don't mention the angle leave
  it unchanged.
- **Side effect:** also answers "disabled vs obligatory priority." If
  global disables an angle and local lists it as obligatory, local wins
  because it is the more specific scope. No special-cased "disabled
  beats everything" rule is needed.

### 3.3 Cross-validation with E1's applicable-angles matrix: hard reject at load

If a policy mentions an `(archetype, angle)` pair that is not in
[E1](2026-05-22-e1-enums-design.md)'s `applicable_angles` matrix
(e.g., `library.obligatory_angles: [e2e-test]`), the **loader rejects**
the file with a clear error. No warnings, no silent drops.

- **Why:** uniform, predictable. Policies stay in sync with the framework
  by construction. When E1 narrows a matrix in a framework version bump,
  existing policies will fail-fast at load and authors must update them
  explicitly.

### 3.4 `inherits_from` resolution: not E9's concern

The two-scope decision (§3.1) drops the `inherits_from` field from the
source schema entirely. If a caller needs to maintain a chain of
upstream policies (org → division → framework defaults), they merge
that chain into a single `*ValidationPolicy` before calling `Resolve`.
E9 does not load, locate, or otherwise interpret upstream identifiers.

### 3.5 Effective-policy serialization: yes, with a distinct marker

`(*EffectivePolicy).MarshalYAML()` emits an audit-friendly YAML view:

- No `scope:` field. The presence of `resolved_from: [...]` is the marker
  that distinguishes a resolved view from a source policy.
- `per_archetype` is rebuilt as the three-list form (obligatory / optional
  / disabled) per archetype.
- Output is deterministic: archetypes sorted by name, angles sorted by
  name within each list, lists empty rather than omitted.
- Effective dumps are **not** re-ingestable through `Load`. The loader's
  strict-additional-properties rule (see §5, rule 8) rejects
  `resolved_from`. This is intentional: source policies are
  human-authored, effective dumps are derived artifacts.

## 4. Types

```go
type Scope string
const (
    ScopeGlobal Scope = "global"
    ScopeLocal  Scope = "local"
)

// ValidationPolicy mirrors the on-disk YAML form. Human-authored.
type ValidationPolicy struct {
    SchemaVersion string
    Scope         Scope                          // global | local
    PerArchetype  map[Archetype]ArchetypeBlock
}

type ArchetypeBlock struct {
    Obligatory []ValidationAngle
    Optional   []ValidationAngle
    Disabled   []ValidationAngle
}

// EffectivePolicy is the resolved view. Derived; not human-authored.
type EffectivePolicy struct {
    SchemaVersion string
    ResolvedFrom  []string                       // e.g. ["global", "local"]
    PerArchetype  map[Archetype]map[ValidationAngle]AngleStatus
}

type AngleStatus string
const (
    StatusObligatory AngleStatus = "obligatory"
    StatusOptional   AngleStatus = "optional"
    StatusDisabled   AngleStatus = "disabled"
)
// Zero value AngleStatus("") represents "unset / no opinion".
// Absence from EffectivePolicy.PerArchetype[archetype] also means unset.
```

Notes:

- `EffectivePolicy` has no `Scope` field — it is not a scope; it is the
  result of merging scopes.
- `Archetype` and `ValidationAngle` come from
  [`internal/enums`](2026-05-22-e1-enums-design.md). E9 imports them; it
  does not redefine them.

## 5. Loader

`func Load(r io.Reader) (*ValidationPolicy, error)`

The loader performs a single strict YAML parse and then validates. All
violations are collected and returned together via a joined error, so a
CI run surfaces the full list of problems rather than one-at-a-time.

Validation rules (all hard rejects):

1. `schema_version` is present and matches a supported version.
2. `scope` is exactly `"global"` or `"local"`. Any other value (including
   the legacy `"org"`, `"repo"`, `"effective"`) is rejected.
3. Every key in `per_archetype` is a valid `Archetype` per E1.
4. Every entry of every list (`obligatory_angles`, `optional_angles`,
   `disabled_angles`) is a valid `ValidationAngle` per E1.
5. Every `(archetype, angle)` pair appears in E1's `ApplicableAngles`
   matrix (see §3.3).
6. Within a single archetype block, the three lists are pairwise disjoint:
   `obligatory ∩ optional = ∅`, `obligatory ∩ disabled = ∅`,
   `optional ∩ disabled = ∅`.
7. No duplicate angles within any single list.
8. Strict YAML parsing: unknown top-level fields, unknown fields inside an
   archetype block, and unknown archetype keys all reject. Typos like
   `obligatorY_angles:` are not silently ignored.

The loader does **not** consult the filesystem or environment. It
operates on the bytes it is given.

## 6. Resolution algorithm

`func Resolve(global, local *ValidationPolicy) *EffectivePolicy`

Pure, deterministic, no I/O. Pseudocode:

```
sources := []
if global != nil: sources.append(("global", global))
if local  != nil: sources.append(("local",  local))

effective := EffectivePolicy{
    SchemaVersion: SupportedSchemaVersion,         // see §12
    ResolvedFrom:  [name for name, _ in sources],
    PerArchetype:  {},
}

archetypeKeys := union of PerArchetype keys across sources
for archetype in archetypeKeys:
    for angle in E1.ApplicableAngles[archetype]:        // matrix-driven
        status := ""                                    // unset sentinel
        for _, src in sources:                          // in order, later overrides
            block, ok := src.PerArchetype[archetype]
            if !ok: continue                            // src silent on this archetype
            switch {
              case angle in block.Obligatory: status = StatusObligatory
              case angle in block.Optional:   status = StatusOptional
              case angle in block.Disabled:   status = StatusDisabled
              // angle absent from this src: leave status unchanged
            }
        if status != "":
            effective.PerArchetype[archetype][angle] = status

return effective
```

Properties this enforces:

- **Local absent → effective is global-resolved.** Pass `local = nil`.
- **Global absent → effective is local-resolved.** Pass `global = nil`.
- **Both absent → empty effective.** `ResolvedFrom: []`, no archetype
  entries.
- **Per-angle override.** Local can flip a single angle's status without
  restating the archetype block.
- **Local mentions archetype global does not.** Union of keys handles it.
- **Global disables, local silent.** Effective stays disabled.
- **Global disables, local obligates.** Effective is obligatory (most
  specific wins).

The inner loop iterates **only the angles E1 declares applicable** to the
archetype. Because the loader has already rejected any inapplicable
`(archetype, angle)` pair, the resolver can trust the matrix without
re-checking.

## 7. Lookup API

```go
// AnglesFor returns the two lists sensor generation cares about.
// Disabled and unset angles are both excluded — sensor generation
// treats them identically (no sensor generated). Slices are sorted by
// angle string for deterministic output. Returns empty slices, never nil.
func (p *EffectivePolicy) AnglesFor(a Archetype) (obligatory, optional []ValidationAngle)

// Status is the escape hatch for reports and audit. Returns one of
// "obligatory", "optional", "disabled", or "" (unset / no opinion).
// Distinguishes "disabled by policy" from "no policy coverage".
func (p *EffectivePolicy) Status(a Archetype, angle ValidationAngle) AngleStatus
```

`AnglesFor` matches the signature called out in the E9 source chunk.
`Status` is added to support the audit and reporting use cases — without
it, callers cannot distinguish a deliberately disabled angle from one
that no scope mentioned.

## 8. Effective serialization

```go
func (p *EffectivePolicy) MarshalYAML() ([]byte, error)
```

YAML shape:

```yaml
schema_version: 1.0.0
resolved_from: [global, local]
per_archetype:
  cli:
    obligatory_angles: [build, contracts, security]
    optional_angles:   [logs, unit-test]
    disabled_angles:   [database, e2e-test]
  http-api:
    obligatory_angles: [build, contracts, e2e-test, security, unit-test]
    optional_angles:   [logs, metrics, performance]
    disabled_angles:   []
```

Rules:

- No `scope:` field. `resolved_from` is the discriminator.
- Archetypes are sorted alphabetically by name.
- Angles within each list are sorted alphabetically.
- Empty lists are emitted as `[]`, not omitted.
- Angles with `AngleStatus("")` (unset) do not appear in any list — they
  are simply absent from the dump.

There is no `Unmarshal` for `EffectivePolicy`. Feeding an effective dump
back through `Load` will fail at rule 8 (strict additional properties:
`resolved_from` is not a recognized top-level field, and the missing
`scope` violates the source schema).

## 9. Testing strategy

Sibling `_test.go` per Go file. Golden YAML fixtures live in
`internal/policy/testdata/`.

**Loader (`loader_test.go`):**

- Positive: load a representative `global.yaml` and a representative
  `local.yaml`; struct contents match expected.
- Negative, one per rule:
  - Missing `schema_version`.
  - Unsupported `schema_version`.
  - `scope: effective`, `scope: org`, `scope: repo`, `scope:` (empty).
  - Unknown archetype key (e.g., `frobnicator:`).
  - Unknown angle (e.g., `obligatory_angles: [not-a-real-angle]`).
  - Inapplicable angle for archetype (e.g.,
    `library.obligatory_angles: [e2e-test]`).
  - Overlapping lists (same angle in obligatory and disabled).
  - Duplicate angle within a single list.
  - Unknown top-level field (typo).
  - Unknown field inside an archetype block.
- Multi-error: a file containing three independent violations returns
  all three in the joined error.

**Resolver (`resolve_test.go`):**

- Local nil → effective equals global-resolved view.
- Global nil → effective equals local-resolved view.
- Both nil → empty effective, `ResolvedFrom: []`.
- Local mentions archetype global does not → union works, archetype
  appears in effective.
- Per-angle override: global has `http-api.optional: [performance]`,
  local has `http-api.obligatory: [performance]` → effective is
  obligatory.
- Global disables `e2e-test`, local silent on it → effective disabled.
- Global disables, local obligates → effective obligatory.

**Lookup (`lookup_test.go`):**

- `AnglesFor` returns sorted obligatory and optional sets for a
  configured archetype.
- `AnglesFor` returns empty (non-nil) slices for unconfigured archetype.
- `Status` returns each of `obligatory`, `optional`, `disabled`, `""`
  for representative inputs.

**Serializer (`serialize_test.go`):**

- Output contains `resolved_from`, omits `scope`.
- Output is byte-identical across two marshals of the same effective
  policy (determinism).
- Marshaling then attempting `Load` on the result fails (round-trip
  intentionally not supported).

**Coverage gate:** every exported function has at least one happy-path
test and at least one error/edge test. No mocks: the package has no I/O
dependencies beyond the `io.Reader` passed to `Load`.

## 10. Dependencies

- [E1 — Fixed Enums](2026-05-22-e1-enums-design.md): `Archetype`,
  `ValidationAngle`, `ApplicableAngles`, `IsValidArchetype`,
  `IsValidAngle`.
- Schema freeze (after amendment, see §11):
  [`schemas/validation-policy.yaml`](../../../schemas/validation-policy.yaml).
- A mature YAML library that supports strict parsing (rejecting unknown
  fields) — pick the standard the rest of the framework converges on.

## 11. Required schema amendment

`schemas/validation-policy.yaml` currently encodes the three-scope
model with an `inherits_from` field. The two-scope design requires
amending the frozen schema:

| Field | Current | Amended |
|---|---|---|
| `scope` enum | `[org, global, repo]` | `[global, local]` |
| `inherits_from` | optional property | removed |

Because the schema lives behind the schema-freeze gate, this is a
gate-level amendment — not an E9-only change. The amendment must land
before E9's loader is implemented. Suggested wording for the
amendment record: *"E9 design replaces the three-tier
global/org/repo model with a flat global/local model; the
`inherits_from` field is removed because upstream chain composition is
delegated to callers."*

## 12. Open / deferred questions

- **Schema-freeze amendment process.** Does this design's §11 amendment
  warrant a fresh schema-freeze entry, or can it be folded into the
  existing freeze doc with a dated revision note? Caller decision.
- **`SchemaVersion` propagation.** What value should `EffectivePolicy.SchemaVersion`
  hold when global and local declare different versions? Current
  assumption: pin to the framework's supported version constant rather
  than mirroring either source. Confirm during implementation.
- **Multi-level upstream composition.** When a caller needs more than two
  tiers (e.g., framework default → org → division → repo), the design
  assumes the caller pre-merges into a single `global`. We may eventually
  want a public helper `Compose(parents ...*ValidationPolicy) *ValidationPolicy`
  to do this without bypassing the loader's validations. Out of scope
  for the first cut.
