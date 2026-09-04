# DS2API API Reference

Language: [中文](API.md) | [English](API.en.md)

This fork exposes OpenAI-compatible routes only. Claude, Gemini, Admin, WebUI,
and Vercel bridge routes are intentionally absent.

## Basics

| Item | Value |
| --- | --- |
| Default base URL | `http://127.0.0.1:5001` |
| Content type | `application/json` |
| Auth headers | `Authorization: Bearer <token>` or `x-api-key: <token>` |
| Health | `GET /healthz`, `HEAD /healthz`, `GET /readyz`, `HEAD /readyz` |

JSON request bodies must be valid UTF-8.

## Route Index

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| `GET` | `/v1/models` | No | List supported model ids |
| `GET` | `/v1/models/{model_id}` | No | Return one model object; aliases are accepted |
| `POST` | `/v1/chat/completions` | Yes | OpenAI Chat Completions compatible endpoint |
| `POST` | `/v1/responses` | Yes | OpenAI Responses compatible endpoint |
| `GET` | `/v1/responses/{response_id}` | Yes | Fetch stored response while TTL is active |
| `POST` | `/v1/files` | Yes | Upload a file for supported multimodal requests |
| `GET` | `/v1/files/{file_id}` | Yes | Fetch uploaded file metadata |
| `POST` | `/v1/embeddings` | Yes | OpenAI Embeddings compatible endpoint |

Root aliases are registered for the same handlers: `/models`, `/models/{id}`,
`/chat/completions`, `/responses`, `/responses/{response_id}`, `/files`,
`/files/{file_id}`, and `/embeddings`.

## Authentication

DS2API accepts:

```http
Authorization: Bearer client-api-key
```

or:

```http
x-api-key: client-api-key
```

If the token is present in `config.keys`, the request uses managed account
rotation. If the token is not present in `config.keys`, it is treated as a direct
DeepSeek token.

`X-Ds2-Target-Account: <email_or_mobile>` may be used with managed account mode
to select a specific configured account.

## Models

`GET /v1/models` returns the canonical DeepSeek model ids known to this fork.
Common OpenAI-family aliases are accepted through `model_aliases`.

Useful built-in aliases include:

- `gpt-4o` -> `deepseek-v4-flash`
- `gpt-5` -> `deepseek-v4-pro`
- `o3` -> `deepseek-v4-pro`

Alias values in `config.json` override built-ins.

## Chat Completions

```bash
curl http://127.0.0.1:5001/v1/chat/completions \
  -H "Authorization: Bearer client-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": false
  }'
```

Streaming uses standard `text/event-stream` chunks and ends with `data: [DONE]`.

## Responses

```bash
curl http://127.0.0.1:5001/v1/responses \
  -H "Authorization: Bearer client-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5",
    "input": "Write one sentence about DS2API."
  }'
```

Stored response lookup is in-memory and controlled by
`responses.store_ttl_seconds`.

## Files

```bash
curl http://127.0.0.1:5001/v1/files \
  -H "Authorization: Bearer client-api-key" \
  -F purpose=user_data \
  -F file=@example.txt
```

## Embeddings

```bash
curl http://127.0.0.1:5001/v1/embeddings \
  -H "Authorization: Bearer client-api-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"text-embedding-3-small","input":"hello"}'
```

The default `config.example.json` uses the deterministic local embeddings
provider.

## Errors

Errors use an OpenAI-style envelope:

```json
{
  "error": {
    "message": "invalid json",
    "type": "invalid_request_error",
    "code": "error"
  }
}
```
