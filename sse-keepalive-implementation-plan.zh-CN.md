# 原生 `/v1/messages` 透传:SSE Keep-alive 实现记录

> 状态:**已实现**
> 提交:`ec94ec8`(保活主体 + 修复 A/B)、`7a32b62`(修复 C/D)—— 分支 `feat/native-messages-sse-keepalive`
> 范围:`anthropic/handler.go` 中 `handleNativeMessagesPassthrough` 的 `stream=true` 分支;`internal/sse/sse.go`(仅响应头,影响全部三条流式路径)
> 不涉及:`proxy/stream.go` 与 `gemini/handler.go` 的 ping 注入、`anthropic` 的 OpenAI→Anthropic 转换路径(`streamSSE`)

本文档已按 `7a32b62` 后的代码回填。§5.3、§5.4 是初版计划之外的追加内容 —— 它们修的是**保活自身在落地时暴露的两个新问题**,详见各节。文中行号对应当前 `main` 之外的本分支代码。

---

## 1. 背景

### 1.1 改动前:代理不发送任何 SSE 保活信号

对三条流式路径的代码审查与运行时实测结论:**改动前的 copilot2api 不发送任何 SSE keep-alive**。

- `anthropic/handler.go`(原生透传)—— 纯 `reader.ReadBytes('\n')` 阻塞循环
- `proxy/stream.go:78` —— `for scanner.Scan()` 阻塞循环
- `gemini/handler.go:443` —— 同上
- 改动前全仓 `time.NewTicker` 仅出现在 `auth/device_flow.go` 与 `internal/stats/stats.go`,与流式无关

唯一相关的是 `internal/sse/sse.go` 设置的 `Connection: keep-alive` **响应头** —— 这只是 HTTP/1.1 连接复用语义,HTTP/2 下被忽略,不产生任何字节。

实测(构造上游静默 3s 的场景,`proxy.streamResponse`):

```
bytes written to client after 2.5s of upstream silence: 0 -> ""
```

### 1.2 影响

与 `docs/copilot2api-issues-retrospective.html:1158` 记录的失败类完全吻合:长推理静默期无字节流动,空闲 SSE 连接被中间层(NAT / CDN / 网关 / LB)在 ~60–350s 驱逐。同页 `:1159` 记录官方诉求为**服务端 SSE 心跳帧(≤30s)**,但尚未进入 roadmap —— 这既确认了问题,也给出了本方案的间隔上界。

`main.go:171` 已刻意不设 `ReadTimeout`(注释:`ReadTimeout would kill long-lived SSE streaming connections`),但那只能规避代理自身的超时,挡不住中间层。

另据 `docs/upstream-messages-live-tests.zh-CN.md:118-126` 实测,GitHub Copilot 上游在 ~630s 后以 HTTP/2 RST_STREAM 切断且无终结事件 —— 说明**上游自身在静默期也不发 ping**,代理拿不到可转发的心跳,必须自己产生。

---

## 2. Anthropic 官方 Keep-alive 机制分析

设计需与官方语义一致,故先明确官方做法。

### 2.1 使用命名事件,而非 SSE 注释行

```
event: ping
data: {"type": "ping"}

```

官方文档(Streaming messages)明确两点:

- *"Each event uses an SSE event name (for example, `event: message_stop`), and includes the matching event `type` in its data."*
- *"Event streams may also include any number of `ping` events."*

即 ping 是流协议的一等公民,不是传输层的注释填充。

### 2.2 契约是"任意数量、任意时刻",不承诺间隔

官方措辞为 `any number of ping events`,**未承诺任何间隔**。社区实测在扩展思考静默期约每 10s 一次。设计意图:服务端把 ping 当作机会性的存活信号,客户端**不应**用"缺失 N 个 ping"判定超时。

### 2.3 客户端侧:显式丢弃,不解析 data

`anthropic-sdk-python/src/anthropic/_streaming.py` 的 `Stream.__stream__`:

```python
if sse.event == "ping":
    continue
```

无条件 `continue`,**data 根本不被解析**。同时 `types/raw_message_stream_event.py` 的 `RawMessageStreamEvent` 联合类型中**没有 ping 分支**(TS SDK issue #749 正在请求补上)。

两条可依赖的推论:

1. **data 内容不敏感** —— `{}` 或 `{"type": "ping"}` 都会被安全吞掉,只要 `event: ping` 正确
2. **ping 不进入累加器** —— 不污染 `message.content`,不干扰 `content_block` 的 index 序列

### 2.4 生成端 vs 转发端的差异(本方案的核心难点来源)

Anthropic 官方处在**生成端**,逐事件产出,天然只在事件边界发 ping。
copilot2api 处在**转发端**,面对的是逐行字节流而非事件流 —— 选择命名事件格式,就必须自己补回边界感知(见 §4)。

---

## 3. 架构设计

### 3.1 三条硬约束

| # | 约束 | 后果 |
|---|---|---|
| 1 | `http.ResponseWriter` 非并发安全 | 不能用"心跳 goroutine + mutex"方案 |
| 2 | ping 不能插进 SSE 事件内部 | 注入方必须知道当前是否处于事件边界 |
| 3 | 需要空闲计时而非固定心跳 | 上游活跃时不应注入 |

### 3.2 选定架构:读 goroutine + 单写者 select

把**阻塞的读**移入 goroutine,**写**全部留在主循环:

```
┌─ read goroutine ─┐        ┌───── main loop (唯一写者) ──────┐
│ ReadBytes('\n')  │──ch──▶ │ select {                        │
│ (阻塞在这里)      │        │   case res := <-lines:   写行    │
└──────────────────┘        │   case <-tickC:        写 ping   │
                            │   case <-idleC:   写 error 并中止 │
                            │   case <-ctx.Done():     退出    │
                            │ }                               │
                            └─────────────────────────────────┘
```

一次性满足全部三个约束:

- 唯一写者 → 无需锁(约束 1)
- 主循环天然知道自己刚写了什么 → 边界可判定(约束 2)
- 收到数据即 `ticker.Reset()` → 空闲语义(约束 3)

`idleC` 分支是后来追加的静默上限(§5.3)。它能以两行接入,正是这套单写者结构的红利 —— 中止帧同样需要边界感知,若走独立 goroutine 则无解。

### 3.3 被否决的备选:心跳 goroutine + mutex

独立 goroutine 定时写 ping,用 mutex 保护 `w`。否决理由:

- 违反约束 1 的精神:即便加锁,心跳 goroutine 也**无法知道**主循环是否正处于事件中间态,约束 2 无解
- 引入锁竞争与更复杂的 teardown

---

## 4. 边界跟踪(`atBoundary`)的必要性

这是设计中唯一不显然的部分,故单列并附实证。

### 4.1 根因:逐行转发,而非逐事件转发

一个 Anthropic SSE 事件横跨多行,以空行终结:

```
event: content_block_delta
data: {"index":0,"text":"hello"}
<空行>
```

而转发循环是 `ReadBytes('\n')` —— **逐行**。定时器触发的瞬间,完全可能正卡在 `event:` 行已写出、`data:` 行尚未写出的中间态。

### 4.2 实测:用仓库自己的 `readSSEEvent`(`handler.go:670`)解析

```
A) 边界注入 ✅
   event="content_block_delta"  data="{"index":0,"text":"hello"}"
   event="ping"                 data="{"type": "ping"}"
   event="content_block_stop"   data="{"index":0}"

B) 中间注入 ❌
   event="ping"                 data="{"type": "ping"}"
   event=""                     data="{"index":0,"text":"hello"}"   ← 事件名丢失
   event="content_block_stop"   data="{"index":0}"

C) 对照组:同一中间位置改用注释行 ": ping"
   event="content_block_delta"  data=""      ← 事件名保住
   event=""                     data="{"index":0,"text":"hello"}"
```

### 4.3 后果分析

B 组同时发生两处损坏:

1. **`event:` 字段被覆盖** —— ping 的 `event: ping` 顶掉了 `content_block_delta`
2. **原事件降级为匿名事件** —— data 尚在,但 event 名变为空串

第 2 点是致命的:`_streaming.py` 的 `__stream__` 是一长串 `if sse.event == "..."` 且**没有 else 分支**,`event=""` 匹配不上任何一条,于是被**静默丢弃**——无异常、无告警、无日志。

用户侧表现为回答中间凭空缺失一段文字。若被劈开的是 `content_block_start`,后续 delta 的 `index` 会指向从未创建的 block,SDK 累加器可能直接崩溃。

### 4.4 风险量级

单次静默窗口是毫秒级,但需乘以流的规模:实测流为 **36,928 行 / 630 秒**(`docs/upstream-messages-live-tests.zh-CN.md:124`)。长期运行下撞上中间态只是概率问题,而代价是**静默数据损坏** —— 保活机制本身反而成为损坏源。

`if !atBoundary { continue }` 仅两行,将该概率压到零。

### 4.5 同一根因的第二个落点:中止帧

静默上限(§5.3)写出的 `error` 事件面对同源的危险,但**损坏形态与 ping 不同**,值得单独实测。

因为 `readSSEEvent` 是**后写覆盖先写**(`handler.go:708` 的 `case "event": eventType = value`),而 `data` 行是**累积**的(`handler.go:710` 的 `append`),所以直接把 `error` 帧追加到半截事件之后时:

```
A) 上游停在 `event:` 行后 / 不闭合
   event="error"  data="{"type":"error",...}"          ← error 侥幸幸存

B) 上游停在 `data:` 行后 / 不闭合
   event="error"  data="{"index":0,"text":"hello"}\n{"type":"error",...}"
                        ↑ 上游残留的 data 被拼进 error payload → JSON 解析必然失败

B') 同一位置 / 先补空行闭合
   event="content_block_delta"  data="{"index":0,"text":"hello"}"   ← 上游事件完整投递
   event="error"                data="{"type":"error",...}"         ← error 干净
```

所以真正的危害不是"事件名被顶掉"(事件名反而是 `error` 赢),而是 **B 组的 data 行拼接**:客户端拿到一个双行 JSON,解析失败,**连中止原因都读不到**;同时上游那条本已完整的 delta 被一并吞掉。

这里不能像 ping 那样"跳过等下一轮"—— 中止是终局动作。处理方式是先补一个空行闭合半截事件(`handler.go:387-391`),两种情况一并解决。由 `TestPipeNativeStream_SilenceCeilingClosesPartialEvent` 锁定(仅覆盖 A 组,见 §8.6)。

### 4.6 附带结论:为何命名事件比注释行"贵"

对照组 C 显示,`handler.go:697` 的 `if !strings.HasPrefix(line, ":")` 让注释行不参与 event/data 累积,**事件名不会被覆盖**,破坏程度轻一级(但仍触发提前 dispatch,故注释行同样不能随意乱插)。

这正是本路径选择命名事件所必须付出的代价 —— 而选择命名事件是正确的,因为下游必定是 Anthropic 客户端。

---

## 5. 四个配套修复

A、B 随保活主体一同落地(`ec94ec8`);C、D 是保活上线后才暴露的问题,由 `7a32b62` 补齐。

### 5.1 修复 A:`BeginSSE(w)` 后立即 Flush

**原因:改动前会把 HTTP 响应头扣留到上游首字节到达。**

`internal/sse/sse.go` 的 `BeginSSE` 只做几次 `Header().Set(...)`,不写任何字节;Go 的 `ResponseWriter` 要到第一次 `Write()` 才发送状态行与响应头。而其后紧接的是阻塞的 `ReadBytes`。

实测(模拟上游首字节延迟 2s):

```
改动前 (BeginSSE 不 Flush)      首字节耗时  2.00s
修复后 (BeginSSE + Flush)       首字节耗时  0.00s
```

延迟的不是数据,而是**协议握手**。在客户端拿到响应头之前:

- **响应头超时会误杀请求** —— 客户端与中间层普遍对响应头单独计时且阈值更低(Go 的 `http.Transport.ResponseHeaderTimeout`、nginx `proxy_read_timeout` 默认 60s)。扩展思考期动辄数分钟不吐字节,这些计时器会在任何数据到达前掐断连接
- **SSE 解析器无法启动** —— 需先看到 `Content-Type: text/event-stream` 才切入流式解析
- **请求状态不可观测** —— 客户端分不清"已接受、正在思考"与"仍卡在连接/鉴权"

**这不是新要求,而是补齐既有惯例** —— 另两条路径均已如此(`proxy/stream.go:52-57`、`gemini/handler.go:199-202`),原生 `/v1/messages` 透传是三条路径中唯一遗漏的。

**与 keep-alive 互补,不可互相替代**:开启保活后首个 ping 的 `Write` 会隐式发送响应头,但需等满一个 interval(15s);且 `interval=0` 时问题原封不动。显式 Flush 将代价降至 0s 且不依赖保活开关。

#### 5.1.1 已知盲区:上游响应头阶段不在覆盖范围内

修复 A 覆盖的是"**上游响应头已到达、body 静默**"。`h.upstream.Do(...)`(`handler.go:229`)会阻塞到上游响应头返回,`BeginSSE` + `Flush`(`handler.go:250-254`)与 ticker 都在其**之后**才开始 —— 因此上游响应头到达**之前**的静默,保活与修复 A 都覆盖不到。

实测印证(`scripts/README.md`):某次真实请求(`claude-sonnet-4.6`,1226 个 SSE 事件 / 63s)整程最长的静默正是**响应头到达前的 5.5s**,而全程 ping 数为 0 —— 后者是正确结果(上游一旦开始流式输出,最长间隔仅 ~0.4s),但前者说明读 ping 计数时必须先看报告里的「响应头到达」行。

该阶段目前只由 `http.Transport.ResponseHeaderTimeout` 单独兜底。要覆盖它就得在发起上游请求**之前**先 `BeginSSE + Flush`,代价是一旦 200 OK 已经发出,就再也无法把上游的错误状态码原样透传给客户端(`handler.go:230-241` 的 `writeRawUpstreamError` 分支会失效)。权衡后未做。

### 5.2 修复 B:主动监听 `ctx.Done()` + ping 写失败即返回

**原因:改动前在上游静默期对下游断连完全失明。**

原循环仅在 `w.Write(line)` 失败时退出。上游静默时阻塞在 `ReadBytes`,**一次写操作都不会发生**,无从得知客户端已断开。

故障场景:客户端 60s 超时放弃,上游流长达 630s —— 代理继续读满剩余 570 秒,持续消耗 Copilot 配额、占用上游连接与 goroutine,直到上游流自然结束才发现对端早已消失。多账号部署下此类僵尸流会累积。

实测断连检测延迟:

```
[0.50s] 客户端强制关闭 TCP 连接
  ctx.Done()  在 0.50s 触发
  Write 探测: 第 5 次写入失败 @ 1.01s: write: broken pipe
```

| 机制 | 断连检测延迟 | 依赖 |
|---|---|---|
| 改动前(仅 Write 失败) | 最长至上游流结束(可达 570s) | 上游恰好有数据要转发 |
| ping 的 Write 失败 | ≤ 1 个 interval(15s) | TCP RST 往返 |
| `ctx.Done()` | 即时 | 无 |

注意 Write 探测**滞后一拍**:客户端 0.50s 断开,但直到 0.50s 后的下一次写入(1.01s)才拿到 `broken pipe` —— 断开瞬间那次写只进了内核缓冲区,需等对端 RST 返回才反映到下一次写。

**决定:两者都保留。** `ctx.Done()` 负责即时退出;ping 写失败作为兜底,覆盖 ctx 未 cancel 但连接已不可写的边缘情况。单写者 select 架构让 `ctx.Done()` 分支的成本仅为两行。

### 5.3 修复 C:上游静默上限(`7a32b62`,初版计划遗漏)

**原因:保活消除了唯一会自然终结"上游卡死"链路的机制。**

修复 B 处理的是**客户端**消失;这里处理的是**上游**卡死不动却不断连的情形。两者的关键差别在于:保活把它从"自限"变成了"永久"。

| | 改动前 | 只加保活 |
|---|---|---|
| 代理行为 | 阻塞在 `ReadBytes`,不写下游 | 每 15s 向下游写一个 ping |
| 下游连接命运 | 中间层空闲驱逐 → `ctx.Done()` → 收工 | **被 ping 主动续命,永不驱逐** |
| 净结果 | 链路最终被外力拆掉 | goroutine + 客户端连接 + 上游连接**无限期占用** |

`http.Transport.ResponseHeaderTimeout` 只守响应头阶段,对 body 阶段的静默无效。也就是说,保活修好了一个问题的同时,把另一条本可自愈的路径变成了泄漏 —— 必须自己补一个上限。

**实现要点**(`handler.go:322-332, 363-365, 382-397`):

- **独立 `time.Timer`,而非"ping 计数"**。保活可被关闭(`interval=0`),而上限必须仍然生效 —— 由 `TestPipeNativeStream_SilenceCeilingWithoutKeepAlive` 锁定。用一次性 `Timer` 而非 `Ticker`,语义即"中止是终局动作"。
- **只有上游字节重置它**,ping 不重置。否则保活会给自己续命,上限形同虚设。
- **默认 10 分钟**。扩展思考可以合法静默数分钟;Claude Code 等客户端自身有 ~300s 的空闲底线;上游实测在 ~630s 后 RST。10 分钟留足余量 —— 它的职责不是"管得严",而是兜住真正卡死的情形。
- **中止帧的构造**:先补空行闭合半截事件(§4.5),再写终结性 `error` 事件,**且不追加 `message_stop`** —— 在被截断的流后面补 `message_stop`,会让 SDK 累加器把一条不完整的消息当作成功完成的消息返回给用户,比直接报错更坏。

### 5.4 修复 D:`X-Accel-Buffering: no`(`7a32b62`,初版计划遗漏)

**原因:在最常见的反向代理后面,保活是静默失效的。**

nginx 默认 `proxy_buffering on`,会把上游增量 flush 的响应缓冲起来,直到缓冲区填满或流结束才下发。对普通响应这只是延迟;对 SSE 保活则是**功能被完全抵消** —— ping 逐个进入 nginx 的缓冲区,客户端一个也收不到,连接照样被判定为空闲。

修复落在 `internal/sse/sse.go:14` 的 `BeginSSE`,因此 Anthropic、OpenAI、Gemini 三条流式路径(`anthropic/handler.go:237,250,598`、`proxy/handler.go:449,496`、`proxy/stream.go:52`、`gemini/handler.go:199`)一并受益。不认识该响应头的代理会忽略它,无副作用。

这一条**越出了初版计划声明的"仅 `anthropic/handler.go`"范围**,是有意为之:该响应头与业务协议无关,收敛到 `BeginSSE` 一处比在四个调用点重复更安全。由 `internal/sse/sse_test.go` 的 `TestBeginSSE` 锁定。

---

## 6. 实现

以下代码块为 `anthropic/handler.go` 当前实际内容(注释保持源码中的英文原文)。

### 6.1 常量与环境变量解析

```go
// nativeKeepAliveFrame matches the Anthropic ping event format. ...
const nativeKeepAliveFrame = "event: ping\ndata: {\"type\": \"ping\"}\n\n"

// defaultKeepAliveInterval sits below the idle-eviction thresholds commonly
// applied by NATs, CDNs and load balancers (AWS ALB defaults to 60s) while
// staying above the ~10s cadence observed from Anthropic upstream.
const defaultKeepAliveInterval = 15 * time.Second

// keepAliveEnvVar configures the native /v1/messages streaming keep-alive.
const keepAliveEnvVar = "COPILOT2API_SSE_KEEPALIVE_SECONDS"

// defaultMaxUpstreamIdle bounds how long a native /v1/messages stream may sit
// without a single upstream byte before it is aborted. ...
const defaultMaxUpstreamIdle = 10 * time.Minute

// maxUpstreamIdleEnvVar configures the native /v1/messages silence ceiling.
const maxUpstreamIdleEnvVar = "COPILOT2API_SSE_MAX_IDLE_SECONDS"

// durationFromEnv parses a whole-number seconds value from the environment.
// An unset or empty value yields def; 0 is honoured as "disabled"; negative or
// unparseable values fall back to def with a warning.
func durationFromEnv(name string, def time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		slog.Warn("invalid "+name+", using default",
			"value", v, "default_seconds", int(def.Seconds()))
		return def
	}
	return time.Duration(n) * time.Second
}

func keepAliveIntervalFromEnv() time.Duration {
	return durationFromEnv(keepAliveEnvVar, defaultKeepAliveInterval)
}

func maxUpstreamIdleFromEnv() time.Duration {
	return durationFromEnv(maxUpstreamIdleEnvVar, defaultMaxUpstreamIdle)
}
```

解析逻辑抽成 `durationFromEnv` 是加入第二个变量时的必然收敛:两者的语义完全相同(空→默认、`0`→禁用、负数/非法→告警+默认),没有理由写两遍。注意 `0` 与"非法"必须区分 —— 前者是用户显式关闭,后者才回退默认。

### 6.2 `Handler` 字段

```go
type Handler struct {
	upstream *upstream.Client
	models   *models.Cache
	// keepAliveInterval is the idle period after which a ping event is injected
	// into native /v1/messages streams. Zero disables keep-alive.
	keepAliveInterval time.Duration
	// maxUpstreamIdle aborts a native /v1/messages stream once the upstream has
	// been silent for this long. Zero disables the ceiling.
	maxUpstreamIdle time.Duration
}

func NewHandler(authClient upstream.TokenProvider, transport *http.Transport, mc *models.Cache) *Handler {
	return &Handler{
		upstream:          upstream.NewClient(authClient, transport),
		models:            mc,
		keepAliveInterval: keepAliveIntervalFromEnv(),
		maxUpstreamIdle:   maxUpstreamIdleFromEnv(),
	}
}
```

在 `NewHandler` 内部读 env,而非扩展签名 —— 与 `internal/accounts/config.go:42` 直接 `os.Getenv` 的既有惯例一致,可避免改动 `accounts_wire.go:63` 这一唯一调用点。测试直接构造 `&Handler{keepAliveInterval: ..., maxUpstreamIdle: ...}`。

### 6.3 主体(`handler.go:276-404`)

```go
// pipeNativeStream forwards the upstream SSE stream to the client line by line,
// injecting Anthropic ping events at event boundaries whenever the upstream has
// been idle for longer than h.keepAliveInterval. If the upstream stays silent
// for longer than h.maxUpstreamIdle the stream is aborted with an error event,
// so a wedged upstream cannot be kept alive indefinitely by the pings.
//
// Reading runs in its own goroutine so the main loop stays the sole writer:
// http.ResponseWriter is not safe for concurrent use, and only the writing loop
// can know whether the stream currently sits on an SSE event boundary.
func (h *Handler) pipeNativeStream(ctx context.Context, w io.Writer, flusher http.Flusher, body io.Reader) {
	reader := bufio.NewReaderSize(body, 32*1024)

	type readResult struct {
		line []byte
		err  error
	}
	lines := make(chan readResult, 8)
	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			line, err := reader.ReadBytes('\n')
			select {
			case lines <- readResult{line: line, err: err}:
			case <-done:
				return
			}
			if err != nil {
				return
			}
		}
	}()

	// A nil tickC is never selected, which disables keep-alive without needing a
	// separate forwarding loop.
	var (
		ticker *time.Ticker
		tickC  <-chan time.Time
	)
	if h.keepAliveInterval > 0 {
		ticker = time.NewTicker(h.keepAliveInterval)
		defer ticker.Stop()
		tickC = ticker.C
	}

	// The silence ceiling runs on its own timer rather than counting pings, so it
	// still applies when keep-alive is disabled. A nil idleC is never selected.
	var (
		idleTimer *time.Timer
		idleC     <-chan time.Time
	)
	if h.maxUpstreamIdle > 0 {
		idleTimer = time.NewTimer(h.maxUpstreamIdle)
		defer idleTimer.Stop()
		idleC = idleTimer.C
	}

	atBoundary := true // the start of the stream is a valid event boundary
	pings := 0

	for {
		select {
		case res := <-lines:
			if len(res.line) > 0 {
				if _, err := w.Write(res.line); err != nil {
					slog.Error("failed to write native /messages stream", "error", err)
					return
				}
				// Flush at SSE event boundaries (blank lines) instead of every line
				// to reduce syscall overhead while maintaining correct SSE delivery.
				atBoundary = isBlankSSELine(res.line)
				if atBoundary {
					flusher.Flush()
				}
			}
			if errors.Is(res.err, io.EOF) {
				slog.Debug("native /messages stream complete", "keepalive_pings", pings)
				return
			}
			if res.err != nil {
				slog.Error("error reading native /messages stream", "error", res.err)
				return
			}
			if ticker != nil {
				ticker.Reset(h.keepAliveInterval)
			}
			if idleTimer != nil {
				idleTimer.Reset(h.maxUpstreamIdle)
			}

		case <-tickC:
			// Never split an SSE event: an injected frame would override the
			// pending `event:` field, demoting the original event to an unnamed
			// one that client SDKs discard silently. Fragmentation pauses last
			// milliseconds, so the next tick will find a boundary.
			if !atBoundary {
				continue
			}
			if _, err := io.WriteString(w, nativeKeepAliveFrame); err != nil {
				slog.Debug("client disconnected during keep-alive", "error", err)
				return
			}
			flusher.Flush()
			pings++

		case <-idleC:
			slog.Error("aborting native /messages stream after upstream silence",
				"max_idle", h.maxUpstreamIdle.String(), "keepalive_pings", pings)
			// Emitting the error mid-event would corrupt the pending frame, so
			// close it out first; a stray blank line is inert to SSE parsers.
			if !atBoundary {
				if _, err := io.WriteString(w, "\n"); err != nil {
					return
				}
			}
			// `error` is terminal for Anthropic clients, so no message_stop
			// follows: emitting one after a partial stream would instead feed the
			// SDK accumulator an incomplete message that looks successful.
			h.writeSSEError(w, fmt.Sprintf("Upstream stopped sending data for %s", h.maxUpstreamIdle))
			flusher.Flush()
			return

		case <-ctx.Done():
			slog.Debug("client disconnected, aborting native /messages stream", "keepalive_pings", pings)
			return
		}
	}
}
```

### 6.4 调用侧(`handler.go:250-257`)

```go
	sse.BeginSSE(w)
	// Send the response headers immediately instead of waiting for the first
	// upstream byte: long thinking phases would otherwise stall the 200 OK
	// past client and intermediary response-header timeouts.
	flusher.Flush()

	h.pipeNativeStream(r.Context(), w, flusher, resp.Body)
	return
```

替换了原有的 26 行内联循环。

### 6.5 关键设计点速查

| 点 | 决策与理由 |
|---|---|
| 帧格式 | `event: ping` + `data: {"type": "ping"}`,与官方一致 |
| 边界跟踪 | `atBoundary = isBlankSSELine(line)`,复用 `handler.go:1140` 既有辅助函数;初值 `true` |
| 空闲语义 | 每次收到上游字节 `ticker.Reset()`。go.mod 为 `go 1.26.0`,Go 1.23+ 已修复 `Ticker.Reset` / `Timer.Reset` 的陈旧 tick 问题,两者均无需 drain |
| 上游自带 ping | 若上游发 ping,它走 `lines` 分支并重置两个计时器 → 我方自动不重复发,静默上限也不会误伤。天然协作,无需特判 |
| 静默上限计时 | 独立 `time.Timer`,仅由上游字节重置;我方 ping **不**重置它,否则保活会自我续命 |
| 中止语义 | 半截事件先补 `\n` 闭合,再发终结性 `error`,**不补 `message_stop`** |
| goroutine 回收 | teardown 顺序:主循环 `defer close(done)` → 外层 `defer resp.Body.Close()` 解除 `ReadBytes` 阻塞 → 读 goroutine 发现 `done` 已关闭后退出。无泄漏 |
| 禁用路径 | 两个特性都用 nil channel 技巧统一实现,避免多套代码路径带来的成倍测试面 |
| channel 容量 | 8,减少每行一次的 goroutine ping-pong |

---

## 7. 配置

| 环境变量 | 默认 | 说明 |
|---|---|---|
| `COPILOT2API_SSE_KEEPALIVE_SECONDS` | `15` | 原生 `/v1/messages` 流式响应的空闲保活间隔(秒)。`0` 禁用 |
| `COPILOT2API_SSE_MAX_IDLE_SECONDS` | `600` | 上游静默上限(秒),超时以终结性 `error` 事件中止该流。`0` 禁用。**与保活开关相互独立** |

- **15s 的依据**:低于常见中间层驱逐阈值(`docs/copilot2api-issues-retrospective.html:1158` 记录的 ~60–350s;AWS ALB 默认 60s),也低于社区向官方提出的 ≤30s 心跳诉求(同页 `:1159`),同时高于 Anthropic 官方观测的 ~10s 量级。开销为每 15s 约 32 字节。
- **600s 的依据**:见 §5.3。它只用于兜住卡死,不是"响应时间上限"。
- 两者都只接受**整秒**,故最小有效保活间隔为 1s。

---

## 8. 测试

`anthropic/native_stream_test.go` 共 16 个测试(含 12 个子测试),另有 `internal/sse/sse_test.go` 1 个。

### 8.1 保活主体

| 测试 | 断言 |
|---|---|
| `InjectsPingWhileIdle` | 静默 > interval 时注入 ping;**且剔除全部 ping 后与上游原文逐字节相等** |
| `NeverSplitsEvent` | 上游发出 `event: foo\n` 后卡住 → ping **不**被注入;再用 `readSSEEvent` 验证事件名与 data 完好 |
| `NoPingWhileActive` | 持续供数(10 帧 × 20ms,interval 100ms)→ 注入 0 个 ping,验证 `Reset` 生效 |
| `KeepAliveDisabled` | `interval=0` 完全不注入,输出与上游原文相同 |
| `ContextCancel` | `ctx` cancel 后及时返回(修复 B) |
| `StopsOnWriteFailure` | 首次 ping 写失败即返回(修复 B 的兜底路径) |
| `NoGoroutineLeak` | 20 轮提前中止后 `runtime.NumGoroutine()` 回落 |

### 8.2 真实链路(`httptest` server + 原始 TCP / HTTP 客户端)

| 测试 | 断言 |
|---|---|
| `HeadersFlushedImmediately` | 上游首字节延迟 700ms 时,响应头仍在其之前送达(修复 A)。用原始 TCP,因为 `ResponseRecorder` 无法反映真实的 header 发送时机 |
| `PingsReachClientIncrementally` | 首个 ping 早于上游 payload 到达,证明帧是逐个刷出而非末尾缓冲 —— in-memory recorder 的 `Flush` 是 no-op,覆盖不到这一维度 |

### 8.3 静默上限(§5.3)

| 测试 | 断言 |
|---|---|
| `AbortsOnUpstreamSilence` | 上限触发 → 先有 ping、后有 `event: error`、**无 `message_stop`**;error 帧可被 `readSSEEvent` 正确解析且 data 含 API 错误类型 |
| `SilenceCeilingResetsOnActivity` | 总时长(~300ms)远超上限(120ms)但单次间隔不超 → 不中止,输出与原文一致 |
| `SilenceCeilingWithoutKeepAlive` | `interval=0` 时上限仍然生效,且全程 0 个 ping |
| `SilenceCeilingDisabled` | `maxIdle=0` → 恢复无界行为 |
| `SilenceCeilingClosesPartialEvent` | 半截事件先被 `\n` 闭合;`readSSEEvent` 确认 error 帧事件名未被顶掉(§4.5) |

### 8.4 配置解析与响应头

| 测试 | 断言 |
|---|---|
| `TestKeepAliveIntervalFromEnv` | 6 子例:未设 / 空 / `0` 禁用 / 自定义 / 负数回退 / 非法回退 |
| `TestMaxUpstreamIdleFromEnv` | 同上 6 子例 |
| `internal/sse.TestBeginSSE` | 四个响应头齐备,含 `X-Accel-Buffering: no`(修复 D) |

### 8.5 测试辅助

- `scriptedBody` —— 按脚本逐段返回并在段前 `time.Sleep`,模拟可控延迟的慢上游
- `recorder` —— 并发安全的 `io.Writer` + `http.Flusher`;`failAt` 字段让第 N 次及之后的写入失败,用于模拟客户端断开
- `stripPings` / `countPings` —— 支撑"剔除 ping 后逐字节相等"这条核心不变量

**注意该不变量的边界**:它在正常完成路径上成立;中止路径会额外写入 `\n` + `error` 帧,这是有意的协议性输出,不属于损坏。

### 8.6 已知覆盖缺口

以下两点经变异测试(§9.1)确认,记录在案:

1. **修复 A 在生产代码中的存在没有被锁定。** `HeadersFlushedImmediately` 与 `PingsReachClientIncrementally` 都自建 `http.HandlerFunc`(`native_stream_test.go:280-290, 342-352`),在其中复刻 `BeginSSE + Flush` 的形状后直接调 `pipeNativeStream`,**不经过** `handleNativeMessagesPassthrough`。因此它们证明的是"这个模式确实能让响应头提前送达",而非"生产代码确实这么做了" —— 删掉 `handler.go:254` 的 `flusher.Flush()`,全部测试仍然通过。要覆盖它需要一个走完整 `ServeHTTP` 路径的测试(须搭配 mock upstream)。
2. **`SilenceCeilingClosesPartialEvent` 只覆盖 §4.5 的 A 组**(上游停在 `event:` 行后),而危害更大的 B 组(停在 `data:` 行后,error payload 被拼接成双行 JSON)无测试覆盖。防护逻辑对两者都有效,缺的只是断言。

---

## 9. 交付清单

`ec94ec8`:

- [x] `anthropic/handler.go` —— `nativeKeepAliveFrame`、`defaultKeepAliveInterval`、`keepAliveEnvVar`、`keepAliveIntervalFromEnv()`
- [x] `anthropic/handler.go` —— `Handler.keepAliveInterval`,`NewHandler` 中初始化
- [x] `anthropic/handler.go` —— `pipeNativeStream()`,替换 `handleNativeMessagesPassthrough` 中的内联循环
- [x] `anthropic/handler.go` —— `sse.BeginSSE(w)` 后补 `flusher.Flush()`(修复 A)
- [x] `anthropic/native_stream_test.go` —— 新建
- [x] `CHANGELOG.md` / `CHANGELOG.zh-CN.md` / `README.md` / `README.zh-CN.md`

`7a32b62`:

- [x] `anthropic/handler.go` —— `defaultMaxUpstreamIdle`、`maxUpstreamIdleEnvVar`、`durationFromEnv()`、`maxUpstreamIdleFromEnv()`
- [x] `anthropic/handler.go` —— `Handler.maxUpstreamIdle` 与 `pipeNativeStream` 的 `idleC` 分支(修复 C)
- [x] `internal/sse/sse.go` —— `X-Accel-Buffering: no`(修复 D)
- [x] `internal/sse/sse_test.go` —— 新建
- [x] `anthropic/native_stream_test.go` —— 补 6 个用例
- [x] `CHANGELOG.md` / `CHANGELOG.zh-CN.md` / `README.md` / `README.zh-CN.md`

验证命令:

```bash
go test ./anthropic/ ./internal/sse/ -race -count=1
go build ./...
```

`-race` 是必需的:本改动引入了 goroutine 间通信。

### 9.1 实测结果

```
go test ./anthropic/ ./internal/sse/ -race -count=1   ok (10.4s / 2.2s)
go test ./... -race -count=1                          全部包通过
gofmt / go vet / go build                             干净
```

注:`gofmt -l .` 报告的 17 个文件在改动前的 `main` 分支上已存在(经 `git stash` 比对确认),与本次改动无关。

**变异测试** —— 临时移除防护逻辑以确认测试非假阳性(2026-08-14 在 `7a32b62` 上重跑):

| 移除的逻辑 | 结果 |
|---|---|
| `if !atBoundary { continue }`(tick 分支,`handler.go:372-374`) | `NeverSplitsEvent` **FAIL**,精确复现 §4.2 B 组:`event: content_block_delta` 之后被塞入 6 个 ping,原事件名被 `ping` 覆盖,尾部 `data: {"index":0}` 降级为匿名事件 |
| 半截事件闭合(idleC 分支,`handler.go:387-391`) | `SilenceCeilingClosesPartialEvent` **FAIL**,产出 `event: content_block_delta\nevent: error\ndata:{...}` —— 两帧被合并 |
| `flusher.Flush()`(修复 A,`handler.go:254`) | **全部测试仍然通过** —— 见 §8.6 第 1 条,该行在生产代码中无测试锁定。改为移除测试自建 handler 里的对应行(`native_stream_test.go:283`)才会让 `HeadersFlushedImmediately` FAIL(响应头被扣留 701.5ms,上游延迟 700ms) |

---

## 10. 风险与取舍

| 风险 | 评估 |
|---|---|
| 上游在**单个事件中间**长时间静默 → 跳过 ping,保护失效 | 刻意取舍:宁可漏发心跳,也不产出破损 SSE 帧。实际风险极低——长静默发生在模型思考期,而思考期必然处于事件边界。极端情况下由静默上限兜底,以 `error` 中止而非无限挂起 |
| 静默上限误伤合法的长思考 | 默认 600s,远高于实测的上游行为与客户端自身的 ~300s 空闲底线;可经 `COPILOT2API_SSE_MAX_IDLE_SECONDS` 调整或设 `0` 关闭 |
| 中止路径破坏"剔除 ping 后逐字节等价"不变量 | 仅限中止路径。正常完成路径逐字节等价,由 `InjectsPingWhileIdle` / `NoPingWhileActive` / `SilenceCeilingResetsOnActivity` 共同锁定 |
| 下游是非标准 Anthropic 客户端,不认识 `event: ping` | 低。本路径是原生 `/v1/messages`,下游必为 Anthropic 兼容客户端;官方文档已声明可能出现任意数量 ping,合规客户端必须容忍 |
| 引入 goroutine 带来并发缺陷 | 由 `-race` 测试 + 单写者架构控制;`done` channel 保证无泄漏 |
| 禁用时仍走 goroutine 路径 | 开销为每行一次 channel 传递,实测规模(3.7 万行)下合计数毫秒。若要求禁用时字节级零差异,可保留原直通循环作为独立分支(不推荐:成倍测试面) |
| `X-Accel-Buffering` 影响了另两条路径 | 只是一个响应头,不识别者忽略;三条路径都是 SSE,禁用反代缓冲对它们一律正确 |

---

## 11. 后续(不在本次范围)

1. **`proxy/stream.go` 与 `gemini/handler.go` 仍无 ping 注入。** 修复 D 已通过 `BeginSSE` 覆盖到它们,但保活本身没有 —— 且**不能复用本方案的帧格式**:OpenAI / Gemini 客户端不认识 `event: ping`,需改用 SSE 注释行 `: ping\n\n`(任何合规解析器都会静默丢弃)。架构(读 goroutine + 单写者 select)可复用,届时宜下沉到 `internal/sse` 包。
2. **静默上限同样只在原生路径**,另两条路径的流依旧无界。
3. **上游响应头阶段的覆盖盲区**(§5.1.1)—— 需要与"透传上游错误状态码"的能力做取舍,当前选择了后者。
