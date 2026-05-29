# http-api-sample

Minimal Go HTTP API used by the harness framework's Track 1 integration
tests as the canonical archetype-`http-api` subject.

## Endpoints

- `GET /orders/{id}` — returns the order if known, 404 otherwise.
- `POST /orders` — creates an order with body `{"item": "..."}`. Returns
  400 when `item` is missing.

## Running standalone

```
cd examples/http-api-sample
go run .
```

## What it demonstrates

- Stack detection (archetype=`http-api`) — see `.harness/stack-manifest.yaml`.
- Three use cases (get-order, create-order-success, create-order-bad-input).
- Six sensors across two angles (`e2e-test`, `unit-test`), with one
  fixture (`valid-order-payload`) reused across both angles of the
  create-order-success use case (plan §11.7).
