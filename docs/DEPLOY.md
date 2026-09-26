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

DeepSeek 账号可选设置 `device_id` 和 `login_device_id`。`device_id` 会作为
`x-device-id` header 发送；`login_device_id` 只用于 `/users/login` JSON body。
未设置时，DS2API 会从账号 email/mobile 生成稳定的账号级 device id。这个默认值
避免所有实例共享同一个指纹；如果 DeepSeek 返回 `RISK_DEVICE_DETECTED`，通常需要
从真实 DeepSeek 客户端登录请求中捕获这两个值，并在 `config.json` 中固定到该账号。

如果 DeepSeek 登录被 AWS WAF 挑战拦截，可以用真实浏览器刷新托管账号 token：

```bash
./scripts/deepseek-web-login-docker.sh
docker compose restart ds2api
```

该脚本会通过 Playwright Chromium 登录，捕获成功的 `/users/login` / `/users/current`
结果，并把 `token`、`device_id`、`login_device_id` 写回 `config.json`。脚本只输出字段长度，
不会打印 token 或密码。

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
