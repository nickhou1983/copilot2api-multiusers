# syntax=docker/dockerfile:1
#
# Playwright's official image ships Chromium plus every system library it needs.
# Keep this tag in sync with the playwright version pinned in package.json.
FROM mcr.microsoft.com/playwright:v1.62.1-noble

ENV NODE_ENV=production \
    PORT=8080 \
    HOST=0.0.0.0 \
    LOGIN_AGENT_PROFILE_DIR=/data/profiles

WORKDIR /app

COPY package.json package-lock.json ./
# Browsers already live in /ms-playwright, so only the driver package is needed.
# Retries make the build tolerate flaky registry connections.
ENV PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
RUN npm ci --omit=dev --no-audit --no-fund \
      --fetch-retries=5 --fetch-retry-maxtimeout=120000 --fetch-timeout=300000

COPY src ./src

# Persistent browser profiles keep GitHub sessions and trusted-device cookies
# between logins, which avoids repeated email verification challenges.
RUN mkdir -p /data/profiles && chown -R pwuser:pwuser /data /app
VOLUME ["/data"]

USER pwuser
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD node -e "fetch('http://127.0.0.1:'+(process.env.PORT||8080)+'/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"

CMD ["node", "src/server.js"]
