# DS2API

Language: [中文](README.MD) | [English](README.en.md)

DS2API is a small Go server that exposes DeepSeek Web access through an
OpenAI-compatible HTTP API. This fork intentionally keeps only the surface
needed by OpenAI SDK-compatible clients.

Removed from this fork: Claude-compatible routes, Gemini-compatible routes,
Admin API, WebUI, Vercel bridge, Node stream helpers, and raw stream replay
tools.

## Routes

Canonical OpenAI-compatible routes:

- `GET /v1/models`
- `GET /v1/models/{model_id}`
- `POST /v1/chat/completions`
- `POST /v1/responses`
- `GET /v1/responses/{response_id}`
- `POST /v1/files`
- `GET /v1/files/{file_id}`
- `POST /v1/embeddings`

Root shortcuts are also registered for clients configured with the bare service
URL: `/models`, `/chat/completions`, `/responses`, `/files`, and `/embeddings`.

## Configuration

Copy the example config and edit it:

```bash
cp config.example.json config.json
```

Minimal shape:

```json
{
  "keys": ["client-api-key"],
  "accounts": [
    {
      "email": "you@example.com",
      "password": "your-deepseek-password"
    }
  ]
}
```

Auth behavior:

- `Authorization: Bearer <token>` and `x-api-key: <token>` are accepted.
- If `<token>` is listed in `keys`, DS2API uses managed account rotation.
- If `<token>` is not listed in `keys`, DS2API treats it as a direct DeepSeek token.

## Run Locally

```bash
go run ./cmd/ds2api
```

The server listens on `PORT`, defaulting to `5001`.

Basic check:

```bash
curl http://127.0.0.1:5001/v1/models
```

## Docker

```bash
docker compose up --build
```

The compose file reads `.env`, mounts `config.json`, and exposes host port
`${DS2API_HOST_PORT:-6011}`.

## Tests

```bash
./scripts/lint.sh
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
```

On Windows PowerShell without a Unix shell, the practical unit-test equivalent is:

```powershell
$env:GOCACHE = "$(Get-Location)\.tmp\go-build-cache"
go test ./...
```

## Docs

- [API reference](API.en.md)
- [Architecture](docs/ARCHITECTURE.en.md)
- [Deployment](docs/DEPLOY.en.md)
- [Testing](docs/TESTING.md)
- [Prompt compatibility](docs/prompt-compatibility.md)
