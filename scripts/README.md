# Capability comparison tester

`capability_test.py` exercises a fixed matrix of Anthropic Messages API
capabilities against two targets and produces a comparison report:

- **direct** — the live GitHub Copilot upstream API. The script reads the stored
  `github_token`, exchanges it for a short-lived copilot token, and derives the
  upstream host from the token's `proxy-ep` (`proxy.* → api.*`).
- **proxy** — a running `copilot2api` instance, via its native `/v1/messages`
  route, authenticated with a `sk-` api key.

With `--target both` (default) every capability is run against both targets and
the report highlights any **差异 (discrepancies)** — e.g. the proxy stripping
`context_management`, dropping `cache_control.scope`, or returning `404` for
`/v1/messages/count_tokens`.

> **Secrets**: the `github_token` and the exchanged copilot token are never
> printed or written to any output file. Only capability data (status codes and
> parsed response fields) is reported.

## Requirements

- Python 3.8+ (stdlib only — no third-party packages).
- A configured account under `~/.config/copilot2api/<account>/credentials.json`
  for the `direct` target.
- Go toolchain if you use `--start-proxy` (runs `go run .`).

## Usage

```bash
# Compare upstream vs a locally auto-started proxy, full matrix:
python3 scripts/capability_test.py --target both --start-proxy

# Only the upstream side:
python3 scripts/capability_test.py --target direct

# Against an already-running proxy:
python3 scripts/capability_test.py --target proxy --proxy-url http://127.0.0.1:7777 --api-key sk-xxx

# A subset of capabilities:
python3 scripts/capability_test.py --only citations,count_tokens,context_management

# Include the expensive >200k-token 1M-context probe (~$1/call):
python3 scripts/capability_test.py --model claude-opus-4.8 --heavy
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--target` | `both` | `direct`, `proxy`, or `both` |
| `--account` | `nick.hou1983@outlook.com` | credentials folder name (enterprise account) |
| `--proxy-url` | `http://127.0.0.1:17777` | base URL of a running proxy |
| `--api-key` | `sk-nickhou1983` | proxy api key (`Authorization: Bearer …`) |
| `--model` | `claude-sonnet-4.6` | model id to test |
| `--report` | `scripts/out/capability-report.md` | Markdown report output |
| `--raw` | `scripts/out/capability-raw.json` | sanitized raw results sidecar |
| `--only` | _(all)_ | comma-separated subset of test names |
| `--timeout` | `120` | per-request timeout (seconds) |
| `--start-proxy` | off | auto-start a local proxy via `go run .` |
| `--proxy-port` | `17777` | port for the auto-started proxy |
| `--heavy` | off | include expensive cases (`context_1m_large` sends a >200k-token input, ~$1/call) |

Environment fallbacks: `COPILOT2API_ACCOUNT`, `COPILOT2API_TEST_URL`,
`COPILOT2API_TEST_API_KEY`, `COPILOT2API_TEST_MODEL`,
`COPILOT2API_GITHUB_TOKEN`.

## Capability matrix

Core / content / tools:

`text`, `streaming`, `function_calling`, `parallel_tools`, `vision_base64`,
`vision_url` (expect reject), `pdf_document`, `extended_thinking`,
`server_tool_bash` / `server_tool_text_editor` / `server_tool_memory`,
`prompt_cache`, `cache_control_scope`, `context_management`, `citations`,
`web_search` (expect reject), `computer_use` (expect reject), `count_tokens`,
`context_1m`, `model_discovery`.

Sampling / request parameters (group A):

`temperature`, `top_p`, `top_k`, `stop_sequences`, `metadata`, `service_tier`.

`tool_choice` variants (group B):

`tool_choice_auto`, `tool_choice_any`, `tool_choice_tool` (forced
`get_weather`), `tool_choice_none`, `tool_choice_no_parallel`
(`any` + `disable_parallel_tool_use`).

Newer Anthropic capabilities (group D):

`structured_outputs` (`output_config.format` JSON schema), `web_fetch` (expect
reject), `code_execution` / `code_execution_beta_header`, `search_result`
blocks, `interleaved_thinking`, `token_efficient_tools`,
`fine_grained_tool_streaming`, `extended_cache_ttl` (1h cache).

4.7/4.8-specific and extended-output cases:

`effort_xhigh` (the `xhigh` effort level — expect support on Opus 4.7/4.8,
reject on other models), `output_300k` (the `output-300k` extended-output beta),
and `context_1m_large` (a real >200k-token input, **`--heavy` only**).

The proxy does **not** blindly forward client `anthropic-beta` headers on the
native route: it auto-injects the `context-management` beta when the body
carries a `context_management` field, and strips every other client beta value.
So the header-only D features (`interleaved_thinking` / `token_efficient_tools`
/ `fine_grained_tool_streaming` / `extended_cache_ttl`) reach the upstream as
plain requests and succeed. `structured_outputs` works end-to-end (Copilot
advertises it and native passthrough forwards `output_config.format`).
`search_result` content blocks also work end-to-end: the Copilot upstream
returns `search_result_location` citations, and the proxy now parses their bare
string `source` (it previously rejected these blocks with `400 "content must be
string or array of blocks"` before the request reached upstream).

`code_execution` is split into two cases because the upstream treats the *tool*
and the *beta header* on different axes. **Without** the beta header the Copilot
upstream actually runs the tool (`server_tool_use` +
`bash_code_execution_tool_result`), so `code_execution` expects **support** on
both targets. **With** the `code-execution-2025-08-25` beta header,
`code_execution_beta_header` is an *expected* direct/proxy divergence: direct is
rejected by the upstream beta-header allowlist (`400 "unsupported beta
header(s)"`) while the proxy strips the header and the tool runs (`200`). The
report renders this row as `↔️ 预期差异` and excludes it from the discrepancy
summary. (`web_fetch` is still a genuine reject — Copilot rejects both its beta
header and the tool.)

`output_300k` expects **reject**: the Copilot upstream hard-caps Opus 4.8 at
128k output tokens regardless of the `output-300k` beta (that beta is an
Anthropic-direct feature), so `max_tokens=200000` returns `400 "> 128000"`.

`effort_xhigh` sets `expect` model-conditionally at build time: `xhigh` is an
Opus 4.7/4.8-only level, so it expects support on those IDs and reject (`400`
listing the supported levels) elsewhere. The proxy forwards
`output_config.effort` verbatim, so both targets agree.

`context_1m_large` (only built with `--heavy`) proves the native 1M window by
sending a >200k-token input to a 1M model; a `200` with echoed
`usage.input_tokens > 200000` means the upstream ingested the whole payload (a
200k-context model would silently truncate). It costs ~$1/call at $5/MTok input.

`reject` tests pass when the upstream returns a `4xx` (capability absent).

The `extended_thinking` test is shape-aware: it tries the legacy
`thinking.type=enabled` + `budget_tokens` form first, then falls back to the
newer `thinking.type=adaptive` + `output_config.effort` form (required by
`claude-opus-4.6/4.7/4.8`), so the capability is detected regardless of model.

### Route coverage note

Claude models advertise `/v1/messages`, so they take the **native passthrough**
route where group A/B fields are forwarded verbatim — run the matrix with the
default `--model claude-sonnet-4.6` to verify that. To also exercise the
**conversion** routes (where the proxy translates the request), point `--model`
at a model that lacks native messages support:

- `--model gpt-5.4` → `/responses` route. Observed: `top_p` is rejected with
  `400` for reasoning models (forwarded despite `temperature` being pinned to
  `1`), and `stop_sequences` is dropped (the Responses API has no stop param).
- `--model gemini-3.5-flash` → `/chat/completions` route. `temperature` /
  `top_p` / `stop_sequences` / `metadata` map across; `top_k` / `service_tier`
  are dropped; `disable_parallel_tool_use` is wired but may be ignored by the
  model.

## Output

Outputs are written under `scripts/out/` which is git-ignored — test artifacts
are not committed.
