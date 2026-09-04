# Testing

## Required Gates

The repository gate is:

```bash
./scripts/lint.sh
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
```

`run-unit-all.sh` currently runs the Go unit suite.

## Windows PowerShell

If shell scripts are not directly executable, run the Go unit suite with a local
cache:

```powershell
$env:GOCACHE = "$(Get-Location)\.tmp\go-build-cache"
go test ./...
```

## Targeted Tests

```bash
go test ./internal/server
go test ./internal/httpapi/openai/...
go test ./internal/promptcompat
go test ./internal/config
```

## Live Smoke

Start the server:

```bash
go run ./cmd/ds2api
```

Then check:

```bash
curl http://127.0.0.1:5001/healthz
curl http://127.0.0.1:5001/v1/models
```

Authenticated request:

```bash
curl http://127.0.0.1:5001/v1/chat/completions \
  -H "Authorization: Bearer client-api-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5","messages":[{"role":"user","content":"hi"}]}'
```
