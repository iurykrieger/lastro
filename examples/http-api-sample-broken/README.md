# http-api-sample-broken

Sibling of `http-api-sample` with a seeded bug used by Track 1 to
exercise the heal flow (plan §11.6).

## The bug

`CreateOrderHandler` is missing its body-validation branch — an empty
or malformed payload yields 201 instead of 400.

## The fix

`heal-fixture/editplan.json` ships a hand-supplied `EditPlan` that
restores the validation branch. The Track 1 heal test pipes this
EditPlan into `/heal` and expects all-pass on iteration 1.

`.harness/` is byte-identical to `http-api-sample/.harness/` — the use
cases describe what the API *should* do; the bug is in the
implementation.
