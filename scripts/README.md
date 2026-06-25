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

Environment fallbacks: `COPILOT2API_ACCOUNT`, `COPILOT2API_TEST_URL`,
`COPILOT2API_TEST_API_KEY`, `COPILOT2API_TEST_MODEL`,
`COPILOT2API_GITHUB_TOKEN`.

## Capability matrix

`text`, `streaming`, `function_calling`, `parallel_tools`, `vision_base64`,
`vision_url` (expect reject), `pdf_document`, `extended_thinking`,
`server_tool_bash` / `server_tool_text_editor` / `server_tool_memory`,
`prompt_cache`, `cache_control_scope`, `context_management`, `citations`,
`web_search` (expect reject), `computer_use` (expect reject), `count_tokens`,
`model_discovery`.

`reject` tests pass when the upstream returns a `4xx` (capability absent).

The `extended_thinking` test is shape-aware: it tries the legacy
`thinking.type=enabled` + `budget_tokens` form first, then falls back to the
newer `thinking.type=adaptive` + `output_config.effort` form (required by
`claude-opus-4.6/4.7/4.8`), so the capability is detected regardless of model.

## Output

Outputs are written under `scripts/out/` which is git-ignored — test artifacts
are not committed.
