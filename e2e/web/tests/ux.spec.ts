// The in-place conveniences: the side panel's navigation, the CSV dialog, and
// the JSON check on the tokens.json editor. Each is an enhancement: the page
// it sits on works without it, which pages.spec.ts covers.
import { test, expect } from '../support/fixtures';

test('a CSV opens a dialog: columns and rows are chosen, previewed, and the download matches', async ({ page }) => {
  await page.goto('/hosts');
  await page.getByRole('link', { name: 'CSV' }).first().click();
  const dialog = page.getByRole('dialog', { name: /Download CSV/ });
  await expect(dialog).toBeVisible();
  const preview = dialog.getByRole('region', { name: 'Preview of the first rows' });
  await expect(preview.locator('thead th').first()).toBeVisible();
  const columns = await preview.locator('thead th').allTextContents();
  expect(columns.length).toBeGreaterThan(1);

  // Leave a column out: the preview loses it and the download link says so.
  const dropped = columns[columns.length - 1];
  await dialog.getByRole('checkbox', { name: dropped, exact: true }).uncheck();
  await expect(preview.locator('thead th', { hasText: dropped })).toHaveCount(0);
  const download = dialog.getByRole('link', { name: 'Download' });
  await expect(download).toHaveAttribute('href', new RegExp(`drop=${encodeURIComponent(dropped)}`));

  // Leave rows out by text: the summary counts them.
  const firstCell = (await preview.locator('tbody tr').first().locator('td').first().textContent()) ?? '';
  await dialog.getByLabel('Leave out rows containing').fill(firstCell);
  await expect(dialog.getByRole('status')).toContainText('left out');
  await expect(download).toHaveAttribute('href', /exclude=/);

  // The file is what the preview showed.
  const res = await page.request.get((await download.getAttribute('href')) ?? '');
  const header = (await res.text()).split('\n')[0];
  expect(header).not.toContain(dropped);

  // Only some profiles: their rows alone, and the choice is in the link.
  await dialog.getByLabel('Leave out rows containing').fill('');
  await expect(download).not.toHaveAttribute('href', /exclude=/);
  const profiles = dialog.getByRole('group', { name: 'Profiles' });
  await expect(profiles).toBeVisible();
  const firstProfile = profiles.getByRole('checkbox').first();
  const label = ((await profiles.locator('label').first().textContent()) ?? '').replace(/ \(\d+\)$/, '');
  await firstProfile.check();
  await expect(download).toHaveAttribute('href', new RegExp(`profile=${encodeURIComponent(label)}`));
  const only = await page.request.get((await download.getAttribute('href')) ?? '');
  const rows = (await only.text()).trim().split('\n').slice(1);
  expect(rows.length).toBeGreaterThan(0);
  await profiles.getByRole('button', { name: 'All' }).click();
  await expect(firstProfile).not.toBeChecked();
  await expect(download).not.toHaveAttribute('href', /profile=/);

  // The choice is remembered for this list.
  await dialog.getByRole('button', { name: 'Cancel' }).click();
  await page.reload();
  await page.getByRole('link', { name: 'CSV' }).first().click();
  await expect(dialog.getByRole('checkbox', { name: dropped, exact: true })).not.toBeChecked();
  await dialog.getByRole('button', { name: 'Reset' }).click();
  await expect(dialog.getByRole('checkbox', { name: dropped, exact: true })).toBeChecked();
});

test('tokens.json is checked as it is typed, and cannot be sent while it is not JSON', async ({ page }) => {
  await page.goto('/tokens');
  await page.getByRole('button', { name: 'Edit tokens.json' }).click();
  const editor = page.getByRole('textbox', { name: 'tokens.json' });
  await expect(editor).toBeVisible();
  const good = await editor.inputValue();
  await editor.fill('{\n  "npm": {\n}');
  await expect(editor).toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByText(/Not valid JSON: line \d+, column \d+/)).toBeVisible();
  // The browser refuses to submit an invalid field: no panel opens.
  await page.getByRole('button', { name: 'Preview the change' }).click();
  await expect(page.locator('#side-panel.show')).toHaveCount(0);

  await editor.fill(good.replace(/\n\s*/g, ''));
  await expect(editor).toHaveAttribute('aria-invalid', 'false');
  await page.getByRole('button', { name: 'Format JSON' }).click();
  await expect(editor).toHaveValue(/\n  "/);
});

test('the side panel stacks pages, goes back, and closes with Escape', async ({ page }) => {
  await page.goto('/history');
  await page.locator('table a[data-inline]').first().click();
  const panel = page.locator('#side-panel');
  await expect(panel).toHaveClass(/show/);
  await expect(panel.getByRole('button', { name: 'Back' })).toBeHidden();
  // The diff is a table of numbered lines, never dark stripes in a pre.
  await expect(panel.locator('pre.diff')).toHaveCount(0);
  await expect(panel.getByRole('link', { name: 'Open as a page' })).toHaveAttribute('href', /\/history\/[^/]+$/);
  await page.keyboard.press('Escape');
  await expect(panel).not.toHaveClass(/show/);
  // Nothing changed, so the page under it was not reloaded.
  await expect(page).toHaveURL(/\/history$/);
});

test('an action explains itself on hover and keyboard focus, and to a screen reader', async ({ page }) => {
  await page.goto('/hosts');
  const add = page.getByRole('button', { name: 'Add a host' });
  await expect(add).toHaveAttribute('aria-description', /Its key is minted on the next reconcile/);
  await add.focus();
  await expect(page.getByRole('tooltip')).toContainText('Adds a host to a profile');
  // A row's destructive item explains that it previews first.
  await page.getByRole('button', { name: /^Actions for host / }).first().click();
  await expect(page.locator('.dropdown-menu.show .text-danger').first()).toHaveAttribute('data-tip', /Previews the delete first/);
});

test("a key's row actions open in the side panel, and the list stays where it was", async ({ page }) => {
  await page.goto('/keys');
  const row = page.locator('tbody tr').first();
  await row.getByRole('button', { name: /^Actions for key / }).click();
  await row.getByRole('link', { name: 'Open' }).click();
  const panel = page.locator('#side-panel');
  await expect(panel).toHaveClass(/show/);
  await expect(panel.getByRole('link', { name: 'Open as a page' })).toHaveAttribute('href', /\/keys\/[^/]+\/[^/]+$/);
  await expect(page).toHaveURL(/\/keys$/);
  await page.keyboard.press('Escape');
  await expect(panel).not.toHaveClass(/show/);
  await row.getByRole('button', { name: /^Actions for key / }).click();
  await row.getByRole('link', { name: 'History' }).click();
  await expect(panel).toHaveClass(/show/);
  await expect(panel.getByRole('link', { name: 'Open as a page' })).toHaveAttribute('href', /\/history\?key=/);
  await expect(page).toHaveURL(/\/keys$/);
});
