# DS2API

语言: [中文](README.MD) | [English](README.en.md)

DS2API 是一个精简的 Go 服务，把 DeepSeek Web 访问封装成
OpenAI-compatible HTTP API。本 fork 只保留 OpenAI SDK 兼容客户端需要的接口。

本 fork 已删除：Claude 兼容路由、Gemini 兼容路由、Admin API、WebUI、Vercel
bridge、Node stream helpers、raw stream replay tools。

## 路由

标准 OpenAI-compatible 路由：

- `GET /v1/models`
- `GET /v1/models/{model_id}`
- `POST /v1/chat/completions`
- `POST /v1/responses`
- `GET /v1/responses/{response_id}`
- `POST /v1/files`
- `GET /v1/files/{file_id}`
- `POST /v1/embeddings`

同时保留根路径快捷入口，便于客户端 base URL 直接指向服务根地址：
`/models`、`/chat/completions`、`/responses`、`/files`、`/embeddings`。

## 配置

复制示例配置并编辑：

```bash
cp config.example.json config.json
```

最小配置：

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

鉴权行为：

- 支持 `Authorization: Bearer <token>` 和 `x-api-key: <token>`。
- `<token>` 在 `keys` 中时，服务使用托管账号轮换。
- `<token>` 不在 `keys` 中时，服务把它当作 DeepSeek token 直连使用。

## 本地运行

```bash
go run ./cmd/ds2api
```

服务监听 `PORT`，默认 `5001`。

快速检查：

```bash
curl http://127.0.0.1:5001/v1/models
```

## Docker

```bash
docker compose up --build
```

compose 会读取 `.env`，挂载 `config.json`，并暴露宿主机端口
`${DS2API_HOST_PORT:-6011}`。

## 测试

```bash
./scripts/lint.sh
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
```

在没有 Unix shell 的 Windows PowerShell 中，单元测试可直接运行：

```powershell
$env:GOCACHE = "$(Get-Location)\.tmp\go-build-cache"
go test ./...
```

## 文档

- [API 文档](API.md)
- [架构说明](docs/ARCHITECTURE.md)
- [部署说明](docs/DEPLOY.md)
- [测试说明](docs/TESTING.md)
- [Prompt compatibility](docs/prompt-compatibility.md)
