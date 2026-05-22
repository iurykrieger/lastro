# E4 — UseCase

> Source plan: [`plan.md`](plan.md) §4.1 (UseCase schema), §4.1.2 (interpolation grammar), §2 (use cases are the problem)

A `UseCase` is the behavioral spec — `given / when / then` text plus structured references to entry points and fixtures. The text can interpolate references via `{{ }}`. This is the largest Phase A chunk because it owns: schema, template grammar, template resolver, and cross-reference validation. It does *not* own EntryPoint internals (E3) or Fixture internals (E5) — it composes them.

## Scope

In:
- The `UseCase` schema and Go type.
- Loader: deserialize a use-case YAML, embed `[]EntryPoint` (via E3) and `[]Fixture` references (via E5).
- `{{ }}` template grammar parser per §4.1.2:
  - `{{fixtures.<id>}}`
  - `{{fixtures.<id>.<jsonpath>}}`
  - `{{entry_points.<id>}}`
  - `{{entry_points.<id>.spec.<field>}}`
- Template resolver: given a UseCase, resolve any `{{ }}` reference to a concrete value (or an error if the id is unknown / jsonpath misses).
- Cross-reference validator: every `fixture_ids[]` resolves; every interpolated id exists; every `entry_points[].archetype` is in `archetype_scope`.
- Tests covering happy path, every interpolation form, every cross-ref failure case.

Out:
- EntryPoint spec parsing (E3 owns it; E4 imports the types).
- Fixture payload semantics (E5 owns; E4 only references by id).
- The `/detect-use-cases` skill (Phase B).

## Schema (from plan §4.1)

Key fields:
- `schema_version`, `id`, `title`, `archetype_scope: [<archetype>]`
- `entry_points: [EntryPoint]` (E3-shaped)
- `given: [string]`, `when: [string]`, `then: [string]` — natural language with `{{ }}` interpolation
- `source_refs: [{path, symbol, reason}]` — pointers only, no embedded code
- `fixture_ids: [<Fixture.id>]`

## Inputs / Outputs

- **Input:** a use-case YAML file.
- **Output:** `internal/usecase/` Go package — types, loader, template parser, template resolver, cross-ref validator.

## Dependencies

- E1 (enums) — for `Archetype`.
- E3 (EntryPoint) — embedded type.
- E5 (Fixture) — referenced by id, but the resolver needs Fixture access to interpolate `{{fixtures.<id>}}`.
- Schema-freeze gate.

**Coordination note for parallel work:** E4's template resolver needs to *read* Fixture payloads (E5's responsibility). The clean cut is: E5 exposes a `LookupFixture(id) (Fixture, bool)` interface; E4 depends on that interface, not on E5's concrete type. Either chunk can stub the other while developing in parallel.

## Open questions for `/brainstorming`

1. **Template engine.** Hand-roll a tiny parser (regex + jsonpath lib), or use an existing engine (Go templates, mustache)? Recommendation: hand-roll. The grammar in §4.1.2 is tiny and constrained; pulling in a template engine adds surface area we'd have to lock down anyway.
2. **JSONPath dialect.** Plan §4.1.2 says `{{fixtures.<id>.<jsonpath>}}`. Which JSONPath dialect — RFC 9535, GJson-style dot notation, something else? Recommendation: dot notation (`.user.name`) — sufficient for fixture payload drilling, no library dependency.
3. **`given/when/then` cardinality.** Plan examples show one-element arrays. Is multi-step `given:` supported (multiple preconditions)? Recommendation: yes — array of strings is the contract; one or many is fine.
4. **`source_refs` validation.** Should the loader verify the referenced file/symbol actually exists in the repo? That couples the loader to filesystem access. Recommendation: no — `source_refs` is provenance metadata, not a runtime invariant. Validation happens at detection time, not load time.
5. **Inert template rendering.** Plan §4.1.2: "Templates are inert in textual contexts (rendered as labels for humans); they are resolved to live values only when consumed by a sensor." Does this chunk own the human-label renderer too, or just the live resolver? Recommendation: own both, since they share the parse step.
6. **`id` determinism.** Plan §4.1: id is "hash of canonical given/when/then + entry_points." Define "canonical" — JSON-canonical, whitespace-normalized YAML, something else? This affects how stable ids are across cosmetic edits.

## Deliverable acceptance

- `internal/usecase/` loads the golden examples (http-api, cli, event-consumer at minimum).
- Every interpolation form resolves correctly.
- Negative tests: undefined fixture id, undefined entry point id, jsonpath miss, entry point archetype outside `archetype_scope`.
- The human-label renderer outputs something readable (e.g., `{{fixtures.fx_order_request}}` → `[fixture: fx_order_request]`) — exact format TBD in brainstorm.
