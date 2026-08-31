import fs from 'node:fs/promises';
import path from 'node:path';
import { chromium } from 'playwright';

import {
  ErrorClass,
  Step,
  classifyDevicePage,
  classifyLoginPage,
  classifyThrownError,
  isDeviceActivated,
  normalizeUserCode,
  profileDirName,
} from './codes.js';
import {
  DEFAULT_DEVICE_URL,
  LOGIN_URL,
  SESSION_PROBE_URL,
  anyAttached,
  firstVisible,
  selectors,
} from './selectors.js';

const NAV_TIMEOUT_MS = Number(process.env.LOGIN_AGENT_NAV_TIMEOUT_MS || 45000);
const PROFILE_ROOT = process.env.LOGIN_AGENT_PROFILE_DIR || '/data/profiles';

/**
 * Runs one automated GitHub Device Flow authorization.
 *
 * The caller (server.js) serializes these so only one browser runs at a time.
 *
 * @param {{accountId: string, username: string, password: string, userCode: string, verificationUri?: string}} input
 * @param {(msg: string, extra?: object) => void} log
 * @returns {Promise<{ok: boolean, step: string, error_class?: string, error?: string, screenshot?: string}>}
 */
export async function runLogin(input, log = () => {}) {
  const { accountId, username, password, userCode } = input;
  const verificationUri = input.verificationUri || DEFAULT_DEVICE_URL;

  let code;
  try {
    code = normalizeUserCode(userCode);
  } catch (err) {
    return fail(Step.DEVICE_CODE, ErrorClass.CODE_REJECTED, err.message);
  }

  // A persistent profile keeps GitHub's session and "trusted device" cookies so
  // repeat authorizations skip the sign-in form and the email verification
  // challenge that a fresh browser would trigger.
  const profileDir = path.join(PROFILE_ROOT, profileDirName(accountId));
  await fs.mkdir(profileDir, { recursive: true });

  let context;
  let step = Step.LAUNCH;
  try {
    context = await chromium.launchPersistentContext(profileDir, {
      headless: true,
      args: ['--no-sandbox', '--disable-dev-shm-usage'],
      viewport: { width: 1280, height: 900 },
      locale: 'en-US',
    });
    context.setDefaultTimeout(NAV_TIMEOUT_MS);
    context.setDefaultNavigationTimeout(NAV_TIMEOUT_MS);

    const page = context.pages()[0] || (await context.newPage());

    step = Step.LOGIN;
    const loginOutcome = await ensureSignedIn(page, username, password, log);
    if (loginOutcome) {
      return fail(step, loginOutcome.errorClass, loginOutcome.message, await screenshot(page));
    }

    step = Step.DEVICE_CODE;
    const codeOutcome = await submitDeviceCode(page, verificationUri, code, log);
    if (codeOutcome) {
      return fail(step, codeOutcome.errorClass, codeOutcome.message, await screenshot(page));
    }

    step = Step.AUTHORIZE;
    const authorizeOutcome = await authorizeDevice(page, log);
    if (authorizeOutcome) {
      return fail(step, authorizeOutcome.errorClass, authorizeOutcome.message, await screenshot(page));
    }

    log('device authorized', { account_id: accountId });
    return { ok: true, step: Step.DONE };
  } catch (err) {
    const { errorClass, message } = classifyThrownError(err);
    let shot;
    try {
      const page = context?.pages()?.[0];
      if (page) shot = await screenshot(page);
    } catch {
      // Screenshot is best-effort.
    }
    return fail(step, errorClass, message, shot);
  } finally {
    if (context) {
      await context.close().catch(() => {});
    }
  }
}

/**
 * Signs in when the persistent profile has no live session.
 * Returns null on success, or a classified failure.
 */
async function ensureSignedIn(page, username, password, log) {
  await page.goto(SESSION_PROBE_URL, { waitUntil: 'domcontentloaded' });

  if (!page.url().includes('/login') && (await anyAttached(page, selectors.signedIn))) {
    log('reusing existing GitHub session');
    return null;
  }

  log('signing in to GitHub');
  await page.goto(LOGIN_URL, { waitUntil: 'domcontentloaded' });

  const loginSelector = await firstVisible(page, selectors.loginField, 10000);
  if (!loginSelector) {
    // No sign-in form and no session marker: classify whatever GitHub is showing.
    const state = await pageState(page);
    return (
      classifyLoginPage(state) || {
        errorClass: ErrorClass.UNKNOWN,
        message: 'GitHub sign-in form was not found.',
      }
    );
  }
  const passwordSelector = await firstVisible(page, selectors.passwordField, 10000);
  if (!passwordSelector) {
    return { errorClass: ErrorClass.UNKNOWN, message: 'GitHub password field was not found.' };
  }

  await page.fill(loginSelector, username);
  await page.fill(passwordSelector, password);

  const submitSelector = await firstVisible(page, selectors.loginSubmit, 5000);
  await Promise.all([
    page.waitForLoadState('domcontentloaded').catch(() => {}),
    submitSelector ? page.click(submitSelector) : page.press(passwordSelector, 'Enter'),
  ]);
  // GitHub may chain redirects (session -> two-factor -> verified-device).
  await page.waitForLoadState('networkidle').catch(() => {});

  const state = await pageState(page);
  const problem = classifyLoginPage(state);
  if (problem) {
    return problem;
  }
  if (state.url.includes('/login') || state.url.includes('/session')) {
    return {
      errorClass: ErrorClass.INVALID_CREDENTIALS,
      message: 'GitHub kept the sign-in page open after submitting the credentials.',
    };
  }
  return null;
}

/**
 * Opens the device page, enters the user code and submits it.
 * Returns null on success, or a classified failure.
 */
async function submitDeviceCode(page, verificationUri, code, log) {
  log('entering device user code');
  await page.goto(verificationUri, { waitUntil: 'domcontentloaded' });

  const loginProblem = classifyLoginPage(await pageState(page));
  if (loginProblem) {
    return loginProblem;
  }

  const filled = await fillUserCode(page, code);
  if (!filled) {
    return { errorClass: ErrorClass.UNKNOWN, message: 'Device code input was not found on the verification page.' };
  }

  const continueSelector = await firstVisible(page, selectors.deviceContinue, 5000);
  if (continueSelector) {
    await page.click(continueSelector).catch(() => {});
  } else {
    await page.keyboard.press('Enter');
  }
  await page.waitForLoadState('networkidle').catch(() => {});

  return classifyDevicePage(await pageState(page));
}

/**
 * Fills the user code into whichever input shape the page renders.
 * @returns {Promise<boolean>} whether an input was found and filled
 */
async function fillUserCode(page, code) {
  const singleSelector = await firstVisible(page, selectors.deviceCodeSingle, 6000);
  if (singleSelector) {
    await page.fill(singleSelector, '');
    // Type rather than fill so GitHub's input handlers (formatting, auto-submit
    // guards) observe the same events a human would produce.
    await page.click(singleSelector);
    await page.keyboard.type(code.dashed, { delay: 20 });
    return true;
  }

  for (const boxSelector of selectors.deviceCodeBoxes) {
    const boxes = page.locator(boxSelector);
    const count = await boxes.count().catch(() => 0);
    if (count < code.chars.length) {
      continue;
    }
    // Per-character boxes auto-advance on input, so click the first one and
    // type the bare code straight through.
    await boxes.first().click();
    await page.keyboard.type(code.bare, { delay: 30 });
    return true;
  }
  return false;
}

/**
 * Clicks the "Authorize" button and confirms the success page.
 * Returns null on success, or a classified failure.
 */
async function authorizeDevice(page, log) {
  const state = await pageState(page);
  if (isDeviceActivated(state.text)) {
    // Some flows land straight on the confirmation page.
    return null;
  }

  const authorizeSelector = await firstVisible(page, selectors.authorize, 10000);
  if (!authorizeSelector) {
    const problem = classifyDevicePage(state);
    return (
      problem || {
        errorClass: ErrorClass.UNKNOWN,
        message: 'Authorize button was not found on the device authorization page.',
      }
    );
  }

  // GitHub keeps the authorize button disabled for a few seconds to force the
  // user to read the scope list; clicking early is a no-op.
  const button = page.locator(authorizeSelector).first();
  await page
    .waitForFunction(
      (sel) => {
        const el = document.querySelector(sel);
        return !!el && !el.disabled && el.getAttribute('aria-disabled') !== 'true';
      },
      authorizeSelector,
      { timeout: 20000 },
    )
    .catch(() => {});

  log('clicking authorize');
  await button.click();
  await page.waitForLoadState('networkidle').catch(() => {});

  const after = await pageState(page);
  if (isDeviceActivated(after.text)) {
    return null;
  }
  const problem = classifyDevicePage(after);
  if (problem) {
    return problem;
  }
  // The device flow poller is the source of truth; if the confirmation text is
  // missing but nothing looks wrong, report it rather than claiming success.
  return {
    errorClass: ErrorClass.UNKNOWN,
    message: 'Authorization was submitted but GitHub did not show the confirmation page.',
  };
}

/** Snapshot of the current URL and visible body text. */
async function pageState(page) {
  const url = page.url();
  let text = '';
  try {
    text = await page.locator('body').innerText({ timeout: 5000 });
  } catch {
    text = '';
  }
  return { url, text };
}

/** Captures a base64-encoded PNG of the current page for troubleshooting. */
async function screenshot(page) {
  try {
    const buf = await page.screenshot({ fullPage: false, timeout: 10000 });
    return buf.toString('base64');
  } catch {
    return undefined;
  }
}

function fail(step, errorClass, error, shot) {
  const body = { ok: false, step, error_class: errorClass, error };
  if (shot) {
    body.screenshot = shot;
  }
  return body;
}
