# DS2API — slim edition

[DS2API](https://github.com/CJackHwang/ds2api) is a server that converts
[DeepSeek](https://chat.deepseek.com/) web chat into an
[OpenAI-compatible](https://platform.openai.com/docs/api-reference/chat) HTTP API.

This **slim edition** ships only the OpenAI-compatible surface (`/v1/chat/completions`,
`/v1/responses`, `/v1/embeddings`, `/v1/files`, `/v1/models`) and a single
Docker image. Admin UI, Vercel/Zeabur deploys, the Cloudflare worker proxy, the
Claude/Gemini/-rp model aliases, and the Node runtime bridge are intentionally
removed.

| Endpoint                                     | Method | Status   |
| -------------------------------------------- | ------ | -------- |
| `/v1/chat/completions`                       | POST   | wired    |
| `/v1/responses`                              | POST   | wired    |
| `/v1/embeddings`                             | POST   | stub     |
| `/v1/files`                                  | GET    | stub     |
| `/v1/files`                                  | POST   | 501      |
| `/v1/models`                                 | GET    | wired    |
| `/healthz`                                   | GET    | wired    |

> **Disclaimer**
>
> This project is for personal research and learning only. It does not grant
> any commercial authorization. Use only in ways that comply with the
> applicable terms of service.

## Quick start (Docker)

```bash
cp config.example.json config.json
# edit config.json — at minimum set accounts[]
docker compose up -d
curl http://localhost:8080/healthz
```

## Configuration

Configuration is loaded from the first existing file:

1. `$DS2API_CONFIG_PATH` (env override)
2. `/app/config.json` (container default)
3. `./config.json` (current directory)

See [docs/configuration.md](docs/configuration.md) for the full schema.

### Hot reload

`SIGHUP` (or `kill -HUP <pid>`) reloads the config from disk.

Set `DS2API_HOT_RELOAD=true` to also enable a background poller that watches
the config file's mtime and self-sends `SIGHUP` on change. This is only
active when the config came from a file (not env-backed mode).

## Build from source

```bash
go build -trimpath -ldflags="-s -w" -o ds2api ./cmd/ds2api
DS2API_CONFIG_PATH=./config.json ./ds2api
```

Requires Go 1.26+.

## Documentation

- [docs/configuration.md](docs/configuration.md) — full config schema
- [docs/api.md](docs/api.md) — OpenAI-compatible endpoints
- [docs/architecture.md](docs/architecture.md) — module map
- [docs/prompt-compatibility.md](docs/prompt-compatibility.md) — prompt
  normalization flow
- [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) — build, test, lint
- [docs/TESTING.md](docs/TESTING.md) — test layout

## License

See [LICENSE](LICENSE).

## Upstream

Upstream (full edition with admin UI, Vercel/Zeabur deploys, Claude/Gemini/-rp
aliases, Node bridge, Cloudflare worker): <https://github.com/CJackHwang/ds2api>
