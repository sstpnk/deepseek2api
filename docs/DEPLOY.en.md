# Deployment

DS2API is a single Go binary. It reads configuration from `config.json`,
`DS2API_CONFIG_PATH`, or `DS2API_CONFIG_JSON`.

## Local

```bash
cp config.example.json config.json
go run ./cmd/ds2api
```

The default port is `5001`; override it with `PORT`.

## Docker Compose

```bash
cp .env.example .env
cp config.example.json config.json
docker compose up --build
```

`docker-compose.yml` mounts `config.json` into the container and exposes
`${DS2API_HOST_PORT:-6011}` on the host.

## Environment Variables

| Variable | Description | Default |
| --- | --- | --- |
| `PORT` | HTTP listen port inside the app/container | `5001` |
| `DS2API_HOST_PORT` | Host port used by Docker Compose | `6011` |
| `LOG_LEVEL` | Logging level | `INFO` |
| `DS2API_CONFIG_PATH` | Config file path | `config.json` or container default |
| `DS2API_CONFIG_JSON` | Raw JSON or Base64 JSON config | empty |
| `CONFIG_JSON` | Legacy alias for `DS2API_CONFIG_JSON` | empty |
| `DS2API_ENV_WRITEBACK` | Write env config back to a file when possible | enabled |
| `DS2API_CHAT_HISTORY_PATH` | Local chat history file path | `data/chat_history.json` |
| `DS2API_RUNTIME_STATS_PATH` | Runtime stats file path | `data/runtime_stats.json` |

## Health Checks

```bash
curl http://127.0.0.1:5001/healthz
curl http://127.0.0.1:5001/readyz
```

## Release Build

```bash
go build -o ds2api ./cmd/ds2api
```
