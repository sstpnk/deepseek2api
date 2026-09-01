# API -> DeepSeek web chat compatibility

> This document is the source of truth for the slim edition's prompt flow.
> Any change to message normalization, tool prompt injection, file handling,
> or completion payload assembly must update this document.

## Core flow

The slim edition keeps the API surface compatible with OpenAI
`/v1/chat/completions` and `/v1/responses`, but the body of the handler is
deliberately thin:

1. The HTTP request body is decoded into an `openai.ChatCompletionRequest`
   (or the Responses equivalent).
2. `auth.Determine(ctx)` picks a DeepSeek account and a `*client.RequestAuth`
   (DeepSeek token, account id, proxy id if any).
3. The request is forwarded to `client.Client.CallCompletion(ctx, auth, payload)`.

The slim `client.Client.CallCompletion` is a **stub**: it returns an error
indicating that the upstream call is not implemented in this build. To wire
a real DeepSeek session, replace the body of
`internal/deepseek/client/client_core.go::CallCompletion` with the upstream
session/login/prompt/upload flow.

## What is intentionally NOT in slim

* Multi-turn tool-call XML serialisation (`internal/toolcall` — removed)
* History split, history re-write, roleplay aliases (`internal/format/*` — removed)
* Thinking-injection prompt rewriting (`internal/stream/thinking_injection` — removed)
* Current-input-file prompt injection (`internal/stream/current_input_file` — removed)
* File inline upload flow (`internal/httpapi/openai/files/file_inline_upload` — removed)
* Auto-delete sessions (`internal/auto_delete` — removed)
* Response history store (`internal/responsehistory` — removed)

The corresponding config knobs (`thinkingInjection`, `currentInputFile`,
`autoDelete`, `modelAliases` for Claude/Gemini/-rp) are kept in `Store` for
compatibility with the config schema but are **not consumed** by any handler
in the slim build.

## How to extend

If you need to re-introduce any of the above in the slim build, the
seam is `internal/httpapi/openai/chat/handler.go::ServeHTTP` (and the
`responses/` twin). Drop the new logic in front of
`deps.DS.CallCompletion` and return the upstream response.
