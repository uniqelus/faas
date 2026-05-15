# Integration tests

## Godog layout

- `tests/integration/features/api-gateway/` and `tests/integration/features/control-plane/` keep service-owned feature files.
- Every feature file has exactly one service tag at `Feature` level:
  - `@api-gateway` or `@control-plane`.
- Reusable step modules live in `tests/integration/steps/`:
  - `http_steps.go`, `grpc_steps.go`, `sql_steps.go`, `process_steps.go`.
- Test runtime configs are treated as test data:
  - `tests/integration/features/api-gateway/testdata/api-gateway.test.yaml`
  - `tests/integration/features/control-plane/testdata/control-plane.test.yaml`

## Run

- Unit/component tests (default): `make test`
- Integration suite: `INTEGRATION=1 make test`
- Integration subset by tags:
  - `INTEGRATION=1 GODOG_TAGS=@api-gateway make test`
  - `INTEGRATION=1 GODOG_TAGS=@control-plane make test`

## Logs

- Suite-level logs: `tests/integration/logs/` (created on test run)
- Service logs are rewritten on each run (no append mode)
- Service-local optional logs: `tests/integration/features/<service>/logs/`
