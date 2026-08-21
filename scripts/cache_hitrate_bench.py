#!/usr/bin/env python3
"""
cache_hitrate_bench.py — Measure Anthropic prompt-cache hit rates through the
copilot2api proxy, testing ONLY the /v1/messages endpoint.

Metrics come straight from the response `usage` object:

    I = input_tokens                    (uncached, after the last breakpoint)
    W = cache_creation_input_tokens     (written to cache this turn)
    R = cache_read_input_tokens         (served from cache this turn)

    hit_rate       = R / (I + W + R)
    cacheable_rate = (W + R) / (I + W + R)

Only the NATIVE upstream /v1/messages route reports W. If a model reports W == 0
everywhere while still reporting R, it is very likely being served through a
converted route and its numbers are not directly comparable; the summary flags
this in the `route` column.

Usage:
    scripts/cache_hitrate_bench.py --api-key sk-... \
        --models claude-sonnet-5,claude-opus-4.8 \
        --scenarios S0,S1,S2 --repeat 3

Outputs:
    scripts/out/cache-bench/<run_id>/raw/<model>/<scenario>/<n>-<turn>.{req,resp}.json
    scripts/out/cache-bench/<run_id>/results.csv
    plus a markdown summary on stdout.
"""

import argparse
import csv
import json
import os
import random
import statistics
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
OUT_ROOT = REPO_ROOT / "scripts" / "out" / "cache-bench"

ALL_SCENARIOS = ["S0", "S1", "S1S", "S2", "S3", "S4", "S5", "S6"]

# Scenarios that are single-pass by design (a scan or a wall-clock TTL probe).
SINGLE_PASS = {"S4", "S5"}

PARAGRAPH = (
    "You are a meticulous corporate travel assistant. Always confirm flight "
    "numbers, timezones, and fare rules before advising. When a flight is "
    "delayed, weigh rebooking fees, layover risk, and the traveler's stated "
    "priorities. Never invent flight data; rely only on tool results. "
    "Summarize options clearly with pros and cons. "
)

TOOLS = [
    {
        "name": "get_flight_status",
        "description": "Get the live status of a flight by its IATA flight number.",
        "input_schema": {
            "type": "object",
            "properties": {
                "flight": {
                    "type": "string",
                    "description": "IATA flight number, e.g. CA1501",
                }
            },
            "required": ["flight"],
        },
    }
]

CANNED_STATUS = (
    "flight CA1501: DELAYED. Scheduled arrival 19:10 local, estimated arrival "
    "22:40 local (about 3h30m late). Gate not yet assigned. Inbound aircraft "
    "held at origin for weather."
)


# ---------------------------------------------------------------------------
# HTTP
# ---------------------------------------------------------------------------


class Client:
    def __init__(self, base_url, api_key, timeout=180):
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.timeout = timeout

    def _headers(self):
        return {
            "x-api-key": self.api_key,
            "anthropic-version": "2023-06-01",
            "content-type": "application/json",
        }

    def post(self, path, body):
        """POST JSON, return (status, parsed_or_raw, latency_ms)."""
        data = json.dumps(body, ensure_ascii=False).encode("utf-8")
        req = urllib.request.Request(
            self.base_url + path, data=data, headers=self._headers(), method="POST"
        )
        start = time.monotonic()
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                raw = resp.read().decode("utf-8", "replace")
                status = resp.status
        except urllib.error.HTTPError as e:
            raw = e.read().decode("utf-8", "replace")
            status = e.code
        except Exception as e:  # network-level failure
            return 0, {"error": {"type": "transport", "message": str(e)}}, int(
                (time.monotonic() - start) * 1000
            )
        latency = int((time.monotonic() - start) * 1000)
        try:
            return status, json.loads(raw), latency
        except json.JSONDecodeError:
            return status, {"error": {"type": "non_json", "message": raw[:2000]}}, latency

    def post_stream(self, path, body):
        """POST an SSE request; reassemble usage from message_start/message_delta."""
        body = dict(body)
        body["stream"] = True
        data = json.dumps(body, ensure_ascii=False).encode("utf-8")
        headers = self._headers()
        headers["accept"] = "text/event-stream"
        req = urllib.request.Request(
            self.base_url + path, data=data, headers=headers, method="POST"
        )
        start = time.monotonic()
        usage = {}
        stop_reason = None
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                status = resp.status
                for line in resp:
                    line = line.decode("utf-8", "replace").strip()
                    if not line.startswith("data:"):
                        continue
                    payload = line[5:].strip()
                    if not payload or payload == "[DONE]":
                        continue
                    try:
                        ev = json.loads(payload)
                    except json.JSONDecodeError:
                        continue
                    if ev.get("type") == "message_start":
                        usage.update((ev.get("message") or {}).get("usage") or {})
                    elif ev.get("type") == "message_delta":
                        usage.update(ev.get("usage") or {})
                        stop_reason = (ev.get("delta") or {}).get(
                            "stop_reason", stop_reason
                        )
        except urllib.error.HTTPError as e:
            raw = e.read().decode("utf-8", "replace")
            return e.code, {"error": {"type": "http", "message": raw[:2000]}}, int(
                (time.monotonic() - start) * 1000
            )
        except Exception as e:
            return 0, {"error": {"type": "transport", "message": str(e)}}, int(
                (time.monotonic() - start) * 1000
            )
        latency = int((time.monotonic() - start) * 1000)
        return status, {"usage": usage, "stop_reason": stop_reason, "content": []}, latency

    def count_tokens(self, model, body):
        payload = {k: v for k, v in body.items() if k in ("system", "messages", "tools")}
        payload["model"] = model
        status, parsed, _ = self.post("/v1/messages/count_tokens", payload)
        if status == 200 and isinstance(parsed, dict):
            return parsed.get("input_tokens")
        return None


# ---------------------------------------------------------------------------
# Request construction
# ---------------------------------------------------------------------------


def make_prefix(salt, paragraphs):
    """Deterministic system prefix. `salt` makes each (scenario,repeat) unique so
    a later repeat never accidentally hits an earlier repeat's cache entry."""
    lines = [f"CORPORATE TRAVEL DESK — POLICY MANUAL [{salt}]", ""]
    for i in range(1, paragraphs + 1):
        lines.append(f"{i}. {PARAGRAPH}")
    return "\n".join(lines)


def user_text(t):
    return {"role": "user", "content": [{"type": "text", "text": t}]}


def build_request(
    model,
    prefix,
    messages,
    *,
    cache_control=True,
    ttl=None,
    thinking_mode=None,
    tools=False,
    max_tokens=512,
):
    block = {"type": "text", "text": prefix}
    if cache_control:
        cc = {"type": "ephemeral"}
        if ttl:
            cc["ttl"] = ttl
        block["cache_control"] = cc

    req = {
        "model": model,
        "max_tokens": max_tokens,
        "temperature": 0,
        "system": [block],
        "messages": messages,
        "stream": False,
    }
    if tools:
        req["tools"] = TOOLS
    if thinking_mode == "adaptive":
        req["thinking"] = {"type": "adaptive"}
        req["output_config"] = {"effort": "low"}
        req.pop("temperature", None)  # thinking models reject non-default temperature
    elif thinking_mode == "enabled":
        req["thinking"] = {"type": "enabled", "budget_tokens": 1024}
        req["max_tokens"] = max(max_tokens, 2048)
        req.pop("temperature", None)
    return req


def usage_of(resp):
    u = (resp or {}).get("usage") or {}
    return (
        int(u.get("input_tokens") or 0),
        int(u.get("cache_creation_input_tokens") or 0),
        int(u.get("cache_read_input_tokens") or 0),
        int(u.get("output_tokens") or 0),
    )


def rates(i, w, r):
    total = i + w + r
    if total <= 0:
        return 0.0, 0.0
    return r / total, (w + r) / total


# ---------------------------------------------------------------------------
# Recorder
# ---------------------------------------------------------------------------


class Recorder:
    def __init__(self, run_dir):
        self.run_dir = run_dir
        self.rows = []
        (run_dir / "raw").mkdir(parents=True, exist_ok=True)

    def record(self, model, scenario, run, turn, label, req, status, resp, latency):
        d = self.run_dir / "raw" / model / scenario
        d.mkdir(parents=True, exist_ok=True)
        stem = f"{run}-{turn}-{label}"
        (d / f"{stem}.req.json").write_text(
            json.dumps(req, ensure_ascii=False, indent=2), encoding="utf-8"
        )
        (d / f"{stem}.resp.json").write_text(
            json.dumps(resp, ensure_ascii=False, indent=2), encoding="utf-8"
        )
        i, w, r, o = usage_of(resp)
        hit, cacheable = rates(i, w, r)
        err = ""
        if status != 200:
            err = json.dumps((resp or {}).get("error", resp), ensure_ascii=False)[:300]
        row = {
            "model": model,
            "scenario": scenario,
            "run": run,
            "turn": turn,
            "label": label,
            "http_status": status,
            "input_tokens": i,
            "cache_write": w,
            "cache_read": r,
            "output_tokens": o,
            "total_input": i + w + r,
            "hit_rate": round(hit, 4),
            "cacheable_rate": round(cacheable, 4),
            "latency_ms": latency,
            "error": err,
        }
        self.rows.append(row)
        return row

    def flush(self):
        path = self.run_dir / "results.csv"
        if not self.rows:
            return path
        with path.open("w", newline="", encoding="utf-8") as f:
            wtr = csv.DictWriter(f, fieldnames=list(self.rows[0].keys()))
            wtr.writeheader()
            wtr.writerows(self.rows)
        return path


# ---------------------------------------------------------------------------
# Probing
# ---------------------------------------------------------------------------


def probe_thinking_mode(client, model, log):
    """Return 'adaptive', 'enabled', or None (thinking unsupported/erroring)."""
    prefix = make_prefix("probe", 4)
    msgs = [user_text("reply with the single word: ok")]
    for mode in ("adaptive", "enabled"):
        req = build_request(model, prefix, msgs, thinking_mode=mode, max_tokens=2048)
        status, resp, _ = client.post("/v1/messages", req)
        if status == 200:
            log(f"    thinking mode for {model}: {mode}")
            return mode
        log(f"    thinking mode {mode} rejected ({status}) for {model}")
    return None


# ---------------------------------------------------------------------------
# Scenarios
# ---------------------------------------------------------------------------


def run_S0(ctx, model, run):
    """Baseline: no cache_control at all. Expect R == 0."""
    prefix = make_prefix(f"{ctx.run_id}-S0-{model}-{run}", 60)
    req = build_request(
        model, prefix, [user_text("Summarize the policy in one sentence.")],
        cache_control=False,
    )
    status, resp, lat = ctx.client.post("/v1/messages", req)
    ctx.rec.record(model, "S0", run, 1, "nocc", req, status, resp, lat)


def run_S1(ctx, model, run, stream=False):
    """Identical prefix sent twice. Second call should read the whole prefix."""
    scen = "S1S" if stream else "S1"
    prefix = make_prefix(f"{ctx.run_id}-{scen}-{model}-{run}", 60)
    msgs = [user_text("Summarize the policy in one sentence.")]
    req = build_request(model, prefix, msgs)
    poster = ctx.client.post_stream if stream else ctx.client.post
    status, resp, lat = poster("/v1/messages", req)
    ctx.rec.record(model, scen, run, 1, "prime", req, status, resp, lat)
    time.sleep(ctx.prime_gap)
    status, resp, lat = poster("/v1/messages", req)
    ctx.rec.record(model, scen, run, 2, "measure", req, status, resp, lat)


def run_S2(ctx, model, run):
    """3-turn appending conversation with a moving message breakpoint."""
    prefix = make_prefix(f"{ctx.run_id}-S2-{model}-{run}", 60)
    msgs = [user_text("Summarize the policy in one sentence.")]
    for turn in range(1, 4):
        # moving breakpoint on the last block of the last message
        for m in msgs:
            for b in m["content"]:
                b.pop("cache_control", None)
        msgs[-1]["content"][-1]["cache_control"] = {"type": "ephemeral"}
        req = build_request(model, prefix, json.loads(json.dumps(msgs)))
        status, resp, lat = ctx.client.post("/v1/messages", req)
        ctx.rec.record(model, "S2", run, turn, f"turn{turn}", req, status, resp, lat)
        if status != 200:
            return
        content = resp.get("content") or []
        msgs.append({"role": "assistant", "content": content})
        msgs.append(user_text(f"Now list {turn} additional risk factors."))
        time.sleep(ctx.turn_gap)


def run_S3(ctx, model, run):
    """Prime a prefix, then mutate its tail. Cache should be invalidated."""
    base = make_prefix(f"{ctx.run_id}-S3-{model}-{run}", 60)
    msgs = [user_text("Summarize the policy in one sentence.")]
    req = build_request(model, base, msgs)
    status, resp, lat = ctx.client.post("/v1/messages", req)
    ctx.rec.record(model, "S3", run, 1, "prime", req, status, resp, lat)
    time.sleep(ctx.prime_gap)
    mutated = base[:-1] + ("Z" if base[-1] != "Z" else "Y")
    req2 = build_request(model, mutated, msgs)
    status, resp, lat = ctx.client.post("/v1/messages", req2)
    ctx.rec.record(model, "S3", run, 2, "mutated", req2, status, resp, lat)


def run_S4(ctx, model, run):
    """Prefix-length scan to find the real minimum cacheable prefix."""
    for paragraphs in ctx.s4_sizes:
        prefix = make_prefix(f"{ctx.run_id}-S4-{model}-{run}-{paragraphs}", paragraphs)
        msgs = [user_text("Reply with the single word: ok.")]
        req = build_request(model, prefix, msgs, max_tokens=64)
        ntok = ctx.client.count_tokens(model, req)
        status, resp, lat = ctx.client.post("/v1/messages", req)
        ctx.rec.record(
            model, "S4", run, 1, f"p{paragraphs}_tok{ntok}", req, status, resp, lat
        )
        time.sleep(ctx.prime_gap)
        status, resp, lat = ctx.client.post("/v1/messages", req)
        ctx.rec.record(
            model, "S4", run, 2, f"p{paragraphs}_tok{ntok}", req, status, resp, lat
        )
        time.sleep(ctx.group_gap)


def s5_prime(ctx, model):
    """S5 phase 1: prime one default-TTL and one 1h-TTL prefix per model."""
    out = {}
    for label, ttl in (("ttl5m", None), ("ttl1h", "1h")):
        prefix = make_prefix(f"{ctx.run_id}-S5-{model}-{label}", 60)
        msgs = [user_text("Summarize the policy in one sentence.")]
        req = build_request(model, prefix, msgs, ttl=ttl)
        status, resp, lat = ctx.client.post("/v1/messages", req)
        ctx.rec.record(model, "S5", 1, 1, f"{label}_prime", req, status, resp, lat)
        out[label] = req
        time.sleep(ctx.turn_gap)
    return out


def s5_measure(ctx, model, primed):
    for label, req in primed.items():
        status, resp, lat = ctx.client.post("/v1/messages", req)
        ctx.rec.record(model, "S5", 1, 2, f"{label}_measure", req, status, resp, lat)
        time.sleep(ctx.turn_gap)


def run_S6(ctx, model, run):
    """tools + thinking, replaying assistant blocks verbatim (signature intact)."""
    mode = ctx.thinking[model]
    if mode is None:
        return
    prefix = make_prefix(f"{ctx.run_id}-S6-{model}-{run}", 60)
    msgs = [user_text("CA1501 今天到港了吗？如果晚点，我该不该改签今晚的转机？")]
    for turn in range(1, 4):
        for m in msgs:
            for b in m["content"]:
                b.pop("cache_control", None)
        msgs[-1]["content"][-1]["cache_control"] = {"type": "ephemeral"}
        req = build_request(
            model,
            prefix,
            json.loads(json.dumps(msgs)),
            thinking_mode=mode,
            tools=True,
            max_tokens=3000,
        )
        status, resp, lat = ctx.client.post("/v1/messages", req)
        ctx.rec.record(model, "S6", run, turn, f"turn{turn}", req, status, resp, lat)
        if status != 200:
            return
        content = resp.get("content") or []
        msgs.append({"role": "assistant", "content": content})
        tool_uses = [b for b in content if b.get("type") == "tool_use"]
        if tool_uses:
            msgs.append(
                {
                    "role": "user",
                    "content": [
                        {
                            "type": "tool_result",
                            "tool_use_id": tu.get("id", ""),
                            "content": CANNED_STATUS,
                        }
                        for tu in tool_uses
                    ],
                }
            )
        else:
            msgs.append(user_text("请把可选的改签方案列成清单，标注费用与风险。"))
        time.sleep(ctx.turn_gap)


# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------


def infer_route(rows, model):
    mine = [r for r in rows if r["model"] == model and r["http_status"] == 200]
    if not mine:
        return "unknown"
    if any(r["cache_write"] > 0 for r in mine):
        return "native"
    if any(r["cache_read"] > 0 for r in mine):
        return "converted?"
    return "no-cache"


def med(vals):
    return statistics.median(vals) if vals else 0.0


def summarize(rows, models, scenarios):
    out = []
    out.append("## Route detection\n")
    out.append("| model | route | 200/total |")
    out.append("|---|---|---|")
    for m in models:
        mine = [r for r in rows if r["model"] == m]
        ok = sum(1 for r in mine if r["http_status"] == 200)
        out.append(f"| {m} | {infer_route(rows, m)} | {ok}/{len(mine)} |")

    out.append("\n## Hit rate on the measurement turn\n")
    out.append(
        "| model | scenario | turn/label | n | median hit_rate | median R | median W | median I | median latency ms |"
    )
    out.append("|---|---|---|---|---|---|---|---|---|")
    for m in models:
        for s in scenarios:
            if s == "S5":
                # S5 mixes two distinct sub-groups (default ~5m TTL vs explicit
                # 1h TTL) under the same scenario/turn; grouping by turn alone
                # averages away the very difference the scenario is meant to
                # show. Group by label instead so each TTL variant gets its
                # own row.
                keys = sorted(
                    {r["label"] for r in rows if r["model"] == m and r["scenario"] == s}
                )
                key_field = "label"
            else:
                keys = sorted(
                    {r["turn"] for r in rows if r["model"] == m and r["scenario"] == s}
                )
                key_field = "turn"
            for k in keys:
                grp = [
                    r
                    for r in rows
                    if r["model"] == m
                    and r["scenario"] == s
                    and r[key_field] == k
                    and r["http_status"] == 200
                ]
                if not grp:
                    continue
                label = k if key_field == "label" else f"turn {k}"
                out.append(
                    f"| {m} | {s} | {label} | {len(grp)} | "
                    f"{med([r['hit_rate'] for r in grp]):.3f} | "
                    f"{med([r['cache_read'] for r in grp]):.0f} | "
                    f"{med([r['cache_write'] for r in grp]):.0f} | "
                    f"{med([r['input_tokens'] for r in grp]):.0f} | "
                    f"{med([r['latency_ms'] for r in grp]):.0f} |"
                )

    s4 = [r for r in rows if r["scenario"] == "S4" and r["http_status"] == 200]
    if s4:
        out.append("\n## S4 — minimum cacheable prefix\n")
        out.append("| model | prefix label | prime W | measure R | cached? |")
        out.append("|---|---|---|---|---|")
        for m in models:
            labels = sorted(
                {r["label"] for r in s4 if r["model"] == m},
                key=lambda x: int(x.split("_")[0][1:]),
            )
            for lb in labels:
                pr = [r for r in s4 if r["model"] == m and r["label"] == lb and r["turn"] == 1]
                me = [r for r in s4 if r["model"] == m and r["label"] == lb and r["turn"] == 2]
                if not pr or not me:
                    continue
                w, r_ = pr[0]["cache_write"], me[0]["cache_read"]
                out.append(f"| {m} | {lb} | {w} | {r_} | {'YES' if r_ > 0 else 'no'} |")

    errs = [r for r in rows if r["http_status"] != 200]
    if errs:
        out.append("\n## Errors\n")
        out.append("| model | scenario | turn | status | error |")
        out.append("|---|---|---|---|---|")
        seen = set()
        for r in errs:
            k = (r["model"], r["scenario"], r["http_status"], r["error"][:80])
            if k in seen:
                continue
            seen.add(k)
            out.append(
                f"| {r['model']} | {r['scenario']} | {r['turn']} | {r['http_status']} | "
                f"`{r['error'][:120]}` |"
            )
    return "\n".join(out)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


class Ctx:
    pass


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base-url", default=os.environ.get("COPILOT2API_URL", "http://127.0.0.1:7777"))
    ap.add_argument("--api-key", default=os.environ.get("COPILOT2API_API_KEY", ""))
    ap.add_argument("--models", default="claude-sonnet-5,claude-sonnet-4.6,claude-opus-4.8,claude-opus-5")
    ap.add_argument("--scenarios", default="S0,S1,S1S,S2,S3,S4,S5,S6")
    ap.add_argument("--repeat", type=int, default=3)
    ap.add_argument("--prime-gap", type=float, default=8.0, help="seconds between prime and measure")
    ap.add_argument("--turn-gap", type=float, default=3.0, help="seconds between conversation turns")
    ap.add_argument("--group-gap", type=float, default=5.0, help="seconds between scenario groups")
    ap.add_argument("--ttl-wait", type=float, default=380.0, help="S5 wall-clock wait in seconds")
    ap.add_argument("--s4-sizes", default="8,14,28,110", help="paragraph counts for the prefix scan")
    ap.add_argument("--run-id", default=None)
    args = ap.parse_args()

    if not args.api_key:
        print("ERROR: --api-key (or COPILOT2API_API_KEY) is required", file=sys.stderr)
        return 2

    models = [m.strip() for m in args.models.split(",") if m.strip()]
    scenarios = [s.strip() for s in args.scenarios.split(",") if s.strip()]
    bad = [s for s in scenarios if s not in ALL_SCENARIOS]
    if bad:
        print(f"ERROR: unknown scenarios {bad}; valid: {ALL_SCENARIOS}", file=sys.stderr)
        return 2

    run_id = args.run_id or time.strftime("%Y%m%d-%H%M%S")
    run_dir = OUT_ROOT / run_id
    run_dir.mkdir(parents=True, exist_ok=True)

    def log(msg):
        print(msg, flush=True)

    ctx = Ctx()
    ctx.client = Client(args.base_url, args.api_key)
    ctx.rec = Recorder(run_dir)
    ctx.run_id = run_id
    ctx.prime_gap = args.prime_gap
    ctx.turn_gap = args.turn_gap
    ctx.group_gap = args.group_gap
    ctx.s4_sizes = [int(x) for x in args.s4_sizes.split(",") if x.strip()]

    log(f"run_id={run_id}  base={args.base_url}")
    log(f"models={models}  scenarios={scenarios}  repeat={args.repeat}")

    log("\n[probe] detecting thinking mode per model")
    ctx.thinking = {}
    for m in models:
        ctx.thinking[m] = probe_thinking_mode(ctx.client, m, log) if "S6" in scenarios else None
        time.sleep(1)

    dispatch = {
        "S0": run_S0,
        "S1": lambda c, m, r: run_S1(c, m, r, stream=False),
        "S1S": lambda c, m, r: run_S1(c, m, r, stream=True),
        "S2": run_S2,
        "S3": run_S3,
        "S4": run_S4,
        "S6": run_S6,
    }

    # Phase A: everything except the wall-clock TTL probe.
    jobs = []
    for s in scenarios:
        if s == "S5":
            continue
        reps = 1 if s in SINGLE_PASS else args.repeat
        for run in range(1, reps + 1):
            for m in models:
                jobs.append((s, run, m))
    random.shuffle(jobs)

    for idx, (s, run, m) in enumerate(jobs, 1):
        log(f"[{idx}/{len(jobs)}] {m} {s} run{run}")
        try:
            dispatch[s](ctx, m, run)
        except Exception as e:
            log(f"    !! {type(e).__name__}: {e}")
        ctx.rec.flush()
        time.sleep(ctx.group_gap)

    # Phase B: S5 — prime every model, wait once, re-measure every model.
    if "S5" in scenarios:
        log(f"\n[S5] priming all models, then waiting {args.ttl_wait}s once")
        primed = {}
        for m in models:
            primed[m] = s5_prime(ctx, m)
        ctx.rec.flush()
        time.sleep(args.ttl_wait)
        for m in models:
            s5_measure(ctx, m, primed[m])
        ctx.rec.flush()

    csv_path = ctx.rec.flush()
    report = summarize(ctx.rec.rows, models, scenarios)
    (run_dir / "summary.md").write_text(report, encoding="utf-8")
    log(f"\nCSV: {csv_path}")
    log(f"Summary: {run_dir / 'summary.md'}\n")
    log(report)
    return 0


if __name__ == "__main__":
    sys.exit(main())
