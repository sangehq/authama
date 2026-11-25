// Two click-throughs the e2e plan (2026-11-16) listed as missing: signing out
// from the account menu on any page, and working the header's dropdowns from
// the keyboard alone.
import { test, expect } from '../support/fixtures';

for (const path of ['/', '/keys', '/settings', '/doctor']) {
  test(`signing out from the account menu on ${path} ends the session`, async ({ page }) => {
    await page.goto(path);
    const ctx = page.context();
    const session = (await ctx.cookies()).find((c) => c.name === 'authama_session');
    expect(session, 'signed in before signing out').toBeTruthy();

    await page.locator('#navbar-user').click();
    await page.getByRole('button', { name: 'Sign out' }).click();
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Signed out');

    const names = (await ctx.cookies()).map((c) => c.name);
    expect(names, 'the session cookie is cleared').not.toContain('authama_session');
    expect(names, 'the remembered-browser cookie is cleared').not.toContain('authama_browser');

    // The session ended on the server, not only in this browser: the old
    // cookie, sent again, is refused.
    await ctx.addCookies([session!]);
    const res = await page.goto('/keys');
    expect(res?.status(), 'a signed-out session is refused').toBe(401);
  });
}

// Each header menu opens with Enter, the arrow keys walk its items, and Escape
// closes it and gives focus back to the button that opened it.
for (const toggle of ['#navbar-new', '#navbar-transfer', '#navbar-user', '#navbar-ssh']) {
  test(`the ${toggle} menu works from the keyboard`, async ({ page }) => {
    await page.goto('/');
    const button = page.locator(toggle);
    await button.focus();
    await page.keyboard.press('Enter');
    await expect(button).toHaveAttribute('aria-expanded', 'true');
    const menu = page.locator(`${toggle} + .dropdown-menu, ${toggle} ~ .dropdown-menu`).first();
    await expect(menu).toBeVisible();

    const items = menu.locator('.dropdown-item:visible');
    const count = await items.count();
    expect(count, `${toggle} has items`).toBeGreaterThan(0);
    const inMenu = () => menu.evaluate((m) => m.contains(document.activeElement) && document.activeElement !== m);

    await page.keyboard.press('ArrowDown');
    await expect.poll(inMenu, 'ArrowDown moves focus into the menu').toBe(true);
    const first = await page.evaluate(() => document.activeElement?.textContent?.trim());
    if (count > 1) {
      await page.keyboard.press('ArrowDown');
      const second = await page.evaluate(() => document.activeElement?.textContent?.trim());
      expect(second, 'a second ArrowDown moves to the next item').not.toBe(first);
      await page.keyboard.press('ArrowUp');
      await expect.poll(() => page.evaluate(() => document.activeElement?.textContent?.trim()), 'ArrowUp moves back').toBe(first);
    }

    await page.keyboard.press('Escape');
    await expect(button).toHaveAttribute('aria-expanded', 'false');
    await expect(menu).toBeHidden();
    await expect(button, 'Escape gives focus back to the menu button').toBeFocused();
  });
}
