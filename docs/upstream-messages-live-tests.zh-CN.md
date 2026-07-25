# GitHub Copilot 上游 `/v1/messages` 实测记录（2026-07-21 / 22，07-25 复测）

本文记录对 GitHub Copilot 上游 Anthropic 兼容端点（`https://api.githubcopilot.com/v1/messages`）
的一组实测：四项 Anthropic 原生能力（web_search、web_fetch、图片 URL、structured output）、
`max_tokens` 真实上限（含 300K 探测）、以及 SSE 流生存期。所有结论均通过
**经 copilot2api 代理**与**绕过代理直连上游**两条链路交叉验证，结果逐项一致。

相关文档：[copilot-capability-report.html](copilot-capability-report.html)（2026-06/07 中旬的
整体能力矩阵）、[copilot2api-issues-retrospective.html](copilot2api-issues-retrospective.html)
（问题复盘，含 SSE 稳定性 Q6）。本文是其后续补充，聚焦最新请求格式复测与流生存期定量结论。

## 测试方法

- **认证**：direct 模式（GitHub OAuth token 直接作为 Copilot bearer），`opencode` 请求头档案：
  `User-Agent: opencode/0.4.2`、`Openai-Intent: conversation-edits`、
  `X-Github-Api-Version: 2026-06-01`、`X-Initiator: user`，另带 `anthropic-version: 2023-06-01`。
- **链路**：① curl → copilot2api（当前代码构建，127.0.0.1:7877）→ 上游；② curl → 上游直连。
- **请求格式**：测试前从 Anthropic 官方文档（platform.claude.com）确认为当时最新：
  - web_search：工具类型 `web_search_20250305`、`web_search_20260209` 与 `web_search_20260318`
    （最新），无需 beta 头；`20260209` 起新增 `allowed_callers` 字段，直调需设 `["direct"]`；
  - web_fetch：`web_fetch_20250910`、`20260209`、`20260309` 与 `web_fetch_20260318`（最新），
    已 GA 无需 beta 头（旧头 `web-fetch-2025-09-10` 亦对照测试）；
  - 图片 URL：`{"type":"image","source":{"type":"url","url":...}}`，无需 beta 头；
  - structured output：GA 新参数 `output_config.format`（`{"type":"json_schema","schema":...}`），
    无需 beta 头；旧参数 `output_format` + `structured-outputs-2025-11-13` 头作对照。

## 一、四项能力结果

| 能力 | 结果 | 上游返回（原文） |
|---|---|---|
| web_search | ❌ 所有模型 | 400 `The use of the web search tool is not supported.`（`unsupported_value`） |
| web_fetch | ❌ 所有模型 | 400 `rejected tool(s): web_fetch`（`invalid_request_body`） |
| 图片 URL | ❌ 所有模型 | 400 `external image URLs are not supported`（base64 图片正常 ✅） |
| structured output | ✅ 40/50 请求 | 取决于后端路由，见下节 |

web_search / web_fetch / 图片 URL 三项的拒绝与模型无关、与工具版本无关、与 beta 头无关，
是 Copilot 网关层的统一拦截。

### 2026-07-25 复测（direct 模式，最新工具版本）

按 platform.claude.com 当日文档取最新版本重测，三项仍全部被拒，**报错文本逐字未变**：

| 能力 | 测试的版本 | 结果 |
|---|---|---|
| web_search | `web_search_20250305` / `20260209` / **`20260318`**（含 `allowed_callers: ["direct"]` 与默认两种写法） | ❌ 全部 400 |
| web_fetch | `web_fetch_20250910` / `20260209` / `20260309` / **`20260318`** | ❌ 全部 400 |
| 图片 URL | `source.type = "url"` | ❌ 全部 400 |

覆盖 8 个 Claude 模型（opus-4.8 / sonnet-5 / fable-5 / sonnet-4.6 / opus-4.5 / haiku-4.5 /
opus-4.7 / opus-4.6）全部 400。

**对照组证明是定向拦截而非通用故障**：

| 对照 | 结果 |
|---|---|
| base64 图片 | ✅ 200 正常返回（4 模型） |
| 普通客户端工具（`tool_use`） | ✅ 200 正常返回 `tool_use` 块（3 模型） |

**拦截发生在 GitHub 网关层，不是 Anthropic**：响应带 `X-GitHub-Request-Id`，且错误体为
GitHub 格式 `{"error":{"message":…,"code":…}}`，而非 Anthropic 的
`{"type":"error","error":{…}}` —— 说明请求在到达模型前即被拒。

## 二、structured output 路由矩阵

Copilot 将 Claude 模型路由到三种后端（可从消息 ID 前缀识别）：`msg_`（Anthropic 一方）、
`msg_bdrk_`（AWS Bedrock）、`msg_vrtx_`（GCP Vertex）。

| 后端（ID 前缀） | `output_config.format` / `strict: true` |
|---|---|
| Anthropic 一方（`msg_`） | ✅ |
| AWS Bedrock（`msg_bdrk_`） | ✅ |
| GCP Vertex（`msg_vrtx_`） | ❌ |

Vertex 路由的失败原因：GitHub 自己 GCP 项目（`projects/524636045653`）的组织策略
`constraints/vertexai.allowedPartnerModelFeatures` 未放行 `structured_outputs` 特性
（400 `FAILED_PRECONDITION`，报错建议联系组织管理员添加
`publishers/anthropic/models/claude-sonnet-4-5:structured_outputs`）。

### 路由随时间漂移（2026-07-25 direct 模式复测修正）

**模型与后端不是固定映射**：同一模型在不同时刻会落到不同后端，因此
**structured output 的失败名单会随时间变化**。同一批请求在 26 分钟内观察到三种结果：

| 时刻 | 失败模型（落到 Vertex） |
|---|---|
| 18:44 | opus-4.6、sonnet-4.6、sonnet-4.5 |
| 18:47 | opus-4.7、opus-4.6 |
| 18:50 | opus-4.6、opus-4.5 |

已用 A/B 排除法确认变量：BIG/SMALL schema × LONG/SHORT prompt 四种组合各 4 次，
结果**完全一致**——schema 与 prompt 都不影响路由，**唯一变量是时间**（网关侧动态负载均衡）。

因此正确的表述是**「凡落到 Vertex 必失败，落到 Anthropic / Bedrock 必成功」**，
而非「某几个模型不支持」。**无法靠避开特定模型规避**，应对该特定 400 做自动重试
（重试大概率改落 Anthropic / Bedrock 而成功）。

最终全量采样（10 模型 × 5 次 = 50 请求）：**40/50 成功**，失败全部且仅来自 Vertex 落点。

旧参数 `output_format` 会被上游明确拒绝：
`output_format: This field is deprecated. Use 'output_config.format' instead.`
——接入时应直接使用新 GA 格式（即便附带 `structured-outputs-2025-11-13` beta 头也同样被拒）。

## 三、max_tokens 真实上限（300K 探测）

用 `max_tokens=300000` 逐模型探测，上游报错自报真实上限：

| 模型 | Copilot `/models` 宣告 | 真实执行上限 |
|---|---|---|
| fable-5、sonnet-5、opus-4.8、opus-4.7、opus-4.6、sonnet-4.6 | 64000 | **128000** |
| haiku-4.5 | 64000 | 64000 |
| sonnet-4.5、opus-4.5 | 32000 | **64000** |

- 300K 在所有模型上都被验证阶段拒绝（400
  `max_tokens: 300000 > 128000, which is the maximum allowed number of output tokens for …`）。
- **`/models` 宣告值普遍低于真实执行值**（宣告 64K 实收 128K；`max_tokens=64001` 正常通过）。
- 128K 请求**无需** `output-128k` beta 头。

## 四、SSE 流生存期：约 630 秒硬上限

三次长输出流式实验（`max_tokens=128000`，任务为大批量数据变换 / 长文翻译）：

| # | 链路 | 模型 | 流时长 | 切断前交付 | 终止方式 |
|---|---|---|---|---|---|
| 1 | 经代理 | sonnet-5 | 630.3 s | 36,928 行（约 21 万字符，估 10 万+ token） | 裸 EOF |
| 2 | 经代理 | opus-4.8 | 631.8 s | 23,529 汉字（估约 3 万 token） | 裸 EOF |
| 3 | **直连上游** | sonnet-5 | 632 s | 37,077 行（约 21.1 万字符） | **HTTP/2 RST_STREAM**（curl 退出码 92） |

- 三次均无 `message_delta`（无 `stop_reason`）、无 `message_stop`、无 `[DONE]`，
  最后一个事件是普通 `content_block_delta`（直连那次最后一行数字被拦腰截断）。
- 代理侧日志表现为 `error reading native /messages stream: unexpected EOF`；直连揭示底层是
  上游在 HTTP/2 层直接重置流。copilot2api 流式路径无超时（`main.go` 特意不设 ReadTimeout），
  已排除代理因素。
- 对照组：haiku-4.5 在 185 秒内完整交付 62,005 output token 并正常收尾
  （`end_turn` + `message_stop` + `[DONE]`），证明限制是**时间维度**而非响应体积维度。
- 与 [issues retrospective](copilot2api-issues-retrospective.html) Q6 记录的 SSE 稳定性问题
  （上游 claude-code #70017）相互印证。

实测吞吐参考（决定 630 秒窗口内的实际可交付量）：

| 模型 | 实测输出速率 | 630 s 窗口实际可交付 |
|---|---|---|
| haiku-4.5 | ~335 tok/s | 可完整跑满 64K 上限 |
| sonnet-5 | ~170 tok/s | 约 10 万 token（跑不满 128K） |
| opus-4.8 | ~50 tok/s | 约 3 万 token |

## 五、直连 vs 经代理对照

绕过 copilot2api 直连上游复测全部项目：冒烟、300K 拒绝（错误文本逐字相同）、128K 接受、
web_search / web_fetch / 图片 URL 拒绝、structured output 成功、630 s 流切断——**逐项一致**。
所有能力边界均为 GitHub Copilot 上游行为；copilot2api 在该链路上仅做认证注入与透传，
未引入额外限制。

## 六、附带发现

- `claude-sonnet-5` **不支持 assistant prefill**：上游报
  `This model does not support assistant message prefill. The conversation must end with a user message.`
- structured output 的 JSON Schema 限制：数组 `minItems` 仅允许 0 或 1
  （`For 'array' type, 'minItems' values other than 0 or 1 are not supported`）。
- 模型行为层面对"大批量机械输出"有抗性，且 opus-4.8 远比 sonnet-5 顽固：
  裸指令数万行、缩减到 1.2 万行、坦承为 max_tokens 基准测试三种方案均被 opus 拒绝；
  只有内容本身有实质价值的长任务（如整本书翻译）能让其持续输出到被强制切断。
  sonnet-5 对"给定数据逐行变换"任务即可稳定配合。

## 七、实操建议

- 需要联网检索 / 抓取网页 / 传 URL 图片：在代理或客户端侧自行实现，不要指望上游；
  图片一律转 base64（或走 Files 方案）再发。
- structured output：用新 GA 参数 `output_config.format`（旧 `output_format` 已被硬拒）。
  **不要用"避开某几个模型"的方式规避 Vertex** —— 路由随时间漂移，失败名单会变；
  正确做法是对 `vertexai.allowedPartnerModelFeatures` 这一特定 400 做自动重试，
  重试大概率改落 Anthropic / Bedrock 而成功。
- 超长输出：单次请求的实际上限 = min(`max_tokens` 上限, 输出速率 × 630 s)。
  300K 级输出需多轮续写拼接（haiku 1 轮 64K、sonnet-5 约 3 轮、opus-4.8 约 10+ 轮）。
- 客户端必须把"流突然 EOF / HTTP/2 stream error 且无 `message_stop`"当作**可恢复的截断**
  处理（保留已收内容、续写重试），不能当作完整响应。

## 数据来源

2026-07-21、07-22 与 07-25 live 实测；账号为 direct 模式（GitHub OAuth `gho_` token 直接作
Copilot bearer，base URL `https://api.githubcopilot.com`）。官方请求格式依据
[platform.claude.com](https://platform.claude.com/docs/) 当日文档
（web search / web fetch / vision / structured outputs 四篇）。07-25 复测取当日最新工具版本
（`web_search_20260318`、`web_fetch_20260318`）并补充对照组与多轮采样。
