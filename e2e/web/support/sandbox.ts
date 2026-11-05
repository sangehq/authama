// Global setup: a throwaway authama home, seeded, with `authama serve` running
// against it. Nothing here touches the real ~/.ssh or ~/.config/authama: HOME and
// both XDG homes point into a fresh temp directory, every AUTHAMA_* (and legacy
// SSH_MANAGER_*/SSHMGR_*) variable from the caller is dropped, and setup refuses
// to go on if the resolved ~/.ssh is not inside that directory.
//
// The seeded home is the repo's shipped example manifest (src/config/manifest.json,
// also the e2e smoke's fixture), reconciled so every key exists, plus one
// encrypted bundle so the restore job has something real to stream.
//
// What the tests need is handed over in the environment, which Playwright's
// workers inherit from this process: AUTHAMA_E2E_URL (the server's origin),
// AUTHAMA_E2E_TOKEN (the sign-in token) and AUTHAMA_E2E_BUNDLE (the bundle's
// file name).
import { execFileSync, spawn, type ChildProcess } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, readdirSync, realpathSync, copyFileSync, rmSync } from 'node:fs';
import { homedir, tmpdir } from 'node:os';
import { dirname, join, relative, resolve, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '..', '..', '..');

// The binary `make build` writes; AUTHAMA_BIN overrides it.
// `make build` writes build/{host-os}/authama: the OS folder is macos, not
// darwin, and windows' binary ends .exe.
const hostOS = process.platform === 'darwin' ? 'macos' : process.platform === 'win32' ? 'windows' : process.platform;
const bin = process.env.AUTHAMA_BIN
  ? resolve(process.env.AUTHAMA_BIN)
  : join(repo, 'build', hostOS, process.platform === 'win32' ? 'authama.exe' : 'authama');

function sandboxEnv(home: string): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = {};
  for (const [k, v] of Object.entries(process.env)) {
    if (/^(AUTHAMA_|SSH_MANAGER_|SSHMGR_|XDG_)/.test(k) || k === 'SSH_AUTH_SOCK') continue;
    env[k] = v;
  }
  return {
    ...env,
    HOME: home,
    USERPROFILE: home,
    XDG_CONFIG_HOME: join(home, '.config'),
    XDG_STATE_HOME: join(home, '.local', 'state'),
    AUTHAMA_NO_DOTENV: '1',
    AUTHAMA_AUTO_PIN: '0', // no network reach from a test
    AUTHAMA_AGE_IDENTITY_FILE: join(home, '.config', 'authama', 'age-identity.txt'),
  };
}

function run(env: NodeJS.ProcessEnv, file: string, args: string[], allowFail = false): string {
  try {
    return execFileSync(file, args, { env, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  } catch (err) {
    const e = err as { stdout?: string; stderr?: string; message: string };
    if (allowFail && e.stdout) return e.stdout;
    throw new Error(`${file} ${args.join(' ')} failed: ${e.message}\n${e.stdout ?? ''}${e.stderr ?? ''}`);
  }
}

function inside(parent: string, child: string): boolean {
  const rel = relative(parent, child);
  return rel !== '' && !rel.startsWith('..') && !isAbsolute(rel);
}

async function waitForFile(path: string, server: ChildProcess, log: () => string, ms: number): Promise<string> {
  const deadline = Date.now() + ms;
  while (Date.now() < deadline) {
    if (server.exitCode !== null) throw new Error(`authama serve exited ${server.exitCode}:\n${log()}`);
    if (existsSync(path)) {
      const text = readFileSync(path, 'utf8').trim();
      if (text) return text;
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(`authama serve wrote no sign-in address in ${ms}ms:\n${log()}`);
}

export default async function globalSetup(): Promise<() => Promise<void>> {
  if (!existsSync(bin)) throw new Error(`no binary at ${bin}: run \`make build\` first, or set AUTHAMA_BIN`);
  for (const tool of ['ssh-keygen', 'age', 'age-keygen']) {
    try {
      execFileSync('sh', ['-c', `command -v ${tool}`], { stdio: 'ignore' });
    } catch {
      throw new Error(`${tool} is required to seed the sandbox home`);
    }
  }

  const home = realpathSync(mkdtempSync(join(tmpdir(), 'authama-web-e2e-')));
  if (home === realpathSync(homedir())) throw new Error('refusing to use the real home as the sandbox');
  let server: ChildProcess | undefined;
  try {
    const env = sandboxEnv(home);
    const cfg = join(home, '.config', 'authama');
    const state = join(home, '.local', 'state', 'authama');

    run(env, bin, ['init']);
    // Where this binary resolves its homes, asked rather than assumed: a sandbox
    // whose ~/.ssh resolved outside the temp dir would write over a real one.
    // A fresh home has drift until it is reconciled, so doctor exits non-zero here.
    const doctor = run(env, bin, ['doctor'], true);
    for (const label of ['home:', 'state:', 'ssh:']) {
      const line = doctor.split('\n').find((l) => l.startsWith(label));
      const path = line?.slice(label.length).trim().split(/\s+/)[0];
      if (!path || !inside(home, path)) throw new Error(`the sandbox's ${label} ${path} is not inside ${home}`);
    }

    copyFileSync(join(repo, 'src', 'config', 'manifest.json'), join(cfg, 'manifest.json'));
    run(env, bin, ['ssh', 'reconcile']);
    // A tokens.json, so the editor (and its JSON check) is on the tokens page.
    run(env, bin, ['token', 'init', '--target', 'npm']);
    run(env, 'age-keygen', ['-o', join(cfg, 'age-identity.txt')]);
    const recipient = run(env, 'age-keygen', ['-y', join(cfg, 'age-identity.txt')]).trim();
    run(env, bin, ['ssh', 'bundle', '--recipient', recipient]);
    const bundle = readdirSync(join(state, 'dist')).find((f) => f.endsWith('.age'));
    if (!bundle) throw new Error(`ssh bundle wrote no .age file into ${join(state, 'dist')}`);

    const tokenFile = join(home, 'serve-url');
    let output = '';
    server = spawn(bin, ['serve', '--no-open', '--port', '0', '--token-file', tokenFile], {
      env,
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    server.stdout?.on('data', (d) => (output += d));
    server.stderr?.on('data', (d) => (output += d));
    const signIn = new URL(await waitForFile(tokenFile, server, () => output, 15_000));

    process.env.AUTHAMA_E2E_URL = signIn.origin;
    process.env.AUTHAMA_E2E_TOKEN = signIn.searchParams.get('token') ?? '';
    process.env.AUTHAMA_E2E_BUNDLE = bundle;
    process.env.AUTHAMA_E2E_HOME = home;
    if (!process.env.AUTHAMA_E2E_TOKEN) throw new Error(`no token in the sign-in address ${signIn.origin}`);
  } catch (err) {
    server?.kill('SIGTERM');
    if (!process.env.AUTHAMA_E2E_KEEP) rmSync(home, { recursive: true, force: true });
    throw err;
  }
  const running = server;

  return async () => {
    running.kill('SIGTERM');
    await new Promise((r) => (running.exitCode !== null ? r(null) : running.once('exit', r)));
    if (process.env.AUTHAMA_E2E_KEEP) {
      console.log(`kept the sandbox home at ${home}`);
    } else {
      rmSync(home, { recursive: true, force: true });
    }
  };
}
