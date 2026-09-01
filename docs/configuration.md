# Configuration

The config file is the only place where DeepSeek credentials live. It is a
JSON document loaded at startup and on `SIGHUP`.

## Path resolution

The server searches the following paths in order and uses the first that
exists:

1. `$DS2API_CONFIG_PATH` — env override
2. `/app/config.json` — container default
3. `$PWD/config.json` — local fallback

If none exist, the server starts in **env-backed mode** and reads
`DS2API_*` environment variables instead.

## Schema (slim edition)

```jsonc
{
  // Server bind address. Default: ":8080".
  // Override with env: DS2API_LISTEN.
  "listen": ":8080",

  // API keys that clients must send as `Authorization: Bearer <key>`.
  // Empty list = auth disabled (use only for local testing).
  "apiKeys": ["sk-local-1"],

  // DeepSeek web accounts. At least one is required.
  "accounts": [
    {
      "name": "primary",
      "email": "you@example.com",
      "password": "...",
      "token": "..." // optional, refreshed on login
    }
  ],

  // Model alias table. The right side is a real upstream model id.
  "modelAliases": {
    "deepseek-chat": "deepseek_chat",
    "deepseek-reasoner": "deepseek_chat"
  },

  // Thinking / reasoning injection on the request side.
  "thinkingInjection": {
    "enabled": true,
    "minChars": 16,
    "prompt": "<think>\n"
  },

  // File upload injection.
  "currentInputFile": {
    "enabled": true,
    "minChars": 32,
    "prompt": "[[file:"
  },

  // CORS: list of allowed origins. Empty = reflect request Origin.
  "corsAllowedOrigins": [],

  // Auto-delete session control.
  "autoDelete": {
    "mode": "off", // "off" | "all" | "expired"
    "sessions": 256
  },

  // Tool-call handling policy. "auto" | "manual" | "off"
  "toolcallMode": "auto",

  // Responses store TTL in seconds.
  "responsesStoreTtlSeconds": 3600
}
```

## Environment overrides

Any field can be overridden via `DS2API_<UPPERCASE_FIELD>` env vars, e.g.
`DS2API_LISTEN=:9000`, `DS2API_API_KEYS=sk-1,sk-2`.

Boolean values accept `1`, `true`, `yes`, `on`. Lists accept CSV.

## Hot reload

Send `SIGHUP` to the server to reload the file from disk. All in-flight
requests continue to use the snapshot they were bound to.

`DS2API_HOT_RELOAD=true` enables a background poller that watches the file
mtime and self-sends `SIGHUP` on change. The poller is only active when the
config was loaded from a file (not in env-backed mode).

## Secrets

`config.json` should be **gitignored**. The `Save` operation rewrites tokens
to `<redacted>` before persisting — never commit a file produced by the
server.
