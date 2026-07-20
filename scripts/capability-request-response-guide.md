# Claude 能力测试：请求与响应学习指南

> 生成时间：2026-07-16 15:52 ・ 模型：`claude-sonnet-4.6` ・ 上游账号：enterprise
> 数据来源：`scripts/capability_test.py --target both` 实测（直连 GitHub Copilot 上游 + copilot2api 代理）。
> 为可读性：响应中 Copilot 计费明细字段 `copilot_usage` 已省略；base64 数据与超长字符串已截断。
> 除特别标注外，下方展示的是**代理**（`/v1/messages` 原生路由）的请求与响应；直连结果不同时会单独列出。

## 目录

**基础能力**
- 1. [`text`](#1-text) 基础文本
- 2. [`streaming`](#2-streaming) 流式输出

**工具调用**
- 3. [`function_calling`](#3-function-calling) 函数调用
- 4. [`parallel_tools`](#4-parallel-tools) 并行工具调用

**多模态**
- 5. [`vision_base64`](#5-vision-base64) 图像理解（base64）
- 6. [`vision_url`](#6-vision-url) 图像理解（URL）
- 7. [`pdf_document`](#7-pdf-document) PDF 文档理解

**思考/推理**
- 8. [`extended_thinking`](#8-extended-thinking) 扩展思考

**内置工具**
- 9. [`server_tool_bash`](#9-server-tool-bash) Bash 工具
- 10. [`server_tool_text_editor`](#10-server-tool-text-editor) 文本编辑器工具
- 11. [`server_tool_memory`](#11-server-tool-memory) 记忆工具

**缓存/上下文**
- 12. [`prompt_cache`](#12-prompt-cache) 提示缓存（块级断点）
- 13. [`cache_control_scope`](#13-cache-control-scope) 缓存 scope 字段
- 14. [`context_management`](#14-context-management) 上下文编辑（清理工具结果）

**RAG/输出**
- 15. [`citations`](#15-citations) 文档引用

**服务端工具**
- 16. [`web_search`](#16-web-search) 联网搜索

**内置工具**
- 17. [`computer_use`](#17-computer-use) 计算机操作

**基础能力**
- 18. [`count_tokens`](#18-count-tokens) Token 计数

**缓存/上下文**
- 19. [`context_1m`](#19-context-1m) 1M 上下文

**采样参数**
- 20. [`temperature`](#20-temperature) 温度采样
- 21. [`top_p`](#21-top-p) 核采样
- 22. [`top_k`](#22-top-k) Top-K 采样
- 23. [`stop_sequences`](#23-stop-sequences) 自定义停止符
- 24. [`metadata`](#24-metadata) 请求元数据
- 25. [`service_tier`](#25-service-tier) 服务层级

**工具调用**
- 26. [`tool_choice_auto`](#26-tool-choice-auto) 工具选择：自动
- 27. [`tool_choice_any`](#27-tool-choice-any) 工具选择：强制任一
- 28. [`tool_choice_tool`](#28-tool-choice-tool) 工具选择：强制指定
- 29. [`tool_choice_none`](#29-tool-choice-none) 工具选择：禁用
- 30. [`tool_choice_no_parallel`](#30-tool-choice-no-parallel) 禁用并行工具

**RAG/输出**
- 31. [`structured_outputs`](#31-structured-outputs) 结构化输出（JSON Schema）

**服务端工具**
- 32. [`web_fetch`](#32-web-fetch) 网页抓取
- 33. [`code_execution`](#33-code-execution) 代码执行
- 34. [`code_execution_beta_header`](#34-code-execution-beta-header) 代码执行（带 beta 头）

**RAG/输出**
- 35. [`search_result`](#35-search-result) 搜索结果块

**思考/推理**
- 36. [`interleaved_thinking`](#36-interleaved-thinking) 交错思考

**工具调用**
- 37. [`token_efficient_tools`](#37-token-efficient-tools) 高效工具 Token
- 38. [`fine_grained_tool_streaming`](#38-fine-grained-tool-streaming) 细粒度工具流

**缓存/上下文**
- 39. [`extended_cache_ttl`](#39-extended-cache-ttl) 1 小时缓存

**思考/推理**
- 40. [`effort_xhigh`](#40-effort-xhigh) effort=xhigh

**RAG/输出**
- 41. [`output_300k`](#41-output-300k) 300k 输出上限

**思考/推理**
- 42. [`effort_max`](#42-effort-max) effort=max
- 43. [`thinking_budget`](#43-thinking-budget) 手动思考预算

**平台/端点**
- 44. [`mid_conv_system`](#44-mid-conv-system) 对话中途 system 消息
- 45. [`fast_mode`](#45-fast-mode) 快速模式

**缓存/上下文**
- 46. [`prompt_cache_1024`](#46-prompt-cache-1024) 1024 token 缓存下限

**平台/端点**
- 47. [`refusal_stop_details`](#47-refusal-stop-details) 拒答详情

**基础能力**
- 48. [`model_discovery`](#48-model-discovery) 模型发现

**缓存/上下文**
- 49. [`auto_prompt_cache`](#49-auto-prompt-cache) 自动提示缓存

**工具调用**
- 50. [`strict_tool_use`](#50-strict-tool-use) 严格工具调用

**平台/端点**
- 51. [`inference_geo`](#51-inference-geo) 数据驻留

**工具基础设施**
- 52. [`mcp_connector`](#52-mcp-connector) MCP 连接器
- 53. [`tool_search`](#53-tool-search) 工具搜索
- 54. [`programmatic_tool_calling`](#54-programmatic-tool-calling) 编程式工具调用
- 55. [`agent_skills`](#55-agent-skills) Agent 技能

**服务端工具**
- 56. [`advisor_tool`](#56-advisor-tool) 顾问工具

**缓存/上下文**
- 57. [`compaction`](#57-compaction) 服务端压缩

**平台/端点**
- 58. [`server_side_fallback`](#58-server-side-fallback) 服务端回退
- 59. [`batches_endpoint`](#59-batches-endpoint) 批量处理 API
- 60. [`files_endpoint`](#60-files-endpoint) Files API


---

# 基础能力

## 1. text

**基础文本**（期望：支持）

最基本的文本问答，验证请求→响应链路。响应主体是 `content` 数组里的 `text` 块，`usage` 里回报输入/输出 token。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01DrWJ5LUL1eN1SYNzMyTmKf",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

## 2. streaming

**流式输出**（期望：支持）

`stream: true` 时以 SSE 事件流逐段返回：`message_start` → `content_block_start` → 多个 `content_block_delta` → `content_block_stop` → `message_delta`（带 stop_reason/usage）→ `message_stop`。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "stream": true,
  "messages": [
    {
      "role": "user",
      "content": "Count from 1 to 5."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
SSE 流式响应，共 8 个事件，事件类型：`content_block_delta`, `content_block_start`, `content_block_stop`, `message_delta`, `message_start`, `message_stop`。前若干事件：
```json
[
  {
    "message": {
      "content": [],
      "id": "msg_bdrk_01DABCPsGX5TwDweJ5apqymy",
      "model": "claude-sonnet-4-6",
      "role": "assistant",
      "stop_details": null,
      "stop_reason": null,
      "stop_sequence": null,
      "type": "message",
      "usage": {
        "cache_creation": {
          "ephemeral_1h_input_tokens": 0,
          "ephemeral_5m_input_tokens": 0
        },
        "cache_creation_input_tokens": 0,
        "cache_read_input_tokens": 0,
        "input_tokens": 16,
        "output_tokens": 3
      }
    },
    "type": "message_start"
  },
  {
    "content_block": {
      "text": "",
      "type": "text"
    },
    "index": 0,
    "type": "content_block_start"
  },
  {
    "delta": {
      "text": "1,",
      "type": "text_delta"
    },
    "index": 0,
    "type": "content_block_delta"
  },
  {
    "delta": {
      "text": " 2, 3, 4",
      "type": "text_delta"
    },
    "index": 0,
    "type": "content_block_delta"
  },
  {
    "delta": {
      "text": ", 5",
      "type": "text_delta"
    },
    "index": 0,
    "type": "content_block_delta"
  },
  {
    "index": 0,
    "type": "content_block_stop"
  },
  {
    "copilot_usage": {
      "token_details": [
        {
          "batch_size": 1000000,
          "cost_per_batch": 300000000000,
          "token_count": 16,
          "token_type": "input"
        },
        {
          "batch_size": 1000000,
          "cost_per_batch": 30000000000,
          "token_count": 0,
          "token_type": "cache_read"
        },
        {
          "batch_size": 1000000,
          "cost_per_batch": 375000000000,
          "token_count": 0,
          "token_type": "cache_write"
        },
        {
          "batch_size": 1000000,
          "cost_per_batch": 1500000000000,
          "token_count": 17,
          "token_type": "output"
        }
      ],
      "total_nano_aiu": 30300000
    },
    "delta": {
      "stop_details": null,
      "stop_reason": "end_turn",
      "stop_sequence": null
    },
    "type": "message_delta",
    "usage": {
      "cache_creation_input_tokens": 0,
      "cache_read_input_tokens": 0,
      "input_tokens": 16,
      "output_tokens": 17
    }
  },
  {
    "amazon-bedrock-invocationMetrics": {
      "firstByteLatency": 4016,
      "inputTokenCount": 16,
      "invocationLatency": 4164,
      "outputTokenCount": 17
    },
    "type": "message_stop"
  }
]
```

---

# 工具调用

## 3. function_calling

**函数调用**（期望：支持）

在 `tools` 里定义函数（名称+JSON Schema 入参），模型返回 `tool_use` 内容块（含 `id`/`name`/`input`），`stop_reason` 为 `tool_use`。客户端执行后以 `tool_result` 块回传继续对话。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in Paris? Use the tool."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "id": "toolu_bdrk_01Gduoj67Uh645TQjY2g5sFT",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01WFhmzE3yx84bGVw6DF3v7w",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 569,
    "output_tokens": 54
  }
}
```

## 4. parallel_tools

**并行工具调用**（期望：支持）

一次响应中返回多个 `tool_use` 块并行调用多个工具，减少往返轮次。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    },
    {
      "name": "get_time",
      "description": "Get current time for a timezone",
      "input_schema": {
        "type": "object",
        "properties": {
          "tz": {
            "type": "string"
          }
        },
        "required": [
          "tz"
        ]
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Get BOTH the weather in Paris and the time in Asia/Tokyo. Call both tools."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "Sure! I'll fetch both the weather in Paris and the current time in Asia/Tokyo simultaneously right away!",
      "type": "text"
    },
    {
      "id": "toolu_bdrk_013h4SrVKD44Nfs3xDHYV6Ca",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    },
    {
      "id": "toolu_bdrk_01DANp4TLnMo83GTt3c1byyx",
      "input": {
        "tz": "Asia/Tokyo"
      },
      "name": "get_time",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01E8yBVUL8btpCcrsyfJxFoF",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 633,
    "output_tokens": 115
  }
}
```

---

# 多模态

## 5. vision_base64

**图像理解（base64）**（期望：支持）

`image` 内容块内嵌 base64 图片数据（`source.type: "base64"` + `media_type` + `data`），模型描述/分析图像内容。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "image",
          "source": {
            "type": "base64",
            "media_type": "image/png",
            "data": "iVBORw0KGgoAAAANSUhEUgAAAMgAAADICAIAAAAiOjnJAAABeElEQVR42u3S...(base64, 共 414 字符已截断)"
          }
        },
        {
          "type": "text",
          "text": "What color dominates this image? One word."
        }
      ]
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "**Crimson**",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01Kgqyz3E9e47urXPoVRjbsv",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 85,
    "output_tokens": 8
  }
}
```

## 6. vision_url

**图像理解（URL）**（期望：拒绝/缺失）

`source.type: "url"` 引用外部图片 URL。Copilot 上游不支持外链图片 → 400 拒绝（固化行为探针）。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "image",
          "source": {
            "type": "url",
            "url": "https://example.com/x.png"
          }
        },
        {
          "type": "text",
          "text": "Describe."
        }
      ]
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "error": {
    "message": "external image URLs are not supported",
    "code": ""
  }
}
```

## 7. pdf_document

**PDF 文档理解**（期望：支持）

`document` 内容块内嵌 base64 PDF（`media_type: "application/pdf"`），模型解析文本与版面视觉内容。

**请求** `POST /v1/messages`（anthropic-beta: pdfs-2024-09-25）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "document",
          "source": {
            "type": "base64",
            "media_type": "application/pdf",
            "data": "JVBERi0xLjQKMSAwIG9iago8PCAvVHlwZSAvQ2F0YWxvZyAvUGFnZXMgMiAw...(base64, 共 414 字符已截断)"
          }
        },
        {
          "type": "text",
          "text": "What secret marker appears in this PDF?"
        }
      ]
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "The secret marker in the PDF is **BANANA**.",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_014P99GzU61R3wxbeBzKUky7",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 1606,
    "output_tokens": 14
  }
}
```

---

# 思考/推理

## 8. extended_thinking

**扩展思考**（期望：支持）

`thinking` 参数开启推理过程外显：响应先出 `thinking` 块（可见的逐步推理）再出 `text` 块。有手动 budget（`type:"enabled"+budget_tokens`）与自适应（`type:"adaptive"`）两种形态，用例带双 variant 兼容不同模型。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 2048,
  "thinking": {
    "type": "enabled",
    "budget_tokens": 1024
  },
  "messages": [
    {
      "role": "user",
      "content": "Think briefly, then answer: what is 17 * 23?"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "signature": "EswBCmcIDxABGAIqQGO/A+F/j0tzmWTO/uWXC1SOSU9svZiAehnCxpMWzbOyDGJC+cw8F4Jh7+s3fb6aW0QQQiHmy75OIFn51NC5pOYyEWNsYXVkZS1zb25uZXQtNC02OABCCHRoaW5raW5nEgwatOQFNUMOG8MOGwQaDP3Fp8MKrOut69+8ESIwOwhlOKcEv/BqTOcRfkc5arqPNHIfP5IEh4zzlufybVp5tbxLGlzB87mY+6jQKYukKhMU7eAK5Wf8T4twQ2wpMw/NYQcKGAE=",
      "thinking": "391",
      "type": "thinking"
    },
    {
      "text": "**17 × 23 = 391**\n\nQuick breakdown: 17 × 20 = 340, plus 17 × 3 = 51 → 340 + 51 = **391**",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01NQEXWdJNdLKZSx6fArRCSX",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 51,
    "output_tokens": 62,
    "output_tokens_details": {
      "thinking_tokens": 7
    }
  }
}
```

---

# 内置工具

## 9. server_tool_bash

**Bash 工具**（期望：支持）

内置 `bash_20250124` 工具类型：模型请求执行 shell 命令（返回 `tool_use`，由客户端实际执行）。Claude Code 等 Agent 的基础。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "bash_20250124",
      "name": "bash"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Use the bash tool to help with a trivial task, or just acknowledge."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "id": "toolu_bdrk_01UcfP4eiC7QYyyqkVwFZNqX",
      "input": {
        "command": "echo \"Ready to help!\""
      },
      "name": "bash",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01SVTZ7H24T8BRK36eMTW6jY",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 764,
    "output_tokens": 57
  }
}
```

## 10. server_tool_text_editor

**文本编辑器工具**（期望：支持）

内置 `text_editor_20250728` 工具（`str_replace_based_edit_tool`）：查看/创建/编辑文件的标准接口。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "text_editor_20250728",
      "name": "str_replace_based_edit_tool"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Use the text_editor tool to help with a trivial task, or just acknowledge."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "Sure! I'm ready to help with any text editing tasks. The `str_replace_based_edit_tool` is available and can:\n\n- **`view`** – View files or directory structures\n- **`create`** – Create or overwrite files\n- **`str_replace`** – Replace specific text in a file\n- **`insert`** – Insert text at a specific line\n\nJust let me know what you'd like to do — whether it's creating a new file, editing an existing...(truncated)",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01BwvsWCmobmfkvXyp64HvqS",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 1267,
    "output_tokens": 126
  }
}
```

## 11. server_tool_memory

**记忆工具**（期望：支持）

内置 `memory_20250818` 工具：跨会话存取记忆，构建长期知识库。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "memory_20250818",
      "name": "memory"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Use the memory tool to help with a trivial task, or just acknowledge."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "id": "toolu_bdrk_011Z2Ez6vrewBsNMehaSnePs",
      "input": {
        "command": "view",
        "path": "/memories"
      },
      "name": "memory",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01BxTfJEPH2YWm6PFr8WBtkm",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 1589,
    "output_tokens": 70
  }
}
```

---

# 缓存/上下文

## 12. prompt_cache

**提示缓存（块级断点）**（期望：支持）

在内容块上加 `cache_control: {type:"ephemeral"}` 设缓存断点。首次请求 `cache_creation_input_tokens` > 0（写缓存，1.25x），后续命中 `cache_read_input_tokens`（0.1x）。本次运行响应显示 `cr: 1202` 即命中了之前的缓存。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "system": [
    {
      "type": "text",
      "text": "You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assis...(truncated)",
      "cache_control": {
        "type": "ephemeral"
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Say hi."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "Hi! 👋 How can I help you today?",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01CZxZFWtwLL7jZs3pyLGNER",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 1202
    },
    "cache_creation_input_tokens": 1202,
    "cache_read_input_tokens": 0,
    "input_tokens": 9,
    "output_tokens": 16
  }
}
```

## 13. cache_control_scope

**缓存 scope 字段**（期望：支持）

Claude Code 会发 `cache_control.scope: "global"`（控制缓存跨地域共享）。上游不认识该字段会 400；本用例验证代理正确剥离 scope 且缓存仍生效。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "system": [
    {
      "type": "text",
      "text": "You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assistant. You are a helpful assis...(truncated)",
      "cache_control": {
        "type": "ephemeral",
        "scope": "global"
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Say hi."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "Hi! 👋 How are you doing? Is there something I can help you with today?",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01HRTibx8atHdjn5S45a7UgM",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 1202,
    "input_tokens": 9,
    "output_tokens": 24
  }
}
```

**直连上游差异**（HTTP 400）
```json
{
  "type": "error",
  "error": {
    "type": "invalid_request_error",
    "message": "system.0.cache_control.ephemeral.scope: Extra inputs are not permitted"
  },
  "request_id": "req_011Cd5MhDWU95csxMb5nghzy"
}
```

## 14. context_management

**上下文编辑（清理工具结果）**（期望：支持）

`context_management.edits` 的 `clear_tool_uses_20250919` 策略：超过 trigger 阈值后服务端自动清除最旧的工具结果。响应的 `context_management.applied_edits` 回报实际清理量。代理自动补 `context-management-2025-06-27` beta 头。

**请求** `POST /v1/messages`（anthropic-beta: context-management-2025-06-27）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 256,
  "tools": [
    {
      "name": "lookup",
      "description": "Look up a record",
      "input_schema": {
        "type": "object",
        "properties": {
          "id": {
            "type": "string"
          }
        },
        "required": [
          "id"
        ]
      }
    }
  ],
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919",
        "trigger": {
          "type": "input_tokens",
          "value": 1
        },
        "keep": {
          "type": "tool_uses",
          "value": 0
        }
      }
    ]
  },
  "messages": [
    {
      "role": "user",
      "content": "Look up record 42."
    },
    {
      "role": "assistant",
      "content": [
        {
          "type": "tool_use",
          "id": "toolu_cm1",
          "name": "lookup",
          "input": {
            "id": "42"
          }
        }
      ]
    },
    {
      "role": "user",
      "content": [
        {
          "type": "tool_result",
          "tool_use_id": "toolu_cm1",
          "content": "RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECORD DATA RECO...(truncated)"
        }
      ]
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "The lookup for record **42** has been completed, but the result was cleared or returned empty. This could mean:\n\n- The record **42** does not exist in the system.\n- The data was intentionally cleared or is unavailable.\n\nWould you like me to try again, or is there another record you'd like to look up?",
      "type": "text"
    }
  ],
  "context_management": {
    "applied_edits": [
      {
        "cleared_input_tokens": 1214,
        "cleared_tool_uses": 1,
        "type": "clear_tool_uses_20250919"
      }
    ]
  },
  "id": "msg_bdrk_014Uuo8PBq9SKRRHegZacd11",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 632,
    "output_tokens": 75
  }
}
```

---

# RAG/输出

## 15. citations

**文档引用**（期望：支持）

`document` 块开启 `citations: {enabled: true}` 后，回答中的 `text` 块携带 `citations` 数组，精确到句子级出处（char_location）。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "document",
          "source": {
            "type": "base64",
            "media_type": "application/pdf",
            "data": "JVBERi0xLjQKMSAwIG9iago8PCAvVHlwZSAvQ2F0YWxvZyAvUGFnZXMgMiAw...(base64, 共 414 字符已截断)"
          },
          "citations": {
            "enabled": true
          }
        },
        {
          "type": "text",
          "text": "What is the secret marker? Cite the document."
        }
      ]
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "citations": [
        {
          "cited_text": "The secret marker is BANANA",
          "document_index": 0,
          "document_title": null,
          "end_page_number": 2,
          "start_page_number": 1,
          "type": "page_location"
        }
      ],
      "text": "The secret marker is **BANANA**.",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01JhFZED5pKws9KrtZVyLugv",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 2125,
    "output_tokens": 28
  }
}
```

---

# 服务端工具

## 16. web_search

**联网搜索**（期望：拒绝/缺失）

`web_search_20250305` 服务端搜索工具。Copilot 上游明确拒绝（400 "not supported"）——reject 探针。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "web_search_20250305",
      "name": "web_search"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Search the web for today's news."
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "error": {
    "message": "The use of the web search tool is not supported.",
    "code": "unsupported_value"
  }
}
```

---

# 内置工具

## 17. computer_use

**计算机操作**（期望：支持）

`computer_20251124` 工具 + `computer-use-2025-11-24` beta 头：截图+鼠标键盘控制 GUI。代理会转发客户端的 computer-use beta 头（这是之前版本修复的缺口）。

**请求** `POST /v1/messages`（anthropic-beta: computer-use-2025-11-24）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "computer_20251124",
      "name": "computer",
      "display_width_px": 1024,
      "display_height_px": 768
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Take a screenshot."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "I'll take a screenshot right away!",
      "type": "text"
    },
    {
      "id": "toolu_bdrk_01VMygiqyXYb9zhQMyQNfjEY",
      "input": {
        "action": "screenshot"
      },
      "name": "computer",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01K4P1WertqsydMULuf4fD5F",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 1833,
    "output_tokens": 60
  }
}
```

---

# 基础能力

## 18. count_tokens

**Token 计数**（期望：支持）

`POST /v1/messages/count_tokens`：发送前预估消息 token 数，响应只有 `{"input_tokens": N}`。

**请求** `POST /v1/messages/count_tokens`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "messages": [
    {
      "role": "user",
      "content": "How many tokens is this sentence?"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "input_tokens": 14
}
```

---

# 缓存/上下文

## 19. context_1m

**1M 上下文**（期望：支持）

`context-1m-2025-08-07` beta 头请求 1M token 上下文窗口。新模型（Sonnet 4.6+）基础模型即 1M，代理不再盲目加 `-1m` 后缀。

**请求** `POST /v1/messages`（anthropic-beta: context-1m-2025-08-07）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01WXaykySPRDgTpPMnpAtkQM",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

---

# 采样参数

## 20. temperature

**温度采样**（期望：支持）

`temperature` 控制输出随机性（0=确定性，1=最大随机）。验证原生透传逐字段转发。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "temperature": 0.0,
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01VSdL4s2e6sz6eAoUQfA26H",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

## 21. top_p

**核采样**（期望：支持）

`top_p` 按累计概率截断候选词表（与 temperature 二选一调节）。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "top_p": 0.5,
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01QRstSnoBnhWhHomhVLE31M",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

## 22. top_k

**Top-K 采样**（期望：支持）

`top_k` 只从概率最高的 K 个候选中采样。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "top_k": 10,
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01R9AukHdWvPMLGU39iZoZep",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

## 23. stop_sequences

**自定义停止符**（期望：支持）

`stop_sequences` 数组：模型输出遇到指定字符串即停止，`stop_reason` 为 `stop_sequence`，`stop_sequence` 字段回报命中的是哪个。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "stop_sequences": [
    "STOP"
  ],
  "messages": [
    {
      "role": "user",
      "content": "Repeat this text verbatim and nothing else: alpha bravo STOP charlie delta"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "alpha bravo ",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01XC2aYsU1JzZQAMVp8MxVms",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "stop_sequence",
  "stop_sequence": "STOP",
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 25,
    "output_tokens": 5
  }
}
```

## 24. metadata

**请求元数据**（期望：支持）

`metadata.user_id` 给请求打标（滥用追踪/审计），不影响生成。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "metadata": {
    "user_id": "capability-test-user"
  },
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01CxBFm6oUCg1einisnFkw6d",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

## 25. service_tier

**服务层级**（期望：支持）

`service_tier`（auto/standard_only）选择优先级容量档位。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "service_tier": "auto",
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01B4aa8XGWQJqodnauWd8szr",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

---

# 工具调用

## 26. tool_choice_auto

**工具选择：自动**（期望：支持）

`tool_choice: {type:"auto"}`（默认）：模型自行决定是否调用工具。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    }
  ],
  "tool_choice": {
    "type": "auto"
  },
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in Paris?"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "id": "toolu_bdrk_01LRKSGXyRCkZAiVpMNxokUK",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01Jvbt9yPLHbEGEQ5sCGSDTj",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 565,
    "output_tokens": 54
  }
}
```

## 27. tool_choice_any

**工具选择：强制任一**（期望：支持）

`tool_choice: {type:"any"}`：必须调用至少一个工具，不允许纯文本回答。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    },
    {
      "name": "get_time",
      "description": "Get current time for a timezone",
      "input_schema": {
        "type": "object",
        "properties": {
          "tz": {
            "type": "string"
          }
        },
        "required": [
          "tz"
        ]
      }
    }
  ],
  "tool_choice": {
    "type": "any"
  },
  "messages": [
    {
      "role": "user",
      "content": "Help me plan a trip to Paris."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "id": "toolu_bdrk_019txnbU5MJG5CPoVpnzDXMg",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    },
    {
      "id": "toolu_bdrk_01G2BPxkXY4v7CkvsSXyteGS",
      "input": {
        "tz": "Europe/Paris"
      },
      "name": "get_time",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01RqzQVKfeCe4RxtcB5FncMf",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 714,
    "output_tokens": 77
  }
}
```

## 28. tool_choice_tool

**工具选择：强制指定**（期望：支持）

`tool_choice: {type:"tool", name:"get_weather"}`：强制调用指定名称的工具。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    },
    {
      "name": "get_time",
      "description": "Get current time for a timezone",
      "input_schema": {
        "type": "object",
        "properties": {
          "tz": {
            "type": "string"
          }
        },
        "required": [
          "tz"
        ]
      }
    }
  ],
  "tool_choice": {
    "type": "tool",
    "name": "get_weather"
  },
  "messages": [
    {
      "role": "user",
      "content": "Do something useful for Paris."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "id": "toolu_bdrk_017LYLVTtRNc1pTGG65SY4bh",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    },
    {
      "id": "toolu_bdrk_01BCpnfRAJtPq7Ky5o1tbiMM",
      "input": {
        "tz": "Europe/Paris"
      },
      "name": "get_time",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01Quvd9FNeZZpiPr87JoJSTQ",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 717,
    "output_tokens": 72
  }
}
```

## 29. tool_choice_none

**工具选择：禁用**（期望：支持）

`tool_choice: {type:"none"}`：禁止调用工具，只能文本回答（`stop_reason: end_turn`）。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    }
  ],
  "tool_choice": {
    "type": "none"
  },
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in Paris? Answer in plain text, do not call any tool."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "I'm sorry, but I don't have the ability to provide current weather information without using tools. My built-in knowledge doesn't include real-time data like live weather updates. I'd recommend checking a weather website or app (such as weather.com or a local forecast service) for the most accurate and up-to-date weather in Paris.",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01USLh8HxN2JyMLBHMhJnwbJ",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 576,
    "output_tokens": 75
  }
}
```

## 30. tool_choice_no_parallel

**禁用并行工具**（期望：支持）

`disable_parallel_tool_use: true`：每轮最多一个 `tool_use` 块，适合顺序敏感的操作。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    },
    {
      "name": "get_time",
      "description": "Get current time for a timezone",
      "input_schema": {
        "type": "object",
        "properties": {
          "tz": {
            "type": "string"
          }
        },
        "required": [
          "tz"
        ]
      }
    }
  ],
  "tool_choice": {
    "type": "any",
    "disable_parallel_tool_use": true
  },
  "messages": [
    {
      "role": "user",
      "content": "Get BOTH the weather in Paris and the time in Asia/Tokyo."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "id": "toolu_bdrk_012o2jsFAeGrKWtkff3E9vax",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01WWhLHdVEdUAhPEhLS2ZnWs",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 722,
    "output_tokens": 29
  }
}
```

---

# RAG/输出

## 31. structured_outputs

**结构化输出（JSON Schema）**（期望：支持）

`output_config.format` 指定 JSON Schema，约束解码保证响应是合法且符合 schema 的 JSON——无需重试解析。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": "Extract structured data: it is 22 degrees Celsius in Paris right now."
    }
  ],
  "output_config": {
    "format": {
      "type": "json_schema",
      "schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          },
          "temp_c": {
            "type": "number"
          }
        },
        "required": [
          "city",
          "temp_c"
        ],
        "additionalProperties": false
      }
    }
  }
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "{\"city\":\"Paris\",\"temp_c\":22}",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01DE2QLF5gqo8qbjH2gSyXDo",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 192,
    "output_tokens": 14
  }
}
```

---

# 服务端工具

## 32. web_fetch

**网页抓取**（期望：拒绝/缺失）

`web_fetch_20250910` 工具 + beta 头：抓取指定 URL 全文。上游拒绝 → reject 探针。

**请求** `POST /v1/messages`（anthropic-beta: web-fetch-2025-09-10）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "web_fetch_20250910",
      "name": "web_fetch"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Fetch https://example.com and summarize it."
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "error": {
    "message": "rejected tool(s): web_fetch",
    "code": "invalid_request_body"
  }
}
```

## 33. code_execution

**代码执行**（期望：拒绝/缺失）

`code_execution_20250825` 沙箱代码执行工具。上游曾支持（2026-07 上旬实测通过），现已从接受的工具类型列表移除 → 400。reject 探针固化当前行为，若上游恢复会 DIFF 报警。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "code_execution_20250825",
      "name": "code_execution"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Use code execution to compute the mean of [1,2,3,4,5]."
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "message": "tools.0: Input tag 'code_execution_20250825' found using 'type' does not match any of the expected tags: 'bash_20250124', 'custom', 'memory_20250818', 'text_editor_20250124', 'text_editor_20250429', 'text_editor_20250728', 'tool_search_tool_bm25', 'tool_search_tool_bm25_20251119', 'tool_search_tool_regex', 'tool_search_tool_regex_20251119'"
}
```

## 34. code_execution_beta_header

**代码执行（带 beta 头）**（期望：拒绝/缺失）

同上，附 `code-execution-2025-08-25` beta 头的变体。直连被 beta 允许列表拒绝；代理剥离 beta 后同样被工具类型校验拒绝。

**请求** `POST /v1/messages`（anthropic-beta: code-execution-2025-08-25）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "code_execution_20250825",
      "name": "code_execution"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Use code execution to compute the mean of [1,2,3,4,5]."
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "message": "tools.0: Input tag 'code_execution_20250825' found using 'type' does not match any of the expected tags: 'bash_20250124', 'custom', 'memory_20250818', 'text_editor_20250124', 'text_editor_20250429', 'text_editor_20250728', 'tool_search_tool_bm25', 'tool_search_tool_bm25_20251119', 'tool_search_tool_regex', 'tool_search_tool_regex_20251119'"
}
```

---

# RAG/输出

## 35. search_result

**搜索结果块**（期望：支持）

`search_result` 内容块：传入带 `source`/`title` 的自有搜索结果，模型回答带 `search_result_location` 引用——让自建 RAG 获得联网搜索级的引用体验。代理曾因 source 是裸字符串解析失败，已修复。

**请求** `POST /v1/messages`（anthropic-beta: search-results-2025-06-09）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "search_result",
          "source": "https://example.com/doc",
          "title": "Capability Test Doc",
          "content": [
            {
              "type": "...",
              "text": "..."
            }
          ],
          "citations": {
            "enabled": true
          }
        },
        {
          "type": "text",
          "text": "What is the secret marker? Cite the source."
        }
      ]
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "citations": [
        {
          "cited_text": "The secret marker is BANANA-42.",
          "end_block_index": 1,
          "search_result_index": 0,
          "source": "https://example.com/doc",
          "start_block_index": 0,
          "title": "Capability Test Doc",
          "type": "search_result_location"
        }
      ],
      "text": "The secret marker is **BANANA-42**.",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01U2tediD3RKUDaJfYJatEfo",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 597,
    "output_tokens": 30
  }
}
```

---

# 思考/推理

## 36. interleaved_thinking

**交错思考**（期望：支持）

`interleaved-thinking-2025-05-14` beta：允许在多次工具调用之间穿插 `thinking` 块，多步 Agent 推理更连贯。代理剥离该 beta 头但请求仍成功（头仅影响思考位置策略）。

**请求** `POST /v1/messages`（anthropic-beta: interleaved-thinking-2025-05-14）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 2048,
  "thinking": {
    "type": "enabled",
    "budget_tokens": 1024
  },
  "messages": [
    {
      "role": "user",
      "content": "Think briefly, then answer: what is 12 * 12?"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "signature": "EswBCmcIDxABGAIqQCp4VioMsLG6i9DGAyPtTtxG0AK8sEmwNkQTGKdmwuYhAThAmQGRA/zKLWE6nwIVdWwF3jTiZHwE2QXPS8w9C6syEWNsYXVkZS1zb25uZXQtNC02OABCCHRoaW5raW5nEgyBueoFlQVrdjOtD60aDHW8vRzeXtQXnwxGhSIwCGXfq8DFNQLik9/fwrqDu0N1mCHHr7Kn2ppJCWLnxXlkeXXgnQlHZkRH8czWJ9VGKhMKGpuWCeSB9T9aHAKM9TODlQZTGAE=",
      "thinking": "144",
      "type": "thinking"
    },
    {
      "text": "**144**",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_013yJ14VtNKsyJYo2uq9RN1A",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 51,
    "output_tokens": 15,
    "output_tokens_details": {
      "thinking_tokens": 7
    }
  }
}
```

---

# 工具调用

## 37. token_efficient_tools

**高效工具 Token**（期望：支持）

`token-efficient-tools-2025-02-19` beta：压缩工具调用的 token 开销（老模型优化，新模型默认高效）。

**请求** `POST /v1/messages`（anthropic-beta: token-efficient-tools-2025-02-19）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in Paris? Use the tool."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "id": "toolu_bdrk_01MJLS4mAga87miKQab1yRgn",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01SEHBSnjqMZFKo3BMKkSa3N",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 569,
    "output_tokens": 54
  }
}
```

## 38. fine_grained_tool_streaming

**细粒度工具流**（期望：支持）

`fine-grained-tool-streaming-2025-05-14` beta：流式输出工具参数时不做缓冲和 JSON 校验，大参数低延迟到达（`input_json_delta` 事件）。

**请求** `POST /v1/messages`（anthropic-beta: fine-grained-tool-streaming-2025-05-14）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "stream": true,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in Paris? Use the tool."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
SSE 流式响应，共 9 个事件，事件类型：`content_block_delta`, `content_block_start`, `content_block_stop`, `message_delta`, `message_start`, `message_stop`。前若干事件：
```json
[
  {
    "message": {
      "content": [],
      "id": "msg_bdrk_01Q2vVZV87LvR3mvBuU958xb",
      "model": "claude-sonnet-4-6",
      "role": "assistant",
      "stop_details": null,
      "stop_reason": null,
      "stop_sequence": null,
      "type": "message",
      "usage": {
        "cache_creation": {
          "ephemeral_1h_input_tokens": 0,
          "ephemeral_5m_input_tokens": 0
        },
        "cache_creation_input_tokens": 0,
        "cache_read_input_tokens": 0,
        "input_tokens": 569,
        "output_tokens": 21
      }
    },
    "type": "message_start"
  },
  {
    "content_block": {
      "id": "toolu_bdrk_01LsJrB3t1eBtNqkYTbzxgvV",
      "input": {},
      "name": "get_weather",
      "type": "tool_use"
    },
    "index": 0,
    "type": "content_block_start"
  },
  {
    "delta": {
      "partial_json": "",
      "type": "input_json_delta"
    },
    "index": 0,
    "type": "content_block_delta"
  },
  {
    "delta": {
      "partial_json": "{\"",
      "type": "input_json_delta"
    },
    "index": 0,
    "type": "content_block_delta"
  },
  {
    "delta": {
      "partial_json": "city\": \"P",
      "type": "input_json_delta"
    },
    "index": 0,
    "type": "content_block_delta"
  },
  {
    "delta": {
      "partial_json": "aris\"}",
      "type": "input_json_delta"
    },
    "index": 0,
    "type": "content_block_delta"
  },
  {
    "index": 0,
    "type": "content_block_stop"
  },
  {
    "copilot_usage": {
      "token_details": [
        {
          "batch_size": 1000000,
          "cost_per_batch": 300000000000,
          "token_count": 569,
          "token_type": "input"
        },
        {
          "batch_size": 1000000,
          "cost_per_batch": 30000000000,
          "token_count": 0,
          "token_type": "cache_read"
        },
        {
          "batch_size": 1000000,
          "cost_per_batch": 375000000000,
          "token_count": 0,
          "token_type": "cache_write"
        },
        {
          "batch_size": 1000000,
          "cost_per_batch": 1500000000000,
          "token_count": 54,
          "token_type": "output"
        }
      ],
      "total_nano_aiu": 251700000
    },
    "delta": {
      "stop_details": null,
      "stop_reason": "tool_use",
      "stop_sequence": null
    },
    "type": "message_delta",
    "usage": {
      "cache_creation_input_tokens": 0,
      "cache_read_input_tokens": 0,
      "input_tokens": 569,
      "output_tokens": 54
    }
  },
  {
    "amazon-bedrock-invocationMetrics": {
      "firstByteLatency": 850,
      "inputTokenCount": 569,
      "invocationLatency": 1153,
      "outputTokenCount": 54
    },
    "type": "message_stop"
  }
]
```

---

# 缓存/上下文

## 39. extended_cache_ttl

**1 小时缓存**（期望：支持）

`cache_control.ttl: "1h"`（`extended-cache-ttl-2025-04-11` beta）：缓存保留 1 小时（默认 5 分钟），写入价 2x，适合低频但重要的上下文。

**请求** `POST /v1/messages`（anthropic-beta: extended-cache-ttl-2025-04-11）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "system": [
    {
      "type": "text",
      "text": "You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assistant. You are a helpful caching assi...(truncated)",
      "cache_control": {
        "type": "ephemeral",
        "ttl": "1h"
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Say hi."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "Hi! How can I help you today?",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01FhZtvypSbPBVzmfVynJcza",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 1602,
    "input_tokens": 9,
    "output_tokens": 12
  }
}
```

---

# 思考/推理

## 40. effort_xhigh

**effort=xhigh**（期望：拒绝/缺失）

`output_config.effort: "xhigh"`——Opus 4.7/4.8 专属档位。本次测试模型是 Sonnet 4.6 → 400 拒绝并列出其支持的档位（模型条件断言）。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "thinking": {
    "type": "adaptive"
  },
  "output_config": {
    "effort": "xhigh"
  },
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "error": {
    "message": "output_config.effort \"xhigh\" is not supported by model claude-sonnet-4.6; supported values: [low medium high max]",
    "code": "invalid_reasoning_effort"
  }
}
```

---

# RAG/输出

## 41. output_300k

**300k 输出上限**（期望：拒绝/缺失）

`output-300k-2026-03-24` beta + `max_tokens: 200000`。Copilot 上游硬性限制 128k 输出 → 400 报出模型上限（reject 断言验证上限执行）。

**请求** `POST /v1/messages`（anthropic-beta: output-300k-2026-03-24）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 200000,
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "message": "The maximum tokens you requested exceeds the model limit of 128000"
}
```

---

# 思考/推理

## 42. effort_max

**effort=max**（期望：支持）

`output_config.effort: "max"`——effort 五档（low/medium/high/xhigh/max）之顶。Claude 4.x effort 模型普遍接受（Sonnet 4.6 接受 max 却拒绝 xhigh）。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "thinking": {
    "type": "adaptive"
  },
  "output_config": {
    "effort": "max"
  },
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_011oTwvepMYAtquoJZ9Ay8K9",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5,
    "output_tokens_details": {
      "thinking_tokens": 0
    }
  }
}
```

## 43. thinking_budget

**手动思考预算**（期望：支持）

`thinking: {type:"enabled", budget_tokens:N}` 手动指定思考预算。Sonnet 4.6/Haiku 4.5/Opus ≤4.6 支持；Opus 4.7/4.8 已移除手动预算（仅自适应）→ 模型条件断言。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 2048,
  "thinking": {
    "type": "enabled",
    "budget_tokens": 1024
  },
  "messages": [
    {
      "role": "user",
      "content": "Think briefly, then answer: what is 17 * 23?"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "signature": "EswBCmcIDxABGAIqQGO/A+F/j0tzmWTO/uWXC1SOSU9svZiAehnCxpMWzbOyDGJC+cw8F4Jh7+s3fb6aW0QQQiHmy75OIFn51NC5pOYyEWNsYXVkZS1zb25uZXQtNC02OABCCHRoaW5raW5nEgw5nyNcQgnU9yJzJGoaDARNrVC1sywVkumPxCIwrZX87xM72BK/2kFbczyPiwlXWasKuYmRkfmzLBbTQsru4jevXOdRdcPTQezHSnrRKhMnghPi6g+WeoFJ+fM2CF1zr16yGAE=",
      "thinking": "391",
      "type": "thinking"
    },
    {
      "text": "**17 × 23 = 391**\n\nQuick breakdown: 17 × 20 = 340, plus 17 × 3 = 51 → 340 + 51 = **391**",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01MGx3xFZ4E5ysH7A3KWbENd",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 51,
    "output_tokens": 62,
    "output_tokens_details": {
      "thinking_tokens": 7
    }
  }
}
```

---

# 平台/端点

## 44. mid_conv_system

**对话中途 system 消息**（期望：拒绝/缺失）

在 `messages` 数组中插入 `role:"system"` 消息动态改指令——Opus 4.8 专属。Sonnet 4.6 拒绝："Unexpected role 'system'"。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": "My name is Ada."
    },
    {
      "role": "assistant",
      "content": "Nice to meet you, Ada."
    },
    {
      "role": "user",
      "content": "What is my name? Reply in one short sentence."
    },
    {
      "role": "system",
      "content": "Always end your reply with the word DONE."
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "message": "messages: Unexpected role \"system\". The Messages API accepts a top-level `system` parameter, not \"system\" as an input message role."
}
```

## 45. fast_mode

**快速模式**（期望：支持）

`speed: "fast"`（`fast-mode-2026-02-01` beta）请求 2.5x 生成加速。Copilot 上游容忍该字段返回 200，但无真实加速。

**请求** `POST /v1/messages`（anthropic-beta: fast-mode-2026-02-01）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "speed": "fast",
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01UGxe2NxMCLNYUj5N1BeJ6k",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 13,
    "output_tokens": 5
  }
}
```

---

# 缓存/上下文

## 46. prompt_cache_1024

**1024 token 缓存下限**（期望：支持）

构造 (1024, 2048) token 区间的可缓存前缀，验证新模型（Opus 4.8/Sonnet 4.6/Haiku 4.5）把可缓存最小前缀从 2048 降到 1024。响应 `cr: 1202` 表示 1202 token 的前缀被命中。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "system": [
    {
      "type": "text",
      "text": "Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. Cache me if you can. C...(truncated)",
      "cache_control": {
        "type": "ephemeral"
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Say hi."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "Hi!",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01PshEV1QmNZewN1jFsPQqLp",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 1202,
    "input_tokens": 9,
    "output_tokens": 5
  }
}
```

---

# 平台/端点

## 47. refusal_stop_details

**拒答详情**（期望：支持）

探测 Opus 4.8 的 `stop_reason: "refusal"` + `stop_details` 分类元数据。模型可能正常回答（不硬性失败），仅记录形态。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": "Give me detailed step-by-step instructions to break into my neighbour's home wifi network without their permission."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "No. This is unauthorized access to a computer network, which is illegal under laws like the Computer Fraud and Abuse Act and equivalent laws in most countries.\n\nI won't provide these instructions.",
      "type": "text"
    }
  ],
  "id": "msg_bdrk_01Rcizrky1hWMcFZZcnsWQ89",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 30,
    "output_tokens": 43
  }
}
```

---

# 基础能力

## 48. model_discovery

**模型发现**（期望：支持）

`GET /v1/models`：列出上游 43 个模型及能力元数据（上下文窗口、最大输出、支持的端点、视觉/并行工具等 capabilities）。

**请求** `GET /v1/models`（anthropic-beta: 无）
_（GET 请求，无请求体）_

**响应**（代理，HTTP 200 ✅）
共 20 个模型，仅展示前 3 个：
```json
{
  "data": [
    {
      "billing": {
        "restricted_to": [
          "pro_plus",
          "business",
          "enterprise",
          "max"
        ],
        "token_prices": {
          "batch_size": 1000000,
          "default": {
            "cache_price": 100,
            "cache_write_price": 1250,
            "context_max": 200000,
            "input_price": 1000,
            "output_price": 5000
          },
          "long_context": {
            "cache_price": 100,
            "cache_write_price": 1250,
            "context_max": 936000,
            "input_price": 1000,
            "output_price": 5000
          }
        }
      },
      "capabilities": {
        "family": "claude-fable-5",
        "limits": {
          "max_context_window_tokens": 1000000,
          "max_non_streaming_output_tokens": 16000,
          "max_output_tokens": 64000,
          "max_prompt_tokens": 936000,
          "vision": {
            "max_prompt_image_size": 3145728,
            "max_prompt_images": 1,
            "supported_media_types": [
              "...",
              "...",
              "...",
              "...",
              "..."
            ]
          }
        },
        "object": "model_capabilities",
        "supports": {
          "adaptive_thinking": true,
          "max_thinking_budget": 32000,
          "min_thinking_budget": 1024,
          "parallel_tool_calls": true,
          "reasoning_effort": [
            "low",
            "medium",
            "high",
            "xhigh",
            "max"
          ],
          "streaming": true,
          "structured_outputs": true,
          "tool_calls": true,
          "vision": true
        },
        "tokenizer": "o200k_base",
        "type": "chat"
      },
      "id": "claude-fable-5",
      "is_chat_default": false,
      "is_chat_fallback": false,
      "model_picker_category": "powerful",
      "model_picker_enabled": true,
      "model_picker_price_category": "very_high",
      "name": "Claude Fable 5",
      "object": "model",
      "policy": {
        "state": "enabled",
        "terms": "Enable access to the latest Claude Fable 5 model from Anthropic. [Learn more about how GitHub Copilot serves Claude Fable 5](https://gh.io/copilot-claude-opus)."
      },
      "preview": false,
      "supported_endpoints": [
        "/v1/messages",
        "/chat/completions"
      ],
      "vendor": "Anthropic",
      "version": "claude-fable-5",
      "warning_messages": [
        {
          "code": "client_version_deprecated",
          "message": "Your billing plan has changed to usage-based billing and model multipliers no longer apply. Please update your client to the latest version to see the new billing information."
        }
      ],
      "warning_text": {
        "data_retention": "When Claude Fable 5 is used, Anthropic retains data, including prompts and outputs, to operate safety classifiers that detect harmful use. You can read more about Anthropic's data handling practices for this model under [Anthropic's Data retention practices for Mythos-class models](https://support.claude.com/en/articles/15425996-data-retention-practices-for-mythos-class-models)."
      }
    },
    {
      "billing": {
        "restricted_to": [
          "pro",
          "pro_plus",
          "individual_trial",
          "business",
          "enterprise",
          "max"
        ],
        "token_prices": {
          "batch_size": 1000000,
          "default": {
            "cache_price": 50,
            "cache_write_price": 625,
            "context_max": 200000,
            "input_price": 500,
            "output_price": 2500
          },
          "long_context": {
            "cache_price": 50,
            "cache_write_price": 625,
            "context_max": 936000,
            "input_price": 500,
            "output_price": 2500
          }
        }
      },
      "capabilities": {
        "family": "claude-opus-4.6",
        "limits": {
          "max_context_window_tokens": 1000000,
          "max_non_streaming_output_tokens": 16000,
          "max_output_tokens": 64000,
          "max_prompt_tokens": 936000,
          "vision": {
            "max_prompt_image_size": 3145728,
            "max_prompt_images": 1,
            "supported_media_types": [
              "...",
              "...",
              "...",
              "...",
              "..."
            ]
          }
        },
        "object": "model_capabilities",
        "supports": {
          "adaptive_thinking": true,
          "max_thinking_budget": 32000,
          "min_thinking_budget": 1024,
          "parallel_tool_calls": true,
          "reasoning_effort": [
            "low",
            "medium",
            "high",
            "max"
          ],
          "streaming": true,
          "structured_outputs": true,
          "tool_calls": true,
          "vision": true
        },
        "tokenizer": "o200k_base",
        "type": "chat"
      },
      "id": "claude-opus-4.6",
      "is_chat_default": false,
      "is_chat_fallback": false,
      "model_picker_category": "powerful",
      "model_picker_enabled": true,
      "model_picker_price_category": "high",
      "name": "Claude Opus 4.6",
      "object": "model",
      "policy": {
        "state": "enabled",
        "terms": "Enable access to the latest Claude Opus 4.6 model from Anthropic. [Learn more about how GitHub Copilot serves Claude Opus 4.6](https://gh.io/copilot-claude-opus)."
      },
      "preview": false,
      "supported_endpoints": [
        "/v1/messages",
        "/chat/completions"
      ],
      "vendor": "Anthropic",
      "version": "claude-opus-4.6",
      "warning_messages": [
        {
          "code": "client_version_deprecated",
          "message": "Your billing plan has changed to usage-based billing and model multipliers no longer apply. Please update your client to the latest version to see the new billing information."
        }
      ]
    },
    {
      "billing": {
        "restricted_to": [
          "pro_plus",
          "business",
          "enterprise",
          "max"
        ],
        "token_prices": {
          "batch_size": 1000000,
          "default": {
            "cache_price": 50,
            "cache_write_price": 625,
            "context_max": 200000,
            "input_price": 500,
            "output_price": 2500
          },
          "long_context": {
            "cache_price": 50,
            "cache_write_price": 625,
            "context_max": 936000,
            "input_price": 500,
            "output_price": 2500
          }
        }
      },
      "capabilities": {
        "family": "claude-opus-4.7",
        "limits": {
          "max_context_window_tokens": 1000000,
          "max_non_streaming_output_tokens": 16000,
          "max_output_tokens": 64000,
          "max_prompt_tokens": 936000,
          "vision": {
            "max_prompt_image_size": 3145728,
            "max_prompt_images": 1,
            "supported_media_types": [
              "...",
              "...",
              "...",
              "...",
              "..."
            ]
          }
        },
        "object": "model_capabilities",
        "supports": {
          "adaptive_thinking": true,
          "max_thinking_budget": 32000,
          "min_thinking_budget": 1024,
          "parallel_tool_calls": true,
          "reasoning_effort": [
            "low",
            "medium",
            "high",
            "xhigh",
            "max"
          ],
          "streaming": true,
          "structured_outputs": true,
          "tool_calls": true,
          "vision": true
        },
        "tokenizer": "o200k_base",
        "type": "chat"
      },
      "id": "claude-opus-4.7",
      "is_chat_default": false,
      "is_chat_fallback": false,
      "model_picker_category": "powerful",
      "model_picker_enabled": true,
      "model_picker_price_category": "high",
      "name": "Claude Opus 4.7",
      "object": "model",
      "policy": {
        "state": "enabled",
        "terms": "Enable access to the latest Claude Opus 4.7 model from Anthropic. [Learn more about how GitHub Copilot serves Claude Opus 4.7](https://gh.io/copilot-claude-opus)."
      },
      "preview": false,
      "supported_endpoints": [
        "/v1/messages",
        "/chat/completions"
      ],
      "vendor": "Anthropic",
      "version": "claude-opus-4.7",
      "warning_messages": [
        {
          "code": "client_version_deprecated",
          "message": "Your billing plan has changed to usage-based billing and model multipliers no longer apply. Please update your client to the latest version to see the new billing information."
        }
      ]
    }
  ],
  "object": "list"
}
```

---

# 缓存/上下文

## 49. auto_prompt_cache

**自动提示缓存**（期望：拒绝/缺失）

顶层单个 `cache_control` 让系统自动把缓存点跟随到最后一个可缓存块（Anthropic 一方功能）。Copilot 上游拒绝顶层字段（"Extra inputs are not permitted"）→ reject 探针；块级断点（prompt_cache 用例）仍是缓存的正确方式。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "cache_control": {
    "type": "ephemeral"
  },
  "system": "You are a helpful capability-probe assistant. You are a helpful capability-probe assistant. You are a helpful capability-probe assistant. You are a helpful capability-probe assistant. You are a helpful capability-probe assistant. You are a helpful capability-probe assistant. You are a helpful capability-probe assistant. You are a helpful capability-probe assistant. You are a helpful capability-pro...(truncated)",
  "messages": [
    {
      "role": "user",
      "content": "Say hi."
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "message": "cache_control: Extra inputs are not permitted"
}
```

---

# 工具调用

## 50. strict_tool_use

**严格工具调用**（期望：支持）

工具定义带 `strict: true`（schema 需 `additionalProperties: false`）：约束解码保证工具入参 100% 符合 schema，杜绝参数解析失败。结构化输出的另一半。实测上游支持。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ],
        "additionalProperties": false
      },
      "strict": true
    }
  ],
  "tool_choice": {
    "type": "tool",
    "name": "get_weather"
  },
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in Paris? Use the tool."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "id": "toolu_bdrk_01UDoHiv4QFGGa3hna2Gm4WL",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01W9r6nUNqrxwuj6mneCDpe6",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 673,
    "output_tokens": 54
  }
}
```

---

# 平台/端点

## 51. inference_geo

**数据驻留**（期望：拒绝/缺失）

`inference_geo: "us"` 指定推理只在美国区运行（合规），响应 usage 回报实际地域。Anthropic 一方参数，Copilot 上游拒绝 → reject 探针。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "inference_geo": "us",
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "message": "inference_geo: Extra inputs are not permitted"
}
```

---

# 工具基础设施

## 52. mcp_connector

**MCP 连接器**（期望：拒绝/缺失）

`mcp_servers` 数组 + `mcp_toolset` 工具类型（`mcp-client-2025-11-20` beta）：Messages API 直连远程 MCP 服务器，免自建 MCP 客户端。上游拒绝 → reject 探针。

**请求** `POST /v1/messages`（anthropic-beta: mcp-client-2025-11-20）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "mcp_servers": [
    {
      "type": "url",
      "url": "https://example-server.modelcontextprotocol.io/sse",
      "name": "example-mcp"
    }
  ],
  "tools": [
    {
      "type": "mcp_toolset",
      "mcp_server_name": "example-mcp"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "What tools do you have available?"
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "message": "tools.0: Input tag 'mcp_toolset' found using 'type' does not match any of the expected tags: 'bash_20250124', 'custom', 'memory_20250818', 'text_editor_20250124', 'text_editor_20250429', 'text_editor_20250728', 'tool_search_tool_bm25', 'tool_search_tool_bm25_20251119', 'tool_search_tool_regex', 'tool_search_tool_regex_20251119'"
}
```

## 53. tool_search

**工具搜索**（期望：支持）

`tool_search_tool_regex_20251119` 服务端工具 + 其余工具标 `defer_loading: true`：不预载全部工具定义，模型按需正则搜索目录并动态加载（省 ~85% 上下文）。实测上游支持——响应含 `server_tool_use`（搜索）→ `tool_search_tool_result`（找到工具）→ `tool_use`（调用）完整链路。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "tool_search_tool_regex_20251119",
      "name": "tool_search_tool_regex"
    },
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      },
      "defer_loading": true
    },
    {
      "name": "get_time",
      "description": "Get current time for a timezone",
      "input_schema": {
        "type": "object",
        "properties": {
          "tz": {
            "type": "string"
          }
        },
        "required": [
          "tz"
        ]
      },
      "defer_loading": true
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in Paris? Find and use a suitable tool."
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "Let me search for a weather tool that can help with this!",
      "type": "text"
    },
    {
      "id": "srvtoolu_bdrk_01MhBqNjx2UfSNDxn25ZLpeE",
      "input": {
        "pattern": "weather"
      },
      "name": "tool_search_tool_regex",
      "type": "server_tool_use"
    },
    {
      "content": {
        "tool_references": [
          {
            "tool_name": "get_weather",
            "type": "tool_reference"
          }
        ],
        "type": "tool_search_tool_search_result"
      },
      "tool_use_id": "srvtoolu_bdrk_01MhBqNjx2UfSNDxn25ZLpeE",
      "type": "tool_search_tool_result"
    },
    {
      "text": "Found a suitable tool! Let me fetch the weather for Paris now.",
      "type": "text"
    },
    {
      "id": "toolu_bdrk_01EuUH5KDsKbwM2Q5LV8TBRK",
      "input": {
        "city": "Paris"
      },
      "name": "get_weather",
      "type": "tool_use"
    }
  ],
  "id": "msg_bdrk_01Nfm87xeHd9LVhZATuc8Uhk",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 1608,
    "output_tokens": 139,
    "server_tool_use": {
      "web_search_requests": 0
    }
  }
}
```

## 54. programmatic_tool_calling

**编程式工具调用**（期望：拒绝/缺失）

`code_execution_20260120` + 工具标 `allowed_callers`：模型在代码沙箱里写代码批量调用工具，N 次查询 1 次往返。上游无该工具版本 → reject 探针。

**请求** `POST /v1/messages`（anthropic-beta: 无）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "code_execution_20260120",
      "name": "code_execution"
    },
    {
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string"
          }
        },
        "required": [
          "city"
        ]
      },
      "allowed_callers": [
        "code_execution_20260120"
      ]
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Check the weather in Paris and London programmatically."
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "message": "tools.0: Input tag 'code_execution_20260120' found using 'type' does not match any of the expected tags: 'bash_20250124', 'custom', 'memory_20250818', 'text_editor_20250124', 'text_editor_20250429', 'text_editor_20250728', 'tool_search_tool_bm25', 'tool_search_tool_bm25_20251119', 'tool_search_tool_regex', 'tool_search_tool_regex_20251119'"
}
```

## 55. agent_skills

**Agent 技能**（期望：拒绝/缺失）

`container.skills` 加载预置技能包（pptx/xlsx/docx/pdf 生成）到代码执行容器（`skills-2025-10-02` beta）。上游拒绝 → reject 探针。

**请求** `POST /v1/messages`（anthropic-beta: skills-2025-10-02, code-execution-2025-08-25）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "container": {
    "skills": [
      {
        "type": "anthropic",
        "skill_id": "pptx",
        "version": "latest"
      }
    ]
  },
  "tools": [
    {
      "type": "code_execution_20250825",
      "name": "code_execution"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Create a one-slide presentation that says hello."
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "message": "tools.0: Input tag 'code_execution_20250825' found using 'type' does not match any of the expected tags: 'bash_20250124', 'custom', 'memory_20250818', 'text_editor_20250124', 'text_editor_20250429', 'text_editor_20250728', 'tool_search_tool_bm25', 'tool_search_tool_bm25_20251119', 'tool_search_tool_regex', 'tool_search_tool_regex_20251119'"
}
```

---

# 服务端工具

## 56. advisor_tool

**顾问工具**（期望：拒绝/缺失）

`advisor_20260301` 工具（`advisor-tool-2026-03-01` beta）：快模型执行 + 强模型（如 Opus 4.8）中途给战略指导的混合推理模式。上游拒绝 → reject 探针。

**请求** `POST /v1/messages`（anthropic-beta: advisor-tool-2026-03-01）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "tools": [
    {
      "type": "advisor_20260301",
      "name": "advisor",
      "model": "claude-opus-4-8"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Build a concurrent worker pool in Go with graceful shutdown."
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "message": "tools.0: Input tag 'advisor_20260301' found using 'type' does not match any of the expected tags: 'bash_20250124', 'custom', 'memory_20250818', 'text_editor_20250124', 'text_editor_20250429', 'text_editor_20250728', 'tool_search_tool_bm25', 'tool_search_tool_bm25_20251119', 'tool_search_tool_regex', 'tool_search_tool_regex_20251119'"
}
```

---

# 缓存/上下文

## 57. compaction

**服务端压缩**（期望：支持）

`context_management.edits` 的 `compact_20260112` 策略：接近上下文上限时服务端自动摘要旧对话生成 `compaction` 块，后续请求从摘要继续。上游支持但需 `compact-2026-01-12` beta 头——代理检测到 compact_* 编辑时自动附加（本次修复的缺口）。短对话不触发压缩，`applied_edits` 为空即表示编辑类型被接受。

**请求** `POST /v1/messages`（anthropic-beta: compact-2026-01-12）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "context_management": {
    "edits": [
      {
        "type": "compact_20260112"
      }
    ]
  },
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**响应**（代理，HTTP 200 ✅）
```json
{
  "content": [
    {
      "text": "pong",
      "type": "text"
    }
  ],
  "context_management": {
    "applied_edits": []
  },
  "id": "msg_bdrk_01EJHopZyrrmRMk2L9jeYijq",
  "model": "claude-sonnet-4-6",
  "role": "assistant",
  "stop_details": null,
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "type": "message",
  "usage": {
    "cache_creation": {
      "ephemeral_1h_input_tokens": 0,
      "ephemeral_5m_input_tokens": 0
    },
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "input_tokens": 84,
    "iterations": [
      {
        "cache_creation": {
          "ephemeral_1h_input_tokens": 0,
          "ephemeral_5m_input_tokens": 0
        },
        "cache_creation_input_tokens": 0,
        "cache_read_input_tokens": 0,
        "input_tokens": 84,
        "output_tokens": 5,
        "type": "message"
      }
    ],
    "output_tokens": 5
  }
}
```

---

# 平台/端点

## 58. server_side_fallback

**服务端回退**（期望：拒绝/缺失）

`fallbacks` 数组（`server-side-fallback-2026-06-01` beta）：被安全分类器拒答时 API 自动换备选模型重试同一请求。上游拒绝 → reject 探针。

**请求** `POST /v1/messages`（anthropic-beta: server-side-fallback-2026-06-01）
```json
{
  "model": "claude-sonnet-4.6",
  "max_tokens": 512,
  "fallbacks": [
    {
      "model": "claude-opus-4-8"
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Reply with exactly: pong"
    }
  ]
}
```

**响应**（代理，HTTP 400 ✅）
```json
{
  "message": "fallbacks: Extra inputs are not permitted"
}
```

## 59. batches_endpoint

**批量处理 API**（期望：拒绝/缺失）

`POST /v1/messages/batches`：批量异步处理大量请求，成本减半。独立端点，直连与代理均 404 → 固化缺失行为。

**请求** `POST /v1/messages/batches`（anthropic-beta: 无）
```json
{
  "requests": [
    {
      "custom_id": "cap-probe-1",
      "params": {
        "model": "claude-sonnet-4.6",
        "max_tokens": 16,
        "messages": [
          {
            "role": "user",
            "content": "Reply with exactly: pong"
          }
        ]
      }
    }
  ]
}
```

**响应**（代理，HTTP 404 ✅）
```json
{
  "_unparsed": "404 page not found\n"
}
```

## 60. files_endpoint

**Files API**（期望：拒绝/缺失）

`GET /v1/files`（`files-api-2025-04-14` beta）：上传一次文件反复引用。独立端点，双端 404 → 固化缺失行为。

**请求** `GET /v1/files`（anthropic-beta: files-api-2025-04-14）
_（GET 请求，无请求体）_

**响应**（代理，HTTP 404 ✅）
```json
{
  "_unparsed": "404 page not found\n"
}
```
