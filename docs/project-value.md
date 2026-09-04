# DS2API Project Value

DS2API exists to make DeepSeek Web access usable from standard OpenAI-compatible
clients.

The project is not a general multi-protocol gateway anymore. The current fork
focuses on one contract:

- accept OpenAI-style HTTP requests;
- normalize messages, tools, files, model aliases, and stream flags;
- call DeepSeek Web-compatible upstream endpoints;
- return OpenAI-shaped JSON or SSE responses.

## Why It Is Useful

DeepSeek Web behavior is not the same as a stable OpenAI API surface. DS2API
bridges the practical gaps:

- session creation and continuation;
- proof-of-work handling;
- managed account selection and queueing;
- model alias mapping;
- tool-call instruction and parsing;
- streaming output cleanup;
- file upload and reference handling;
- OpenAI Chat Completions, Responses, Files, Models, and Embeddings endpoints.

The value is in keeping those details behind one small API-compatible service
that existing clients can point at.

## Boundaries

DS2API is not an official DeepSeek API, not a training platform, and not a
browser management application. This fork deliberately avoids extra UI and
protocol surfaces so the runtime contract stays easy to inspect and test.
