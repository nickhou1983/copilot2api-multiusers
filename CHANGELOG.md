# Changelog

[English](CHANGELOG.md) | [简体中文](CHANGELOG.zh-CN.md)

## [Unreleased]

### Features

- Add API key auto-generation: creating an account without specifying `api_key` now automatically generates a cryptographically random key (`sk-` prefix + 32 base62 characters). The admin UI includes a "Generate" button next to the API Key input, and a new `GET /admin/api/generate-key` endpoint returns a freshly generated key on demand.

- Add a native Anthropic token-counting endpoint: `POST /v1/messages/count_tokens` now proxies to the upstream Copilot token counter (previously it returned `404`). The request is forwarded with the same model-alias resolution and `cache_control.scope` stripping as `/v1/messages`, and the upstream `{ "input_tokens": N }` response is returned verbatim.
- Forward `context_management` on native `/v1/messages` requests instead of stripping it. When a request body includes a `context_management` field, the proxy preserves it and adds the `anthropic-beta: context-management-2025-06-27` header to the upstream call so context edits (e.g. `clear_tool_uses_20250919`) are actually applied and reported back in `usage`/`context_management.applied_edits`.
- Add multi-account support: map API keys to GitHub accounts 1:1 via an `accounts.json` config file. Each account uses an isolated credential store and its own models cache, so token refresh and capability-based routing stay per-account. Configure the file path with `COPILOT2API_ACCOUNTS_FILE` (defaults to `<token-dir>/accounts.json`).
- API keys are extracted from `Authorization: Bearer`, `x-api-key`, `x-goog-api-key`, or the `?key=` query parameter, covering OpenAI, Anthropic, and Gemini clients.
- Add a web admin UI at `/admin/` (multi-account mode only) to maintain the API key ↔ GitHub account mapping: list, add, rotate keys, and delete accounts, plus authenticate accounts via a browser-driven GitHub Device Flow. Changes are saved to `accounts.json` and applied live without a restart. Optionally protect it with `COPILOT2API_ADMIN_TOKEN` (sent as `X-Admin-Token` header or `?admin_token=`).
- Bootstrap multi-account mode from an empty `accounts.json` (`{"accounts":[]}`) and populate it entirely through the admin UI.
- Add a token-usage statistics page to the admin UI (new "Stats" tab) showing per-account, per-model token counts — input, output, cached (prompt-cache hits), cache-write, and request totals — across all OpenAI, Anthropic, and Gemini endpoints. Usage is persisted to `<token-dir>/stats.json` and survives restarts. Backed by a new `GET /admin/api/stats` endpoint, with `DELETE /admin/api/stats/{id}` to reset one account. Note: OpenAI Chat Completions streaming only contributes token counts when the client sends `stream_options.include_usage`; the request is always counted.

### Bug Fixes

- Fix `search_result` content blocks being rejected with `400 "content must be string or array of blocks"`. Search-result blocks carry a bare string `source`, but the proxy only modeled the object image `source`, so parsing the whole content array failed before the request ever reached upstream. `AnthropicImageSource` now accepts both an object source and a bare string source, restoring native passthrough of `search_result` blocks — which the Copilot upstream supports, returning `search_result_location` citations. The Chat Completions and Responses conversion paths downgrade `search_result` blocks to plain text (preserving the content, dropping citation metadata that those APIs can't express).
- Fix 1M context handling for the `anthropic-beta: context-1m` header (used by Claude Code): the proxy no longer blindly appends a `-1m` suffix to the model ID. It now only switches to a `-1m` variant when the base model doesn't already advertise a 1M context window and that variant actually exists upstream. Newer Claude models (e.g. `claude-sonnet-4.6`, `claude-opus-4.6/4.7/4.8`) expose 1M on the base model ID, so requesting the 1M context no longer produces a non-existent `-1m` model ID that broke capability detection and routing.

### Compatibility

- The proxy now always runs in multi-account mode: when no `accounts.json` exists at startup it is auto-created as an empty config (`{"accounts": []}`) and the admin UI is enabled out of the box. Requests must present a valid API key or receive `401 Unauthorized`; until at least one account is configured (e.g. via the admin UI), every request is rejected with `401`. This replaces the previous single-account, no-validation fallback that ran when the config file was absent.

### Docs

- Document the `/v1/messages/count_tokens` endpoint and native-passthrough fields (`context_management`, `search_result`) in both `README.md` and `README.zh-CN.md` (Features list and API Endpoints table).
- Document multi-account, admin UI, and token-usage stats in the README, and add Simplified Chinese translations (`README.zh-CN.md`, `CHANGELOG.zh-CN.md`) with language switch links.

### Tests

- Add `scripts/capability_test.py`, a dependency-free capability comparison tester that runs the same Anthropic Messages API matrix against the live GitHub Copilot upstream and a running copilot2api proxy, then emits a Markdown comparison report plus a sanitized raw-JSON sidecar. Use `--target direct|proxy|both` (with optional `--start-proxy` to auto-launch a local proxy). The matrix covers ~36 capabilities — text/streaming, function & parallel tools, `tool_choice` variants, sampling params (`temperature`/`top_p`/`top_k`/`stop_sequences`/`metadata`/`service_tier`), vision, PDF documents, extended/interleaved thinking, server tools, prompt cache (incl. 1h `extended_cache_ttl`), `context_management`, `count_tokens`, `structured_outputs`, `search_result`, citations, 1M context, and reject-cases (web search, computer use, web fetch, code execution). It pinpoints where the proxy diverges from upstream; after the fixes in this release the only remaining native-path difference is the intentional `cache_control.scope` strip, while conversion paths (`/responses`, `/chat/completions`) still drop some fields (e.g. `stop_sequences`, `disable_parallel_tool_use`). Stored tokens are never printed or written to output. See `scripts/README.md`.

## [0.3.1] - 2026-04-26

### Bug Fixes

- Fix Anthropic thinking signatures being emitted as a separate block instead of attached to the currently open thinking block
- Fix Docker image crash (`exec /copilot2api: no such file or directory`) caused by dynamically-linked binary in `scratch` image — add `CGO_ENABLED=0` to CI cross-compilation
- Fix Docker multi-arch build: arm64 image was shipping the amd64 binary due to `ARG TARGETARCH=amd64` default overriding buildx's automatic platform arg
- Fix CI triggering redundant runs on tag pushes — `on: push` now scoped to `main` branch only

### CI

- Add Docker smoke test — `docker run --version` gate before pushing to prevent broken images from reaching the registry

### Docs

- Refresh README quick start and examples

## [0.3.0] - 2026-04-03

### Features

- Add Gemini-compatible `/v1beta/models` endpoints for local `gemini-cli` usage, including `generateContent`, `streamGenerateContent`, and `countTokens`
- Expose the full upstream model list on the Gemini `/v1beta/models` surface instead of limiting the listing to a small allowlist
- Add smart fallback routing between `/v1/chat/completions` and `/v1/responses`, so requests can still work when a model only supports one of the two OpenAI-compatible endpoints
- Improve OpenAI request conversion compatibility across the two endpoints, including better handling for system instructions, structured output, tool choice, reasoning state, and `previous_response_id`
- Improve Claude Code native `/v1/messages` compatibility by removing unsupported passthrough fields before forwarding requests upstream
- Add AmpCode support: chat completions via `/amp/v1/*` and `/api/provider/*` route through Copilot API; management routes (`/api/*`) and login redirects reverse-proxy to `ampcode.com`

## [0.2.0]

### Performance

- Batch SSE flushes in Anthropic streaming — flush once per upstream event instead of per translated event (~3-5x fewer syscalls)
- Flush at SSE event boundaries in native `/v1/messages` passthrough instead of every line (~3x fewer syscalls)
- Defer model alias body re-encode to only the native passthrough path — Responses and Chat Completions paths skip the JSON round-trip entirely
- Remove unnecessary `string()` copy in `writeSSEEvent`

### Architecture

- Consolidate models cache — single upstream `/models` fetch populates both raw JSON (for proxying) and parsed model info (for capability detection), eliminating duplicate HTTP calls
- Remove dead `internal/cache` package after consolidation
- Centralize request body size limit as `upstream.MaxRequestBody` constant (was magic number `10<<20` in 3 files)
- Consistent SSE header setup via `sse.BeginSSE()` across all streaming paths

### Logging

- nginx-style single access log per request at completion with method, endpoint, model, route, duration
- Downgrade client disconnect / context cancellation errors from ERROR to WARN via `upstream.LogRequestError`
- Add `duration_ms` to token refresh logs
- Promote key request lifecycle logs to Info level (was all Debug — invisible in default mode)
- Remove noisy per-chunk/per-event debug logs from streaming hot path
- Add `route` field to Anthropic access log (`native`, `responses`, `chat_completions`)
- Add `endpoint` field to Anthropic access log for consistency with proxy handler
- Add models cache miss debug log

### Bug Fixes

- Fix split choices in OpenAI Chat Completions responses — merge text and tool_calls from separate choices into a single Anthropic message
- Fix `AnthropicContentBlockDelta` / `AnthropicMessageDelta` type confusion in streaming events
- Remove hardcoded "Thinking..." placeholder text in thinking blocks
- Request usage in streaming chunks (`stream_options.include_usage`) so `message_delta` gets real output token counts

### Features

- 1M context window support — automatically appends `-1m` suffix when `anthropic-beta: context-1m-...` header is detected
- Document 1M context window usage in README

## [0.1.0]

- Initial commit
