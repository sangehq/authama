// The in-place conveniences: the side panel's navigation, the CSV dialog, and
// the JSON check on the tokens.json editor. Each is an enhancement: the page
// it sits on works without it, which pages.spec.ts covers.
import { test, expect } from '../support/fixtures';

test('a CSV opens a dialog: profiles, keys, columns and rows are chosen in tables, previewed, and the download matches', async ({ page }) => {
  await page.goto('/hosts');
  await page.getByRole('link', { name: 'CSV' }).first().click();
  const dialog = page.getByRole('dialog', { name: /Download CSV/ });
  await expect(dialog).toBeVisible();
  const preview = dialog.getByRole('region', { name: 'Preview of the first rows' });
  await expect(preview.locator('thead th').first()).toBeVisible();
  const columns = await preview.locator('thead th').allTextContents();
  expect(columns.length).toBeGreaterThan(1);
  const download = dialog.getByRole('link', { name: 'Download' });
  const panel = dialog.getByRole('tabpanel');

  // Leave a column out: the preview loses it, the tab counts it, and the
  // download link says so.
  await dialog.getByRole('tab', { name: /Columns/ }).click();
  const dropped = columns[columns.length - 1];
  await panel.getByRole('checkbox', { name: dropped, exact: true }).uncheck();
  await expect(preview.locator('thead th', { hasText: dropped })).toHaveCount(0);
  await expect(dialog.getByRole('tab', { name: /Columns/ })).toContainText(`${columns.length - 1}/${columns.length}`);
  await expect(download).toHaveAttribute('href', new RegExp(`drop=${encodeURIComponent(dropped)}`));
  // The header box is now partly ticked.
  await expect(panel.getByRole('checkbox', { name: 'All columns' })).not.toBeChecked();

  // Leave rows out by text: the summary counts them, with a badge to undo it.
  const firstCell = (await preview.locator('tbody tr').first().locator('td').first().textContent()) ?? '';
  await dialog.getByLabel('Leave out rows containing').fill(firstCell);
  await expect(dialog.getByRole('status')).toContainText('containing');
  await expect(download).toHaveAttribute('href', /exclude=/);

  // The file is what the preview showed.
  const res = await page.request.get((await download.getAttribute('href')) ?? '');
  const header = (await res.text()).split('\n')[0];
  expect(header).not.toContain(dropped);
  await dialog.getByRole('button', { name: 'Stop leaving these out' }).click();
  await expect(download).not.toHaveAttribute('href', /exclude=/);

  // Every profile is ticked to start with. Unticking all of them blocks the
  // download and says why; ticking one gives its rows alone.
  await dialog.getByRole('tab', { name: /Profiles/ }).click();
  const all = panel.getByRole('checkbox', { name: 'All profiles' });
  await expect(all).toBeChecked();
  await all.uncheck();
  await expect(download).toHaveAttribute('aria-disabled', 'true');
  await expect(dialog.getByText('Tick at least one profile.').first()).toBeVisible();
  const firstProfile = panel.locator('tbody tr[data-csv-value]').first();
  const name = (await firstProfile.getAttribute('data-csv-value')) ?? '';
  await firstProfile.getByRole('checkbox').check();
  await expect(download).toHaveAttribute('href', new RegExp(`profile=${encodeURIComponent(name)}`));
  const only = await page.request.get((await download.getAttribute('href')) ?? '');
  expect((await only.text()).trim().split('\n').slice(1).length).toBeGreaterThan(0);

  // Within it, one key: the Keys table shows that profile's keys. A home
  // with no keys minted yet has no Keys tab; the Go test covers ?key= then.
  const keysTab = dialog.getByRole('tab', { name: /Keys/ });
  if (await keysTab.isVisible()) {
    await keysTab.click();
    await panel.getByRole('checkbox', { name: 'All keys' }).uncheck();
    await expect(download).toHaveAttribute('aria-disabled', 'true');
    await panel.locator('tbody tr[data-csv-value] input').first().check();
    await expect(download).not.toHaveAttribute('aria-disabled', 'true');
    // One key ticked of several keeps its rows; the only key is every key.
    if ((await panel.locator('tbody tr[data-csv-value]').count()) > 1) {
      await expect(download).toHaveAttribute('href', /key=/);
    }
    await panel.getByRole('checkbox', { name: 'All keys' }).check();
    await expect(download).not.toHaveAttribute('href', /key=/);
  }
  await dialog.getByRole('tab', { name: /Profiles/ }).click();
  await all.check();
  await expect(download).not.toHaveAttribute('href', /profile=/);

  // The choice is remembered for this list.
  await dialog.getByRole('button', { name: 'Cancel' }).click();
  await page.reload();
  await page.getByRole('link', { name: 'CSV' }).first().click();
  await dialog.getByRole('tab', { name: /Columns/ }).click();
  await expect(panel.getByRole('checkbox', { name: dropped, exact: true })).not.toBeChecked();
  await dialog.getByRole('button', { name: 'Reset' }).click();
  await expect(panel.getByRole('checkbox', { name: dropped, exact: true })).toBeChecked();
});

test('the profiles CSV keeps, and narrows to, the profiles holding the keys ticked', async ({ page }) => {
  await page.goto('/profiles');
  await page.getByRole('link', { name: 'CSV' }).first().click();
  const dialog = page.getByRole('dialog', { name: /Download CSV/ });
  const panel = dialog.getByRole('tabpanel');
  const download = dialog.getByRole('link', { name: 'Download' });
  await dialog.getByRole('tab', { name: /Keys/ }).click();
  await expect(panel.locator('thead th', { hasText: 'Hosts' })).toBeVisible();
  const narrow = dialog.getByRole('checkbox', { name: 'Count only the ticked keys' });
  await expect(narrow).toBeChecked();

  // Untick every key: nothing to download, and it says why.
  await panel.getByRole('checkbox', { name: 'All keys' }).uncheck();
  await expect(download).toHaveAttribute('aria-disabled', 'true');
  await expect(dialog.getByText('Tick at least one key.').first()).toBeVisible();

  // One key: its profile alone, counted for that key.
  const first = panel.locator('tbody tr[data-csv-value]').first();
  const key = (await first.getAttribute('data-csv-value')) ?? '';
  await first.getByRole('checkbox').check();
  if ((await panel.locator('tbody tr[data-csv-value]').count()) > 1) {
    await expect(download).toHaveAttribute('href', /narrow=1/);
    const res = await page.request.get((await download.getAttribute('href')) ?? '');
    const [header, row] = (await res.text()).trim().split('\n');
    const cols = header.split(',');
    expect(row.split(',')[cols.indexOf('key_names')]).toBe(key);
    expect(row.split(',')[cols.indexOf('keys')]).toBe('1');
    // Off: the profile is kept whole.
    await narrow.uncheck();
    await expect(download).not.toHaveAttribute('href', /narrow=1/);
  }
  await dialog.getByRole('button', { name: 'Reset' }).click();
  await expect(download).not.toHaveAttribute('href', /key=/);
});

test('a CSV with profiles and no key column says where keys are chosen', async ({ page }) => {
  await page.goto('/hosts');
  await page.locator('a[href$="/exports/termius-hosts.csv"]').first().dispatchEvent('click');
  const dialog = page.getByRole('dialog', { name: /Download CSV/ });
  await expect(dialog.getByRole('tab', { name: /Profiles/ })).toBeVisible();
  await expect(dialog.getByRole('tab', { name: /Keys/ })).toHaveCount(0);
  await expect(dialog.getByText('no keys to choose here')).toBeVisible();
  await expect(dialog.getByRole('link', { name: 'Profiles', exact: true })).toHaveAttribute('href', /\/profiles$/);
});

test('the CSV dialog downloads any format, and opens its choice in Export', async ({ page }) => {
  await page.goto('/hosts');
  await page.getByRole('link', { name: 'CSV' }).first().click();
  const dialog = page.getByRole('dialog', { name: /Download CSV/ });
  const download = dialog.getByRole('link', { name: 'Download' });
  await expect(download).toHaveAttribute('href', /\/exports\/hosts\.csv/);
  await dialog.getByLabel('Format').selectOption('json');
  await expect(download).toHaveAttribute('href', /\/exports\/run\?.*preset=hosts.*format=json|\/exports\/run\?.*format=json.*preset=hosts/);
  const res = await page.request.get((await download.getAttribute('href')) ?? '');
  expect(Array.isArray(await res.json())).toBe(true);
  await expect(dialog.getByRole('link', { name: 'Open in Export' })).toHaveAttribute('href', /\/export#s=.+&f=json/);
  await dialog.getByLabel('Format').selectOption('csv');
  await dialog.getByRole('button', { name: 'Cancel' }).click();

  // The Termius CSV is one format, with no preset to open.
  await page.locator('a[href$="/exports/termius-hosts.csv"]').first().dispatchEvent('click');
  await expect(dialog.getByLabel('Format')).toBeHidden();
  await expect(dialog.getByRole('link', { name: 'Open in Export' })).toBeHidden();
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

test('a labelled action explains itself to a screen reader, and shows no tooltip over itself', async ({ page }) => {
  await page.goto('/hosts');
  const add = page.getByRole('button', { name: 'Add a host' });
  await expect(add).toHaveAttribute('aria-description', /Its key is minted on the next reconcile/);
  await add.hover();
  await add.focus();
  await page.waitForTimeout(800);
  await expect(page.getByRole('tooltip')).toHaveCount(0);
  // A row's destructive item explains that it previews first, to a screen reader.
  await page.getByRole('button', { name: /^Actions for host / }).first().click();
  await expect(page.locator('.dropdown-menu.show .text-danger').first()).toHaveAttribute('aria-description', /Previews the delete first/);
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
