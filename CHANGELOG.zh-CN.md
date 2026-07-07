# 更新日志

[English](CHANGELOG.md) | [简体中文](CHANGELOG.zh-CN.md)

## [未发布]

### 新特性

- 将管理界面与 `/admin/api/*` 端点从公开 API 监听器拆到独立管理监听器（`COPILOT2API_ADMIN_HOST` / `COPILOT2API_ADMIN_PORT`，默认 `0.0.0.0:7778`）。`COPILOT2API_HOST` / `COPILOT2API_PORT` 上的公开 API 监听器不再服务 `/admin`，因此部署时可只暴露推理接口而不暴露账号管理面。
- 管理界面现在必须通过 `COPILOT2API_ADMIN_USERNAME` 与 `COPILOT2API_ADMIN_PASSWORD` 用户名/密码登录。登录会话使用 HttpOnly SameSite Cookie；旧的 `COPILOT2API_ADMIN_TOKEN` 仅作为已废弃的 `X-Admin-Token` 脚本兼容路径保留。
- 在原生 `/v1/messages`（及 `/v1/messages/count_tokens`）路由上支持 Computer Use 工具：代理会把客户端的 `computer-use-*` beta 头转发到上游。代理仍不会盲目转发任意客户端 `anthropic-beta` 头，但现在放行 `computer-use-2025-11-24`（Claude Opus 4.8/4.7/4.6、Sonnet 4.6 等）与 `computer-use-2025-01-24`（更旧的模型），并与自动注入的 `context-management` beta 合并成单个 `anthropic-beta` 值。Copilot 上游本就支持 computer use；此前该 beta 头被剥离，导致 `computer_20251124` / `computer_20250124` 工具型被 `400` 拒绝。不带 `computer-use-*` 头的请求不受影响。
- 新增原生 Anthropic Token 计数端点：`POST /v1/messages/count_tokens` 现已转发到上游 Copilot 的 Token 计数接口（此前返回 `404`）。请求会与 `/v1/messages` 一样做模型别名解析与 `cache_control.scope` 剥离，并原样返回上游的 `{ "input_tokens": N }` 响应。
- 在原生 `/v1/messages` 请求上透传 `context_management` 而非剥离它。当请求体包含 `context_management` 字段时，代理会保留该字段，并自动为上游请求加上 `anthropic-beta: context-management-2025-06-27` 头，使上下文编辑（如 `clear_tool_uses_20250919`）真正生效，并在 `usage` / `context_management.applied_edits` 中回传结果。
- 新增多账号支持：通过 `accounts.json` 配置文件把 API Key 与 GitHub 账号 1:1 映射。每个账号使用独立的凭证存储与各自的模型缓存，因此 Token 刷新与基于能力的路由都按账号隔离。可用 `COPILOT2API_ACCOUNTS_FILE` 配置文件路径（默认为 `<token-dir>/accounts.json`）。
- API Key 从 `Authorization: Bearer`、`x-api-key`、`x-goog-api-key` 或 `?key=` 查询参数中提取，覆盖 OpenAI、Anthropic 与 Gemini 客户端。
- 新增 `/admin/` Web 管理界面（仅多账号模式）用于维护 API Key ↔ GitHub 账号映射：列出、新增、轮换 Key、删除账号，并支持通过浏览器驱动的 GitHub Device Flow 认证账号。改动会保存到 `accounts.json` 并实时生效、无需重启。
- 支持从空的 `accounts.json`（`{"accounts":[]}`）引导多账号模式，并完全通过管理界面填充。
- 在管理界面新增 Token 用量统计页（新增「Stats」标签页），按账号、按模型展示 Token 计数 —— 输入、输出、缓存命中（prompt-cache）、缓存写入以及请求总数 —— 覆盖所有 OpenAI、Anthropic 与 Gemini 端点。用量持久化到 `<token-dir>/stats.json`，重启后仍保留。由新增的 `GET /admin/api/stats` 端点提供，`DELETE /admin/api/stats/{id}` 可重置单个账号。注意：OpenAI Chat Completions 流式仅在客户端发送 `stream_options.include_usage` 时才计入 Token 数；但请求本身始终计数。

### Bug 修复

- 修复 `search_result` 内容块被以 `400 "content must be string or array of blocks"` 拒绝的问题。search_result 块携带的是裸字符串 `source`，而代理此前只建模了对象形式的 image `source`，导致在请求到达上游之前整个内容数组解析就失败。`AnthropicImageSource` 现同时接受对象 source 与裸字符串 source，恢复了 `search_result` 块的原生透传 —— Copilot 上游支持该能力，会返回 `search_result_location` 引用。Chat Completions 与 Responses 转换路径会把 `search_result` 块降级为纯文本（保留内容，丢弃这两类 API 无法表达的引用元数据）。
- 修复 `anthropic-beta: context-1m` 头（Claude Code 使用）的 1M 上下文处理：代理不再盲目给模型 ID 追加 `-1m` 后缀，而是仅在基础模型尚未声明 1M 上下文窗口、且该 `-1m` 变体在上游确实存在时才切换。较新的 Claude 模型（如 `claude-sonnet-4.6`、`claude-opus-4.6/4.7/4.8`）在基础模型 ID 上即暴露 1M，因此请求 1M 上下文不再生成上游不存在的 `-1m` 模型 ID，避免破坏能力探测与路由。

### 兼容性

- 代理现在始终以多账号模式运行：启动时若不存在 `accounts.json`，会自动创建为空配置（`{"accounts": []}`）并默认启用管理界面。请求必须携带有效的 API Key，否则返回 `401 Unauthorized`；在至少配置一个账号（如通过管理界面）之前，所有请求都会被 `401` 拒绝。此行为取代了此前配置文件缺失时的单账号、无校验回退模式。

### 文档

- 记录独立管理监听器、必填的管理登录环境变量、Docker 端口映射，以及 Azure Application Gateway 仅暴露公开 API 监听器的部署建议。
- 在 `README.md` 与 `README.zh-CN.md` 中记录 `/v1/messages/count_tokens` 端点及原生透传字段（`context_management`、`search_result`）（Features 列表与 API 端点表）。
- 在 README 中记录多账号、管理界面与 Token 用量统计，并新增简体中文翻译（`README.zh-CN.md`、`CHANGELOG.zh-CN.md`）及语言切换链接。

### 测试

- 新增 `scripts/capability_test.py`，一个零依赖的能力对比测试器：对真实 GitHub Copilot 上游与运行中的 copilot2api 代理执行同一套 Anthropic Messages API 测试矩阵，并输出 Markdown 对比报告及脱敏后的原始 JSON 附件。支持 `--target direct|proxy|both`（可选 `--start-proxy` 自动拉起本地代理）。矩阵覆盖约 36 项能力 —— 文本/流式、function 与并行工具、`tool_choice` 变体、采样参数（`temperature`/`top_p`/`top_k`/`stop_sequences`/`metadata`/`service_tier`）、视觉、PDF 文档、扩展/交错思考、server 工具、prompt 缓存（含 1 小时 `extended_cache_ttl`）、`context_management`、`count_tokens`、`structured_outputs`、`search_result`、citations、1M 上下文，以及拒绝类用例（web search、computer use、web fetch、code execution）。它能精确定位代理与上游的差异；在本版修复之后，原生路径上仅剩 `cache_control.scope` 一处刻意保留的差异，而转换路径（`/responses`、`/chat/completions`）仍会丢弃部分字段（如 `stop_sequences`、`disable_parallel_tool_use`）。存储的 Token 绝不会被打印或写入输出。详见 `scripts/README.md`。

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
