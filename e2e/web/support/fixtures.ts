// The test fixture every spec uses: a page already signed in to the server the
// global setup started, which fails the test on any console error or failed
// same-origin request. The UI loads nothing remote and runs under a strict CSP,
// so a console error is a broken asset, a CSP violation or a script bug.
import { test as base, expect, type Page } from '@playwright/test';

export const bundle = (): string => process.env.AUTHAMA_E2E_BUNDLE ?? '';

// The pages the specs walk, in nav order, with the heading each one carries.
export const pages: { path: string; heading: string }[] = [
  { path: '/', heading: 'Overview' },
  { path: '/profiles', heading: 'Profiles' },
  { path: '/naming', heading: 'Naming' },
  { path: '/hosts', heading: 'Hosts' },
  { path: '/hosts/hpc', heading: 'hpc' },
  { path: '/ssh-config', heading: 'SSH config' },
  { path: '/keys', heading: 'Keys' },
  { path: '/keys/work/work_hpc-ed25519', heading: 'work_hpc-ed25519' },
  { path: '/snapshots', heading: 'Snapshots' },
  { path: '/history', heading: 'History' },
  { path: '/events', heading: 'Events' },
  { path: '/doctor', heading: 'Doctor' },
  { path: '/audit', heading: 'Audit' },
  { path: '/tokens', heading: 'Auth tokens' },
  { path: '/security', heading: 'Owner & PIN' },
  { path: '/repair', heading: 'Repair' },
  { path: '/settings', heading: 'Settings' },
  { path: '/about', heading: 'About' },
  { path: '/about/licenses', heading: 'Third-party licences' },
];

async function signIn(page: Page): Promise<void> {
  const token = process.env.AUTHAMA_E2E_TOKEN;
  if (!token) throw new Error('AUTHAMA_E2E_TOKEN is not set: the global setup did not run');
  const res = await page.goto(`/?token=${encodeURIComponent(token)}`);
  expect(res?.status(), 'signing in with the launch token').toBe(200);
  // The token is exchanged for a cookie and stripped from the address.
  expect(new URL(page.url()).searchParams.has('token')).toBe(false);
}

export const test = base.extend<{ problems: string[] }>({
  // Checked when the test ends, so every spec gets it without asking.
  problems: async ({}, use) => {
    const problems: string[] = [];
    await use(problems);
    expect(problems, 'console errors or failed requests on the page').toEqual([]);
  },
  page: async ({ page, problems }, use) => {
    page.on('console', (m) => {
      // An error status is reported below, by URL, and asserted by the spec
      // that expects it; Chromium's own line for it says less.
      if (m.type() === 'error' && !m.text().startsWith('Failed to load resource: the server responded with a status of')) {
        problems.push(`console: ${m.text()}`);
      }
    });
    page.on('response', (r) => {
      // A page may answer 4xx on purpose (401, 404, 409); an asset never may.
      // A page the side panel loads is a page too: it carries X-Authama-Panel.
      const page = r.request().isNavigationRequest() || r.request().headers()['x-authama-panel'] === '1';
      if (r.status() >= 400 && !page) {
        problems.push(`response: ${r.status()} ${r.url()}`);
      }
    });
    page.on('pageerror', (e) => problems.push(`pageerror: ${e.message}`));
    // The tour starts by itself on a browser's first overview; every spec but
    // the tour's own starts as a returning user, so it never covers a control.
    await page.addInitScript(() => {
      try {
        window.localStorage.setItem('authama.tour', 'seen');
      } catch {
        // no storage: the tour never starts by itself anyway
      }
    });
    page.on('requestfailed', (r) => {
      // A reload during a live job aborts its event stream; that is the page working.
      if (!r.url().endsWith('/events')) problems.push(`requestfailed: ${r.url()} ${r.failure()?.errorText}`);
    });
    await signIn(page);
    await use(page);
  },
});

export { expect };
