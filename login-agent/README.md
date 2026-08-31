# login-agent

A small Node + Playwright service that completes GitHub's Device Flow
authorization in a headless Chromium, so copilot2api can authenticate accounts
without a human clicking **Authorize**.

It is optional. copilot2api only calls it when `COPILOT2API_LOGIN_AGENT_URL` is
set, and any failure falls back to the normal manual flow.

See [../docs/auto-login.md](../docs/auto-login.md) for setup, security notes, and
troubleshooting.

## Layout

| File | Purpose |
|------|---------|
| `src/server.js` | HTTP surface (`/health`, `/login`), shared-token auth, serial task queue. |
| `src/github.js` | The Playwright automation: sign in, enter the code, authorize. |
| `src/selectors.js` | Every GitHub selector, with ordered fallbacks. **Edit here when GitHub changes its markup.** |
| `src/codes.js` | Pure helpers: user-code normalization, page classification, error mapping. Fully unit-tested. |

## Development

```bash
npm install                 # downloads Chromium
npm test                    # unit tests, no network required
LOGIN_AGENT_TOKEN=dev npm start
```

To watch the browser while debugging, set `LOGIN_AGENT_HEADLESS=false`.

The Playwright version in `package.json` must stay in sync with the base image
tag in `Dockerfile`; the image supplies the browsers, so the container installs
the driver package only.
