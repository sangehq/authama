// Tools, services, and structure classify / approve / rules / drag, each
// checked with axe. The flows share the home, so they run in order, and they
// leave the stray key the other structure spec uses.
import AxeBuilder from '@axe-core/playwright';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { test, expect } from '../support/fixtures';

test.describe.configure({ mode: 'serial' });

const tags = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'];

async function audit(page: Page, info: TestInfo, label: string): Promise<void> {
  const { violations } = await new AxeBuilder({ page }).withTags(tags).analyze();
  if (violations.length) {
    await info.attach(`axe ${label}`, { body: JSON.stringify(violations, null, 2), contentType: 'application/json' });
  }
  const failing = violations
    .filter((v) => v.impact === 'serious' || v.impact === 'critical')
    .map((v) => `${v.impact} ${v.id}: ${v.help} -> ${v.nodes.map((n) => n.target.join(' ')).join('; ')}`);
  expect(failing, `serious or critical axe violations on ${label}`).toEqual([]);
}

const panel = (page: Page): Locator => page.locator('#side-panel');

async function confirm(page: Page): Promise<void> {
  const p = panel(page);
  const destroy = p.getByRole('button', { name: 'Make this change…' });
  if (await destroy.count()) {
    await destroy.click();
    await page.getByRole('button', { name: 'Yes, make it' }).click();
  } else {
    await p.getByRole('button', { name: 'Make this change' }).click();
  }
  await expect(p.getByRole('heading', { level: 1 })).toHaveText('Done');
}

async function preview(page: Page, form: Locator, button?: string): Promise<void> {
  await form.getByRole('button', { name: button ?? /Preview/ }).first().click();
  await expect(panel(page).getByRole('heading', { level: 1 })).toHaveText('Review the change');
}

test('the tools page shows status and is accessible', async ({ page }, info) => {
  await page.goto('/tools');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Tools');
  await expect(page.locator('[data-tool-status]')).toContainText(/tool key\(s\) declared/);
  await audit(page, info, 'tools');
});

test('the services page adds, edits and removes a catalog entry, with axe', async ({ page }, info) => {
  await page.goto('/services');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Services');
  await expect(page.locator('#service-health')).toBeVisible();
  await audit(page, info, 'services');

  const add = page.locator('form', { has: page.locator('input[name="action"][value="service-add"]') });
  await add.getByLabel('Name').fill('e2e-access');
  await add.getByLabel('Kind').selectOption('cloudflare-access');
  await add.getByLabel('Label (optional)').fill('E2E Access');
  await preview(page, add);
  await confirm(page);

  await page.goto('/services');
  await expect(page.locator('.list-group-item', { hasText: 'e2e-access' })).toBeVisible();

  const edit = page.locator('#service-edit-form');
  await edit.getByLabel('Name').fill('e2e-access');
  await edit.getByLabel('Kind').selectOption('cloudflare-access');
  await edit.getByLabel('Label (optional)').fill('E2E Access updated');
  await preview(page, edit, 'Preview edit');
  await confirm(page);

  await page.goto('/services');
  const row = page.locator('.list-group-item', { hasText: 'e2e-access' });
  await expect(row).toContainText('E2E Access updated');
  await row.getByRole('button', { name: 'Remove' }).click();
  await expect(panel(page).getByRole('heading', { level: 1 })).toHaveText('Review the change');
  await confirm(page);
  await page.goto('/services');
  await expect(page.locator('.list-group-item', { hasText: 'e2e-access' })).toHaveCount(0);
});

test('structure classifies a purpose key, remembers a rule, previews approve, and drags', async ({ page }, info) => {
  await page.setViewportSize({ width: 1280, height: 1600 });
  await page.goto('/structure');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Structure');
  const watch = page.locator('#structure-watch');
  await expect(watch).toHaveAttribute('data-watch-mode', /watching|polling/);
  await expect(watch.getByRole('heading', { level: 2 })).toHaveText(/Watching|Polling/);
  await audit(page, info, 'structure');

  const suggestion = page.locator('li.structure-key[id^="suggestion-"]', { hasText: 'git-signing' }).filter({ hasText: /purpose/ }).first();
  await expect(suggestion).toBeVisible();
  await expect(suggestion).toContainText(/purpose|git/);
  await suggestion.getByRole('button', { name: 'Remember this as a rule' }).click();
  await expect(panel(page).getByRole('heading', { level: 1 })).toHaveText('Review the change');
  await confirm(page);

  await page.goto('/structure');
  await expect(page.locator('#structure-rules')).toContainText('git');
  const listed = page.locator('li.structure-key[id^="suggestion-"]', { hasText: 'git-signing' }).filter({ hasText: /purpose/ }).first();
  await expect(listed).toBeVisible();

  const menu = listed.getByRole('combobox').first();
  const options = await menu.locator('option').allTextContents();
  const purpose = options.find((o) => /tools\/git|purpose|git/i.test(o));
  expect(purpose, `a purpose move in ${options.join(', ')}`).toBeTruthy();
  await menu.selectOption({ label: purpose! });
  const pending = page.locator('[data-structure-pending]');
  await expect(pending).toContainText(/git-signing/);
  await page.getByRole('button', { name: 'Preview the moves' }).click();
  await expect(panel(page).getByRole('heading', { level: 1 })).toHaveText('Review the change');
  await expect(panel(page)).toContainText(/tools\/git|git-signing/);
  await panel(page).getByRole('button', { name: 'Cancel' }).click();

  const toolDrop = page.locator('li[data-drop^="tool:"][data-drop$=":git"]').first();
  if (await toolDrop.count()) {
    await listed.dragTo(toolDrop, { sourcePosition: { x: 4, y: 4 } });
    await expect(pending).toContainText(/git/);
  }
  await audit(page, info, 'structure after classify');
});
