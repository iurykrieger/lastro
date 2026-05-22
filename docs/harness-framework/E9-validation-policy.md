# E9 — ValidationPolicy

> Source plan: [`plan.md`](plan.md) §4.7 (ValidationPolicy schema), §2 (policy as inherited/overridable), §2.1 (per-archetype obligatory/optional/disabled)

A `ValidationPolicy` declares, per archetype, which validation angles are obligatory, optional, or disabled. Policies inherit (global → org → repo) and can be overridden. This chunk owns the schema, the Go type, the **resolution function** (merge inherited + overrides into a single effective policy), and the lookup API used by sensor generation and runtime gating.

## Scope

In:
- The `ValidationPolicy` schema and Go type.
- Loader.
- **Resolution function:** `Resolve(global, org, repo *ValidationPolicy) *EffectivePolicy` — repo overrides org overrides global, with explicit `disabled_angles` honored.
- Validator: every referenced archetype is in E1; every referenced angle is in E1; obligatory/optional/disabled sets are disjoint per archetype.
- Lookup API: `policy.AnglesFor(archetype) (obligatory, optional []ValidationAngle)`.
- Tests.

Out:
- Where policies *come from* (file paths, env, defaults) — the CLI/loader bootstrap is Phase B.
- The runtime *gating* logic that uses the policy to decide use-case verdict (Phase B aggregator).
- Inferential confidence floor (plan §10.3 — config knob, design later).

## Schema (from plan §4.7)

```yaml
schema_version: 1.0.0
scope: org | global | repo
inherits_from: <policy-id>

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

## Inputs / Outputs

- **Input:** one or more policy YAML files (typically `global.yaml`, `org/<id>.yaml`, repo's `.harness/policy.yaml`).
- **Output:** `internal/policy/` Go package — types, loader, validator, `Resolve()`, lookup.

## Dependencies

- E1 (enums) — `ValidationAngle`, `Archetype`.
- Schema-freeze gate.

## Open questions for `/brainstorming`

1. **Resolution semantics.** When repo overrides org:
   - Is it a *full replacement* per archetype (repo's `obligatory_angles` replaces org's) or a *merge* (repo adds to org's)? Plan §4.7 says "repo overrides org overrides global" but doesn't define granularity. Recommendation: full replacement per archetype block, since partial merges produce surprising effective sets.
   - What if repo doesn't mention an archetype that org configures? Inherit unchanged. Confirmed by plan.
2. **`disabled_angles` priority.** If an angle is in `disabled_angles` at the org level but `obligatory_angles` at the repo level, who wins? Plan §4.7: "repo overrides org" suggests repo wins. But disabled is also explicit. Recommendation: repo's explicit listing always wins (it's a deliberate override).
3. **Cross-validation with E1's archetype→applicable_angles matrix.** An angle in a policy can be inapplicable to the archetype (e.g., `e2e-test` for `library`). Should the loader reject this? Recommendation: warn but don't reject — policy may be authored before the matrix evolves. Hard reject at the *runtime gating* layer.
4. **`inherits_from` resolution.** Is `inherits_from` a file path, a registry id, or a URL? Recommendation: an opaque id; the *caller* (Phase B bootstrap) provides a resolver. E9 takes already-loaded parents as parameters.
5. **Effective-policy serialization.** Should the resolved effective policy be serializable (so a CI step can dump "this is what's running")? Recommendation: yes — it's the audit trail. Use the same schema with `scope: effective` and an `inherits_from` chain in metadata.

## Deliverable acceptance

- `internal/policy/` loads golden examples for global/org/repo.
- `Resolve(global, org, repo)` produces the expected effective policy across:
  - Repo mentions archetype not in org
  - Repo overrides archetype org configured
  - Repo disables an angle org made obligatory
  - Repo absent → returns org-resolved
- `AnglesFor(archetype)` returns the correct sets.
- Negative tests: overlapping obligatory+disabled in same archetype, unknown archetype, unknown angle.
