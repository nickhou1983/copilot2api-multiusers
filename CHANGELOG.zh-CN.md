# 更新日志

[English](CHANGELOG.md) | [简体中文](CHANGELOG.zh-CN.md)

## [未发布]

### 新特性

- 新增可选的自动化 GitHub Device Flow 登录。新增 `login-agent` 容器（Node + Playwright），可代替人工完成登录 GitHub、填入 `user_code` 并点击 **Authorize**，因此在管理界面认证账号时无需人工打开浏览器。为每个账号存好 GitHub 用户名与密码（在管理界面设置，或创建账号时传 `github_username` / `github_password`），点击新增的 **Auto authenticate** 按钮即可。device flow 的轮询与凭证落盘逻辑完全未变——agent 只替代「人手点授权」这一步；一旦失败会回退到原有手动流程，界面继续显示 `user_code` 与链接，并附上浏览器停住那一页的截图。仅在设置 `COPILOT2API_LOGIN_AGENT_URL` 时启用（另有 `COPILOT2API_LOGIN_AGENT_TOKEN` 与 `COPILOT2API_LOGIN_AGENT_TIMEOUT_SECONDS`）；未设置时相关控件隐藏，行为保持不变。凭证以明文存储于 `<token-dir>/github_logins.json`（权限 `0600`，可用 `COPILOT2API_LOGINS_FILE` 配置），任何 API 响应都不会返回，日志也不会打印。要求账号**未开启**两步验证。同时提供根目录 `docker-compose.yml`，在内网中运行两个容器且不为 agent 发布任何端口。详见 [docs/auto-login.zh-CN.md](docs/auto-login.zh-CN.md)。
- 原生 `/v1/messages` 流式响应在长推理静默期保持连接存活:当上游持续 `COPILOT2API_SSE_KEEPALIVE_SECONDS` 秒(默认 `15`,设为 `0` 关闭)不发送任何字节时,代理会注入 Anthropic `ping` 事件(`event: ping` / `data: {"type": "ping"}`),避免空闲 SSE 连接被 NAT、CDN、网关或负载均衡器驱逐。ping 只会在 SSE 事件边界注入,因此剔除 ping 后转发的流与上游逐字节一致;符合规范的 Anthropic 客户端会直接丢弃 ping,不会影响消息累加。
- 为原生 `/v1/messages` 流式响应增加上游静默上限：当上游持续 `COPILOT2API_SSE_MAX_IDLE_SECONDS` 秒（默认 `600`，设为 `0` 关闭）不发送任何字节时，以终止性的 Anthropic `error` 事件中止该流，而非让保活 ping 无限维持连接。上游卡死时可及时释放客户端与上游连接；无论保活是否开启，该上限均生效。
- 所有 SSE 响应新增 `X-Accel-Buffering: no` 响应头。nginx 默认开启 `proxy_buffering`，会缓冲被代理的响应，导致逐帧 flush 的事件（包括保活 ping）要等到缓冲区写满或流结束才下发。部署在 nginx 及兼容反向代理之后无需额外配置即可正常流式输出；不识别该头的代理会直接忽略。
- 新增按账号的认证模式:`exchange`(默认,行为不变)通过 `copilot_internal/v2/token` 铸造短时 Copilot token;新增的 `direct` 模式直接把 GitHub OAuth token 作为 Copilot bearer,使用静态 base URL(`https://api.githubcopilot.com`)且无 token 刷新。可在 `accounts.json` 中按账号设置 `auth_mode`,或通过新的 `COPILOT2API_AUTH_MODE` 环境变量设置全局默认(账号设置优先)。GitHub Device Flow 按模式选择 client id(`exchange` → `Iv1.b507a08c87ecfe98`,`direct` → `Ov23li8tweQw6odWQebz`),出站请求使用与模式对应的请求头 profile:exchange 用 `editor`(VS Code Copilot Chat,与之前一致),direct 用 `opencode`(`User-Agent: opencode/…`、`Openai-Intent: conversation-edits`、`X-Initiator: user`,不含 editor / request-id 头)。管理界面与 API 支持在创建账号时设置 `auth_mode`,也支持更新时修改(会重建该账号的 auth client)。既有配置不受影响——缺省 `auth_mode` 即保持 exchange 行为。
- 原生 `/v1/messages`（及 `/v1/messages/count_tokens`）的上游请求发送 `anthropic-version: 2023-06-01` 与基础 beta `interleaved-thinking-2025-05-14`。客户端 `anthropic-beta` token 默认仍会被过滤，仅窄范围放行 Anthropic Computer Use 工具类型所需的 `computer-use-*`，并对重复白名单 token 去重；`context-1m*` token 仍只在本地消费用于切换 `-1m` 模型变体，不向上游转发。
- 原生 `/v1/messages`（及 count_tokens）自动注入 context-management beta：当请求 body 顶层含 `context_management` 字段时，代理在出站 `anthropic-beta` 头中追加 `context-management-2025-06-27` 与 `compact-2026-01-12`（与基础的 `interleaved-thinking-2025-05-14` 并列），客户端无需自带 beta 头即可使用上下文编辑与服务端压缩。
- 新增原生 Anthropic Token 计数端点：`POST /v1/messages/count_tokens` 现已转发到上游 Copilot 的 Token 计数接口（此前返回 `404`）。请求会与 `/v1/messages` 一样做模型别名解析与 `cache_control.scope` 剥离，并原样返回上游的 `{ "input_tokens": N }` 响应。
- 在原生 `/v1/messages` 请求上透传 `context_management` 而非剥离它：当请求体包含 `context_management` 字段时，代理会在发往上游的请求体中保留该字段。
- 新增多账号支持：通过 `accounts.json` 配置文件把 API Key 与 GitHub 账号 1:1 映射。每个账号使用独立的凭证存储与各自的模型缓存，因此 Token 刷新与基于能力的路由都按账号隔离。可用 `COPILOT2API_ACCOUNTS_FILE` 配置文件路径（默认为 `<token-dir>/accounts.json`）。
- API Key 从 `Authorization: Bearer`、`x-api-key`、`x-goog-api-key` 或 `?key=` 查询参数中提取，覆盖 OpenAI、Anthropic 与 Gemini 客户端。
- 新增 `/admin/` Web 管理界面（仅多账号模式）用于维护 API Key ↔ GitHub 账号映射：列出、新增、轮换 Key、删除账号，并支持通过浏览器驱动的 GitHub Device Flow 认证账号。改动会保存到 `accounts.json` 并实时生效、无需重启。可选用 `COPILOT2API_ADMIN_TOKEN`（以 `X-Admin-Token` 头或 `?admin_token=` 传入）加以保护。
- 支持从空的 `accounts.json`（`{"accounts":[]}`）引导多账号模式，并完全通过管理界面填充。
- 在管理界面新增 Token 用量统计页（新增「Stats」标签页），按账号、按模型展示 Token 计数 —— 输入、输出、缓存命中（prompt-cache）、缓存写入以及请求总数 —— 覆盖所有 OpenAI、Anthropic 与 Gemini 端点。用量持久化到 `<token-dir>/stats.json`，重启后仍保留。由新增的 `GET /admin/api/stats` 端点提供，`DELETE /admin/api/stats/{id}` 可重置单个账号。注意：OpenAI Chat Completions 流式仅在客户端发送 `stream_options.include_usage` 时才计入 Token 数；但请求本身始终计数。
- 在管理界面新增上游模型页（新增「Models」标签页），按所选账号列出 GitHub Copilot 上游支持的模型 —— 模型 ID、厂商、版本、上下文窗口、最大输出 Token 数、支持的端点，以及 preview/picker 标记。由新增的 `GET /admin/api/accounts/{id}/models` 端点提供，返回该账号缓存的上游 `/models` 响应。
- 在管理界面 Models 标签页新增手动更新模型列表功能：新增「Update from upstream」按钮，强制绕过缓存 TTL 从上游重新拉取 `/models` 响应，并替换该账号用于能力路由的模型缓存。由新增的 `POST /admin/api/accounts/{id}/models/refresh` 端点提供。原有「Refresh」按钮仍为重新读取缓存列表。
- 在管理界面 Stats 标签页新增「Cache hit」列，按模型与账号合计展示提示词缓存命中率，计算公式为 cached / (input + cached + cache write)（基于输入侧 Token）。无输入 Token 记录时显示"—"。

### Bug 修复

- 修复自动化 GitHub Device Flow 登录报错 `Device code input was not found on the verification page.`（`step: device_code`）的问题。当浏览器 profile 中已有 GitHub 会话时，`https://github.com/login/device` 会重定向到 **Device Activation** 账号确认页（`/login/device/select_account`，显示「Signed in as …」与 **Continue** 按钮），而不是直接渲染验证码表单。现在 agent 会先点过该页面再填入 `user_code`；若 GitHub 直接跳到授权页，也会跳过填码步骤直接进入授权。此问题只影响同一账号的第二次及后续自动登录，首次（profile 为空）不受影响。
- 修复原生 `/v1/messages` 流式响应把 HTTP 响应头扣留到上游首字节到达的问题。现在上游流建立后会立即 flush 响应头(含 `Content-Type: text/event-stream`),长推理静默期不会再在产出任何数据之前就触发客户端与中间层的「响应头超时」。此行为与 OpenAI、Gemini 流式路径原本的做法一致。
- 修复原生 `/v1/messages` 流在客户端断开后仍继续读完上游响应的问题。现在请求 context 被取消(或写入失败)时会立即中止并释放上游连接,不再继续读取剩余响应 —— 长流场景下此前可能白白多跑数分钟。
- 修复 `POST /v1/responses` 对所有上游不原生支持 Responses API 的模型（如 `claude-*`、`gemini-*`、`kimi-k2.7-code`)一律返回 `400 "Invalid JSON in request body"` 的问题。Responses→Chat Completions 转换路径此前只把 `input` 解析为数组，导致 OpenAI 官方文档中的字符串简写形式（`"input": "hello"`）解析失败。现在 `input` 同时接受纯字符串与输入项数组。
- 修复 `POST /v1/responses` 在转换为 Chat Completions 时丢弃缺省 `type` 字段的输入项的问题（表现为上游返回 `400 "messages must be non-empty"`）。携带 `role` 的输入项现在按消息处理，与 Responses API 中 `type` 默认为 `"message"` 的行为一致。
- 修复 `direct` 认证模式账号的 `GET /usage`:改用原始 GitHub OAuth token 查询 `copilot_internal/user`。响应现在包含账号的 Copilot 套餐、配额重置日期以及 `chat`、`completions`、`premium_interactions` 配额快照,不再错误地走仅适用于 exchange 模式的 `copilot_internal/v2/token` 路径。
- 修复 `search_result` 内容块被以 `400 "content must be string or array of blocks"` 拒绝的问题。search_result 块携带的是裸字符串 `source`，而代理此前只建模了对象形式的 image `source`，导致在请求到达上游之前整个内容数组解析就失败。`AnthropicImageSource` 现同时接受对象 source 与裸字符串 source，恢复了 `search_result` 块的原生透传 —— Copilot 上游支持该能力，会返回 `search_result_location` 引用。Chat Completions 与 Responses 转换路径会把 `search_result` 块降级为纯文本（保留内容，丢弃这两类 API 无法表达的引用元数据）。
- 修复 `anthropic-beta: context-1m` 头（Claude Code 使用）的 1M 上下文处理：代理不再盲目给模型 ID 追加 `-1m` 后缀，而是仅在基础模型尚未声明 1M 上下文窗口、且该 `-1m` 变体在上游确实存在时才切换。较新的 Claude 模型（如 `claude-sonnet-4.6`、`claude-opus-4.6/4.7/4.8`）在基础模型 ID 上即暴露 1M，因此请求 1M 上下文不再生成上游不存在的 `-1m` 模型 ID，避免破坏能力探测与路由。

### 兼容性

- 将发送给 GitHub Copilot 上游的 `X-Github-Api-Version` 头从 `2025-04-01` 升级为 `2026-06-01`。对客户端 API 无变化，仅影响代理向上游声明的版本。
- 代理现在始终以多账号模式运行：启动时若不存在 `accounts.json`，会自动创建为空配置（`{"accounts": []}`）并默认启用管理界面。请求必须携带有效的 API Key，否则返回 `401 Unauthorized`；在至少配置一个账号（如通过管理界面）之前，所有请求都会被 `401` 拒绝。此行为取代了此前配置文件缺失时的单账号、无校验回退模式。

### 文档

- 新增 `docs/auto-login-design.md` 与 `docs/auto-login-design.zh-CN.md` —— 自动化 Device Flow 登录的**方案与业务流**说明文档，面向「想了解方案本身而非代码改动」的读者。内容包括：为什么只有浏览器授权那一步需要自动化（GitHub 不返回 `verification_uri_complete`，`user_code` 必须被输入到页面）、四条设计原则、代理与浏览器 agent 的职责边界、两份互不相干的凭证存储与两个独立状态机（账户认证状态 vs. 自动化执行状态）、把轮询与自动化画成并行支路的端到端时序图、agent 内部五个执行阶段、七类失败分类及各自对应的处置动作、回退路径、含明文密码取舍在内的安全模型，以及已知边界。
- 新增 `docs/auto-login-slides.html` —— 自包含的 12 页 16:9 HTML 说明文档，完整讲解自动化 Device Flow 登录方案：双容器拓扑与内网边界、`login-agent` 容器的职责边界与四个内部模块（接口与排队、浏览器编排、页面选择器、纯逻辑判定）、agent 内部的六步登录时序、七类错误分类及其触发的手动回退、管理界面各状态、安全模型（明文 `0600` 凭证文件、agent 不发布端口、密码不入响应也不入日志）、compose 与环境变量配置、验证结果，以及已知边界（不支持 2FA、陌生 IP 邮箱验证、GitHub 页面改版、登录请求串行排队）。浏览器直接打开，无需构建；方向键 / 空格 / 滚轮 / 滑动翻页。
- 新增 `docs/upstream-messages-live-tests.zh-CN.md` —— Copilot 上游 `/v1/messages` 实测记录（2026-07-21/22，07-25 复测），经代理与直连上游两条链路交叉验证：web_search / web_fetch / 图片 URL 在网关层对全部模型拦截（含 Anthropic 最新工具版本，并以 base64 图片与普通工具作对照证明为定向拦截）；structured outputs 在 Anthropic / Bedrock 落点可用、Vertex 落点被 GCP 组织策略拦截——模型与后端**不是固定映射**且随时间漂移，故避开特定模型并非有效规避手段（应对该组织策略 400 做自动重试）；同步 `max_tokens` 真实上限 128K / 64K（高于 `/models` 宣告值；`/v1/messages` 拒绝 300K，因为 Anthropic 将其限定在异步 Message Batches API）；SSE 流约 630 秒生存期，以 HTTP/2 RST_STREAM 终止且无任何终止事件——附各模型吞吐实测与客户端处理建议。
- 新增 `docs/auth-flow.md` 与 `docs/auth-flow.zh-CN.md` —— 端到端的认证流程参考,涵盖下游 API Key 校验与上游 GitHub Device Flow,包括按模式区分的 Device Flow client id（`exchange` → `Iv1.b507a08c87ecfe98`，`direct` → `Ov23li8tweQw6odWQebz`）、exchange 与 direct 两种 token 获取方式、`editor`/`opencode` 请求头 profile、token 刷新,以及管理界面驱动的登录。已从 README「自动认证」特性处链接。
- 新增 `docs/copilot2api-issues-retrospective.html` —— 27 页 16:9 HTML 复盘报告（与能力报告同款 TD 档案风格与翻页交互），汇总 2026-06-10 ~ 08-13 期间测试与咨询过的全部问题，按「现象 → 分析 → 解决方案」组织：上游能力缺口、413 与超长上下文、Thinking Signature、Structured Outputs、Token 刷新与直连认证、SSE 断流与原生 `/v1/messages` Ping 保活机制（含仅保护下游链路、无法解决 630 秒硬上限及 WriteTimeout 的边界，以及 Claude Code 2.1.222+ 使用 Ping 避免 `Check your network` watchdog 误报的条件）、Client-ID 模型可见性、双模式真实调用与 Claude 地域过滤，并新增两页完整的通用功能清单（文本 / 中文 / SSE、多模态、1M 上下文、请求控制、工具调用、推理、结构化输出、缓存、Context Editing、Compaction、Computer Use、路由相关 Code Execution 等）及 Anthropic beta 支持清单；同时保留 beta 头接受度、后端路由复测与缓存命中机制，并新增一页 `cache_control` 上游实测（2026-08-14，直连 `api.enterprise.githubcopilot.com`、模型 `claude-opus-4.8`）：确认块级 ephemeral 写/读、`scope` 被拒与 1024 token 最低门槛，并发现两处相较旧结论的上游变化 —— 顶层自动 `cache_control` 现已被接受（旧为 400），1h TTL 现无需 `extended-cache-ttl-2025-04-11` beta 头即可生效；另经 copilot2api 代理复测，7 项中 6 项与直连逐字节一致，唯一分叉为刻意保留的 `cache_control.scope` 剥离（直连 400 → 经代理 200 并成功建缓存）。另新增一页 beta 头功能级复测（2026-08-19，直连 `api.githubcopilot.com`、模型 `claude-opus-4.8`）：对 Anthropic 官方文档中仍标注为「必需」的 28 个 `anthropic-beta` token 逐个跑三探针（仅带头、带头＋真实功能载荷、同载荷但不带头），把「网关接受」与「功能真正生效」区分开。结果仅 3 个在上游确实必需且生效 —— `computer-use-2025-11-24`、`context-management-2025-06-27`、`compact-2026-01-12`（不带头 400、带头 200），与代理现有的转发/注入策略完全重合，因此白名单无需扩大；另有 6 个被接受但完全无效（其中 `extended-cache-ttl-2025-04-11` 以全新 cache key 反序对照证明 1h TTL 桶在带头与不带头时同样被写入，`thinking-token-count-2026-05-13` 的 `thinking_tokens` 字段默认即已返回），4 个头被放行但在 Copilot 当前同步接口不可用（`output-300k`、两个 `server-side-fallback` 版本、`computer-use-2025-01-24`），5 个 token 的黑名单与 2026-07-26 完全一致 —— 进一步印证剥离未知客户端 beta 是必需的保护：一旦带上 `files-api-2025-04-14` 这类黑名单 token，整个请求会以 400 失败。另新增 300K Output 专页，明确 Anthropic 仅在异步 Message Batches API（`/v1/messages/batches` + `output-300k-2026-03-24`）开放 300K，而同步 `/v1/messages` 与 Copilot 上游仍受 128K 上限约束。
- 将复盘报告从 27 页扩展为 31 页，在开头新增四页使用前须知：账户命名避免规律化；基于源码的 OpenCode/Direct 与 VS Code/Exchange 请求头对照（含动态 initiator、视觉、编辑器、集成与请求 ID 字段）；出站 `anthropic-beta` 白名单与 Copilot 网关五项黑名单；以及开发者身份隔离、账户池 Session 粘性与健康感知负载策略。
- 从复盘 HTML 中删除 Computer Use 相关介绍，同时保留其他 beta 头过滤、能力、实测与运维说明。
- 新增独立的 7 页 `docs/copilot2api-quick-start.html` 并单独发布为 `quick-start.html`；第 3 页对比 OpenCode/Direct 与 VS Code/Exchange 的身份验证流程，从 GitHub Device Flow 到直接使用 bearer，或交换并刷新短时 Copilot token。
- 调整两份 HTML 开篇的身份验证建议，优先采用 OpenCode Device Flow、直接使用 GitHub OAuth bearer，并配套 `ProfileOpenCode` 请求头画像。
- 将独立 quick-start deck 重命名为「访问 GitHub 模型注意事项」，并同步调整封面摘要与元数据，使其与 7 页部署和接入范围一致。
- 将 `docs/copilot-capability-report.html` 从滚动长页报告重新设计为 17 页 16:9 HTML 幻灯片（支持键盘 / 滚轮 / 触摸翻页、打印导出 PDF、页内文本编辑）；报告内容全部保留。
- 在 `README.md` 与 `README.zh-CN.md` 中记录 `/v1/messages/count_tokens` 端点及原生透传字段（`context_management`、`search_result`）（Features 列表与 API 端点表）。
- 在 README 中记录多账号、管理界面与 Token 用量统计，并新增简体中文翻译（`README.zh-CN.md`、`CHANGELOG.zh-CN.md`）及语言切换链接。
- 新增 `scripts/capability-request-response-guide.md` —— 一份基于全量矩阵实测生成的中文学习指南：逐条列出每个能力用例的作用说明、实际发送的请求（端点、beta 头、JSON 请求体）与观测到的响应（解析后的 JSON，流式用例为 SSE 事件样本），并在代理与直连上游行为不同处单独标注。
- 新增 `scripts/capability-request-response-guide.fr.md` —— 能力请求/响应指南的法语翻译版，覆盖与中文原版相同的用例与实测数据。

### 测试

- 新增 `scripts/capability_test.py`，一个零依赖的能力对比测试器：对真实 GitHub Copilot 上游与运行中的 copilot2api 代理执行同一套 Anthropic Messages API 测试矩阵，并输出 Markdown 对比报告及脱敏后的原始 JSON 附件。支持 `--target direct|proxy|both`（可选 `--start-proxy` 自动拉起本地代理）。矩阵覆盖约 36 项能力 —— 文本/流式、function 与并行工具、`tool_choice` 变体、采样参数（`temperature`/`top_p`/`top_k`/`stop_sequences`/`metadata`/`service_tier`）、视觉、PDF 文档、扩展/交错思考、server 工具、prompt 缓存（含 1 小时 `extended_cache_ttl`）、`context_management`、`count_tokens`、`structured_outputs`、`search_result`、citations、1M 上下文，以及拒绝类用例（web search、computer use、web fetch、code execution）。它能精确定位代理与上游的差异；在本版修复之后，原生路径上仅剩 `cache_control.scope` 一处刻意保留的差异，而转换路径（`/responses`、`/chat/completions`）仍会丢弃部分字段（如 `stop_sequences`、`disable_parallel_tool_use`）。存储的 Token 绝不会被打印或写入输出。详见 `scripts/README.md`。
- 扩展能力矩阵以覆盖官方 Claude 平台功能清单的剩余部分（新增 12 项用例，均以真实上游校准）。实测确认 Copilot 上游支持：`strict_tool_use`（严格工具调用，结构化输出的另一半）、`tool_search`（GA 的 tool-search server 工具 + `defer_loading`）、`compaction`（经由代理新增的自动 beta 头）。固化为拒绝/缺失行为：`auto_prompt_cache`（顶层 `cache_control`）、`inference_geo`、`mcp_connector`、`programmatic_tool_calling`、`agent_skills`、`advisor_tool`、`server_side_fallback`，以及独立端点探针 `batches_endpoint`（`/v1/messages/batches`）与 `files_endpoint`（`/v1/files`）（直连与代理均为 404）。
- 将 `code_execution` / `code_execution_beta_header` 的期望重新校准为**拒绝**：Copilot 上游已将 `code_execution` server 工具从其接受的工具类型列表中移除（早前运行确实执行过代码）。两个用例保留为行为固化探针，若上游恢复该工具将转为 DIFF 报警。
- 能力测试的原始 JSON 附件现在会记录每个用例的出站请求（方法、端点、`anthropic-beta`、截断后的请求体）以及流式 SSE 事件样本，可用一次运行学习真实的请求/响应形态。

## [0.3.1] - 2026-04-26

### Bug 修复

- 修复 Anthropic thinking 签名被作为独立块发出、而非附加到当前打开的 thinking 块的问题
- 修复 Docker 镜像崩溃（`exec /copilot2api: no such file or directory`）—— 由 `scratch` 镜像中动态链接的二进制导致，已在 CI 交叉编译中加入 `CGO_ENABLED=0`
- 修复 Docker 多架构构建：因 `ARG TARGETARCH=amd64` 默认值覆盖了 buildx 的自动平台参数，arm64 镜像曾误打包 amd64 二进制
- 修复 CI 在 tag 推送时触发冗余运行 —— `on: push` 现仅限定到 `main` 分支

### CI

- 新增 Docker 冒烟测试 —— 在推送到镜像仓库前以 `docker run --version` 作为门禁，防止损坏的镜像进入仓库

### 文档

- 刷新 README 的快速开始与示例

## [0.3.0] - 2026-04-03

### 新特性

- 新增兼容 Gemini 的 `/v1beta/models` 端点以支持本地 `gemini-cli`，包括 `generateContent`、`streamGenerateContent` 与 `countTokens`
- 在 Gemini `/v1beta/models` 接口上暴露完整的上游模型列表，不再将列表限制为小范围白名单
- 在 `/v1/chat/completions` 与 `/v1/responses` 之间新增智能回退路由，使模型仅支持其中一个 OpenAI 兼容端点时请求仍可工作
- 改进两个端点间的 OpenAI 请求转换兼容性，包括对系统指令、结构化输出、tool choice、推理状态以及 `previous_response_id` 的更好处理
- 改进 Claude Code 原生 `/v1/messages` 兼容性：在转发到上游前移除不受支持的透传字段
- 新增 AmpCode 支持：`/amp/v1/*` 与 `/api/provider/*` 的 chat completions 经由 Copilot API；管理路由（`/api/*`）与登录重定向反向代理到 `ampcode.com`

## [0.2.0]

### 性能

- 在 Anthropic 流式中批量刷新 SSE —— 每个上游事件刷新一次，而非每个转换后的事件刷新一次（系统调用减少约 3-5 倍）
- 在原生 `/v1/messages` 透传中按 SSE 事件边界刷新，而非逐行刷新（系统调用减少约 3 倍）
- 将模型别名 body 重新编码延迟到仅原生透传路径执行 —— Responses 与 Chat Completions 路径完全跳过该 JSON 往返
- 移除 `writeSSEEvent` 中不必要的 `string()` 拷贝

### 架构

- 整合模型缓存 —— 单次上游 `/models` 拉取同时填充原始 JSON（用于代理）与解析后的模型信息（用于能力探测），消除重复 HTTP 调用
- 整合后移除无用的 `internal/cache` 包
- 将请求体大小限制集中为 `upstream.MaxRequestBody` 常量（此前为分散在 3 个文件中的魔法数字 `10<<20`）
- 所有流式路径统一通过 `sse.BeginSSE()` 设置 SSE 头

### 日志

- 每个请求在完成时输出一条 nginx 风格的访问日志，包含 method、endpoint、model、route、duration
- 通过 `upstream.LogRequestError` 将客户端断开 / 上下文取消错误从 ERROR 降级为 WARN
- 在 Token 刷新日志中加入 `duration_ms`
- 将关键请求生命周期日志提升到 Info 级别（此前全为 Debug —— 默认模式下不可见）
- 从流式热路径移除嘈杂的逐 chunk / 逐 event 调试日志
- 在 Anthropic 访问日志中加入 `route` 字段（`native`、`responses`、`chat_completions`）
- 为与 proxy handler 保持一致，在 Anthropic 访问日志中加入 `endpoint` 字段
- 新增模型缓存未命中的调试日志

### Bug 修复

- 修复 OpenAI Chat Completions 响应中 choices 被拆分的问题 —— 将来自不同 choices 的 text 与 tool_calls 合并到同一条 Anthropic 消息
- 修复流式事件中 `AnthropicContentBlockDelta` / `AnthropicMessageDelta` 类型混淆
- 移除 thinking 块中硬编码的 "Thinking..." 占位文本
- 在流式 chunk 中请求 usage（`stream_options.include_usage`），使 `message_delta` 获得真实的输出 Token 计数

### 新特性

- 1M 上下文窗口支持 —— 检测到 `anthropic-beta: context-1m-...` 头时自动追加 `-1m` 后缀
- 在 README 中记录 1M 上下文窗口用法

## [0.1.0]

- 初始提交
