# 自动化 Device Flow 登录

默认情况下，在 admin UI 里认证一个账户只是「半自动」：代理拿到 `user_code` 和验证链接后，
**需要人工**打开 GitHub、粘贴 code、点击 **Authorize**。

账户一多，这一步就很烦。这个可选功能新增一个专用的 **login agent** 容器，替你完成浏览器操作。

> 本文是操作手册（部署、配置、排查）。方案设计思路与端到端业务流程见
> [auto-login-design.zh-CN.md](auto-login-design.zh-CN.md)。

## 架构

```
┌──────────────────────────────┐              ┌────────────────────────────────┐
│ copilot2api (Go)             │              │ login-agent (Node + Playwright)│
│                              │  POST /login │                                │
│ admin UI  [Auto authenticate]├─────────────►│ 1. 在 github.com/login 登录     │
│                              │  user_code   │ 2. 打开验证页面                 │
│ StartDeviceFlow()  ──┐       │  username    │ 3. 填入 User Code               │
│                      │ 轮询   │  password    │ 4. 点击 Authorize               │
│ CompleteDeviceFlow() │       │◄─────────────┤ 结果 / 失败截图                 │
│   └─► credentials.json       │              │                                │
└──────────────────────────────┘              └────────────────────────────────┘
        （逻辑未改动）                            compose 内网，不暴露宿主端口
```

关键点：**device flow 的轮询与凭证落盘逻辑完全没有改动。** agent 只是替代「人手点授权」这一步。
代理依然自己轮询 GitHub，依然把 token 写入 `<token_dir>/credentials.json`。

因此自动化是纯增量的。一旦 agent 失败，`user_code` 在有效期内依然可用，admin UI 也会继续显示
code 和链接，你可以随时手动完成。

## 使用 Docker Compose 部署

```bash
cp .env.example .env
# 编辑 .env，把两个 token 都改成足够长的随机值
docker compose up -d --build
```

然后打开 `http://127.0.0.1:7777/admin/?admin_token=<你的 admin token>`。

> 如果宿主机的 7777 端口已被占用（例如已有一个 copilot2api 容器在跑），可以在
> `.env` 中设置 `COPILOT2API_HOST_PORT`，把代理发布到别的宿主端口。

1. 添加账户，同时填写 **GitHub username** 和 **GitHub password**。
   （也可以之后用账户行上的 **Login** 按钮补录凭证。）
2. 点击 **Auto authenticate**。
3. 对话框会显示自动化进度。成功后账户状态变为 `authenticated`，token 落到该账户的凭证目录。

## 配置

**不设置 `COPILOT2API_LOGIN_AGENT_URL` 时功能完全关闭。** 此时 admin UI 会隐藏所有自动化控件，
行为与之前完全一致。

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `COPILOT2API_LOGIN_AGENT_URL` | login agent 的地址。不设置则彻底关闭自动化。 | _(未设置)_ |
| `COPILOT2API_LOGIN_AGENT_TOKEN` | 以 `X-Agent-Token` 发送的共享密钥，需与 agent 的 `LOGIN_AGENT_TOKEN` 一致。 | _(未设置，不鉴权)_ |
| `COPILOT2API_LOGIN_AGENT_TIMEOUT_SECONDS` | 单次登录超时。 | `180` |
| `COPILOT2API_LOGINS_FILE` | GitHub 凭证存储位置。 | `<token-dir>/github_logins.json` |

login agent 容器的变量：

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `LOGIN_AGENT_TOKEN` | 共享密钥。设置后，`X-Agent-Token` 不匹配的请求返回 `401`。 | _(未设置，不鉴权)_ |
| `PORT` / `HOST` | 监听地址。 | `8080` / `0.0.0.0` |
| `LOGIN_AGENT_PROFILE_DIR` | 持久化浏览器 profile 的目录。 | `/data/profiles` |
| `LOGIN_AGENT_HEADLESS` | 设为 `false` 可看到浏览器界面（仅用于调试）。 | `true` |

## 安全

这个功能会存储 GitHub 密码，启用前请务必读完本节。

- **密码以明文存储**在 `github_logins.json` 中（文件权限 `0600`）。代理必须能读取它才能转发给 agent。
  请把 token 目录当作密钥库对待，不要放在你不放心存密码的共享卷或备份里。
- **绝不要暴露 login agent。** compose 文件刻意没有为它声明 `ports:`。
  任何能访问 agent 的人都能用你的凭证驱动浏览器。
- **请设置 `COPILOT2API_ADMIN_TOKEN`。** 否则任何能访问 admin UI 的人都能用已存储的凭证触发登录。
  compose 文件强制要求这个变量。
- **请设置 `LOGIN_AGENT_TOKEN`**，确保只有代理能调用 agent。
- 密码不会出现在任何 API 响应里（`GET .../login` 只返回 `username` 和 `has_password`），
  两侧的日志也都不会打印密码。
- 用自己的凭证驱动浏览器登录属于你自己的行为，请自行确认符合你账户适用的 GitHub 条款。

## 前提与限制

- **不支持两步验证（2FA）。** 如果账户开启了 2FA，自动化会返回 `two_factor_required`，只能手动授权。
- **新设备邮箱验证。** GitHub 在检测到陌生设备登录时可能发送邮箱验证码。
  agent 会按账户持久化浏览器 profile（存放在 `login-agent-profiles` 卷中），
  后续登录复用可信设备 cookie；但**首次**从新主机登录仍可能被拦截。
  这一次手动授权即可，之后应该就能自动完成。
- 登录请求**串行处理**以控制内存：Chromium 开销很大，多账户并发会同时拉起多个浏览器。

## 排查

自动化失败时，admin UI 会显示失败分类和浏览器停住那一页的截图。截图通常足以判断发生了什么。

| 分类 | 含义 | 处理方式 |
|------|------|----------|
| `invalid_credentials` | GitHub 拒绝了用户名或密码。 | 用 **Login** 按钮修正。 |
| `two_factor_required` | 账户开启了 2FA。 | 只能手动授权。 |
| `device_verification` | GitHub 要求邮箱设备验证码。 | 手动授权一次，之后保存的 profile 应能避免再次触发。 |
| `captcha` | 出现了人机校验。 | 手动授权，稍后再试。 |
| `code_rejected` | GitHub 不接受该 user code。 | 通常是 code 过期，重新发起认证。 |
| `timeout` | agent 超时。 | 调大 `COPILOT2API_LOGIN_AGENT_TIMEOUT_SECONDS`，并确认 agent 能访问外网。 |
| `agent_unreachable` | 代理连不上 agent。 | 检查 `COPILOT2API_LOGIN_AGENT_URL` 和 agent 容器健康状态。 |

常用命令：

```bash
docker compose logs -f login-agent      # 结构化日志，不含密码
docker compose exec login-agent \
  node -e "fetch('http://127.0.0.1:8080/health').then(r=>r.text()).then(console.log)"
```

`unknown` 类失败最值得配合截图排查。已知的一种形态：**"Device code input was not
found on the verification page."** 表示 GitHub 返回了 agent 不认识的页面，而不是填码表单。
当会话已存在时，`/login/device` 会重定向到 `/login/device/select_account`
（「Device Activation — Signed in as …」外加一个 **Continue** 按钮），agent 会自动点过该页。
如果截图显示的是**别的**页面，多半是页面结构变了。

GitHub 改版时，所有选择器集中在
[`login-agent/src/selectors.js`](../login-agent/src/selectors.js) 一个文件里，且都带有降级候选。

## Agent HTTP 接口

agent 是个很小的服务，调试时可以直接调用。

`GET /health` → `{"status":"ok","queued":0}`

`POST /login`

```json
{
  "account_id": "alice",
  "username": "alice@example.com",
  "password": "...",
  "user_code": "ABCD-1234",
  "verification_uri": "https://github.com/login/device"
}
```

成功返回：`{"ok": true, "step": "done"}`。
失败返回：`{"ok": false, "step": "login", "error_class": "invalid_credentials",
"error": "...", "screenshot": "<base64 PNG>"}`。

## Admin API 新增接口

| 接口 | 用途 |
|------|------|
| `GET /admin/api/config` | 返回 `auto_login_enabled` / `login_store_enabled`，供 UI 决定是否显示控件。 |
| `GET /admin/api/accounts/{id}/login` | 返回 `{username, has_password}`，绝不返回密码。 |
| `PUT /admin/api/accounts/{id}/login` | 设置 `{username, password}`。 |
| `DELETE /admin/api/accounts/{id}/login` | 清除已存凭证。 |
| `POST /admin/api/accounts/{id}/auth/start` | 支持 `{"auto": true}`，同时启动 agent。 |
| `GET /admin/api/accounts/{id}/auth/status` | 新增 `auto` 对象：`{state, step, error_class, error, has_screenshot}`。 |
| `GET /admin/api/accounts/{id}/auth/screenshot` | 以 `image/png` 返回最近一次失败截图。 |

创建账户时支持可选的 `github_username` / `github_password`，可以一次请求同时建账户和凭证。
删除账户时会一并删除其存储的凭证。
