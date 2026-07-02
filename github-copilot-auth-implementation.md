# OpenCode GitHub Copilot 认证实现分析

## 1. 核心结论

OpenCode 的 GitHub Copilot 认证并没有实现“用 GitHub Token 再换取 Copilot Token”的二次交换流程。它的实际逻辑是：

1. 通过 GitHub OAuth Device Flow 获取 GitHub OAuth `access_token`。
2. 将该 token 以 OpenCode OAuth 凭据形式保存。
3. 在访问 GitHub Copilot API 时，直接使用该 token 作为 `Authorization: Bearer <token>`。
4. Copilot API host 为：
   - GitHub.com：`https://api.githubcopilot.com`
   - GitHub Enterprise：`https://copilot-api.<enterprise-domain>`

与之不同，仓库中的 `exchange_github_app_token` / `exchange_github_app_token_with_pat` 是 GitHub Action 使用的 GitHub App installation token 交换逻辑，不属于 Copilot Provider 认证流程。

## 2. 主要涉及文件

| 文件 | 作用 |
|---|---|
| `packages/opencode/src/plugin/github-copilot/copilot.ts` | GitHub Copilot 认证插件主体：OAuth Device Flow、模型发现、请求头注入 |
| `packages/opencode/src/plugin/github-copilot/models.ts` | 调用 Copilot `/models`，解析远端模型列表 |
| `packages/opencode/src/provider/auth.ts` | Provider auth 流程协调：authorize/callback/pending 状态和保存 auth |
| `packages/opencode/src/auth/index.ts` | auth 数据结构和本地持久化 |
| `packages/opencode/src/provider/provider.ts` | 加载 provider、合并 auth loader 返回的 provider options、构造 SDK |
| `packages/core/src/github-copilot/copilot-provider.ts` | 内置 OpenAI-compatible Copilot SDK 包装 |
| `packages/llm/src/providers/github-copilot.ts` | native LLM provider facade，要求显式 `baseURL` 和 bearer auth |

## 3. 认证流程总览

```text
用户选择 Login with GitHub Copilot
        |
        v
CopilotAuthPlugin.authorize()
        |
        | POST https://github.com/login/device/code
        v
返回 verification_uri + user_code + device_code
        |
        v
用户打开 GitHub 页面输入 user_code
        |
        v
OpenCode callback() 轮询
        |
        | POST https://github.com/login/oauth/access_token
        v
获取 GitHub OAuth access_token
        |
        v
ProviderAuth.callback() 保存为 Auth.Oauth
        |
        v
Provider 初始化时 auth.loader() 生成自定义 fetch
        |
        v
模型发现 / 请求 Copilot API 时注入：
Authorization: Bearer <access token>
```

## 4. Copilot OAuth 插件入口

文件：`packages/opencode/src/plugin/github-copilot/copilot.ts`

```ts
const CLIENT_ID = "Ov23li8tweQw6odWQebz"
const API_VERSION = "2026-06-01"
const UTILITY_MODELS = ["gpt-5.4-nano", "gpt-4.1", "gpt-4o", "gpt-4o-mini"]
const OAUTH_POLLING_SAFETY_MARGIN_MS = 3000
```

这里的 `CLIENT_ID` 是用于 GitHub OAuth Device Flow 的 OAuth App client id。`API_VERSION` 则用于 Copilot API 请求头。

## 5. GitHub.com 与 Enterprise URL 处理

```ts
function normalizeDomain(url: string) {
  return url.replace(/^https?:\/\//, "").replace(/\/$/, "")
}

function getUrls(domain: string) {
  return {
    DEVICE_CODE_URL: `https://${domain}/login/device/code`,
    ACCESS_TOKEN_URL: `https://${domain}/login/oauth/access_token`,
  }
}

function base(enterpriseUrl?: string) {
  return enterpriseUrl ? `https://copilot-api.${normalizeDomain(enterpriseUrl)}` : "https://api.githubcopilot.com"
}
```

含义：

- GitHub.com 登录：
  - Device code URL：`https://github.com/login/device/code`
  - Token URL：`https://github.com/login/oauth/access_token`
  - Copilot API：`https://api.githubcopilot.com`
- Enterprise 登录：
  - Device code URL：`https://<enterprise-domain>/login/device/code`
  - Token URL：`https://<enterprise-domain>/login/oauth/access_token`
  - Copilot API：`https://copilot-api.<enterprise-domain>`

## 6. Auth Method 声明

```ts
auth: {
  provider: "github-copilot",
  async loader(getAuth) {
    // ...
  },
  methods: [
    {
      type: "oauth",
      label: "Login with GitHub Copilot",
      prompts: [
        {
          type: "select",
          key: "deploymentType",
          message: "Select GitHub deployment type",
          options: [
            {
              label: "GitHub.com",
              value: "github.com",
              hint: "Public",
            },
            {
              label: "GitHub Enterprise",
              value: "enterprise",
              hint: "Data residency or self-hosted",
            },
          ],
        },
        {
          type: "text",
          key: "enterpriseUrl",
          message: "Enter your GitHub Enterprise URL or domain",
          placeholder: "company.ghe.com or https://company.ghe.com",
          when: { key: "deploymentType", op: "eq", value: "enterprise" },
          validate: (value) => {
            if (!value) return "URL or domain is required"
            try {
              const url = value.includes("://") ? new URL(value) : new URL(`https://${value}`)
              if (!url.hostname) return "Please enter a valid URL or domain"
              return undefined
            } catch {
              return "Please enter a valid URL (e.g., company.ghe.com or https://company.ghe.com)"
            }
          },
        },
      ],
      async authorize(inputs = {}) {
        // ...
      },
    },
  ],
}
```

这里定义了一个 `oauth` 类型的 Provider auth method。用户可以选择 GitHub.com 或 GitHub Enterprise。如果选择 Enterprise，会额外要求输入 Enterprise URL/domain。

## 7. 发起 Device Authorization

```ts
async authorize(inputs = {}) {
  const deploymentType = inputs.deploymentType || "github.com"

  let domain = "github.com"

  if (deploymentType === "enterprise") {
    const enterpriseUrl = inputs.enterpriseUrl
    domain = normalizeDomain(enterpriseUrl!)
  }

  const urls = getUrls(domain)

  const deviceResponse = await fetch(urls.DEVICE_CODE_URL, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      "User-Agent": `opencode/${InstallationVersion}`,
    },
    body: JSON.stringify({
      client_id: CLIENT_ID,
      scope: "read:user",
    }),
  })

  if (!deviceResponse.ok) {
    throw new Error("Failed to initiate device authorization")
  }

  const deviceData = (await deviceResponse.json()) as {
    verification_uri: string
    user_code: string
    device_code: string
    interval: number
  }

  return {
    url: deviceData.verification_uri,
    instructions: `Enter code: ${deviceData.user_code}`,
    method: "auto" as const,
    async callback() {
      // polling
    },
  }
}
```

对应 HTTP 请求：

```http
POST https://github.com/login/device/code
Accept: application/json
Content-Type: application/json
User-Agent: opencode/<version>

{
  "client_id": "Ov23li8tweQw6odWQebz",
  "scope": "read:user"
}
```

返回结果用于提示用户打开 GitHub 页面并输入 `user_code`。

## 8. 轮询 OAuth Access Token

```ts
async callback() {
  while (true) {
    const response = await fetch(urls.ACCESS_TOKEN_URL, {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
        "User-Agent": `opencode/${InstallationVersion}`,
      },
      body: JSON.stringify({
        client_id: CLIENT_ID,
        device_code: deviceData.device_code,
        grant_type: "urn:ietf:params:oauth:grant-type:device_code",
      }),
    })

    if (!response.ok) return { type: "failed" as const }

    const data = (await response.json()) as {
      access_token?: string
      error?: string
      interval?: number
    }

    if (data.access_token) {
      const result: {
        type: "success"
        refresh: string
        access: string
        expires: number
        provider?: string
        enterpriseUrl?: string
      } = {
        type: "success",
        refresh: data.access_token,
        access: data.access_token,
        expires: 0,
      }

      if (deploymentType === "enterprise") {
        result.enterpriseUrl = domain
      }

      return result
    }

    if (data.error === "authorization_pending") {
      await sleep(deviceData.interval * 1000 + OAUTH_POLLING_SAFETY_MARGIN_MS)
      continue
    }

    if (data.error === "slow_down") {
      let newInterval = (deviceData.interval + 5) * 1000

      const serverInterval = data.interval
      if (serverInterval && typeof serverInterval === "number" && serverInterval > 0) {
        newInterval = serverInterval * 1000
      }

      await sleep(newInterval + OAUTH_POLLING_SAFETY_MARGIN_MS)
      continue
    }

    if (data.error) return { type: "failed" as const }

    await sleep(deviceData.interval * 1000 + OAUTH_POLLING_SAFETY_MARGIN_MS)
    continue
  }
}
```

关键点：

- 使用 OAuth Device Flow 标准 grant type：`urn:ietf:params:oauth:grant-type:device_code`
- 成功后返回：
  - `access = data.access_token`
  - `refresh = data.access_token`
  - `expires = 0`
- 这里没有真正的 refresh token。
- `expires: 0` 表示 OpenCode 目前不基于过期时间自动刷新这个 token。
- Enterprise 模式额外保存 `enterpriseUrl`。

## 9. Auth 结果如何保存

文件：`packages/opencode/src/provider/auth.ts`

ProviderAuth 先保存 pending OAuth callback：

```ts
const authorize = Effect.fn("ProviderAuth.authorize")(function* (
  input: { providerID: ProviderV2.ID } & AuthorizeInput,
) {
  const { hooks, pending } = yield* InstanceState.get(state)
  const method = hooks[input.providerID].methods[input.method]
  if (method.type !== "oauth") return

  const result = yield* Effect.promise(() => method.authorize(input.inputs))
  pending.set(input.providerID, result)

  return {
    url: result.url,
    method: result.method,
    instructions: result.instructions,
  }
})
```

用户完成 GitHub Device Flow 后，`callback()` 会执行 pending callback，并保存 auth：

```ts
const callback = Effect.fn("ProviderAuth.callback")(function* (
  input: { providerID: ProviderV2.ID } & CallbackInput,
) {
  const pending = (yield* InstanceState.get(state)).pending
  const match = pending.get(input.providerID)
  if (!match) return yield* new OauthMissing({ providerID: input.providerID })

  const result = yield* Effect.promise(() =>
    match.method === "code" ? match.callback(input.code!) : match.callback(),
  )
  if (!result || result.type !== "success") return yield* new OauthCallbackFailed({})

  if ("refresh" in result) {
    const { type: _, provider: __, refresh, access, expires, ...extra } = result
    yield* auth.set(input.providerID, {
      type: "oauth",
      access,
      refresh,
      expires,
      ...extra,
    })
  }
})
```

保存后的结构大致是：

```json
{
  "github-copilot": {
    "type": "oauth",
    "access": "<github-oauth-access-token>",
    "refresh": "<same-token>",
    "expires": 0,
    "enterpriseUrl": "ghe.example.com"
  }
}
```

## 10. Auth 数据结构和持久化

文件：`packages/opencode/src/auth/index.ts`

```ts
export class Oauth extends Schema.Class<Oauth>("OAuth")({
  type: Schema.Literal("oauth"),
  refresh: Schema.String,
  access: Schema.String,
  expires: NonNegativeInt,
  accountId: Schema.optional(Schema.String),
  enterpriseUrl: Schema.optional(Schema.String),
}) {}
```

写入本地 auth 文件：

```ts
const file = path.join(Global.Path.data, "auth.json")

const set = Effect.fn("Auth.set")(function* (key: string, info: Info) {
  const norm = key.replace(/\/+$/, "")
  const data = yield* all()
  if (norm !== key) delete data[key]
  delete data[norm + "/"]
  yield* fsys
    .writeJson(file, { ...data, [norm]: info }, 0o600)
    .pipe(Effect.mapError(fail("Failed to write auth data")))
})
```

权限为 `0o600`，说明本地保存时限制为当前用户可读写。

## 11. Provider 初始化时加载 auth.loader

文件：`packages/opencode/src/provider/provider.ts`

Provider 初始化时会遍历插件，如果插件提供 `auth.loader`，就调用它：

```ts
for (const plugin of plugins) {
  if (!plugin.auth) continue
  const providerID = ProviderV2.ID.make(plugin.auth.provider)
  if (disabled.has(providerID)) continue

  const stored = yield* auth.get(providerID).pipe(Effect.orDie)
  if (!stored) continue
  if (!plugin.auth.loader) continue

  const options = yield* Effect.promise(() =>
    plugin.auth!.loader!(
      () => bridge.promise(auth.get(providerID).pipe(Effect.orDie)) as any,
      toPublicInfo(database[plugin.auth!.provider]),
    ),
  )

  const opts = options ?? {}
  const patch: Partial<Info> = providers[providerID] ? { options: opts } : { source: "custom", options: opts }
  mergeProvider(providerID, patch)
}
```

这一步会把 Copilot auth loader 返回的 options 合并到 provider options 中。Copilot loader 返回的核心内容是一个自定义 `fetch`。

## 12. Copilot auth.loader：请求时注入 Bearer Token

文件：`packages/opencode/src/plugin/github-copilot/copilot.ts`

```ts
async loader(getAuth) {
  const info = await getAuth()
  if (!info || info.type !== "oauth") return {}

  return {
    apiKey: "",
    async fetch(request: RequestInfo | URL, init?: RequestInit) {
      const info = await getAuth()
      if (info.type !== "oauth") return fetch(request, init)

      const url = request instanceof URL ? request.href : typeof request === "string" ? request : request.url

      const { isVision, isAgent } = iife(() => {
        try {
          const body = typeof init?.body === "string" ? JSON.parse(init.body) : init?.body

          // Completions API
          if (body?.messages && url.includes("completions")) {
            const last = body.messages[body.messages.length - 1]
            return {
              isVision: body.messages.some(
                (msg: any) =>
                  Array.isArray(msg.content) && msg.content.some((part: any) => part.type === "image_url"),
              ),
              isAgent: last?.role !== "user" || imgMsg(last),
            }
          }

          // Responses API
          if (body?.input) {
            const last = body.input[body.input.length - 1]
            return {
              isVision: body.input.some(
                (item: any) =>
                  Array.isArray(item?.content) && item.content.some((part: any) => part.type === "input_image"),
              ),
              isAgent: last?.role !== "user" || imgMsg(last),
            }
          }

          // Messages API
          if (body?.messages) {
            const last = body.messages[body.messages.length - 1]
            const hasNonToolCalls =
              Array.isArray(last?.content) && last.content.some((part: any) => part?.type !== "tool_result")
            return {
              isVision: body.messages.some(
                (item: any) =>
                  Array.isArray(item?.content) &&
                  item.content.some(
                    (part: any) =>
                      part?.type === "image" ||
                      (part?.type === "tool_result" &&
                        Array.isArray(part?.content) &&
                        part.content.some((nested: any) => nested?.type === "image")),
                  ),
              ),
              isAgent: !(last?.role === "user" && hasNonToolCalls) || imgMsg(last),
            }
          }
        } catch {}
        return { isVision: false, isAgent: false }
      })

      const headers: Record<string, string> = {
        "x-initiator": isAgent ? "agent" : "user",
        ...(init?.headers as Record<string, string>),
        "User-Agent": `opencode/${InstallationVersion}`,
        Authorization: `Bearer ${info.access}`,
        "Openai-Intent": "conversation-edits",
      }

      if (isVision) {
        headers["Copilot-Vision-Request"] = "true"
      }

      delete headers["x-api-key"]
      delete headers["authorization"]

      return fetch(request, {
        ...init,
        headers,
      })
    },
  }
}
```

关键行为：

- 每次请求前重新调用 `getAuth()`，获取最新保存的 auth。
- 如果 auth 不是 OAuth，则退回原始 `fetch`。
- 设置：
  - `Authorization: Bearer <info.access>`
  - `User-Agent: opencode/<version>`
  - `Openai-Intent: conversation-edits`
  - `x-initiator: user | agent`
- 如果检测到图片输入，设置：`Copilot-Vision-Request: true`
- 删除：
  - `x-api-key`
  - 小写 `authorization`

这说明 Copilot 请求真正使用的是保存的 GitHub OAuth token，而不是 SDK 默认 api key。

## 13. 模型发现时也使用同一个 token

文件：`packages/opencode/src/plugin/github-copilot/copilot.ts`

```ts
async models(provider, ctx) {
  if (ctx.auth?.type !== "oauth") {
    models = {}
    return Object.fromEntries(Object.entries(provider.models).map(([id, model]) => [id, fix(model, base())]))
  }

  const auth = ctx.auth

  return CopilotModels.get(
    base(auth.enterpriseUrl),
    {
      ...(provider.options?.headers as Record<string, string> | undefined),
      Authorization: `Bearer ${auth.access}`,
      "User-Agent": `opencode/${InstallationVersion}`,
      "X-GitHub-Api-Version": API_VERSION,
    },
    provider.models,
  )
    .then((result) => {
      models = result.models
      return Object.fromEntries(
        Object.entries(result.models).filter(([, model]) => result.pickerEnabled.has(model.api.id)),
      )
    })
    .catch((error) => {
      models = {}
      return Object.fromEntries(
        Object.entries(provider.models).map(([id, model]) => [id, fix(model, base(auth.enterpriseUrl))]),
      )
    })
}
```

如果已有 OAuth auth，会请求 Copilot `/models`：

```http
GET https://api.githubcopilot.com/models
Authorization: Bearer <github-oauth-access-token>
User-Agent: opencode/<version>
X-GitHub-Api-Version: 2026-06-01
```

Enterprise 情况：

```http
GET https://copilot-api.<enterprise-domain>/models
Authorization: Bearer <github-oauth-access-token>
User-Agent: opencode/<version>
X-GitHub-Api-Version: 2026-06-01
```

## 14. `/models` 实现

文件：`packages/opencode/src/plugin/github-copilot/models.ts`

```ts
export async function get(
  baseURL: string,
  headers: HeadersInit = {},
  existing: Record<string, Model> = {},
): Promise<{ models: Record<string, Model>; pickerEnabled: Set<string> }> {
  const data = await fetch(`${baseURL}/models`, {
    headers,
    signal: AbortSignal.timeout(5_000),
  }).then(async (res) => {
    if (!res.ok) {
      throw new Error(`Failed to fetch models: ${res.status}`)
    }
    return decodeModels(await res.json())
  })

  const result = { ...existing }
  const remote = new Map(
    data.data.flatMap((raw) => {
      const item = Option.getOrUndefined(decodeItem(raw))
      return item && usable(item) ? ([[item.id, item]] as const) : []
    }),
  )

  for (const [key, model] of Object.entries(result)) {
    const m = remote.get(model.api.id)
    if (!m) {
      delete result[key]
      continue
    }
    result[key] = build(key, m, baseURL, model)
  }

  for (const [id, m] of remote) {
    if (id in result) continue
    result[id] = build(id, m, baseURL)
  }

  return {
    models: result,
    pickerEnabled: new Set([...remote].filter(([, item]) => item.model_picker_enabled).map(([id]) => id)),
  }
}
```

该函数负责：

- 调用 `${baseURL}/models`
- 解析 Copilot 返回的模型元数据
- 过滤不可用模型
- 合并已有模型配置
- 返回 picker enabled 的模型集合

## 15. Copilot 模型 API URL 和 SDK 包选择

文件：`packages/opencode/src/plugin/github-copilot/models.ts`

```ts
function build(key: string, remote: SelectableItem, url: string, prev?: Model): Model {
  const isMsgApi = remote.supported_endpoints?.includes("/v1/messages")

  const model: Model = {
    id: key,
    providerID: "github-copilot",
    api: {
      id: remote.id,
      url: isMsgApi ? `${url}/v1` : url,
      npm: isMsgApi ? "@ai-sdk/anthropic" : "@ai-sdk/github-copilot",
    },
    status: "active",
    // ...
  }

  return model
}
```

含义：

- 支持 `/v1/messages` 的模型会走 Anthropic SDK 兼容路径：
  - `api.url = <baseURL>/v1`
  - `npm = "@ai-sdk/anthropic"`
- 其他 Copilot 模型走 OpenCode 自己包装的 Copilot SDK：
  - `api.url = <baseURL>`
  - `npm = "@ai-sdk/github-copilot"`

## 16. Provider SDK 加载

文件：`packages/opencode/src/provider/provider.ts`

```ts
const BUNDLED_PROVIDERS: Record<string, () => Promise<(opts: any) => BundledSDK>> = {
  "@ai-sdk/github-copilot": () =>
    import("@opencode-ai/core/github-copilot/copilot-provider").then((m) => m.createOpenaiCompatible),
}
```

Copilot 的 SDK 实际来自：

```text
packages/core/src/github-copilot/copilot-provider.ts
```

它包装成 OpenAI-compatible provider。

## 17. Copilot SDK 的 Authorization 兜底

文件：`packages/core/src/github-copilot/copilot-provider.ts`

```ts
export function createOpenaiCompatible(options: OpenaiCompatibleProviderSettings = {}): OpenaiCompatibleProvider {
  const baseURL = withoutTrailingSlash(options.baseURL ?? "https://api.openai.com/v1")

  if (!baseURL) {
    throw new Error("baseURL is required")
  }

  const headers = {
    ...(options.apiKey && { Authorization: `Bearer ${options.apiKey}` }),
    ...options.headers,
  }

  const getHeaders = () => withUserAgentSuffix(headers, `ai-sdk/openai-compatible/${VERSION}`)

  const createChatModel = (modelId: OpenaiCompatibleModelId) => {
    return new OpenAICompatibleChatLanguageModel(modelId, {
      provider: `${options.name ?? "openai-compatible"}.chat`,
      headers: getHeaders,
      url: ({ path }) => `${baseURL}${path}`,
      fetch: options.fetch,
    })
  }

  const createResponsesModel = (modelId: OpenaiCompatibleModelId) => {
    return new OpenAIResponsesLanguageModel(modelId, {
      provider: `${options.name ?? "openai-compatible"}.responses`,
      headers: getHeaders,
      url: ({ path }) => `${baseURL}${path}`,
      fetch: options.fetch,
    })
  }

  // ...
}
```

虽然 SDK 支持 `apiKey` 转 Bearer header，但 Copilot OAuth 场景中关键是 `options.fetch`。因为 `auth.loader()` 返回的自定义 fetch 会覆盖请求头里的 Authorization。

## 18. 请求模型选择：Chat vs Responses

文件：`packages/opencode/src/provider/provider.ts`

```ts
"github-copilot": () =>
  Effect.succeed({
    autoload: false,
    async getModel(sdk: any, modelID: string, _options?: Record<string, any>) {
      if (sdk.responses === undefined && sdk.chat === undefined) return sdk.languageModel(modelID)
      const match = /^gpt-(\d+)/.exec(modelID)
      if (match && Number(match[1]) >= 5 && !modelID.startsWith("gpt-5-mini")) return sdk.responses(modelID)
      return sdk.chat(modelID)
    },
    options: {},
  }),
```

规则：

- `gpt-5` 及以上模型默认使用 Responses API。
- `gpt-5-mini` 例外，仍走 Chat Completions。
- 其他模型走 Chat。

## 19. 请求头增强逻辑

文件：`packages/opencode/src/plugin/github-copilot/copilot.ts`

```ts
"chat.headers": async (incoming, output) => {
  if (!incoming.model.providerID.includes("github-copilot")) return

  output.headers["X-GitHub-Api-Version"] = API_VERSION

  if (incoming.agent === "title") {
    output.headers["X-Interaction-Type"] = "agent-session-name-generation"
  }

  if (incoming.model.api.npm === "@ai-sdk/anthropic") {
    output.headers["anthropic-beta"] = "interleaved-thinking-2025-05-14"
  }

  const parts = await sdk.session
    .message({
      path: {
        id: incoming.message.sessionID,
        messageID: incoming.message.id,
      },
      query: {
        directory: input.directory,
      },
      throwOnError: true,
    })
    .catch(() => undefined)

  if (
    parts?.data.parts?.some(
      (part) =>
        part.type === "compaction" ||
        (part.type === "text" && part.synthetic && part.metadata?.compaction_continue === true),
    )
  ) {
    output.headers["x-initiator"] = "agent"
    return
  }

  const session = await sdk.session
    .get({
      path: {
        id: incoming.sessionID,
      },
      query: {
        directory: input.directory,
      },
      throwOnError: true,
    })
    .catch(() => undefined)

  if (!session || !session.data.parentID) return

  output.headers["x-initiator"] = "agent"
}
```

该 hook 会补充：

- `X-GitHub-Api-Version`
- title 生成时的 `X-Interaction-Type`
- Anthropic Copilot 模型所需的 `anthropic-beta`
- 对 agent/subagent/compaction 场景设置 `x-initiator: agent`

最终这些 headers 会和 auth loader 中的 headers 合并进入请求。

## 20. 参数修正逻辑

文件：`packages/opencode/src/plugin/github-copilot/copilot.ts`

```ts
"chat.params": async (incoming, output) => {
  if (!incoming.model.providerID.includes("github-copilot")) return

  // Match github copilot cli, omit maxOutputTokens for gpt models
  if (incoming.model.api.id.includes("gpt")) {
    output.maxOutputTokens = undefined
  }

  if (incoming.model.api.npm === "@ai-sdk/anthropic") {
    output.options.toolStreaming = false
  }
}
```

作用：

- GPT 系列模型不显式传 `maxOutputTokens`，匹配 GitHub Copilot CLI 行为。
- Anthropic Messages shim 不接受部分 tool streaming 字段，因此禁用 `toolStreaming`。

## 21. GitHub Action token exchange 与 Copilot 无关

仓库中存在 GitHub App token 交换逻辑。

文件：`packages/function/src/api.ts`

```ts
.post("/exchange_github_app_token", async (c) => {
  const EXPECTED_AUDIENCE = "opencode-github-action"
  const GITHUB_ISSUER = "https://token.actions.githubusercontent.com"
  const JWKS_URL = `${GITHUB_ISSUER}/.well-known/jwks`

  const token = c.req.header("Authorization")?.replace(/^Bearer\s+/i, "")
  if (!token) return c.json({ error: "Authorization header is required" }, { status: 401 })

  const JWKS = createRemoteJWKSet(new URL(JWKS_URL))
  let owner, repo
  try {
    const { payload } = await jwtVerify(token, JWKS, {
      issuer: GITHUB_ISSUER,
      audience: EXPECTED_AUDIENCE,
    })
    const sub = payload.sub
    const parts = sub.split(":")[1].split("/")
    owner = parts[0]
    repo = parts[1]
  } catch (err) {
    return c.json({ error: "Invalid or expired token" }, { status: 403 })
  }

  const auth = createAppAuth({
    appId: Resource.GITHUB_APP_ID.value,
    privateKey: Resource.GITHUB_APP_PRIVATE_KEY.value,
  })

  const appAuth = await auth({ type: "app" })
  const octokit = new Octokit({ auth: appAuth.token })

  const { data: installation } = await octokit.apps.getRepoInstallation({
    owner,
    repo,
  })

  const installationAuth = await auth({
    type: "installation",
    installationId: installation.id,
  })

  return c.json({ token: installationAuth.token })
})
```

这个 token 用途是：

- GitHub API
- issue/comment 操作
- PR 操作
- git push

不是 GitHub Copilot API token。

## 22. 认证逻辑的关键特征

| 特征 | 说明 |
|---|---|
| 没有 Copilot token exchange | 不调用 `api.github.com/copilot_internal/v2/token` |
| 直接使用 GitHub OAuth token | `Authorization: Bearer ${info.access}` |
| token 不刷新 | `refresh` 与 `access` 相同，`expires = 0` |
| 每次请求动态读取 auth | `fetch()` 内部调用 `getAuth()` |
| 支持 Enterprise | 登录 host 和 Copilot API host 都根据 enterpriseUrl 切换 |
| 支持模型动态发现 | 使用 OAuth token 调 `${baseURL}/models` |
| Provider SDK 与 auth 解耦 | SDK 构造请求，auth loader 自定义 fetch 注入认证 |

## 23. 简化伪代码

```ts
// 1. Device Flow
device = POST https://github.com/login/device/code {
  client_id,
  scope: "read:user",
}

// 2. User approves in browser
show(device.verification_uri, device.user_code)

// 3. Poll token
token = POST https://github.com/login/oauth/access_token {
  client_id,
  device_code,
  grant_type: "urn:ietf:params:oauth:grant-type:device_code",
}

// 4. Store auth
auth["github-copilot"] = {
  type: "oauth",
  access: token.access_token,
  refresh: token.access_token,
  expires: 0,
}

// 5. Discover models
GET https://api.githubcopilot.com/models
Authorization: Bearer auth.access

// 6. Chat request
POST https://api.githubcopilot.com/chat/completions
Authorization: Bearer auth.access
X-GitHub-Api-Version: 2026-06-01
Openai-Intent: conversation-edits
x-initiator: user | agent
```

## 24. 实现评价

这个实现的优点是简单：不需要维护 Copilot token refresh 机制，也不需要依赖 GitHub 私有 token exchange endpoint。OpenCode 将 GitHub OAuth access token 作为 Copilot API 的 Bearer token 使用，并通过 Provider plugin 的 `auth.loader()` 把认证细节封装在自定义 `fetch` 中。

潜在限制是：如果 GitHub 后续要求专门的 Copilot API token 或明确的过期刷新策略，当前 `refresh = access`、`expires = 0` 的实现需要升级为真正的 token refresh 或 token exchange 流程。
