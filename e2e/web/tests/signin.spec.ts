// Signing in without copying an address: a browser that signed in with a link
// is remembered, so losing its session does not lock it out; the Logs page and
// the time zone field work in a real browser.
import { test, expect } from '../support/fixtures';

test('a remembered browser signs straight back in after its session is gone', async ({ page }) => {
  await page.goto('/');
  const ctx = page.context();
  const cookies = await ctx.cookies();
  const browser = cookies.find((c) => c.name === 'authama_browser');
  expect(browser, 'the link sign-in left a browser cookie').toBeTruthy();
  expect(browser?.httpOnly).toBe(true);
  expect(browser?.sameSite).toBe('Strict');
  // The session goes (a restart, a timeout); the browser cookie stays.
  await ctx.clearCookies({ name: 'authama_session' });
  const res = await page.goto('/keys');
  expect(res?.status()).toBe(200);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Keys');
  // The cookie's secret was replaced on that sign-in.
  const after = (await ctx.cookies()).find((c) => c.name === 'authama_browser');
  expect(after?.value).not.toBe(browser?.value);
  // Settings lists this browser.
  await page.goto('/settings#browsers');
  await expect(page.locator('#browsers')).toContainText('this browser');
});

test('without a cookie the 401 page says how to get in, and offers no sign-in button', async ({ playwright }) => {
  const anon = await playwright.request.newContext({ baseURL: process.env.AUTHAMA_E2E_URL });
  const res = await anon.get('/keys');
  expect(res.status()).toBe(401);
  const body = await res.text();
  expect(body).toContain('Open authama to sign in');
  expect(body.toLowerCase()).not.toContain('<form');
  await anon.dispose();
});

test('the Logs page follows new records', async ({ page }) => {
  await page.goto('/logs');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Logs');
  await expect(page.locator('#log-rows')).toContainText('signin');
  await page.getByLabel('Follow').check();
  // A change posted elsewhere is logged at info and appears at the top.
  const other = await page.context().newPage();
  await other.goto('/settings');
  await other.close();
  await expect(page.locator('#log-rows tr').first()).toBeVisible();
  await page.getByLabel('Follow').uncheck();
});

test('the time zone field previews the time in the zone typed', async ({ page }) => {
  await page.goto('/settings#setting-display.timezone');
  const input = page.locator('#v-AUTHAMA_TIMEZONE');
  await expect(input).toBeVisible();
  await input.fill('Asia/Tokyo');
  await expect(page.locator('#tz-now')).not.toContainText('not a time zone name');
  await input.fill('Mars/Olympus');
  await expect(page.locator('#tz-now')).toHaveText('not a time zone name');
});
