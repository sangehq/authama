package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
)

// commandExamples are the --help examples for the commands that do not set
// their own, keyed by the path under the root ("ssh host add"). A command's
// own Example wins. TestEveryCommandHasExamplesThatParse keeps every visible
// command covered and every line here a valid invocation.
var commandExamples = map[string][]string{
	"owner status":        {"owner status"},
	"repair":              {"repair", "repair --check   # CI: exit non-zero when anything is wrong", "repair --area ssh --yes"},
	"reset":               {"reset", "reset --confirm reset   # a script"},
	"owner init":          {"owner init", "owner init --yes"},
	"owner verify":        {"owner verify   # exits non-zero when a file changed outside authama"},
	"owner reseal":        {"owner reseal   # after editing tokens.json by hand"},
	"owner forget":        {"owner forget"},
	"owner pin set":       {"owner pin set"},
	"owner pin reset":     {"owner pin reset   # forgot it: proves the OS account instead"},
	"owner pin remove":    {"owner pin remove"},
	"owner fingerprint":   {"owner fingerprint on", "owner fingerprint off"},
	"doctor":              {"doctor", "doctor --fix", "doctor --strict --json   # CI: fail on anything worth fixing"},
	"history":             {"history", "history --since 7d --command \"host add\"", "history --host web1 --json"},
	"history show":        {"history show 224f5bf1   # an OP id from history"},
	"history reset":       {"history reset", "history reset --confirm history   # a script"},
	"settings set":        {"settings set backup.age_recipient age1...   # your age recipient", "settings set web.port 8000"},
	"settings unset":      {"settings unset web.port"},
	"settings backup-key": {"settings backup-key"},
	"init":                {"init", "init --force --backup   # reset manifest, inventory and .env, keeping copies"},
	"migrate":             {"migrate", "migrate --force --yes"},
	"settings path":       {"settings path", "settings path --json"},
	"settings show":       {"settings show", "settings show --effective   # every setting, its value and where it came from"},
	"settings validate":   {"settings validate", "settings validate ./authama.json"},
	"settings init":       {"settings init", "settings init --force"},
	"api token create":    {"api token create ci   # prints the token once", "api token create dashboard --expires 0   # never expires"},
	"api token list":      {"api token list", "api token list --json"},
	"api token revoke":    {"api token revoke ci"},
	"webhook add":         {"webhook add ops https://hooks.example.com/authama   # prints its signing secret once", "webhook add audit https://hooks.example.com/a --event operation.finished"},
	"webhook list":        {"webhook list", "webhook list --json"},
	"webhook remove":      {"webhook remove ops"},
	"webhook enable":      {"webhook enable ops   # after it was turned off"},
	"webhook disable":     {"webhook disable ops"},
	"webhook test":        {"webhook test ops"},
	"webhook deliver":     {"webhook deliver   # send what is due now"},
	"mcp":                 {"mcp   # started by an AI client, which talks to it over stdio"},
	"serve":               {"serve", "serve --read-only --no-open", "serve --port 8765 --token-file ~/.cache/authama-url"},
	"version":             {"version"},
	"ssh audit":           {"ssh audit", "ssh audit --notify"},
	"ssh bundle":          {"ssh bundle", "ssh bundle --recipient age1... --output /Volumes/backup --keep 5"},
	"ssh clean":           {"ssh clean --dry-run", "ssh clean --adopt --yes"},
	"ssh config check":    {"ssh config check   # exit 1 when the files drift from the manifest"},
	"ssh config export":   {"ssh config export --flat --out -", "ssh config export --out ./ssh_config"},
	"ssh config render":   {"ssh config render --dry-run", "ssh config render --yes"},
	"ssh config show":     {"ssh config show", "ssh config show web1   # what ssh -G resolves for one alias"},
	"ssh deploy":          {"ssh deploy work/github", "ssh deploy web1 --yes"},
	"ssh diff":            {"ssh diff"},
	"ssh expiry":          {"ssh expiry"},
	"ssh git status":      {"ssh git status", "ssh git status ~/code/acme ~/code/personal"},
	"ssh git use":         {"ssh git use profile --dry-run", "ssh git use https ~/code/acme/api --yes", "ssh git use ssh --profile work"},
	"ssh host add": {
		"ssh host add work web1 --hostname web1.example.com --user deploy",
		"ssh host add work gh --hostname github.com --user git --provider github --token-env GH_TOKEN_WORK",
	},
	"ssh host delete":     {"ssh host delete work web1", "ssh host delete work web1 --purge --revoke --yes"},
	"ssh host edit":       {"ssh host edit work web1 --port 2222", "ssh host edit work web1 --tag prod --untag staging"},
	"ssh import":          {"ssh import --dry-run", "ssh import ~/old-laptop/.ssh --yes", "ssh import ~/.config --dry-run"},
	"ssh key add":         {"ssh key add work deploy --type ed25519", "ssh key add work deploy --host web1 --passphrase"},
	"ssh key delete":      {"ssh key delete work/deploy", "ssh key delete work/deploy --revoke --purge --yes"},
	"ssh key list":        {"ssh key list", "ssh key list work"},
	"ssh key regen-pub":   {"ssh key regen-pub work/deploy", "ssh key regen-pub --all --dry-run"},
	"ssh key reserve":     {"ssh key reserve docker-tunnel --for devbox --note \"Docker VM, later\"", "ssh key reserve ~/.ssh/old-laptop --dry-run"},
	"ssh key unreserve":   {"ssh key unreserve docker-tunnel"},
	"ssh keygen":          {"ssh keygen work", "ssh keygen web1 --force --yes"},
	"ssh knownhosts init": {"ssh knownhosts init work", "ssh knownhosts init --all --yes"},
	"ssh knownhosts pin":  {"ssh knownhosts pin web1", "ssh knownhosts pin git.example.com --port 2222", "ssh knownhosts pin --all --yes"},
	"ssh list":            {"ssh list", "ssh list --profile work --tag prod", "ssh list --type vcs"},
	"ssh load":            {"ssh load work"},
	"ssh net":             {"ssh net", "ssh net work"},
	"checklist":           {"checklist", "checklist --open", "checklist --json"},
	"checklist show":      {"checklist show work/deploy", "checklist show token:npm/registry.npmjs.org --json"},
	"checklist tick":      {"checklist tick work/deploy copies", "checklist tick token:npm/registry.npmjs.org issuer"},
	"checklist untick":    {"checklist untick work/deploy copies"},
	"checklist revoking":  {"checklist revoking token:npm/registry.npmjs.org", "checklist revoking token:npm/registry.npmjs.org --stop"},
	"ssh notify doctor":   {"ssh notify doctor"},
	"ssh notify install":  {"ssh notify install --yes"},
	"ssh notify ping":     {"ssh notify ping -title \"Deploy finished\" -body \"billing-api is on v1.4.0\" -group deploy"},
	"ssh notify send":     {"ssh notify send -title Backup -body \"42 files\"", "ssh notify send -json < notification.json"},
	"ssh notify test":     {"ssh notify test"},
	"ssh profile add":     {"ssh profile add work", "ssh profile add personal --shared --key-name personal --category home"},
	"ssh profile delete":  {"ssh profile delete old", "ssh profile delete old --purge --revoke --yes"},
	"ssh profile edit":    {"ssh profile edit work --category clients", "ssh profile edit work --key-scope per_service"},
	"ssh providers":       {"ssh providers", "ssh providers --export"},
	"ssh reconcile":       {"ssh reconcile --dry-run", "ssh reconcile --yes"},
	"ssh recover":         {"ssh recover", "ssh recover work/deploy"},
	"ssh restore":         {"ssh restore ~/backup/authama-20261101-094319.age --identity ~/.age/key.txt"},
	"ssh rollback":        {"ssh rollback work/deploy"},
	"ssh rotate": {"ssh rotate work/deploy", "ssh rotate --all --dry-run", "ssh rotate --all --check   # test every key first, change nothing", "ssh rotate --all --no-key-backup --yes   # a clean rotation, no backup bundle", "ssh rotate --all --force --yes   # replace locally what cannot rotate in place",
		"ssh rotate --failed --yes        # run again the keys the last run left", "ssh rotate --category clients --yes"},
	"ssh show":              {"ssh show work", "ssh show web1", "ssh show work/deploy"},
	"ssh snapshots list":    {"ssh snapshots list"},
	"ssh snapshots prune":   {"ssh snapshots prune --keep 5"},
	"ssh snapshots restore": {"ssh snapshots restore   # the newest", "ssh snapshots restore ssh-20261101-094319.tar.gz --yes"},
	"ssh validate":          {"ssh validate", "ssh validate work"},
	"ssh view":              {"ssh view web1"},
	"ssh vm add":            {"ssh vm add lab dev --hypervisor parallels --vm Ubuntu --user me"},
	"ssh vm sync":           {"ssh vm sync --dry-run", "ssh vm sync --yes", "ssh vm sync --install-schedule"},
	"ssh vm adopt-key":      {"ssh vm adopt-key", "ssh vm adopt-key devbox-key devbox devbox --hypervisor parallels --vm Ubuntu --user me --dry-run"},
	"ssh vm docker-context": {"ssh vm docker-context dev"},
	"ssh vm list":           {"ssh vm list"},
}

// applyExamples fills in Example from commandExamples on c and below. path is
// c's path under the root; the hidden pre-group copies of the SSH verbs look
// theirs up under "ssh ", so they show the same lines.
func applyExamples(c *cobra.Command, path string) {
	if c.Example == "" {
		lines, ok := commandExamples[path]
		if !ok {
			lines = commandExamples["ssh "+path]
		}
		if len(lines) > 0 {
			var b strings.Builder
			for i, l := range lines {
				if i > 0 {
					b.WriteString("\n")
				}
				b.WriteString("  " + brand.Command + " " + l)
			}
			c.Example = b.String()
		}
	}
	for _, s := range c.Commands() {
		p := s.Name()
		if path != "" {
			p = path + " " + s.Name()
		}
		applyExamples(s, p)
	}
}
