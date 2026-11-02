// The write flows, in order: a change previewed and then made, and a job run
// with its event stream watched to the end. They share one home, so they run
// one after another.
import { test, expect, bundle } from '../support/fixtures';

test.describe.configure({ mode: 'serial' });

const added = 'e2e-added';

test('a profile is previewed, then added', async ({ page }) => {
  await page.goto('/profiles');
  const form = page.locator('form', { has: page.locator('input[name="action"][value="profile-add"]') });
  await form.getByLabel('Name').fill(added);
  await form.getByRole('button', { name: 'Preview' }).click();

  await expect(page).toHaveURL(/\/changes\/preview$/);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Review the change');
  await expect(page.locator('pre.code-block').first()).toContainText(`profile add ${added}`);
  // The diff of the manifest the change would write names the new profile.
  await expect(page.locator('.diff-line', { hasText: added }).first()).toBeVisible();
  // A preview writes nothing.
  const look = await page.context().newPage();
  await look.goto('/profiles');
  await expect(look.getByRole('rowheader', { name: 'personal', exact: true })).toBeVisible();
  await expect(look.getByRole('rowheader', { name: added, exact: true })).toHaveCount(0);
  await look.close();

  await page.getByRole('button', { name: 'Make this change' }).click();
  await expect(page).toHaveURL(/\/changes\/apply$/);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Done');
  await expect(page.getByRole('status')).toContainText(`profile add ${added}`);

  await page.getByRole('link', { name: 'See it in the history' }).click();
  await expect(page.getByRole('heading', { level: 1 })).toContainText('profile add');

  await page.goto('/profiles');
  await expect(page.getByRole('rowheader', { name: added, exact: true })).toBeVisible();
  await page.goto('/history?source=web');
  await expect(page.getByRole('link', { name: 'profile add', exact: true }).first()).toBeVisible();
});

test('a preview whose home moved underneath is refused, and nothing is written', async ({ page }) => {
  await page.goto('/profiles');
  const form = page.locator('form', { has: page.locator('input[name="action"][value="profile-add"]') });
  await form.getByLabel('Name').fill('e2e-stale');
  await form.getByRole('button', { name: 'Preview' }).click();
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Review the change');

  // Another change lands between the preview and the confirm.
  const other = await page.context().newPage();
  await other.goto('/profiles');
  const otherForm = other.locator('form', { has: other.locator('input[name="action"][value="profile-add"]') });
  await otherForm.getByLabel('Name').fill('e2e-moved');
  await otherForm.getByRole('button', { name: 'Preview' }).click();
  await other.getByRole('button', { name: 'Make this change' }).click();
  await expect(other.getByRole('heading', { level: 1 })).toHaveText('Done');
  await other.close();

  await page.getByRole('button', { name: 'Make this change' }).click();
  await expect(page.getByRole('alert').filter({ hasText: 'Something changed since you looked' })).toBeVisible();
  await page.goto('/profiles');
  await expect(page.getByRole('rowheader', { name: 'e2e-stale', exact: true })).toHaveCount(0);
});

test('a bundle restore runs as a job and its stream is watched to the end', async ({ page }) => {
  const name = bundle();
  expect(name, 'the global setup wrote a bundle').toMatch(/^authama-\d{8}-\d{6}\.age$/);

  await page.goto('/snapshots');
  const bundles = page.getByRole('table', { name: 'Encrypted bundles, newest first' });
  await expect(bundles.getByRole('rowheader', { name, exact: true })).toBeVisible();
  await bundles.getByRole('button', { name: `Restore ${name}` }).click();

  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Review the job');
  await expect(page.locator('.list-group-item', { hasText: `decrypts ${name}` })).toBeVisible();
  await page.getByRole('button', { name: /^Run authama ssh restore / }).click();
  await expect(page).toHaveURL(/\/jobs\/[^/]+$/);

  // The event stream replays every line and ends once the job does; reading
  // it to the end is watching the job finish.
  const events = await page.request.get(`${new URL(page.url()).pathname}/events`, { timeout: 60_000 });
  expect(events.headers()['content-type']).toContain('text/event-stream');
  const stream = await events.text();
  expect(stream).toMatch(/event: line\ndata: restore: laid down \d+ file\(s\)/);
  expect(stream).toContain('event: line\ndata:   work_hpc-ed25519  SHA256:');
  expect(stream.trimEnd()).toMatch(/event: state\ndata: done$/);

  // The page follows the same stream, and reloads into its finished state.
  await expect(page.locator('#job-state')).toHaveAttribute('data-state', 'done', { timeout: 30_000 });
  await expect(page.locator('#job-log')).toContainText('restore: laid down');
  await expect(page.locator('#job-state .badge')).toHaveText('done');

  // The job snapshotted ~/.ssh before laying the bundle back.
  await page.goto('/snapshots');
  const snapshots = page.getByRole('table', { name: 'Snapshots, newest first' });
  await expect(snapshots.locator('tbody tr').first()).toBeVisible();
  await snapshots.getByRole('link', { name: 'with now' }).first().click();
  await expect(page).toHaveURL(/\/snapshots\/ssh-\d{8}-\d{6}/);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(/^ssh-\d{8}-\d{6}/);
});
