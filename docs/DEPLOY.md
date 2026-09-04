# 部署说明

DS2API 是单个 Go binary。配置来源可以是 `config.json`、`DS2API_CONFIG_PATH`
或 `DS2API_CONFIG_JSON`。

## 本地

```bash
cp config.example.json config.json
go run ./cmd/ds2api
```

默认端口是 `5001`，可用 `PORT` 覆盖。

## Docker Compose

```bash
cp .env.example .env
cp config.example.json config.json
docker compose up --build
```

`docker-compose.yml` 会把 `config.json` 挂载进容器，并在宿主机暴露
`${DS2API_HOST_PORT:-6011}`。

## 环境变量

| Variable | 说明 | 默认值 |
| --- | --- | --- |
| `PORT` | app/container 内 HTTP 监听端口 | `5001` |
| `DS2API_HOST_PORT` | Docker Compose 使用的宿主机端口 | `6011` |
| `LOG_LEVEL` | 日志级别 | `INFO` |
| `DS2API_CONFIG_PATH` | 配置文件路径 | `config.json` 或容器默认值 |
| `DS2API_CONFIG_JSON` | raw JSON 或 Base64 JSON 配置 | empty |
| `CONFIG_JSON` | `DS2API_CONFIG_JSON` 的 legacy alias | empty |
| `DS2API_ENV_WRITEBACK` | 尽可能把 env config 写回文件 | enabled |
| `DS2API_CHAT_HISTORY_PATH` | 本地 chat history 文件路径 | `data/chat_history.json` |
| `DS2API_RUNTIME_STATS_PATH` | runtime stats 文件路径 | `data/runtime_stats.json` |

## 健康检查

```bash
curl http://127.0.0.1:5001/healthz
curl http://127.0.0.1:5001/readyz
```

## Release Build

```bash
go build -o ds2api ./cmd/ds2api
```
