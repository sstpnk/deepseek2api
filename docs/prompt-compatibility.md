# Prompt Compatibility

This document tracks the current OpenAI request to DeepSeek prompt pipeline.

## Scope

Only OpenAI-compatible inputs are supported:

- Chat Completions: `internal/httpapi/openai/chat`
- Responses: `internal/httpapi/openai/responses`
- Shared normalization: `internal/promptcompat`

## Pipeline

1. HTTP handlers decode JSON and validate request size/UTF-8.
2. Inline data URLs are uploaded when supported and replaced with file ids.
3. `promptcompat.NormalizeOpenAIChatRequest` converts OpenAI messages, tools,
   model aliases, stream flags, and feature flags into a `StandardRequest`.
4. `completionruntime` creates the DeepSeek session, obtains PoW, and submits
   the final prompt payload.
5. `assistantturn` and OpenAI formatters convert DeepSeek output into
   OpenAI-shaped responses.

## Tool Calls

Tool definitions are injected into the prompt as DSML instructions. The model is
expected to emit tool calls at the end of the response in DSML form. Streaming
filters hide partial tool markup from user-visible content and emit OpenAI tool
call deltas when a complete call is recognized.

## Reduced Prompt

Aliases ending in `-rp` use reduced-prompt behavior for roleplay-style clients:
tool schema injection is skipped and only the latest compact context is sent.

## Removed Inputs

Non-OpenAI protocol inputs are no longer normalized. Removed legacy config keys
such as `compat`, `toolcall`, `history_split`, `admin`, and `vercel` are ignored
when config is loaded.

## Required Tests

```bash
go test ./internal/promptcompat
go test ./internal/httpapi/openai/...
go test ./internal/completionruntime
```
