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
  await page.goto('/jobs/review?action=rotate-keys');
  const form = page.locator('#job-options');
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
  await page.goto('/jobs/review?action=delete-keys');
  const del = page.locator('#job-options [data-key-picker]');
  await expect(del.locator('.key-picker-key.is-ineligible').first()).toContainText('a host uses it');
  await expect(del.locator('[data-pick-count]')).toHaveText('No key here can be deleted; each says why.');
  await expect(page.locator('#job-options [type="submit"]')).toBeDisabled();
});

test('the key picker filters, clears, counts per profile and names what Review will act on', async ({ page }) => {
  await page.goto('/jobs/review?action=rotate-keys');
  const form = page.locator('#job-options');
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
  // Its keys are chosen on the rotation's review page, not in a slide-in.
  await expect(page).toHaveURL(/\/jobs\/review\?action=rotate-keys$/);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Rotate keys');
  await expect(page.locator('#job-options [data-key-picker]')).toBeVisible();
  await page.screenshot({ path: 'test-results/rotate-keys-page.png', fullPage: true });
  // An older link to the slide-in lands there too.
  await page.goto('/keys#revoke-keys');
  await expect(page).toHaveURL(/\/jobs\/review\?action=revoke-keys$/);

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
  await page.locator('a[href*="all=1"]').click(); // Rotate every key's row
  await expect(page).toHaveURL(/\/jobs\/review\?action=rotate-keys&all=1$/);
  const picker = page.locator('#job-options [data-key-picker]');
  await expect(picker.getByLabel('Every key in every profile')).toBeChecked();
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(/^Rotate \d+ keys?$/);
  // Only rotate takes the preset: revoking every key is never chosen for you.
  await page.goto('/jobs/review?action=revoke-keys');
  await expect(page.locator('#job-options [data-pick-all]')).not.toBeChecked();

  await page.goto('/hosts');
  await page.getByRole('button', { name: 'Actions', exact: true }).click();
  await page.locator('header.navbar-expand-md:not(.navbar)').screenshot({ path: 'test-results/main-menu-actions.png' });
});

test('the token audit counts its findings and offers each tool with no file', async ({ page }) => {
  await page.goto('/tokens');
  const audit = page.locator('#audit').locator('..');
  await expect(audit.getByRole('heading', { name: 'Audit' })).toBeVisible();
  const absent = audit.locator('details.audit-absent');
  if (await absent.count()) {
    await absent.locator('summary').click();
    const chip = absent.locator('[data-panel-open="add-token"]').first();
    await expect(chip).toBeVisible();
    await chip.click();
    await expect(page.locator('#add-token')).toBeVisible();
  }
  await audit.screenshot({ path: 'test-results/token-audit.png' });
});

test('a preview asked for again by its address says why, instead of Not found', async ({ page }) => {
  const res = await page.goto('/changes/preview');
  expect(res?.status()).toBe(410);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('This page answered a form');
  await expect(page.getByRole('link', { name: 'See the history' })).toBeVisible();
});

test('a setting is edited from its page, through a preview', async ({ page }) => {
  await page.goto('/settings#setting-git.mode');
  const row = page.locator('[id="setting-git.mode"]');
  // Arriving at #setting-<key> opens that setting's form.
  const select = row.locator('select[name="value"]');
  await expect(select).toBeVisible();
  await select.selectOption('https');
  await row.getByRole('button', { name: 'Preview' }).click();
  const panel = page.locator('#side-panel');
  await expect(panel.getByRole('heading', { level: 1 })).toHaveText('Review the change');
  await panel.getByRole('button', { name: 'Make this change' }).click();
  await expect(panel.getByRole('heading', { level: 1 })).toHaveText('Done');

  await page.goto('/settings');
  const saved = page.locator('[id="setting-git.mode"]');
  await expect(saved.locator('.setting-value')).toContainText('https');
  await expect(saved.locator('.badge', { hasText: 'authama.json' })).toBeVisible();
  // The filter narrows the page.
  await page.locator('[data-settings-filter]').fill('git.mode');
  await expect(page.locator('[id="settings-web"]')).toBeHidden();
  await page.locator('[data-settings-filter]').fill('');
  await page.screenshot({ path: 'test-results/settings-page.png', fullPage: true });
});

test('a rotation is reviewed on its own page, forced from its options, and dry-run as a tree', async ({ page }) => {
  test.setTimeout(120_000);
  // The keys are chosen on the review page itself.
  await page.goto('/jobs/review?action=rotate-keys');
  const form = page.locator('#job-options');
  await expect(page.getByText('Choose keys above')).toBeVisible();
  await form.getByLabel('Every key in every profile').check();
  await form.locator('[data-pick-submit]').click();
  await expect(page).toHaveURL(/\/jobs\/review\?.*all=1/);
  // The steps follow the options: a clean rotation writes no backup.
  const steps = page.locator('.job-steps');
  await expect(steps).toContainText('encrypted backup');
  await page.getByLabel('Skip the encrypted backup').check();
  await expect(steps).not.toContainText('writes an encrypted backup');
  await page.getByLabel('Skip the encrypted backup').uncheck();
  await expect(steps).toContainText('encrypted backup');

  // The review is a page of its own, not a slide-in panel.
  await expect(page.locator('#side-panel')).toBeHidden();
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(/^Rotate \d+ keys?$/);
  await page.screenshot({ path: 'test-results/job-review-page.png', fullPage: true });

  // Force reviews it again, marked dangerous; the confirm says so.
  await page.getByLabel('Force: replace what cannot rotate in place').check();
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(/\(forced\)$/);
  await expect(page.getByRole('alert').filter({ hasText: 'can lock you out' })).toBeVisible();
  await expect(page.locator('.command-card')).toContainText('--force');
  // Its Start button is red, and the dialog it opens says what forcing does.
  await expect(page.getByRole('button', { name: /^Start: Rotate .*\(forced\)…$/ })).toHaveClass(/btn-danger/);
  await expect(page.locator('#m-job')).toContainText('until you deploy it');
  await expect(page.locator('#m-job').getByRole('button', { name: 'Yes, force it', includeHidden: true })).toHaveCount(1);
  await page.getByLabel('Force: replace what cannot rotate in place').uncheck();
  await expect(page.getByRole('heading', { level: 1 })).not.toHaveText(/forced/);

  // The dry run opens the job as a page too.
  await page.getByRole('button', { name: 'Dry run: test every key' }).click();
  await expect(page).toHaveURL(/\/jobs\/(?!review$)[^/]+$/);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(/^Dry run: rotate \d+ keys?$/);
  await expect(page.locator('#job-state')).toHaveAttribute('data-state', /done|failed/, { timeout: 90_000 });
  // The run ends in its summary, as the Output: keys grouped by owner under
  // one trunk, a coloured result for each, the notes once, what is left.
  const summary = page.locator('#job-summary');
  await expect(summary).toBeVisible();
  await expect(summary.locator('.sum-cmd')).toContainText('authama ssh rotate --all --check');
  await expect(summary.locator('tr.is-root')).toHaveText('rotate');
  await expect(summary.locator('.sum-head')).toContainText(/of \d+ keys? · \d+ owners?/);
  await summary.screenshot({ path: 'test-results/rotation-summary.png' });
  // Every line the job printed is still there, folded under Full log.
  await page.locator('.job-full-log > summary').click();
  const tree = page.locator('#job-log');
  await expect(tree.locator('.console-line.lvl-0').first()).toBeVisible();
  await expect(tree.locator('.console-branch').first()).toHaveText(/[├└]─/);
  await page.screenshot({ path: 'test-results/job-page.png', fullPage: true });
  // A key that would fail leaves a next step: review it again, with Force.
  const next = page.locator('#job-next');
  await expect(next).toBeVisible();
  await next.getByRole('button', { name: 'Review again with Force' }).click();
  await expect(page).toHaveURL(/\/jobs\/review$/);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(/\(forced\)$/);
  await expect(page.getByLabel('Force: replace what cannot rotate in place')).toBeChecked();
});
