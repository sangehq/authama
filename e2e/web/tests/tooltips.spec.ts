// Tooltips are for icon-only controls, one at a time, and get out of the way:
// nothing that shows its own text, no nav item and no dropdown toggle has one
// (or a native title, which the browser shows as one), and an open menu,
// panel or dialog, a click, Escape or a scroll puts any away.
import { test, expect } from '../support/fixtures';
import { pages } from '../support/fixtures';

test('no labelled control, nav item or dropdown toggle carries a tooltip or a title, on any page', async ({ page }) => {
  let checked = 0;
  for (const { path } of pages) {
    await page.goto(path);
    const bad = await page.evaluate(() => {
      const shows = (el: Element) => {
        const walk = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
        for (let n = walk.nextNode(); n; n = walk.nextNode()) {
          const p = n.parentElement;
          if (n.textContent?.trim() && p && !p.closest('.visually-hidden, [hidden], kbd') && p.getClientRects().length > 0) {
            return true;
          }
        }
        return false;
      };
      const out: string[] = [];
      const marked = document.querySelectorAll('[data-tip], [data-bs-toggle="tooltip"], [title]');
      for (const el of marked) {
        if (el.closest('head, svg') || el.matches('[data-bs-toggle="popover"]')) {
          continue;
        }
        const name = `${el.tagName.toLowerCase()}.${[...el.classList].join('.')} "${(el.textContent ?? '').trim().slice(0, 30)}"`;
        if (el.matches('[data-bs-toggle="dropdown"], .dropdown-item, .navbar .nav-link:not(.px-0)')) {
          out.push(`menu or toggle: ${name}`);
        } else if (shows(el)) {
          out.push(`shows text: ${name}`);
        } else if (el.hasAttribute('title') && el.hasAttribute('data-tip')) {
          out.push(`title beside a tooltip: ${name}`);
        }
      }
      return { out, n: marked.length };
    });
    checked += bad.n;
    expect(bad.out, path).toEqual([]);
  }
  expect(checked, 'elements with a tooltip or title across every page').toBeGreaterThan(pages.length);
});

test('an icon-only control shows its tooltip after a pause, one at a time, and a click puts it away', async ({ page }) => {
  await page.goto('/keys/work/work_hpc-ed25519');
  const copy = page.getByRole('button', { name: 'Copy the fingerprint' });
  await copy.hover();
  await expect(page.getByRole('tooltip')).toHaveText('Copy the fingerprint');
  // Moving to another icon-only control swaps the tooltip; there is never two.
  const other = page.getByRole('button', { name: /^Copy the identity file/ });
  await other.hover();
  await expect(page.getByRole('tooltip')).toHaveCount(1);
  await expect(page.getByRole('tooltip')).toContainText('identity file');
  await page.mouse.click(5, 5);
  await expect(page.getByRole('tooltip')).toHaveCount(0);
  // Escape does too.
  await copy.focus();
  await expect(page.getByRole('tooltip')).toHaveText('Copy the fingerprint');
  await page.keyboard.press('Escape');
  await expect(page.getByRole('tooltip')).toHaveCount(0);
});

test('nav menus show each page\'s description in the menu, with no tooltip over it', async ({ page }) => {
  await page.goto('/');
  const toggle = page.locator('#navbar-activity');
  await toggle.hover();
  await page.waitForTimeout(800);
  await expect(page.getByRole('tooltip')).toHaveCount(0);
  await toggle.click();
  const menu = page.locator('[aria-labelledby="navbar-activity"]');
  await expect(menu).toBeVisible();
  await expect(menu.getByRole('link', { name: /History/ })).toContainText('Every change made to this home');
  // While a menu is open, hovering an icon-only control shows nothing.
  await page.getByRole('link', { name: 'Dark theme' }).or(page.getByRole('link', { name: 'Light theme' })).first().hover();
  await page.waitForTimeout(800);
  await expect(page.getByRole('tooltip')).toHaveCount(0);
});

test.describe('on a touch screen', () => {
  test.use({ hasTouch: true, isMobile: true, viewport: { width: 375, height: 812 } });

  test('no tooltip is ever made', async ({ page }) => {
    await page.goto('/keys/work/work_hpc-ed25519');
    await page.getByRole('button', { name: 'Copy the fingerprint' }).focus();
    await page.waitForTimeout(800);
    await expect(page.getByRole('tooltip')).toHaveCount(0);
  });
});
