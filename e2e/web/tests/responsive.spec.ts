// Every page at phone width and from the keyboard: nothing wider than the
// screen, and the main navigation and the page's controls reachable by Tab
// with a visible focus ring.
import { test, expect, pages } from '../support/fixtures';

test.describe('at 375px', () => {
  test.use({ viewport: { width: 375, height: 812 } });

  for (const { path } of pages) {
    test(`${path} does not scroll sideways`, async ({ page }) => {
      await page.goto(path);
      // Wide content (tables, code) may scroll inside its own box; the page
      // itself must not. The page is asked to scroll, as a swipe would: a
      // hidden off-canvas panel makes scrollWidth large without letting it.
      const overflow = await page.evaluate(() => {
        window.scrollTo({ left: 400, top: 0, behavior: 'instant' as ScrollBehavior });
        const scrolled = window.scrollX;
        window.scrollTo({ left: 0, top: 0, behavior: 'instant' as ScrollBehavior });
        const W = document.documentElement.clientWidth;
        // What sticks out: anything past the right edge that is neither
        // fixed (off-canvas panels) nor clipped by a scrolling ancestor -
        // except an absolutely positioned one, which a static scroller does
        // not clip.
        const wide: string[] = [];
        for (const el of Array.from(document.body.querySelectorAll<HTMLElement>('*'))) {
          const r = el.getBoundingClientRect();
          const cs = getComputedStyle(el);
          if (r.right <= W + 1 || r.width === 0 || cs.position === 'fixed' || el.closest('.offcanvas, .modal')) continue;
          let p = el.parentElement;
          let clipped = false;
          while (p && p !== document.body && cs.position !== 'absolute') {
            if (getComputedStyle(p).overflowX !== 'visible') {
              clipped = true;
              break;
            }
            p = p.parentElement;
          }
          if (!clipped) wide.push(`${el.tagName.toLowerCase()}.${String(el.className).slice(0, 60)} (right ${Math.round(r.right)})`);
        }
        return { scrolled, wide: wide.slice(0, 8) };
      });
      expect(overflow.scrolled, `the page scrolls sideways: ${overflow.wide.join(', ')}`).toBe(0);
    });
  }
});

test('the keyboard reaches the navigation and the page, with a visible focus', async ({ page }) => {
  for (const path of ['/', '/hosts', '/api-tokens', '/webhooks', '/export']) {
    await page.goto(path);
    const seen = new Set<string>();
    let inMain = false;
    let inNav = false;
    for (let i = 0; i < 80 && !(inMain && inNav); i++) {
      await page.keyboard.press('Tab');
      const info = await page.evaluate(() => {
        const el = document.activeElement as HTMLElement | null;
        if (!el || el === document.body) return null;
        const cs = getComputedStyle(el);
        const ring = (cs.outlineStyle !== 'none' && parseFloat(cs.outlineWidth) > 0) || cs.boxShadow !== 'none';
        return {
          key: `${el.tagName}:${el.getAttribute('href') ?? el.getAttribute('name') ?? el.textContent?.trim().slice(0, 30)}`,
          nav: !!el.closest('nav[aria-label="Main"]'),
          main: !!el.closest('main'),
          ring,
        };
      });
      if (!info) continue;
      expect(info.ring, `${path}: ${info.key} has no visible focus`).toBe(true);
      seen.add(info.key);
      inNav ||= info.nav;
      inMain ||= info.main;
    }
    expect(inNav, `${path}: Tab never reached the main navigation`).toBe(true);
    expect(inMain, `${path}: Tab never reached the page's content`).toBe(true);
  }
});
