// Tables fit their box at every width, and their cells can be copied.
// A wide table used to scroll sideways inside its card (the Termius keys
// table at 1440px, a code cell that would not wrap); now long values wrap and,
// below 1200px, a table is one card per row.
import { test, expect, pages } from '../support/fixtures';

for (const width of [1440, 1200, 1024, 768, 375]) {
  test.describe(`at ${width}px`, () => {
    test.use({ viewport: { width, height: 900 } });

    test('no table and no page scrolls sideways', async ({ page }) => {
      let tables = 0;
      const wide: string[] = [];
      for (const { path } of pages) {
        await page.goto(path);
        const r = await page.evaluate(() => {
          const boxes = Array.from(document.querySelectorAll<HTMLElement>('.table-responsive'));
          const doc = document.documentElement;
          return {
            n: boxes.length,
            wide: boxes
              .filter((b) => b.scrollWidth > b.clientWidth + 1)
              .map((b) => `${b.querySelector('table')?.id || 'a table'} ${b.scrollWidth}>${b.clientWidth}`),
            page: doc.scrollWidth > doc.clientWidth + 1 ? `the page ${doc.scrollWidth}>${doc.clientWidth}` : '',
          };
        });
        tables += r.n;
        for (const w of [...r.wide, r.page].filter(Boolean)) wide.push(`${path}: ${w}`);
      }
      // A selector that stopped matching would pass with nothing checked.
      expect(tables, 'tables inspected').toBeGreaterThan(20);
      expect(wide).toEqual([]);
    });
  });
}

test('a right-click copies a cell, its row, its column or the table', async ({ page }) => {
  await page.addInitScript(() => {
    (window as unknown as { copied: string[] }).copied = [];
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: (t: string) => ((window as unknown as { copied: string[] }).copied.push(t), Promise.resolve()) },
    });
  });
  await page.goto('/termius');
  const cell = page.locator('#termius-preview tbody tr').first().locator('td').nth(1);
  const menu = page.locator('.copy-cells-menu');
  for (const item of ['Copy this cell', 'Copy this row', 'column', 'Copy the table']) {
    await cell.click({ button: 'right' });
    await menu.getByRole('menuitem', { name: new RegExp(item) }).click();
    await expect(menu).toHaveCount(0);
  }
  const copied = await page.evaluate(() => (window as unknown as { copied: string[] }).copied);
  const label = await cell.innerText();
  expect(copied[0]).toBe(label.trim());
  expect(copied[1].split('\t')).toContain(label.trim());
  expect(copied[1].split('\t').at(-1), 'the Password cell copies as empty, not "never written"').toBe('');
  expect(copied[2].split('\n')[0]).toBe(label.trim());
  expect(copied[3].split('\n')[0].split('\t')[0]).toBe('Groups');

  // The keyboard opens it on a focused cell, and Escape closes it.
  await cell.focus();
  await page.keyboard.press('Shift+F10');
  await expect(menu.getByRole('menuitem').first()).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(menu).toHaveCount(0);
  await expect(cell).toBeFocused();
});
