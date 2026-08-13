#!/usr/bin/env python3
"""Live SSE keep-alive tester for the native ``/v1/messages`` streaming route.

Sends a deliberately slow request -- translate ``capability-request-response-guide.md``
into German -- through a running copilot2api proxy with ``stream: true``, then
reads the SSE response frame by frame with arrival timestamps and reports:

  * how many ``ping`` events the proxy injected (the keep-alive under test),
    against how many it *owed* given the idle windows actually observed,
  * the longest silence on the wire (must stay under the configured keep-alive
    interval, otherwise an idle proxy/LB would evict the connection),
  * whether the injected pings corrupted the upstream stream (a ping written
    mid-event would demote the following event to an unnamed one).

With ``--target both`` the same request also goes straight to the GitHub Copilot
upstream, which injects no pings -- the contrast is the point: the upstream goes
silent for the whole prompt-processing window, the proxy does not.

The translation payload matters. A ~22k-token input keeps the upstream quiet for
seconds before it answers, and the long German output leaves room for mid-stream
stalls; a short prompt would stream straight through and produce no pings.

Measured against the live upstream (2026-08-12, sonnet-4.6, 88k-char document):
the keep-alive fires rarely, and asserting a fixed ping count is a coin flip.
Two things drive that. First, most of the wait is the ~2-4s before the upstream
response headers arrive, and the ticker only starts after that -- see the
``headers_s`` figure in the report. Second, once the stream is flowing it is
dense: at the 1s floor the longest silence measured 1.8s, at the 15s default
nothing comes close. So the primary assertion here is not "at least N pings"
but "every ping this run made room for was sent" -- ``pings_owed`` recomputes
the ticker's schedule from the observed line arrival times, which holds whether
the upstream was chatty or slow. ``--min-pings`` remains available to force the
stricter check. ``anthropic/native_stream_test.go`` covers the deterministic
cases with a synthetic upstream.

Exit code is 0 only if every assertion holds.

Secret handling: the github_token and the exchanged copilot_token are NEVER
printed or written to any output file.

Zero third-party dependencies (urllib + stdlib only).
"""

from __future__ import annotations

import argparse
import json
import os
import ssl
import subprocess
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from capability_test import (  # noqa: E402  (local helper module)
    ANTHROPIC_VERSION,
    COPILOT_USER_AGENT,
    DEFAULT_ACCOUNT,
    DEFAULT_API_KEY,
    DEFAULT_MODEL,
    DEFAULT_PROXY_URL,
    EDITOR_PLUGIN_VERSION,
    EDITOR_VERSION,
    exchange_copilot_token,
    extract_base_url,
    read_github_token,
)

DEFAULT_DOC = "scripts/capability-request-response-guide.md"

# Mirrors anthropic/handler.go: nativeKeepAliveFrame / keepAliveEnvVar.
KEEPALIVE_ENV = "COPILOT2API_SSE_KEEPALIVE_SECONDS"

SYSTEM_PROMPT = (
    "Du bist ein technischer Fachübersetzer. Übersetze das folgende "
    "Markdown-Dokument vollständig ins Deutsche."
)

USER_INSTRUCTIONS = (
    "Übersetze das folgende Dokument ins Deutsche.\n"
    "Regeln:\n"
    "1. Übersetze nur Fließtext, Überschriften und Aufzählungen.\n"
    "2. JSON-Codeblöcke, Feldnamen, base64-Daten und Anker-Links bleiben "
    "unverändert.\n"
    "3. Behalte die Markdown-Struktur exakt bei.\n"
    "4. Gib das Ergebnis in einem Stück aus, ohne Kommentar.\n\n"
    "--- DOKUMENT ---\n"
)


# --------------------------------------------------------------------------- #
# SSE reading with arrival timestamps
# --------------------------------------------------------------------------- #
class Frame:
    """One SSE event block, with the monotonic time its last line arrived."""

    __slots__ = ("event", "data_lines", "at", "raw")

    def __init__(self, event, data_lines, at, raw):
        self.event = event
        self.data_lines = data_lines
        self.at = at
        self.raw = raw

    @property
    def is_ping(self):
        return self.event == "ping"


def stream_sse(url, headers, payload, timeout, raw_path=None):
    """POST and read the SSE body incrementally, timestamping every frame.

    Returns (status, frames, gaps, line_gaps, timing, error), where ``gaps``
    holds the idle seconds between consecutive SSE *events* and ``line_gaps``
    the idle seconds between consecutive *lines*. Both are measured from the
    moment the response headers arrived -- that leading gap is the window the
    keep-alive fills.

    ``line_gaps`` is the metric that decides whether a ping was due: the proxy
    resets its idle ticker on every line it forwards, and an intermediary
    counts silent bytes rather than silent events. A frame-level gap can exceed
    the interval with no ping owed, because a stall between an ``event:`` line
    and its ``data:`` line is deliberately not a ping boundary.

    ``timing["headers_s"]`` is recorded separately on purpose: the proxy can
    only start its keep-alive ticker once it has the upstream response headers,
    so silence before that point is invisible to the keep-alive and must not be
    blamed on it.
    """
    body = json.dumps(payload).encode()
    req = urllib.request.Request(url, data=body, headers=headers, method="POST")
    ctx = ssl.create_default_context()

    started = time.monotonic()
    frames, gaps, line_gaps = [], [], []
    cur_event, cur_data, cur_raw = None, [], []
    ttfb = None
    raw_sink = open(raw_path, "w", encoding="utf-8") if raw_path else None

    try:
        resp = urllib.request.urlopen(req, timeout=timeout, context=ctx)
    except urllib.error.HTTPError as e:
        timing = {"headers_s": time.monotonic() - started, "ttfb_s": None,
                  "total_s": time.monotonic() - started}
        return e.code, [], [], [], timing, e.read().decode("utf-8", "replace")
    except Exception as e:  # noqa: BLE001 -- surfaced in the report
        timing = {"headers_s": None, "ttfb_s": None, "total_s": time.monotonic() - started}
        return 0, [], [], [], timing, str(e)

    headers_at = time.monotonic()
    status = resp.status
    last_frame_at = headers_at
    last_line_at = headers_at
    at_boundary = True  # mirrors pipeNativeStream: a ping may only be injected here
    error = None

    try:
        while True:
            line = resp.readline()
            if not line:
                break
            now = time.monotonic()
            # The handler resets its idle ticker on every line it forwards, and
            # an intermediary sees bytes rather than events, so line-level gaps
            # are the metric that matches both. Frame-level gaps can exceed the
            # interval without any ping being due (a stall between `event:` and
            # `data:` is deliberately not a ping boundary).
            line_gaps.append((now - last_line_at, at_boundary))
            last_line_at = now
            if ttfb is None:
                ttfb = now - started
            text = line.decode("utf-8", "replace")
            cur_raw.append(text)
            if raw_sink:
                raw_sink.write(text)

            stripped = text.rstrip("\r\n")
            if stripped == "":
                # Blank line terminates one SSE event block.
                if cur_event is not None or cur_data:
                    frames.append(Frame(cur_event, cur_data, now, "".join(cur_raw)))
                    gaps.append(now - last_frame_at)
                    last_frame_at = now
                cur_event, cur_data, cur_raw = None, [], []
                at_boundary = True
                continue
            at_boundary = False
            if stripped.startswith("event:"):
                cur_event = stripped[len("event:"):].strip()
            elif stripped.startswith("data:"):
                cur_data.append(stripped[len("data:"):].strip())
    except Exception as e:  # noqa: BLE001
        error = f"stream aborted: {e}"
    finally:
        resp.close()
        if raw_sink:
            raw_sink.close()

    # A trailing block with no terminating blank line means the stream was cut
    # mid-event; surface it rather than silently dropping the bytes.
    if cur_event is not None or cur_data:
        error = error or f"stream ended mid-event (event={cur_event!r}, data lines={len(cur_data)})"

    timing = {
        "headers_s": headers_at - started,
        "ttfb_s": ttfb,
        "total_s": time.monotonic() - started,
    }
    return status, frames, gaps, line_gaps, timing, error


# --------------------------------------------------------------------------- #
# Assertions
# --------------------------------------------------------------------------- #
def check_stream_integrity(frames):
    """Every non-ping frame must still carry its own ``event:`` name.

    A ping written between an ``event:`` line and its ``data:`` line would leave
    the following block unnamed, which client SDKs discard silently. This is the
    failure mode the handler's boundary check prevents.

    ``data: [DONE]`` is exempt: the Copilot upstream terminates the stream with
    that sentinel, which legitimately carries no event name.
    """
    problems = []
    for i, f in enumerate(frames):
        if f.event is None:
            if f.data_lines == ["[DONE]"]:
                continue
            problems.append(f"frame #{i} has data but no event name (likely split by a ping): "
                            f"{f.data_lines[:1]}")
        if f.is_ping and f.data_lines != ['{"type": "ping"}']:
            problems.append(f"frame #{i} is a malformed ping: {f.data_lines}")
    return problems


def pings_owed(line_gaps, keepalive, jitter=0.35):
    """How many pings pipeNativeStream owed, given the idle windows we observed.

    The handler resets a ``time.Ticker`` on every upstream line, so an idle
    window of ``g`` seconds fires ``floor(g / interval)`` times -- but only the
    ticks that land while the stream sits at an event boundary may inject. This
    turns "did we see enough pings" from a coin flip about upstream pacing into
    a deterministic check against what this particular stream made possible.

    ``jitter`` is subtracted from each window before dividing so that scheduling
    slop near an exact multiple of the interval cannot manufacture a phantom
    ping; the result is a lower bound, never an over-count.
    """
    if keepalive <= 0:
        return 0
    owed = 0
    for gap, at_boundary in line_gaps:
        if not at_boundary:
            continue  # a stall mid-event is deliberately not a ping opportunity
        usable = gap - jitter
        if usable > keepalive:
            owed += int(usable // keepalive)
    return owed


def summarize(label, status, frames, gaps, line_gaps, timing, error, keepalive=0):
    """Print one target's result block and return its stats dict."""
    by_type = {}
    for f in frames:
        by_type[f.event or "<unnamed>"] = by_type.get(f.event or "<unnamed>", 0) + 1
    pings = [f for f in frames if f.is_ping]
    origin = frames[0].at - gaps[0] if frames else 0.0
    ping_times = [round(f.at - origin, 2) for f in pings]
    max_gap = max(gaps) if gaps else 0.0
    max_line_gap = max((g for g, _ in line_gaps), default=0.0)
    # Split the wire silence by whether the proxy could have filled it. Only
    # silence at an event boundary is the keep-alive's responsibility; a stall
    # between an `event:` line and its `data:` line is silence the boundary rule
    # deliberately refuses to break, and holding it against the proxy would be
    # asserting against the very invariant the feature exists to protect.
    max_boundary_gap = max((g for g, b in line_gaps if b), default=0.0)
    max_mid_event_gap = max((g for g, b in line_gaps if not b), default=0.0)
    owed = pings_owed(line_gaps, keepalive)

    # Usage from message_delta, if the run got that far.
    out_tokens = None
    for f in frames:
        if f.event == "message_delta":
            try:
                out_tokens = json.loads(f.data_lines[0])["usage"]["output_tokens"]
            except Exception:  # noqa: BLE001
                pass

    def fmt(v):
        return f"{v:.2f}s" if v is not None else "—"

    print(f"\n=== {label} ===")
    print(f"  HTTP 状态          : {status}")
    if error:
        print(f"  错误               : {error[:400]}")
    print(f"  响应头到达         : {fmt(timing['headers_s'])}   (此前 keep-alive 尚未起跑)")
    print(f"  首个 SSE 帧        : {fmt(timing['ttfb_s'])}")
    print(f"  总耗时             : {fmt(timing['total_s'])}")
    print(f"  SSE 事件总数       : {len(frames)}")
    print(f"  ping 事件数        : {len(pings)}")
    if keepalive > 0:
        print(f"  本次应发 ping 数   : {owed}   (由观测到的空闲窗口推算，下界)")
    if ping_times:
        shown = ping_times[:12]
        tail = " ..." if len(ping_times) > len(shown) else ""
        print(f"  ping 到达时刻 (s)  : {shown}{tail}")
    print(f"  最长静默间隔(字节) : {max_line_gap:.2f}s   (逐行计，与代理的空闲计时器口径一致)")
    print(f"    其中事件边界处   : {max_boundary_gap:.2f}s   (keep-alive 该覆盖的部分)")
    if max_mid_event_gap > 0:
        print(f"    其中事件内部     : {max_mid_event_gap:.2f}s   (按设计不可插入 ping)")
    print(f"  最长静默间隔(事件) : {max_gap:.2f}s   (逐事件计，仅供参考)")
    if out_tokens is not None:
        print(f"  输出 token         : {out_tokens}")
    print(f"  事件类型分布       : {dict(sorted(by_type.items()))}")

    return {
        "label": label, "status": status, "error": error,
        "headers_s": round(timing["headers_s"], 3) if timing["headers_s"] is not None else None,
        "ttfb_s": round(timing["ttfb_s"], 3) if timing["ttfb_s"] is not None else None,
        "total_s": round(timing["total_s"], 3), "frames": len(frames),
        "pings": len(pings), "ping_times_s": ping_times,
        "max_gap_s": round(max_gap, 3), "max_line_gap_s": round(max_line_gap, 3),
        "max_boundary_gap_s": round(max_boundary_gap, 3),
        "max_mid_event_gap_s": round(max_mid_event_gap, 3),
        "pings_owed": owed,
        "output_tokens": out_tokens,
        "event_counts": by_type,
    }


def assert_proxy(stats, frames, keepalive, min_pings, tolerance):
    """Proxy-side assertions. Returns a list of failure strings."""
    fails = []
    if stats["status"] != 200:
        fails.append(f"HTTP {stats['status']} != 200 ({(stats['error'] or '')[:200]})")
        return fails

    # Primary, deterministic check: every ping this stream made room for was
    # actually sent. Unlike a fixed count it does not depend on how chatty the
    # upstream happened to be on this particular run.
    owed = stats["pings_owed"]
    if stats["pings"] < owed:
        fails.append(
            f"应发 {owed} 个 ping，实收 {stats['pings']} 个 —— keep-alive 漏发"
        )

    # Secondary, opt-in check: the caller demanded a specific ping count.
    if stats["pings"] < min_pings:
        if owed == 0:
            why = (f"本次上游始终活跃（事件边界处最长静默 {stats['max_boundary_gap_s']}s "
                   f"≤ keep-alive 间隔 {keepalive}s），没有 ping 是正确行为，"
                   f"并非代理故障；如需强制观测 ping，请调低 --keepalive-seconds")
        else:
            why = f"本次仅有 {owed} 次发 ping 的机会"
        # The ticker only starts once handleNativeMessagesPassthrough has the
        # upstream response headers, so a long pre-header wait is a blind spot
        # rather than a keep-alive failure. Say so instead of leaving the reader
        # to guess why a visibly slow request produced no pings.
        if stats["headers_s"] is not None and stats["headers_s"] > keepalive:
            why += (f"；另外本次响应头等待了 {stats['headers_s']}s（> {keepalive}s），"
                    f"该窗口在 keep-alive 起跑之前，代理无法覆盖")
        fails.append(f"仅收到 {stats['pings']} 个 ping，少于 --min-pings 要求的 {min_pings} 个：{why}")

    limit = keepalive + tolerance
    if keepalive > 0 and stats["max_boundary_gap_s"] > limit:
        fails.append(
            f"事件边界处最长静默 {stats['max_boundary_gap_s']}s 超过 keep-alive 间隔 "
            f"{keepalive}s + 容差 {tolerance}s —— 空闲连接仍可能被中断代理回收"
        )

    fails.extend(check_stream_integrity(frames))

    names = [f.event for f in frames]
    if "message_start" not in names:
        fails.append("缺少 message_start 事件")
    if "message_stop" not in names:
        fails.append("缺少 message_stop 事件（流未正常结束）")
    return fails


# --------------------------------------------------------------------------- #
# Request building
# --------------------------------------------------------------------------- #
def build_payload(doc_text, model, max_tokens, thinking_budget):
    payload = {
        "model": model,
        "max_tokens": max_tokens,
        "stream": True,
        "system": SYSTEM_PROMPT,
        "messages": [{"role": "user", "content": USER_INSTRUCTIONS + doc_text}],
    }
    if thinking_budget > 0:
        payload["thinking"] = {"type": "enabled", "budget_tokens": thinking_budget}
    return payload


def proxy_headers(api_key):
    return {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {api_key}",
        "anthropic-version": ANTHROPIC_VERSION,
        "Accept": "text/event-stream",
        # Identity encoding keeps readline() incremental; a gzip stream would
        # buffer and destroy the arrival timestamps this test is built on.
        "Accept-Encoding": "identity",
    }


def direct_headers(copilot_token):
    return {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {copilot_token}",
        "anthropic-version": ANTHROPIC_VERSION,
        "Accept": "text/event-stream",
        "Accept-Encoding": "identity",
        "User-Agent": COPILOT_USER_AGENT,
        "Editor-Version": EDITOR_VERSION,
        "Editor-Plugin-Version": EDITOR_PLUGIN_VERSION,
        "Copilot-Integration-Id": "vscode-chat",
        "X-Initiator": "user",
    }


def wait_ready(url, timeout_s):
    deadline = time.monotonic() + timeout_s
    while time.monotonic() < deadline:
        try:
            urllib.request.urlopen(url + "/v1/models", timeout=3)
            return True
        except urllib.error.HTTPError:
            return True  # reachable; auth rejection is fine here
        except Exception:  # noqa: BLE001
            time.sleep(0.7)
    return False


def start_proxy(repo_dir, port, keepalive):
    print(f"  启动本地代理: go run . -host 127.0.0.1 -port {port}  ({KEEPALIVE_ENV}={keepalive})")
    env = dict(os.environ, **{KEEPALIVE_ENV: str(keepalive)})
    return subprocess.Popen(
        ["go", "run", ".", "-host", "127.0.0.1", "-port", str(port), "-debug"],
        cwd=repo_dir, env=env,
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )


# --------------------------------------------------------------------------- #
def main():
    ap = argparse.ArgumentParser(
        description="Live SSE keep-alive (ping) tester: German translation over /v1/messages")
    ap.add_argument("--target", choices=["proxy", "direct", "both"], default="proxy",
                    help="proxy = 被测代理；direct = 上游对照（无 ping）")
    ap.add_argument("--doc", default=DEFAULT_DOC, help="要翻译的 Markdown 文件")
    ap.add_argument("--max-chars", type=int, default=0,
                    help="截断输入字符数（0 = 全文，输入越大上游静默越久）")
    ap.add_argument("--proxy-url", default=os.environ.get("COPILOT2API_TEST_URL", DEFAULT_PROXY_URL))
    ap.add_argument("--api-key", default=os.environ.get("COPILOT2API_TEST_API_KEY", DEFAULT_API_KEY))
    ap.add_argument("--account", default=os.environ.get("COPILOT2API_ACCOUNT", DEFAULT_ACCOUNT))
    ap.add_argument("--model", default=os.environ.get("COPILOT2API_TEST_MODEL", DEFAULT_MODEL))
    ap.add_argument("--max-tokens", type=int, default=16000)
    ap.add_argument("--thinking-budget", type=int, default=2048,
                    help="0 = 关闭；开启会拉长首字节前的静默窗口")
    ap.add_argument("--keepalive-seconds", type=int, default=1,
                    help=f"代理的 {KEEPALIVE_ENV}（整数秒，1 是下限）；仅 --start-proxy 时会实际生效")
    ap.add_argument("--min-pings", type=int, default=0,
                    help="额外要求的最少 ping 数；默认 0，即只断言“该发的 ping 一个没漏”"
                         "（由本次实测的空闲窗口推算）。真实上游流很密集，写死一个数容易 flaky")
    ap.add_argument("--gap-tolerance", type=float, default=3.0,
                    help="最长静默相对 keep-alive 间隔的允许超出秒数")
    ap.add_argument("--timeout", type=int, default=900)
    ap.add_argument("--start-proxy", action="store_true", help="用 `go run .` 自动起一个本地代理")
    ap.add_argument("--proxy-port", type=int, default=17777)
    ap.add_argument("--repo-dir", default=str(Path(__file__).resolve().parent.parent))
    ap.add_argument("--json", default="", help="把统计写入 JSON 文件")
    ap.add_argument("--dump-raw", default="", help="把原始 SSE 流写入该前缀 (前缀-proxy.sse / -direct.sse)")
    args = ap.parse_args()

    doc_path = Path(args.doc)
    if not doc_path.is_absolute():
        doc_path = Path(args.repo_dir) / args.doc
    if not doc_path.exists():
        print(f"找不到文档: {doc_path}", file=sys.stderr)
        return 2
    doc_text = doc_path.read_text(encoding="utf-8")
    if args.max_chars > 0:
        doc_text = doc_text[:args.max_chars]

    print("SSE keep-alive 活体测试 —— 德文翻译长请求")
    print(f"  时间       : {datetime.now(timezone.utc).astimezone():%Y-%m-%d %H:%M:%S %z}")
    print(f"  文档       : {doc_path.name}  ({len(doc_text)} 字符, ~{len(doc_text)//4} tokens)")
    print(f"  模型       : {args.model}")
    print(f"  max_tokens : {args.max_tokens}  thinking budget: {args.thinking_budget}")
    print(f"  keep-alive : {args.keepalive_seconds}s   "
          f"断言: 应发的 ping 一个不漏" + (f"，且 >= {args.min_pings}" if args.min_pings else ""))

    payload = build_payload(doc_text, args.model, args.max_tokens, args.thinking_budget)

    def raw_for(which):
        if not args.dump_raw:
            return None
        p = Path(args.dump_raw)
        if not p.is_absolute():
            p = Path(args.repo_dir) / args.dump_raw
        p.parent.mkdir(parents=True, exist_ok=True)
        return f"{p}-{which}.sse"

    proxy_url = args.proxy_url
    proc = None
    results, failures = [], []
    try:
        if args.target in ("proxy", "both"):
            if args.start_proxy:
                proc = start_proxy(args.repo_dir, args.proxy_port, args.keepalive_seconds)
                proxy_url = f"http://127.0.0.1:{args.proxy_port}"
                if not wait_ready(proxy_url, 60):
                    print("代理未在 60s 内就绪", file=sys.stderr)
                    return 2
            else:
                print(f"  注意: 未使用 --start-proxy，请确认目标代理的 {KEEPALIVE_ENV}"
                      f" 与 --keepalive-seconds ({args.keepalive_seconds}) 一致")

            print(f"\n  → 请求 {proxy_url}/v1/messages (stream)")
            status, frames, gaps, line_gaps, timing, err = stream_sse(
                proxy_url + "/v1/messages", proxy_headers(args.api_key), payload,
                args.timeout, raw_path=raw_for("proxy"))
            stats = summarize("代理 (copilot2api)", status, frames, gaps, line_gaps,
                              timing, err, keepalive=args.keepalive_seconds)
            results.append(stats)
            failures = assert_proxy(stats, frames, args.keepalive_seconds,
                                    args.min_pings, args.gap_tolerance)

        if args.target in ("direct", "both"):
            token = exchange_copilot_token(read_github_token(args.account), 30)
            base = extract_base_url(token)
            print(f"\n  → 请求 {base}/v1/messages (stream, 上游直连对照)")
            status, frames, gaps, line_gaps, timing, err = stream_sse(
                base + "/v1/messages", direct_headers(token), payload,
                args.timeout, raw_path=raw_for("direct"))
            stats = summarize("上游直连 (GitHub Copilot)", status, frames, gaps,
                              line_gaps, timing, err)
            results.append(stats)
            if stats["pings"]:
                print(f"  说明: 上游本身也发了 {stats['pings']} 个 ping"
                      f"（代理的注入是叠加在其之上的）")
            else:
                print("  说明: 上游全程未发 ping —— 这正是代理需要补 keep-alive 的原因")
    finally:
        if proc is not None:
            proc.terminate()
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                proc.kill()

    if args.json:
        out = Path(args.json)
        if not out.is_absolute():
            out = Path(args.repo_dir) / args.json
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(json.dumps({
            "generated_at": datetime.now(timezone.utc).isoformat(),
            "model": args.model, "doc": str(doc_path), "doc_chars": len(doc_text),
            "keepalive_seconds": args.keepalive_seconds, "min_pings": args.min_pings,
            "results": results, "failures": failures,
        }, ensure_ascii=False, indent=2), encoding="utf-8")
        print(f"\n  统计已写入 {out}")

    print("\n" + "=" * 60)
    if args.target == "direct":
        print("对照运行完成（direct 目标不做断言）")
        return 0
    if failures:
        print("❌ 测试失败:")
        for f in failures:
            print(f"   - {f}")
        return 1
    # Say which of the two situations we are in. "Passed" with zero pings means
    # the upstream never idled long enough to need one -- a real result, but not
    # evidence that injection works; claiming otherwise would be a green light
    # that never actually exercised the feature.
    proxy_stats = next((r for r in results if r["label"].startswith("代理")), None)
    if proxy_stats and proxy_stats["pings"] == 0:
        print("✅ 测试通过: SSE 事件边界完好，且本次没有任何该发未发的 ping")
        print(f"   但本次上游全程活跃（边界处最长静默 "
              f"{proxy_stats['max_boundary_gap_s']}s ≤ {args.keepalive_seconds}s），"
              f"注入路径实际未被触发；")
        if proxy_stats["headers_s"] and proxy_stats["headers_s"] > args.keepalive_seconds:
            print(f"   注意最长的一段静默是等待上游响应头的 {proxy_stats['headers_s']}s，"
                  f"这段在 keep-alive 起跑之前，代理目前覆盖不到")
    else:
        print("✅ 测试通过: 代理在长时间翻译请求中注入了 ping，且未破坏 SSE 事件边界")
    return 0


if __name__ == "__main__":
    sys.exit(main())
