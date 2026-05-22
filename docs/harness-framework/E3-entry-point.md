# E3 — EntryPoint

> Source plan: [`plan.md`](plan.md) §4.1.1 (Archetype-specific spec shapes), §2.1 (entry_points typed by archetype)

An `EntryPoint` is an archetype-typed observable surface — an HTTP route, a queue subscription, a CLI command, an exported SDK symbol, etc. It is *embedded inside* a UseCase (a UseCase has N entry points), but the schema work is non-trivial enough to deserve its own chunk: the `spec` field is a discriminated union with a different shape for each of the 9 archetypes.

## Scope

In:
- The `EntryPoint` schema with discriminated union over 9 archetype-specific `spec` shapes.
- A Go type for each archetype's spec (`HTTPEntryPointSpec`, `EventConsumerSpec`, `CLISpec`, etc.).
- A polymorphic loader: read `archetype`, dispatch to the right spec parser, return a typed `EntryPoint`.
- Validators per archetype (e.g., `http-api` requires non-empty `method` + `path`).
- Tests: every archetype's happy path + targeted negative cases.

Out:
- The UseCase that owns these entry points (that's E4).
- The `{{entry_points.<id>}}` interpolation grammar (also E4 — interpolation operates on a fully loaded UseCase).

## Archetype-specific spec shapes (from plan §4.1.1)

| Archetype | Required spec fields |
|---|---|
| `http-api` | `method`, `path` |
| `event-consumer` | `channel_kind` (queue\|topic), `channel_name` |
| `event-producer` | `target_channel_kind`, `target_channel_name` |
| `cli` | `command` |
| `sdk` / `library` | `exported_symbol` |
| `worker` | `trigger_kind` (cron\|signal), `schedule_or_signal` |
| `batch-job` | `input_source`, `output_destination` |
| `static-site` | `route_path` |

## Inputs / Outputs

- **Input:** an `entry_points:` array inside a use-case YAML (or a standalone test fixture).
- **Output:** `internal/entrypoint/` Go package — typed structs, polymorphic loader, validators.

## Dependencies

- E1 (enums): `Archetype` constants drive the discriminated union dispatch.
- Schema-freeze gate.

## Open questions for `/brainstorming`

1. **Discriminated union encoding.** Go has no native sum types. Options: (a) interface + type assertions, (b) one big struct with optional fields per archetype, (c) `map[string]any` + per-archetype getters. Recommendation: interface + per-archetype struct, with the loader picking the right concrete type from `archetype`.
2. **`http-api` `path` syntax.** Is `path` a literal string, a template (`/orders/:id`), or a regex? Plan example shows `/orders` (literal). Recommendation: framework-style path template with `:param` placeholders, no regex.
3. **`channel_kind` for event-* archetypes.** Plan limits to `queue | topic`. Are there ecosystems we miss (e.g., Kafka partitions, NATS subjects with wildcards)? Recommendation: keep `queue | topic` enum-closed for now; add a free-form `channel_attributes` map if needed.
4. **`worker` `trigger_kind`.** Plan: `cron | signal`. What about HTTP-triggered workers (webhook-driven)? Or event-triggered workers (already covered by `event-consumer`)? Recommendation: keep `cron | signal`; HTTP workers are `http-api`, event workers are `event-consumer`.
5. **Cross-archetype invariants.** A UseCase with `archetype_scope: [http-api]` cannot declare a `cli` entry point. Where does this invariant live — in E3 (loader rejects mismatch) or E4 (UseCase validator checks scope)? Recommendation: E4 owns the cross-check, since it has the full UseCase context.

## Deliverable acceptance

- `internal/entrypoint/` loads every archetype's spec from golden examples.
- Negative test rejects missing required fields per archetype.
- A test fixture with one entry point per archetype loads cleanly and produces the right Go concrete type.
- A test for the polymorphic loader: an unknown `archetype` value is a load-time error.
