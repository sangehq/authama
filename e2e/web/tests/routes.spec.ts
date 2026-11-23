// Host routes from the web, end to end: a host added with a second name, a
// proxy, options and a section; the offer to update a host that exists; its
// edit form, which removes what is taken out; a WARP route variant set,
// edited and removed; hand edits in a managed file shown, kept and discarded;
// an import preview that takes the same blocks in. Each panel and preview is
// checked with axe. The flows share the home, so they run in order, and the
// last one deletes the host they made.
import AxeBuilder from '@axe-core/playwright';
import { readFileSync, readdirSync, writeFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { test, expect } from '../support/fixtures';

test.describe.configure({ mode: 'serial' });

const alias = 'e2e-web0';

async function audit(page: Page, info: TestInfo, label: string): Promise<void> {
  const { violations } = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'])
    .analyze();
  if (violations.length) {
    await info.attach(`axe ${label}`, { body: JSON.stringify(violations, null, 2), contentType: 'application/json' });
  }
  const failing = violations
    .filter((v) => v.impact === 'serious' || v.impact === 'critical')
    .map((v) => `${v.impact} ${v.id}: ${v.help} -> ${v.nodes.map((n) => n.target.join(' ')).join('; ')}`);
  expect(failing, `serious or critical axe violations on ${label}`).toEqual([]);
}

const panel = (page: Page): Locator => page.locator('#side-panel');

// confirm makes the previewed change, through the confirm dialog when the
// change destroys something, and waits for the Done page.
async function confirm(page: Page): Promise<void> {
  const p = panel(page);
  const destroy = p.getByRole('button', { name: 'Make this change…' });
  if (await destroy.count()) {
    await destroy.click();
    await page.getByRole('button', { name: 'Yes, make it' }).click();
  } else {
    await p.getByRole('button', { name: 'Make this change' }).click();
  }
  await expect(p.getByRole('heading', { level: 1 })).toHaveText('Done');
}

async function preview(page: Page, form: Locator): Promise<void> {
  await form.getByRole('button', { name: /Preview/ }).first().click();
  await expect(panel(page).getByRole('heading', { level: 1 })).toHaveText('Review the change');
}

function home(): string {
  const h = process.env.AUTHAMA_E2E_HOME;
  if (!h) throw new Error('AUTHAMA_E2E_HOME is not set: the global setup did not run');
  return h;
}

// managedFile is the managed config file holding the host's block.
function managedFile(name: string): string {
  const f = findManaged(name);
  expect(f, `a managed file holding Host ${name}`).not.toBe('');
  return f;
}

function findManaged(name: string): string {
  const walk = (dir: string): string => {
    for (const e of readdirSync(dir)) {
      const p = join(dir, e);
      if (statSync(p).isDirectory()) {
        const found = walk(p);
        if (found) return found;
      } else if (e === 'config' || e.endsWith('.conf')) {
        const text = readFileSync(p, 'utf8');
        if (text.includes('Managed by authama') && new RegExp(`\\nHost ${name}[ \\n]`).test(text)) return p;
      }
    }
    return '';
  };
  return walk(join(home(), '.ssh'));
}

async function openRoutes(form: Locator): Promise<void> {
  const summary = form.locator('summary', { hasText: 'Routes' });
  const details = summary.locator('xpath=..');
  if ((await details.getAttribute('open')) === null) await summary.click();
}

test('a host is added with a second name, Cloudflare Access, options and a section', async ({ page }, info) => {
  await page.goto('/hosts');
  await page.getByRole('button', { name: 'Add a host' }).click();
  const form = page.locator('form', { has: page.locator('input[name="action"][value="host-add"]') });
  await expect(form).toBeVisible();
  await form.getByLabel('Profile', { exact: true }).selectOption('work');
  await form.getByLabel('Alias', { exact: true }).fill(alias);
  await form.getByLabel('Hostname', { exact: true }).fill('ssh-web.example.com');
  await form.getByLabel('User', { exact: true }).fill('ploi');
  await openRoutes(form);
  await form.getByLabel('Also named', { exact: true }).fill(`${alias}-cf`);
  await form.getByLabel('Proxy', { exact: true }).selectOption('cloudflared');
  await form.getByLabel('Other ssh options', { exact: true }).fill('ServerAliveInterval=30');
  await form.getByLabel('Section heading', { exact: true }).fill('e2e prod: Cloudflare Access');
  await form.getByLabel('Comment', { exact: true }).fill('the e2e web box');
  await audit(page, info, 'Add a host with routes');
  await preview(page, form);
  const cmd = panel(page).locator('pre.code-block').first();
  for (const flag of [`--alias=${alias}-cf`, '--proxy=cloudflared', '--option=ServerAliveInterval=30', '--section=']) {
    await expect(cmd).toContainText(flag);
  }
  await audit(page, info, 'host add preview');
  await confirm(page);
  const text = readFileSync(managedFile(alias), 'utf8');
  expect(text).toContain(`# e2e prod: Cloudflare Access\nHost ${alias} ${alias}-cf\n    # the e2e web box\n`);
  expect(text).toMatch(/ProxyCommand \S*cloudflared access ssh --hostname %h/);
});

test('a typo in an option is refused with a suggestion', async ({ page }) => {
  await page.goto(`/hosts/${alias}`);
  await page.getByRole('button', { name: 'Edit', exact: true }).first().click();
  const form = page.locator('#host-edit form').first();
  await openRoutes(form);
  await form.getByLabel('Other ssh options', { exact: true }).fill('ServerAliveInterval=30\nServerAlivInterval=5');
  await form.getByRole('button', { name: /Preview/ }).first().click();
  await expect(page.getByText('did you mean ServerAliveInterval?').first()).toBeVisible();
});

test('adding a host that exists offers to update it', async ({ page }, info) => {
  await page.goto('/hosts');
  await page.getByRole('button', { name: 'Add a host' }).click();
  const form = page.locator('form', { has: page.locator('input[name="action"][value="host-add"]') });
  await form.getByLabel('Profile', { exact: true }).selectOption('work');
  await form.getByLabel('Alias', { exact: true }).fill(alias);
  await form.getByLabel('Hostname', { exact: true }).fill('ssh-web-00.example.com');
  await form.getByLabel('User', { exact: true }).fill('ploi');
  await preview(page, form);
  const offer = panel(page).locator('#offer-update');
  await expect(offer).toBeVisible();
  await audit(page, info, 'host add of an existing host');
  await offer.getByRole('button', { name: 'Preview the update' }).click();
  await expect(panel(page).locator('pre.code-block').first()).toContainText('--update');
  await confirm(page);
  expect(readFileSync(managedFile(alias), 'utf8')).toContain('HostName ssh-web-00.example.com');
});

test('the edit form changes names, proxy, jump, timeout and options, and removes what is taken out', async ({ page }, info) => {
  await page.goto(`/hosts/${alias}`);
  await page.getByRole('button', { name: 'Edit', exact: true }).first().click();
  const form = page.locator('#host-edit form').first();
  await openRoutes(form);
  await expect(form.getByLabel('Also named', { exact: true })).toHaveValue(`${alias}-cf`);
  await expect(form.getByLabel('Other ssh options', { exact: true })).toHaveValue('ServerAliveInterval=30');
  await form.getByLabel('Also named', { exact: true }).fill(`${alias}-b`);
  await form.getByLabel('Proxy', { exact: true }).selectOption('off');
  await form.getByLabel('Jump host', { exact: true }).fill('bastion');
  await form.getByLabel('Connect timeout', { exact: true }).fill('15');
  await form.getByLabel('Other ssh options', { exact: true }).fill('Compression=yes');
  await audit(page, info, 'host edit with routes');
  await preview(page, form);
  const cmd = panel(page).locator('pre.code-block').first();
  for (const flag of [`--alias=${alias}-b`, `--remove-alias=${alias}-cf`, '--proxy=off', '--proxy-jump=bastion',
    '--connect-timeout=15', '--option=Compression=yes', '--unset-option=ServerAliveInterval']) {
    await expect(cmd).toContainText(flag);
  }
  await confirm(page);
  const text = readFileSync(managedFile(alias), 'utf8');
  expect(text).toContain(`Host ${alias} ${alias}-b\n`);
  expect(text).toContain('    ProxyJump bastion\n');
  expect(text).toContain('    ConnectTimeout 15\n');
  expect(text).not.toContain('ServerAliveInterval 30');

  // A command of one's own, then back to none.
  await page.goto(`/hosts/${alias}`);
  await page.getByRole('button', { name: 'Edit', exact: true }).first().click();
  const again = page.locator('#host-edit form').first();
  await openRoutes(again);
  await again.getByLabel('Jump host', { exact: true }).fill('');
  await again.getByLabel('Or a command', { exact: true }).fill('/usr/bin/nc -X 5 -x proxy:1080 %h %p');
  await preview(page, again);
  await expect(panel(page).locator('pre.code-block').first()).toContainText('--proxy-command=');
  await confirm(page);
  expect(readFileSync(managedFile(alias), 'utf8')).toContain('    ProxyCommand /usr/bin/nc -X 5 -x proxy:1080 %h %p\n');
});

test('a WARP route variant is set, edited and removed', async ({ page }, info) => {
  await page.goto(`/hosts/${alias}`);
  await page.getByRole('button', { name: 'Add or change one' }).click();
  const form = page.locator('#host-variant form').first();
  await expect(form).toBeVisible();
  await form.getByLabel('Name', { exact: true }).fill('warp');
  await form.getByLabel('Hostname', { exact: true }).fill('10.116.0.3');
  await form.getByLabel('Proxy', { exact: true }).selectOption('none');
  await form.getByLabel('Connect timeout', { exact: true }).fill('10');
  await form.getByLabel('Other ssh options', { exact: true }).fill('ServerAliveInterval=20');
  await form.getByLabel('Section heading', { exact: true }).fill('e2e prod: private IPs over WARP');
  await audit(page, info, 'route variant panel');
  await preview(page, form);
  const cmd = panel(page).locator('pre.code-block').first();
  for (const flag of ['host variant set', '--hostname=10.116.0.3', '--proxy=none', '--connect-timeout=10']) {
    await expect(cmd).toContainText(flag);
  }
  await confirm(page);
  let text = readFileSync(managedFile(alias), 'utf8');
  expect(text).toContain(`# e2e prod: private IPs over WARP\nHost warp-${alias}\n    HostName 10.116.0.3\n`);
  expect(text).toContain('    ProxyCommand none\n');

  await page.goto(`/hosts/${alias}`);
  await expect(page.getByText(`warp-${alias}`).first()).toBeVisible();
  await page.getByRole('button', { name: `Edit warp-${alias}` }).click();
  const edit = page.locator('#variant-warp form').first();
  await expect(edit.getByLabel('Hostname', { exact: true })).toHaveValue('10.116.0.3');
  await edit.getByLabel('Port', { exact: true }).fill('2222');
  await edit.getByLabel('Other ssh options', { exact: true }).fill('');
  await audit(page, info, 'route variant edit');
  await preview(page, edit);
  await expect(panel(page).locator('pre.code-block').first()).toContainText('--unset-option=ServerAliveInterval');
  await confirm(page);
  text = readFileSync(managedFile(alias), 'utf8');
  expect(text).toContain('    Port 2222\n');
  expect(text).not.toContain('ServerAliveInterval 20');

  await page.goto(`/hosts/${alias}`);
  await page.getByRole('button', { name: `Remove warp-${alias}` }).click();
  await expect(panel(page).getByRole('heading', { level: 1 })).toHaveText('Review the change');
  await confirm(page);
  expect(readFileSync(managedFile(alias), 'utf8')).not.toContain(`Host warp-${alias}`);
});

test('hand edits are listed, a render refuses over them, and they are kept or discarded', async ({ page }, info) => {
  const file = managedFile(alias);
  const edit = (line: string): void =>
    writeFileSync(file, readFileSync(file, 'utf8').replace(new RegExp(`(\\nHost ${alias} [^\\n]*\\n)`), `$1    ${line}\n`));

  edit('ServerAliveCountMax 4');
  await page.goto('/ssh-config');
  const card = page.locator('#hand-edits');
  await expect(card).toContainText('ServerAliveCountMax');
  await audit(page, info, 'SSH config with hand edits');

  // A render refuses, and offers both ways out.
  await page.getByRole('button', { name: 'Re-render ~/.ssh/config' }).click();
  const choice = panel(page).locator('#hand-edit-choice');
  await expect(choice).toBeVisible();
  await expect(panel(page).getByRole('button', { name: 'Make this change' })).toHaveCount(0);
  await audit(page, info, 'render refused over hand edits');

  // Keep: the preview says exactly what is adopted.
  await choice.getByRole('button', { name: 'Keep hand edits' }).click();
  await expect(panel(page).getByRole('heading', { level: 1 })).toHaveText('Review the change');
  await expect(panel(page).locator('pre.code-block').first()).toContainText('ssh config keep-edits');
  await expect(panel(page).getByText(new RegExp(`adopted work/${alias}`))).toBeVisible();
  await expect(panel(page).locator('tr.diff-add', { hasText: 'ServerAliveCountMax' }).first()).toBeVisible();
  await audit(page, info, 'keep-edits preview');
  await confirm(page);
  await page.goto('/ssh-config');
  await expect(page.locator('#hand-edits')).toHaveCount(0);

  // Discard: previewed, confirmed as destructive, and the backup is named.
  edit('Compression no');
  await page.goto('/ssh-config');
  await expect(page.locator('#hand-edits')).toContainText('Compression');
  await page.getByRole('button', { name: 'Discard hand edits and render' }).click();
  await expect(panel(page).locator('pre.code-block').first()).toContainText('--force');
  await audit(page, info, 'render --force preview');
  await confirm(page);
  await expect(panel(page).getByText(/config-hand-edits-/).first()).toBeVisible();
  expect(readFileSync(file, 'utf8')).not.toContain('Compression no');
});

test('an import preview takes in more names, ProxyCommand none and sections', async ({ page }, info) => {
  const cfg = join(home(), '.ssh', 'config');
  const before = readFileSync(cfg, 'utf8');
  const managed = readFileSync(managedFile(alias), 'utf8');
  const keyFile = /IdentityFile (\S+)/.exec(managed.slice(managed.indexOf(`Host ${alias}`)))?.[1] ?? '';
  writeFileSync(cfg, `${before}\n# e2e import: WARP\nHost e2e-imp-00 e2e-imp-cf\n    HostName 10.116.0.9\n    User ploi\n    IdentityFile ${keyFile}\n    ProxyCommand none\n    ConnectTimeout 10\n`);
  try {
    await page.goto('/import');
    const form = page.locator('form', { has: page.locator('input[name="action"][value="ssh-import"]') }).first();
    const replace = form.getByLabel('Replace the profiles in the manifest');
    if (await replace.count()) await replace.check();
    await preview(page, form);
    const diff = panel(page).locator('.file-diff', { hasText: 'manifest.json' }).first();
    for (const want of ['"e2e-imp-cf"', '"kind": "none"', '"ConnectTimeout"', '"section": "e2e import: WARP"']) {
      await expect(diff).toContainText(want);
    }
    await audit(page, info, 'import preview with routes');
  } finally {
    writeFileSync(cfg, before);
  }
});

test('the host and its names are deleted', async ({ page }) => {
  await page.goto(`/hosts/${alias}`);
  await page.getByRole('button', { name: 'Edit', exact: true }).first().click();
  await page.getByRole('button', { name: `Preview deleting ${alias}` }).click();
  await expect(panel(page).getByRole('heading', { level: 1 })).toHaveText('Review the change');
  await confirm(page);
  await page.goto('/hosts');
  await expect(page.locator(`a[href$="/hosts/${alias}"]`)).toHaveCount(0);
  expect(findManaged(alias)).toBe('');
});
