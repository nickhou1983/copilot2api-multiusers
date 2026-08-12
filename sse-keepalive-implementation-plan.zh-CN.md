# 原生 `/v1/messages` 透传:SSE Keep-alive 实现计划

> 状态:**已实现**(提交 `ec94ec8`,分支 `feat/native-messages-sse-keepalive`)
> 范围:**仅** `anthropic/handler.go` 的 `handleNativeMessagesPassthrough` 的 `stream=true` 分支
> 不涉及:`proxy/stream.go`(OpenAI 透传)、`gemini/handler.go`、`anthropic` 的 OpenAI→Anthropic 转换路径(`streamSSE`)

---

## 1. 背景

### 1.1 现状:代理不发送任何 SSE 保活信号

对三条流式路径的代码审查与运行时实测结论:**copilot2api 当前不发送任何 SSE keep-alive**。

- `anthropic/handler.go:186-211`(原生透传)—— 纯 `reader.ReadBytes('\n')` 阻塞循环
- `proxy/stream.go:78` —— `for scanner.Scan()` 阻塞循环
- `gemini/handler.go:443` —— 同上
- 全仓 `time.NewTicker` 仅出现在 `auth/device_flow.go` 与 `internal/stats/stats.go`,与流式无关

唯一相关的是 `internal/sse/sse.go:9` 设置的 `Connection: keep-alive` **响应头** —— 这只是 HTTP/1.1 连接复用语义,HTTP/2 下被忽略,不产生任何字节。

实测(构造上游静默 3s 的场景,`proxy.streamResponse`):

```
bytes written to client after 2.5s of upstream silence: 0 -> ""
```

### 1.2 影响

与 `docs/copilot2api-issues-retrospective.html:1103-1105` 记录的失败类完全吻合:长推理静默期无字节流动,空闲 SSE 连接被中间层(NAT / CDN / 网关 / LB)在 ~60–350s 驱逐。

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
┌─ read goroutine ─┐        ┌──── main loop (唯一写者) ────┐
│ ReadBytes('\n')  │──ch──▶ │ select {                     │
│ (阻塞在这里)      │        │   case res := <-lines:  写行  │
└──────────────────┘        │   case <-tickC:       写 ping │
                            │   case <-ctx.Done():    退出  │
                            │ }                            │
                            └──────────────────────────────┘
```

一次性满足全部三个约束:

- 唯一写者 → 无需锁(约束 1)
- 主循环天然知道自己刚写了什么 → 边界可判定(约束 2)
- 收到数据即 `ticker.Reset()` → 空闲语义(约束 3)

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

### 4.2 实测:用仓库自己的 `readSSEEvent`(`handler.go:493`)解析

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

### 4.5 附带结论:为何命名事件比注释行"贵"

对照组 C 显示,`handler.go:520` 的 `if !strings.HasPrefix(line, ":")` 让注释行不参与 event/data 累积,**事件名不会被覆盖**,破坏程度轻一级(但仍触发提前 dispatch,故注释行同样不能随意乱插)。

这正是本路径选择命名事件所必须付出的代价 —— 而选择命名事件是正确的,因为下游必定是 Anthropic 客户端。

---

## 5. 两个顺带修复

### 5.1 修复 A:`BeginSSE(w)` 后立即 Flush

**原因:当前实现会把 HTTP 响应头扣留到上游首字节到达。**

`internal/sse/sse.go` 的 `BeginSSE` 只做三次 `Header().Set(...)`,不写任何字节;Go 的 `ResponseWriter` 要到第一次 `Write()` 才发送状态行与响应头。而 `handler.go:185` 之后紧接阻塞的 `ReadBytes`。

实测(模拟上游首字节延迟 2s):

```
当前实现 (BeginSSE 不 Flush)      首字节耗时  2.00s
修复后 (BeginSSE + Flush)       首字节耗时  0.00s
```

延迟的不是数据,而是**协议握手**。在客户端拿到响应头之前:

- **响应头超时会误杀请求** —— 客户端与中间层普遍对响应头单独计时且阈值更低(Go 的 `http.Transport.ResponseHeaderTimeout`、nginx `proxy_read_timeout` 默认 60s)。扩展思考期动辄数分钟不吐字节,这些计时器会在任何数据到达前掐断连接
- **SSE 解析器无法启动** —— 需先看到 `Content-Type: text/event-stream` 才切入流式解析
- **请求状态不可观测** —— 客户端分不清"已接受、正在思考"与"仍卡在连接/鉴权"

**这不是新要求,而是补齐既有惯例** —— 另两条路径均已如此:

- `proxy/stream.go:52-57`(注释 `// Flush headers`)
- `gemini/handler.go:199-204`

原生 `/v1/messages` 透传是三条路径中唯一遗漏的。

**与 keep-alive 互补,不可互相替代**:开启保活后首个 ping 的 `Write` 会隐式发送响应头,但需等满一个 interval(15s);且 `interval=0` 时问题原封不动。显式 Flush 将代价降至 0s 且不依赖保活开关。

### 5.2 修复 B:主动监听 `ctx.Done()` + ping 写失败即返回

**原因:当前实现在上游静默期对下游断连完全失明。**

现有循环仅在 `w.Write(line)` 失败时退出。上游静默时阻塞在 `ReadBytes`,**一次写操作都不会发生**,无从得知客户端已断开。

故障场景:客户端 60s 超时放弃,上游流长达 630s —— 代理继续读满剩余 570 秒,持续消耗 Copilot 配额、占用上游连接与 goroutine,直到上游流自然结束才发现对端早已消失。多账号部署下此类僵尸流会累积。

实测断连检测延迟:

```
[0.50s] 客户端强制关闭 TCP 连接
  ctx.Done()  在 0.50s 触发
  Write 探测: 第 5 次写入失败 @ 1.01s: write: broken pipe
```

| 机制 | 断连检测延迟 | 依赖 |
|---|---|---|
| 现状(仅 Write 失败) | 最长至上游流结束(可达 570s) | 上游恰好有数据要转发 |
| ping 的 Write 失败 | ≤ 1 个 interval(15s) | TCP RST 往返 |
| `ctx.Done()` | 即时 | 无 |

注意 Write 探测**滞后一拍**:客户端 0.50s 断开,但直到 0.50s 后的下一次写入(1.01s)才拿到 `broken pipe` —— 断开瞬间那次写只进了内核缓冲区,需等对端 RST 返回才反映到下一次写。

**决定:两者都保留。** `ctx.Done()` 负责即时退出;ping 写失败作为兜底,覆盖 ctx 未 cancel 但连接已不可写的边缘情况。单写者 select 架构让 `ctx.Done()` 分支的成本仅为两行。

---

## 6. 实现

### 6.1 新增常量与配置

```go
// nativeKeepAliveFrame 与 Anthropic 官方 ping 事件格式一致。官方 SDK 在
// _streaming.py 中以 `if sse.event == "ping": continue` 无条件跳过该事件
// 且不解析其 data,因此注入它对任何合规 Anthropic 客户端都是安全的。
const nativeKeepAliveFrame = "event: ping\ndata: {\"type\": \"ping\"}\n\n"

// defaultKeepAliveInterval 取 15s:低于常见中间层空闲驱逐阈值(AWS ALB 默认
// 60s),同时高于 Anthropic 官方观测到的 ~10s 量级。
const defaultKeepAliveInterval = 15 * time.Second

// keepAliveIntervalFromEnv 解析 COPILOT2API_SSE_KEEPALIVE_SECONDS。
// 0 表示禁用保活;非法值回退到默认值。
func keepAliveIntervalFromEnv() time.Duration {
	v := os.Getenv("COPILOT2API_SSE_KEEPALIVE_SECONDS")
	if v == "" {
		return defaultKeepAliveInterval
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		slog.Warn("invalid COPILOT2API_SSE_KEEPALIVE_SECONDS, using default",
			"value", v, "default_seconds", int(defaultKeepAliveInterval.Seconds()))
		return defaultKeepAliveInterval
	}
	return time.Duration(n) * time.Second
}
```

### 6.2 `Handler` 字段

```go
type Handler struct {
	upstream          *upstream.Client
	models            *models.Cache
	keepAliveInterval time.Duration
}

func NewHandler(authClient upstream.TokenProvider, transport *http.Transport, mc *models.Cache) *Handler {
	return &Handler{
		upstream:          upstream.NewClient(authClient, transport),
		models:            mc,
		keepAliveInterval: keepAliveIntervalFromEnv(),
	}
}
```

在 `NewHandler` 内部读 env,而非扩展签名 —— 与 `internal/accounts/config.go:42` 直接 `os.Getenv` 的既有惯例一致,可避免改动 `accounts_wire.go:63` 这一唯一调用点。测试可直接构造 `&Handler{keepAliveInterval: ...}`。

### 6.3 主体

```go
// pipeNativeStream 把上游 SSE 逐行转发给下游,并在上游空闲超过
// keepAliveInterval 时于事件边界注入 Anthropic ping 事件。
//
// 读取放在独立 goroutine 中,主循环是唯一写者:http.ResponseWriter 不是并发
// 安全的,且只有主循环能知道当前是否处于 SSE 事件边界。
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

	// keepAliveInterval <= 0 时 tickC 为 nil,select 永不选中该分支(保活禁用)
	var (
		ticker *time.Ticker
		tickC  <-chan time.Time
	)
	if h.keepAliveInterval > 0 {
		ticker = time.NewTicker(h.keepAliveInterval)
		defer ticker.Stop()
		tickC = ticker.C
	}

	atBoundary := true // 流开头即合法的事件边界
	pings := 0

	for {
		select {
		case res := <-lines:
			if len(res.line) > 0 {
				if _, err := w.Write(res.line); err != nil {
					slog.Error("failed to write native /messages stream", "error", err)
					return
				}
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

		case <-tickC:
			// 绝不切开一个 SSE 事件:切开会覆盖 event: 字段,使原事件降级为
			// 匿名事件而被客户端 SDK 静默丢弃。分片停顿是毫秒级,下个周期
			// 必已回到边界。
			if !atBoundary {
				continue
			}
			if _, err := io.WriteString(w, nativeKeepAliveFrame); err != nil {
				slog.Debug("client disconnected during keep-alive", "error", err)
				return
			}
			flusher.Flush()
			pings++

		case <-ctx.Done():
			slog.Debug("client disconnected, aborting native /messages stream", "keepalive_pings", pings)
			return
		}
	}
}
```

### 6.4 调用侧改动(`handler.go:185-211`)

```go
	sse.BeginSSE(w)
	flusher.Flush() // 修复 A:立即送出响应头,不等上游首字节

	h.pipeNativeStream(r.Context(), w, flusher, resp.Body)
	return
```

替换原有的 26 行内联循环。

### 6.5 关键设计点速查

| 点 | 决策与理由 |
|---|---|
| 帧格式 | `event: ping` + `data: {"type": "ping"}`,与官方一致 |
| 边界跟踪 | `atBoundary = isBlankSSELine(line)`,复用 `handler.go:963` 既有辅助函数;初值 `true` |
| 空闲语义 | 每次收到上游字节 `ticker.Reset()`。go.mod 为 `go 1.26.0`,Go 1.23+ 已修复 Ticker.Reset 的陈旧 tick 问题,无需 drain |
| 上游自带 ping | 若上游发 ping,它走 `lines` 分支并重置计时器 → 我方自动不重复发。天然协作,无需特判 |
| goroutine 回收 | teardown 顺序:主循环 `defer close(done)` → 外层 `defer resp.Body.Close()` 解除 `ReadBytes` 阻塞 → 读 goroutine 发现 `done` 已关闭后退出。无泄漏 |
| 禁用路径 | 用 nil channel 技巧统一实现,避免两套代码路径带来的双倍测试面。代价:禁用时仍多一次 channel 传递/行(3.7 万行合计约数毫秒,可忽略) |
| channel 容量 | 8,减少每行一次的 goroutine ping-pong |

---

## 7. 配置

| 环境变量 | 默认 | 说明 |
|---|---|---|
| `COPILOT2API_SSE_KEEPALIVE_SECONDS` | `15` | 原生 `/v1/messages` 流式响应的空闲保活间隔(秒)。`0` 禁用 |

选 15s 的依据:低于常见中间层驱逐阈值(`docs/copilot2api-issues-retrospective.html:1103` 记录的 ~60–350s;AWS ALB 默认 60s),同时高于 Anthropic 官方观测的 ~10s 量级。开销为每 15s 约 32 字节。

---

## 8. 测试计划

`anthropic` 包目前没有原生透传的流式测试,需新建(建议 `anthropic/native_stream_test.go`)。

| # | 用例 | 断言 |
|---|---|---|
| 1 | 上游静默 > interval | 下游收到 `event: ping\ndata: {"type": "ping"}\n\n` |
| 2 | **流内容完整性** | 剔除所有 ping 帧后,下游字节流与上游原文**逐字节相等** |
| 3 | **边界安全** | 上游发出 `event: foo\n` 后卡住(事件未闭合)→ 断言 ping **不**被注入 |
| 4 | 活跃流 | 持续供数时注入 0 个 ping(验证 `Reset` 生效) |
| 5 | `interval = 0` | 完全不注入,行为与改动前一致 |
| 6 | 响应头即时性 | 上游首字节延迟时,下游仍立即收到响应头(修复 A) |
| 7 | ctx 取消 | `ctx` cancel 后 `pipeNativeStream` 及时返回(修复 B) |
| 8 | goroutine 泄漏 | 客户端提前断开后 `runtime.NumGoroutine()` 回落 |

测试辅助:用可控延迟的 `io.Reader`(逐段返回 + `time.Sleep`)模拟慢上游;用实现了 `http.Flusher` 的 `httptest.ResponseRecorder` 包装器捕获输出。用例 6 需 `httptest.NewServer` + 原始 TCP 客户端(`ResponseRecorder` 无法反映真实的 header 发送时机)。

---

## 9. 交付清单

- [x] `anthropic/handler.go` —— 新增 `nativeKeepAliveFrame`、`defaultKeepAliveInterval`、`keepAliveIntervalFromEnv()`
- [x] `anthropic/handler.go` —— `Handler` 增加 `keepAliveInterval` 字段,`NewHandler` 中初始化
- [x] `anthropic/handler.go` —— 新增 `pipeNativeStream()`,替换 `handleNativeMessagesPassthrough` 中的内联循环
- [x] `anthropic/handler.go` —— `sse.BeginSSE(w)` 后补 `flusher.Flush()`(修复 A)
- [x] `anthropic/native_stream_test.go` —— 覆盖 §8 全部 8 个用例
- [x] `CHANGELOG.md` —— `## [Unreleased]` 下的 `Features` / `Bug Fixes`
- [x] `CHANGELOG.zh-CN.md` —— 同步
- [x] `README.md` / `README.zh-CN.md` —— 环境变量说明

验证命令:

```bash
go test ./anthropic/ -race -run 'Native|KeepAlive' -v
go build ./...
```

`-race` 是必需的:本改动引入了 goroutine 间通信。

### 9.1 实测结果(已完成)

```
go test ./... -race -count=1     全部包通过
gofmt / go vet / go build        干净
```

注:`gofmt -l .` 报告的 17 个文件在改动前的 `main` 分支上已存在(经 `git stash` 比对确认),与本次改动无关;本次新增/修改的两个文件格式干净。

**变异测试** —— 临时移除防护逻辑以确认测试非假阳性:

| 移除的逻辑 | 结果 |
|---|---|
| `if !atBoundary { continue }` | `NeverSplitsEvent` **FAIL**,精确复现 §4.2 B 组的损坏:`event: content_block_delta` 之后被塞入 6 个 ping,原事件名被覆盖 |
| `flusher.Flush()` | `HeadersFlushedImmediately` **FAIL**,响应头被扣留 702ms(上游延迟 700ms) |

**相对计划的偏离:**

1. 增补 `keepAliveEnvVar` 常量,使测试引用变量名而非硬编码字符串
2. 增补两个计划外用例:`StopsOnWriteFailure`(写失败兜底路径)与 `PingsReachClientIncrementally`(经真实 `httptest` server + HTTP 客户端验证 ping 实时刷新而非末尾缓冲——in-memory recorder 的 `Flush` 是 no-op,覆盖不到这一维度)

最终测试文件共 10 个测试(含 6 个子测试)。

---

## 10. 风险与取舍

| 风险 | 评估 |
|---|---|
| 上游在**单个事件中间**长时间静默 → 跳过 ping,保护失效 | 刻意取舍:宁可漏发心跳,也不产出破损 SSE 帧。实际风险极低——长静默发生在模型思考期,而思考期必然处于事件边界 |
| 下游是非标准 Anthropic 客户端,不认识 `event: ping` | 低。本路径是原生 `/v1/messages`,下游必为 Anthropic 兼容客户端;官方文档已声明可能出现任意数量 ping,合规客户端必须容忍 |
| 引入 goroutine 带来并发缺陷 | 由 `-race` 测试 + 单写者架构控制;`done` channel 保证无泄漏 |
| 禁用时仍走 goroutine 路径 | 开销为每行一次 channel 传递,实测规模下合计数毫秒。若要求禁用时字节级零差异,可保留原直通循环作为独立分支(不推荐:双倍测试面) |

---

## 11. 后续(不在本次范围)

`proxy/stream.go` 与 `gemini/handler.go` 存在同类问题,但**不能复用本方案的帧格式** —— OpenAI / Gemini 客户端不认识 `event: ping`,需改用 SSE 注释行 `: ping\n\n`(任何合规解析器都会静默丢弃)。架构(读 goroutine + 单写者 select)可复用,届时宜下沉到 `internal/sse` 包。
