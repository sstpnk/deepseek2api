# DS2API Docs

Language: Chinese and English docs are kept together where that is practical.

Current fork scope: OpenAI-compatible API only. Removed surfaces are not part of
the runtime contract.

Recommended reading:

- [Project overview](../README.en.md)
- [API reference](../API.en.md)
- [Architecture](ARCHITECTURE.en.md)
- [Deployment](DEPLOY.en.md)
- [Testing](TESTING.md)
- [Development](DEVELOPMENT.md)
- [Prompt compatibility](prompt-compatibility.md)
- [Tool-call semantics](toolcall-semantics.md)
- [Project value note](project-value.md)

Runtime source of truth:

- Routes: `internal/server/router.go`
- Models and aliases: `internal/config/models.go`
- Chat normalization: `internal/promptcompat`
- OpenAI handlers: `internal/httpapi/openai`
- DeepSeek client: `internal/deepseek/client`
