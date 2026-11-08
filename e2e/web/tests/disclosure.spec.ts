// What a reader can see opens: every accordion shows a chevron that flips when
// it opens, and on a narrow screen the Export page's filters slide in from a
// button instead of stacking above the file.
import { test, expect } from '../support/fixtures';

test('every accordion button shows a chevron, and it turns when the item opens', async ({ page }) => {
  for (const path of ['/export', '/about']) {
    await page.goto(path);
    const buttons = page.locator('.accordion-button');
    await expect(buttons.first()).toBeVisible();
    const count = await buttons.count();
    expect(count, `${path} has accordions`).toBeGreaterThan(0);
    for (let i = 0; i < count; i++) {
      const toggle = buttons.nth(i).locator('.accordion-button-toggle');
      await expect(toggle, `${path} accordion ${i} has a chevron`).toHaveCount(1);
      const box = await toggle.boundingBox();
      expect(box && box.width > 0 && box.height > 0, `${path} accordion ${i}'s chevron has a size`).toBeTruthy();
    }
  }
  // Opening a collapsed one turns its chevron (Tabler rotates the toggle).
  await page.goto('/about');
  const first = page.locator('.accordion-button').first();
  const toggle = first.locator('.accordion-button-toggle');
  const before = await toggle.evaluate((el) => getComputedStyle(el).transform);
  await first.click();
  await expect(first).toHaveAttribute('aria-expanded', 'true');
  await expect.poll(() => toggle.evaluate((el) => getComputedStyle(el).transform)).not.toBe(before);
});

test.describe('on a phone', () => {
  test.use({ viewport: { width: 375, height: 812 } });

  test('the export filters slide in from a button, and the row kinds scroll in one line', async ({ page }) => {
    await page.goto('/export');
    const filters = page.locator('#export-filters');
    await expect(filters).toBeHidden();
    // The row kinds stay on one line instead of wrapping.
    const tabs = page.locator('.export-grains');
    const tops = await tabs.locator('.nav-link').evaluateAll((els) => new Set(els.map((e) => Math.round(e.getBoundingClientRect().top))).size);
    expect(tops, 'every row kind on one line').toBe(1);

    await page.getByRole('button', { name: 'Filters' }).click();
    await expect(filters).toBeVisible();
    await expect(filters.getByRole('heading', { name: 'Filters' })).toBeVisible();
    await expect(filters.locator('.accordion-button').first()).toBeVisible();
    await filters.getByRole('button', { name: 'Close' }).click();
    await expect(filters).toBeHidden();
  });
});

test.describe('on a wide screen', () => {
  test.use({ viewport: { width: 1440, height: 900 } });

  test('the export filters sit beside the file with no Filters button', async ({ page }) => {
    await page.goto('/export');
    await expect(page.locator('#export-filters')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Filters' })).toBeHidden();
  });
});
