# copilot2api

[English](README.md) | [简体中文](README.zh-CN.md)

> 本项目 fork 自 [whtsky/copilot2api](https://github.com/whtsky/copilot2api)，在其基础上新增了多账号支持、Web 管理界面以及 Token 用量统计。原项目的所有贡献归于上游作者。

一个轻量级的 Go 代理，将 GitHub Copilot 暴露为兼容 OpenAI、Anthropic、Gemini 以及 AmpCode 的 API 端点。

## 功能特性

- **兼容 OpenAI API**：`/v1/chat/completions`、`/v1/models`、`/v1/embeddings`、`/v1/responses`
- **支持 Embeddings**：原生兼容 OpenAI 的 `/v1/embeddings` 端点
- **兼容 Anthropic API**：`/v1/messages`、`/v1/messages/count_tokens`
- **兼容 Gemini API**：`/v1beta/models`、`/v1beta/models/{model}:generateContent`、`/v1beta/models/{model}:streamGenerateContent`、`/v1beta/models/{model}:countTokens`
- **兼容 AmpCode**：`/amp/v1/*` 路由用于对话，`/api/provider/*` 用于特定 provider 的调用，管理类请求反向代理到 `ampcode.com`
- **流式支持**：OpenAI 与 Anthropic 格式均支持完整的 SSE 流式输出
- **Anthropic 智能路由**：模型原生支持时使用 `/v1/messages`，否则通过 `/responses` 或 `/chat/completions` 转发。原生路径会透传高级字段，如 `context_management`（自动补加 `context-management-2025-06-27` beta 头）与 `search_result` 内容块，并转发客户端的 `computer-use-*` beta 头以让 Computer Use 工具在上游生效。
- **多账号支持**：将 API Key 与 GitHub 账号一对一映射，各账号使用独立的凭据存储（详见 [多 GitHub 账号](#多-github-账号)）
- **Web 管理界面**：在 `/admin/` 管理账号并查看 Token 使用统计（多账号模式）
- **自动认证**：GitHub Device Flow OAuth，自动刷新 Token
- **用量监控**：内置 `/usage` 端点用于配额追踪
- **模型缓存**：`/v1/models` 与 Anthropic 模型能力查询结果缓存 5 分钟

## 快速开始

### Docker

从源码构建镜像（本 fork 包含上游镜像没有的多账号与用量统计功能）：

```bash
docker build -t copilot2api-multiusers .
```

运行：

```bash
docker run -it --rm \
  -p 7777:7777 \
  -p 7778:7778 \
  -e COPILOT2API_ADMIN_USERNAME=admin \
  -e COPILOT2API_ADMIN_PASSWORD='change-me' \
  -v ~/.config/copilot2api:/root/.config/copilot2api \
  copilot2api-multiusers
```

挂载卷可在容器重启后保留你的 GitHub 凭据。示例会发布公开 API 端口（`7777`）和管理端口（`7778`）。

> 提示：公开 API 监听 `0.0.0.0:7777`。管理界面由独立监听器服务在 `0.0.0.0:7778`。

<details>
<summary>Docker Compose</summary>

```yaml
services:
  copilot2api:
    build: .
    ports:
      - "7777:7777"
      - "7778:7778"
    environment:
      COPILOT2API_ADMIN_USERNAME: admin
      COPILOT2API_ADMIN_PASSWORD: change-me
    volumes:
      - ${HOME}/.config/copilot2api:/root/.config/copilot2api
```

构建并启动：

```bash
docker compose up --build
```

</details>

公开 API 默认监听 `0.0.0.0:7777`。设置 `COPILOT2API_ADMIN_USERNAME` 与 `COPILOT2API_ADMIN_PASSWORD` 后，管理界面会监听 **`0.0.0.0:7778`**，可打开 `http://<server-ip>:7778/admin/` 通过浏览器驱动的 Device Flow 新增并认证 GitHub 账号（参见 [多 GitHub 账号](#多-github-账号)）。

## 安全说明

⚠️ **本代理仅为本地开发设计。**

- 默认即校验 API Key：每个请求都必须携带映射到已配置账号的 Key，否则返回 `401 Unauthorized`（参见 [多 GitHub 账号](#多-github-账号)）。
- 请勿公网暴露 —— 否则会成为一个开放代理，消耗你的 Copilot 配额。
- 请将管理监听器与公开 API 分离。面向公网的网关只暴露 `7777`，`7778` 建议通过本机回环、SSH 隧道、VPN 或其他受限管理通道访问。
- 各账号的凭据存储于 `~/.config/copilot2api/<token_dir>/credentials.json`。

## 多 GitHub 账号

代理始终以多账号模式运行，通过 Token 目录下的 `accounts.json` 文件（默认 `~/.config/copilot2api/accounts.json`，或通过 `COPILOT2API_ACCOUNTS_FILE` 指定）将 API Key 与 GitHub 账号一对一映射。**若该文件不存在，首次启动时会自动创建为空配置**（`{"accounts": []}`），配置好管理账号密码后即可通过管理界面填充它（参见 [管理界面](#管理界面)）。

你也可以手动编辑 `accounts.json`：

```json
{
  "accounts": [
    { "id": "alice", "api_key": "sk-alice-...", "token_dir": "alice" },
    { "id": "bob",   "api_key": "sk-bob-...",   "token_dir": "bob" }
  ]
}
```

- `id` —— 唯一的账号标识（用于日志；默认作为 Token 子目录名）。
- `api_key` —— 客户端需提供的 Key，各账号之间必须唯一。
- `token_dir` —— 该账号 `credentials.json` 的存放位置。相对路径基于基础 Token 目录解析；默认为 `id`。

启动时，代理会对每个尚无存储 Token 的账号**逐个**执行一次 GitHub Device Flow。每个账号拥有独立的凭据存储与模型缓存，因此 Token 刷新与基于能力的路由相互独立。

客户端通过发送对应的 `api_key` 选择账号：

- OpenAI：`Authorization: Bearer <api_key>`
- Anthropic：`x-api-key: <api_key>`
- Gemini：`x-goog-api-key: <api_key>` 或 `?key=<api_key>`

请求**必须**携带有效的 Key，否则返回 `401 Unauthorized`。在尚未配置任何账号之前（例如通过管理界面添加），所有请求都会被拒绝并返回 `401`。

### 管理界面

代理会在独立管理监听器上提供一个带密码保护的 Web 界面，默认监听 **`0.0.0.0:7778`**，无需手动编辑 `accounts.json` 即可维护映射：

- 列出账号及其认证状态。
- 新增账号（id + API Key + 可选 token 目录），并通过浏览器驱动的 GitHub Device Flow 完成认证（显示验证码与验证链接，并轮询直到完成）。
- 轮换账号的 API Key，或删除账号。
- **Stats 标签页**：查看按账号、按模型的 Token 用量 —— 输入、输出、缓存命中（prompt-cache）、缓存写入以及请求总数 —— 覆盖所有 OpenAI、Anthropic、Gemini 端点。用量数据持久化到 `<token-dir>/stats.json`，重启后仍保留（由 `GET /admin/api/stats` 提供，`DELETE /admin/api/stats/{id}` 可重置单个账号）。

> 注意：OpenAI Chat Completions 流式响应只有在客户端发送 `stream_options.include_usage` 时才会统计 Token 数量；但请求本身始终会被计数。

所有更改都会写回 `accounts.json`，并立即应用到正在运行的代理 —— 无需重启。

⚠️ 管理界面可以读取 API Key、显示已保存的 GitHub/Copilot Token，并触发 GitHub 认证。请设置 `COPILOT2API_ADMIN_USERNAME` 与 `COPILOT2API_ADMIN_PASSWORD`；若未设置且 `COPILOT2API_ADMIN_ENABLED` 不是 `false`，管理服务会拒绝启动。`COPILOT2API_ADMIN_TOKEN` 仅保留为面向脚本调用的已废弃请求头兼容方式。

在 Azure VM + Application Gateway 部署中，请只把公网流量转发到 API 监听器（`7777`）。不要把管理监听器（`7778`）加入公网后端规则。建议用 NSG 限制只有 Application Gateway 子网可访问 VM 的 `7777`，管理访问则通过 SSH 隧道、Bastion、VPN，或另行配置强限制的管理 listener。

## 配合 Claude Code 使用

添加到 `~/.claude/settings.json`：

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:7777",
    "ANTHROPIC_API_KEY": "dummy",
    "ANTHROPIC_MODEL": "claude-opus-4.6",
    "ANTHROPIC_SMALL_FAST_MODEL": "claude-haiku-4.5",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  },
  "permissions": {
    "deny": [
      "WebSearch"
    ]
  }
}
```

### 1M 上下文窗口

copilot2api 支持 Claude 的 1M 上下文模型。当 Claude Code 发送 `anthropic-beta: context-1m-...` 请求头时，代理会自动在模型 ID 后追加 `-1m`（例如 `claude-opus-4.6` → `claude-opus-4.6-1m`），让 Copilot 路由到 1M 变体。

使用时，在 Claude Code 中通过 `/model` 命令选择 1M 模型变体（如 `Opus (1M)`）。否则 Claude Code 默认使用标准的 200K 上下文窗口。

## 配合 Codex 使用

添加到 `~/.codex/config.toml`：

```toml
model = "gpt-5.3-codex"
model_provider = "copilot2api"
model_reasoning_effort = "high"
web_search = "disabled"

[model_providers.copilot2api]
name = "copilot2api"
base_url = "http://127.0.0.1:7777/v1"
wire_api = "responses"
api_key = "dummy"
```

## 配合 Gemini CLI 使用

添加到 `~/.gemini/.env`：

```env
GOOGLE_GEMINI_BASE_URL=http://127.0.0.1:7777
GEMINI_API_KEY=dummy
GEMINI_MODEL=claude-opus-4.6-1m
```

## 配合 AmpCode 使用

设置 `AMP_URL` 环境变量指向 copilot2api：

```bash
AMP_URL=http://127.0.0.1:7777/amp amp
```

或添加到 `~/.config/amp/settings.json`：

```json
{
  "amp.url": "http://127.0.0.1:7777/amp"
}
```

对话补全、工具调用和图片输入都会经由 Copilot API。登录与管理类路由（threads、telemetry）会被代理到 `ampcode.com` —— 需要一个免费的 amp 账号用于认证。

<details>
<summary>使用 curl</summary>

```bash
# OpenAI 对话补全
curl http://localhost:7777/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.3-codex","messages":[{"role":"user","content":"Hello!"}]}'

# Anthropic 消息
curl http://localhost:7777/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -d '{"model":"claude-sonnet-4.6","messages":[{"role":"user","content":"Hello!"}],"max_tokens":100}'

# 列出模型
curl http://localhost:7777/v1/models

# 查看用量/配额
curl http://localhost:7777/usage
```

</details>

<details>
<summary>使用 SDK</summary>

### OpenAI Python SDK

```python
import openai

client = openai.OpenAI(
    api_key="dummy",
    base_url="http://127.0.0.1:7777/v1"
)

response = client.chat.completions.create(
    model="gpt-5.3-codex",
    messages=[{"role": "user", "content": "Hello!"}]
)
```

### Anthropic Python SDK

```python
import anthropic

client = anthropic.Anthropic(
    api_key="dummy",
    base_url="http://127.0.0.1:7777"
)

message = client.messages.create(
    model="claude-sonnet-4.6",
    max_tokens=1024,
    messages=[{"role": "user", "content": "Hello!"}]
)
```

</details>

## API 端点

| 端点 | 方法 | 说明 |
|----------|--------|-------------|
| `/v1/chat/completions` | POST | OpenAI Chat Completions（流式与非流式） |
| `/v1/responses` | POST | OpenAI Responses API |
| `/v1/models` | GET | 列出可用模型（缓存 5 分钟） |
| `/v1/embeddings` | POST | 生成 Embeddings（支持字符串或数组输入） |
| `/v1/messages` | POST | Anthropic Messages API（流式与非流式） |
| `/v1/messages/count_tokens` | POST | Anthropic Token 计数（转发至上游） |
| `/v1beta/models` | GET | 列出 Gemini 兼容模型 |
| `/v1beta/models/{model}:generateContent` | POST | Gemini Generate Content |
| `/v1beta/models/{model}:streamGenerateContent` | POST | Gemini Generate Content 流式 SSE |
| `/v1beta/models/{model}:countTokens` | POST | Gemini Token 计数估算 |
| `/amp/v1/chat/completions` | POST | AmpCode 对话补全（经由 Copilot API） |
| `/amp/v1/models` | GET | AmpCode 模型列表 |
| `/api/provider/*` | POST | AmpCode 特定 provider 路由 |
| `/api/*` | ANY | AmpCode 管理类反向代理到 ampcode.com |
| `/usage` | GET | Copilot 用量与配额信息 |
| `/admin/` | GET | 独立管理监听器上的 Web 管理界面 |
| `/admin/api/stats` | GET | 独立管理监听器上的按账号 / 按模型 Token 用量统计 |
| `/admin/api/stats/{id}` | DELETE | 独立管理监听器上的单账号用量统计重置 |

## 配置

### 命令行参数

```
./copilot2api [options]

  -host string       服务监听地址（默认 "0.0.0.0"）
  -port int          服务端口（默认 7777）
  -admin-host string 管理服务监听地址（默认 "0.0.0.0"）
  -admin-port int    管理服务端口（默认 7778）
  -token-dir string  Token 存储目录（默认 ~/.config/copilot2api）
  -debug             开启调试日志
  -version           显示版本并退出
```

### 环境变量

当未提供命令行参数时，环境变量将作为默认值：

| 变量 | 说明 | 默认值 |
|----------|-------------|---------|
| `COPILOT2API_HOST` | 服务监听地址 | `0.0.0.0` |
| `COPILOT2API_PORT` | 服务端口 | `7777` |
| `COPILOT2API_TOKEN_DIR` | Token 存储目录 | `~/.config/copilot2api` |
| `COPILOT2API_ACCOUNTS_FILE` | 多账号配置文件路径（参见 [多 GitHub 账号](#多-github-账号)） | `<token-dir>/accounts.json` |
| `COPILOT2API_ADMIN_ENABLED` | 是否启动独立管理服务 | `true` |
| `COPILOT2API_ADMIN_HOST` | 管理服务监听地址 | `0.0.0.0` |
| `COPILOT2API_ADMIN_PORT` | 管理服务端口 | `7778` |
| `COPILOT2API_ADMIN_USERNAME` | 管理登录用户名；启用管理服务时必填 | _（未设置）_ |
| `COPILOT2API_ADMIN_PASSWORD` | 管理登录密码；启用管理服务时必填 | _（未设置）_ |
| `COPILOT2API_ADMIN_TOKEN` | 已废弃的兼容 Token，仅用于 `X-Admin-Token` 脚本访问 | _（未设置）_ |
| `COPILOT2API_DEBUG` | 开启调试日志（`true`/`false`、`1`/`0`） | `false` |

命令行参数优先级高于环境变量。

## 工作原理

1. 通过 Device Flow OAuth 与 GitHub 认证
2. 将 GitHub Token 换取 Copilot API Token（自动刷新）
3. 将 OpenAI 格式请求直接转发到 Copilot API
4. 根据模型能力路由 Anthropic Messages 请求（原生 `/v1/messages`、转换为 `/responses` 或转换为 `/chat/completions`）
5. 自动从 Token 检测 API 端点（个人版/商业版/企业版）

## 开发

```bash
go test ./...              # 运行测试
go build -o copilot2api .  # 构建
```

## 致谢

本项目 fork 自 [whtsky/copilot2api](https://github.com/whtsky/copilot2api)。核心代理、协议转换与认证都来自上游仓库；本 fork 在其基础上增加了多账号支持、Web 管理界面以及 Token 用量统计。感谢原作者与各位贡献者。

## 许可证

MIT
