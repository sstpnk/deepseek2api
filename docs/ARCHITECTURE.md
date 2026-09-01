# Architecture (slim edition)

A single Go binary. The slim surface intentionally removes the admin UI,
Vercel/Zeabur deploys, the Cloudflare worker proxy, the Claude/Gemini/-rp
model aliases, and the Node runtime bridge from the upstream fork.

## Top-level layout

```
cmd/ds2api/         # main: signal handling, config load, server start
internal/
  account/          # account pool (slim placeholder)
  auth/             # forwarder: type aliases to internal/deepseek/client
  config/           # Store + Config types, env-backed fallback
  deepseek/client/  # Resolver, RequestAuth, Client stubs
  httpapi/openai/
    chat/           # /v1/chat/completions handler
    responses/      # /v1/responses handler
    embeddings/     # /v1/embeddings stub
    files/          # /v1/files stub
    shared/         # Deps, ConfigReader, DeepSeekCaller, models list
  server/           # Mux, CORS middleware
  testsuite/        # shared test helpers
  version/          # build version constant
pow/                # SHA-3 (Keccak) for DeepSeek PoW (unused in slim)
```

## Request lifecycle

1. `cmd/ds2api/main.go` resolves the config path, builds a `*config.Store`,
   starts an `*server.App`.
2. Incoming HTTP request hits `server.App.Handler()` — a `http.ServeMux`
   wrapped in CORS middleware.
3. The route is matched to one of the OpenAI handler packages. Each handler
   calls `deps.Auth.Determine(ctx)` to acquire a `*client.RequestAuth`
   (account + token), then `deps.DS.CallCompletion(ctx, auth, payload)` to
   send the request upstream.
4. The DeepSeek client in this build is a thin wrapper — `CallCompletion`
   returns a `*client.CompletionResult` and the handler maps it to the
   OpenAI response shape.

## Config hot reload

* `(*Store).ReloadFromFile()` re-reads the JSON and atomically swaps the
  in-memory config snapshot.
* `SIGHUP` calls `ReloadFromFile()`.
* `DS2API_HOT_RELOAD=true` enables a polling watcher (5s interval) that
  detects mtime changes and self-sends `SIGHUP`.

## Why slim

The slim edition exists so the project can be packaged as a single static
binary (or 7 MB distroless Docker image) with one surface to maintain: the
OpenAI-compatible HTTP API. Everything that was tied to multi-tenant
deployment, admin keys, or alternative LLM providers has been stripped.
