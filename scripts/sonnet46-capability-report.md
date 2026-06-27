# Claude Sonnet 4.6 原生能力三层对照(Anthropic 原生 / Copilot 直连 / 经 Copilot2API)

> 对照对象:
> - **① Anthropic 原生官方** — `platform.claude.com`(models overview)+ `anthropic.com/news/claude-sonnet-4-6`(取于 2026-06-27)
> - **② GitHub Copilot 直连** — 上游 `api.enterprise.githubcopilot.com`(企业版标准端点)
> - **③ 经 Copilot2API 代理** — 运行中的 copilot2api,`/v1/messages` 原生透传
>
> - 模型:`claude-sonnet-4-6`(矩阵以别名 `claude-sonnet-4.6` 调用)
> - 实测:`scripts/capability_test.py --target both --model claude-sonnet-4.6`,全量 **48 项**矩阵,生成于 2026-06-27(企业版账号,已脱敏)
> - 原始数据(gitignore):`scripts/out/sonnet46-authoritative-raw.json` / `sonnet46-authoritative.md`
> - 姊妹报告:`scripts/opus48-capability-report.md`(Opus 4.8 三层对照)
> - 说明:`reject` 类用例以上游 4xx 视为「该能力在该层不存在=符合预期」。token / 账号邮箱从不打印或落盘。

---

## 一、Anthropic 原生 Sonnet 4.6 特性清单(参照系)

| 维度 | 值 | 说明 |
|---|---|---|
| API model ID | `claude-sonnet-4-6` | 发布于 2026-02-17;Anthropic 当前最新 Sonnet |
| 上下文窗口 | **1M tokens(beta)** / 200K 标准 | Claude API / Bedrock / Vertex;Foundry 200K |
| 最大输出 | **128K tokens** | 实测 Copilot 亦在 128k 处硬限(`max_tokens=200000` → 400 `> 128000`) |
| 扩展思考(手动预算) | **支持** | `thinking:{type:"enabled",budget_tokens:N}` 正常产出 thinking 块(**与 Opus 4.8 相反**) |
| Adaptive thinking | **支持** | `thinking:{type:"adaptive"}` |
| effort 档位 | `low / medium / high` **+ `max`**(顶档) | **不支持 `xhigh`**(`xhigh` 为 Opus 4.7/4.8 在 high 与 max 间的插入档) |
| 采样参数 | `temperature` / `top_p` / `top_k` **可设非默认值** | **无 Opus 4.7/4.8 式 400 限制** |
| computer use | **原生增强**(OSWorld 大幅提升) | Anthropic 服务端工具;Copilot 不支持该工具类型 |
| 可靠知识截止 | **Aug 2025**(训练数据 Jan 2026) | Opus 4.8 为 Jan 2026 可靠 |
| 定价 | $3 / $15 每 MTok | 与 Sonnet 4.5 同档 |
| 输入模态 | 文本 + 图像(含 URL 图源) | — |

### Sonnet 4.6 vs Opus 4.8 原生差异速查(本报告关键)

| 能力 | Sonnet 4.6 原生 | Opus 4.8 原生 |
|---|---|---|
| 非默认 `temperature`/`top_p`/`top_k` | **✅ 支持** | ⛔ 400 |
| 手动思考预算 `thinking.enabled+budget_tokens` | **✅ 支持** | ⛔ 400(仅 adaptive) |
| effort `xhigh` | ⛔(无此档) | ✅ |
| effort `max` | ✅ | ✅ |
| `mid_conv_system`(数组内 `role:"system"`) | ⛔(Unexpected role) | ✅(Opus 4.8 新增) |
| `fast_mode` | ⛔(Opus 4.8 研究预览) | ✅ |
| 输出上限 | 128K | 128K |
| prompt cache 下限 | ~1024(常规) | 1024(4.8 由 2048 下调) |
| 知识截止 | Aug 2025 | Jan 2026 |

> 概括:**Sonnet 4.6 是「采样可调 + 手动思考预算」的通用 workhorse;Opus 4.8 是「采样锁定 + 仅 adaptive + xhigh/mid-conv/fast-mode」的高自治推理模型**。二者顶档 effort 都叫 `max`,但 `xhigh` 与手动 budget 恰好互斥地分属两侧。

---

## 二、三层对照矩阵(48 项)

> 图例:✅ 支持 / 已按预期工作 · ⛔ 4xx(该层不存在或被拒) · ⚠️ 200*(200 但断言未满足,多为模型非确定性) · ↔️ 预期差异 · † 见脚注 · 🆕 本轮新增/Sonnet 专项核对的用例
> 「直连=代理?」列只比较 ②③ 两层;「原生官方」列为 Anthropic 文档参照(Sonnet 4.6)。

| # | 能力 | 原生官方 | 直连(实测) | 经代理(实测) | 直连=代理? | 备注 |
|---|---|---|---|---|---|---|
| 1 | `text` | ✅ | ✅ | ✅ | ✅ |  |
| 2 | `streaming` | ✅ | ✅ | ✅ | ✅ |  |
| 3 | `function_calling` | ✅ | ✅ | ✅ | ✅ |  |
| 4 | `parallel_tools` | ✅ | ✅ | ✅ | ✅ |  |
| 5 | `vision_base64` | ✅ | ✅ | ✅ | ✅ |  |
| 6 | `vision_url` | ✅ | ⛔ 4xx | ⛔ 4xx | ✅ | 原生支持 URL 图源,Copilot 拒外链图 |
| 7 | `pdf_document` | ✅ | ✅ | ✅ | ✅ |  |
| 8 | `extended_thinking` | ✅ | ✅ | ✅ | ✅ | Sonnet 支持手动 budget 与 adaptive(Opus 4.8 仅 adaptive) |
| 9 | `server_tool_bash` | ✅ | ✅ | ✅ | ✅ |  |
| 10 | `server_tool_text_editor` | ✅ | ✅ | ✅ | ✅ |  |
| 11 | `server_tool_memory` | ✅ | ✅ | ✅ | ✅ |  |
| 12 | `prompt_cache` | ✅ | ✅ | ✅ | ✅ |  |
| 13 | `cache_control_scope` | ⛔ | ⛔ 4xx | ✅ | ↔️ 代理剥离(预期) | scope 非标准字段,原生亦拒;代理剥离后 200 |
| 14 | `context_management` | ✅ | ✅ | ✅ | ✅ | beta,代理自动注入 |
| 15 | `citations` | ✅ | ✅ | ✅ | ✅ |  |
| 16 | `web_search` | ✅ | ⛔ 4xx | ⛔ 4xx | ✅ | 原生服务端工具,Copilot 不支持 |
| 17 | `computer_use` | ✅ | ⛔ 4xx | ⛔ 4xx | ✅ | Sonnet 4.6 原生增强 computer use;Copilot 不支持 |
| 18 | `count_tokens` | ✅ | ✅ | ✅ | ✅ |  |
| 19 | `context_1m` | ✅ | ✅ | ✅ | ✅ | 原生 1M |
| 20 | `temperature` | ✅ | ✅ | ✅ | ✅ | Sonnet 无 Opus 4.7/4.8 式采样限制(原生接受非默认值) |
| 21 | `top_p` | ✅ | ✅ | ✅ | ✅ | 同上 |
| 22 | `top_k` | ✅ | ✅ | ✅ | ✅ | 同上 |
| 23 | `stop_sequences` | ✅ | ✅ | ✅ | ✅ |  |
| 24 | `metadata` | ✅ | ✅ | ✅ | ✅ |  |
| 25 | `service_tier` | ✅ | ✅ | ✅ | ✅ |  |
| 26 | `tool_choice_auto` | ✅ | ✅ | ✅ | ✅ |  |
| 27 | `tool_choice_any` | ✅ | ✅ | ✅ | ✅ |  |
| 28 | `tool_choice_tool` | ✅ | ✅ | ✅ | ✅ |  |
| 29 | `tool_choice_none` | ✅ | ✅ | ✅ | ✅ |  |
| 30 | `tool_choice_no_parallel` | ✅ | ✅ | ✅ | ✅ |  |
| 31 | `structured_outputs` | ✅ | ✅ | ✅ | ✅ |  |
| 32 | `web_fetch` | ✅ | ⛔ 4xx | ⛔ 4xx | ✅ | 原生服务端工具,Copilot 不支持 |
| 33 | `code_execution` | ✅ | ✅ | ✅ | ✅ † | Copilot 实际执行(本轮直连 400 为上游瞬时,复测 3/3 两侧 200) |
| 34 | `code_execution_beta_header` | ✅ | ⛔ 4xx | ✅ | ↔️ 预期差异 | 原生需 beta;直连被 allowlist 拒,代理剥离后执行 |
| 35 | `search_result` | ✅ | ✅ | ✅ | ✅ |  |
| 36 | `interleaved_thinking` | ✅ | ✅ | ✅ | ✅ | 本轮两侧均产出 thinking 块 |
| 37 | `token_efficient_tools` | ✅ | ✅ | ✅ | ✅ |  |
| 38 | `fine_grained_tool_streaming` | ✅ | ✅ | ✅ | ✅ |  |
| 39 | `extended_cache_ttl` | ✅ | ✅ | ✅ | ✅ | 1h 缓存 |
| 40 | `effort_xhigh` | ⛔ | ⛔ 4xx | ⛔ 4xx | ✅ | xhigh 为 Opus 4.7/4.8 专属;Sonnet 原生与 Copilot 均拒 |
| 41 | `output_300k` | ⛔ | ⛔ 4xx | ⛔ 4xx | ✅ | Sonnet 输出上限 128k;Copilot 亦 128k |
| 42 | `effort_max`  🆕 | ✅ | ✅ | ✅ | ✅ | max 为通用顶档;Sonnet 支持(xhigh 不支持) |
| 43 | `thinking_budget`  🆕 | ✅ | ✅ | ✅ | ✅ | Sonnet 支持手动思考预算;Opus 4.7/4.8 拒(adaptive-only) |
| 44 | `mid_conv_system`  🆕 | ⛔ | ⛔ 4xx | ⛔ 4xx | ✅ | mid-conv system 为 Opus 4.8 特性;Sonnet 原生不支持(Unexpected role 'system') |
| 45 | `fast_mode`  🆕 | ⛔ | ✅ | ✅ | ✅ | fast mode 为 Opus 4.8 研究预览;Sonnet 原生不支持。Copilot 仍容忍字段返回 200 |
| 46 | `prompt_cache_1024`  🆕 | ✅ | ✅ | ✅ | ✅ | Sonnet 常规 1024 缓存下限;命中(1201 token) |
| 47 | `refusal_stop_details`  🆕 | ✅ | ✅ | ✅ | ✅ | 4.8 文档化;本轮未触发硬拒绝 |
| 48 | `model_discovery` | ✅ | ✅ | ✅ | ✅ | `/v1/models` 34 个模型,max_ctx=1,000,000 |

† **`code_execution`(#33)**:本轮 authoritative 运行中**直连**偶发返回 `400`(上游 schema 未将 `code_execution_20250825` 纳入工具联合类型),**经代理** `200`。复测 3/3 两侧均 `200`(执行成功),确认该 400 为**上游瞬时错误**,非代理行为差异。

**一句话结论**:48 项里,**②直连 与 ③经代理 完全一致**,唯二的「不一致」是 `cache_control_scope` 与 `code_execution_beta_header` —— 二者都是 Copilot2API **有意为之**(剥离非标准 `scope` / 剥离客户端 beta),非缺陷;`code_execution` 的一次性 400 为上游瞬时。真正差异在 **①原生 与 ②③Copilot 之间**,且 **Sonnet 与 Opus 的原生约束此消彼长**,见 §三。

---

## 三、关键差异解读

### 3.1 Sonnet vs Opus:采样参数与手动思考预算(此消彼长)
- **采样参数**:Sonnet 4.6 **原生支持**非默认 `temperature`/`top_p`/`top_k`(无 Opus 4.7/4.8 的 400 限制),Copilot 直连/代理也都 200 → 此处**三层一致**。而 Opus 4.8 原生**拒绝**这些参数,仅靠 Copilot 的宽松才 200(见 Opus 报告 §3.1)。
- **手动思考预算**:Sonnet 4.6 **支持** `thinking:{type:"enabled",budget_tokens:N}`(`thinking_budget` 用例两侧 200 且产出 thinking 块);Opus 4.7/4.8 已移除手动预算(仅 adaptive),同样请求返回 400。
- 因此 harness 用同一机型门控把这对能力**互斥**地表达:`effort_xhigh`/`effort_max`(Opus 4.7/4.8 接受 xhigh)与 `thinking_budget`(Opus 4.7/4.8 拒手动预算)互为反面。

### 3.2 effort 档位:`max` 通用、`xhigh` 仅 Opus
Sonnet 4.6 接受 `output_config.effort:"max"`(200),但拒绝 `"xhigh"`(400「not supported by model」)。即 effort 全量档位在 Sonnet 上是 `low/medium/high/max`,`xhigh` 是 Opus 4.7/4.8 在 high 与 max 之间的专属插入档。直连=代理。

### 3.3 mid-conversation system message:Opus 4.8 专属
数组内 `role:"system"` 是 Opus 4.8 新特性。Sonnet 4.6 **原生不支持**,直连与经代理均返回相同的上游 400:`Unexpected role "system". The Messages API accepts a top-level `system` parameter`。两侧一致 → 非代理问题。

### 3.4 fast mode:Sonnet 原生无,但 Copilot 容忍字段
`speed:"fast"` + `fast-mode-2026-02-01` beta 是 Opus 4.8 的研究预览;Sonnet 4.6 原生不在支持名单。但 Copilot 直连/代理对该字段+beta **仍返回 200**(容忍/忽略,不报错),两侧一致。即「原生无此能力,Copilot 不拒绝」——与 Opus 上的表现一致。

### 3.5 原生 vs Copilot:服务端工具与外链图(含 Sonnet 的强项 computer use)
`web_search` / `web_fetch` / `computer_use` / `vision_url` 是 Anthropic 原生能力,**Copilot 上游一律 4xx**,直连与经代理一致。尤其 **`computer_use`**:Sonnet 4.6 的一大卖点是原生增强的 computer use,但 Copilot 不支持该工具类型(`'claude-sonnet-4-6' does not support tool types: computer_20250124`)——这是 Copilot 相对原生 Sonnet 最显著的能力缺口。

### 3.6 输出上限与缓存下限
- **输出**:Sonnet 4.6 与 Copilot 都把上限定在 **128k**(`max_tokens=200000` → 400 `> 128000`);`output-300k` 不适用本路径。
- **缓存**:Sonnet 4.6 常规最小可缓存前缀 **1024 token**(非 Opus 4.8 式「由 2048 下调」)。`prompt_cache`(1201 token)与 `prompt_cache_1024`(同 1201 token,经分词器校准后落在 1024–2048)均命中。注:Opus 4.7+ 改用新分词器,同文本 token 数比 Sonnet 旧分词器约多 30%,故该探针按目标分词器定长。

### 3.7 直连 vs 代理:仅两处「有意」分叉
- **`cache_control_scope`**:`scope` 非标准字段,直连被上游 400 拒,经代理 200(代理有意剥离)。
- **`code_execution_beta_header`**:带 `code-execution-2025-08-25` beta 头时,直连被上游 allowlist 拒(400),经代理 200(代理不转发客户端 beta,头被剥离后工具照常运行)。
- 其余 46 项(含 `code_execution` 本身,见脚注 †)直连与代理一致。

### 3.8 非确定性说明
- **`code_execution`**:见脚注 †,直连一次性 400 为上游瞬时,复测两侧 200。
- **adaptive/extended thinking**:本轮 Sonnet 的 `extended_thinking` 与 `interleaved_thinking` 两侧均产出 thinking 块;思考块是否出现仍受模型自决影响。

---

## 四、本轮针对 Sonnet 的矩阵改动

| 改动 | 说明 |
|---|---|
| 新增 `thinking_budget` | 手动扩展思考预算;机型条件:Sonnet/Haiku/Opus≤4.6 **support**,Opus 4.7/4.8 **reject**(为 `effort_xhigh` 的反面) |
| 修正 `effort_max` 门控 | 由「仅 Opus 4.7/4.8」改为「Claude 4.x 通用」——实测 Sonnet 4.6 接受 `max`、拒 `xhigh` |
| 修正 `mid_conv_system` 门控 | 由固定 support 改为机型条件:仅 Opus 4.8 support,Sonnet 等 reject(`Unexpected role "system"`) |
| `prompt_cache_1024` 分词器健壮化 | 前缀由 *170 调整为 *200,使其在 Opus 4.7+ 新分词器(多 ~30% token)与旧分词器下都落在 1024–2048 区间并命中 |

> 以上均为**机型条件**改动,不影响 Opus 4.8 既有结论(已抽查保持一致)。

## 五、复现

```bash
# 全量 48 项,直连 + 经代理,Sonnet 4.6
python3 scripts/capability_test.py --target both --model claude-sonnet-4.6 \
  --account <your-account-dir> --proxy-url http://127.0.0.1:<port> --api-key sk-xxx \
  --report scripts/out/sonnet46-authoritative.md --raw scripts/out/sonnet46-authoritative-raw.json

# 仅 Sonnet 专项用例
python3 scripts/capability_test.py --target both --model claude-sonnet-4.6 \
  --only thinking_budget,effort_max,effort_xhigh,mid_conv_system,prompt_cache_1024
```

`scripts/out/` 为 gitignore,原始 JSON 与逐行报告不入库;本对照报告为脱敏的可提交版本。
