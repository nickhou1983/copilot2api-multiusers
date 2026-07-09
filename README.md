# copilot2api

[English](README.md) | [简体中文](README.zh-CN.md)

> This is a fork of [whtsky/copilot2api](https://github.com/whtsky/copilot2api) that adds multi-account support, a web admin UI, and token-usage statistics. All credit for the original project goes to the upstream authors.

A lightweight Go proxy that exposes GitHub Copilot as OpenAI-compatible, Anthropic-compatible, Gemini-compatible, and AmpCode-compatible API endpoints.

## Features

- **OpenAI API Compatible**: `/v1/chat/completions`, `/v1/models`, `/v1/embeddings`, `/v1/responses`
- **Embeddings Support**: Native OpenAI-compatible `/v1/embeddings` endpoint
- **Anthropic API Compatible**: `/v1/messages`, `/v1/messages/count_tokens`
- **Gemini API Compatible**: `/v1beta/models`, `/v1beta/models/{model}:generateContent`, `/v1beta/models/{model}:streamGenerateContent`, `/v1beta/models/{model}:countTokens`
- **AmpCode Compatible**: `/amp/v1/*` routes for chat, `/api/provider/*` for provider-specific calls, management proxied to `ampcode.com`
- **Streaming Support**: Full SSE streaming for both OpenAI and Anthropic formats
- **Anthropic Routing**: Uses native `/v1/messages` when the model supports it, otherwise routes via `/responses` or `/chat/completions`. Native passthrough preserves advanced fields such as `context_management` (auto-adding the `context-management-2025-06-27` beta) and `search_result` content blocks, and forwards the client's `computer-use-*` beta header so the Computer Use tool works upstream.
- **Multi-Account**: Map API keys to GitHub accounts 1:1 with isolated credential stores (see [Multiple GitHub Accounts](#multiple-github-accounts))
- **Web Admin UI**: Manage accounts and view token-usage statistics at `/admin/` (multi-account mode)
- **Auto Authentication**: GitHub Device Flow OAuth with automatic token refresh
- **Usage Monitoring**: Built-in `/usage` endpoint for quota tracking
- **Models Cache**: 5-minute cache for `/v1/models` and Anthropic model capability lookups

## Quick Start

### Docker

Build the image from source (this fork includes multi-account and usage-stats features not in the upstream image):

```bash
docker build -t copilot2api-multiusers-safeadmin .
```

Run it:

```bash
docker run -it --rm \
  -p 8888:8888 \
  -p 8889:8889 \
  -e COPILOT2API_ADMIN_USERNAME=admin \
  -e COPILOT2API_ADMIN_PASSWORD='change-me' \
  -v ~/.config/copilot2api:/root/.config/copilot2api \
  copilot2api-multiusers-safeadmin
```

The volume mount persists your GitHub credentials across container restarts. The examples publish both the public API port (`8888`) and admin port (`8889`).

> Tip: the public API listens on `0.0.0.0:8888`. The admin UI is served by a separate listener on `0.0.0.0:8889`.
> Health probes can call `GET /health` on either listener without authentication. The public listener reports `copilot2api`; the admin listener reports `copilot2api-admin`.

<details>
<summary>Docker Compose</summary>

```yaml
services:
  copilot2api:
    build: .
    ports:
      - "8888:8888"
      - "8889:8889"
    environment:
      COPILOT2API_ADMIN_USERNAME: admin
      COPILOT2API_ADMIN_PASSWORD: change-me
    volumes:
      - ${HOME}/.config/copilot2api:/root/.config/copilot2api
```

Build and start it with:

```bash
docker compose up --build
```

</details>

The public API listens on `0.0.0.0:8888` by default. The admin UI listens on **`0.0.0.0:8889`** when `COPILOT2API_ADMIN_USERNAME` and `COPILOT2API_ADMIN_PASSWORD` are set; open `http://<server-ip>:8889/admin/` to add and authenticate GitHub accounts via a browser-driven Device Flow (see [Multiple GitHub Accounts](#multiple-github-accounts)).

## Security

⚠️ **This proxy is designed for local development only.**

- Validates API keys by default: every request must present a key mapping to a configured account, otherwise it gets `401 Unauthorized` (see [Multiple GitHub Accounts](#multiple-github-accounts)).
- Do not expose publicly — it becomes an open proxy consuming your Copilot quota
- Keep the admin listener separate from the public API. Expose only `8888` through internet-facing gateways, and reach `8889` through loopback, SSH tunnel, VPN, or another restricted management path.
- Each account's credentials are stored under `~/.config/copilot2api/<token_dir>/credentials.json`

## Multiple GitHub Accounts

The proxy always runs in multi-account mode and maps API keys to GitHub accounts 1:1 via an `accounts.json` file in your token directory (`~/.config/copilot2api/accounts.json` by default, or set `COPILOT2API_ACCOUNTS_FILE`). **If the file does not exist it is created automatically as an empty config** (`{"accounts": []}`) on first start, so the admin UI can populate it after you configure admin credentials (see [Admin UI](#admin-ui)).

You can also edit `accounts.json` by hand:

```json
{
  "accounts": [
    { "id": "alice", "api_key": "sk-alice-...", "token_dir": "alice" },
    { "id": "bob",   "api_key": "sk-bob-...",   "token_dir": "bob" }
  ]
}
```

- `id` — unique account identifier (used in logs; defaults the token sub-directory name).
- `api_key` — the key clients must present. Must be unique across accounts.
- `token_dir` — where this account's `credentials.json` is stored. Relative paths resolve under the base token directory; defaults to `id`.

On startup the proxy runs the GitHub Device Flow once **per account** (sequentially) for any account that has no stored token. Each account keeps an isolated credential store and its own models cache, so token refresh and capability-based routing stay independent.

Clients select an account by sending its `api_key`:

- OpenAI: `Authorization: Bearer <api_key>`
- Anthropic: `x-api-key: <api_key>`
- Gemini: `x-goog-api-key: <api_key>` or `?key=<api_key>`

Requests **must** present a valid key or receive `401 Unauthorized`. Until at least one account is configured (e.g. via the admin UI), every request is rejected with `401`.

### Admin UI

The proxy serves a password-protected web UI on a separate admin listener, **`0.0.0.0:8889`** by default, to maintain the mapping without editing `accounts.json` by hand:

- List accounts and their authentication status.
- Add an account (id + API key + optional token dir) and authenticate it via a browser-driven GitHub Device Flow (shows the code + verification link, polls until done).
- Rotate an account's API key, or delete an account.
- **Stats tab**: view per-account, per-model token counts — input, output, cached (prompt-cache hits), cache-write, and request totals — across all OpenAI, Anthropic, and Gemini endpoints. Usage is persisted to `<token-dir>/stats.json` and survives restarts (backed by `GET /admin/api/stats`, with `DELETE /admin/api/stats/{id}` to reset one account).

> Note: OpenAI Chat Completions streaming only contributes token counts when the client sends `stream_options.include_usage`; the request itself is always counted.

All changes are written back to `accounts.json` and applied to the running proxy immediately — no restart needed.

⚠️ The admin UI can read API keys, reveal stored GitHub/Copilot tokens, and trigger GitHub authentication. Set `COPILOT2API_ADMIN_USERNAME` and `COPILOT2API_ADMIN_PASSWORD`; the admin server refuses to start without them unless `COPILOT2API_ADMIN_ENABLED=false`. `COPILOT2API_ADMIN_TOKEN` is retained only as a deprecated header-only compatibility option for scripted callers.

For Azure VM deployments behind Application Gateway, route public traffic only to the API listener (`8888`). Do not add the admin listener (`8889`) to the public backend rule. Restrict the VM NSG so only the Application Gateway subnet can reach `8888`, and use SSH tunnel, Bastion, VPN, or a separately locked-down listener if you need remote admin access.

For Application Gateway health probes, prefer `GET /health` on the backend port you expose instead of `/usage` or model endpoints, because those routes can require API keys or request bodies.

## Usage with Claude Code

Add to `~/.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:8888",
    "ANTHROPIC_API_KEY": "dummy",
    "ANTHROPIC_MODEL": "claude-opus-4.6",
    "ANTHROPIC_SMALL_FAST_MODEL": "claude-haiku-4.5",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  },
  "permissions": {
    "deny": [
      "WebSearch"
    ]
  }
}
```

### 1M Context Window

copilot2api supports Claude 1M context models. When Claude Code sends the `anthropic-beta: context-1m-...` header, the proxy automatically appends `-1m` to the model ID (e.g. `claude-opus-4.6` → `claude-opus-4.6-1m`) so Copilot routes to the 1M variant.

To use it, select the 1M model variant in Claude Code via the `/model` command (e.g. `Opus (1M)`). Without this, Claude Code defaults to the standard 200K context window.

## Usage with Codex

Add to `~/.codex/config.toml`:

```toml
model = "gpt-5.3-codex"
model_provider = "copilot2api"
model_reasoning_effort = "high"
web_search = "disabled"

[model_providers.copilot2api]
name = "copilot2api"
base_url = "http://127.0.0.1:8888/v1"
wire_api = "responses"
api_key = "dummy"
```

## Usage with Gemini CLI

Add to `~/.gemini/.env`:

```env
GOOGLE_GEMINI_BASE_URL=http://127.0.0.1:8888
GEMINI_API_KEY=dummy
GEMINI_MODEL=claude-opus-4.6-1m
```

## Usage with AmpCode

Set the `AMP_URL` environment variable to point at copilot2api:

```bash
AMP_URL=http://127.0.0.1:8888/amp amp
```

Or add to `~/.config/amp/settings.json`:

```json
{
  "amp.url": "http://127.0.0.1:8888/amp"
}
```

Chat completions, tool calls, and image input all route through Copilot API. Login and management routes (threads, telemetry) are proxied to `ampcode.com` — a free amp account is required for authentication.

<details>
<summary>Usage with curl</summary>

```bash
# OpenAI chat completion
curl http://localhost:8888/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.3-codex","messages":[{"role":"user","content":"Hello!"}]}'

# Anthropic message
curl http://localhost:8888/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -d '{"model":"claude-sonnet-4.6","messages":[{"role":"user","content":"Hello!"}],"max_tokens":100}'

# List models
curl http://localhost:8888/v1/models

# Check usage/quota
curl http://localhost:8888/usage
```

</details>

<details>
<summary>Usage with SDKs</summary>

### OpenAI Python SDK

```python
import openai

client = openai.OpenAI(
    api_key="dummy",
    base_url="http://127.0.0.1:8888/v1"
)

response = client.chat.completions.create(
    model="gpt-5.3-codex",
    messages=[{"role": "user", "content": "Hello!"}]
)
```

### Anthropic Python SDK

```python
import anthropic

client = anthropic.Anthropic(
    api_key="dummy",
    base_url="http://127.0.0.1:8888"
)

message = client.messages.create(
    model="claude-sonnet-4.6",
    max_tokens=1024,
    messages=[{"role": "user", "content": "Hello!"}]
)
```

</details>

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/chat/completions` | POST | OpenAI Chat Completions (streaming & non-streaming) |
| `/v1/responses` | POST | OpenAI Responses API |
| `/v1/models` | GET | List available models (5min cache) |
| `/v1/embeddings` | POST | Generate embeddings (string or array input) |
| `/v1/messages` | POST | Anthropic Messages API (streaming & non-streaming) |
| `/v1/messages/count_tokens` | POST | Anthropic token counting (proxied to upstream) |
| `/v1beta/models` | GET | List Gemini-compatible models |
| `/v1beta/models/{model}:generateContent` | POST | Gemini Generate Content |
| `/v1beta/models/{model}:streamGenerateContent` | POST | Gemini Generate Content streaming SSE |
| `/v1beta/models/{model}:countTokens` | POST | Gemini token counting estimate |
| `/amp/v1/chat/completions` | POST | AmpCode chat completions (via Copilot API) |
| `/amp/v1/models` | GET | AmpCode model listing |
| `/api/provider/*` | POST | AmpCode provider-specific routes |
| `/api/*` | ANY | AmpCode management proxy to ampcode.com |
| `/usage` | GET | Copilot usage and quota info |
| `/admin/` | GET | Web admin UI on the separate admin listener |
| `/admin/api/stats` | GET | Per-account / per-model token-usage statistics on the admin listener |
| `/admin/api/stats/{id}` | DELETE | Reset usage statistics for one account on the admin listener |

## Configuration

### CLI Flags

```
./copilot2api [options]

  -host string       Server host (default "0.0.0.0")
  -port int          Server port (default 8888)
  -admin-host string Admin server host (default "0.0.0.0")
  -admin-port int    Admin server port (default 8889)
  -token-dir string  Token storage directory (default ~/.config/copilot2api)
  -debug             Enable debug logging
  -version           Show version and exit
```

### Environment Variables

Environment variables are used as defaults when flags are not provided:

| Variable | Description | Default |
|----------|-------------|---------|
| `COPILOT2API_HOST` | Server host | `0.0.0.0` |
| `COPILOT2API_PORT` | Server port | `8888` |
| `COPILOT2API_TOKEN_DIR` | Token storage directory | `~/.config/copilot2api` |
| `COPILOT2API_ACCOUNTS_FILE` | Multi-account config file path (see [Multiple GitHub Accounts](#multiple-github-accounts)) | `<token-dir>/accounts.json` |
| `COPILOT2API_ADMIN_ENABLED` | Start the separate admin server | `true` |
| `COPILOT2API_ADMIN_HOST` | Admin server host | `0.0.0.0` |
| `COPILOT2API_ADMIN_PORT` | Admin server port | `8889` |
| `COPILOT2API_ADMIN_USERNAME` | Admin login username; required when admin is enabled | _(unset)_ |
| `COPILOT2API_ADMIN_PASSWORD` | Admin login password; required when admin is enabled | _(unset)_ |
| `COPILOT2API_ADMIN_TOKEN` | Deprecated compatibility token for `X-Admin-Token` scripted access | _(unset)_ |
| `COPILOT2API_DEBUG` | Enable debug logging (`true`/`false`, `1`/`0`) | `false` |

CLI flags take precedence over environment variables.

## How It Works

1. Authenticates with GitHub via Device Flow OAuth
2. Exchanges GitHub token for Copilot API token (auto-refreshes)
3. Proxies OpenAI-format requests directly to Copilot API
4. Routes Anthropic Messages requests by model capabilities (native `/v1/messages`, translated `/responses`, or translated `/chat/completions`)
5. Automatically detects API endpoint from token (Individual/Business/Enterprise)

## Development

```bash
go test ./...              # Run tests
go build -o copilot2api .  # Build
```

## Acknowledgements

This project is a fork of [whtsky/copilot2api](https://github.com/whtsky/copilot2api). The core proxy, protocol conversion, and authentication originate from the upstream repository; this fork builds on it with multi-account support, the web admin UI, and token-usage statistics. Thanks to the original authors and contributors.

## License

MIT
