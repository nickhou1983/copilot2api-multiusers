import http from 'node:http';

import { Step, validateLoginRequest } from './codes.js';
import { runLogin } from './github.js';

const PORT = Number(process.env.PORT || 8080);
const HOST = process.env.HOST || '0.0.0.0';
const AGENT_TOKEN = process.env.LOGIN_AGENT_TOKEN || '';
const MAX_BODY_BYTES = 64 * 1024;

/**
 * Serial task queue.
 *
 * Each login starts a Chromium instance, so running them concurrently would
 * multiply memory use and make GitHub more likely to flag the traffic. Requests
 * queue up and run one at a time.
 */
function createQueue() {
  let tail = Promise.resolve();
  let depth = 0;
  return {
    get depth() {
      return depth;
    },
    run(task) {
      depth += 1;
      const result = tail.then(task, task);
      // Keep the chain alive regardless of task outcome.
      tail = result.then(
        () => {},
        () => {},
      );
      return result.finally(() => {
        depth -= 1;
      });
    },
  };
}

const queue = createQueue();

function log(message, extra = {}) {
  // Structured, single-line logs. Credentials are never included.
  console.log(JSON.stringify({ time: new Date().toISOString(), msg: message, ...extra }));
}

function sendJSON(res, status, body) {
  const payload = JSON.stringify(body);
  res.writeHead(status, {
    'Content-Type': 'application/json',
    'Content-Length': Buffer.byteLength(payload),
  });
  res.end(payload);
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    let size = 0;
    req.on('data', (chunk) => {
      size += chunk.length;
      if (size > MAX_BODY_BYTES) {
        reject(new Error('request body too large'));
        req.destroy();
        return;
      }
      chunks.push(chunk);
    });
    req.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
    req.on('error', reject);
  });
}

/** Constant-time-ish comparison so the shared token is not leaked by timing. */
function tokenMatches(provided) {
  if (!AGENT_TOKEN) {
    return true;
  }
  const a = Buffer.from(String(provided || ''));
  const b = Buffer.from(AGENT_TOKEN);
  if (a.length !== b.length) {
    return false;
  }
  let diff = 0;
  for (let i = 0; i < a.length; i += 1) {
    diff |= a[i] ^ b[i];
  }
  return diff === 0;
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, `http://${req.headers.host || 'localhost'}`);

  if (req.method === 'GET' && url.pathname === '/health') {
    sendJSON(res, 200, { status: 'ok', queued: queue.depth });
    return;
  }

  if (req.method !== 'POST' || url.pathname !== '/login') {
    sendJSON(res, 404, { ok: false, error: 'not found' });
    return;
  }

  if (!tokenMatches(req.headers['x-agent-token'])) {
    log('rejected request with invalid agent token');
    sendJSON(res, 401, { ok: false, error_class: 'unknown', error: 'invalid agent token' });
    return;
  }

  let body;
  try {
    body = JSON.parse(await readBody(req));
  } catch (err) {
    sendJSON(res, 400, { ok: false, error_class: 'unknown', error: `invalid JSON body: ${err.message}` });
    return;
  }

  const problems = validateLoginRequest(body);
  if (problems.length > 0) {
    sendJSON(res, 400, { ok: false, error_class: 'unknown', error: problems.join('; ') });
    return;
  }

  const accountId = body.account_id;
  log('login queued', { account_id: accountId, queued: queue.depth });

  try {
    const result = await queue.run(() =>
      runLogin(
        {
          accountId,
          username: body.username,
          password: body.password,
          userCode: body.user_code,
          verificationUri: body.verification_uri,
        },
        (msg, extra) => log(msg, { account_id: accountId, ...extra }),
      ),
    );
    log('login finished', {
      account_id: accountId,
      ok: result.ok,
      step: result.step,
      error_class: result.error_class,
    });
    sendJSON(res, 200, result);
  } catch (err) {
    log('login crashed', { account_id: accountId, error: err.message });
    sendJSON(res, 500, {
      ok: false,
      step: Step.LAUNCH,
      error_class: 'unknown',
      error: err.message,
    });
  }
});

server.headersTimeout = 30000;
// A login involves a full browser session; do not cut the response short.
server.requestTimeout = 0;
server.setTimeout(0);

server.listen(PORT, HOST, () => {
  log('login agent listening', { host: HOST, port: PORT, token_protected: AGENT_TOKEN !== '' });
});

for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => {
    log('shutting down', { signal });
    server.close(() => process.exit(0));
  });
}
