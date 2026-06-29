# Claude Opus 4.8 模型功能说明

> 数据来源:Anthropic 官方文档(`platform.claude.com` — Models overview / Effort / Extended thinking,取于 2026-06-29)。
> 本文为 Opus 4.8 模型能力的「参照系」说明;三层实测对照(Anthropic 原生 / Copilot 直连 / 经 Copilot2API)见 [opus48-capability-report.md](opus48-capability-report.md)。

## 一、定位

Claude Opus 4.8 是 Anthropic **Opus 档最强模型**,面向复杂推理、长时程(long-horizon)智能体编码与高自主度任务。属于 Claude 4 系列,可用于 Claude API、Claude Platform on AWS、Amazon Bedrock、Google Cloud、Microsoft Foundry。

## 二、核心规格

| 维度 | 值 | 说明 |
| --- | --- | --- |
| Claude API ID | `claude-opus-4-8` | 无日期后缀的固定快照(pinned snapshot,非 evergreen) |
| Claude API 别名 | `claude-opus-4-8` | 4.6 代起别名与 ID 同形 |
| AWS Bedrock ID | `anthropic.claude-opus-4-8` | 走 Messages-API Bedrock 端点 |
| Google Cloud ID | `claude-opus-4-8` | — |
| 上下文窗口 | **1M tokens**(≈555k 词 / ≈2.5M Unicode 字符) | Microsoft Foundry 上为 **200K** |
| 最大输出 | **128K tokens** | 同步 Messages API 上限 |
| 扩展输出 | 300K tokens | 仅 Batch API,经 `output-300k-2026-03-24` beta |
| 输入模态 | 文本 + 图像(vision) | 文本输出;多语言 |
| 可靠知识截止 | **2026-01** | 训练数据截止亦为 2026-01 |
| 定价 | **$5 / $25** 每 MTok(输入 / 输出) | 与 Opus 4.6/4.7 同档 |
| 相对延迟 | Moderate(中等) | Sonnet 4.6 为 Fast |

## 三、思考与 effort

- **扩展思考(Extended thinking):不支持** — 无 `budget_tokens` 手动预算,显式设置返回 400。
- **自适应思考(Adaptive thinking):支持(始终在线)** — 通过 `thinking:{type:"adaptive"}`,模型自决是否思考及思考量。
- **effort 档位:`low / medium / high / xhigh / max`**,默认 **`high`**(含 Claude API 与 Claude Code,所有平台)。`xhigh` / `max` 为 Opus 4.7/4.8 高端档。需通过 `output_config.effort` 显式设置以切换。

## 四、4.8 相对 4.7 的新特性

1. **会话内 system 消息(Mid-conversation system messages)** — `messages` 数组中可插入 `role:"system"`,须「位于某个 assistant 消息之前,或作为数组末尾」;用于长会话追加指令而无需重述整个 system。无 beta。
2. **拒绝细节(Refusal `stop_details`)** — 硬拒绝在 `stop_reason:"refusal"` 之外附带 `stop_details` 描述拒绝类别。无 beta。
3. **Fast mode** — `speed:"fast"` + `fast-mode-2026-02-01` beta,溢价换约 2.5× 输出速率(Claude API 研究预览)。
4. **Prompt cache 下限降低** — 最小可缓存前缀降至 **1024 tokens**(4.7 为 2048)。
5. **effort `max` 顶档** — 5 档 effort 的最高档。

## 五、采样参数约束

Opus 4.8(同 4.7)对 `temperature` / `top_p` / `top_k` 设非默认值会 **返回 400**;官方建议改用提示词控制,而非采样参数。

## 六、分词器提示

Opus 4.7 起换用新分词器,同一文本 token 数较旧分词器约多 **30%**;按 token 计量(上下文、缓存前缀、计费)时需据此估算。

## 七、迁移与生命周期

- 从 Opus 4.7 及更早迁移:参见官方 *Migrating to Claude Opus 4.8*。
- Opus 4.1(`claude-opus-4-1-20250805`)已弃用,**2026-08-05 退役**,建议迁至 Opus 4.8。

> 可用 Models API(`GET /v1/models`)程序化查询能力与 token 限额,响应含 `max_input_tokens`、`max_tokens` 与 `capabilities`。
