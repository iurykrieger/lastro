# E5 — Fixture

> Source plan: [`plan.md`](plan.md) §4.2 (Fixture schema), §2 ("Fixtures are the concrete proof"), §3.6 (FixtureRole)

A `Fixture` is a concrete I/O payload — a request body, a CLI args list, an event message, an expected response. It belongs to exactly one UseCase but can be referenced by many sensors across many angles. This chunk owns the schema, the Go type, the loader, and the `LookupFixture(id)` interface that E4's template resolver and E6's step-level `uses:` bind against.

## Scope

In:
- The `Fixture` schema and Go type.
- Loader: read a fixture YAML, validate, return typed Fixture.
- A `FixtureStore` interface: `LookupFixture(id) (Fixture, bool)` plus iteration. This is the seam E4 and E6 depend on.
- Validator: required fields, valid `role`, well-formed `binding`, non-empty `payload`.
- Tests.

Out:
- Fixture *generation* (Phase B, `/detect-use-cases`).
- The interpolation that *uses* fixtures (E4 owns it).

## Schema (from plan §4.2)

```yaml
schema_version: 1.0.0
id: <stable-id>
use_case_id: <UseCase.id>
role: input | expected-output | expected-side-effect

content_type: application/json   # or xml, text/plain, binary, etc.
payload: |
  { ... }

binding:
  channel: http                  # http, cli-args, event, stdout, log-line, db-row
  selector:                      # archetype-specific addressing
    method: POST
    path: /orders

source_refs:
  - path: src/handlers/orders.ts
    symbol: createOrder
```

## Inputs / Outputs

- **Input:** a fixture YAML file (or a directory of them — typically one fixture file per fixture).
- **Output:** `internal/fixture/` Go package — types, loader, validator, `FixtureStore` interface + in-memory implementation.

## Dependencies

- E1 (enums) — for `FixtureRole`.
- Schema-freeze gate.

**Coordination note for parallel work:** E4 (UseCase) and E6 (Sensor) both depend on `FixtureStore`. E5 defines and owns this interface so the other chunks can develop in parallel against a stub.

## Open questions for `/brainstorming`

1. **`payload` typing.** Plan shows `payload: |` (multi-line string). Should the loader keep it as a raw string (let consumers parse based on `content_type`), or parse JSON/YAML payloads eagerly? Recommendation: keep as `[]byte` plus `content_type`; provide a `PayloadAsJSON()` helper. E4's jsonpath interpolation will need parsed JSON.
2. **`binding.channel` vocabulary.** Plan lists `http, cli-args, event, stdout, log-line, db-row`. Is this list closed (enum) or open (free-form with a recommended set)? Recommendation: closed for now — start with the listed set, fold into E1 as `FixtureChannel`.
3. **`binding.selector` shape.** It's archetype-specific like EntryPoint's `spec`. Reuse the discriminated-union pattern from E3? Or keep it as `map[string]any`? Recommendation: discriminated union — predictable typing, parallel to E3.
4. **Reuse contract.** Plan §4.2 sharing rule: same fixture referenced by multiple sensors across angles. Does the framework deduplicate fixtures by content hash, or by id only? Recommendation: id only — the user (or `/detect-use-cases`) decides reuse intentionally.
5. **`payload` size ceiling.** Binary payloads are allowed (per `content_type`). Should the schema impose a size cap, or external-reference large payloads (`payload_uri: file:foo.bin`)? Recommendation: external reference for binary, inline for text — but punt to brainstorm.
6. **Validity across `role`.** An `input` fixture for an `http-api` use case probably needs `binding.channel: http` + a `selector.method/path`. Should E5 cross-check role+channel+selector consistency, or leave that to a sensor-time validator?

## Deliverable acceptance

- `internal/fixture/` loads golden examples for each role × each binding channel pair.
- `FixtureStore.LookupFixture` works and is tested.
- Negative tests: missing required fields, invalid role, invalid channel.
- A test demonstrates payload reuse: one fixture loaded once, referenced from two contexts.
