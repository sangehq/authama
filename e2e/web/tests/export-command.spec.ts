// The Export page's "Copy as command": the command it copies, run in a
// terminal against the same home, writes the file the page previews.
import { execFileSync } from 'node:child_process';
import { test, expect } from '../support/fixtures';
import { bin, sandboxEnv } from '../support/sandbox';

// The page records what it writes to the clipboard: the pages refuse
// clipboard reads (Permissions-Policy clipboard-read=()).
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

// The words of a command the page quoted for a POSIX shell: bare words, and
// single-quoted ones where '\'' is a quote.
function shellWords(cmd: string): string[] {
  const out: string[] = [];
  let word = '';
  let has = false;
  for (let i = 0; i < cmd.length; i++) {
    const c = cmd[i];
    if (c === "'") {
      const end = cmd.indexOf("'", i + 1);
      if (end < 0) throw new Error(`unclosed quote in ${cmd}`);
      word += cmd.slice(i + 1, end);
      has = true;
      i = end;
    } else if (c === '\\') {
      word += cmd[++i];
      has = true;
    } else if (c === ' ') {
      if (has) out.push(word);
      word = '';
      has = false;
    } else {
      word += c;
      has = true;
    }
  }
  if (has) out.push(word);
  return out;
}

function runCommand(cmd: string): string {
  const home = process.env.AUTHAMA_E2E_HOME;
  if (!home) throw new Error('AUTHAMA_E2E_HOME is not set: the global setup did not run');
  const [name, ...args] = shellWords(cmd);
  expect(name).toBe('authama');
  return execFileSync(bin, args, { env: sandboxEnv(home), encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
}

const text = (v: unknown): string => (Array.isArray(v) ? v.join(' ') : v === null || v === undefined ? '' : String(v));

test('Copy as command copies the authama export that writes the previewed rows', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-write']);
  await recordCopies(page);
  await page.goto('/export');
  const exp = page.locator('[data-export-page]');
  const preview = exp.getByRole('region', { name: 'Preview of the first rows' });
  await expect(preview.locator('tbody tr').first()).toBeVisible();

  // A filter, a renamed column whose heading holds a comma and a space, a
  // descending sort, and JSON.
  await exp.getByLabel('Keep rows containing').fill('app');
  await expect(preview.locator('tbody tr')).toHaveCount(3);
  await exp.getByLabel('Heading for hostname').fill('host, name');
  await exp.getByLabel('Heading for hostname').press('Enter');
  await expect(preview.locator('thead th').filter({ hasText: 'host, name' })).toHaveCount(1);
  const alias = preview.locator('thead th button[data-sort="alias"]');
  await alias.click();
  await expect(preview.locator('thead th[aria-sort="ascending"]')).toHaveCount(1);
  await preview.locator('thead th button[data-sort="alias"]').click();
  await expect(preview.locator('thead th[aria-sort="descending"]')).toHaveCount(1);
  await exp.getByLabel('Format').selectOption('json');

  const headings = (await preview.locator('thead th').allTextContents()).map((h) => h.trim());
  const shown = await preview.locator('tbody tr').evaluateAll((trs) =>
    trs.map((tr) => [...tr.querySelectorAll('td')].map((td) => td.textContent ?? '')),
  );
  await exp.getByRole('button', { name: 'Copy as command' }).click();
  const copied = await page.evaluate(() => (window as unknown as { copied: string[] }).copied);
  expect(copied).toHaveLength(1);
  const cmd = copied[0];
  expect(cmd).toMatch(/^authama export hosts /);
  expect(cmd).toContain('--format=json');

  const rows = JSON.parse(runCommand(cmd)) as Record<string, unknown>[];
  expect(rows.map((r) => Object.keys(r))).toEqual(rows.map(() => headings));
  expect(rows.map((r) => headings.map((h) => text(r[h])))).toEqual(shown);

  // A host format: the command writes the ssh_config of the same hosts.
  await exp.getByLabel('Format').selectOption('ssh_config');
  await exp.getByRole('button', { name: 'Copy as command' }).click();
  const hostCmd = (await page.evaluate(() => (window as unknown as { copied: string[] }).copied))[1];
  expect(hostCmd).toContain('--format=ssh_config');
  const config = runCommand(hostCmd);
  for (const a of ['app-web', 'app-db-maria', 'app-db-psql']) expect(config).toContain(`Host ${a}\n`);
  expect(config).not.toContain('Host hpc\n');
});
