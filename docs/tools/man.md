# `man`

`authama man` writes a manual page, in troff for section 1, for every command, so `man authama` and
`man authama-ssh-deploy` work like any other tool's.

## Writing the pages

```sh
authama man                           # into ./man
authama man ~/.local/share/man/man1   # where man(1) finds them on most systems
make man                              # from a checkout: into build/man
```

There is one page per command, named for its path: `authama.1`, `authama-ssh.1`,
`authama-ssh-deploy.1`, `authama-token-set.1`, and so on. The hidden top-level `vm` and `audit`,
kept for older `~/.ssh/config` files and scheduled jobs, get no page of their own.

## The layout

Every page, and every command's `--help`, uses the same sections in the same order:

| Section | Holds |
|---|---|
| NAME | the command and its one-line summary |
| SYNOPSIS | how to call it: `authama ssh deploy <[profile/]key> [target] [options]` |
| DESCRIPTION | what it does |
| ALIASES | the other names it answers to (`ls` for every `list`, `rm` for every `delete`, `auth` for `token`) |
| COMMANDS | its subcommands, for a group |
| OPTIONS | its own options |
| GLOBAL OPTIONS | the options every command takes (`--dev-root`, `--no-dotenv`) |
| EXAMPLES | commands to copy |
| SEE ALSO | the parent command and the hosted manual |

`authama help <command>`, `authama <group> help <command>` and `<command> --help` print the same text
as the page.

---

[← Docs index](../../README.md#documentation)
