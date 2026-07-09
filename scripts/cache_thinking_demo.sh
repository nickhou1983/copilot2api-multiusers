#!/usr/bin/env bash
#
# cache_thinking_demo.sh — Drive a 3-turn Anthropic /v1/messages conversation
# (extended thinking + tool use) against the copilot2api NATIVE route and print
# each turn's prompt-cache usage, so you can SEE how thinking blocks and
# cache_control interact in a real session.
#
# It talks to the proxy (not the upstream directly), using curl. A small python3
# helper builds each turn's body and parses the response. Thinking blocks are
# passed back VERBATIM (including their `signature`) between turns.
#
# Requires: a running copilot2api server whose model routes NATIVELY to the
# upstream /v1/messages endpoint (e.g. claude-opus-4.8). Prompt caching only
# happens on the native route.
#
# Usage:
#   scripts/cache_thinking_demo.sh
# Env overrides:
#   COPILOT2API_URL      (default: http://127.0.0.1:7777)
#   COPILOT2API_API_KEY  (default: dummy   — your proxy API key / x-api-key)
#   MODEL                (default: claude-opus-4.8)
#
set -euo pipefail

BASE_URL="${COPILOT2API_URL:-http://127.0.0.1:7777}"
API_KEY="${COPILOT2API_API_KEY:-dummy}"
MODEL="${MODEL:-claude-opus-4.8}"

command -v curl    >/dev/null || { echo "ERROR: curl not found"    >&2; exit 1; }
command -v python3 >/dev/null || { echo "ERROR: python3 not found" >&2; exit 1; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# --------------------------------------------------------------------------
# python driver: builds request bodies, parses responses.
# --------------------------------------------------------------------------
cat > "$WORK/driver.py" <<'PY'
import json, sys, os

CANNED_STATUS = ("flight CA1501: DELAYED. Scheduled arrival 19:10 local, "
                 "estimated arrival 22:40 local (about 3h30m late). Gate not yet "
                 "assigned. Inbound aircraft held at origin for weather.")

FOLLOWUP = ("请把可选的改签方案列成一个清单，并标注每个方案的额外费用与风险。")

def big_system():
    # Pad the system prompt well past the 1024-token minimum so the tools+system
    # prefix is actually eligible for caching on Opus 4.8.
    para = ("You are a meticulous corporate travel assistant. Always confirm "
            "flight numbers, timezones, and fare rules before advising. When a "
            "flight is delayed, weigh rebooking fees, layover risk, and the "
            "traveler's stated priorities. Never invent flight data; rely only "
            "on tool results. Summarize options clearly with pros and cons. ")
    out = ["CORPORATE TRAVEL DESK — POLICY MANUAL", ""]
    for i in range(1, 51):
        out.append(f"{i}. {para}")
    return "\n".join(out)

TOOLS = [{
    "name": "get_flight_status",
    "description": "Get the live status of a flight by its IATA flight number.",
    "input_schema": {
        "type": "object",
        "properties": {
            "flight": {"type": "string",
                       "description": "IATA flight number, e.g. CA1501"}
        },
        "required": ["flight"],
    },
}]

def base_req(model, messages):
    # Newer Copilot-hosted Claude models (e.g. opus-4.8) reject
    # thinking.type=enabled and require adaptive thinking + output_config.effort.
    # Set THINK_MODE=enabled to force the legacy budget form for older models.
    think_mode = os.environ.get("THINK_MODE", "adaptive")
    effort = os.environ.get("THINK_EFFORT", "high")
    if think_mode == "enabled":
        thinking = {"type": "enabled", "budget_tokens": 1500}
        output_config = None
    else:
        thinking = {"type": "adaptive"}
        output_config = {"effort": effort}
    req = {
        "model": model,
        "max_tokens": 3000,
        "thinking": thinking,
        "tools": TOOLS,
        "system": [{
            "type": "text",
            "text": big_system(),
            "cache_control": {"type": "ephemeral", "ttl": "1h"},  # breakpoint A
        }],
        "messages": messages,
    }
    if output_config is not None:
        req["output_config"] = output_config
    return req

def strip_msg_cache_control(messages):
    # Keep a single moving message breakpoint: remove cache_control from every
    # message content block, we re-add it to the newest block afterwards.
    for m in messages:
        c = m.get("content")
        if isinstance(c, list):
            for b in c:
                if isinstance(b, dict):
                    b.pop("cache_control", None)

def add_cc_last(messages):
    # breakpoint B: cache_control on the last block of the last (user) message.
    last = messages[-1]["content"]
    if isinstance(last, list) and last and isinstance(last[-1], dict):
        last[-1]["cache_control"] = {"type": "ephemeral"}

def user_text(t):
    return {"role": "user", "content": [{"type": "text", "text": t}]}

def load(p):
    with open(p) as f:
        return json.load(f)

def build1(model):
    msgs = [user_text("CA1501 今天到港了吗？如果晚点，我该不该改签今晚的转机？")]
    add_cc_last(msgs)
    print(json.dumps(base_req(model, msgs), ensure_ascii=False))

def next_messages(prev_req, prev_resp):
    msgs = prev_req["messages"]
    strip_msg_cache_control(msgs)
    # Append the assistant turn VERBATIM (preserves thinking + signature).
    assistant_content = prev_resp.get("content", [])
    msgs.append({"role": "assistant", "content": assistant_content})
    tool_uses = [b for b in assistant_content if b.get("type") == "tool_use"]
    if tool_uses:
        results = [{
            "type": "tool_result",
            "tool_use_id": tu.get("id", ""),
            "content": CANNED_STATUS,
        } for tu in tool_uses]
        msgs.append({"role": "user", "content": results})
    else:
        msgs.append(user_text(FOLLOWUP))
    return msgs

def build2(model, req1, resp1):
    msgs = next_messages(load(req1), load(resp1))
    add_cc_last(msgs)
    print(json.dumps(base_req(model, msgs), ensure_ascii=False))

def build3(model, req2, resp2):
    prev_req, prev_resp = load(req2), load(resp2)
    msgs = prev_req["messages"]
    strip_msg_cache_control(msgs)
    msgs.append({"role": "assistant", "content": prev_resp.get("content", [])})
    # A NON-tool-result user message — the cache-invalidation fork point.
    msgs.append(user_text(FOLLOWUP))
    add_cc_last(msgs)
    print(json.dumps(base_req(model, msgs), ensure_ascii=False))

def reqinfo(reqf):
    req = load(reqf)
    for m in req["messages"]:
        c = m["content"]
        parts = []
        if isinstance(c, list):
            for b in c:
                t = b.get("type", "?")
                cc = " [cache_control]" if b.get("cache_control") else ""
                parts.append(t + cc)
        else:
            parts.append("text")
        print(f"  {m['role']:9s}: " + ", ".join(parts))

def summary(respf):
    r = load(respf)
    if r.get("type") == "error" or "error" in r:
        print("  ERROR:", json.dumps(r.get("error", r), ensure_ascii=False)[:400])
        return
    print("  stop_reason:", r.get("stop_reason"))
    for b in r.get("content", []):
        t = b.get("type")
        if t == "thinking":
            sig = b.get("signature", "") or ""
            form = "has '@' (responses-route composite)" if "@" in sig else "opaque (native Anthropic signature)"
            print(f"    - thinking   sig[{len(sig)}]={sig[:24]}...  -> {form}")
        elif t == "tool_use":
            print(f"    - tool_use   {b.get('name')}({json.dumps(b.get('input', {}), ensure_ascii=False)})")
        elif t == "text":
            txt = (b.get("text") or "").replace("\n", " ")
            print(f"    - text       {txt[:70]}...")
        else:
            print(f"    - {t}")

def _usage(respf):
    u = load(respf).get("usage", {}) or {}
    return (u.get("input_tokens", 0),
            u.get("cache_creation_input_tokens", 0),
            u.get("cache_read_input_tokens", 0),
            u.get("output_tokens", 0))

def usage(respf):
    i, cw, cr, o = _usage(respf)
    total_in = i + cw + cr
    print(f"  input_tokens (after breakpoint) : {i}")
    print(f"  cache_creation_input_tokens (W) : {cw}")
    print(f"  cache_read_input_tokens     (R) : {cr}")
    print(f"  output_tokens                   : {o}")
    print(f"  => total input processed        : {total_in}")

def ledger(*resps):
    print(f"  {'turn':4s} {'read(R)':>9s} {'write(W)':>9s} {'input':>7s} {'output':>7s}")
    for n, p in enumerate(resps, 1):
        i, cw, cr, o = _usage(p)
        print(f"  {n:<4d} {cr:>9d} {cw:>9d} {i:>7d} {o:>7d}")
    print()
    print("  R grows across turns  => earlier prefix (incl. thinking blocks) is")
    print("  being read from cache. W is the new delta cached each turn.")

cmd = sys.argv[1]
{
    "build1": lambda: build1(sys.argv[2]),
    "build2": lambda: build2(sys.argv[2], sys.argv[3], sys.argv[4]),
    "build3": lambda: build3(sys.argv[2], sys.argv[3], sys.argv[4]),
    "reqinfo": lambda: reqinfo(sys.argv[2]),
    "summary": lambda: summary(sys.argv[2]),
    "usage": lambda: usage(sys.argv[2]),
    "ledger": lambda: ledger(*sys.argv[2:]),
}[cmd]()
PY

# --------------------------------------------------------------------------
# curl helper + turn runner
# --------------------------------------------------------------------------
post() {  # $1 = request json file
  curl -sS -w $'\n__HTTP__%{http_code}' -X POST "$BASE_URL/v1/messages" \
    -H "x-api-key: $API_KEY" \
    -H "anthropic-version: 2023-06-01" \
    -H "content-type: application/json" \
    -d @"$1"
}

run_turn() {  # $1 = turn number, $2 = request file
  local n="$1" reqf="$2" raw status body
  echo "================  TURN $n  ================"
  echo "--- request (roles / blocks; [cache_control] = breakpoint) ---"
  python3 "$WORK/driver.py" reqinfo "$reqf"
  raw="$(post "$reqf" || true)"
  status="${raw##*__HTTP__}"
  body="${raw%$'\n'__HTTP__*}"
  printf '%s' "$body" > "$WORK/resp${n}.json"
  echo "--- HTTP $status ---"
  echo "--- response ---"
  python3 "$WORK/driver.py" summary "$WORK/resp${n}.json"
  echo "--- usage ---"
  python3 "$WORK/driver.py" usage "$WORK/resp${n}.json"
  echo
  [ "$status" = "200" ] || { echo "Turn $n failed (HTTP $status); stopping." >&2; exit 1; }
}

echo "=================================================================="
echo " Proxy   : $BASE_URL/v1/messages"
echo " Model   : $MODEL   (must route NATIVELY for caching to apply)"
echo "=================================================================="
echo

python3 "$WORK/driver.py" build1 "$MODEL"                              > "$WORK/req1.json"
run_turn 1 "$WORK/req1.json"

python3 "$WORK/driver.py" build2 "$MODEL" "$WORK/req1.json" "$WORK/resp1.json" > "$WORK/req2.json"
run_turn 2 "$WORK/req2.json"

python3 "$WORK/driver.py" build3 "$MODEL" "$WORK/req2.json" "$WORK/resp2.json" > "$WORK/req3.json"
run_turn 3 "$WORK/req3.json"

echo "================  LEDGER  ================"
python3 "$WORK/driver.py" ledger "$WORK/resp1.json" "$WORK/resp2.json" "$WORK/resp3.json"
