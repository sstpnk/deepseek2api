# 架构说明

DS2API 当前是 Go-only 的 OpenAI-compatible proxy，用于把 DeepSeek Web 访问封装成
OpenAI 风格接口。

## Runtime Flow

1. `cmd/ds2api` 启动 `server.NewApp`。
2. `internal/server/router.go` 组装 chi router 和 middleware。
3. OpenAI-compatible handlers 归一化传入 payload。
4. `internal/completionruntime` 创建 DeepSeek session、获取 PoW、调用 completion。
5. `internal/assistantturn` 与 `internal/format/openai` 输出 OpenAI 形状的响应和
   streaming events。

## 主要包

- `internal/server`：app 装配、CORS、健康检查、路由注册。
- `internal/auth`：调用方鉴权和托管账号解析。
- `internal/account`：账号池、lease、queue。
- `internal/config`：配置加载、校验、模型 alias、runtime 参数。
- `internal/httpapi/openai`：OpenAI-compatible HTTP handlers。
- `internal/promptcompat`：OpenAI payload 到 DeepSeek prompt 的归一化。
- `internal/completionruntime`：stream/non-stream 执行逻辑。
- `internal/deepseek`：DeepSeek protocol、transport、login、completion、files。
- `internal/toolcall` 与 `internal/toolstream`：tool-call 解析和流式过滤。
- `internal/chathistory` 与 `internal/responsehistory`：OpenAI handler 使用的本地
  runtime history store。
- `pow`：纯 Go PoW 实现。

## 已删除范围

本 fork 不再包含或注册：

- OpenAI-compatible 之外的多协议 HTTP adapters；
- browser Admin API 和静态 UI；
- Node/serverless bridge；
- raw-stream capture/replay fixtures 和工具。
