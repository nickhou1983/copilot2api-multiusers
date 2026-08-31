/**
 * Pure helpers shared by the automation and covered by unit tests.
 *
 * Keeping user-code handling and failure classification free of Playwright
 * means the tricky logic can be tested without a browser or network.
 */

/** Failure classes. Mirrors the Err* constants in internal/loginagent/client.go. */
export const ErrorClass = {
  INVALID_CREDENTIALS: 'invalid_credentials',
  TWO_FACTOR_REQUIRED: 'two_factor_required',
  DEVICE_VERIFICATION: 'device_verification',
  CAPTCHA: 'captcha',
  CODE_REJECTED: 'code_rejected',
  TIMEOUT: 'timeout',
  UNKNOWN: 'unknown',
};

/** Automation stages, reported back so the admin UI can show where it stopped. */
export const Step = {
  LAUNCH: 'launch',
  LOGIN: 'login',
  DEVICE_CODE: 'device_code',
  AUTHORIZE: 'authorize',
  DONE: 'done',
};

/**
 * Normalizes a GitHub device user code.
 *
 * GitHub issues codes like "ABCD-1234". The device page renders either a single
 * input that wants the dashed form, or one box per character that wants bare
 * characters, so both shapes are returned.
 *
 * @param {string} code
 * @returns {{ dashed: string, bare: string, chars: string[] }}
 */
export function normalizeUserCode(code) {
  if (typeof code !== 'string') {
    throw new TypeError('user code must be a string');
  }
  const trimmed = code.trim().toUpperCase();
  if (!trimmed) {
    throw new Error('user code is empty');
  }
  // Strip everything outside the code alphabet: GitHub codes are alphanumeric
  // plus a separating dash.
  const dashed = trimmed.replace(/[^A-Z0-9-]/g, '');
  const bare = dashed.replace(/-/g, '');
  if (!bare) {
    throw new Error(`user code has no usable characters: ${code}`);
  }
  return { dashed, bare, chars: bare.split('') };
}

/**
 * Classifies a GitHub login page into a failure class.
 *
 * Both the URL and the visible page text are considered because GitHub signals
 * some states only by navigation (two-factor, device verification) and others
 * only by an inline flash message (bad password).
 *
 * @param {{url?: string, text?: string}} page
 * @returns {{ errorClass: string, message: string } | null} null when nothing is wrong
 */
export function classifyLoginPage({ url = '', text = '' } = {}) {
  const u = url.toLowerCase();
  const t = text.toLowerCase();

  if (u.includes('/sessions/verified-device') || t.includes('device verification') || t.includes('verify your device')) {
    return {
      errorClass: ErrorClass.DEVICE_VERIFICATION,
      message:
        'GitHub asked for an email device-verification code. Authorize once manually so the browser profile becomes trusted, then retry.',
    };
  }
  if (
    u.includes('/sessions/two-factor') ||
    u.includes('/two_factor') ||
    t.includes('two-factor authentication') ||
    t.includes('authentication code')
  ) {
    return {
      errorClass: ErrorClass.TWO_FACTOR_REQUIRED,
      message: 'This GitHub account has two-factor authentication enabled, which automated login does not support.',
    };
  }
  if (t.includes('unable to verify') || t.includes('captcha') || t.includes('are you a robot')) {
    return {
      errorClass: ErrorClass.CAPTCHA,
      message: 'GitHub presented a human-verification challenge.',
    };
  }
  if (
    t.includes('incorrect username or password') ||
    t.includes('the username or password you entered is incorrect') ||
    t.includes('incorrect password')
  ) {
    return {
      errorClass: ErrorClass.INVALID_CREDENTIALS,
      message: 'GitHub rejected the stored username or password.',
    };
  }
  if (t.includes('your account has been flagged') || t.includes('account is suspended')) {
    return {
      errorClass: ErrorClass.INVALID_CREDENTIALS,
      message: 'The GitHub account is flagged or suspended.',
    };
  }
  return null;
}

/**
 * Classifies the device-authorization page after the user code is submitted.
 *
 * @param {{url?: string, text?: string}} page
 * @returns {{ errorClass: string, message: string } | null}
 */
export function classifyDevicePage({ url = '', text = '' } = {}) {
  const t = text.toLowerCase();

  if (
    t.includes('the code you entered is incorrect') ||
    t.includes('incorrect or expired') ||
    t.includes('this code is invalid') ||
    t.includes('code is no longer valid') ||
    t.includes('code has expired')
  ) {
    return {
      errorClass: ErrorClass.CODE_REJECTED,
      message: 'GitHub rejected the device code as incorrect or expired.',
    };
  }
  // A login redirect at this point means the session was not established.
  if (String(url).toLowerCase().includes('/login?') || String(url).toLowerCase().endsWith('/login')) {
    return {
      errorClass: ErrorClass.UNKNOWN,
      message: 'GitHub redirected back to the sign-in page; the session was not established.',
    };
  }
  return null;
}

/** Recognizes the "device activated" confirmation page. */
export function isDeviceActivated(text = '') {
  const t = String(text).toLowerCase();
  return (
    t.includes('device activated') ||
    t.includes("you're all set") ||
    t.includes('you are all set') ||
    t.includes('congratulations, you')
  );
}

/**
 * Maps a thrown error to a failure class. Playwright timeouts get their own
 * class because the operator fix differs from a generic failure.
 */
export function classifyThrownError(err) {
  const message = err && err.message ? err.message : String(err);
  if ((err && err.name === 'TimeoutError') || /timeout/i.test(message)) {
    return { errorClass: ErrorClass.TIMEOUT, message };
  }
  return { errorClass: ErrorClass.UNKNOWN, message };
}

/**
 * Validates an incoming /login request body.
 *
 * @returns {string[]} missing/invalid field messages, empty when valid
 */
export function validateLoginRequest(body) {
  if (!body || typeof body !== 'object' || Array.isArray(body)) {
    return ['request body must be a JSON object'];
  }
  const problems = [];
  for (const field of ['account_id', 'username', 'password', 'user_code']) {
    if (typeof body[field] !== 'string' || body[field].trim() === '') {
      problems.push(`${field} is required`);
    }
  }
  if (body.verification_uri !== undefined && typeof body.verification_uri !== 'string') {
    problems.push('verification_uri must be a string');
  }
  return problems;
}

/** Sanitizes an account id so it is safe to use as a filesystem path segment. */
export function profileDirName(accountId) {
  const safe = String(accountId).replace(/[^A-Za-z0-9._-]/g, '_');
  return safe === '' || safe === '.' || safe === '..' ? '_' : safe;
}
