# Claude Opus 4.8 原生能力三层对照(Anthropic 原生 / Copilot 直连 / 经 Copilot2API)

> 对照对象:
> - **① Anthropic 原生官方** — `platform.claude.com` 文档(Opus 4.8 What's new / models / effort,取于 2026-06-27)
> - **② GitHub Copilot 直连** — 上游 `api.enterprise.githubcopilot.com`(企业版标准端点;`github_token` 交换 copilot token 后直打 `/v1/messages`)
> - **③ 经 Copilot2API 代理** — 运行中的 copilot2api,`/v1/messages` 原生透传
>
> - 模型:`claude-opus-4-8`(矩阵以别名 `claude-opus-4.8` 调用,代理 `resolveModelAlias` 归一化为 `claude-opus-4-8`)
> - 实测:`scripts/capability_test.py --target both --model claude-opus-4.8`,全量 **47 项**矩阵,生成于 2026-06-27(企业版账号,已脱敏)
> - 原始数据(gitignore):`scripts/out/opus48-authoritative-raw.json` / `opus48-authoritative.md`
> - 说明:`reject` 类用例以上游返回 4xx 视为「该能力在 Copilot 不存在 = 符合预期」。token / 账号邮箱从不打印或落盘。

---

## 一、Anthropic 原生 Opus 4.8 特性清单(参照系)

| 维度 | 值 | 说明 |
|---|---|---|
| API model ID | `claude-opus-4-8` | 无日期后缀的固定快照 |
| 上下文窗口 | **1M tokens(原生默认)** | Claude API / Bedrock / Vertex 为 1M;Microsoft Foundry 为 200K |
| 最大输出 | **128K tokens** | 同步 Messages API 上限;300K 仅 Batch API 经 `output-300k` beta |
| 思考模式 | **仅 adaptive** | `thinking:{type:"adaptive"}`,模型自决是否/思考多少 |
| 手动思考预算 | **不支持** | `thinking:{type:"enabled",budget_tokens:N}` 返回 **400** |
| effort 档位 | `low / medium / high / xhigh / max`,默认 **high** | GA,无 beta;`xhigh` `max` 为 Opus 4.7/4.8 高端档 |
| 采样参数 | `temperature` / `top_p` / `top_k` 非默认值 **400** | 4.8(同 4.7)硬约束;用提示词替代 |
| 知识截止 | **Jan 2026** | 4.6 为 May 2025 |
| 定价 | $5 / $25 每 MTok(输入/输出) | 与 4.6/4.7 同档 |
| 输入模态 | 文本 + 图像(含 URL 图源) | — |

**Opus 4.8 相对 4.7 的「新特性」(本轮重点补测)**

1. **Mid-conversation system messages** — `messages` 数组内可放 `role:"system"`(须「位于某个 assistant 消息之前或作为数组末尾」的占位规则),用于长会话中追加指令而无需重述整个 system。无 beta。
2. **Refusal `stop_details`** — 硬拒绝响应在 `stop_reason:"refusal"` 之外附带 `stop_details` 对象,描述拒绝类别。无 beta。
3. **Fast mode** — `speed:"fast"` + `fast-mode-2026-02-01` beta,溢价换 ~2.5× 输出速率(Claude API 研究预览)。
4. **Prompt cache 下限降低** — 最小可缓存前缀 **1024 token**(4.7 为 2048)。
5. **effort `max` 顶档** — 5 档 effort 的最高档(4.7/4.8)。

---

## 二、三层对照矩阵(47 项)

> 图例:✅ 支持 / 已按预期工作 · ⛔ 4xx(该能力在该层不存在或被拒) · ⚠️ 200*(返回 200 但断言未满足,多为模型非确定性) · ↔️ 预期差异 · 🆕 本轮新增用例
> 「直连=代理?」列只比较 ②③ 两层。「原生官方」列为 Anthropic 文档参照。

| # | 能力 | 原生官方 | 直连(实测) | 经代理(实测) | 直连=代理? | 备注 |
|---|---|---|---|---|---|---|
| 1 | `text` | ✅ | ✅ | ✅ | ✅ |  |
| 2 | `streaming` | ✅ | ✅ | ✅ | ✅ |  |
| 3 | `function_calling` | ✅ | ✅ | ✅ | ✅ |  |
| 4 | `parallel_tools` | ✅ | ✅ | ✅ | ✅ |  |
| 5 | `vision_base64` | ✅ | ✅ | ✅ | ✅ |  |
| 6 | `vision_url` | ✅ | ⛔ 4xx | ⛔ 4xx | ✅ | 原生支持 URL 图源,Copilot 拒外链图 |
| 7 | `pdf_document` | ✅ | ✅ | ✅ | ✅ |  |
| 8 | `extended_thinking` | ✅ | ✅ | ✅ | ✅ | 仅 adaptive(4.8 拒 budget_tokens) |
| 9 | `server_tool_bash` | ✅ | ✅ | ✅ | ✅ |  |
| 10 | `server_tool_text_editor` | ✅ | ✅ | ✅ | ✅ |  |
| 11 | `server_tool_memory` | ✅ | ✅ | ✅ | ✅ |  |
| 12 | `prompt_cache` | ✅ | ✅ | ✅ | ✅ |  |
| 13 | `cache_control_scope` | ⛔ | ⛔ 4xx | ✅ | ↔️ 代理剥离(预期) | scope 非标准字段,原生亦拒;代理剥离后 200 |
| 14 | `context_management` | ✅ | ✅ | ✅ | ✅ | beta,代理自动注入 |
| 15 | `citations` | ✅ | ✅ | ✅ | ✅ |  |
| 16 | `web_search` | ✅ | ⛔ 4xx | ⛔ 4xx | ✅ | 原生服务端工具,Copilot 不支持 |
| 17 | `computer_use` | ✅ | ⛔ 4xx | ⛔ 4xx | ✅ | 原生服务端工具,Copilot 不支持 |
| 18 | `count_tokens` | ✅ | ✅ | ✅ | ✅ |  |
| 19 | `context_1m` | ✅ | ✅ | ✅ | ✅ | 原生 1M |
| 20 | `temperature` | ⛔ | ✅ | ✅ | ✅ | 4.8 原生拒非默认采样,Copilot 容忍 |
| 21 | `top_p` | ⛔ | ✅ | ✅ | ✅ | 同上 |
| 22 | `top_k` | ⛔ | ✅ | ✅ | ✅ | 同上 |
| 23 | `stop_sequences` | ✅ | ⚠️ 200* | ✅ | ⚠️ 非确定 | 本轮直连输出未含 STOP(模型非确定),字段两侧均被接受 |
| 24 | `metadata` | ✅ | ✅ | ✅ | ✅ |  |
| 25 | `service_tier` | ✅ | ✅ | ✅ | ✅ |  |
| 26 | `tool_choice_auto` | ✅ | ✅ | ✅ | ✅ |  |
| 27 | `tool_choice_any` | ✅ | ✅ | ✅ | ✅ |  |
| 28 | `tool_choice_tool` | ✅ | ✅ | ✅ | ✅ |  |
| 29 | `tool_choice_none` | ✅ | ✅ | ✅ | ✅ |  |
| 30 | `tool_choice_no_parallel` | ✅ | ✅ | ✅ | ✅ |  |
| 31 | `structured_outputs` | ✅ | ✅ | ✅ | ✅ |  |
| 32 | `web_fetch` | ✅ | ⛔ 4xx | ⛔ 4xx | ✅ | 原生服务端工具,Copilot 不支持 |
| 33 | `code_execution` | ✅ | ✅ | ✅ | ✅ | Copilot 实际执行(server_tool_use + bash_code_execution_tool_result) |
| 34 | `code_execution_beta_header` | ✅ | ⛔ 4xx | ✅ | ↔️ 预期差异 | 原生需 beta;直连被 allowlist 拒,代理剥离后执行 |
| 35 | `search_result` | ✅ | ✅ | ✅ | ✅ |  |
| 36 | `interleaved_thinking` | ✅ | ⚠️ 200* | ⚠️ 200* | ✅ | beta;本轮两侧均未显式 thinking(adaptive 自决),彼此一致 |
| 37 | `token_efficient_tools` | ✅ | ✅ | ✅ | ✅ |  |
| 38 | `fine_grained_tool_streaming` | ✅ | ✅ | ✅ | ✅ |  |
| 39 | `extended_cache_ttl` | ✅ | ✅ | ✅ | ✅ | 1h 缓存(cr=2402) |
| 40 | `effort_xhigh` | ✅ | ✅ | ✅ | ✅ | 4.7/4.8 专属 |
| 41 | `output_300k` | ⛔ | ⛔ 4xx | ⛔ 4xx | ✅ | 同步 Messages API 上限 128k(300k 仅 Batch);Copilot 亦 128k |
| 42 | `effort_max`  🆕 | ✅ | ✅ | ✅ | ✅ | 4.7/4.8 顶档,Copilot 支持 |
| 43 | `mid_conv_system`  🆕 | ✅ | ✅ | ✅ | ✅ | 4.8 新增;Copilot 同样支持(强制占位规则) |
| 44 | `fast_mode`  🆕 | ✅ | ✅ | ✅ | ✅ | 4.8 研究预览;Copilot 仅容忍 speed/beta(200)不实现 |
| 45 | `prompt_cache_1024`  🆕 | ✅ | ✅ | ✅ | ✅ | 4.8 缓存下限降至 1024;前缀落在 1024–2048 区间并命中缓存(同前缀在 4.7 的 2048 下限下不缓存) |
| 46 | `refusal_stop_details`  🆕 | ✅ | ✅ | ✅ | ✅ | 4.8 文档化;本轮未触发硬拒绝,stop_details 未出现 |
| 47 | `model_discovery` | ✅ | ✅ | ✅ | ✅ | `/v1/models` 34 个模型,capabilities + max_ctx=1,000,000 |

**一句话结论**:在 47 项里,**②直连 与 ③经代理 完全一致**,唯二的「不一致」是 `cache_control_scope` 与 `code_execution_beta_header` —— 二者都是 Copilot2API **有意为之**(剥离非标准 `scope` / 剥离客户端 beta 头),并非缺陷。真正的「能力鸿沟」发生在 **①原生 与 ②③Copilot 之间**,见 §三。

---

## 三、关键差异解读

### 3.1 原生 vs Copilot:采样参数(`temperature` / `top_p` / `top_k`)— Copilot 更宽松
Anthropic 文档明确:Opus 4.8(同 4.7)对 `temperature` / `top_p` / `top_k` 设非默认值 **返回 400**,要求改用提示词。但 **Copilot 上游(直连与经代理一致)对 `temperature=0.0` / `top_p=0.5` / `top_k=10` 均返回 200**。即 Copilot 没有移植 Anthropic 的 4.7/4.8 采样硬约束 —— 对客户端更宽松,但也意味着这些参数在 Copilot 上的实际效果未必与原生一致(可能被静默接受/忽略)。这是**原生 vs Copilot 的行为偏差,非代理 bug**。

### 3.2 原生 vs Copilot:服务端工具与外链图(`web_search` / `computer_use` / `web_fetch` / `vision_url`)
这些是 Anthropic 原生支持的服务端能力,**Copilot 上游一律 4xx 拒绝**,直连与经代理表现一致。其中 `web_fetch` 直连报「unsupported beta header」,经代理报「rejected tool(s): web_fetch」(代理剥 beta 后由上游按工具维度拒),**拒绝来源不同但结果一致(均 4xx)**。`computer_use` 同理:直连由上游拒、经代理由代理本地 schema 校验拒,均 400。

### 3.3 原生 vs Copilot:`fast_mode`(容忍但不实现)
`speed:"fast"` + `fast-mode-2026-02-01` beta 是 4.8 的 Claude API 研究预览。实测 **Copilot 直连与经代理均返回 200** —— 上游既不拒 beta 头也不拒 `speed` 字段,但几乎可以肯定**并未真正提供 2.5× 加速**,只是「接受/忽略」。对比 `code_execution-2025-08-25` beta 会被 allowlist **拒**,可见 Copilot 的 beta 白名单并不一致。

### 3.4 输出上限:128k(`output_300k`)
原生同步 Messages API 与 Copilot **都**把 Opus 4.8 输出硬上限定在 **128k**(`max_tokens=200000` → 400 `> 128000`)。300k 仅在 Anthropic Batch API 经 `output-300k` beta 可达,不适用于本路径。直连=代理。

### 3.5 直连 vs 代理:`cache_control.scope` 剥离(预期内)
`scope` 非 Anthropic 标准字段,**直连被上游 400 拒**(`Extra inputs are not permitted`);**经代理 200** —— 代理在 `/v1/messages` 上有意剥离该非标准字段后再转发。这是 Copilot2API 的既定行为(见 CHANGELOG / README),非缺陷。

### 3.6 直连 vs 代理:`code_execution` beta 头剥离(预期内)
不带 beta 头时,**Copilot 实际执行 code_execution**(直连=代理,均 200,返回 `server_tool_use` + `bash_code_execution_tool_result`)。带 `code-execution-2025-08-25` beta 头时:**直连被上游 allowlist 拒(400)**,**经代理 200**(代理不转发客户端 beta,头被剥离后工具照常运行)。属预期内的直连/代理分叉。

### 3.7 4.8 新特性在 Copilot 上的落地情况(本轮新增 🆕)
- **`mid_conv_system`**:Copilot **支持**。两侧均强制 Anthropic 的占位规则(system 须「位于 assistant 之前或作为数组末尾」);非法摆放会得到与原生一致的 400 `role 'system' must precede an 'assistant' message or end the array`,合法摆放 200。
- **`prompt_cache_1024`**:Copilot **支持** 4.8 的下限降级。实测**落在 1024–2048 区间**的系统前缀成功命中缓存(直连 `cache_creation`、随后经代理 `cache_read`),证明 4.8 的 1024 下限生效(该前缀在 4.7 的 2048 下限下不会缓存)。注:Opus 4.7+ 采用新分词器,同一文本 token 数比旧分词器约多 30%,故该前缀按目标分词器定长。
- **`effort_max` / `effort_xhigh`**:Copilot **支持** 4.7/4.8 的 `xhigh` / `max` 顶档(均 200);非 Opus-4.7/4.8 机型则按预期 400 拒。
- **`refusal_stop_details`**:本轮**未触发硬拒绝**(模型给出软回答,`stop_reason=end_turn`/`max_tokens`,无 `stop_details`),故该字段形状未被实测覆盖;两侧一致。属非确定性探针。

### 3.8 非确定性说明
- **`stop_sequences`(#23)**:字段两侧均被接受;本轮直连恰好生成的文本未原样含 `STOP`,故 `stop_reason=end_turn`(历史运行中直连为 `stop_sequence`)。这是模型输出的非确定性,非能力差异。
- **adaptive thinking(`interleaved_thinking` #36)**:trivial 问题下 adaptive 模型自行决定不产出 `thinking` 块,两侧一致返回纯 `text`。`extended_thinking`(#8)在稍重的问题上则两侧都产出 `thinking` 块。

---

## 四、本轮新增到矩阵的用例(5)

| 用例 | 触发 | 期望(机型条件) | 实测(直连/代理) |
|---|---|---|---|
| `effort_max` | `output_config.effort:"max"` + adaptive | Opus 4.7/4.8 support,余 reject | 200 / 200 |
| `mid_conv_system` | `messages` 内 `role:"system"`(末尾占位) | support | 200 / 200 |
| `fast_mode` | `speed:"fast"` + `fast-mode-2026-02-01` beta | (Copilot 容忍)200 | 200 / 200 |
| `prompt_cache_1024` | ~1.2k–1.6k token 系统前缀(随分词器)+ `cache_control` | support(命中缓存) | cc / cr > 0 |
| `refusal_stop_details` | 易被拒提示,探 `stop_details` | 探针(不硬失败) | 200 / 200(未触发拒绝) |

> `temperature`/`top_p`/`top_k` 三项虽是既有用例,但本报告据官方文档把它们的「原生官方」列标注为 ⛔(原生 4.8 拒非默认采样),以呈现原生 vs Copilot 的偏差。

## 五、复现

```bash
# 全量 47 项,直连 + 经代理,Opus 4.8
python3 scripts/capability_test.py --target both --model claude-opus-4.8 \
  --account <your-account-dir> --proxy-url http://127.0.0.1:<port> --api-key sk-xxx \
  --report scripts/out/opus48-authoritative.md --raw scripts/out/opus48-authoritative-raw.json

# 仅本轮新增的 5 项
python3 scripts/capability_test.py --target both --model claude-opus-4.8 \
  --only effort_max,mid_conv_system,fast_mode,prompt_cache_1024,refusal_stop_details
```

`scripts/out/` 为 gitignore,原始 JSON 与逐行报告不入库;本对照报告为脱敏的可提交版本。
