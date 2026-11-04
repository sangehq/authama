// Every read-only page, against the seeded home: each answers, carries its
// heading, and shows what the manifest and the reconciled ~/.ssh actually hold.
import { test, expect, pages } from '../support/fixtures';

// The shipped example manifest (src/config/manifest.json).
const profiles = ['work', 'personal', 'simtabi', 'development', 'school'];
const hosts: Record<string, string> = {
  hpc: 'hpc.example.edu',
  'github-personal': 'github.com',
  'github-simtabi': 'github.com',
  'app-web': '192.0.2.10',
  'app-db-maria': '198.51.100.20',
  'app-db-psql': '203.0.113.30',
};
const keys = [
  'work/work_hpc-ed25519',
  'personal/personal_github-ed25519',
  'simtabi/simtabi_github-ed25519',
  'development/development_app-web-ed25519',
  'development/development_app-db-maria-ed25519',
  'development/development_app-db-psql-ed25519',
];

for (const { path, heading } of pages) {
  test(`${path} answers with its heading`, async ({ page }) => {
    const res = await page.goto(path);
    expect(res?.status()).toBe(200);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(heading);
    await expect(page.getByRole('navigation', { name: 'Main' })).toBeVisible();
  });
}

test('without the token every page answers 401', async ({ playwright }) => {
  const anon = await playwright.request.newContext({ baseURL: process.env.AUTHAMA_E2E_URL });
  for (const { path } of pages) {
    expect((await anon.get(path)).status(), path).toBe(401);
  }
  expect((await anon.get('/?token=not-the-token')).status()).toBe(401);
  await anon.dispose();
});

test('the overview counts what the other pages list', async ({ page }) => {
  await page.goto('/profiles');
  const profileRows = await page.locator('table tbody tr').count();
  await page.goto('/');
  const count = (label: string) =>
    page.locator('.card-sm .card-body', { has: page.getByText(label, { exact: true }) }).locator('.h2');
  await expect(count('Profiles')).toHaveText(String(profileRows));
  await expect(count('Hosts')).toHaveText(String(Object.keys(hosts).length));
  await expect(count('Keys')).toHaveText(String(keys.length));
  // Nothing is pinned in the sandbox, so known_hosts is a row that needs attention.
  await expect(page.getByRole('table', { name: /state of this authama home/ }).locator('tbody tr').first()).toBeVisible();
});

test('profiles lists every profile in the manifest', async ({ page }) => {
  await page.goto('/profiles');
  for (const name of profiles) {
    await expect(page.getByRole('rowheader', { name, exact: true })).toBeVisible();
  }
  const work = page.getByRole('row').filter({ has: page.getByRole('rowheader', { name: 'work', exact: true }) });
  await expect(work).toContainText('per_service');
});

test('hosts lists every host with its hostname, and a host opens', async ({ page }) => {
  await page.goto('/hosts');
  for (const [alias, hostname] of Object.entries(hosts)) {
    const row = page.getByRole('row').filter({ has: page.getByRole('link', { name: alias, exact: true }) });
    await expect(row).toContainText(hostname);
  }
  await page.getByRole('link', { name: 'hpc', exact: true }).click();
  await expect(page).toHaveURL(/\/hosts\/hpc$/);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('hpc');
  await expect(page.locator('dd', { hasText: 'hpc.example.edu' })).toBeVisible();
});

test('keys lists every reconciled key as on disk, with a fingerprint', async ({ page }) => {
  await page.goto('/keys');
  for (const ref of keys) {
    const row = page.getByRole('row').filter({ has: page.getByRole('link', { name: ref, exact: true }) });
    await expect(row).toContainText('SHA256:');
    await expect(row.locator('.badge-ok')).toHaveText('yes');
  }
  await page.getByRole('link', { name: 'work/work_hpc-ed25519', exact: true }).click();
  await expect(page).toHaveURL(/\/keys\/work\/work_hpc-ed25519$/);
  await expect(page.getByText(/SHA256:/).first()).toBeVisible();
});

test('doctor shows the report authama doctor prints', async ({ page }) => {
  await page.goto('/doctor');
  const report = page.locator('pre.code-block');
  await expect(report).toContainText('RESULT: ready');
  await expect(report).toContainText('config drift: none');
  await expect(report).toContainText('host keys NOT pinned');
});

test('history records the seeding commands', async ({ page }) => {
  await page.goto('/history');
  for (const command of ['init', 'reconcile', 'bundle']) {
    await expect(page.getByRole('link', { name: command, exact: true }).first()).toBeVisible();
  }
});

test('an unknown page is a 404 with the error page', async ({ page }) => {
  const res = await page.goto('/no-such-page');
  expect(res?.status()).toBe(404);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Not found');
});

test('the command palette opens with Ctrl+K and goes where you type', async ({ page }) => {
  await page.goto('/');
  await page.keyboard.press('Control+k');
  const input = page.getByRole('combobox', { name: 'Go to a page or an action' });
  await expect(input).toBeFocused();
  await input.fill('auth tokens');
  await input.press('Enter');
  await expect(page).toHaveURL(/\/tokens$/);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Auth tokens');
});

test('a link to a form in a panel opens the panel', async ({ page }) => {
  await page.goto('/hosts#add-host');
  await expect(page.locator('form', { has: page.locator('input[name="action"][value="host-add"]') })).toBeVisible();
});

test('the tour walks the header step by step, and Escape ends it', async ({ page }) => {
  await page.goto('/');
  await page.locator('a[data-tour="6"]').click(); // the account menu
  await page.locator('[data-tour-start]').click();
  const pop = page.locator('.popover.tour-popover');
  await expect(pop).toBeVisible();
  await expect(pop.locator('.popover-header')).toHaveText(/^Search \(1 of \d+\)$/);
  await pop.getByRole('button', { name: 'Next' }).click();
  await expect(pop.locator('.popover-header')).toHaveText(/\(2 of \d+\)$/);
  await pop.getByRole('button', { name: 'Back' }).click();
  await expect(pop.locator('.popover-header')).toHaveText(/\(1 of \d+\)$/);
  await page.keyboard.press('Escape');
  await expect(page.locator('.popover.tour-popover')).toHaveCount(0);
});

// The pages refuse clipboard reads (Permissions-Policy clipboard-read=()), so
// a test records what a page writes rather than reading the clipboard back.
async function recordCopies(page: import('@playwright/test').Page): Promise<void> {
  await page.addInitScript(() => {
    const w = window as unknown as { copied: string[] };
    w.copied = [];
    const write = navigator.clipboard.writeText.bind(navigator.clipboard);
    navigator.clipboard.writeText = (text: string) => {
      w.copied.push(text);
      return write(text);
    };
  });
}
const copied = (page: import('@playwright/test').Page) => page.evaluate(() => (window as unknown as { copied: string[] }).copied);

test("a row's actions are one menu, and Copy puts the value on the clipboard", async ({ page, context }) => {
  await context.grantPermissions(['clipboard-write']);
  await recordCopies(page);
  await page.goto('/hosts');
  const menu = page.getByRole('button', { name: 'Actions for host hpc' });
  await menu.click();
  await page.locator('.dropdown-menu.show [data-copy="ssh hpc"]').click();
  await expect(page.locator('#toast-stack .toast')).toContainText('Copied');
  expect(await copied(page)).toEqual(['ssh hpc']);
});

test("a key's page shows its public key with a copy button", async ({ page, context }) => {
  await context.grantPermissions(['clipboard-write']);
  await recordCopies(page);
  await page.goto('/keys/work/work_hpc-ed25519');
  const pub = page.locator('pre.code-block').first();
  await expect(pub).toContainText(/^ssh-ed25519 AAAA/);
  await page.getByRole('button', { name: 'Copy the public key' }).click();
  expect(await copied(page)).toEqual([(await pub.textContent())?.trim()]);
});

// A row's menu opens over the page, not inside the table's scrolling box: its
// last item is on screen and is what a click there reaches, even for the last
// row of a short table, where a clipped menu was cut off at the card's edge.
for (const { path, table, label } of [
  { path: '/profiles', table: '#profiles-table', label: /^Actions for profile / },
  { path: '/keys', table: '#keys-table', label: /^Actions for key / },
  { path: '/hosts', table: '#hosts-table', label: /^Actions for host / },
]) {
  test(`a row's menu is not clipped by its table on ${path}`, async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto(path);
    await page.locator(`${table} tbody tr`).last().getByRole('button', { name: label }).click();
    const menu = page.locator('.dropdown-menu.show');
    // Fixed to the viewport, so no scrolling ancestor can clip it: a short
    // table (two tokens, say) has room for the menu neither below nor above.
    expect(await menu.evaluate((el) => getComputedStyle(el).position)).toBe('fixed');
    const last = menu.locator('.dropdown-item').last();
    await expect(last).toBeVisible();
    await last.scrollIntoViewIfNeeded();
    const box = await last.boundingBox();
    expect(box, 'the last item has a box').not.toBeNull();
    const reached = await page.evaluate(([x, y]) => {
      const el = document.elementFromPoint(x, y);
      return !!el?.closest('.dropdown-menu.show');
    }, [box!.x + box!.width / 2, box!.y + box!.height / 2]);
    expect(reached, 'the last item is on top where it is drawn').toBe(true);
  });
}
