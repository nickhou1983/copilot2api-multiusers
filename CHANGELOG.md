# Changelog

[English](CHANGELOG.md) | [简体中文](CHANGELOG.zh-CN.md)

## [Unreleased]

### Features

- Add per-account auth modes: `exchange` (default, unchanged behavior) mints short-lived Copilot tokens via `copilot_internal/v2/token`, while the new `direct` mode uses the GitHub OAuth token directly as the Copilot bearer with a static base URL (`https://api.githubcopilot.com`) and no token refresh. Set `auth_mode` per account in `accounts.json`, or a global default via the new `COPILOT2API_AUTH_MODE` environment variable (account setting wins). The GitHub Device Flow picks the client id by mode (`exchange` → `Iv1.b507a08c87ecfe98`, `direct` → `Ov23li8tweQw6odWQebz`), and outbound requests use a mode-specific header profile: `editor` (VS Code Copilot Chat, as before) for exchange, `opencode` (`User-Agent: opencode/…`, `Openai-Intent: conversation-edits`, `X-Initiator: user`, no editor/request-id headers) for direct. The admin UI and API support setting `auth_mode` when creating an account and changing it on update (which rebuilds the account's auth client). Existing configs are unaffected — an absent `auth_mode` keeps the exchange behavior.
- Send a fixed header pair on native `/v1/messages` (and `/v1/messages/count_tokens`) upstream requests, matching the opencode CLI: `anthropic-version: 2023-06-01` and `anthropic-beta: interleaved-thinking-2025-05-14`. Previously the proxy sent no `anthropic-version` header and assembled `anthropic-beta` dynamically (auto-injecting `context-management-2025-06-27` / `compact-2026-01-12` and forwarding allowlisted `computer-use-*` / `interleaved-thinking-*` client tokens); all of that is replaced by the fixed value. Client `anthropic-beta` headers are never forwarded — a `context-1m*` token is still consumed locally to select the `-1m` model variant. Note: requests relying on `computer-use-*` tool types may now be rejected upstream since their beta tokens are no longer sent.
- Auto-inject context-management betas on native `/v1/messages` (and count_tokens): when the request body carries a top-level `context_management` field, the proxy appends `context-management-2025-06-27` and `compact-2026-01-12` to the outbound `anthropic-beta` header (alongside the base `interleaved-thinking-2025-05-14`), so context editing and server-side compaction work without the client sending any beta header.

- Add API key auto-generation: creating an account without specifying `api_key` now automatically generates a cryptographically random key (`sk-` prefix + 32 base62 characters). The admin UI includes a "Generate" button next to the API Key input, and a new `GET /admin/api/generate-key` endpoint returns a freshly generated key on demand.

- Add a native Anthropic token-counting endpoint: `POST /v1/messages/count_tokens` now proxies to the upstream Copilot token counter (previously it returned `404`). The request is forwarded with the same model-alias resolution and `cache_control.scope` stripping as `/v1/messages`, and the upstream `{ "input_tokens": N }` response is returned verbatim.
- Forward `context_management` on native `/v1/messages` requests instead of stripping it: when a request body includes a `context_management` field, the proxy preserves it in the body sent upstream.
- Add multi-account support: map API keys to GitHub accounts 1:1 via an `accounts.json` config file. Each account uses an isolated credential store and its own models cache, so token refresh and capability-based routing stay per-account. Configure the file path with `COPILOT2API_ACCOUNTS_FILE` (defaults to `<token-dir>/accounts.json`).
- API keys are extracted from `Authorization: Bearer`, `x-api-key`, `x-goog-api-key`, or the `?key=` query parameter, covering OpenAI, Anthropic, and Gemini clients.
- Add a web admin UI at `/admin/` (multi-account mode only) to maintain the API key ↔ GitHub account mapping: list, add, rotate keys, and delete accounts, plus authenticate accounts via a browser-driven GitHub Device Flow. Changes are saved to `accounts.json` and applied live without a restart. Optionally protect it with `COPILOT2API_ADMIN_TOKEN` (sent as `X-Admin-Token` header or `?admin_token=`).
- Bootstrap multi-account mode from an empty `accounts.json` (`{"accounts":[]}`) and populate it entirely through the admin UI.
- Add a token-usage statistics page to the admin UI (new "Stats" tab) showing per-account, per-model token counts — input, output, cached (prompt-cache hits), cache-write, and request totals — across all OpenAI, Anthropic, and Gemini endpoints. Usage is persisted to `<token-dir>/stats.json` and survives restarts. Backed by a new `GET /admin/api/stats` endpoint, with `DELETE /admin/api/stats/{id}` to reset one account. Note: OpenAI Chat Completions streaming only contributes token counts when the client sends `stream_options.include_usage`; the request is always counted.
- Add an upstream-models page to the admin UI (new "Models" tab) listing the models the GitHub Copilot upstream advertises for a selected account — model ID, vendor, version, context window, max output tokens, supported endpoints, and preview/picker flags. Backed by a new `GET /admin/api/accounts/{id}/models` endpoint that serves the account's cached upstream `/models` response.
- Add a manual model-list update to the admin UI Models tab: a new "Update from upstream" button forces a fresh fetch of the upstream `/models` response, bypassing the cache TTL, and replaces the account's cached model list used for capability-based routing. Backed by a new `POST /admin/api/accounts/{id}/models/refresh` endpoint. The existing "Refresh" button keeps re-reading the cached list.
- Add a "Cache hit" column to the admin UI Stats tab showing the prompt-cache hit rate per model and per account total, computed as cached / (input + cached + cache write) over input-side tokens. Shows "—" when no input tokens are recorded.

### Bug Fixes

- Fix `search_result` content blocks being rejected with `400 "content must be string or array of blocks"`. Search-result blocks carry a bare string `source`, but the proxy only modeled the object image `source`, so parsing the whole content array failed before the request ever reached upstream. `AnthropicImageSource` now accepts both an object source and a bare string source, restoring native passthrough of `search_result` blocks — which the Copilot upstream supports, returning `search_result_location` citations. The Chat Completions and Responses conversion paths downgrade `search_result` blocks to plain text (preserving the content, dropping citation metadata that those APIs can't express).
- Fix 1M context handling for the `anthropic-beta: context-1m` header (used by Claude Code): the proxy no longer blindly appends a `-1m` suffix to the model ID. It now only switches to a `-1m` variant when the base model doesn't already advertise a 1M context window and that variant actually exists upstream. Newer Claude models (e.g. `claude-sonnet-4.6`, `claude-opus-4.6/4.7/4.8`) expose 1M on the base model ID, so requesting the 1M context no longer produces a non-existent `-1m` model ID that broke capability detection and routing.

### Compatibility

- Bump the `X-Github-Api-Version` header sent to the GitHub Copilot upstream from `2025-04-01` to `2026-06-01`. No client-facing API change; this only affects the version the proxy advertises upstream.
- The proxy now always runs in multi-account mode: when no `accounts.json` exists at startup it is auto-created as an empty config (`{"accounts": []}`) and the admin UI is enabled out of the box. Requests must present a valid API key or receive `401 Unauthorized`; until at least one account is configured (e.g. via the admin UI), every request is rejected with `401`. This replaces the previous single-account, no-validation fallback that ran when the config file was absent.

### Docs

- Add `docs/upstream-messages-live-tests.zh-CN.md` — a live-test record (2026-07-21/22, re-tested 07-25) of the Copilot upstream `/v1/messages` endpoint, cross-verified through the proxy and via a direct upstream connection: web_search / web_fetch / image-URL blocked at the gateway for all models (latest Anthropic tool versions included, with base64-image and ordinary-tool controls proving the blocks are targeted), structured outputs working on Anthropic- and Bedrock-routed requests but blocked by a GCP org policy on Vertex-routed ones — note the model-to-backend mapping is **not** fixed and drifts over time, so avoiding specific models is not a valid workaround (retry on the org-policy 400 instead), real `max_tokens` caps (128K / 64K, higher than the `/models`-advertised values; 300K rejected), and a ~630-second SSE stream lifetime terminated by an HTTP/2 RST_STREAM with no terminal events — with per-model throughput numbers and client-side handling advice.
- Add `docs/auth-flow.md` and `docs/auth-flow.zh-CN.md` — an end-to-end authentication-flow reference covering downstream API-key validation and the upstream GitHub Device Flow, including the mode-specific Device Flow client ids (`exchange` → `Iv1.b507a08c87ecfe98`, `direct` → `Ov23li8tweQw6odWQebz`), exchange vs direct token acquisition, the `editor`/`opencode` header profiles, token refresh, and the admin-driven login. Linked from the README "Auto Authentication" feature.
- Add `docs/copilot2api-issues-retrospective.html` — a 20-slide 16:9 HTML retrospective (same TD dossier style and navigation as the capability report) summarizing all issues tested and consulted between 2026-06-10 and 2026-07-22, organized as symptom → analysis → resolution: upstream capability gaps and the beta-header allowlist (re-tested 2026-07-26: fast mode flipped to supported — the dedicated `claude-opus-4.8-fast` model ID now returns `usage.speed: "fast"` with a measured 2.46× output speedup in both auth modes, while the official `speed` parameter + beta header remains silently ignored), HTTP 413 root cause on the Business endpoint, oversized-context behavior, thinking-signature loss on the `/chat/completions` conversion path, structured-outputs failures on the Vertex backend, token refresh and direct-token auth, SSE stream drops (linked to upstream claude-code #70017), Client-ID-gated model visibility in OpenCode-style flows (use OpenCode's Client-ID to obtain a `gho_` token for the full model list), a live `/models` probe comparing the `ghu_` (exchange) vs `gho_` (direct) model lists, the requirement to send `X-Github-Api-Version: 2026-06-01` to see the 1M context window, a live invocation test (42 calls on 2026-07-26 across `/v1/messages`, `/chat/completions`, and `/responses`) showing shared models behave identically in both auth modes while mode-exclusive models are enforced at invoke time (400) and Claude visibility is geo-filtered by the client's egress IP (without a VPN all Claude models disappear from both lists and invocations return 400, while other vendors are unaffected; behind a VPN they all return), a slide on proxy header filtering (client headers are never copied upstream — auth headers are replaced and client `anthropic-beta` tokens are never forwarded; the proxy sends a fixed `anthropic-version: 2023-06-01` + `anthropic-beta: interleaved-thinking-2025-05-14` baseline, auto-appends the `context-management` + `compact` beta pair when the body carries `context_management`, consumes `context-1m*` to switch to the `-1m` model variant, and passes the request body through byte-for-byte except for model-alias rewriting and `cache_control.scope` removal — verified 2026-07-27 with an echo-server probe), a live beta-header acceptance matrix (74 calls on 2026-07-26: the Copilot gateway is blocklist-based, the opposite of api.anthropic.com — only 5 tokens are rejected with 400 `unsupported beta header(s)` (`output-128k`, `files-api`, `mcp-client-2025-11-20`, `skills`, `advisor-tool`) while the other 31 including made-up tokens pass through, identically in both auth modes, though acceptance does not imply activation), a backend routing re-test (150 calls on 2026-08-02 in exchange mode: Google Vertex has disappeared from the Claude routing pool — 0 hits vs ~20% on 07-25 — routing is now statically pinned per model with newest-generation models on the Anthropic first-party API and previous-generation models on AWS Bedrock, and the structured-outputs failures from Issue 04 no longer reproduce: 50/50 pass), and prompt-cache hit mechanics.
- Redesign `docs/copilot-capability-report.html` from a scrolling report page into a 17-slide 16:9 HTML presentation (keyboard/wheel/touch navigation, print-to-PDF support, inline text editing); all report content is preserved.
- Document the `/v1/messages/count_tokens` endpoint and native-passthrough fields (`context_management`, `search_result`) in both `README.md` and `README.zh-CN.md` (Features list and API Endpoints table).
- Document multi-account, admin UI, and token-usage stats in the README, and add Simplified Chinese translations (`README.zh-CN.md`, `CHANGELOG.zh-CN.md`) with language switch links.
- Add `scripts/capability-request-response-guide.md` — a Chinese learning guide generated from a live full-matrix run, listing every capability case with its purpose, the exact request sent (endpoint, beta headers, JSON body) and the observed response (parsed JSON, or SSE event samples for streaming), with direct-upstream differences noted where the proxy behaves differently.

### Tests

- Add `scripts/capability_test.py`, a dependency-free capability comparison tester that runs the same Anthropic Messages API matrix against the live GitHub Copilot upstream and a running copilot2api proxy, then emits a Markdown comparison report plus a sanitized raw-JSON sidecar. Use `--target direct|proxy|both` (with optional `--start-proxy` to auto-launch a local proxy). The matrix covers ~36 capabilities — text/streaming, function & parallel tools, `tool_choice` variants, sampling params (`temperature`/`top_p`/`top_k`/`stop_sequences`/`metadata`/`service_tier`), vision, PDF documents, extended/interleaved thinking, server tools, prompt cache (incl. 1h `extended_cache_ttl`), `context_management`, `count_tokens`, `structured_outputs`, `search_result`, citations, 1M context, and reject-cases (web search, computer use, web fetch, code execution). It pinpoints where the proxy diverges from upstream; after the fixes in this release the only remaining native-path difference is the intentional `cache_control.scope` strip, while conversion paths (`/responses`, `/chat/completions`) still drop some fields (e.g. `stop_sequences`, `disable_parallel_tool_use`). Stored tokens are never printed or written to output. See `scripts/README.md`.
- Extend the capability matrix to cover the rest of the official Claude-platform feature list (12 new cases, calibrated against the live upstream). Verified as supported by the Copilot upstream: `strict_tool_use` (strict tool use, the other half of structured outputs), `tool_search` (the GA tool-search server tool with `defer_loading`), and `compaction` (via the proxy's new auto-added beta). Pinned as rejected/absent: `auto_prompt_cache` (top-level `cache_control`), `inference_geo`, `mcp_connector`, `programmatic_tool_calling`, `agent_skills`, `advisor_tool`, `server_side_fallback`, plus the standalone `batches_endpoint` (`/v1/messages/batches`) and `files_endpoint` (`/v1/files`) probes (404 on direct and proxy alike).
- Recalibrate the `code_execution` / `code_execution_beta_header` expectations to **reject**: the Copilot upstream dropped the `code_execution` server tool from its accepted tool-type list (earlier runs genuinely executed code). Both cases are kept as behavior-pinning probes and will flip to DIFF if the upstream re-enables the tool.
- The capability raw JSON sidecar now records each case's outbound request (method, endpoint, `anthropic-beta`, truncated body) and a sample of streaming SSE events, so a run can be used to study actual request/response shapes.

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
