# DS2API API 文档

语言: [中文](API.md) | [English](API.en.md)

本 fork 只暴露 OpenAI-compatible 路由。Claude、Gemini、Admin、WebUI、Vercel
bridge 路由已被有意删除。

## 基础信息

| 项目 | 值 |
| --- | --- |
| 默认 base URL | `http://127.0.0.1:5001` |
| Content type | `application/json` |
| 鉴权头 | `Authorization: Bearer <token>` 或 `x-api-key: <token>` |
| 健康检查 | `GET /healthz`, `HEAD /healthz`, `GET /readyz`, `HEAD /readyz` |

JSON 请求体必须是合法 UTF-8。

## 路由索引

| Method | Path | Auth | 说明 |
| --- | --- | --- | --- |
| `GET` | `/v1/models` | 否 | 列出支持的模型 id |
| `GET` | `/v1/models/{model_id}` | 否 | 返回单个模型对象，支持 alias |
| `POST` | `/v1/chat/completions` | 是 | OpenAI Chat Completions 兼容接口 |
| `POST` | `/v1/responses` | 是 | OpenAI Responses 兼容接口 |
| `GET` | `/v1/responses/{response_id}` | 是 | 在 TTL 内读取缓存 response |
| `POST` | `/v1/files` | 是 | 上传文件，用于支持的多模态请求 |
| `GET` | `/v1/files/{file_id}` | 是 | 获取上传文件元数据 |
| `POST` | `/v1/embeddings` | 是 | OpenAI Embeddings 兼容接口 |

同一组 handler 也注册了根路径快捷入口：`/models`、`/models/{id}`、
`/chat/completions`、`/responses`、`/responses/{response_id}`、`/files`、
`/files/{file_id}`、`/embeddings`。

## 鉴权

支持：

```http
Authorization: Bearer client-api-key
```

或：

```http
x-api-key: client-api-key
```

如果 token 存在于 `config.keys`，请求使用托管账号轮换；如果不存在，则把该
token 当作 DeepSeek token 直连使用。

托管账号模式可用 `X-Ds2-Target-Account: <email_or_mobile>` 指定账号。

## 模型

`GET /v1/models` 返回本 fork 已知的 DeepSeek canonical model ids。请求中的
OpenAI-family 常用模型名通过 `model_aliases` 映射。

常用内置 alias：

- `gpt-4o` -> `deepseek-v4-flash`
- `gpt-5` -> `deepseek-v4-pro`
- `o3` -> `deepseek-v4-pro`

`config.json` 中的 `model_aliases` 会覆盖内置值。

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

流式响应使用标准 `text/event-stream`，以 `data: [DONE]` 结束。

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

response 查询是内存缓存，由 `responses.store_ttl_seconds` 控制 TTL。

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

默认 `config.example.json` 使用 deterministic local embeddings provider。

## 错误格式

错误使用 OpenAI-style envelope：

```json
{
  "error": {
    "message": "invalid json",
    "type": "invalid_request_error",
    "code": "error"
  }
}
```
