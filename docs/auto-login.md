# Automated Device Flow Login

By default, authenticating an account in the admin UI is only half-automated: the
proxy obtains a `user_code` and a verification URL, and then a human has to open
GitHub, paste the code, and click **Authorize**.

With more than a handful of accounts, that last step gets tedious. This optional
feature adds a dedicated **login agent** container that performs the browser work
for you.

> This page is the operational guide (deploy, configure, troubleshoot). For the
> design rationale and the end-to-end business flow, see
> [auto-login-design.md](auto-login-design.md).

## Architecture

```
┌──────────────────────────────┐              ┌────────────────────────────────┐
│ copilot2api (Go)             │              │ login-agent (Node + Playwright)│
│                              │  POST /login │                                │
│ admin UI  [Auto authenticate]├─────────────►│ 1. sign in at github.com/login │
│                              │  user_code   │ 2. open the verification URI   │
│ StartDeviceFlow()  ──┐       │  username    │ 3. type the user code          │
│                      │ poll  │  password    │ 4. click Authorize             │
│ CompleteDeviceFlow() │       │◄─────────────┤ result / failure screenshot    │
│   └─► credentials.json       │              │                                │
└──────────────────────────────┘              └────────────────────────────────┘
       (unchanged logic)                        compose network, no host ports
```

The key property: **the device-flow polling and credential storage are unchanged.**
The agent only replaces the human click. The proxy still polls GitHub and still
writes the token to `<token_dir>/credentials.json` exactly as before.

That means automation is strictly additive. If the agent fails for any reason,
the `user_code` remains valid for its full lifetime and the admin UI keeps showing
the code and link, so you can finish by hand.

## Setup with Docker Compose

```bash
cp .env.example .env
# edit .env and set both tokens to long random values
docker compose up -d --build
```

Then open `http://127.0.0.1:7777/admin/?admin_token=<your admin token>`.

> If port 7777 is already in use on the host (for example by an existing
> copilot2api container), set `COPILOT2API_HOST_PORT` in `.env` to publish the
> proxy on a different host port.

1. Add an account, filling in the **GitHub username** and **GitHub password** fields.
   (You can also add credentials later with the **Login** button on the account row.)
2. Click **Auto authenticate**.
3. The dialog shows automation progress. On success the account flips to
   `authenticated` and the token lands in the account's credential directory.

## Configuration

The feature is **off unless `COPILOT2API_LOGIN_AGENT_URL` is set.** Without it,
the admin UI hides the automation controls and behaves exactly as before.

| Variable | Description | Default |
|----------|-------------|---------|
| `COPILOT2API_LOGIN_AGENT_URL` | Base URL of the login agent. Unset disables automation entirely. | _(unset)_ |
| `COPILOT2API_LOGIN_AGENT_TOKEN` | Shared secret sent as `X-Agent-Token`. Must match the agent's `LOGIN_AGENT_TOKEN`. | _(unset, no auth)_ |
| `COPILOT2API_LOGIN_AGENT_TIMEOUT_SECONDS` | Per-login timeout. | `180` |
| `COPILOT2API_LOGINS_FILE` | Where GitHub credentials are stored. | `<token-dir>/github_logins.json` |

Login agent container variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `LOGIN_AGENT_TOKEN` | Shared secret. When set, requests without a matching `X-Agent-Token` get `401`. | _(unset, no auth)_ |
| `PORT` / `HOST` | Listen address. | `8080` / `0.0.0.0` |
| `LOGIN_AGENT_PROFILE_DIR` | Where persistent browser profiles are kept. | `/data/profiles` |
| `LOGIN_AGENT_HEADLESS` | Set to `false` to watch the browser (debugging only). | `true` |

## Security

This feature stores GitHub passwords. Read this section before enabling it.

- **Passwords are stored in plain text** in `github_logins.json` (file mode `0600`).
  They must be readable by the proxy to be replayed to the agent. Treat the token
  directory as a secret store, and keep it off any shared or backed-up volume you
  would not trust with a password.
- **Never expose the login agent.** The compose file deliberately declares no
  `ports:` for it. Anyone who can reach the agent can drive a browser with your
  credentials.
- **Set `COPILOT2API_ADMIN_TOKEN`.** Without it, anyone who can reach the admin UI
  can trigger a login using stored credentials. The compose file requires it.
- **Set `LOGIN_AGENT_TOKEN`** so only the proxy can call the agent.
- The password is never returned by any API (`GET .../login` reports only
  `username` and `has_password`), and never appears in logs on either side.
- Automating a login with your own credentials is your responsibility; confirm it
  is acceptable under GitHub's terms for your account.

## Requirements and limitations

- **Two-factor authentication is not supported.** If the account has 2FA enabled,
  automation reports `two_factor_required` and you must authorize manually.
- **New-device email verification.** GitHub may email a verification code when it
  sees a login from an unfamiliar device. Browser profiles are persisted per
  account (in the `login-agent-profiles` volume) so subsequent logins reuse the
  trusted-device cookie, but the *first* login from a new host can still be
  challenged. Authorize that one manually; later runs should succeed.
- Logins are processed **one at a time** to bound memory: Chromium is expensive,
  and several accounts at once would otherwise run several browsers concurrently.

## Troubleshooting

When automation fails, the admin UI shows the failure class and a screenshot of
the page the browser stopped on. That screenshot is usually enough to tell what
happened.

| Class | Meaning | What to do |
|-------|---------|------------|
| `invalid_credentials` | GitHub rejected the username or password. | Fix them with the **Login** button. |
| `two_factor_required` | The account has 2FA enabled. | Authorize manually; automation cannot proceed. |
| `device_verification` | GitHub asked for an emailed device code. | Authorize manually once; the saved profile should prevent a repeat. |
| `captcha` | A CAPTCHA or similar challenge appeared. | Authorize manually and retry later. |
| `code_rejected` | GitHub did not accept the user code. | Usually an expired code — start authentication again. |
| `timeout` | The agent ran out of time. | Raise `COPILOT2API_LOGIN_AGENT_TIMEOUT_SECONDS`; check the agent has network access. |
| `agent_unreachable` | The proxy could not reach the agent. | Check `COPILOT2API_LOGIN_AGENT_URL` and that the agent container is healthy. |

Useful commands:

```bash
docker compose logs -f login-agent      # structured logs, never contain passwords
docker compose exec login-agent \
  node -e "fetch('http://127.0.0.1:8080/health').then(r=>r.text()).then(console.log)"
```

`unknown` failures are the ones worth reading the screenshot for. One known
shape: **"Device code input was not found on the verification page."** means
GitHub served a page the agent did not recognize instead of the code form. When
a session already exists, `/login/device` redirects to
`/login/device/select_account` ("Device Activation — Signed in as …" with a
**Continue** button); the agent clicks through that automatically. If the
screenshot shows some *other* page, the markup has probably changed.

If GitHub changes its markup, the selectors live in one place —
[`login-agent/src/selectors.js`](../login-agent/src/selectors.js) — and each has
ordered fallbacks.

## Agent HTTP API

The agent is a small service you can call directly when debugging.

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

Response on success: `{"ok": true, "step": "done"}`.
On failure: `{"ok": false, "step": "login", "error_class": "invalid_credentials",
"error": "...", "screenshot": "<base64 PNG>"}`.

## Admin API additions

| Endpoint | Purpose |
|----------|---------|
| `GET /admin/api/config` | Reports `auto_login_enabled` / `login_store_enabled` so the UI can hide controls. |
| `GET /admin/api/accounts/{id}/login` | Returns `{username, has_password}`. Never the password. |
| `PUT /admin/api/accounts/{id}/login` | Sets `{username, password}`. |
| `DELETE /admin/api/accounts/{id}/login` | Clears stored credentials. |
| `POST /admin/api/accounts/{id}/auth/start` | Accepts `{"auto": true}` to also run the agent. |
| `GET /admin/api/accounts/{id}/auth/status` | Adds an `auto` object: `{state, step, error_class, error, has_screenshot}`. |
| `GET /admin/api/accounts/{id}/auth/screenshot` | The most recent failure screenshot as `image/png`. |

Creating an account accepts optional `github_username` / `github_password` so an
account and its credentials can be added in one request. Deleting an account also
deletes its stored credentials.
