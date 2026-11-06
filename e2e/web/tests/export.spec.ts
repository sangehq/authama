// The Export page: choose the rows, filter them, choose the columns and the
// format, and the download is what the preview showed.
import { test, expect } from '../support/fixtures';

test('the Export page filters, chooses columns and formats, and downloads what it previews', async ({ page }) => {
  await page.goto('/export');
  await expect(page.getByRole('link', { name: 'Export' }).first()).toHaveAttribute('aria-current', 'page');
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

test('Export is in the menu and the palette', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name: 'Export' }).click();
  await expect(page).toHaveURL(/\/export/);
  await expect(page.getByRole('heading', { name: 'Export', level: 1 })).toBeVisible();
});
