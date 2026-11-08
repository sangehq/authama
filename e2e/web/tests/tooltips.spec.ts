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
  // Keyboard focus shows it too, and Escape puts it away.
  await copy.focus();
  await page.keyboard.press('Shift+Tab');
  await page.keyboard.press('Tab');
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

// Where a tooltip opens depends on the room around its control: it never
// covers the control, never runs off the window, and a control at the top of
// the window (the header's theme switch) gets its tooltip below.
test('a tooltip opens where there is room, clear of its control and inside the window', async ({ page }) => {
  const check = async (control: import('@playwright/test').Locator, side?: 'below') => {
    await control.hover();
    const tip = page.getByRole('tooltip');
    await expect(tip).toBeVisible();
    const t = (await tip.boundingBox())!;
    const c = (await control.boundingBox())!;
    const vp = page.viewportSize()!;
    const overlaps = t.x < c.x + c.width && c.x < t.x + t.width && t.y < c.y + c.height && c.y < t.y + t.height;
    expect(overlaps, 'the tooltip covers its control').toBe(false);
    expect(t.x >= 0 && t.y >= 0 && t.x + t.width <= vp.width && t.y + t.height <= vp.height, 'the tooltip is inside the window').toBe(true);
    if (side === 'below') {
      expect(t.y, 'opens below a control at the top of the window').toBeGreaterThanOrEqual(c.y + c.height);
    }
    await page.mouse.click(1, vp.height - 1);
    await expect(tip).toHaveCount(0);
  };
  await page.goto('/keys/work/work_hpc-ed25519');
  await check(page.getByRole('link', { name: 'Dark theme' }).or(page.getByRole('link', { name: 'Light theme' })).first(), 'below');
  await check(page.getByRole('button', { name: 'Copy the fingerprint' }));
  // At the right edge of a narrow window it still fits.
  await page.setViewportSize({ width: 420, height: 800 });
  await page.goto('/keys/work/work_hpc-ed25519');
  await check(page.getByRole('button', { name: 'Copy the fingerprint' }));
});

// A tooltip never outlives the reason for it: the pointer leaving, the control
// leaving the page, or a click (which focuses without explaining).
test('a tooltip goes when the pointer leaves, when its control is removed, and a click shows none', async ({ page }) => {
  await page.goto('/keys/work/work_hpc-ed25519');
  const copy = page.getByRole('button', { name: 'Copy the fingerprint' });
  const tip = page.getByRole('tooltip');

  await copy.hover();
  await expect(tip).toBeVisible();
  await page.mouse.move(5, 400);
  await expect(tip).toHaveCount(0);

  // The control removed while its tooltip shows: the watchdog takes it away.
  await copy.hover();
  await expect(tip).toBeVisible();
  await copy.evaluate((el) => el.remove());
  await expect(tip).toHaveCount(0, { timeout: 2000 });

  // A click focuses the control; with the pointer then moved off, nothing
  // stays on screen.
  await page.reload();
  const again = page.getByRole('button', { name: 'Copy the fingerprint' });
  await again.click();
  await page.mouse.move(5, 400);
  await page.waitForTimeout(800);
  await expect(tip).toHaveCount(0);
});
