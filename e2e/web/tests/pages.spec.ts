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
