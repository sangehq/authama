// An axe pass over every page the e2e specs drive, in the light and the dark
// theme. A serious or critical violation fails; minor and moderate ones are
// attached to the report so they are seen without blocking.
import AxeBuilder from '@axe-core/playwright';
import type { Page, TestInfo } from '@playwright/test';
import { test, expect, pages } from '../support/fixtures';

const tags = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'];
const blocking = new Set(['serious', 'critical']);

async function audit(page: Page, info: TestInfo, label: string): Promise<void> {
  const { violations } = await new AxeBuilder({ page }).withTags(tags).analyze();
  if (violations.length) {
    await info.attach(`axe ${label}`, { body: JSON.stringify(violations, null, 2), contentType: 'application/json' });
  }
  const failing = violations
    .filter((v) => blocking.has(v.impact ?? ''))
    .map((v) => `${v.impact} ${v.id}: ${v.help} -> ${v.nodes.map((n) => n.target.join(' ')).join(', ')}`);
  expect(failing, `serious or critical axe violations on ${label}`).toEqual([]);
}

for (const theme of ['light', 'dark'] as const) {
  test.describe(`${theme} theme`, () => {
    test.use({ colorScheme: theme });

    for (const { path } of pages) {
      test(`${path} has no serious or critical violations`, async ({ page }, info) => {
        await page.goto(`${path}${path.includes('?') ? '&' : '?'}theme=${theme}`);
        await expect(page.locator('html')).toHaveAttribute('data-bs-theme', theme);
        await audit(page, info, `${path} (${theme})`);
      });
    }

    test('a change preview has no serious or critical violations', async ({ page }, info) => {
      await page.goto(`/?theme=${theme}`);
      await page.getByRole('button', { name: 'Re-render ~/.ssh/config' }).click();
      await expect(page.getByRole('heading', { level: 1 })).toHaveText('Review the change');
      await expect(page.locator('html')).toHaveAttribute('data-bs-theme', theme);
      await audit(page, info, `change preview (${theme})`);
    });
  });
}
