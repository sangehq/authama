// The revocation checklists and the key picker: every key and token has a
// checklist, a step authama cannot see is ticked by hand and kept, and the
// bulk key actions choose any mix of keys, profiles and categories.
import { test, expect } from '../support/fixtures';

test('the checklists list every key, and one opens beside the list and takes a tick', async ({ page }) => {
  await page.goto('/checklists');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Checklists');
  const first = page.locator('table tbody th a').first();
  const name = (await first.textContent())?.trim() ?? '';
  expect(name).toMatch(/^\w[\w.-]*\/\w/);

  // It opens in the side panel; the list stays where it was.
  await first.click();
  const panel = page.locator('[data-side-panel]');
  await expect(panel).toBeVisible();
  await expect(page).toHaveURL(/\/checklists$/);
  await expect(panel.getByRole('heading', { level: 1 })).toHaveText(name);

  // A step authama cannot see is ticked by hand, kept, and can be undone.
  const step = panel.locator('.checklist-step', { hasText: 'Every copy outside authama is known' });
  await step.getByRole('button', { name: 'Mark done' }).click();
  await expect(panel.locator('.checklist-step', { hasText: 'Every copy outside authama is known' })).toContainText('Ticked');
  await page.goto('/checklist?item=' + encodeURIComponent(name));
  const kept = page.locator('.checklist-step', { hasText: 'Every copy outside authama is known' });
  await expect(kept).toContainText('Ticked');
  await kept.getByRole('button', { name: 'Mark not done' }).click();
  await expect(page.locator('.checklist-step', { hasText: 'Every copy outside authama is known' }).getByRole('button', { name: 'Mark done' })).toBeVisible();

  // The filters narrow the list.
  await page.goto('/checklists?show=keys');
  await expect(page.locator('table tbody tr').first()).toContainText('SSH key');
});

test('a key page links its checklist, and a key row menu opens it', async ({ page }) => {
  await page.goto('/keys');
  await page.getByRole('button', { name: /^Actions for key / }).first().click();
  await page.locator('.dropdown-menu.show').getByRole('link', { name: 'Checklist' }).click();
  const panel = page.locator('[data-side-panel]');
  await expect(panel.locator('.checklist-steps').first()).toBeVisible();
});

test('the key picker chooses every key, whole profiles or single keys, and counts them', async ({ page }) => {
  await page.goto('/keys#rotate-keys');
  const form = page.locator('#rotate-keys');
  await expect(form).toBeVisible();
  const picker = form.locator('[data-key-picker]');
  const review = form.locator('[type="submit"]');
  await expect(picker.locator('[data-pick-count]')).toHaveText('No key chosen yet.');
  await expect(review).toBeDisabled();

  // Every key locks the rest and counts every key the action can act on.
  await picker.getByLabel('Every key in every profile').check();
  await expect(picker.locator('[data-pick-key]:not([data-pick-ineligible])').first()).toBeDisabled();
  await expect(picker.locator('[data-pick-count]')).toContainText('chosen');
  await expect(review).toBeEnabled();
  await picker.getByLabel('Every key in every profile').uncheck();
  await expect(review).toBeDisabled();

  // A whole profile ticks and locks its keys.
  const group = picker.locator('[data-pick-group]').first();
  await group.locator('[data-pick-profile]').check();
  for (const k of await group.locator('[data-pick-key]:not([data-pick-ineligible])').all()) {
    await expect(k).toBeChecked();
    await expect(k).toBeDisabled();
  }
  await group.locator('[data-pick-profile]').uncheck();

  // Where no key qualifies, the picker says so rather than "none chosen": in
  // this home every key is used by a host, so none can be deleted.
  await page.goto('/keys#delete-keys');
  const del = page.locator('#delete-keys [data-key-picker]');
  await expect(del.locator('.key-picker-key.is-ineligible').first()).toContainText('a host uses it');
  await expect(del.locator('[data-pick-count]')).toHaveText('No key here can be deleted; each says why.');
  await expect(page.locator('#delete-keys [type="submit"]')).toBeDisabled();
});

test('the key picker filters, clears, counts per profile and names what Review will act on', async ({ page }) => {
  await page.goto('/keys#rotate-keys');
  const form = page.locator('#rotate-keys');
  const picker = form.locator('[data-key-picker]');
  const review = form.locator('[data-pick-submit]');
  await expect(review).toHaveText(/^\s*Review\s*$/);

  // One key: the button and the summary name it, and its profile counts it.
  const group = picker.locator('[data-pick-group]').first();
  const key = group.locator('[data-pick-key]:not([data-pick-ineligible])').first();
  await key.check();
  await expect(review).toHaveText(/Review 1 key to rotate/);
  await expect(group.locator('[data-pick-group-count]')).toContainText('1 of');
  await expect(picker.locator('[data-pick-count]')).toContainText('1 of');

  // Every key hides the rest of the picker behind one sentence.
  await picker.getByLabel('Every key in every profile').check();
  await expect(review).toHaveText(/Review every key to rotate/);
  await expect(picker.locator('[data-pick-all-note]')).toBeVisible();
  await expect(picker.locator('[data-pick-rest]')).toBeHidden();

  // Clear gives everything back, "every key" included.
  await picker.getByLabel('Every key in every profile').uncheck();
  await picker.locator('[data-pick-clear]').click();
  await expect(key).not.toBeChecked();
  await expect(review).toBeDisabled();

  // The filter narrows the view, not the choice.
  const filter = picker.locator('[data-pick-filter]');
  if (await filter.isVisible()) {
    await filter.fill('zz-no-such-key');
    await expect(picker.locator('[data-pick-nomatch]')).toBeVisible();
    await filter.fill('');
    await expect(picker.locator('[data-pick-nomatch]')).toBeHidden();
  }
});

test('the Actions menu opens the rotate form from any page, and the Overview offers Get started', async ({ page }) => {
  await page.goto('/hosts');
  await page.getByRole('button', { name: 'Actions', exact: true }).click();
  await page.getByRole('link', { name: /Rotate keys/ }).click();
  await expect(page).toHaveURL(/\/keys#rotate-keys$/);
  await expect(page.locator('#rotate-keys')).toBeVisible();
  await page.locator('#rotate-keys').screenshot({ path: 'test-results/rotate-keys-panel.png' });

  await page.goto('/');
  const setup = page.locator('[data-setup-card]');
  if (await setup.count()) {
    await expect(setup.getByRole('heading', { name: 'Get started' })).toBeVisible();
    await setup.screenshot({ path: 'test-results/get-started.png' });
  }
});

test('the Reset page opens the rotate picker with every key already chosen', async ({ page }) => {
  await page.goto('/reset');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Reset');
  await page.screenshot({ path: 'test-results/reset-page.png', fullPage: true });
  await page.locator('a[href*="pick=all"]').click(); // Rotate every key's row
  await expect(page).toHaveURL(/\/keys\?pick=all#rotate-keys$/);
  const picker = page.locator('#rotate-keys [data-key-picker]');
  await expect(picker.getByLabel('Every key in every profile')).toBeChecked();
  await expect(page.locator('#rotate-keys [data-pick-submit]')).toHaveText(/Review every key to rotate/);
  // Only rotate takes the preset: revoking every key is never chosen for you.
  await expect(page.locator('#revoke-keys [data-pick-all]')).not.toBeChecked();

  await page.goto('/hosts');
  await page.getByRole('button', { name: 'Actions', exact: true }).click();
  await page.locator('header.navbar-expand-md:not(.navbar)').screenshot({ path: 'test-results/main-menu-actions.png' });
});
