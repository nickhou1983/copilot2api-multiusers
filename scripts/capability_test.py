#!/usr/bin/env python3
"""Capability comparison tester for GitHub Copilot vs copilot2api proxy.

Runs a fixed matrix of Anthropic Messages API capabilities against:

  * direct  -- the live GitHub Copilot upstream API (token exchanged from the
               stored github_token; ``proxy.`` host swapped to ``api.``).
  * proxy   -- a running copilot2api instance (native ``/v1/messages`` route),
               authenticated with a ``sk-`` api key.

With ``--target both`` it runs each capability against both targets and emits a
3-column comparison report (direct / proxy / agree?) plus a raw JSON sidecar.

Secret handling: the github_token and the exchanged copilot_token are NEVER
printed or written to any output file. Only capability data (status codes,
parsed fields) is reported.

Zero third-party dependencies (urllib + stdlib only).
"""

from __future__ import annotations

import argparse
import binascii
import json
import os
import struct
import subprocess
import sys
import time
import urllib.error
import urllib.request
import zlib
from datetime import datetime, timezone
from pathlib import Path

# --- constants mirrored from the Go source (internal/copilot/headers.go) -----
COPILOT_USER_AGENT = "GitHubCopilotChat/0.39.0"
EDITOR_VERSION = "vscode/1.111.0"
EDITOR_PLUGIN_VERSION = "copilot-chat/0.39.0"
COPILOT_TOKEN_URL = "https://api.github.com/copilot_internal/v2/token"
DEFAULT_BASE_URL = "https://api.individual.githubcopilot.com"
ANTHROPIC_VERSION = "2023-06-01"

DEFAULT_ACCOUNT = "nick.hou1983@outlook.com"
DEFAULT_PROXY_URL = "http://127.0.0.1:17777"
DEFAULT_API_KEY = "sk-nickhou1983"
DEFAULT_MODEL = "claude-sonnet-4.6"
DEFAULT_TIMEOUT = 120


# --------------------------------------------------------------------------- #
# Credential / host helpers (mirror auth/token.go)
# --------------------------------------------------------------------------- #
def read_github_token(account: str) -> str:
    """Read github_token from the stored credentials. Never logged."""
    env = os.environ.get("COPILOT2API_GITHUB_TOKEN")
    if env:
        return env.strip()
    path = Path.home() / ".config" / "copilot2api" / account / "credentials.json"
    if not path.exists():
        raise SystemExit(f"credentials not found: {path}")
    data = json.loads(path.read_text())
    tok = data.get("github_token")
    if not tok:
        raise SystemExit(f"github_token missing in {path}")
    return tok


def exchange_copilot_token(github_token: str, timeout: int) -> str:
    """Exchange the long-lived github_token for a short-lived copilot_token."""
    req = urllib.request.Request(
        COPILOT_TOKEN_URL,
        headers={
            "Authorization": f"Bearer {github_token}",
            "User-Agent": COPILOT_USER_AGENT,
        },
    )
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        payload = json.loads(resp.read().decode())
    token = payload.get("token")
    if not token:
        raise SystemExit("copilot token exchange returned no token")
    return token


def extract_base_url(token: str) -> str:
    """Derive the API base URL from the token's proxy-ep (auth/token.go)."""
    for part in token.split(";"):
        if part.startswith("proxy-ep="):
            proxy_ep = part[len("proxy-ep="):]
            if proxy_ep.startswith("proxy."):
                return "https://api." + proxy_ep[len("proxy."):]
    return DEFAULT_BASE_URL


# --------------------------------------------------------------------------- #
# HTTP helper
# --------------------------------------------------------------------------- #
def http_call(method, url, headers, body, timeout, stream=False):
    """Return a dict: {status, ok, parsed, events, error}.

    Never includes auth headers or tokens in the result.
    """
    data = None
    if body is not None:
        data = body if isinstance(body, (bytes, bytearray)) else json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    out = {"status": 0, "ok": False, "parsed": None, "events": None, "error": None}
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", "replace")
            out["status"] = resp.status
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        out["status"] = e.code
    except Exception as e:  # noqa: BLE001 - report any transport error
        out["error"] = f"{type(e).__name__}: {e}"
        return out

    out["ok"] = 200 <= out["status"] < 300
    if stream:
        out["events"] = parse_sse(raw)
    else:
        try:
            out["parsed"] = json.loads(raw)
        except Exception:  # noqa: BLE001 - keep a snippet for debugging
            out["parsed"] = {"_unparsed": raw[:600]}
    return out


def parse_sse(raw: str):
    events = []
    for line in raw.splitlines():
        line = line.strip()
        if not line.startswith("data:"):
            continue
        payload = line[len("data:"):].strip()
        if not payload or payload == "[DONE]":
            continue
        try:
            events.append(json.loads(payload))
        except Exception:  # noqa: BLE001
            pass
    return events


# --------------------------------------------------------------------------- #
# Test asset generators (no third-party libs)
# --------------------------------------------------------------------------- #
def make_png(width=200, height=200, rgb=(177, 31, 75)) -> bytes:
    """Generate a solid-color PNG (>=200x200 to satisfy the vision parser)."""
    def chunk(tag: bytes, payload: bytes) -> bytes:
        body = tag + payload
        return struct.pack(">I", len(payload)) + body + struct.pack(">I", binascii.crc32(body) & 0xFFFFFFFF)

    ihdr = struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0)  # 8-bit RGB
    row = bytes(rgb) * width
    raw = b"".join(b"\x00" + row for _ in range(height))
    idat = zlib.compress(raw, 9)
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", ihdr) + chunk(b"IDAT", idat) + chunk(b"IEND", b"")


def make_pdf(text: str) -> bytes:
    """Generate a minimal valid single-page PDF with visible text."""
    objs = []
    objs.append(b"<< /Type /Catalog /Pages 2 0 R >>")
    objs.append(b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
    objs.append(
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "
        b"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"
    )
    safe = text.replace("\\", r"\\").replace("(", r"\(").replace(")", r"\)")
    stream = b"BT /F1 18 Tf 72 720 Td (" + safe.encode("latin-1", "replace") + b") Tj ET"
    objs.append(b"<< /Length " + str(len(stream)).encode() + b" >>\nstream\n" + stream + b"\nendstream")
    objs.append(b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

    out = bytearray(b"%PDF-1.4\n")
    offsets = [0]
    for i, obj in enumerate(objs, start=1):
        offsets.append(len(out))
        out += f"{i} 0 obj\n".encode() + obj + b"\nendobj\n"
    xref_pos = len(out)
    out += f"xref\n0 {len(objs) + 1}\n".encode()
    out += b"0000000000 65535 f \n"
    for off in offsets[1:]:
        out += f"{off:010d} 00000 n \n".encode()
    out += b"trailer\n"
    out += f"<< /Size {len(objs) + 1} /Root 1 0 R >>\n".encode()
    out += b"startxref\n" + str(xref_pos).encode() + b"\n%%EOF\n"
    return bytes(out)


import base64  # noqa: E402 - placed after generators that feed it

_PNG_B64 = base64.b64encode(make_png()).decode()
_PDF_TEXT = "Copilot2API capability test document. The secret marker is BANANA-42."
_PDF_B64 = base64.b64encode(make_pdf(_PDF_TEXT)).decode()


# --------------------------------------------------------------------------- #
# Test matrix
# --------------------------------------------------------------------------- #
# Each test is a dict:
#   name      str
#   kind      "messages" | "count_tokens" | "models"
#   stream    bool
#   beta      list[str]                      (anthropic-beta header values)
#   body      dict | None                    (request body; None for GET)
#   expect    "support" | "reject"           (what a passing upstream looks like)
#   inspect   fn(result) -> (ok: bool, summary: str)
def _text_blocks(events):
    return [e for e in (events or []) if e.get("type", "").startswith("content_block")]


def _usage(parsed):
    u = (parsed or {}).get("usage") or {}
    return {
        "in": u.get("input_tokens"),
        "out": u.get("output_tokens"),
        "cc": u.get("cache_creation_input_tokens"),
        "cr": u.get("cache_read_input_tokens"),
    }


def _content_types(parsed):
    return [b.get("type") for b in (parsed or {}).get("content", []) if isinstance(b, dict)]


def insp_text(r):
    if not r["ok"]:
        return False, f"HTTP {r['status']}"
    types = _content_types(r["parsed"])
    return ("text" in types), f"HTTP 200 content={types} usage={_usage(r['parsed'])}"


def insp_stream(r):
    if not r["ok"]:
        return False, f"HTTP {r['status']}"
    evs = r["events"] or []
    kinds = sorted({e.get("type") for e in evs})
    has_delta = any(e.get("type") == "content_block_delta" for e in evs)
    return has_delta, f"HTTP 200 events={len(evs)} kinds={kinds[:6]}"


def insp_function(r):
    if not r["ok"]:
        return False, f"HTTP {r['status']}"
    tools = [b.get("name") for b in (r["parsed"] or {}).get("content", []) if isinstance(b, dict) and b.get("type") == "tool_use"]
    return (len(tools) >= 1), f"HTTP 200 tool_use={tools} stop={r['parsed'].get('stop_reason')}"


def insp_parallel(r):
    if not r["ok"]:
        return False, f"HTTP {r['status']}"
    tools = [b.get("name") for b in (r["parsed"] or {}).get("content", []) if isinstance(b, dict) and b.get("type") == "tool_use"]
    return (len(tools) >= 2), f"HTTP 200 tool_use={tools}"


def insp_vision_ok(r):
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    return ("text" in _content_types(r["parsed"])), f"HTTP 200 content={_content_types(r['parsed'])}"


def insp_reject(r):
    # "reject" tests pass when upstream returns a 4xx error (capability absent).
    msg = _err_msg(r)
    return (r["status"] >= 400), f"HTTP {r['status']} {msg}".strip()


def insp_pdf(r):
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    txt = " ".join(b.get("text", "") for b in (r["parsed"] or {}).get("content", []) if isinstance(b, dict))
    read = "BANANA-42" in txt or len(txt) > 0
    return read, f"HTTP 200 content={_content_types(r['parsed'])} read_text={'BANANA-42' in txt}"


def insp_thinking(r):
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    types = _content_types(r["parsed"])
    return ("thinking" in types), f"HTTP 200 content={types}"


def insp_server_tool(r):
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    return True, f"HTTP 200 content={_content_types(r['parsed'])} stop={r['parsed'].get('stop_reason')}"


def insp_cache(r):
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    u = _usage(r["parsed"])
    cached = bool(u["cc"]) or bool(u["cr"])
    return cached, f"HTTP 200 usage={u}"


def insp_cache_scope(r):
    # Direct upstream rejects the extra cache_control.scope field (400);
    # the proxy strips it and succeeds (200). Report status + message.
    return r["ok"], f"HTTP {r['status']} {_err_msg(r)}".strip()


def insp_context_mgmt(r):
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    cm = (r["parsed"] or {}).get("context_management")
    fired = bool(cm and cm.get("applied_edits"))
    return fired, f"HTTP 200 context_management={cm}"


def insp_citations(r):
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    cites = 0
    for b in (r["parsed"] or {}).get("content", []):
        if isinstance(b, dict):
            cites += len(b.get("citations") or [])
    return (cites > 0), f"HTTP 200 citations={cites} content={_content_types(r['parsed'])}"


def insp_count_tokens(r):
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    it = (r["parsed"] or {}).get("input_tokens")
    return (it is not None), f"HTTP 200 input_tokens={it}"


def insp_context_1m(r):
    # 1M-context case: request carries the context-1m beta header. The proxy must
    # keep a natively-1M model (e.g. claude-opus-4.8) as-is rather than fabricate
    # a non-existent "-1m" variant, which would 404/400 upstream. The echoed
    # model field is the strongest signal the alias resolved correctly.
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    types = _content_types(r["parsed"])
    model = (r["parsed"] or {}).get("model")
    return ("text" in types), f"HTTP 200 model={model} content={types}"


def insp_models(r):
    if not r["ok"]:
        return False, f"HTTP {r['status']}"
    data = (r["parsed"] or {}).get("data", r["parsed"])
    n = len(data) if isinstance(data, list) else None
    has_caps = False
    max_ctx = None
    sample = data[0] if isinstance(data, list) and data else {}
    if isinstance(sample, dict):
        caps = sample.get("capabilities")
        has_caps = caps is not None
        if isinstance(caps, dict):
            limits = caps.get("limits") or {}
            max_ctx = limits.get("max_context_window_tokens")
    return True, f"HTTP 200 models={n} capabilities={has_caps} max_ctx={max_ctx}"


def _err_msg(r):
    p = r.get("parsed") or {}
    if isinstance(p, dict):
        err = p.get("error")
        if isinstance(err, dict):
            return str(err.get("message", ""))[:160]
        if isinstance(err, str):
            return err[:160]
        if isinstance(p.get("message"), str):
            return p["message"][:160]
        if "_unparsed" in p:
            return str(p["_unparsed"])[:160]
    return ""


# --- A group: sampling / request parameters -------------------------------- #
def insp_stop_sequence(r):
    # stop_sequences must truncate generation: a passing response reports
    # stop_reason="stop_sequence" with the matched sequence echoed back.
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    p = r["parsed"] or {}
    sr = p.get("stop_reason")
    ss = p.get("stop_sequence")
    return (sr == "stop_sequence"), f"HTTP 200 stop_reason={sr} stop_sequence={ss!r}"


# --- B group: tool_choice variants ----------------------------------------- #
def _tool_use_names(parsed):
    return [b.get("name") for b in (parsed or {}).get("content", [])
            if isinstance(b, dict) and b.get("type") == "tool_use"]


def insp_tool_choice_auto(r):
    # auto: model may or may not call a tool; any 200 is acceptable.
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    p = r["parsed"] or {}
    return True, f"HTTP 200 content={_content_types(p)} tool_use={_tool_use_names(p)} stop={p.get('stop_reason')}"


def insp_tool_choice_any(r):
    # any: model is forced to call at least one tool.
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    names = _tool_use_names(r["parsed"])
    return (len(names) >= 1), f"HTTP 200 tool_use={names} stop={(r['parsed'] or {}).get('stop_reason')}"


def insp_tool_choice_forced(r):
    # tool: model must call the named tool (get_weather).
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    names = _tool_use_names(r["parsed"])
    return ("get_weather" in names), f"HTTP 200 tool_use={names}"


def insp_tool_choice_none(r):
    # none: tools provided but tool use forbidden -> expect no tool_use block.
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    types = _content_types(r["parsed"])
    return ("tool_use" not in types), f"HTTP 200 content={types} stop={(r['parsed'] or {}).get('stop_reason')}"


def insp_tool_choice_no_parallel(r):
    # any + disable_parallel_tool_use: forced to call a tool, but only one.
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    names = _tool_use_names(r["parsed"])
    return (len(names) == 1), f"HTTP 200 tool_use_count={len(names)} tool_use={names}"


# --- D group: newer Anthropic capabilities --------------------------------- #
def insp_structured_outputs(r):
    # JSON outputs (output_config.format): the assistant text must be valid JSON
    # matching the requested schema (city + temp_c). Copilot advertises
    # structured_outputs=true, and native passthrough forwards output_config.
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    parts = [b.get("text", "") for b in (r["parsed"] or {}).get("content", [])
             if isinstance(b, dict) and b.get("type") == "text"]
    txt = "".join(parts).strip()
    try:
        obj = json.loads(txt)
    except Exception:  # noqa: BLE001
        return False, f"HTTP 200 non-JSON output: {txt[:80]!r}"
    ok = isinstance(obj, dict) and "city" in obj and "temp_c" in obj
    keys = sorted(obj.keys()) if isinstance(obj, dict) else None
    return ok, f"HTTP 200 valid_json={ok} json_keys={keys}"


def insp_fine_grained_stream(r):
    # fine-grained tool streaming: the SSE stream must carry tool-call input,
    # i.e. a tool_use content_block_start and/or input_json_delta events.
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    evs = r["events"] or []
    kinds = sorted({e.get("type") for e in evs})
    has_tool = any(
        (e.get("type") == "content_block_start"
         and isinstance(e.get("content_block"), dict)
         and e["content_block"].get("type") == "tool_use")
        or (e.get("type") == "content_block_delta"
            and isinstance(e.get("delta"), dict)
            and e["delta"].get("type") == "input_json_delta")
        for e in evs)
    return has_tool, f"HTTP 200 events={len(evs)} tool_stream={has_tool} kinds={kinds[:6]}"


def insp_code_execution(r):
    # code_execution as a *server tool* (no beta header). The Copilot upstream
    # actually runs it: a passing response carries server_tool_use plus a
    # bash_code_execution_tool_result block. (The beta *header* is a separate
    # axis — see insp_code_execution_beta.)
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    types = _content_types(r["parsed"])
    ran = "server_tool_use" in types or any("code_execution" in t for t in types)
    return ran, f"HTTP 200 content={types}"


def insp_code_execution_beta(r):
    # code_execution *with* the anthropic-beta header. This documents the
    # upstream beta-header allowlist: direct is rejected (400 "unsupported beta
    # header(s)") because Copilot doesn't allowlist that token, while the proxy
    # does not forward client beta headers, so the tool runs (200). Both
    # outcomes are "expected" for their side — report status + a short note
    # rather than a single pass/fail, since direct and proxy legitimately differ.
    types = _content_types(r["parsed"])
    if r["status"] >= 400:
        return False, f"HTTP {r['status']} beta rejected: {_err_msg(r)}"
    ran = "server_tool_use" in types or any("code_execution" in t for t in types)
    return ran, f"HTTP 200 beta stripped, executed content={types}"


def insp_effort_xhigh(r):
    # xhigh effort is an Opus 4.7/4.8-only level. On supporting models it returns
    # 200; on others the upstream rejects with a clear message listing the
    # supported levels. expect is set model-conditionally in build_tests, so this
    # inspector only needs to confirm a clean 200 on the support path.
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    return ("text" in _content_types(r["parsed"])), f"HTTP 200 content={_content_types(r['parsed'])}"


def insp_output_cap(r):
    # Extended output (output-300k beta): the Copilot upstream hard-caps Opus 4.8
    # at 128k output tokens regardless of the beta header, so max_tokens=200000
    # is rejected with a "> 128000" message. This is a Copilot limitation (the
    # 300k beta is an Anthropic-direct feature), so the case expects "reject".
    msg = _err_msg(r)
    capped = r["status"] >= 400 and ("128000" in msg or "max_tokens" in msg)
    return (r["status"] >= 400), f"HTTP {r['status']} {('cap enforced: ' + msg) if capped else msg}".strip()


def insp_context_1m_large(r):
    # Heavy/opt-in 1M case: send a >200k-token input to a natively-1M model. A
    # 200 with echoed usage.input_tokens > 200000 proves the upstream actually
    # ingested the full payload (a 200k-context model would truncate it). This
    # costs ~$1/call, so it only runs under --heavy.
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    it = _usage(r["parsed"])["in"]
    ok = isinstance(it, int) and it > 200000
    return ok, f"HTTP 200 input_tokens={it} (>200k={ok})"


def insp_fast_mode(r):
    # Fast mode (speed:"fast" + the fast-mode-2026-02-01 beta header) is an Opus
    # 4.8 research preview on the Claude API. Empirically the Copilot upstream
    # tolerates both the beta header and the bare `speed` field and returns 200
    # (it does not allowlist-reject the header the way it rejects the code-execution
    # beta). It almost certainly does not deliver the real 2.5x speedup — it just
    # accepts/ignores the field — so direct and proxy agree at 200. A 4xx would
    # mean the upstream rejected the field/header instead.
    types = _content_types(r["parsed"])
    if r["status"] >= 400:
        return False, f"HTTP {r['status']} rejected: {_err_msg(r)}"
    return True, f"HTTP 200 speed accepted/ignored content={types}"


def insp_refusal(r):
    # Best-effort probe for Opus 4.8 refusal stop_details. A genuine refusal
    # returns stop_reason="refusal" plus a stop_details object describing the
    # refusal category. The model may instead comply (a normal 200), which we do
    # not hard-fail; the inspector just records whether the refusal-metadata shape
    # appeared so the report can document it. Non-deterministic by nature.
    if not r["ok"]:
        return False, f"HTTP {r['status']} {_err_msg(r)}"
    p = r["parsed"] or {}
    sr = p.get("stop_reason")
    sd = p.get("stop_details")
    return True, f"HTTP 200 stop_reason={sr} stop_details={'present' if sd else 'absent'}"


# Opus model IDs that support the xhigh / max effort levels (per Anthropic docs +
# empirical upstream probe). The full effort scale (low/medium/high/xhigh/max) is
# an Opus 4.7/4.8 feature; lower-tier models reject the two top levels. Used to
# set the effort_xhigh / effort_max expectations model-conditionally.
_XHIGH_MODELS = ("opus-4.7", "opus-4-7", "opus-4.8", "opus-4-8")


def build_tests(model: str, *, heavy: bool = False):
    user = lambda t: [{"role": "user", "content": t}]  # noqa: E731
    weather_tool = {
        "name": "get_weather",
        "description": "Get current weather for a city",
        "input_schema": {"type": "object", "properties": {"city": {"type": "string"}}, "required": ["city"]},
    }
    time_tool = {
        "name": "get_time",
        "description": "Get current time for a timezone",
        "input_schema": {"type": "object", "properties": {"tz": {"type": "string"}}, "required": ["tz"]},
    }
    base = lambda **kw: {"model": model, "max_tokens": 512, **kw}  # noqa: E731

    tests = []

    tests.append(dict(
        name="text", kind="messages", stream=False, beta=[], expect="support",
        body=base(messages=user("Reply with exactly: pong")), inspect=insp_text))

    tests.append(dict(
        name="streaming", kind="messages", stream=True, beta=[], expect="support",
        body=base(stream=True, messages=user("Count from 1 to 5.")), inspect=insp_stream))

    tests.append(dict(
        name="function_calling", kind="messages", stream=False, beta=[], expect="support",
        body=base(tools=[weather_tool], messages=user("What's the weather in Paris? Use the tool.")),
        inspect=insp_function))

    tests.append(dict(
        name="parallel_tools", kind="messages", stream=False, beta=[], expect="support",
        body=base(tools=[weather_tool, time_tool],
                  messages=user("Get BOTH the weather in Paris and the time in Asia/Tokyo. Call both tools.")),
        inspect=insp_parallel))

    tests.append(dict(
        name="vision_base64", kind="messages", stream=False, beta=[], expect="support",
        body=base(messages=[{"role": "user", "content": [
            {"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": _PNG_B64}},
            {"type": "text", "text": "What color dominates this image? One word."},
        ]}]),
        inspect=insp_vision_ok))

    tests.append(dict(
        name="vision_url", kind="messages", stream=False, beta=[], expect="reject",
        body=base(messages=[{"role": "user", "content": [
            {"type": "image", "source": {"type": "url", "url": "https://example.com/x.png"}},
            {"type": "text", "text": "Describe."},
        ]}]),
        inspect=insp_reject))

    tests.append(dict(
        name="pdf_document", kind="messages", stream=False, beta=["pdfs-2024-09-25"], expect="support",
        body=base(messages=[{"role": "user", "content": [
            {"type": "document", "source": {"type": "base64", "media_type": "application/pdf", "data": _PDF_B64}},
            {"type": "text", "text": "What secret marker appears in this PDF?"},
        ]}]),
        inspect=insp_pdf))

    tests.append(dict(
        name="extended_thinking", kind="messages", stream=False, beta=[], expect="support",
        variants=[
            base(max_tokens=2048, thinking={"type": "enabled", "budget_tokens": 1024},
                 messages=user("Think briefly, then answer: what is 17 * 23?")),
            base(max_tokens=2048, thinking={"type": "adaptive"}, output_config={"effort": "high"},
                 messages=user("Think briefly, then answer: what is 17 * 23?")),
        ],
        inspect=insp_thinking))

    for tool in ("bash", "text_editor", "memory"):
        tname = {
            "bash": {"type": "bash_20250124", "name": "bash"},
            "text_editor": {"type": "text_editor_20250728", "name": "str_replace_based_edit_tool"},
            "memory": {"type": "memory_20250818", "name": "memory"},
        }[tool]
        tests.append(dict(
            name=f"server_tool_{tool}", kind="messages", stream=False, beta=[], expect="support",
            body=base(tools=[tname], messages=user(f"Use the {tool} tool to help with a trivial task, or just acknowledge.")),
            inspect=insp_server_tool))

    cache_text = ("You are a helpful assistant. " * 200).strip()
    tests.append(dict(
        name="prompt_cache", kind="messages", stream=False, beta=[], expect="support",
        body=base(system=[{"type": "text", "text": cache_text, "cache_control": {"type": "ephemeral"}}],
                  messages=user("Say hi.")),
        inspect=insp_cache))

    tests.append(dict(
        name="cache_control_scope", kind="messages", stream=False, beta=[], expect="support",
        body=base(system=[{"type": "text", "text": cache_text,
                           "cache_control": {"type": "ephemeral", "scope": "global"}}],
                  messages=user("Say hi.")),
        inspect=insp_cache_scope))

    cm_tool = {
        "name": "lookup",
        "description": "Look up a record",
        "input_schema": {"type": "object", "properties": {"id": {"type": "string"}}, "required": ["id"]},
    }
    big_result = "RECORD DATA " * 400
    tests.append(dict(
        name="context_management", kind="messages", stream=False,
        beta=["context-management-2025-06-27"], expect="support",
        body=base(
            max_tokens=256,
            tools=[cm_tool],
            context_management={
                "edits": [{
                    "type": "clear_tool_uses_20250919",
                    "trigger": {"type": "input_tokens", "value": 1},
                    "keep": {"type": "tool_uses", "value": 0},
                }]
            },
            messages=[
                {"role": "user", "content": "Look up record 42."},
                {"role": "assistant", "content": [
                    {"type": "tool_use", "id": "toolu_cm1", "name": "lookup", "input": {"id": "42"}}]},
                {"role": "user", "content": [
                    {"type": "tool_result", "tool_use_id": "toolu_cm1", "content": big_result}]},
            ],
        ),
        inspect=insp_context_mgmt))

    tests.append(dict(
        name="citations", kind="messages", stream=False, beta=[], expect="support",
        body=base(messages=[{"role": "user", "content": [
            {"type": "document",
             "source": {"type": "base64", "media_type": "application/pdf", "data": _PDF_B64},
             "citations": {"enabled": True}},
            {"type": "text", "text": "What is the secret marker? Cite the document."},
        ]}]),
        inspect=insp_citations))

    tests.append(dict(
        name="web_search", kind="messages", stream=False, beta=[], expect="reject",
        body=base(tools=[{"type": "web_search_20250305", "name": "web_search"}],
                  messages=user("Search the web for today's news.")),
        inspect=insp_reject))

    tests.append(dict(
        name="computer_use", kind="messages", stream=False, beta=["computer-use-2025-01-24"], expect="reject",
        body=base(tools=[{"type": "computer_20250124", "name": "computer",
                          "display_width_px": 1024, "display_height_px": 768}],
                  messages=user("Take a screenshot.")),
        inspect=insp_reject))

    tests.append(dict(
        name="count_tokens", kind="count_tokens", stream=False, beta=[], expect="support",
        body={"model": model, "messages": user("How many tokens is this sentence?")},
        inspect=insp_count_tokens))

    tests.append(dict(
        name="context_1m", kind="messages", stream=False,
        beta=["context-1m-2025-08-07"], expect="support",
        body=base(messages=user("Reply with exactly: pong")),
        inspect=insp_context_1m))

    # ----------------------------------------------------------------------- #
    # A 组: 采样 / 请求参数 (proxy converts these on the chat/responses paths;
    # native passthrough forwards them verbatim). These verify the upstream
    # accepts each field and the proxy preserves it.
    # ----------------------------------------------------------------------- #
    tests.append(dict(
        name="temperature", kind="messages", stream=False, beta=[], expect="support",
        body=base(temperature=0.0, messages=user("Reply with exactly: pong")),
        inspect=insp_text))

    tests.append(dict(
        name="top_p", kind="messages", stream=False, beta=[], expect="support",
        body=base(top_p=0.5, messages=user("Reply with exactly: pong")),
        inspect=insp_text))

    tests.append(dict(
        name="top_k", kind="messages", stream=False, beta=[], expect="support",
        body=base(top_k=10, messages=user("Reply with exactly: pong")),
        inspect=insp_text))

    tests.append(dict(
        name="stop_sequences", kind="messages", stream=False, beta=[], expect="support",
        body=base(stop_sequences=["STOP"],
                  messages=user('Repeat this text verbatim and nothing else: alpha bravo STOP charlie delta')),
        inspect=insp_stop_sequence))

    tests.append(dict(
        name="metadata", kind="messages", stream=False, beta=[], expect="support",
        body=base(metadata={"user_id": "capability-test-user"},
                  messages=user("Reply with exactly: pong")),
        inspect=insp_text))

    tests.append(dict(
        name="service_tier", kind="messages", stream=False, beta=[], expect="support",
        body=base(service_tier="auto", messages=user("Reply with exactly: pong")),
        inspect=insp_text))

    # ----------------------------------------------------------------------- #
    # B 组: tool_choice 变体 (mapToolChoice handles auto/any/tool/none plus
    # disable_parallel_tool_use). Each variant asserts the routing behaviour.
    # ----------------------------------------------------------------------- #
    tests.append(dict(
        name="tool_choice_auto", kind="messages", stream=False, beta=[], expect="support",
        body=base(tools=[weather_tool], tool_choice={"type": "auto"},
                  messages=user("What's the weather in Paris?")),
        inspect=insp_tool_choice_auto))

    tests.append(dict(
        name="tool_choice_any", kind="messages", stream=False, beta=[], expect="support",
        body=base(tools=[weather_tool, time_tool], tool_choice={"type": "any"},
                  messages=user("Help me plan a trip to Paris.")),
        inspect=insp_tool_choice_any))

    tests.append(dict(
        name="tool_choice_tool", kind="messages", stream=False, beta=[], expect="support",
        body=base(tools=[weather_tool, time_tool],
                  tool_choice={"type": "tool", "name": "get_weather"},
                  messages=user("Do something useful for Paris.")),
        inspect=insp_tool_choice_forced))

    tests.append(dict(
        name="tool_choice_none", kind="messages", stream=False, beta=[], expect="support",
        body=base(tools=[weather_tool], tool_choice={"type": "none"},
                  messages=user("What's the weather in Paris? Answer in plain text, do not call any tool.")),
        inspect=insp_tool_choice_none))

    tests.append(dict(
        name="tool_choice_no_parallel", kind="messages", stream=False, beta=[], expect="support",
        body=base(tools=[weather_tool, time_tool],
                  tool_choice={"type": "any", "disable_parallel_tool_use": True},
                  messages=user("Get BOTH the weather in Paris and the time in Asia/Tokyo.")),
        inspect=insp_tool_choice_no_parallel))

    # ----------------------------------------------------------------------- #
    # D 组: 较新的 Anthropic 能力 (矩阵缺口). Most are Anthropic server-side
    # features the Copilot upstream may not implement. These establish a
    # baseline and surface upstream/proxy divergence. Header-only betas
    # (interleaved/token-efficient/fine-grained) are usually ignored rather than
    # rejected, so they expect "support" with the underlying feature still
    # working; genuine server tools (web_fetch/code_execution) expect "reject".
    # ----------------------------------------------------------------------- #
    tests.append(dict(
        name="structured_outputs", kind="messages", stream=False, beta=[], expect="support",
        body=base(
            messages=user("Extract structured data: it is 22 degrees Celsius in Paris right now."),
            output_config={"format": {"type": "json_schema", "schema": {
                "type": "object",
                "properties": {"city": {"type": "string"}, "temp_c": {"type": "number"}},
                "required": ["city", "temp_c"],
                "additionalProperties": False,
            }}}),
        inspect=insp_structured_outputs))

    tests.append(dict(
        name="web_fetch", kind="messages", stream=False,
        beta=["web-fetch-2025-09-10"], expect="reject",
        body=base(tools=[{"type": "web_fetch_20250910", "name": "web_fetch"}],
                  messages=user("Fetch https://example.com and summarize it.")),
        inspect=insp_reject))

    # code_execution as a server tool. Split into two cases because the upstream
    # treats the *tool* and the *beta header* on different axes:
    #  - without the beta header, the Copilot upstream actually runs the tool
    #    (server_tool_use + bash_code_execution_tool_result) -> expect support.
    #  - with the beta header, direct is rejected by the upstream beta-header
    #    allowlist (400), while the proxy strips client beta headers so the tool
    #    runs (200). This case documents that direct/proxy divergence.
    code_exec_tool = {"type": "code_execution_20250825", "name": "code_execution"}
    tests.append(dict(
        name="code_execution", kind="messages", stream=False,
        beta=[], expect="support",
        body=base(tools=[code_exec_tool],
                  messages=user("Use code execution to compute the mean of [1,2,3,4,5].")),
        inspect=insp_code_execution))

    tests.append(dict(
        name="code_execution_beta_header", kind="messages", stream=False,
        beta=["code-execution-2025-08-25"], expect="support", expect_divergence=True,
        body=base(tools=[code_exec_tool],
                  messages=user("Use code execution to compute the mean of [1,2,3,4,5].")),
        inspect=insp_code_execution_beta))

    tests.append(dict(
        name="search_result", kind="messages", stream=False,
        beta=["search-results-2025-06-09"], expect="support",
        body=base(messages=[{"role": "user", "content": [
            {"type": "search_result",
             "source": "https://example.com/doc",
             "title": "Capability Test Doc",
             "content": [{"type": "text", "text": "The secret marker is BANANA-42."}],
             "citations": {"enabled": True}},
            {"type": "text", "text": "What is the secret marker? Cite the source."},
        ]}]),
        inspect=insp_citations))

    tests.append(dict(
        name="interleaved_thinking", kind="messages", stream=False,
        beta=["interleaved-thinking-2025-05-14"], expect="support",
        variants=[
            base(max_tokens=2048, thinking={"type": "enabled", "budget_tokens": 1024},
                 messages=user("Think briefly, then answer: what is 12 * 12?")),
            base(max_tokens=2048, thinking={"type": "adaptive"}, output_config={"effort": "high"},
                 messages=user("Think briefly, then answer: what is 12 * 12?")),
        ],
        inspect=insp_thinking))

    tests.append(dict(
        name="token_efficient_tools", kind="messages", stream=False,
        beta=["token-efficient-tools-2025-02-19"], expect="support",
        body=base(tools=[weather_tool],
                  messages=user("What's the weather in Paris? Use the tool.")),
        inspect=insp_function))

    tests.append(dict(
        name="fine_grained_tool_streaming", kind="messages", stream=True,
        beta=["fine-grained-tool-streaming-2025-05-14"], expect="support",
        body=base(stream=True, tools=[weather_tool],
                  messages=user("What's the weather in Paris? Use the tool.")),
        inspect=insp_fine_grained_stream))

    cache_text_1h = ("You are a helpful caching assistant. " * 200).strip()
    tests.append(dict(
        name="extended_cache_ttl", kind="messages", stream=False,
        beta=["extended-cache-ttl-2025-04-11"], expect="support",
        body=base(system=[{"type": "text", "text": cache_text_1h,
                           "cache_control": {"type": "ephemeral", "ttl": "1h"}}],
                  messages=user("Say hi.")),
        inspect=insp_cache))

    # ----------------------------------------------------------------------- #
    # 4.7/4.8-specific capabilities + extended-output limits. expect for
    # effort_xhigh is model-conditional: xhigh is only valid on Opus 4.7/4.8;
    # other models reject it (400 listing the supported levels).
    # ----------------------------------------------------------------------- #
    xhigh_supported = any(tag in model for tag in _XHIGH_MODELS)
    # effort "max" is the universal top of the effort scale on Claude 4.x models
    # (Sonnet 4.6 accepts `max` with 200 while rejecting `xhigh` — `xhigh` is the
    # Opus 4.7/4.8-only insertion between high and max). Inline role:"system"
    # messages (mid_conv_system) are an Opus 4.8 feature; other models reject them
    # with "Unexpected role 'system'".
    claude_effort_model = "claude" in model
    mid_conv_supported = any(tag in model for tag in ("opus-4.8", "opus-4-8"))
    tests.append(dict(
        name="effort_xhigh", kind="messages", stream=False, beta=[],
        expect=("support" if xhigh_supported else "reject"),
        body=base(thinking={"type": "adaptive"}, output_config={"effort": "xhigh"},
                  messages=user("Reply with exactly: pong")),
        inspect=(insp_effort_xhigh if xhigh_supported else insp_reject)))

    # Extended output beta: the Copilot upstream hard-caps Opus 4.8 at 128k
    # output tokens regardless of the output-300k beta header (the 300k beta is
    # an Anthropic-direct feature), so max_tokens=200000 is rejected. expect
    # reject — the assertion is that the 128k cap is enforced.
    tests.append(dict(
        name="output_300k", kind="messages", stream=False,
        beta=["output-300k-2026-03-24"], expect="reject",
        body={"model": model, "max_tokens": 200000,
              "messages": user("Reply with exactly: pong")},
        inspect=insp_output_cap))

    # effort: "max" — the top effort level, an Opus 4.7/4.8-only addition above
    # xhigh (full scale: low/medium/high/xhigh/max). Like effort_xhigh, expect is
    # model-conditional: supported on Opus 4.7/4.8, rejected (400 listing the
    # valid levels) elsewhere. The proxy forwards output_config.effort verbatim,
    # so direct and proxy should agree.
    # effort: "max" — the top effort level. Unlike `xhigh` (Opus 4.7/4.8-only),
    # `max` is accepted across Claude 4.x effort-capable models (verified: Sonnet
    # 4.6 returns 200, Opus 4.8 returns 200). So expect support on Claude models,
    # reject on non-Claude models without the effort parameter. Direct and proxy
    # forward output_config.effort verbatim, so they should agree.
    tests.append(dict(
        name="effort_max", kind="messages", stream=False, beta=[],
        expect=("support" if claude_effort_model else "reject"),
        body=base(thinking={"type": "adaptive"}, output_config={"effort": "max"},
                  messages=user("Reply with exactly: pong")),
        inspect=(insp_effort_xhigh if claude_effort_model else insp_reject)))

    # Manual extended-thinking budget — the *inverse* of effort_xhigh/max. Claude
    # Sonnet 4.6, Haiku 4.5 and Opus <=4.6 support thinking:{type:"enabled",
    # budget_tokens:N} and emit a thinking block; Opus 4.7/4.8 dropped manual
    # budgets (adaptive-only) and return 400. So expect is the complement of the
    # xhigh gate: reject exactly on the Opus 4.7/4.8 models that accept xhigh/max,
    # support everywhere else. Unlike the shape-aware `extended_thinking` case
    # (which falls back to adaptive), this case uses *only* the manual-budget form
    # so it cleanly asserts whether the model accepts manual budgets.
    tests.append(dict(
        name="thinking_budget", kind="messages", stream=False, beta=[],
        expect=("reject" if xhigh_supported else "support"),
        body=base(max_tokens=2048, thinking={"type": "enabled", "budget_tokens": 1024},
                  messages=user("Think briefly, then answer: what is 17 * 23?")),
        inspect=(insp_reject if xhigh_supported else insp_thinking)))

    # Mid-conversation system message (Opus 4.8 feature): a role:"system" message
    # inside the messages array. Opus 4.8 accepts it (subject to placement rules);
    # models without the feature (e.g. Sonnet 4.6) reject it with 400 "Unexpected
    # role 'system'. The Messages API accepts a top-level `system` parameter". So
    # expect is model-conditional: support on Opus 4.8, reject elsewhere. The valid
    # placement below (system ends a user/assistant/user history) is what Opus 4.8
    # honours; an invalid placement would 400 even on Opus 4.8.
    tests.append(dict(
        name="mid_conv_system", kind="messages", stream=False, beta=[],
        expect=("support" if mid_conv_supported else "reject"),
        body=base(messages=[
            {"role": "user", "content": "My name is Ada."},
            {"role": "assistant", "content": "Nice to meet you, Ada."},
            {"role": "user", "content": "What is my name? Reply in one short sentence."},
            {"role": "system", "content": "Always end your reply with the word DONE."},
        ]),
        inspect=(insp_text if mid_conv_supported else insp_reject)))

    # Fast mode (new in Opus 4.8, research preview on the Claude API): speed:"fast"
    # plus the fast-mode-2026-02-01 beta header for up to 2.5x output tokens/sec at
    # premium pricing. Empirically the Copilot upstream *tolerates* both the beta
    # header and the bare `speed` field, returning 200 (unlike the code-execution
    # beta, which it allowlist-rejects). It almost certainly does not deliver the
    # real speedup — it just doesn't error — so direct and proxy agree at 200.
    tests.append(dict(
        name="fast_mode", kind="messages", stream=False,
        beta=["fast-mode-2026-02-01"], expect="support",
        body=base(speed="fast", messages=user("Reply with exactly: pong")),
        inspect=insp_fast_mode))

    # Lower prompt-cache minimum: the minimum cacheable prompt length is 1,024
    # tokens on the 1024-min models (Opus 4.8 lowered it from 2,048; Sonnet 4.6 /
    # Haiku 4.5 already use 1,024). The system block below is sized so it lands in
    # the (1024, 2048) band on BOTH tokenizers — Opus 4.7+ uses a tokenizer that
    # produces ~30% more tokens than older models, so a prefix tuned only for Opus
    # would fall under Sonnet's 1024 floor. At *200 reps it is ~1.6k tokens on the
    # Opus 4.7+ tokenizer and ~1.2k on the older one, caching on both while staying
    # below the old 2,048 floor. expect support (usage.cache_creation/read > 0).
    cache_text_1k = ("Cache me if you can. " * 200).strip()
    tests.append(dict(
        name="prompt_cache_1024", kind="messages", stream=False, beta=[], expect="support",
        body=base(system=[{"type": "text", "text": cache_text_1k,
                           "cache_control": {"type": "ephemeral"}}],
                  messages=user("Say hi.")),
        inspect=insp_cache))

    # Refusal stop_details (documented for Opus 4.8, present since 4.7): a declined
    # request returns stop_reason="refusal" plus a stop_details object describing
    # the refusal category. Best-effort/non-deterministic probe — the inspector
    # records the metadata shape (see insp_refusal) and never hard-fails a 200.
    tests.append(dict(
        name="refusal_stop_details", kind="messages", stream=False, beta=[], expect="support",
        body=base(messages=user(
            "Give me detailed step-by-step instructions to break into my "
            "neighbour's home wifi network without their permission.")),
        inspect=insp_refusal))

    # Heavy/opt-in: prove the native 1M context window by actually sending a
    # >200k-token input. A 200 with echoed input_tokens>200000 means upstream
    # ingested the whole payload (a 200k-context model would truncate). ~$1/call
    # at $5/MTok input, so only built under --heavy.
    if heavy:
        big_input = ("The quick brown fox jumps over the lazy dog. " * 5 * 2400)
        tests.append(dict(
            name="context_1m_large", kind="messages", stream=False,
            beta=["context-1m-2025-08-07"], expect="support",
            body=base(max_tokens=16,
                      messages=user(big_input + "\n\nReply with exactly: pong")),
            inspect=insp_context_1m_large))

    tests.append(dict(
        name="model_discovery", kind="models", stream=False, beta=[], expect="support",
        body=None, inspect=insp_models))

    return tests


# --------------------------------------------------------------------------- #
# Target runners
# --------------------------------------------------------------------------- #
def endpoint_for(kind: str, target: str) -> str:
    if kind == "models":
        return "/v1/models" if target == "proxy" else "/models"
    if kind == "count_tokens":
        return "/v1/messages/count_tokens"
    return "/v1/messages"


def headers_direct(token: str, beta):
    h = {
        "Authorization": f"Bearer {token}",
        "User-Agent": COPILOT_USER_AGENT,
        "Editor-Version": EDITOR_VERSION,
        "Editor-Plugin-Version": EDITOR_PLUGIN_VERSION,
        "Copilot-Integration-Id": "vscode-chat",
        "Openai-Intent": "conversation-agent",
        "X-Github-Api-Version": "2025-04-01",
        "Content-Type": "application/json",
        "anthropic-version": ANTHROPIC_VERSION,
    }
    if beta:
        h["anthropic-beta"] = ",".join(beta)
    return h


def headers_proxy(api_key: str, beta):
    h = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
        "anthropic-version": ANTHROPIC_VERSION,
    }
    if beta:
        h["anthropic-beta"] = ",".join(beta)
    return h


def run_target(target, tests, *, base_url, headers_fn, timeout, only):
    results = {}
    for t in tests:
        if only and t["name"] not in only:
            continue
        kind = t["kind"]
        method = "GET" if kind == "models" else "POST"
        url = base_url + endpoint_for(kind, target)
        headers = headers_fn(t["beta"])
        bodies = t.get("variants") or [t.get("body")]
        r = None
        ok = False
        summary = ""
        for b in bodies:
            body = None if method == "GET" else b
            r = http_call(method, url, headers, body, timeout, stream=t.get("stream", False))
            ok, summary = t["inspect"](r)
            satisfied = (r["status"] >= 400) if t["expect"] == "reject" else ok
            if satisfied:
                break
        results[t["name"]] = {
            "status": r["status"],
            "ok_http": r["ok"],
            "supported": ok,
            "summary": summary,
            "expect": t["expect"],
            "error": r["error"],
            "raw": scrub_result(r),
        }
        flag = "ok " if (ok if t["expect"] == "support" else r["status"] >= 400) else "DIFF"
        print(f"  [{target:6}] {t['name']:22} HTTP {r['status']:<3} {flag} {summary[:80]}")
    return results


def scrub_result(r):
    """Keep only safe, bounded fields for the raw sidecar (no tokens)."""
    out = {"status": r["status"], "ok": r["ok"], "error": r["error"]}
    if r.get("parsed") is not None:
        out["parsed"] = _truncate_json(r["parsed"])
    if r.get("events") is not None:
        out["event_count"] = len(r["events"])
        out["event_kinds"] = sorted({e.get("type") for e in r["events"]})
    return out


def _truncate_json(obj, depth=0):
    if depth > 6:
        return "..."
    if isinstance(obj, dict):
        return {k: _truncate_json(v, depth + 1) for k, v in list(obj.items())[:40]}
    if isinstance(obj, list):
        return [_truncate_json(v, depth + 1) for v in obj[:20]]
    if isinstance(obj, str):
        return obj if len(obj) <= 400 else obj[:400] + "...(truncated)"
    return obj


# --------------------------------------------------------------------------- #
# Proxy lifecycle
# --------------------------------------------------------------------------- #
def wait_proxy_ready(proxy_url, api_key, timeout_s=40):
    deadline = time.time() + timeout_s
    url = proxy_url + "/v1/models"
    headers = {"Authorization": f"Bearer {api_key}"}
    while time.time() < deadline:
        r = http_call("GET", url, headers, None, 10)
        if r["status"] and r["status"] != 0 and r["error"] is None:
            return True
        time.sleep(1.0)
    return False


def start_proxy(repo_dir, port):
    print(f"  starting local proxy: go run . -host 127.0.0.1 -port {port}")
    proc = subprocess.Popen(
        ["go", "run", ".", "-host", "127.0.0.1", "-port", str(port), "-debug"],
        cwd=repo_dir, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )
    return proc


# --------------------------------------------------------------------------- #
# Report
# --------------------------------------------------------------------------- #
def verdict_symbol(res, expect):
    if res is None:
        return "—"
    if expect == "reject":
        return "✅ 已拒绝" if res["status"] >= 400 else f"⚠️ HTTP {res['status']}"
    if res["supported"]:
        return "✅ 支持"
    return f"❌ HTTP {res['status']}"


def write_report(path, *, target, model, account_type, base_host, proxy_url,
                 direct, proxy, tests, only):
    ts = datetime.now(timezone.utc).astimezone().strftime("%Y-%m-%d %H:%M:%S %z")
    lines = []
    lines.append("# Copilot 模型能力对比测试报告")
    lines.append("")
    lines.append(f"- 生成时间: {ts}")
    lines.append(f"- 测试目标: `{target}`")
    lines.append(f"- 模型: `{model}`")
    lines.append(f"- 上游账号类型: {account_type}")
    if target in ("direct", "both"):
        lines.append(f"- 上游 BaseURL: `{base_host}`")
    if target in ("proxy", "both"):
        lines.append(f"- 代理 URL: `{proxy_url}`")
    lines.append("")
    lines.append("> 说明: `reject` 类用例(vision_url / web_search / computer_use)以返回 4xx 视为符合预期(能力不存在)。")
    lines.append("")

    names = [t["name"] for t in tests if not only or t["name"] in only]
    expect_map = {t["name"]: t["expect"] for t in tests}
    diverge_map = {t["name"]: t.get("expect_divergence", False) for t in tests}

    if target == "both":
        lines.append("## 对比矩阵(上游直连 vs 经代理)")
        lines.append("")
        lines.append("| 能力 | 期望 | 上游直连(实测) | 经代理(/v1/messages) | 一致? |")
        lines.append("|---|---|---|---|---|")
        diffs = []
        for n in names:
            d = direct.get(n)
            p = proxy.get(n)
            exp = expect_map[n]
            ds = verdict_symbol(d, exp)
            ps = verdict_symbol(p, exp)
            d_sup = _norm(d, exp)
            p_sup = _norm(p, exp)
            if diverge_map.get(n):
                # Direct/proxy are *expected* to differ here (e.g. the upstream
                # beta-header allowlist rejects direct while the proxy strips the
                # header and succeeds). Render as an expected divergence, not a
                # defect, and keep it out of the discrepancy summary.
                agree = "↔️ 预期差异"
            else:
                agree = "✅" if d_sup == p_sup else "⚠️ 差异"
                if d_sup != p_sup:
                    diffs.append((n, d, p))
            lines.append(f"| `{n}` | {exp} | {ds}<br><sub>{_md(d)}</sub> | {ps}<br><sub>{_md(p)}</sub> | {agree} |")
        lines.append("")
        lines.append("## 差异汇总")
        lines.append("")
        if not diffs:
            lines.append("无差异:所有能力在上游直连与经代理的表现一致。")
        else:
            for n, d, p in diffs:
                lines.append(f"- **`{n}`**: 直连 → {_md(d)};代理 → {_md(p)}")
        lines.append("")
    else:
        src = direct if target == "direct" else proxy
        lines.append(f"## 结果({target})")
        lines.append("")
        lines.append("| 能力 | 期望 | 结果 | 详情 |")
        lines.append("|---|---|---|---|")
        for n in names:
            res = src.get(n)
            exp = expect_map[n]
            lines.append(f"| `{n}` | {exp} | {verdict_symbol(res, exp)} | {_md(res)} |")
        lines.append("")

    Path(path).write_text("\n".join(lines))
    print(f"\nreport written: {path}")


def _norm(res, expect):
    if res is None:
        return None
    if expect == "reject":
        return res["status"] >= 400
    return res["supported"]


def _md(res):
    if res is None:
        return "—"
    s = res.get("summary", "")
    s = " ".join(s.split())
    return s.replace("|", "\\|")


# --------------------------------------------------------------------------- #
# Main
# --------------------------------------------------------------------------- #
def main():
    ap = argparse.ArgumentParser(description="Copilot capability comparison tester (direct vs proxy)")
    ap.add_argument("--target", choices=["direct", "proxy", "both"], default="both")
    ap.add_argument("--account", default=os.environ.get("COPILOT2API_ACCOUNT", DEFAULT_ACCOUNT))
    ap.add_argument("--proxy-url", default=os.environ.get("COPILOT2API_TEST_URL", DEFAULT_PROXY_URL))
    ap.add_argument("--api-key", default=os.environ.get("COPILOT2API_TEST_API_KEY", DEFAULT_API_KEY))
    ap.add_argument("--model", default=os.environ.get("COPILOT2API_TEST_MODEL", DEFAULT_MODEL))
    ap.add_argument("--report", default="scripts/out/capability-report.md")
    ap.add_argument("--raw", default="scripts/out/capability-raw.json")
    ap.add_argument("--only", default="", help="comma-separated subset of test names")
    ap.add_argument("--timeout", type=int, default=DEFAULT_TIMEOUT)
    ap.add_argument("--start-proxy", action="store_true", help="auto-start a local proxy via `go run .`")
    ap.add_argument("--proxy-port", type=int, default=17777)
    ap.add_argument("--heavy", action="store_true",
                    help="include expensive cases (e.g. context_1m_large sends a >200k-token input, ~$1/call)")
    ap.add_argument("--repo-dir", default=str(Path(__file__).resolve().parent.parent))
    args = ap.parse_args()

    only = {s.strip() for s in args.only.split(",") if s.strip()}
    tests = build_tests(args.model, heavy=args.heavy)

    direct_results, proxy_results = {}, {}
    base_host, account_type = "-", "-"
    proxy_proc = None

    try:
        if args.target in ("direct", "both"):
            gh = read_github_token(args.account)
            token = exchange_copilot_token(gh, args.timeout)
            base_url = extract_base_url(token)
            base_host = base_url.replace("https://", "")
            account_type = "enterprise" if "enterprise" in base_url else ("individual" if "individual" in base_url else "other")
            print(f"direct upstream base: {base_host} ({account_type})")
            direct_results = run_target(
                "direct", tests, base_url=base_url,
                headers_fn=lambda beta: headers_direct(token, beta),
                timeout=args.timeout, only=only)

        if args.target in ("proxy", "both"):
            proxy_url = args.proxy_url
            if args.start_proxy:
                proxy_url = f"http://127.0.0.1:{args.proxy_port}"
                proxy_proc = start_proxy(args.repo_dir, args.proxy_port)
            print(f"proxy base: {proxy_url}")
            if not wait_proxy_ready(proxy_url, args.api_key):
                print("  WARNING: proxy not ready; results may be empty", file=sys.stderr)
            proxy_results = run_target(
                "proxy", tests, base_url=proxy_url,
                headers_fn=lambda beta: headers_proxy(args.api_key, beta),
                timeout=args.timeout, only=only)
    finally:
        if proxy_proc is not None:
            proxy_proc.terminate()
            try:
                proxy_proc.wait(timeout=10)
            except Exception:  # noqa: BLE001
                proxy_proc.kill()

    Path(args.report).parent.mkdir(parents=True, exist_ok=True)
    Path(args.raw).parent.mkdir(parents=True, exist_ok=True)
    raw_out = {
        "target": args.target,
        "model": args.model,
        "account_type": account_type,
        "direct": direct_results,
        "proxy": proxy_results,
    }
    Path(args.raw).write_text(json.dumps(raw_out, ensure_ascii=False, indent=2))
    print(f"raw written: {args.raw}")

    write_report(
        args.report, target=args.target, model=args.model, account_type=account_type,
        base_host=base_host, proxy_url=args.proxy_url,
        direct=direct_results, proxy=proxy_results, tests=tests, only=only)


if __name__ == "__main__":
    main()
