package cli

import "github.com/spf13/cobra"

// commandLong are the --help descriptions for the commands that do not set
// their own Long, keyed by the path under the root ("ssh host edit"), as
// commandExamples are. A command's own Long wins.
// TestEveryCommandHasADescription keeps every visible command covered.
var commandLong = map[string]string{
	"api token": "API tokens let a program read authama's API (served by `authama serve` at /api/v1/ when api.enabled\n" +
		"is on). `create` prints a token once - authama keeps only its hash - `list` shows names, ages and\n" +
		"expiry but never a token, and `revoke` stops one at once.",
	"api token create": "Make a read-only API token called <name> and print it once; authama keeps only its hash, so copy it\n" +
		"now. It expires after --expires days (90 by default; 0 never expires). Programs send it as\n" +
		"`Authorization: Bearer ama_...`.",
	"api token list": "List the API tokens: name, scopes, when each was made, when it expires and when it was last used.\n" +
		"The tokens themselves are never shown; they cannot be, since only their hashes are kept.",
	"api token revoke": "Revoke the API token called <name>. Calls that use it are refused from that moment; the programs\n" +
		"holding it need a new one from `api token create`.",
	"checklist show": "Show one key's or token's checklist: every place it is used, each step of rotating or revoking it,\n" +
		"which steps are done, and the command for each step left. Name a key as profile/key and a token as\n" +
		"token:<tool>/<host>.",
	"companion": "Answers for a companion tool such as sange, which drives authama and needs to know what this build\n" +
		"supports. `companion info` prints the name, version, contract versions, capabilities and homes, and\n" +
		"changes nothing.",
	"doctor": "Check this machine and home: the tools authama needs, file permissions, the ssh agent, known_hosts,\n" +
		"drift between the manifest and ~/.ssh, dangling or loose keys, VM hosts and token files others can\n" +
		"read. It changes nothing unless --fix, which repairs permissions, removes stale authama pins and\n" +
		"runs a VM sync, backing up before it removes anything (--dry-run shows it first). It exits non-zero\n" +
		"when something blocks; --strict also fails on every dangling-key state and on readable token files,\n" +
		"for CI.",
	"export schedule install": "Register one daily job with this system's scheduler (launchd, systemd or cron, schtasks) that runs\n" +
		"`export schedule run`, which writes every export schedule. Run it once; adding or removing schedules\n" +
		"later needs no reinstall. It is refused in dev mode, since the job is machine-wide.",
	"export schedule list": "List the export schedules: each saved selection, and the folder and format it is written to.",
	"export schedule uninstall": "Remove the daily job `export schedule install` registered. The schedules themselves are kept;\n" +
		"`export schedule run` still writes them by hand.",
	"export selections list": "List the saved export selections from both stores: the shared one in the manifest and your own under\n" +
		"the state home. Each shows its name, store and what it selects; --json prints them as JSON.",
	"export selections show": "Print a saved selection as JSON, the same shape `export --selection` and the Export page read. Which\n" +
		"store it came from goes to stderr, so stdout can be saved as a file.",
	"history show": "Show one operation from the history: what ran, when, from where (CLI, web or a job), its result, its\n" +
		"events, and what it changed in the manifest. Name it by the OP id `authama history` lists, or a\n" +
		"unique prefix of it.",
	"init": "Create the authama home - the config home with its manifest, inventory and .env, and the state home\n" +
		"- or bring an existing one up to date without touching what is in it. Safe to run again. --force\n" +
		"resets manifest.json, inventory.json and .env to their defaults, copying the current ones into the\n" +
		"state home first unless --no-backup; it asks before it does.",
	"migrate": "Copy a home made before 0.1.1 (ssh-manager or sshmgr) into the authama home. The old home is kept\n" +
		"and marked as migrated, never deleted. When an authama home exists too, --force backs it up and\n" +
		"replaces it.",
	"owner init": "Bind this home to you and this machine: authama makes a random sealing key, keeps it where the OS\n" +
		"keeps your secrets (the login Keychain, the Secret Service, a DPAPI file, or else an owner-only file\n" +
		"in the state home), and seals the state files so a change made outside authama is noticed. You can\n" +
		"then set a PIN with `owner pin set`.",
	"owner pin": "With a PIN set, the web UI asks for it after the sign-in link and again on confirmations, and the\n" +
		"command line asks before revealing or copying a token value and before destructive commands (delete,\n" +
		"revoke, rotate, rollback, restore, prune, clean, recover, deploy, keygen --force). `set` sets or\n" +
		"changes it, `remove` turns it off (asking for it first), and `reset` replaces a forgotten one by\n" +
		"proving your OS account instead.",
	"owner pin remove": "Turn the owner PIN off. It asks for the current PIN first; what it guarded is then guarded by the\n" +
		"bound home and the web sign-in alone.",
	"owner status": "Show whether this home is bound to you and this machine, since when, where its sealing key is kept,\n" +
		"whether a PIN and fingerprint unlock are on, and whether the seal still matches the state files.",
	"settings init": "Write a starter authama.json in the config home: the schema reference and version, with every\n" +
		"setting at its default, ready to edit. It will not replace an existing file without --force.",
	"settings path": "Print where authama keeps things: the config home, the state home, the .env file and authama.json.\n" +
		"--json prints them for a script.",
	"settings show": "Print authama.json as it is on disk. --effective prints every setting instead, with the value in\n" +
		"effect and where it came from: the environment, .env, authama.json or the default.",
	"settings unset": "Take one setting out of authama.json, so the environment, .env or the default decides it again. Name\n" +
		"it as section.name, as `settings show --effective` lists it.",
	"settings validate": "Check authama.json (or FILE) against the schema - unknown keys, wrong types, invalid values - and\n" +
		".env's syntax and permissions. It exits non-zero when anything is wrong and changes nothing.",
	"setup export": "Write this setup as a document another machine can import. By default it is secrets-free JSON: the\n" +
		"manifest, providers, settings without this machine's paths, and the token config naming the\n" +
		"variables tokens come from, never a value. --encrypt writes a full export instead: every private\n" +
		"key, the inventory and the token values too, in an age file encrypted with a passphrase (needs\n" +
		"--out, and the owner PIN when one is set). --sign-key also writes <file>.sig, which `setup import`\n" +
		"checks.",
	"setup import": "Merge a setup into this home: a setup document, an `export pack` zip (its setup.json) or an\n" +
		"encrypted full export. It shows what it would add, keep and replace first. --on-conflict settles\n" +
		"what differs: keep (the default) leaves what is here, replace takes the import's and keeps a copy of\n" +
		"every key file and .env it replaces, fail writes nothing. A <file>.sig beside it is checked before\n" +
		"anything is read; --require-signature also refuses a file with no trusted signature.",
	"ssh audit": "Report on every key: where it is deployed, how close it is to its rotation date, and the recent\n" +
		"activity behind it. --csv prints the deployments, one row per key and target. --notify also fires\n" +
		"the desktop alert, at most as often as its cadence allows; `ssh notify install` runs that daily.",
	"ssh config": "authama writes ~/.ssh/config and the per-profile config files from the manifest. `render` writes\n" +
		"them, `check` reports drift without writing, `show` prints them (or what ssh resolves for one\n" +
		"alias), `adopt` moves hand-written Host blocks that duplicate managed hosts out of the way, and\n" +
		"`export` writes the config as one file for another tool.",
	"ssh config check": "Compare the config files on disk with what the manifest renders. It writes nothing, prints what\n" +
		"differs and exits 1 on drift, so a script or CI can gate on it; `ssh config render` brings them back\n" +
		"in line.",
	"ssh config render": "Write ~/.ssh/config and each profile's config file from the manifest, after showing what would\n" +
		"change. A snapshot of ~/.ssh is taken first; hand-written lines outside authama's blocks are kept.\n" +
		"--dry-run shows the change and writes nothing.",
	"ssh config show": "Print the config authama renders for every profile. With an alias, print what `ssh -G` resolves for\n" +
		"it instead: the hostname, user, port, identity file and options ssh will actually use.",
	"ssh deploy": "Install a key's public half on its targets: the hosts that use it, or the one target named. It\n" +
		"connects through the host's alias, so its ProxyJump or ProxyCommand applies (--direct uses the\n" +
		"manifest's hostname, user and port instead), or through the provider's API for a git host. The\n" +
		"inventory records each deployment.",
	"ssh diff": "Preview what `ssh reconcile` would change: how the config files differ from what the manifest\n" +
		"renders, and which keys the manifest wants that are not on disk yet. It writes nothing.",
	"ssh expiry": "A table of every key, by profile/key, with its state (ok, due soon, overdue), the date it expires\n" +
		"and the days left, from each key's lifetime and when it was last minted or rotated.",
	"ssh host": "A host is one server in a profile: its alias, hostname, user, port and key. `add` adds one (or names\n" +
		"it by the naming rules), `apply` adds or updates many from compact specs, `edit` changes one, and\n" +
		"`delete` removes it with its config block and pins.",
	"ssh host edit": "Change a host in a profile: its hostname, user, port, key, provider, proxy or tags. Only the flags\n" +
		"given change, and the config is rendered again. A key the host now names that is not on disk yet is\n" +
		"minted by `ssh reconcile`.",
	"ssh key": "A profile's keys: `add` declares and mints one, `list` shows every key with its state, `adopt`\n" +
		"manages a key where it already is, `delete` removes one no host uses, `revoke` takes one off every\n" +
		"target for good, `lifetime` sets when rotation is due, `reserve` keeps a loose key on purpose, and\n" +
		"`root` moves the directory that holds the profiles.",
	"ssh key list": "Every key with its fingerprint, the hosts that use it, its expiry and state (new, due, deployed,\n" +
		"retired). Name a profile, alias or key to narrow it; --new and --since show only the recent ones,\n" +
		"and --json and --csv print them for a script.",
	"ssh key unreserve": "Drop a loose key's reservation, so `doctor` lists it as loose again. The key file is left where it\n" +
		"is.",
	"ssh keygen": "Mint the keys a profile or host needs that are missing, into their place under ~/.ssh, and pin the\n" +
		"reachable hosts' host keys into known_hosts (--no-pin skips that). --force replaces existing keys,\n" +
		"moving each to old/ after an encrypted backup; --passphrase protects new keys.",
	"ssh knownhosts": "authama keeps one trust store, ~/.ssh/known_hosts. `init` pins every reachable host on first use and\n" +
		"reports the fingerprints; `pin` scans a host (or --all) with ssh-keyscan and asks before trusting\n" +
		"what it found.",
	"ssh knownhosts pin": "Scan a host's keys with ssh-keyscan, show their fingerprints, and add them to ~/.ssh/known_hosts\n" +
		"once you confirm. Name a manifest host or any host (with --port for a non-standard one), or --all\n" +
		"for every host in the manifest.",
	"ssh list": "A tree of profiles and their hosts, each with its hostname, provider, key and deployment state.\n" +
		"Filter by --profile, --tag, --provider or provider --type (vcs for git hosts); --csv prints one row\n" +
		"per host.",
	"ssh load": "Add a profile's private keys to the running ssh agent (ssh-add, with the keychain on macOS). Keys\n" +
		"not on disk are skipped and named; it fails when ssh-add adds nothing, which usually means no agent\n" +
		"is running (SSH_AUTH_SOCK).",
	"ssh naming show": "Show the naming rules in effect for aliases and key names, the template each follows, and each\n" +
		"profile's code and any rules of its own.",
	"ssh net": "Check each host's connection: whether its port answers, and whether it needs a VPN that is down.\n" +
		"Name a profile or alias to check fewer. It exits non-zero when a host behind a VPN cannot be\n" +
		"reached.",
	"ssh notify": "Desktop notifications for keys that are due. `install` registers a daily job that runs `ssh audit\n" +
		"--notify`; `test` fires one now; `doctor` says whether notifications can show here and why not;\n" +
		"`send` and `ping` show any message, from this process or through the running web UI.",
	"ssh notify doctor": "Say whether desktop notifications can show on this machine: which backend would carry them, whether\n" +
		"it is installed and allowed, and what to change when it is not. It sends nothing.",
	"ssh notify install": "Register a daily job with this system's scheduler (launchd, systemd or schtasks) that runs `ssh\n" +
		"audit --notify`, so a key coming due shows a desktop notification. It asks first, and is refused in\n" +
		"dev mode since the job is machine-wide.",
	"ssh notify test": "Fire one test desktop notification now, to check that notifications show. When no backend is found\n" +
		"it says what to install or allow.",
	"ssh profile": "A profile groups hosts that share an identity - a client, an employer, your own servers - with its\n" +
		"keys under ~/.ssh/profiles/. `add` makes one, `edit` changes its category, code, key scope or key\n" +
		"comment, and `delete` removes it with its config blocks and host pins.",
	"ssh profile add": "Add a profile. By default each host gets its own key (per_service); --shared (or --key-scope shared\n" +
		"with --key-name) uses one key for every host. --category files it under profiles/<category>/, --code\n" +
		"sets the organisation code its server aliases start with, and --key-comment sets the comment of keys\n" +
		"minted from now on.",
	"ssh profile edit": "Change a profile: move it to another category (its key directory and inventory follow), set its\n" +
		"code, switch between one key per host and one shared key, or change the key comment template. Only\n" +
		"the flags given change.",
	"ssh providers": "List the providers hosts can deploy keys through (GitHub, GitLab, web panels and the like): each\n" +
		"one's category, kind, the environment variable its API token comes from, and whether that token is\n" +
		"set. --export writes the shipped catalog to providers.json in the config home to customise.",
	"ssh reconcile": "Make ~/.ssh match the manifest: mint the keys it wants that are missing, render the config files,\n" +
		"and pin reachable hosts' host keys. It shows the plan and asks first, and takes a snapshot of ~/.ssh\n" +
		"before it writes; --dry-run shows the plan and writes nothing.",
	"ssh recover": "Print a break-glass snippet for getting back into a server when a key is lost: the public key to put\n" +
		"back in authorized_keys and the commands to do it from a console. With no key, print the standalone\n" +
		"fixkeys tool for every key.",
	"ssh rollback": "Undo a rotation: move a key's previous version back from old/, deploy it to the key's targets again\n" +
		"and revoke the key the rotation put in. It asks first, snapshots ~/.ssh, and exits non-zero if it\n" +
		"could not finish.",
	"ssh show": "Everything authama knows about a profile, host or key: the files on disk, the config blocks rendered\n" +
		"for it, the known_hosts pins and where each key is deployed. This is the inventory view; `ssh view`\n" +
		"shows one host as ssh will use it.",
	"ssh snapshots": "authama copies the files it manages in ~/.ssh before each change. `list` shows the snapshots,\n" +
		"`restore` puts a snapshot's config back (private keys are left as they are), and `prune` deletes old\n" +
		"ones.",
	"ssh snapshots list": "List the snapshots of ~/.ssh, oldest first, each a tar.gz in the state home with its size. --csv\n" +
		"prints one row each.",
	"ssh snapshots prune": "Delete old snapshots, keeping the --keep most recent (10 by default).",
	"ssh structure dismiss": "Hide structure suggestions you mean to leave as they are, by the ids `ssh structure list` shows;\n" +
		"they stop appearing in the list and the web UI. --undo shows them again.",
	"ssh structure list": "List the moves authama suggests for keys outside the expected layout (~/.ssh/profiles/<profile>/),\n" +
		"each with its id, kind, path and what approving it would do. --all includes dismissed ones; --json\n" +
		"prints them for a script.",
	"ssh structure uninstall": "Remove the background job `ssh structure install` registered, which checks the layout and raises\n" +
		"suggestions. Suggestions already found are kept. It is refused in dev mode, since the job is\n" +
		"machine-wide.",
	"ssh view": "One host as ssh will connect to it: the resolved config (what `ssh -G` reports), the key it offers\n" +
		"and whether that key is on disk, and whether it is deployed to the host. Name a host's alias, or a\n" +
		"profile to see each of its hosts.",
	"ssh vm add": "Add a local VM as a host in a profile. Its address is not stored: at connect time ssh asks the\n" +
		"hypervisor for it (and starts the VM if it is stopped), so a new IP on each boot does not matter.\n" +
		"Name the hypervisor and the VM as it lists it; the key is <profile>_<alias>-<algo> unless --key-\n" +
		"name.",
	"ssh vm docker-context": "Create or update a Docker context called <alias> that reaches the VM host's Docker engine over\n" +
		"ssh://<alias>, through the managed config. Needs the docker CLI.",
	"ssh vm list": "List every enabled hypervisor's VMs with their state, and which manifest host uses each. A\n" +
		"hypervisor whose CLI does not answer within 5 seconds is reported as timed out.",
	"token init": "Write a starter token config, tokens.json in the config home (or FILE), from the shipped templates:\n" +
		"an entry per tool to fill in with the environment variable each token comes from, then `token\n" +
		"apply`. --target starts from those tools' templates only. It never writes over an existing file.",
	"token list": "List the tokens installed in the tools' credential files - Composer's auth.json, .npmrc, Docker's\n" +
		"config, git credentials, .netrc and the rest - with their hosts and masked values. --target narrows\n" +
		"the tools, --project reads a project's own files, and --reveal shows the values after asking.",
	"version": "Print the authama version, the commit it was built from and its build date.",
	"webhook add": "Add a webhook endpoint called <name> (https only) that receives authama's events as signed POSTs:\n" +
		"each a CloudEvent, signed as Standard Webhooks describes (webhook-id, webhook-timestamp, webhook-\n" +
		"signature). The signing secret is printed once; verify each request with it. --event sends only the\n" +
		"event types named, by prefix.",
	"webhook deliver": "Send every enabled endpoint the events due to it now, including failed ones whose backoff has\n" +
		"passed, and report each delivery. Use it to deliver without `serve` running, from a job or by hand.\n" +
		"It exits non-zero when a delivery failed.",
	"webhook disable": "Stop sending to an endpoint. It keeps its secret and its place in the event stream, so `webhook\n" +
		"enable` resumes where it stopped.",
	"webhook enable": "Send to a disabled endpoint again, starting from the first event it has not received.",
	"webhook list": "List the webhook endpoints: name, URL, the events each takes, whether it is on, and when it was last\n" +
		"sent to and with what result. Secrets are never shown; --json prints the list for a script.",
	"webhook remove": "Remove a webhook endpoint and its signing secret. Nothing more is sent to it.",
	"webhook test": "Send one webhook.test event to an endpoint now and print the HTTP status it answered with, to check\n" +
		"the URL and the signature check on the receiving side.",
}

// applyLong fills in Long from commandLong on c and below; the hidden
// pre-group copies of the SSH verbs look theirs up under "ssh ".
func applyLong(c *cobra.Command, path string) {
	if c.Long == "" {
		if long, ok := commandLong[path]; ok {
			c.Long = long
		} else if long, ok := commandLong["ssh "+path]; ok {
			c.Long = long
		}
	}
	for _, s := range c.Commands() {
		p := s.Name()
		if path != "" {
			p = path + " " + s.Name()
		}
		applyLong(s, p)
	}
}
