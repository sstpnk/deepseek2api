# HTTP API (slim edition)

This is a partial OpenAI-compatible surface. The `Authorization` header is
required when `apiKeys` is set in the config.

## `POST /v1/chat/completions`

OpenAI-compatible non-streaming and streaming chat completion endpoint.

```json
{
  "model": "deepseek-chat",
  "messages": [
    { "role": "system", "content": "You are a helpful assistant." },
    { "role": "user", "content": "Hello!" }
  ],
  "stream": false,
  "temperature": 0.7
}
```

`stream: true` returns `text/event-stream` chunks with `data: {...}` payloads
terminated by `data: [DONE]`.

## `POST /v1/responses`

OpenAI Responses API compatible endpoint. Same request shape as
`/v1/chat/completions`; non-streaming JSON or `text/event-stream` chunks.

## `POST /v1/embeddings`

**Stub** in this build. Returns an empty `data` list. To enable, plug a
provider into `internal/httpapi/openai/embeddings/embeddings_handler.go`.

## `GET /v1/files`

**Stub**. Returns an empty list of files.

## `POST /v1/files`

**Stub**. Returns `501 Not Implemented`.

## `GET /v1/models`

Returns the current model alias table resolved against the upstream
account's available models.

```json
{
  "object": "list",
  "data": [
    { "id": "deepseek-chat", "object": "model", "owned_by": "deepseek" }
  ]
}
```

## `GET /healthz`

Always returns `200 OK` once the server is ready to accept connections.
