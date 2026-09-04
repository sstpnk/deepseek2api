# Contributing

This fork is intentionally small. Contributions should preserve the current
OpenAI-compatible-only scope.

Before opening or updating a PR, run:

```bash
./scripts/lint.sh
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
```

Guidelines:

- Keep changes narrowly scoped.
- Run `gofmt -w` on changed Go files.
- Do not ignore cleanup errors from `Close`, `Flush`, `Sync`, or similar calls.
- Update docs with behavior changes.
- Do not reintroduce removed protocol adapters or UI/runtime bridges without an
  explicit project decision.
