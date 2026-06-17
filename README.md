# copilot2api

[English](README.md) | [简体中文](README.zh-CN.md)

A lightweight Go proxy that exposes GitHub Copilot as OpenAI-compatible, Anthropic-compatible, Gemini-compatible, and AmpCode-compatible API endpoints.

## Features

- **OpenAI API Compatible**: `/v1/chat/completions`, `/v1/models`, `/v1/embeddings`, `/v1/responses`
- **Embeddings Support**: Native OpenAI-compatible `/v1/embeddings` endpoint
- **Anthropic API Compatible**: `/v1/messages`
- **Gemini API Compatible**: `/v1beta/models`, `/v1beta/models/{model}:generateContent`, `/v1beta/models/{model}:streamGenerateContent`, `/v1beta/models/{model}:countTokens`
- **AmpCode Compatible**: `/amp/v1/*` routes for chat, `/api/provider/*` for provider-specific calls, management proxied to `ampcode.com`
- **Streaming Support**: Full SSE streaming for both OpenAI and Anthropic formats
- **Anthropic Routing**: Uses native `/v1/messages` when the model supports it, otherwise routes via `/responses` or `/chat/completions`
- **Multi-Account**: Map API keys to GitHub accounts 1:1 with isolated credential stores (see [Multiple GitHub Accounts](#multiple-github-accounts))
- **Web Admin UI**: Manage accounts and view token-usage statistics at `/admin/` (multi-account mode)
- **Auto Authentication**: GitHub Device Flow OAuth with automatic token refresh
- **Usage Monitoring**: Built-in `/usage` endpoint for quota tracking
- **Models Cache**: 5-minute cache for `/v1/models` and Anthropic model capability lookups

## Quick Start

### Docker

Build the image from source (this fork includes multi-account and usage-stats features not in the upstream image):

```bash
docker build -t copilot2api-multiusers .
```

Run it:

```bash
docker run -it --rm \
  -p 127.0.0.1:7777:7777 \
  -v ~/.config/copilot2api:/root/.config/copilot2api \
  copilot2api-multiusers
```

The volume mount persists your GitHub credentials across container restarts. The examples publish the port on `127.0.0.1` only so the proxy stays local by default.

> Tip: when running the admin UI / multi-account mode, you connect from your host browser to `http://127.0.0.1:7777/admin/`. The container listens on `0.0.0.0:7777` internally (set via `COPILOT2API_HOST`), so the published `127.0.0.1` port stays local-only.

<details>
<summary>Docker Compose</summary>

```yaml
services:
  copilot2api:
    build: .
    ports:
      - "127.0.0.1:7777:7777"
    volumes:
      - ${HOME}/.config/copilot2api:/root/.config/copilot2api
```

Build and start it with:

```bash
docker compose up --build
```

</details>

The server starts on `http://127.0.0.1:7777` by default. Open the admin UI at **`http://127.0.0.1:7777/admin/`** to add and authenticate GitHub accounts via a browser-driven Device Flow (see [Multiple GitHub Accounts](#multiple-github-accounts)).

## Security

⚠️ **This proxy is designed for local development only.**

- Does **not** validate API keys by default — any request is accepted. Enable per-account API key validation by configuring multiple accounts (see [Multiple GitHub Accounts](#multiple-github-accounts)).
- Do not expose publicly — it becomes an open proxy consuming your Copilot quota
- Credentials are stored in `~/.config/copilot2api/credentials.json`

## Multiple GitHub Accounts

You can map API keys to GitHub accounts 1:1 by creating an `accounts.json` file in your token directory (`~/.config/copilot2api/accounts.json` by default, or set `COPILOT2API_ACCOUNTS_FILE`):

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

When `accounts.json` is present, requests **must** present a valid key or receive `401 Unauthorized`. When the file is absent, the proxy runs in single-account mode with no API key validation (unchanged behavior).

### Admin UI

In multi-account mode the proxy serves a web UI at **`http://127.0.0.1:7777/admin/`** to maintain the mapping without editing `accounts.json` by hand:

- List accounts and their authentication status.
- Add an account (id + API key + optional token dir) and authenticate it via a browser-driven GitHub Device Flow (shows the code + verification link, polls until done).
- Rotate an account's API key, or delete an account.
- **Stats tab**: view per-account, per-model token counts — input, output, cached (prompt-cache hits), cache-write, and request totals — across all OpenAI, Anthropic, and Gemini endpoints. Usage is persisted to `<token-dir>/stats.json` and survives restarts (backed by `GET /admin/api/stats`, with `DELETE /admin/api/stats/{id}` to reset one account).

> Note: OpenAI Chat Completions streaming only contributes token counts when the client sends `stream_options.include_usage`; the request itself is always counted.

All changes are written back to `accounts.json` and applied to the running proxy immediately — no restart needed. You can bootstrap from an empty file:

```json
{ "accounts": [] }
```

then add and authenticate every account from the UI.

⚠️ The admin UI can read API keys and trigger GitHub authentication. Keep it local. To require a token, set `COPILOT2API_ADMIN_TOKEN`; the UI then expects it as an `X-Admin-Token` header or `?admin_token=<token>` query parameter (open `http://127.0.0.1:7777/admin/?admin_token=<token>`).

## Usage with Claude Code

Add to `~/.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:7777",
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
base_url = "http://127.0.0.1:7777/v1"
wire_api = "responses"
api_key = "dummy"
```

## Usage with Gemini CLI

Add to `~/.gemini/.env`:

```env
GOOGLE_GEMINI_BASE_URL=http://127.0.0.1:7777
GEMINI_API_KEY=dummy
GEMINI_MODEL=claude-opus-4.6-1m
```

## Usage with AmpCode

Set the `AMP_URL` environment variable to point at copilot2api:

```bash
AMP_URL=http://127.0.0.1:7777/amp amp
```

Or add to `~/.config/amp/settings.json`:

```json
{
  "amp.url": "http://127.0.0.1:7777/amp"
}
```

Chat completions, tool calls, and image input all route through Copilot API. Login and management routes (threads, telemetry) are proxied to `ampcode.com` — a free amp account is required for authentication.

<details>
<summary>Usage with curl</summary>

```bash
# OpenAI chat completion
curl http://localhost:7777/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.3-codex","messages":[{"role":"user","content":"Hello!"}]}'

# Anthropic message
curl http://localhost:7777/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -d '{"model":"claude-sonnet-4.6","messages":[{"role":"user","content":"Hello!"}],"max_tokens":100}'

# List models
curl http://localhost:7777/v1/models

# Check usage/quota
curl http://localhost:7777/usage
```

</details>

<details>
<summary>Usage with SDKs</summary>

### OpenAI Python SDK

```python
import openai

client = openai.OpenAI(
    api_key="dummy",
    base_url="http://127.0.0.1:7777/v1"
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
    base_url="http://127.0.0.1:7777"
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
| `/v1beta/models` | GET | List Gemini-compatible models |
| `/v1beta/models/{model}:generateContent` | POST | Gemini Generate Content |
| `/v1beta/models/{model}:streamGenerateContent` | POST | Gemini Generate Content streaming SSE |
| `/v1beta/models/{model}:countTokens` | POST | Gemini token counting estimate |
| `/amp/v1/chat/completions` | POST | AmpCode chat completions (via Copilot API) |
| `/amp/v1/models` | GET | AmpCode model listing |
| `/api/provider/*` | POST | AmpCode provider-specific routes |
| `/api/*` | ANY | AmpCode management proxy to ampcode.com |
| `/usage` | GET | Copilot usage and quota info |
| `/admin/` | GET | Web admin UI (multi-account mode only) |
| `/admin/api/stats` | GET | Per-account / per-model token-usage statistics |
| `/admin/api/stats/{id}` | DELETE | Reset usage statistics for one account |

## Configuration

### CLI Flags

```
./copilot2api [options]

  -host string       Server host (default "127.0.0.1")
  -port int          Server port (default 7777)
  -token-dir string  Token storage directory (default ~/.config/copilot2api)
  -debug             Enable debug logging
  -version           Show version and exit
```

### Environment Variables

Environment variables are used as defaults when flags are not provided:

| Variable | Description | Default |
|----------|-------------|---------|
| `COPILOT2API_HOST` | Server host | `127.0.0.1` |
| `COPILOT2API_PORT` | Server port | `7777` |
| `COPILOT2API_TOKEN_DIR` | Token storage directory | `~/.config/copilot2api` |
| `COPILOT2API_ACCOUNTS_FILE` | Multi-account config file path (see [Multiple GitHub Accounts](#multiple-github-accounts)) | `<token-dir>/accounts.json` |
| `COPILOT2API_ADMIN_TOKEN` | If set, the `/admin/` UI requires this token (`X-Admin-Token` header or `?admin_token=`) | _(unset, no auth)_ |
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

## License

MIT
