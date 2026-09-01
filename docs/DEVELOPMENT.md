# Development

## Layout

```
cmd/ds2api/         # main entrypoint
internal/           # Go packages
pow/                # C/ASM Keccak (unused in slim)
scripts/            # helper scripts
tests/              # cross-package integration tests
docs/               # documentation
Dockerfile          # multi-stage Docker build
docker-compose.yml  # one-service compose
```

## Build

```bash
go build -trimpath -ldflags="-s -w" -o ds2api ./cmd/ds2api
```

The repo has a `Dockerfile` (multi-stage) and a `docker-compose.yml` for
running the binary with `/app/config.json` mounted.

## Test

```bash
go test ./...
```

## Lint

```bash
gofmt -l .             # list files needing formatting
./scripts/lint.sh      # if present
```

## Local run

```bash
cp config.example.json config.json
# edit accounts[]
DS2API_CONFIG_PATH=./config.json go run ./cmd/ds2api
```

## Hot reload during development

```bash
DS2API_HOT_RELOAD=true DS2API_CONFIG_PATH=./config.json go run ./cmd/ds2api
# edit config.json — server reloads within ~5 seconds
```

## Project rules

See [AGENTS.md](../AGENTS.md) for change scope, gofmt policy, and PR gates.
