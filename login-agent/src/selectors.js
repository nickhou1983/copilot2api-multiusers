/**
 * Every GitHub DOM selector lives here.
 *
 * GitHub redesigns these pages periodically, so each logical element lists
 * several candidates tried in order. When the automation breaks, this file plus
 * the failure screenshot is normally all that needs updating.
 */

export const GITHUB_ORIGIN = 'https://github.com';
export const LOGIN_URL = `${GITHUB_ORIGIN}/login`;
export const DEFAULT_DEVICE_URL = `${GITHUB_ORIGIN}/login/device`;
// Cheap authenticated-session probe: redirects to /login when signed out.
export const SESSION_PROBE_URL = `${GITHUB_ORIGIN}/settings/profile`;

export const selectors = {
  /** Username field on the sign-in form. */
  loginField: ['#login_field', 'input[name="login"]'],

  /** Password field on the sign-in form. */
  passwordField: ['#password', 'input[name="password"]'],

  /** Submit button on the sign-in form. */
  loginSubmit: [
    'input[type="submit"][name="commit"]',
    '.js-sign-in-button',
    'form[action="/session"] button[type="submit"]',
  ],

  /** Present only when a session exists (avatar / signed-in chrome). */
  signedIn: [
    'meta[name="user-login"][content]:not([content=""])',
    'button[data-login]',
    'summary[aria-label="View profile and more"]',
  ],

  /** Single-input variant of the device code form. */
  deviceCodeSingle: ['#user-code', 'input[name="user_code"]', 'input[autocomplete="one-time-code"]'],

  /** Per-character variant of the device code form. */
  deviceCodeBoxes: [
    'input[data-index]',
    '.js-device-activation-code-input',
    'form input[type="text"][maxlength="1"]',
  ],

  /** Continue button on the device code form. */
  deviceContinue: [
    'button[type="submit"]',
    'input[type="submit"][value="Continue"]',
    'form[action="/login/device/code"] button',
  ],

  /** Green "Authorize <app>" button on the OAuth grant page. */
  authorize: [
    'button[name="authorize"][value="1"]',
    'input[name="authorize"][value="1"]',
    'button[data-octo-click="oauth_application_authorization"]',
    'form[action*="/login/device/"] button[type="submit"]',
  ],
};

/**
 * Returns the first selector in `candidates` that matches a visible element,
 * or null when none do.
 *
 * @param {import('playwright').Page} page
 * @param {string[]} candidates
 * @param {number} timeoutMs total budget across all candidates
 */
export async function firstVisible(page, candidates, timeoutMs = 5000) {
  const perCandidate = Math.max(250, Math.floor(timeoutMs / candidates.length));
  for (const selector of candidates) {
    try {
      const locator = page.locator(selector).first();
      await locator.waitFor({ state: 'visible', timeout: perCandidate });
      return selector;
    } catch {
      // Try the next candidate.
    }
  }
  return null;
}

/** Returns true when any candidate selector currently matches an attached element. */
export async function anyAttached(page, candidates) {
  for (const selector of candidates) {
    try {
      if ((await page.locator(selector).count()) > 0) {
        return true;
      }
    } catch {
      // Ignore malformed/unsupported selectors and keep checking.
    }
  }
  return false;
}
