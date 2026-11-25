# Signed bundles

A bundle (`authama ssh bundle`) or a setup export (`authama setup export --out`) can be signed with an SSH key.
The signature proves the file is the one the key's owner wrote. Restore and import check it before they decrypt or
read anything. This closes the gap the risky-options check names: on its own, a bundle does not prove who made it.

## Design

| Question | Answer |
| --- | --- |
| Format | A detached SSHSIG signature, `<file>.sig`, next to the file: what `ssh-keygen -Y sign` writes. |
| Tool | `ssh-keygen`, which every supported OS ships. It signs with a key file, asks for its passphrase at the terminal, and signs through the agent when the key is loaded there. |
| Namespace | `authama-bundle`. A signature made for git (`-n git`) or any other namespace never passes as a bundle signature, and a bundle signature never passes as a commit signature. |
| What is signed | The file's bytes as written: the encrypted `.age` bundle, or the export file. Nothing is decrypted to sign or check. |
| Who is trusted | `bundle_signers` in the config home, in `allowed_signers` format, with each line limited to `namespaces="authama-bundle"`. It is kept apart from git's `allowed_signers`, so trusting a key for commits does not trust it for bundles. |
| Self-trust | `--sign-key` adds the key's own public line to this home's `bundle_signers`, so a bundle restores here with `--require-signature`. It prints the line another machine needs. |

## Checking

Restore and import check `<file>.sig` before anything else:

| What is found | Without `--require-signature` | With `--require-signature` |
| --- | --- | --- |
| No `.sig` | restored, as before | refused |
| A signature that does not match the file, or another namespace | refused | refused |
| A good signature by a key `bundle_signers` does not trust | restored, printing the key's fingerprint | refused |
| A good signature by a trusted signer | restored, naming the signer | restored, naming the signer |

A file changed after it was signed is always refused. `--signers FILE` checks against another `allowed_signers`
file instead of `bundle_signers`.

The web UI's **Restore bundle** job checks the same way against `bundle_signers`. Its review has a **Require a
trusted signature** option, which is `--require-signature`. Signing stays in the terminal, because `ssh-keygen`
may ask for the key's passphrase. A setup import uploaded to the web arrives without its `.sig`, so it is checked
in the terminal only.

## Use

```sh
# Sign as you write. A key with a passphrase is asked for it, or the agent signs.
authama ssh bundle -o ~/Backups --sign-key ~/.ssh/profiles/personal/id_ed25519
authama setup export --encrypt --out setup.age --sign-key ~/.ssh/profiles/personal/id_ed25519

# On the new machine, add the line the command printed to bundle_signers
# in the config home:
#   me@laptop namespaces="authama-bundle" ssh-ed25519 AAAA...

# Then require it:
authama ssh restore ~/Backups/authama-<stamp>.age -i age-identity.txt --require-signature
authama setup import setup.age --require-signature
```

Copy the `.sig` with the file. A bundle without its `.sig` restores as unsigned, and only `--require-signature`
refuses it. `ssh restore` and `setup import` of a full export warn when a file is unsigned, or signed by a key
no `bundle_signers` line trusts, and `ssh restore` repeats the warning in its question; the web UI's restore job
prints it too. A plain setup document is not warned about.

## Code

`services/bundlesig` holds `Sign`, `Check`, `Trust` and `SignerLine`. Its tests sign with throwaway keys and cover
the trusted, untrusted, unsigned and tampered cases, plus a git-namespace signature. `cli/bundlesign.go` adds the
flags, and the web restore job calls `bundlesig.Check`.

---

[← Docs index](../../README.md#documentation)
