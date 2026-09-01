# Testing

## Run all tests

```bash
go test ./...
```

## Run only unit tests

```bash
./tests/scripts/run-unit-all.sh   # if present
```

## Layout

* `internal/**/*_test.go` — unit tests next to source
* `tests/` — cross-package integration tests
* `pow/*_test.go` — Keccak tests (PoW is unused in slim, but kept for parity)

## Style

* No fixtures outside the test file
* No external services (no network, no Docker)
* Use `internal/testsuite` for shared helpers (paths, temp dirs, etc.)
