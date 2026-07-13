# GitHub Copilot 模型能力与 Copilot2API 支持总结报告

> 生成日期:2026-07-13。本报告汇总本仓库全部实测结论,按「最新结论优先」原则整理——凡早期结论已被后续复测修正的(如 computer use、sonnet-4.6 托管落点),一律以最新结论为准并注明修正历史。
>
> 数据来源(按时间倒序):
> - `scripts/out/session-test-report-2026-07-12.md` — Computer use 端到端 / 托管平台指纹 / Structured outputs 全模型遍历与平台约定矩阵(2026-07-12 ~ 07-13,**最新**)
> - `scripts/opus48-capability-report.md` — Opus 4.8 三层对照 47 项(2026-06-27 生成,2026-07-07 修订)
> - `scripts/sonnet46-capability-report.md` — Sonnet 4.6 三层对照 48 项(同上)
> - `scripts/out/claude-haiku-4.5-compare.md` / `claude-opus-4.6-compare.md` / `claude-opus-4.7-compare.md` — 其余模型两层对比(2026-06-27)
> - `scripts/opus48-features.md` — Anthropic 官方文档参照系(2026-06-29)
> - `CHANGELOG.md` [Unreleased] — 代理侧行为与修复
>
> 测试方法:**三层对照**——① Anthropic 原生官方文档(参照系)/ ② GitHub Copilot 上游直连(`api.enterprise.githubcopilot.com`,企业账号)/ ③ 经 Copilot2API 代理(`/v1/messages` 原生透传)。主力工具为 `scripts/capability_test.py`(47~48 项能力矩阵)与 `scripts/structured_outputs_platform_test.py`。

---

## 一、总体结论(TL;DR)

1. **Copilot 上游对 Claude 模型的 Anthropic Messages API 支持度非常高**:核心能力(文本/流式/工具调用/视觉/PDF/思考/缓存/1M 上下文/structured outputs/computer use 等)在 Opus 4.8 与 Sonnet 4.6 上全部可用,与 Anthropic 原生的差距集中在少数几项服务端工具(web_search / web_fetch)、外链图片和 300K 扩展输出。
2. **Copilot2API 代理与上游直连几乎完全等价**:47/48 项矩阵中仅 2 处「不一致」,且都是代理**有意为之**(剥离非标准 `cache_control.scope`、剥离非白名单客户端 beta 头),不是缺陷。computer use 曾是第 3 处差异,已于 2026-07-07 修复(放行 `computer-use-*` beta 头)。
3. **Copilot 上游由三朵云动态托管**(Anthropic 第一方 / AWS Bedrock / Google Vertex),同一模型的落点随时间漂移。这带来一个实际影响:**structured outputs 在 Vertex 落点因 GitHub 的 GCP 组织策略被 400 拒绝**,导致会漂到 Vertex 的模型间歇性失败——客户端只能重试,无法通过改写请求规避。
4. **代理的已知缺口在转换路径**:Anthropic→Responses / Chat Completions 转换会静默丢弃 `output_config.format` 与工具 `strict` 字段;上游的 OpenAI 兼容端点也会静默忽略 `response_format.json_schema`。只有走原生 `/v1/messages` 透传才能保证 structured outputs 生效。

---

## 二、Copilot 上游托管架构(2026-07-12 指纹实测)

通过消息 ID 前缀指纹识别后端(`msg_01…` = Anthropic 第一方,`msg_bdrk_…` = Bedrock,`msg_vrtx_…` = Vertex):

| 模型 | 观测到的落点 | 说明 |
|---|---|---|
| claude-opus-4.8 / claude-fable-5 / claude-sonnet-5 | Anthropic(采样期内稳定) | opus-4.8 带 `inference_geo: "global"` |
| claude-sonnet-4.6 | Bedrock 为主,**会漂移到 Vertex** | 07-13 复现连续 3 次落 Vertex 并命中组织策略 400 |
| claude-haiku-4.5 | Bedrock / Vertex / Anthropic 均观测到 | 路由动态 |
| claude-sonnet-4.5 | Vertex / Bedrock 均观测到 | 路由动态 |
| claude-opus-4.7 / 4.5 | Vertex(采样期内) | 因组织策略,structured outputs 持续失败 |

要点:
- 与 GitHub 官方 model-hosting 文档一致(AWS + Anthropic PBC + GCP 三方托管);**同一模型的落点不固定**,失败呈短时间窗内连续出现的特征(疑似路由粘性)。
- GitHub 网关剥除全部上游响应 headers,仅保留自有 headers(`x-quota-snapshot-*` 等);Bedrock 落点的响应保留 `msg_bdrk_` / `toolu_bdrk_` 原始 ID,并附加 Copilot 私有计价字段 `copilot_usage`。
- 上游 API 层**只接受 Claude API 约定**:平面模型 ID + `output_config.format` / strict tool use。Bedrock 风格模型命名(`anthropic.claude-…` 等 4 种变体)全部 400;`anthropic_version` 字段(Vertex 约定)**完全不校验**(垃圾值也通过,属被忽略而非被支持);真实 Vertex SDK 因端点路径差异无法直连。

---

## 三、模型能力矩阵(Copilot 上游实测)

### 3.1 旗舰双模型全量矩阵(Opus 4.8 = 47 项,Sonnet 4.6 = 48 项)

三层全绿(原生 ✅ / 直连 ✅ / 经代理 ✅)的能力——两个模型一致:

> text、streaming、function_calling、parallel_tools、vision_base64、pdf_document、extended_thinking、server_tool_bash / text_editor / memory、prompt_cache(含 1024-token 低下限与 1h `extended_cache_ttl`)、context_management、citations、**computer_use**、count_tokens、**context_1m(原生 1M 窗口)**、stop_sequences、metadata、service_tier、tool_choice 全部 5 种变体、**structured_outputs**、code_execution(Copilot 实际执行)、search_result、interleaved_thinking、token_efficient_tools、fine_grained_tool_streaming、effort `max`、model_discovery(`/v1/models` 34 个模型,capabilities + max_ctx=1,000,000)

两个模型的关键差异(原生约束「此消彼长」,Copilot 忠实体现):

| 能力 | Sonnet 4.6 | Opus 4.8 |
|---|---|---|
| 非默认 `temperature` / `top_p` / `top_k` | ✅ 原生支持,三层一致 | 原生 ⛔ 400,**Copilot 容忍(200)**——原生 vs Copilot 的行为偏差 |
| 手动思考预算 `thinking:{enabled,budget_tokens}` | ✅ 支持 | ⛔ 400(仅 adaptive) |
| effort 档位 | `low/medium/high/max`(拒 `xhigh`) | `low/medium/high/xhigh/max`(默认 high) |
| 会话内 `role:"system"` 消息(mid-conv system) | ⛔ 400 `Unexpected role` | ✅(须满足占位规则) |
| fast mode(`speed:"fast"` + beta) | 原生无;Copilot 容忍字段返回 200 | Copilot 容忍(200)但**不提供真实 2.5× 加速** |
| prompt cache 最小前缀 | 1024 token(常规) | 1024 token(4.8 由 2048 下调,实测生效) |
| 输出上限 | 128K(`max_tokens=200000` → 400) | 128K(同左;`output-300k` beta 仅 Anthropic Batch API,Copilot 拒) |
| 知识截止 | Aug 2025(可靠) | Jan 2026 |

**Copilot 上游不支持的 Anthropic 原生能力**(两模型一致,2026-07-07 用最新 GA 工具型号复测确认为上游硬限制):

| 能力 | 实测 | 上游报错 |
|---|---|---|
| `web_search`(服务端工具,任何版本/beta 组合) | ⛔ 400 | `The use of the web search tool is not supported.` |
| `web_fetch`(工具与 beta 头双白名单均堵) | ⛔ 400 | `rejected tool(s): web_fetch` / `unsupported beta header(s)` |
| 外链图片 `image.source.type=url` | ⛔ 400 | `external image URLs are not supported`(仅收 base64) |
| 300K 扩展输出(`output-300k` beta) | ⛔ 400 | 128K 硬上限 |
| fast mode 真实加速 | 200 但无效 | 字段/beta 被容忍,不实现加速 |

### 3.2 其余 Claude 模型(2026-06-27 两层对比 + 07-12 补测)

- **claude-haiku-4.5**:核心能力(文本/流式/工具/视觉/PDF/思考/interleaved thinking/structured outputs/1M 上下文/context_management/citations/search_result)全部可用。**不支持 reasoning effort**(`output_config.effort` 任何档位 400)。`code_execution_20250825` 工具型号被拒(该模型不在支持列表)。06-27 该轮 prompt cache 未命中(cc=0),后续未复测定论。
- **claude-opus-4.6**:能力面与 4.8 接近;effort 支持 `low/medium/high/max`(**无 `xhigh`**);支持手动思考预算(与 4.7/4.8 相反);`code_execution_20250825` 型号在 06-27 该轮被拒;128K 输出上限一致。
- **claude-opus-4.7**:effort 含 `xhigh`(与 4.8 同);采样参数硬约束、adaptive-only 思考(同 4.8);缓存最小前缀 2048(4.8 才降到 1024);06-27 该轮因落 Vertex,structured outputs 400(见 §四);`code_execution` 型号被拒。
- **claude-fable-5 / claude-sonnet-5**:07-12 实测落 Anthropic 第一方,structured outputs 两种机制均 OK;未跑全量矩阵。
- **claude-opus-4.8-fast**:structured outputs 的 **strict tool use 不可靠**(首轮缺 required 键,复测出现工具调用标记泄漏进字符串值),疑似 strict 约束未真正启用 grammar 强制;`output_config.format` 机制正常。

---

## 四、Structured Outputs 专项(2026-07-12 ~ 07-13,最新)

两种机制:A = JSON outputs(`output_config.format`,json_schema);B = strict tool use(`strict: true` + 强制 tool_choice)。

**可用性完全取决于路由落点**:

| 落点 | 机制 A | 机制 B | 备注 |
|---|---|---|---|
| Anthropic 第一方 | ✅ | ✅ | |
| AWS Bedrock | ✅ | ✅ | 10/10 双重指纹绑定验证(`msg_bdrk_` + `toolu_bdrk_`) |
| Google Vertex | ⛔ 400 | ⛔ 400 | GitHub 的 GCP 组织策略 `vertexai.allowedPartnerModelFeatures` 未放行 `structured_outputs`——**Copilot 侧配置缺口**,非 Vertex 平台能力问题 |

- 因路由动态漂移,会漂到 Vertex 的模型(sonnet-4.6 / sonnet-4.5 / haiku-4.5 / opus-4.7 / opus-4.5 等)**间歇性失败**;失败时返回 Google 原生格式错误(`FAILED_PRECONDITION`)。**生产建议:对该特定 400 做客户端重试**。
- 废弃的 `output_format` 字段返回 400 并指引改用 `output_config.format`——证明特性链路是真实实现而非透传巧合。
- token 开销:同一 schema 下机制 A 输入 227 tokens,机制 B 721 tokens(约 3 倍)——只要结构化输出时优先用 `output_config.format`。
- **OpenAI 兼容端点 `/chat/completions` 的 `response_format.json_schema` 被上游静默忽略**(HTTP 200 但输出不受约束)。

---

## 五、Copilot2API 代理支持情况

### 5.1 原生 `/v1/messages` 透传:与直连等价

47/48 项矩阵中,直连与经代理仅 **2 处有意分叉**:

1. **`cache_control.scope` 剥离**:`scope` 非 Anthropic 标准字段,直连被上游 400 拒;代理有意剥离后 200(既定行为)。
2. **客户端 `anthropic-beta` 头白名单**:代理不盲转客户端 beta 头——
   - **放行** `computer-use-*`(2026-07-07 新增,`anthropic/handler.go` 的 `extractComputerUseBetas`,覆盖 `/v1/messages` 与 `/count_tokens`);
   - **自动注入** `context-management`(当 body 含 `context_management` 字段时);
   - **其余全部剥离**。副作用是良性的:如带 `code-execution-2025-08-25` beta 直连被上游 allowlist 拒(400),经代理剥头后工具照常执行(200);仅靠 header 的特性(interleaved_thinking / token_efficient_tools / fine_grained_tool_streaming / extended_cache_ttl)以普通请求形式到达上游且全部成功。

**Computer use 端到端已打通**(2026-07-12 四用例复验):新版 `computer-use-2025-11-24` + `computer_20251124`(Opus 4.8 / Sonnet 4.6)与旧版 `computer-use-2025-01-24` + `computer_20250124`(Sonnet 4.5 等旧模型)均经代理正常返回 `tool_use`;负向对照(不带 beta 头 → 400)证明 header 转发正是关键路径;完整 agent 循环(请求截图 → 回传截图 tool_result → 下一动作)可用。

**其他已验证的代理行为**:
- `output_config.format` / strict tool 在 native 路由**逐字节透传**,端到端 schema 严格生效(07-12 经代理落 Bedrock 实测通过)。
- `POST /v1/messages/count_tokens` 已代理到上游计数器(此前 404)。
- `search_result` 内容块的裸字符串 `source` 已修复解析(此前代理在请求出门前就 400)。
- 1M 上下文的 `anthropic-beta: context-1m` 处理已修复:仅当基础模型不带 1M 窗口且 `-1m` 变体真实存在时才切换,新模型(sonnet-4.6、opus-4.6/4.7/4.8)基础 ID 即 1M。

### 5.2 已知缺口(转换路径)

| 缺口 | 位置 | 影响 |
|---|---|---|
| `AnthropicOutputConfig` 缺 `Format` 字段 | `anthropic/responses_convert.go:342` | Anthropic→Responses 转换时 `output_config.format` 被静默丢弃 |
| `AnthropicTool` 缺 `Strict` 字段 | `proxy/convert.go:246` | strict 被硬编码为 false,strict tool use 失效 |
| 上游 `/chat/completions` 忽略 `response_format.json_schema` | 上游行为 | OpenAI 客户端要用 structured outputs,需代理侧转换或显式报错(当前静默忽略) |
| Responses 路由(如 gpt-5.4) | 转换路径 | reasoning 模型拒非默认 `top_p`(400);`stop_sequences` 被丢弃(Responses API 无此参数) |
| Chat Completions 路由(如 gemini-3.5-flash) | 转换路径 | `top_k` / `service_tier` 被丢弃;`disable_parallel_tool_use` 已接线但模型可能忽略 |

### 5.3 代理自身功能面(与模型能力正交)

多账号(API key ↔ GitHub 账号 1:1,`accounts.json`)、Web 管理台 `/admin/`(Device Flow 认证、key 轮换/自动生成、token 用量统计含缓存命中率、上游模型列表页)、API key 多来源提取(`Authorization: Bearer` / `x-api-key` / `x-goog-api-key` / `?key=`,覆盖 OpenAI / Anthropic / Gemini 客户端)、Gemini `/v1beta` 兼容端点、`/chat/completions` ↔ `/responses` 智能回退。

---

## 六、使用建议

1. **Claude 模型一律走原生 `/v1/messages`**(Anthropic SDK 指到代理即可):透传路径能力最全,structured outputs、computer use、1M 上下文、prompt cache 全部端到端可用;不要经 OpenAI 兼容端点用 Claude 的 structured outputs。
2. **structured outputs 生产使用需容错**:对 Vertex 组织策略 400(报文含 `vertexai.allowedPartnerModelFeatures` / `FAILED_PRECONDITION`)做重试;优先选路由稳定落 Anthropic 的模型(opus-4.8 / fable-5 / sonnet-5);只需结构化输出时用 `output_config.format`(比 strict tool 省约 3 倍输入 token);避免依赖 opus-4.8-fast 的 strict tool。
3. **按模型选参数**:Opus 4.7/4.8 用 `output_config.effort`(可到 `xhigh`/`max`)且不要设手动 thinking 预算与非默认采样参数(即使 Copilot 容忍,效果无保证);Sonnet 4.6 / Haiku 4.5 / Opus ≤4.6 可用手动 `budget_tokens` 与采样参数;Haiku 4.5 不支持 effort。
4. **不要指望的能力**:web_search、web_fetch、外链图片 URL、>128K 输出、fast mode 真实加速——这些是 Copilot 上游硬限制,换任何 beta 头/工具版本都打不通;图片一律转 base64 内嵌。
5. **computer use 记得带对 beta 头**:新模型(Opus 4.6+/Sonnet 4.6)用 `computer-use-2025-11-24` + `computer_20251124`,旧模型用 `computer-use-2025-01-24` + `computer_20250124`;代理会放行转发。
