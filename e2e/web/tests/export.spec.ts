// The Export page: choose the rows, filter them, choose the columns and the
// format, and the download is what the preview showed.
import { test, expect } from '../support/fixtures';

test('the Export page filters, chooses columns and formats, and downloads what it previews', async ({ page }) => {
  await page.goto('/export');
  // Export sits in the Backups & export menu, marked as the current page.
  await expect(page.locator('nav[aria-label="Main"] a[href="/export"]')).toHaveAttribute('aria-current', 'page');
  const preview = page.getByRole('region', { name: 'Preview of the first rows' });
  await expect(preview.locator('tbody tr').first()).toBeVisible();
  await expect(page.getByRole('tab', { name: 'Hosts' })).toHaveAttribute('aria-selected', 'true');
  const exp = page.locator('[data-export-page]');
  const download = exp.getByRole('link', { name: 'Download' });

  // Untick every profile: nothing to download, and it says why.
  const allProfiles = page.getByRole('checkbox', { name: 'All profiles' });
  await allProfiles.uncheck();
  await expect(download).toHaveAttribute('aria-disabled', 'true');
  await expect(page.getByText('Tick at least one of profiles.')).toBeVisible();
  await allProfiles.check();
  await expect(download).not.toHaveAttribute('aria-disabled', 'true');

  // Leave a column out: the preview loses it.
  const columns = await preview.locator('thead th').allTextContents();
  const last = columns[columns.length - 1].trim();
  await exp.getByRole('button', { name: `Remove ${last}` }).click();
  await expect(preview.locator('thead th')).toHaveCount(columns.length - 1);

  // JSON: the file holds the preview's rows, without the column left out.
  await exp.getByLabel('Format').selectOption('json');
  await expect(download).toHaveAttribute('href', /format=json/);
  const res = await page.request.get((await download.getAttribute('href')) ?? '');
  const rows = (await res.json()) as Record<string, unknown>[];
  expect(rows.length).toBe(await preview.locator('tbody tr').count());
  expect(Object.keys(rows[0])).not.toContain(last);

  // The choice is in the address, so a reload shows the same file.
  await page.reload();
  await expect(preview.locator('thead th')).toHaveCount(columns.length - 1);
  await expect(exp.getByLabel('Format')).toHaveValue('json');

  // Another kind of row, with its own default columns.
  await page.getByRole('tab', { name: 'Keys' }).click();
  await expect(page.getByRole('tab', { name: 'Keys' })).toHaveAttribute('aria-selected', 'true');
  await expect(preview.locator('thead th').first()).toHaveText(/key/i);
});

test('Export columns reorder by keyboard and by dragging', async ({ page }) => {
  await page.goto('/export');
  const exp = page.locator('[data-export-page]');
  const preview = exp.getByRole('region', { name: 'Preview of the first rows' });
  await expect(preview.locator('tbody tr').first()).toBeVisible();
  const first = ((await preview.locator('thead th').nth(0).textContent()) ?? '').trim();
  const second = ((await preview.locator('thead th').nth(1).textContent()) ?? '').trim();

  // Keyboard: the handle takes the arrow keys, and keeps the focus.
  const handle = exp.getByRole('button', { name: new RegExp(`^Reorder ${first}:`) });
  await handle.focus();
  await page.keyboard.press('ArrowDown');
  await expect(preview.locator('thead th').nth(0)).toHaveText(second);
  await expect(preview.locator('thead th').nth(1)).toHaveText(first);
  await expect(exp.getByRole('button', { name: new RegExp(`^Reorder ${first}:`) })).toBeFocused();

  // Drag: the row by its handle, onto the top row.
  const rows = exp.locator('[data-export-columns] tbody tr');
  await rows.nth(1).dragTo(rows.nth(0), { targetPosition: { x: 10, y: 2 } });
  await expect(preview.locator('thead th').nth(0)).toHaveText(first);
});

test('Export is in the Backups & export menu and the palette', async ({ page }) => {
  await page.goto('/');
  const nav = page.getByRole('navigation', { name: 'Main' });
  await nav.getByRole('button', { name: 'Backups & export' }).click();
  await nav.getByRole('link', { name: /^Export/ }).click();
  await expect(page).toHaveURL(/\/export/);
  await expect(page.getByRole('heading', { name: 'Export', level: 1 })).toBeVisible();
});

test('Saved selections: save, open, compare, and delete; the manifest goes through a preview', async ({ page }) => {
  await page.goto('/export');
  const exp = page.locator('[data-export-page]');
  const preview = exp.getByRole('region', { name: 'Preview of the first rows' });
  await expect(preview.locator('tbody tr').first()).toBeVisible();
  const saved = exp.getByRole('group', { name: 'Saved selections' });

  // Save every host as mine: it is listed at once.
  await saved.getByLabel('A name to save this selection as').fill('e2e-all');
  await saved.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(saved.getByRole('button', { name: 'e2e-all', exact: true })).toBeVisible();

  // Narrow the page, then compare it with the saved one: the hosts it left
  // out are only in e2e-all.
  await exp.getByLabel('Keep rows containing').fill('app');
  await expect(preview.locator('tbody tr')).toHaveCount(3);
  await saved.getByRole('button', { name: 'Compare with e2e-all' }).click();
  const cmp = exp.getByRole('region', { name: 'Comparison' });
  await expect(cmp).toBeVisible();
  await expect(cmp.getByText(/3 in both, 0 only here, \d+ only in e2e-all\./)).toBeVisible();
  await expect(cmp.getByRole('heading', { name: /^Only in e2e-all/ })).toBeVisible();
  await cmp.getByRole('button', { name: 'Close the comparison' }).click();
  await expect(cmp).toBeHidden();

  // Opening it brings every host back.
  await saved.getByRole('button', { name: 'e2e-all', exact: true }).click();
  await expect(exp.getByLabel('Keep rows containing')).toHaveValue('');
  await expect(preview.locator('tbody tr').nth(3)).toBeVisible();

  // Deleting it takes it off the list.
  await saved.getByRole('button', { name: 'Delete e2e-all (mine)' }).click();
  await expect(saved.getByRole('button', { name: 'e2e-all', exact: true })).toHaveCount(0);

  // The manifest's store is a change, previewed before anything is written.
  await saved.getByLabel('A name to save this selection as').fill('e2e-shared');
  await saved.getByLabel('Where to save it').selectOption('manifest');
  await saved.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(page).toHaveURL(/\/changes\/preview/);
  await expect(page.getByText('export selections save e2e-shared').first()).toBeVisible();
});

test('Add column groups related fields', async ({ page }) => {
  await page.goto('/export');
  const exp = page.locator('[data-export-page]');
  await expect(exp.getByRole('region', { name: 'Preview of the first rows' }).locator('tbody tr').first()).toBeVisible();
  const add = exp.getByLabel('A field to add as a column');
  await expect(add.locator('optgroup[label="This row"]')).toHaveCount(1);
  await expect(add.locator('optgroup[label="key (related)"] option').first()).toBeAttached();
});
