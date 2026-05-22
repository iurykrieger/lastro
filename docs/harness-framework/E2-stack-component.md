# E2 — StackComponent

> Source plan: [`plan.md`](plan.md) §4.3 (StackComponent schema), §2 (Stack as "the toolbox")

A `StackComponent` is one entry in the detected stack manifest — a library, runtime, framework, datastore, protocol, or tool the repo uses. Sensors are *grounded* in the toolbox: a sensor cannot declare `uses: [<id>]` for a component not present in the manifest. This chunk owns the schema, the Go type, and the loader/validator.

## Scope

In:
- The Go type for `StackComponent` and the manifest container `StackManifest` (which holds `archetype` + `[]StackComponent`).
- Loader: read `stack-manifest.yaml`, deserialize, validate.
- Validator: required fields, valid `kind`, well-formed `version`, non-empty `capabilities`.
- An accessor: `manifest.ById(id) (StackComponent, bool)` and `manifest.HasCapability(cap)`.
- Tests.

Out:
- The `/detect-stack` skill (Phase B). This chunk only deals with the data shape, not how it's produced.

## Schema (from plan §4.3)

```yaml
schema_version: 1.0.0
id: <library-or-capability-id>
kind: library | runtime | framework | datastore | protocol | tool
name: express
version: 4.18.x
capabilities: [http-routing, middleware, json-body-parsing]
detection_evidence:
  - package.json:dependencies.express
```

The manifest itself adds `archetype` at top level (plan §2.1).

## Inputs / Outputs

- **Input:** `stack-manifest.yaml` produced by `/detect-stack` (or hand-crafted for testing).
- **Output:** `internal/stack/` Go package — types, loader, validator, accessors.

## Dependencies

- E1 (enums): consumes `Archetype` and a `StackKind` enum (`library | runtime | framework | datastore | protocol | tool`).
- Schema-freeze gate.

## Open questions for `/brainstorming`

1. **`kind` as enum or open string.** Plan lists 6 kinds. Is the list closed (like other enums) or open? If closed, E1 should own it; if open, this chunk validates loosely. Recommendation: closed enum, fold into E1.
2. **`version` semantics.** Plan example shows `4.18.x` (range). Is the version field free-form string, semver-range, or strict semver? Free-form is forgiving but unparseable; semver-range is parseable but breaks for non-semver ecosystems (Go modules pseudo-versions, Python wheels).
3. **`capabilities` vocabulary.** Is the capabilities list free-form strings (detector decides), or a fixed vocabulary the framework owns? Free-form lets `/detect-stack` evolve without framework changes; fixed lets sensors safely match by capability. Recommendation: free-form initially, with a non-blocking lint that warns on unrecognized capabilities.
4. **`detection_evidence` shape.** Plan shows `package.json:dependencies.express` — a `<file>:<path>` string. Should this be a structured object (`{file, path, value}`) for richer reporting? Recommendation: structured object, with a string convenience renderer.
5. **Manifest-level invariants.** Should two components share an `id`? Probably not — the manifest should be a map keyed by id, with a load-time uniqueness check.

## Deliverable acceptance

- `internal/stack/` loads the golden example manifest from `schemas/examples/stack-manifest.yaml`.
- A negative test rejects: missing `id`, missing `kind`, invalid `kind` value, duplicate `id`.
- `manifest.ById` and `manifest.HasCapability` work and are tested.
- A round-trip test: load → serialize → load again produces identical structure.
