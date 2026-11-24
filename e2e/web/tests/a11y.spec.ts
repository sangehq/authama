// An axe pass over every page the e2e specs drive, in the light and the dark
// theme. A serious or critical violation fails; minor and moderate ones are
// attached to the report so they are seen without blocking.
import AxeBuilder from '@axe-core/playwright';
import type { Page, TestInfo } from '@playwright/test';
import { test, expect, pages } from '../support/fixtures';

const tags = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'];
const blocking = new Set(['serious', 'critical']);

async function audit(page: Page, info: TestInfo, label: string, within?: string[]): Promise<void> {
  let axe = new AxeBuilder({ page }).withTags(tags);
  for (const sel of within ?? []) axe = axe.include(sel);
  const { violations } = await axe.analyze();
  if (violations.length) {
    await info.attach(`axe ${label}`, { body: JSON.stringify(violations, null, 2), contentType: 'application/json' });
  }
  const failing = violations
    .filter((v) => blocking.has(v.impact ?? ''))
    .map(
      (v) =>
        `${v.impact} ${v.id}: ${v.help} -> ` +
        v.nodes.map((n) => `${n.target.join(' ')} (${n.failureSummary?.replace(/\s+/g, ' ')})`).join('; '),
    );
  expect(failing, `serious or critical axe violations on ${label}`).toEqual([]);
}

type TileReading = { label: string; state: string; icon: number; edge: number };

// tileContrast reads every colour tile in an open menu: its icon against the
// tile, and the tile's edge against the entry behind it, as contrast ratios.
// Colours are resolved through a canvas, which turns any CSS colour (the
// tiles mix theirs in oklab) into sRGB bytes.
async function tileContrast(page: Page, menu: string): Promise<TileReading[]> {
  const items = page.locator(`[aria-labelledby="navbar-${menu}"] .nav-tile`);
  const n = await items.count();
  const out: TileReading[] = [];
  for (const state of ['rest', 'focus'] as const) {
    for (let i = 0; i < n; i++) {
      const tile = items.nth(i);
      if (state === 'focus') {
        const entry = tile.locator('xpath=ancestor::*[contains(@class,"dropdown-item") or @data-running-as][1]');
        if ((await entry.evaluate((el) => el.tagName)) === 'DIV') continue; // Running as is text, not focusable
        await entry.focus();
      }
      out.push(
        await tile.evaluate((el, st) => {
          const canvas = document.createElement('canvas');
          canvas.width = canvas.height = 1;
          const ctx = canvas.getContext('2d', { willReadFrequently: true })!;
          const rgba = (css: string): number[] => {
            ctx.clearRect(0, 0, 1, 1);
            ctx.fillStyle = '#000';
            ctx.fillStyle = css;
            ctx.fillRect(0, 0, 1, 1);
            const d = ctx.getImageData(0, 0, 1, 1).data;
            return [d[0], d[1], d[2], d[3] / 255];
          };
          const over = (top: number[], under: number[]): number[] =>
            [0, 1, 2].map((k) => top[k] * top[3] + under[k] * (1 - top[3])).concat(1);
          // The opaque colour behind el: its ancestors' backgrounds, composited.
          const backdrop = (from: Element | null): number[] => {
            const layers: number[][] = [];
            for (let e = from; e; e = e.parentElement) {
              const c = rgba(getComputedStyle(e).backgroundColor);
              if (c[3] > 0) layers.push(c);
              if (c[3] === 1) break;
            }
            return layers.reverse().reduce((acc, c) => over(c, acc), [255, 255, 255, 1]);
          };
          const lum = (c: number[]): number => {
            const ch = c.slice(0, 3).map((v) => {
              const s = v / 255;
              return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
            });
            return 0.2126 * ch[0] + 0.7152 * ch[1] + 0.0722 * ch[2];
          };
          const ratio = (a: number[], b: number[]): number => {
            const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p);
            return Math.round(((x + 0.05) / (y + 0.05)) * 100) / 100;
          };
          const behind = backdrop(el.parentElement);
          const tileBg = over(rgba(getComputedStyle(el).backgroundColor), behind);
          const icon = over(rgba(getComputedStyle(el).color), tileBg);
          const edge = over(rgba(getComputedStyle(el).borderTopColor), behind);
          const entry = el.closest('.dropdown-item, [data-running-as]');
          return {
            label: (entry?.textContent ?? '').trim().split(/\s{2,}|\n/)[0],
            state: st,
            icon: ratio(icon, tileBg),
            edge: ratio(edge, behind),
          };
        }, state),
      );
    }
  }
  return out;
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

    // Each menu open, on a page it holds so one entry is the current one:
    // a closed dropdown is display:none, which axe skips, so the entries'
    // colours (a description on the active tint, a red destructive entry)
    // are only checked like this.
    for (const { menu, path } of [
      { menu: 'ssh', path: '/hosts' },
      { menu: 'tokens', path: '/tokens' },
      { menu: 'security', path: '/audit' },
      { menu: 'history', path: '/history' },
      { menu: 'system', path: '/reset' },
      { menu: 'new', path: '/' },
      { menu: 'user', path: '/about' },
    ]) {
      test(`the ${menu} menu, open on ${path}, has no serious or critical violations`, async ({ page }, info) => {
        await page.goto(`${path}?theme=${theme}`);
        await expect(page.locator('html')).toHaveAttribute('data-bs-theme', theme);
        await page.locator(`#navbar-${menu}`).click();
        await expect(page.locator(`[aria-labelledby="navbar-${menu}"]`)).toBeVisible();
        if (menu === 'user') {
          // The Theme submenu open too, so Light, Dark and the check are judged.
          await page.locator('#navbar-theme').click();
          await expect(page.locator('[aria-labelledby="navbar-theme"]')).toBeVisible();
        }
        // The headers, which hold the menus. The page under an open menu is
        // audited with every menu closed above; checking it here judged a page
        // button the menu happened to cover as a too-small target (axe
        // target-size, "partially obscured"), which is the menu's overlap, not
        // the button's size.
        await audit(page, info, `${menu} menu on ${path} (${theme})`, ['header']);
        // axe measures text, not an icon or the edge of its tile; WCAG 1.4.11
        // asks 3:1 of both, so they are measured here, at rest and with the
        // focus on each entry (its hover and focus tint behind the tile).
        const tiles = await tileContrast(page, menu);
        await info.attach(`tile contrast ${menu} (${theme})`, { body: JSON.stringify(tiles, null, 2), contentType: 'application/json' });
        expect(tiles.length, `the ${menu} menu's colour tiles`).toBeGreaterThanOrEqual(3);
        const low = tiles.filter((t) => t.icon < 3 || t.edge < 3).map((t) => `${t.label}: icon ${t.icon}, edge ${t.edge}`);
        expect(low, `icons or tile edges under 3:1 in the ${menu} menu (${theme})`).toEqual([]);
      });
    }

    test('a change preview has no serious or critical violations', async ({ page }, info) => {
      await page.goto(`/?theme=${theme}`);
      await page.getByRole('button', { name: 'Re-render ~/.ssh/config' }).click();
      // The preview opens in the side panel, over the page.
      await expect(page.locator('#side-panel').getByRole('heading', { level: 1 })).toHaveText('Review the change');
      await expect(page.locator('html')).toHaveAttribute('data-bs-theme', theme);
      await audit(page, info, `change preview (${theme})`);
    });
  });
}
