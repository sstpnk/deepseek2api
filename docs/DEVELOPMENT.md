# Development

## Common Commands

```bash
go test ./...
go vet ./...
./scripts/lint.sh
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
```

Run the app locally:

```bash
go run ./cmd/ds2api
```

## Change Map

| Area | Files |
| --- | --- |
| Routes and middleware | `internal/server/router.go` |
| OpenAI chat | `internal/httpapi/openai/chat` |
| Responses API | `internal/httpapi/openai/responses` |
| Files and inline uploads | `internal/httpapi/openai/files`, `internal/httpapi/openai/file_inline_upload.go` |
| Embeddings | `internal/httpapi/openai/embeddings` |
| Model aliases | `internal/config/models.go` |
| Config loading | `internal/config` |
| DeepSeek calls | `internal/deepseek/client` |
| Prompt conversion | `internal/promptcompat` |
| Tool calls | `internal/toolcall`, `internal/toolstream` |

## Rules

- Keep the public surface OpenAI-compatible only.
- Do not reintroduce serverless, browser UI, or non-OpenAI protocol adapters
  unless the project scope changes explicitly.
- Run `gofmt -w` on changed Go files.
- Update docs when routes, config, prompt normalization, or payload assembly
  change.
