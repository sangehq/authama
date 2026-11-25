# Owner binding

`authama owner` ties a home to the person and machine it belongs to: a PIN that guards secrets and
destructive changes, and a seal that notices when the state files are changed outside authama or
copied to another account or machine.

It is off until you run `authama owner init`. A home that is never bound behaves exactly as before.

## At a glance

| Command | What it does |
|---|---|
| `authama owner init` | keeps a sealing key in the OS's credential store and seals the state files |
| `authama owner status` | whether the home is bound, where the key is, whether a PIN and fingerprint unlock are on, and the seal's state |
| `authama owner verify` | checks the files against the seal and names each one changed, removed or added; exits non-zero if any |
| `authama owner reseal` | accepts the files as they are now, after a change you made yourself |
| `authama owner pin set` | sets a PIN, or changes it (asks for the current one first); without a terminal it reads them from stdin, one per line |
| `authama owner pin reset` | replaces a forgotten PIN without the old one, by proving the OS account |
| `authama owner pin remove` | turns the PIN off (asks for it first) |
| `authama owner fingerprint on\|off` | lets a fingerprint stand in for the PIN at the terminal (Linux, fprintd) |
| `authama owner forget` | unbinds the home: removes the key, the seal and the PIN, and keeps every other file |

Each of these except `pin reset` is also on the web UI's **Owner & PIN** page, previewed and
confirmed like any change. Two guards keep you from locking yourself out: a PIN is only set when
`owner pin reset` would work on this account and machine (the sealing key can be read), and
unbinding removes `owner.json` before the key, so an unbind that stops part-way leaves a home with no
PIN rather than one asking for a PIN nothing can reset. The reset stays at a terminal: it proves
the OS account, which a browser cannot.

## The sealing key

`owner init` makes a random 256-bit key and keeps it where the OS keeps secrets for the user:

| Platform | Store |
|---|---|
| macOS | the login Keychain, as the generic password `authama-owner-key` (written through `security -i`, so it never appears in the process list) |
| Linux | the Secret Service (GNOME Keyring, KWallet) through `secret-tool`, when a desktop session is running |
| Windows | a file in the state home encrypted with DPAPI for the current user |
| anywhere else | an owner-only (0600) file in the state home |

`AUTHAMA_OWNER_STORE=file` forces the file, for a machine with no keyring session (a server reached
over ssh, a container). A `--dev-root` sandbox always uses the file inside the sandbox, so a test
never touches the real Keychain.

> Every store above guards the key from other accounts, not from other programs running as you.
> On macOS the item is created without an access list, so any process of the same user can read it
> with `security find-generic-password` and no prompt; the Secret Service and DPAPI behave the same
> way. The seal proves the files were last written by authama under this account. It does not stop
> malware already running as you, which could rewrite them and re-seal.

## The seal

The seal is an HMAC-SHA256, made with the key, over each of these files' content and over the
binding (the OS user, its id and the machine's id):

`manifest.json`, `inventory.json`, `revoked.json`, `providers.json`, `tokens.json`, `authama.json`,
`.env`, `age-identity.txt` and `owner.json`.

authama reseals after each operation it performs. So when a file no longer matches, something other
than authama changed it, and authama says so:

- on the next command that changes anything, as a warning, and that command then **does not
  reseal**, so the outside change is not blessed along with authama's own;
- in `authama doctor`, which fails its verdict and reports it under `owner_seal` in `--json`;
- on the web UI's overview and doctor pages, as a banner.

The machine id is `/etc/machine-id` on Linux, the hardware UUID on macOS, and `MachineGuid` on
Windows. A home copied to another account or machine reports that it was sealed elsewhere, and is
never resealed silently there: `owner forget` and `owner init` bind it to the new place.

This is detection, not prevention. Anyone who can run programs as you can read your files and run
authama. What the seal adds is that a change made behind authama's back - by a sync tool, a restored
backup, another program, or a person editing a file - is named the next time authama runs, instead
of being acted on.

## The PIN

A PIN is 6 to 128 characters: digits, or a passphrase. A run such as `123456`, or one repeated
character, is refused. It is stored only as a salted Argon2id hash (t=3, 64 MiB, p=4, RFC 9106's
second recommended setting), in `owner.json`.

With a PIN set:

- **The web UI** asks for it after the sign-in link, before any page opens, and again on the
  confirmation of every change and job once five minutes have passed since it was last given.
- **The command line** asks for it before `token list --reveal`, `token show --reveal`,
  `token copy` and `ssh termius keys <alias>`, and before a destructive command: `delete`, `revoke`,
  `rotate`, `rollback`, `restore`, `prune`, `clean`, `recover`, `deploy`, and `keygen --force`.
  A `--dry-run` is not asked. A command that guards more than one step asks once.
- **Without a terminal** those commands refuse, rather than run unguarded.

Five wrong PINs in a row start a wait of 30 seconds, doubling with each further wrong one up to 15
minutes. The count is kept in the state home, so restarting the server or the command does not
reset it. Checks run one at a time under a lock (`pin.lock` in the state home), so guesses sent at
once, from browser tabs or several commands, are each counted, and a guess is recorded before it
is checked, so stopping the command mid-check does not skip the count.

### Forgotten PIN

`authama owner pin reset` replaces it without the old one. It must run at a terminal, read the key
from this account's own credential store (on macOS the Keychain may ask for your login password),
and run on the machine the home was bound on. That is the proof that you are the account the home
belongs to. The reset is on the record, and any lockout is cleared.

### A missing `owner.json`

Deleting `owner.json` does not switch the seal or the PIN off. While the seal, the attempt count or
a file-store key is still in the state home, the home counts as bound: `owner verify` and `doctor`
report that `owner.json` is missing, the web UI keeps every page behind `/unlock` and refuses the
PIN with the same message, and changes made from the command line warn and do not reseal. Restore
`owner.json` from a backup, or run `authama owner forget`, which asks before removing the seal and
the sealing key (there is no PIN left to ask for) and leaves a home that was never bound. The same
command finishes a `forget` that stopped part-way.

### A seal removed

Deleting `owner.json`, `seal.json` and `pin-attempts.json` together leaves nothing in the homes
to say the home was bound, but the sealing key is still in the macOS Keychain or the Secret Service.
`owner verify`, `doctor` and `serve` (when it starts a server) ask the credential store as well as
the files, and refuse such a home: they say the seal was removed, what is gone and what is left, and
what to do. Each asks the store once; the web UI never asks it per page. A bound home whose
`seal.json` is gone is refused the same way, with `owner reseal` as the way back.

What to do, as the message says:

- **You did not remove them:** put `owner.json` (config home), `seal.json` and `pin-attempts.json`
  (state home) back from a backup of the home (Time Machine, restic, a copy), then run
  `authama owner verify`. authama's snapshots and bundles do not carry them.
- **You meant to start again:** `authama owner init`, then `authama owner pin set`. `init` replaces
  the key left behind.
- **You meant to unbind it:** `authama owner forget` removes the key left behind. Nothing else shows
  the key was yours, so it asks you to type `forget`; a script passes `--confirm forget`. By hand
  (the account is the config home's path):

  | store | command |
  |---|---|
  | macOS Keychain | `security delete-generic-password -a '<config home>' -s authama-owner-key` |
  | Linux Secret Service | `secret-tool clear service authama-owner-key account '<config home>'` |
  | Windows | authama keeps nothing in Credential Manager: the key is the DPAPI file `owner.key.dpapi` in the state home; `Remove-Item` it |
  | the owner-only file | `rm <state home>/owner.key` |

With `AUTHAMA_OWNER_STORE=file`, or in a `--dev-root` sandbox, the credential store is not asked.

### Why Argon2id

Argon2id is memory-hard: every guess at a stolen hash costs 64 MiB of memory as well as time, which
is what makes a GPU or an ASIC slow at it, where an iterated hash such as PBKDF2 costs only time.
A PIN set by an earlier release is a PBKDF2-HMAC-SHA256 hash at 600,000 iterations; it still
unlocks, and the next time it is given correctly it is rehashed as Argon2id. The stored hash names
its algorithm and cost, so this can change again without invalidating anyone's PIN.

## Fingerprint unlock

`authama owner fingerprint on` makes the command line ask for a fingerprint first, falling back to
the PIN on any failure or cancel. It is a convenience in front of the PIN, never a replacement:

| Platform | How | Needs |
|---|---|---|
| macOS | Touch ID, through `authama-touchid` | the helper beside `authama` (or in `Authama.app`), and an enrolled finger |
| Windows | Windows Hello, through Windows PowerShell 5.1 | Hello set up in Settings > Accounts > Sign-in options |
| Linux | `fprintd-verify` | `fprintd` and an enrolled finger |

authama is built without cgo, so it cannot call LocalAuthentication or WinRT itself.

- **macOS:** `authama-touchid` is a small Swift program (`src/helpers/touchid/main.swift`) that
  asks for Touch ID with the biometrics-only policy and answers with its exit status. `make build`
  builds it beside `authama` on a Mac with Xcode's command line tools, and `Authama.app` carries it.
  It is ad-hoc signed, not notarized: a copy downloaded through a browser is quarantined, so allow it
  once in System Settings > Privacy & Security, or remove the flag with
  `xattr -d com.apple.quarantine authama-touchid`. A build from source is not quarantined.
  Notarizing it needs a Developer ID.
- **Windows:** authama runs `powershell.exe` with an encoded script that calls WinRT's
  `UserConsentVerifier`. No helper is installed. The prompt can open behind the terminal, in which
  case it shows in the taskbar.

Over SSH (`SSH_CONNECTION` set) and in Remote Desktop, macOS and Windows report it unavailable and
ask for the PIN. The web UI always uses the PIN.

## History and events

Every owner command is on the record in [`history`](history.md), as `owner.init`, `owner.reseal`,
`owner.pin.set`, `owner.pin.reset`, `owner.pin.remove`, `owner.biometric` or `owner.forget`. No
event carries a PIN or the key.

---

[← Docs index](../../README.md#documentation)
