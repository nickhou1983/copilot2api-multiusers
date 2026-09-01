import assert from 'node:assert/strict';
import test from 'node:test';

import {
  ErrorClass,
  classifyDevicePage,
  classifyLoginPage,
  classifyThrownError,
  isDeviceActivated,
  isSessionInterstitial,
  normalizeUserCode,
  profileDirName,
  validateLoginRequest,
} from '../src/codes.js';

test('normalizeUserCode returns both dashed and bare forms', () => {
  const code = normalizeUserCode('ABCD-1234');
  assert.equal(code.dashed, 'ABCD-1234');
  assert.equal(code.bare, 'ABCD1234');
  assert.deepEqual(code.chars, ['A', 'B', 'C', 'D', '1', '2', '3', '4']);
});

test('normalizeUserCode upper-cases and trims', () => {
  const code = normalizeUserCode('  abcd-1234 \n');
  assert.equal(code.dashed, 'ABCD-1234');
  assert.equal(code.bare, 'ABCD1234');
});

test('normalizeUserCode strips characters outside the code alphabet', () => {
  const code = normalizeUserCode('AB CD_1234!');
  assert.equal(code.bare, 'ABCD1234');
});

test('normalizeUserCode rejects unusable input', () => {
  assert.throws(() => normalizeUserCode(''), /empty/);
  assert.throws(() => normalizeUserCode('   '), /empty/);
  assert.throws(() => normalizeUserCode('---'), /no usable characters/);
  assert.throws(() => normalizeUserCode(1234), TypeError);
});

test('classifyLoginPage returns null for a healthy signed-in page', () => {
  assert.equal(classifyLoginPage({ url: 'https://github.com/settings/profile', text: 'Public profile' }), null);
  assert.equal(classifyLoginPage(), null);
});

test('classifyLoginPage detects bad credentials', () => {
  const got = classifyLoginPage({
    url: 'https://github.com/session',
    text: 'Incorrect username or password.',
  });
  assert.equal(got.errorClass, ErrorClass.INVALID_CREDENTIALS);
});

test('classifyLoginPage detects two-factor prompts', () => {
  assert.equal(
    classifyLoginPage({ url: 'https://github.com/sessions/two-factor' }).errorClass,
    ErrorClass.TWO_FACTOR_REQUIRED,
  );
  assert.equal(
    classifyLoginPage({ url: 'https://github.com/x', text: 'Two-factor authentication' }).errorClass,
    ErrorClass.TWO_FACTOR_REQUIRED,
  );
});

test('classifyLoginPage detects email device verification', () => {
  assert.equal(
    classifyLoginPage({ url: 'https://github.com/sessions/verified-device' }).errorClass,
    ErrorClass.DEVICE_VERIFICATION,
  );
  assert.equal(
    classifyLoginPage({ text: 'Device verification' }).errorClass,
    ErrorClass.DEVICE_VERIFICATION,
  );
});

test('device verification is classified ahead of two-factor', () => {
  // The device-verification page also mentions an authentication code, so the
  // more specific match has to win or operators get pointed at the wrong fix.
  const got = classifyLoginPage({
    url: 'https://github.com/sessions/verified-device',
    text: 'Device verification\nEnter the authentication code sent to your email.',
  });
  assert.equal(got.errorClass, ErrorClass.DEVICE_VERIFICATION);
});

test('classifyLoginPage detects human verification', () => {
  assert.equal(classifyLoginPage({ text: 'Unable to verify your request' }).errorClass, ErrorClass.CAPTCHA);
});

test('classifyDevicePage detects a rejected code', () => {
  for (const text of [
    'The code you entered is incorrect',
    'This code is invalid or has expired',
    'Your code has expired',
  ]) {
    assert.equal(classifyDevicePage({ text }).errorClass, ErrorClass.CODE_REJECTED, text);
  }
});

test('classifyDevicePage flags a bounce back to the sign-in page', () => {
  assert.equal(
    classifyDevicePage({ url: 'https://github.com/login?return_to=%2Flogin%2Fdevice' }).errorClass,
    ErrorClass.UNKNOWN,
  );
  assert.equal(classifyDevicePage({ url: 'https://github.com/login' }).errorClass, ErrorClass.UNKNOWN);
});

test('classifyDevicePage returns null on the authorization page', () => {
  assert.equal(
    classifyDevicePage({ url: 'https://github.com/login/device/confirm', text: 'Authorize GitHub Copilot' }),
    null,
  );
});

test('isDeviceActivated recognizes the confirmation page', () => {
  assert.ok(isDeviceActivated('Device activated'));
  assert.ok(isDeviceActivated("Congratulations, you're all set!"));
  assert.ok(!isDeviceActivated('Authorize GitHub Copilot'));
  assert.ok(!isDeviceActivated(''));
});

test('classifyThrownError separates timeouts from unknown failures', () => {
  const timeoutErr = new Error('locator.click: Timeout 30000ms exceeded.');
  assert.equal(classifyThrownError(timeoutErr).errorClass, ErrorClass.TIMEOUT);

  const named = new Error('boom');
  named.name = 'TimeoutError';
  assert.equal(classifyThrownError(named).errorClass, ErrorClass.TIMEOUT);

  assert.equal(classifyThrownError(new Error('net::ERR_CONNECTION_REFUSED')).errorClass, ErrorClass.UNKNOWN);
});

test('validateLoginRequest requires the credential fields', () => {
  assert.deepEqual(
    validateLoginRequest({
      account_id: 'alice',
      username: 'alice-gh',
      password: 's3cret',
      user_code: 'ABCD-1234',
    }),
    [],
  );

  const problems = validateLoginRequest({ account_id: 'alice' });
  assert.deepEqual(problems, ['username is required', 'password is required', 'user_code is required']);

  assert.deepEqual(validateLoginRequest(null), ['request body must be a JSON object']);
  assert.deepEqual(validateLoginRequest([]), ['request body must be a JSON object']);
});

test('validateLoginRequest rejects blank strings', () => {
  const problems = validateLoginRequest({
    account_id: '  ',
    username: 'alice-gh',
    password: 's3cret',
    user_code: 'ABCD-1234',
  });
  assert.deepEqual(problems, ['account_id is required']);
});

test('validateLoginRequest type-checks verification_uri', () => {
  const problems = validateLoginRequest({
    account_id: 'alice',
    username: 'alice-gh',
    password: 's3cret',
    user_code: 'ABCD-1234',
    verification_uri: 42,
  });
  assert.deepEqual(problems, ['verification_uri must be a string']);
});

test('profileDirName sanitizes path traversal and separators', () => {
  assert.equal(profileDirName('alice'), 'alice');
  assert.equal(profileDirName('team/alice'), 'team_alice');
  assert.equal(profileDirName('../../etc/passwd'), '.._.._etc_passwd');
  assert.equal(profileDirName('..'), '_');
  assert.equal(profileDirName('.'), '_');
  assert.equal(profileDirName(''), '_');
});

test('isSessionInterstitial detects the account confirmation card', () => {
  assert.equal(
    isSessionInterstitial('Device Activation\nSigned in as rubber-duck-fly\nContinue\nUse a different account'),
    true,
  );
  assert.equal(isSessionInterstitial('Signed in as octocat Continue'), true);
  assert.equal(isSessionInterstitial('USE A DIFFERENT ACCOUNT'), true);
});

test('isSessionInterstitial ignores the code form and success pages', () => {
  assert.equal(isSessionInterstitial('Enter the code displayed on your device\nContinue'), false);
  assert.equal(isSessionInterstitial('Device activated\nSigned in as octocat'), false);
  assert.equal(isSessionInterstitial(''), false);
  assert.equal(isSessionInterstitial(), false);
});
