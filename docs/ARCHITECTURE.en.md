# Architecture

DS2API is now a Go-only OpenAI-compatible proxy around DeepSeek Web access.

## Runtime Flow

1. `cmd/ds2api` starts `server.NewApp`.
2. `internal/server/router.go` builds the chi router and middleware.
3. OpenAI-compatible handlers normalize incoming payloads.
4. `internal/completionruntime` creates DeepSeek sessions, obtains PoW, and
   calls completion endpoints.
5. `internal/assistantturn` and `internal/format/openai` render OpenAI-shaped
   responses and streaming events.

## Main Packages

- `internal/server`: app assembly, CORS, health checks, route registration.
- `internal/auth`: caller auth and managed-account resolution.
- `internal/account`: account pool, leases, queueing.
- `internal/config`: config loading, validation, model aliases, runtime knobs.
- `internal/httpapi/openai`: OpenAI-compatible HTTP handlers.
- `internal/promptcompat`: OpenAI payload to DeepSeek prompt normalization.
- `internal/completionruntime`: shared non-stream and stream execution logic.
- `internal/deepseek`: DeepSeek protocol, transport, login, completion, files.
- `internal/toolcall` and `internal/toolstream`: tool-call parsing and streaming
  filtering.
- `internal/chathistory` and `internal/responsehistory`: local runtime history
  stores used by OpenAI handlers.
- `pow`: pure Go proof-of-work implementation.

## Removed Surfaces

This fork no longer contains or registers:

- multi-protocol HTTP adapters outside OpenAI-compatible routes;
- browser Admin API and static UI;
- Node or serverless bridge code;
- raw-stream capture/replay fixtures and tools.
