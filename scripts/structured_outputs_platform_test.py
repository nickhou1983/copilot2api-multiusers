#!/usr/bin/env python3
"""Structured-outputs platform probe for the GitHub Copilot upstream (Claude).

Builds request bodies shaped after each of the THREE Anthropic delivery
platforms and fires them at the live Copilot upstream `/v1/messages`
(token exchanged from the stored github_token; `proxy.` host swapped to `api.`),
so you can see which platform's field conventions the Copilot Claude models
actually accept and honour.

The three platform conventions (from platform.claude.com docs):

  * Claude API (first-party): `model` in body (plain id, e.g. claude-sonnet-4.6),
    `anthropic-version: 2023-06-01` as a header, structured output via
    `output_config.format`.
  * Amazon Bedrock: same body, but `model` follows Bedrock model-id / inference
    profile conventions. Because the plain `anthropic.` prefix is rejected by
    the Copilot upstream (`model_not_supported`), the Bedrock probe now sweeps
    several realistic Bedrock naming variants (provider prefix, cross-region
    inference-profile prefix, and versioned `-v1:0` ids) to see if ANY are
    accepted.
  * Google Vertex AI: `anthropic_version: "vertex-2023-10-16"` in the BODY
    (rather than a header). On real Vertex `model` lives in the URL; since the
    Copilot upstream needs it in the body we keep it there too.

For each platform we test both structured-output MECHANISMS:

  * JSON outputs      -> `output_config.format` (json_schema)
  * Strict tool use   -> a tool with `strict: true` + forced `tool_choice`

Plus two controls:

  * deprecated `output_format` (expected: 400 "use output_config.format")
  * OpenAI-style `/chat/completions` `response_format` (expected: ignored)

Secret handling: the github_token and the exchanged copilot_token are NEVER
printed or written anywhere.

Usage:
  scripts/structured_outputs_platform_test.py
  scripts/structured_outputs_platform_test.py --account you@example.com
  scripts/structured_outputs_platform_test.py --model claude-opus-4.8 --show-body

Env overrides:
  COPILOT2API_ACCOUNT        account dir under ~/.config/copilot2api
  COPILOT2API_GITHUB_TOKEN   use this github_token directly (skips file read)
  SOTEST_MODEL               model id (plain, without anthropic. prefix)
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path

# --- constants mirrored from the Go source (internal/copilot/headers.go) -----
COPILOT_USER_AGENT = "GitHubCopilotChat/0.39.0"
EDITOR_VERSION = "vscode/1.111.0"
EDITOR_PLUGIN_VERSION = "copilot-chat/0.39.0"
COPILOT_TOKEN_URL = "https://api.github.com/copilot_internal/v2/token"
DEFAULT_BASE_URL = "https://api.individual.githubcopilot.com"
ANTHROPIC_VERSION = "2023-06-01"
VERTEX_ANTHROPIC_VERSION = "vertex-2023-10-16"

# Account is resolved from --account / COPILOT2API_ACCOUNT; when unset we fall
# back to the top-level ~/.config/copilot2api/credentials.json.
DEFAULT_ACCOUNT = ""
DEFAULT_MODEL = "claude-sonnet-4.6"
DEFAULT_TIMEOUT = 120

PROMPT = (
    "What's the weather in Paris right now? Reply with city, temp_c (a number), "
    "and a short conditions string. Keep it brief."
)

# Shared JSON Schema used by every variant. `additionalProperties:false` and
# all-required is what structured outputs expects.
SCHEMA = {
    "type": "object",
    "properties": {
        "city": {"type": "string"},
        "temp_c": {"type": "number"},
        "conditions": {"type": "string"},
    },
    "required": ["city", "temp_c", "conditions"],
    "additionalProperties": False,
}
REQUIRED_KEYS = set(SCHEMA["required"])


# --------------------------------------------------------------------------- #
# Credential / host helpers (mirror auth/token.go)
# --------------------------------------------------------------------------- #
def read_github_token(account: str) -> str:
    env = os.environ.get("COPILOT2API_GITHUB_TOKEN")
    if env:
        return env.strip()
    base = Path.home() / ".config" / "copilot2api"
    candidates = []
    if account:
        candidates.append(base / account / "credentials.json")
    candidates.append(base / "credentials.json")
    path = next((p for p in candidates if p.exists()), None)
    if path is None:
        raise SystemExit(f"credentials not found: {candidates[0]}")
    data = json.loads(path.read_text())
    tok = data.get("github_token")
    if not tok:
        raise SystemExit(f"github_token missing in {path}")
    return tok


def exchange_copilot_token(github_token: str, timeout: int) -> str:
    req = urllib.request.Request(
        COPILOT_TOKEN_URL,
        headers={
            "Authorization": f"Bearer {github_token}",
            "User-Agent": COPILOT_USER_AGENT,
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            payload = json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        raise SystemExit(
            f"copilot token exchange failed: HTTP {e.code} "
            f"(the stored github_token may be expired/revoked)"
        )
    token = payload.get("token")
    if not token:
        raise SystemExit("copilot token exchange returned no token")
    return token


def extract_base_url(token: str) -> str:
    for part in token.split(";"):
        if part.startswith("proxy-ep="):
            proxy_ep = part[len("proxy-ep="):]
            if proxy_ep.startswith("proxy."):
                return "https://api." + proxy_ep[len("proxy."):]
    return DEFAULT_BASE_URL


def copilot_headers(token: str, anthropic_version: str | None) -> dict:
    h = {
        "Authorization": f"Bearer {token}",
        "User-Agent": COPILOT_USER_AGENT,
        "Editor-Version": EDITOR_VERSION,
        "Editor-Plugin-Version": EDITOR_PLUGIN_VERSION,
        "Copilot-Integration-Id": "vscode-chat",
        "Openai-Intent": "conversation-agent",
        "Content-Type": "application/json",
        "X-Github-Api-Version": "2026-06-01",
    }
    if anthropic_version:
        h["anthropic-version"] = anthropic_version
    return h


# --------------------------------------------------------------------------- #
# HTTP helper (never returns auth headers/tokens)
# --------------------------------------------------------------------------- #
def http_call(url: str, headers: dict, body: dict, timeout: int) -> dict:
    data = json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, headers=headers, method="POST")
    out = {"status": 0, "ok": False, "parsed": None, "error": None}
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", "replace")
            out["status"] = resp.status
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        out["status"] = e.code
    except Exception as e:  # noqa: BLE001
        out["error"] = f"{type(e).__name__}: {e}"
        return out
    out["ok"] = 200 <= out["status"] < 300
    try:
        out["parsed"] = json.loads(raw)
    except Exception:  # noqa: BLE001
        out["parsed"] = {"_unparsed": raw[:600]}
    return out


# --------------------------------------------------------------------------- #
# Request body builders (per platform / mechanism)
# --------------------------------------------------------------------------- #
def body_json_outputs(model: str, vertex_version: bool = False) -> dict:
    body = {
        "model": model,
        "max_tokens": 512,
        "messages": [{"role": "user", "content": PROMPT}],
        "output_config": {"format": {"type": "json_schema", "schema": SCHEMA}},
    }
    if vertex_version:
        body["anthropic_version"] = VERTEX_ANTHROPIC_VERSION
    return body


def body_strict_tool(model: str, vertex_version: bool = False) -> dict:
    body = {
        "model": model,
        "max_tokens": 512,
        "messages": [{"role": "user", "content": PROMPT}],
        "tools": [
            {
                "name": "emit_weather",
                "description": "Emit structured weather info",
                "strict": True,
                "input_schema": SCHEMA,
            }
        ],
        "tool_choice": {"type": "tool", "name": "emit_weather"},
    }
    if vertex_version:
        body["anthropic_version"] = VERTEX_ANTHROPIC_VERSION
    return body


def body_deprecated_output_format(model: str) -> dict:
    return {
        "model": model,
        "max_tokens": 512,
        "messages": [{"role": "user", "content": PROMPT}],
        "output_format": {"type": "json_schema", "schema": SCHEMA},
    }


def body_openai_response_format(model: str) -> dict:
    return {
        "model": model,
        "max_tokens": 512,
        "messages": [{"role": "user", "content": PROMPT}],
        "response_format": {
            "type": "json_schema",
            "json_schema": {"name": "weather", "strict": True, "schema": SCHEMA},
        },
    }


# --------------------------------------------------------------------------- #
# Inspectors -> (passed: bool, note: str)
# --------------------------------------------------------------------------- #
def _err_msg(r: dict) -> str:
    p = r.get("parsed") or {}
    if isinstance(p, dict):
        err = p.get("error")
        if isinstance(err, dict):
            return str(err.get("message") or err.get("code") or err)[:160]
        if isinstance(err, str):
            return err[:160]
        if "message" in p:
            return str(p["message"])[:160]
        if "_unparsed" in p:
            return str(p["_unparsed"])[:160]
    return ""


def insp_json_outputs(r: dict) -> tuple[bool, str]:
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    p = r["parsed"] or {}
    stop = p.get("stop_reason")
    parts = [b.get("text", "") for b in p.get("content", [])
             if isinstance(b, dict) and b.get("type") == "text"]
    txt = "".join(parts).strip()
    try:
        obj = json.loads(txt)
    except Exception:  # noqa: BLE001
        return False, f"HTTP 200 non-JSON output (stop={stop}): {txt[:70]!r}"
    ok = isinstance(obj, dict) and REQUIRED_KEYS.issubset(obj.keys())
    return ok, f"HTTP 200 conforms={ok} keys={sorted(obj.keys())} stop={stop}"


def insp_strict_tool(r: dict) -> tuple[bool, str]:
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    p = r["parsed"] or {}
    stop = p.get("stop_reason")
    tool_blocks = [b for b in p.get("content", [])
                   if isinstance(b, dict) and b.get("type") == "tool_use"]
    if not tool_blocks:
        return False, f"HTTP 200 no tool_use block (stop={stop})"
    inp = tool_blocks[0].get("input") or {}
    ok = isinstance(inp, dict) and REQUIRED_KEYS.issubset(inp.keys())
    return ok, f"HTTP 200 conforms={ok} input_keys={sorted(inp.keys())} stop={stop}"


def insp_deprecated(r: dict) -> tuple[bool, str]:
    # "pass" here = upstream understood the field and told us the modern name,
    # confirming structured outputs is wired up (just under output_config).
    msg = _err_msg(r)
    if r["status"] == 400 and "output_config" in msg:
        return True, f"HTTP 400 (expected) -> {msg}"
    if r["ok"]:
        return True, "HTTP 200 (legacy output_format still accepted)"
    return False, f"HTTP {r['status']} {msg}"


def insp_openai_rf(r: dict) -> tuple[bool, str]:
    # For /chat/completions we only report whether the JSON schema was honoured.
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    p = r["parsed"] or {}
    try:
        content = p["choices"][0]["message"]["content"]
        obj = json.loads(content)
        ok = isinstance(obj, dict) and REQUIRED_KEYS.issubset(obj.keys())
        return ok, f"HTTP 200 conforms={ok} keys={sorted(obj.keys())}"
    except Exception:  # noqa: BLE001
        snippet = ""
        try:
            snippet = p["choices"][0]["message"]["content"][:70]
        except Exception:  # noqa: BLE001
            pass
        return False, f"HTTP 200 free-form/ignored: {snippet!r}"


# --------------------------------------------------------------------------- #
# Test matrix
# --------------------------------------------------------------------------- #
def bedrock_model_variants(model: str) -> list[str]:
    """Realistic Bedrock model-id / inference-profile spellings to sweep.

    Real Bedrock never uses a plain id; it uses `anthropic.<id>`, versioned
    `...-v1:0` ids, and cross-region inference profiles prefixed with a region
    group such as `us.`/`eu.`/`apac.`.
    """
    plain = model[len("anthropic."):] if model.startswith("anthropic.") else model
    dashed = plain.replace(".", "-")  # bedrock uses dashes, e.g. claude-sonnet-4-6
    return [
        f"anthropic.{plain}",             # provider prefix (original attempt)
        f"anthropic.{dashed}-v1:0",       # versioned bedrock model id
        f"us.anthropic.{plain}",          # US cross-region inference profile
        f"us.anthropic.{dashed}-v1:0",    # US inference profile, versioned
    ]


def build_matrix(model: str):
    return [
        # platform, mechanism, endpoint, anthropic_version_header, body, inspector
        ("Claude API", "json_outputs (output_config.format)", "/v1/messages",
         ANTHROPIC_VERSION, body_json_outputs(model), insp_json_outputs),
        ("Claude API", "strict tool use (strict:true)", "/v1/messages",
         ANTHROPIC_VERSION, body_strict_tool(model), insp_strict_tool),
        ("Claude API", "control: deprecated output_format", "/v1/messages",
         ANTHROPIC_VERSION, body_deprecated_output_format(model), insp_deprecated),

        # Amazon Bedrock: sweep several Bedrock-style model-id spellings.
        *[
            ("Amazon Bedrock", f"json_outputs :: model={variant}", "/v1/messages",
             ANTHROPIC_VERSION, body_json_outputs(variant), insp_json_outputs)
            for variant in bedrock_model_variants(model)
        ],

        ("Google Vertex AI", "json_outputs + anthropic_version in body", "/v1/messages",
         None, body_json_outputs(model, vertex_version=True), insp_json_outputs),
        ("Google Vertex AI", "strict tool use + anthropic_version in body", "/v1/messages",
         None, body_strict_tool(model, vertex_version=True), insp_strict_tool),

        ("OpenAI-compat", "control: /chat/completions response_format", "/chat/completions",
         None, body_openai_response_format(model), insp_openai_rf),
    ]


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--account", default=os.environ.get("COPILOT2API_ACCOUNT", DEFAULT_ACCOUNT))
    ap.add_argument("--model", default=os.environ.get("SOTEST_MODEL", DEFAULT_MODEL))
    ap.add_argument("--timeout", type=int, default=DEFAULT_TIMEOUT)
    ap.add_argument("--show-body", action="store_true", help="print each request body")
    args = ap.parse_args()

    gh = read_github_token(args.account)
    token = exchange_copilot_token(gh, args.timeout)
    base = extract_base_url(token)
    print(f"account   : {args.account}")
    print(f"base_url  : {base}")
    print(f"model     : {args.model}")
    print("=" * 100)

    matrix = build_matrix(args.model)
    results = []
    for platform, mech, endpoint, av, body, inspector in matrix:
        headers = copilot_headers(token, av)
        if args.show_body:
            print(f"\n--- {platform} :: {mech} ---")
            print(f"POST {endpoint}  (anthropic-version header={av})")
            print(json.dumps(body, indent=2))
        r = http_call(base + endpoint, headers, body, args.timeout)
        if r["error"]:
            passed, note = False, f"transport error: {r['error']}"
        else:
            passed, note = inspector(r)
        results.append((platform, mech, passed, note))

    # Report
    print("\n" + "=" * 100)
    print(f"{'PLATFORM':<18}{'MECHANISM':<48}{'RESULT':<8}NOTE")
    print("-" * 100)
    for platform, mech, passed, note in results:
        flag = "PASS" if passed else "FAIL"
        print(f"{platform:<18}{mech:<48}{flag:<8}{note}")
    print("=" * 100)

    # Summary: does each platform's field convention work on the Copilot upstream?
    # NOTE: a Vertex "yes" only means the request was not rejected. Follow-up
    # controls (2026-07-13) showed the upstream does NOT validate
    # `anthropic_version` at all (garbage values in header or body also pass),
    # so this is field tolerance, not real Vertex-convention support.
    def any_pass(name):
        return any(p and pl == name for pl, _, p, _ in results)
    print("\nSUMMARY (does the Copilot upstream accept this platform's fields?):")
    print(f"  Claude API (plain model + output_config/strict) : {'YES' if any_pass('Claude API') else 'NO'}")
    print(f"  Amazon Bedrock (anthropic. model prefix)        : {'YES' if any_pass('Amazon Bedrock') else 'NO'}")
    vertex = ("TOLERATED (anthropic_version ignored, not validated)"
              if any_pass('Google Vertex AI') else "NO")
    print(f"  Google Vertex AI (anthropic_version in body)    : {vertex}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
