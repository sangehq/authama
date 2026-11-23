package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/providers"
	"github.com/sangehq/authama/src/internal/core/tokens"
	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/services/configsvc"
	"github.com/sangehq/authama/src/internal/services/editor"
	"github.com/sangehq/authama/src/internal/services/keysvc"
	"github.com/sangehq/authama/src/internal/services/repair"
	"github.com/sangehq/authama/src/internal/services/savedsel"
	"github.com/sangehq/authama/src/internal/services/settings"
	"github.com/sangehq/authama/src/internal/services/tokensvc"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// A change is two requests. POST /changes/preview runs it against a private
// copy of the home and shows exactly what it would write, with a confirm
// button bound to the preview's hash. POST /changes/apply makes it only if a
// fresh preview, taken under the lock, still has that hash. The lock is never
// held while a person reads.
//
// Both take the same form: an action and its fields, every one a name or a
// plain value, never a path. The form is carried from the preview into the
// confirmation unchanged, so what is applied is what was previewed.

// actionFields are the form fields each action reads, in the order the
// confirmation carries them.
var actionFields = map[string][]string{
	"profile-add":      {"name", "key_scope", "key_name", "key_comment", "category", "code"},
	"profile-comment":  {"name", "key_comment"},
	"profile-edit":     {"name", "key_scope", "key_name"},
	"profile-delete":   {"name", "purge"},
	"profile-category": {"name", "category"},
	// host-add with no alias names the host by the naming rules from env and
	// role (`host add <profile> --env --role`).
	"host-add": {"profile", "alias", "env", "role", "hostname", "user", "port", "tags", "key_name", "provider", "token_env",
		"update", "aliases", "proxy", "proxy_command", "proxy_jump", "connect_timeout", "options", "section", "comment"},
	"host-edit": {"profile", "alias", "hostname", "user", "port", "tags", "key_name", "provider", "token_env",
		"aliases", "proxy", "proxy_command", "proxy_jump", "connect_timeout", "options", "section", "comment"},
	// host-variant-set is `host variant set`: a second route to a host.
	"host-variant-set": {"profile", "alias", "name", "hostname", "port", "proxy", "proxy_command", "proxy_jump",
		"connect_timeout", "options", "section", "comment"},
	"host-variant-remove": {"profile", "alias", "name"},
	"host-delete":         {"profile", "alias"},
	"key-delete":          {"profile", "key"},
	"key-reserve":         {"key", "for", "note"},
	"key-unreserve":       {"key"},
	// key-ack acknowledges one key, every key (all) or a category's.
	"key-ack":             {"key", "all", "category"},
	"vm-add":              {"profile", "alias", "hypervisor", "vm", "user", "key_name"},
	"config-render":       {},
	"config-render-force": {},
	"config-adopt":        {},
	"config-keep-edits":   {},
	"reconcile":           {},
	"keygen":              {"target"},
	"key-add":             {"profile", "key", "key_type", "host", "rotate_after_days"},
	"providers-export":    {},
	"snapshots-prune":     {"keep"},
	"doctor-fix":          {},
	"host-apply":          {"profile", "specs", "create", "category", "tags", "reconcile"},
	"clean":               {"adopt"},
	"bundle":              {"keep"},
	"vm-docker-context":   {"alias"},
	"snapshots-restore":   {"id"},
	"notify-test":         {},
	"audit-notify":        {},
	"token-delete":        {"target", "host", "project"},
	// token-apply takes the command line's --config, --project, --source and
	// --target (only_<tool>, one box each).
	"token-apply": {"config", "project", "source", "only_composer", "only_npm", "only_docker", "only_git", "only_netrc", "only_pypi",
		"only_cargo", "only_gem", "only_gradle", "only_maven", "only_yarn", "only_helm", "only_helm-oci", "only_gh"},
	"vm-sync":       {"repin", "no_probe"},
	"vm-adopt-key":  {"key", "profile", "alias", "hypervisor", "vm", "user", "move", "docker_context"},
	"vm-schedule":   {"install"},
	"settings-init": {},
	// settings-reset writes authama.json again at its defaults, keeping the
	// old file beside it.
	"settings-reset": {},
	// settings-set writes one setting into authama.json; backup-key makes an
	// age key for encrypted backups and sets it up.
	"settings-set":   {"key", "value"},
	"settings-unset": {"key"},
	"backup-key":     {},
	// home-init writes the manifest, inventory and .env again with fresh
	// defaults, after a backup; it asks for "init" to be typed.
	"home-init":               {"confirm"},
	"export-selection-save":   {"name", "selection"},
	"export-selection-delete": {"name"},
	"token-set":               {"target", "service", "server", "username", "scope", "docker_mode", "project", "secret_ref"},
	"token-init":              {"t_composer", "t_npm", "t_docker", "t_git", "t_netrc", "t_pypi", "t_cargo", "t_gem", "t_gradle", "t_maven", "t_yarn", "t_helm", "t_helm-oci", "t_gh"},
	"token-import":            {},
	"token-config-edit":       {"body"},
	"setup-import":            {"body", "on_conflict"},
	"transfer-import-full":    {"file_ref", "pass_ref", "on_conflict"},
	// structure-plan carries every move select (one plan line each) and
	// the free-form plan lines; structure-dismiss hides one suggestion.
	"structure-plan":        {"move", "moves"},
	"structure-dismiss":     {"id", "undo"},
	"structure-rule-add":    {"profile", "name", "comment", "fingerprint", "tool", "kind", "auto"},
	"structure-rule-auto":   {"id", "auto"},
	"structure-rule-remove": {"id"},
	"service-add":           {"name", "kind", "binary", "label"},
	"service-remove":        {"name"},
	// git-sign-set is one profile's git identity (clear removes it);
	// git-sign-use points checkouts at a profile; git-sign-apply writes the
	// git files from the manifest as it is.
	"git-sign-set":   {"profile", "email", "user_name", "key", "dirs", "sign", "clear"},
	"git-sign-use":   {"profile", "checkouts"},
	"git-sign-apply": {},
	// git-remote-use is `ssh git use`: checkouts' origin pointed at a mode.
	"git-remote-use":      {"mode", "profile", "checkouts"},
	"key-lifetime":        {"key", "lifetime"},
	"key-regen-pub":       {"key", "profile", "all"},
	"ssh-import":          {"source", "replace"},
	"key-root":            {"root", "confirm"},
	"naming-set":          {"profile", "alias", "envs", "roles", "enforce", "start", "code_fallback"},
	"naming-key-template": {"profile", "key_name"},
	"profile-code":        {"name", "code"},
	"owner-init":          {},
	"owner-pin-set":       {"pin_current_ref", "pin_new_ref", "pin_confirm_ref"},
	"owner-pin-remove":    {"pin_current_ref"},
	"owner-reseal":        {},
	"owner-forget":        {"confirm"},
	"owner-fingerprint":   {"on"},
	// repair takes --area as one box per area; none is every area.
	"repair":        {"area_tool", "area_ssh", "area_tokens"},
	"factory-reset": {"confirm"},
}

// secretFields are the typed secrets each action takes. Each goes into the
// session's stash on preview and is carried as <name>_ref, so no page,
// confirmation or error ever holds it; any other action given one is refused.
var secretFields = map[string][]string{
	"token-set":        {"secret"},
	"owner-pin-set":    {"pin_current", "pin_new", "pin_confirm"},
	"owner-pin-remove": {"pin_current"},
}

// allSecretFields is every field name secretFields lists.
func allSecretFields() map[string]bool {
	out := map[string]bool{}
	for _, fs := range secretFields {
		for _, f := range fs {
			out[f] = true
		}
	}
	return out
}

// nameRe is what a profile, alias or key name may be on the web: the
// characters the CLI's own names use, and nothing that could be a path.
var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type formError struct{ msg string }

func (e formError) Error() string { return e.msg }

func badForm(format string, a ...any) error { return formError{fmt.Sprintf(format, a...)} }

func name(f url.Values, field string) (string, error) {
	v := strings.TrimSpace(f.Get(field))
	if !nameRe.MatchString(v) {
		return "", badForm("%s %q is not a name authama accepts: letters, digits, '.', '_' and '-'", field, v)
	}
	return v, nil
}

// plain is a free-text value: a hostname, a user, a comment template. It may
// not hold a control character, which nothing legitimate in those needs.
func plain(f url.Values, field string) (string, error) {
	v := strings.TrimSpace(f.Get(field))
	if strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", badForm("%s holds a control character", field)
	}
	if len(v) > 512 {
		return "", badForm("%s is too long", field)
	}
	return v, nil
}

// optional is plain, as a pointer that is nil when the field was left empty
// on an edit form: an empty edit field means "leave it", as an unset flag does.
func optional(f url.Values, field string) (*string, error) {
	v, err := plain(f, field)
	if err != nil || v == "" {
		return nil, err
	}
	return &v, nil
}

// lines is a multi-line value - host specs - which may hold line breaks and
// tabs, and no other control character.
func lines(f url.Values, field string) (string, error) {
	v := f.Get(field)
	if strings.ContainsFunc(v, func(r rune) bool { return (r < 0x20 && r != '\n' && r != '\r' && r != '\t') || r == 0x7f }) {
		return "", badForm("%s holds a control character", field)
	}
	if len(v) > 16384 {
		return "", badForm("%s is too long", field)
	}
	return v, nil
}

// checked is a checkbox: "1" when ticked, absent when not.
func checked(f url.Values, field string) bool { return f.Get(field) == "1" }

// optionalName is a name field that may be left empty.
func optionalName(f url.Values, field string) (string, error) {
	if strings.TrimSpace(f.Get(field)) == "" {
		return "", nil
	}
	return name(f, field)
}

// envVarRe is an environment variable name, as --token-env takes.
var envVarRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// hostExtras reads the host form's optional --key-name, --provider and
// --token-env. Each left empty is nil: unset on a new host, unchanged on an
// edit. A provider must be one the catalog or providers.json names.
func hostExtras(f url.Values, h home) (keyName, provider, tokenEnv *string, err error) {
	if v, e := optionalName(f, "key_name"); e != nil {
		return nil, nil, nil, e
	} else if v != "" {
		keyName = &v
	}
	if v := strings.TrimSpace(f.Get("provider")); v != "" {
		if _, ok := providers.AllSpecs(h.p.Providers())[v]; !ok {
			return nil, nil, nil, badForm("provider %q is not one `authama ssh providers` lists", v)
		}
		provider = &v
	}
	if v := strings.TrimSpace(f.Get("token_env")); v != "" {
		if !envVarRe.MatchString(v) {
			return nil, nil, nil, badForm("token env %q is not an environment variable name", v)
		}
		tokenEnv = &v
	}
	return keyName, provider, tokenEnv, nil
}

// hostSpecs parses the host apply box, one spec per line, and holds every
// alias to the same rule as a name and every hostname and user to plain.
func hostSpecs(f url.Values) ([]editor.HostSpec, error) {
	text, err := lines(f, "specs")
	if err != nil {
		return nil, err
	}
	specs, err := editor.ParseHostSpecs(strings.NewReader(text))
	if err != nil {
		return nil, badForm("%s", err.Error())
	}
	if len(specs) == 0 {
		return nil, badForm("no host specs: one per line, `<alias> <host>[:port] <user> [role]`")
	}
	for _, s := range specs {
		if !nameRe.MatchString(s.Alias) {
			return nil, badForm("alias %q is not a name authama accepts: letters, digits, '.', '_' and '-'", s.Alias)
		}
		for _, v := range append([]string{s.Hostname, s.User, s.Role}, s.Tags...) {
			if strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
				return nil, badForm("spec for %s holds a control character", s.Alias)
			}
		}
	}
	return specs, nil
}

func tagsOf(f url.Values) (*[]string, error) {
	if !f.Has("tags") {
		return nil, nil
	}
	v, err := plain(f, "tags")
	if err != nil {
		return nil, err
	}
	tags := []string{}
	for _, t := range strings.Split(v, ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	return &tags, nil
}

func portOf(f url.Values) (*int, error) {
	v := strings.TrimSpace(f.Get("port"))
	if v == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		return nil, badForm("port %q is not 1-65535", v)
	}
	return &n, nil
}

// home is what a change needs to know about the home beyond the form: the
// snapshot restore reads its snapshot from the real home's snapshots.
type home struct {
	p      paths.Paths
	retain int
	// secret resolves a token form's secret reference; nil outside a request.
	secret func(ref string) (string, bool)
}

// stashed is the secret a form's <field>_ref stands for, or "" when the form
// carries none or it has expired.
func (h home) stashed(f url.Values, field string) string {
	ref := f.Get(field + "_ref")
	if ref == "" || h.secret == nil {
		return ""
	}
	v, _ := h.secret(ref)
	return v
}

// buildChange turns a form into the change it asks for.
func buildChange(f url.Values, h home) (change.Change, error) {
	action := f.Get("action")
	if _, ok := actionFields[action]; !ok {
		return change.Change{}, badForm("unknown action %q", action)
	}
	var errs []error
	n := func(field string) string {
		v, err := name(f, field)
		errs = append(errs, err)
		return v
	}
	pl := func(field string) string {
		v, err := plain(f, field)
		errs = append(errs, err)
		return v
	}
	opt := func(field string) *string {
		v, err := optional(f, field)
		errs = append(errs, err)
		return v
	}
	host := func(edit bool) change.HostInput {
		in := change.HostInput{}
		if edit {
			in.Hostname, in.User = opt("hostname"), opt("user")
		} else {
			h, u := pl("hostname"), pl("user")
			if h == "" || u == "" {
				errs = append(errs, badForm("a new host needs a hostname and a user"))
			}
			in.Hostname, in.User = &h, &u
		}
		var err error
		in.Port, err = portOf(f)
		errs = append(errs, err)
		// The edit form shows the host's current tags, so what it posts is the
		// whole new list: an emptied box clears them.
		in.Tags, err = tagsOf(f)
		errs = append(errs, err)
		in.KeyName, in.Provider, in.TokenEnv, err = hostExtras(f, h)
		errs = append(errs, err)
		in.AddAliases, in.RemoveAliases, in.RouteInput, err = hostRoutes(f, h, strings.TrimSpace(f.Get("profile")),
			strings.TrimSpace(f.Get("alias")), edit)
		errs = append(errs, err)
		return in
	}
	var ch change.Change
	switch action {
	case "profile-add":
		scope := pl("key_scope")
		if scope != "" && scope != "per_service" && scope != "shared" {
			errs = append(errs, badForm("key scope must be per_service or shared"))
		}
		keyName := ""
		if strings.TrimSpace(f.Get("key_name")) != "" {
			keyName = n("key_name")
		}
		category, err := optionalName(f, "category")
		errs = append(errs, err)
		code := ""
		if strings.TrimSpace(f.Get("code")) != "" {
			code = n("code")
		}
		ch = change.ProfileAddWith(n("name"), scope, keyName, pl("key_comment"), category, code)
	case "profile-comment":
		ch = change.ProfileKeyComment(n("name"), pl("key_comment"))
	case "profile-edit":
		scope := opt("key_scope")
		if scope != nil && *scope != "per_service" && *scope != "shared" {
			errs = append(errs, badForm("key scope must be per_service or shared"))
		}
		var keyName *string
		if strings.TrimSpace(f.Get("key_name")) != "" {
			v := n("key_name")
			keyName = &v
		}
		ch = change.ProfileEdit(n("name"), scope, keyName)
	case "profile-delete":
		if checked(f, "purge") {
			ch = change.ProfilePurge(n("name"))
		} else {
			ch = change.ProfileDelete(n("name"))
		}
	case "profile-category":
		category, err := optionalName(f, "category")
		errs = append(errs, err)
		ch = change.ProfileCategory(n("name"), category)
	case "host-add":
		in := host(false)
		alias := strings.TrimSpace(f.Get("alias"))
		env, role := strings.TrimSpace(f.Get("env")), strings.TrimSpace(f.Get("role"))
		switch {
		case alias == "" && (env != "" || role != ""):
			if (env != "" && !nameRe.MatchString(env)) || (role != "" && !nameRe.MatchString(role)) {
				errs = append(errs, badForm("env and role are names: letters, digits, '.', '_' and '-'"))
				break
			}
			m, err := manifest.Load(h.p.Manifest())
			if err != nil {
				errs = append(errs, err)
				break
			}
			generated, extra, err := editor.GenerateAlias(m, n("profile"), env, role)
			if err != nil {
				errs = append(errs, badForm("%s", err.Error()))
				break
			}
			alias = generated
			tags := []string{}
			if in.Tags != nil {
				tags = *in.Tags
			}
			for _, t := range extra {
				if !slices.Contains(tags, t) {
					tags = append(tags, t)
				}
			}
			in.Tags = &tags
		case alias != "" && (env != "" || role != ""):
			errs = append(errs, badForm("env and role name the host; leave the alias empty to use them, or add them as tags"))
		default:
			alias = n("alias")
		}
		in.Update = checked(f, "update")
		ch = change.HostAdd(n("profile"), alias, in)
	case "host-edit":
		ch = change.HostEdit(n("profile"), n("alias"), host(true))
	case "host-delete":
		ch = change.HostDelete(n("profile"), n("alias"))
	case "host-variant-set":
		in, err := variantInput(f, h)
		errs = append(errs, err)
		ch = change.HostVariantSet(n("profile"), n("alias"), n("name"), in)
	case "host-variant-remove":
		ch = change.HostVariantRemove(n("profile"), n("alias"), n("name"))
	case "key-delete":
		ch = change.KeyDelete(manifest.KeyRef{Profile: n("profile"), KeyName: n("key")})
	case "vm-add":
		hv := pl("hypervisor")
		if !knownHypervisor(hv) {
			errs = append(errs, badForm("hypervisor must be one of %s", strings.Join(manifest.KnownHypervisors(), ", ")))
		}
		vm, user := pl("vm"), pl("user")
		if vm == "" || user == "" {
			errs = append(errs, badForm("a VM host needs the VM's name and a guest user"))
		}
		keyName := ""
		if strings.TrimSpace(f.Get("key_name")) != "" {
			keyName = n("key_name")
		}
		ch = change.VMAddWith(n("profile"), n("alias"), hv, vm, user, keyName)
	case "config-render":
		ch = change.ConfigRender()
	case "config-render-force":
		ch = change.ConfigRenderWith(true)
	case "config-adopt":
		ch = change.ConfigAdopt()
	case "config-keep-edits":
		ch = change.ConfigKeepEdits()
	case "reconcile":
		ch = change.Reconcile()
	case "keygen":
		ch = change.Keygen(n("target"))
	case "key-add":
		kt := pl("key_type")
		switch kt {
		case "", "ed25519", "rsa", "ecdsa", "ed25519-sk", "ecdsa-sk":
		default:
			errs = append(errs, badForm("key type must be ed25519, rsa, ecdsa, ed25519-sk or ecdsa-sk"))
		}
		host := ""
		if strings.TrimSpace(f.Get("host")) != "" {
			host = n("host")
		}
		var days *int
		if v := strings.TrimSpace(f.Get("rotate_after_days")); v != "" {
			d, err := strconv.Atoi(v)
			if err != nil || d < 0 || d > 3650 {
				errs = append(errs, badForm("rotate after is a number of days from 0 to 3650"))
			}
			days = &d
		}
		ch = change.KeyAddWith(n("profile"), n("key"), kt, host, days)
	case "key-reserve":
		forWhat, note := pl("for"), pl("note")
		if len(forWhat) > 200 || len(note) > 200 {
			errs = append(errs, badForm("for and note are at most 200 characters"))
		}
		ch = change.KeyReserve(n("key"), forWhat, note)
	case "key-unreserve":
		ch = change.KeyUnreserve(n("key"))
	case "key-ack":
		// A category narrows the form's "every key" to its keys.
		switch {
		case strings.TrimSpace(f.Get("category")) != "":
			ch = change.KeyAck(keysvc.Selection{Category: n("category")})
		case checked(f, "all"):
			ch = change.KeyAck(keysvc.Selection{All: true})
		default:
			ref, err := keyRef(f)
			errs = append(errs, err)
			ch = change.KeyAck(keysvc.Selection{Selectors: []string{ref.String()}})
		}
	case "providers-export":
		ch = change.ProvidersExport()
	case "settings-init":
		ch = change.SettingsInit(false)
	case "settings-set":
		key, value := strings.TrimSpace(f.Get("key")), strings.TrimSpace(f.Get("value"))
		if _, ok := settings.Find(key); !ok {
			errs = append(errs, badForm("%s is not a setting", key))
		}
		ch = change.SettingsSet(key, value)
	case "settings-unset":
		key := strings.TrimSpace(f.Get("key"))
		if _, ok := settings.Find(key); !ok {
			errs = append(errs, badForm("%s is not a setting", key))
		}
		ch = change.SettingsUnset(key)
	case "backup-key":
		ch = change.BackupKey()
	case "settings-reset":
		ch = change.SettingsInit(true)
	case "history-reset":
		ch = change.HistoryReset(strings.TrimSpace(f.Get("confirm")) == "history")
	case "home-init":
		ch = change.HomeInit(strings.TrimSpace(f.Get("confirm")) == "init")
	case "export-selection-save":
		name, sel, err := savedSelectionForm(f)
		errs = append(errs, err)
		ch = change.ExportSelectionSave(name, sel)
	case "export-selection-delete":
		name := strings.TrimSpace(f.Get("name"))
		errs = append(errs, savedsel.CheckName(name))
		ch = change.ExportSelectionDelete(name)
	case "snapshots-prune":
		keep, err := strconv.Atoi(strings.TrimSpace(f.Get("keep")))
		if err != nil || keep < 0 || keep > 1000 {
			errs = append(errs, badForm("keep must be a number from 0 to 1000"))
		}
		ch = change.SnapshotsPrune(keep)
	case "doctor-fix":
		ch = change.DoctorFix()
	case "vm-sync":
		ch = change.VMSyncWith(checked(f, "repin"), checked(f, "no_probe"))
	case "vm-adopt-key":
		hv := pl("hypervisor")
		if !knownHypervisor(hv) {
			errs = append(errs, badForm("hypervisor must be one of %s", strings.Join(manifest.KnownHypervisors(), ", ")))
		}
		vm, user := pl("vm"), pl("user")
		if vm == "" || user == "" {
			errs = append(errs, badForm("adopting a key onto a VM needs the VM's name and a guest user"))
		}
		ch = change.VMAdoptKeyWith(n("key"), n("profile"), n("alias"), hv, vm, user, checked(f, "move"), checked(f, "docker_context"))
	case "vm-schedule":
		ch = change.VMSchedule(checked(f, "install"))
	case "host-apply":
		specs, err := hostSpecs(f)
		errs = append(errs, err)
		category, err := optionalName(f, "category")
		errs = append(errs, err)
		var extra []string
		if t, err := tagsOf(f); err != nil {
			errs = append(errs, err)
		} else if t != nil {
			extra = *t
		}
		ch = change.HostApplyWith(n("profile"), specs, checked(f, "create"), category, extra, checked(f, "reconcile"))
	case "clean":
		ch = change.Clean(checked(f, "adopt"))
	case "bundle":
		keep := 0
		if v := strings.TrimSpace(f.Get("keep")); v != "" {
			var err error
			if keep, err = strconv.Atoi(v); err != nil || keep < 0 || keep > 1000 {
				errs = append(errs, badForm("keep must be a number from 0 to 1000"))
			}
		}
		ch = change.Bundle(keep)
	case "vm-docker-context":
		ch = change.VMDockerContext(n("alias"))
	case "snapshots-restore":
		retain := h.retain
		if retain < 1 {
			retain = 10
		}
		ch = change.SnapshotRestore(h.p.SnapshotsDir(), n("id"), retain)
	case "notify-test":
		ch = change.NotifyTest()
	case "repair":
		var areas []repair.Area
		for _, a := range repair.Areas {
			if checked(f, "area_"+string(a)) {
				areas = append(areas, a)
			}
		}
		ch = change.RepairAreas(repairHome, areas)
	case "factory-reset":
		ch = change.FactoryReset(strings.TrimSpace(f.Get("confirm")) == "reset")
	case "owner-init":
		ch = change.OwnerInit()
	case "owner-pin-set":
		ch = change.OwnerPINSet(h.stashed(f, "pin_current"), h.stashed(f, "pin_new"), h.stashed(f, "pin_confirm"))
	case "owner-pin-remove":
		ch = change.OwnerPINRemove(h.stashed(f, "pin_current"))
	case "owner-reseal":
		ch = change.OwnerReseal()
	case "owner-forget":
		ch = change.OwnerForget(strings.TrimSpace(f.Get("confirm")) == "forget")
	case "owner-fingerprint":
		ch = change.OwnerFingerprint(f.Get("on") == "1")
	case "audit-notify":
		ch = change.AuditNotify()
	case "token-delete":
		target := strings.TrimSpace(f.Get("target"))
		if !tokens.IsTarget(target) {
			errs = append(errs, badForm("target must be one of %s", strings.Join(tokens.Targets(), ", ")))
		}
		host := pl("host")
		if host == "" {
			errs = append(errs, badForm("host is required"))
		}
		project, err := scopePath(f, "project")
		errs = append(errs, err)
		ch = change.TokenDeleteIn(target, host, project)
	case "token-apply":
		var sc change.TokenScope
		var err error
		for _, fld := range []struct {
			name string
			dst  *string
		}{{"config", &sc.Config}, {"project", &sc.Project}, {"source", &sc.Source}} {
			*fld.dst, err = scopePath(f, fld.name)
			errs = append(errs, err)
		}
		for _, t := range tokens.Targets() {
			if checked(f, "only_"+t) {
				sc.Targets = append(sc.Targets, t)
			}
		}
		ch = change.TokenApplyIn(sc)
	case "token-set":
		spec := tokensvc.SetSpec{Target: strings.TrimSpace(f.Get("target")), Service: strings.TrimSpace(f.Get("service")),
			Server: pl("server"), Username: pl("username"), Scope: pl("scope")}
		// The form's tool reads "composer" beside a service; any other tool
		// beside one is a contradiction Check refuses, never a silent pick.
		if spec.Service != "" && spec.Target == tokens.Composer {
			spec.Target = ""
		}
		if _, err := spec.Check(); err != nil {
			errs = append(errs, badForm("%s", strings.NewReplacer("--service", "a service", "--target", "a tool",
				"--server", "the server", "--username", "a user name").Replace(err.Error())))
		}
		mode := strings.TrimSpace(f.Get("docker_mode"))
		if !tokensvc.ValidDockerMode(mode) {
			errs = append(errs, badForm("docker mode must be one of %s", strings.Join(tokensvc.DockerModes(), ", ")))
		}
		ref := f.Get("secret_ref")
		if ref == "" {
			errs = append(errs, badForm("the token form sent no secret"))
		}
		project, err := scopePath(f, "project")
		errs = append(errs, err)
		resolve := h.secret
		ch = change.TokenSetIn(spec, mode, project, func() (string, error) {
			if resolve != nil {
				if v, ok := resolve(ref); ok {
					return v, nil
				}
			}
			return "", errSecretGone
		})
	case "token-init":
		var targets []string
		for _, t := range tokens.Targets() {
			if checked(f, "t_"+t) {
				targets = append(targets, t)
			}
		}
		ch = change.TokenInit(targets)
	case "key-lifetime":
		ref, err := keyRef(f)
		errs = append(errs, err)
		days := 0
		if v := strings.TrimSpace(f.Get("lifetime")); v != "default" {
			d, err := editor.ParseLifetime(v)
			errs = append(errs, err)
			days = d
		}
		ch = change.KeyLifetime(ref, days)
	case "key-regen-pub":
		// One key, one profile's keys, or every key: the CLI's selectors, each
		// a name, never a path.
		var sel keysvc.Selection
		key, profile, all := strings.TrimSpace(f.Get("key")), strings.TrimSpace(f.Get("profile")), checked(f, "all")
		switch {
		case all && key == "":
			// The panel's profile choice stays filled in when "every key" is
			// ticked; every key covers it.
			sel.All = true
		case key != "" && profile == "" && !all:
			ref, err := keyRef(f)
			errs = append(errs, err)
			sel.Selectors = []string{ref.String()}
		case profile != "" && key == "" && !all:
			sel.Selectors = []string{n("profile")}
		default:
			errs = append(errs, badForm("choose one key, one profile, or every key"))
		}
		ch = change.KeyRegenPub(h.p, sel)
	case "key-root":
		// One directory name under ~/.ssh, never a path; the manifest's own
		// check refuses a slash, a dot, a hidden name and ssh's files.
		root := strings.TrimSpace(f.Get("root"))
		if root == "" {
			errs = append(errs, badForm("name the directory the keys move to"))
		} else if err := manifest.CheckKeyRoot(root); err != nil {
			errs = append(errs, badForm("%s", err.Error()))
		}
		ch = change.KeyRoot(root, strings.TrimSpace(f.Get("confirm")))
	case "ssh-import":
		// The source is a name for a config this machine has, never a path.
		src, ok := importSource(h.p, f.Get("source"))
		if !ok {
			errs = append(errs, badForm("choose an ssh config this machine has"))
		}
		ch = change.SSHImport(h.p, src.Path, checked(f, "replace"))
	case "naming-set", "naming-key-template", "profile-code":
		var err error
		ch, err = namingChange(action, f, h)
		errs = append(errs, err)
	case "transfer-import-full":
		var err error
		if ch, err = transferImportFullChange(f, h); err != nil {
			errs = append(errs, err)
		}
	case "setup-import":
		ch, errs = setupImportChange(f, errs)
	case "structure-plan":
		ch, errs = structurePlanChange(f, errs)
	case "structure-dismiss":
		ch, errs = structureDismissChange(f, errs)
	case "structure-rule-add":
		ch, errs = structureRuleAddChange(f, errs)
	case "structure-rule-auto":
		ch, errs = structureRuleAutoChange(f, errs)
	case "structure-rule-remove":
		ch, errs = structureRuleRemoveChange(f, errs)
	case "service-add":
		ch, errs = serviceUpsertChange(f, errs, false)
	case "service-remove":
		ch, errs = serviceRemoveChange(f, errs)
	case "git-sign-set":
		ch, errs = gitSignSetChange(f, errs)
	case "git-sign-use":
		ch, errs = gitSignUseChange(f, errs)
	case "git-sign-apply":
		ch = change.GitSignApply()
	case "git-remote-use":
		ch, errs = gitRemoteUseChange(f, errs)
	case "token-config-edit":
		body := f.Get("body")
		switch {
		case !utf8.ValidString(body):
			errs = append(errs, badForm("the token config is not UTF-8 text"))
		case strings.ContainsFunc(body, func(r rune) bool { return (r < 0x20 && r != '\n' && r != '\r' && r != '\t') || r == 0x7f }):
			errs = append(errs, badForm("the token config holds a control character"))
		default:
			if err := tokens.CheckNamesOnly(body); err != nil {
				errs = append(errs, badForm("%s", err.Error()))
			}
		}
		ch = change.TokenConfigEdit(body)
	case "token-import":
		ch = change.TokenImport()
	}
	return ch, errors.Join(errs...)
}

// carried is the form, cut to the fields its action reads, for the
// confirmation to post back unchanged.
func carried(f url.Values) []formField {
	out := []formField{{"action", f.Get("action")}}
	for _, k := range actionFields[f.Get("action")] {
		// Every value: a field a form repeats (the structure page's move
		// selects) reaches the confirmation whole.
		for _, v := range f[k] {
			out = append(out, formField{k, v})
		}
	}
	return append(out, thenFields(f, thenPrefix)...)
}

// thenPrefix marks the fields of a job a change was started from (a fix on
// a job's review): carried through the preview untouched, they let the done
// page offer to review that job again.
const thenPrefix = "then_"

// thenFields is the job a change came from: every field named then_<name>
// whose name is one a job reads, with prefix kept when keep is the prefix,
// or stripped when it is "".
func thenFields(f url.Values, keep string) []formField {
	action := f.Get(thenPrefix + "action")
	allowed, ok := jobFields[action]
	if !ok {
		return nil
	}
	out := []formField{{keep + "action", action}}
	for _, k := range allowed {
		for _, v := range f[thenPrefix+k] {
			out = append(out, formField{keep + k, v})
		}
	}
	return out
}

type formField struct{ Name, Value string }

type changePage struct {
	Plan    change.Plan
	Fields  []formField
	Stale   bool
	Command string
	Destroy bool // the action removes something; the button says so
	Purge   bool // it deletes private keys too, which the page spells out
	// HandEdits is set when a render would discard hand edits: the page
	// offers to keep them or to discard them.
	HandEdits bool
	// OfferUpdate is set when host add names a host the profile has: the
	// page offers the same form as an update.
	OfferUpdate bool
	Update      []formField
}

type donePage struct {
	Command string
	Result  change.Result
	// Then is the job the change was a fix for, to review again.
	Then []formField
}

func commandLine(p change.Plan) string {
	return strings.TrimSpace(brand.Line(p.Command) + " " + strings.Join(p.Args, " "))
}

func (h pages) parseChange(w http.ResponseWriter, r *http.Request, preview bool) (change.Change, bool) {
	if err := r.ParseForm(); err != nil {
		h.s.renderError(w, r, http.StatusBadRequest, "Unreadable form", err.Error())
		return change.Change{}, false
	}
	sess := sessionFrom(r.Context())
	// A typed secret goes into the stash at once and out of the form, so no
	// page, confirmation or error message can carry it.
	allowed := map[string]bool{}
	for _, f := range secretFields[r.PostForm.Get("action")] {
		allowed[f] = true
	}
	for field := range allSecretFields() {
		if !r.PostForm.Has(field) {
			continue
		}
		v := r.PostForm.Get(field)
		r.PostForm.Del(field)
		if !preview || !allowed[field] {
			h.s.renderError(w, r, http.StatusBadRequest, "That change cannot be made", "a secret is taken only by its own form's preview")
			return change.Change{}, false
		}
		if field != "secret" && v == "" {
			continue // an empty PIN field (no current PIN yet) is no secret
		}
		v, err := secretField(v)
		if err != nil {
			h.s.renderError(w, r, http.StatusBadRequest, "That change cannot be made", err.Error())
			return change.Change{}, false
		}
		r.PostForm.Set(field+"_ref", h.s.secrets.put(sess, v, h.s.now()))
	}
	ch, err := buildChange(r.PostForm, home{p: h.s.opts.Engine.Home(), retain: h.s.opts.Retain,
		secret: func(ref string) (string, bool) { return h.s.secrets.get(sess, ref, h.s.now()) }})
	if err != nil {
		h.s.renderError(w, r, http.StatusBadRequest, "That change cannot be made", err.Error())
		return change.Change{}, false
	}
	return ch, true
}

func (h pages) handlePreview(w http.ResponseWriter, r *http.Request) {
	ch, ok := h.parseChange(w, r, true)
	if !ok {
		return
	}
	plan, err := h.s.opts.Engine.Preview(ch)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.renderChange(w, r, http.StatusOK, plan, false)
}

func (h pages) handleApply(w http.ResponseWriter, r *http.Request) {
	ch, ok := h.parseChange(w, r, false)
	if !ok || !ownerGate(h).stepUp(w, r) {
		return
	}
	fresh, res, err := h.s.opts.Engine.Apply(ch, r.PostForm.Get("hash"))
	switch {
	case errors.Is(err, change.ErrStale):
		h.renderChange(w, r, http.StatusConflict, fresh, true)
	case err != nil:
		h.s.renderError(w, r, http.StatusUnprocessableEntity, "The change was not made", err.Error())
	default:
		for field := range allSecretFields() {
			if ref := r.PostForm.Get(field + "_ref"); ref != "" {
				h.s.secrets.drop(ref)
			}
		}
		// An uploaded full export and its passphrase are used once too.
		for _, field := range []string{"file_ref", "pass_ref"} {
			if ref := r.PostForm.Get(field); ref != "" {
				h.s.secrets.drop(ref)
			}
		}
		h.s.render(w, http.StatusOK, "done", h.s.newView(r, "", "Done",
			donePage{Command: strings.TrimSpace(brand.Line(ch.Command) + " " + strings.Join(ch.Args, " ")), Result: res,
				Then: thenFields(r.PostForm, "")}))
	}
}

func (h pages) renderChange(w http.ResponseWriter, r *http.Request, status int, plan change.Plan, stale bool) {
	action := r.PostForm.Get("action")
	h.s.render(w, status, "change", h.s.newView(r, "", "Review the change", changePage{
		Plan: plan, Fields: carried(r.PostForm), Stale: stale, Command: commandLine(plan),
		Destroy:     strings.HasSuffix(action, "-delete") || action == "config-render-force",
		Purge:       action == "profile-delete" && checked(r.PostForm, "purge"),
		HandEdits:   strings.Contains(plan.Err, configsvc.HandEditFix),
		OfferUpdate: action == "host-add" && strings.Contains(plan.Err, "already exists in"),
		Update:      withUpdate(r.PostForm),
	}))
}

func knownHypervisor(hv string) bool {
	for _, h := range manifest.KnownHypervisors() {
		if h == hv {
			return true
		}
	}
	return false
}

// scopePath reads an optional path field of a token change: --project,
// --config or --source. It must be absolute (or ~/...), since the server has
// no current directory of yours, and hold no control character.
func scopePath(f url.Values, field string) (string, error) {
	v := strings.TrimSpace(f.Get(field))
	switch {
	case v == "":
		return "", nil
	case strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return "", badForm("%s holds a control character", field)
	case !strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "~/") && (len(v) < 3 || v[1] != ':'):
		return "", badForm("%s %q: give a full path (or ~/…); the server has no current directory of yours", field, v)
	}
	abs, ok := plainAbs(expandHome(v))
	if !ok {
		return "", badForm("%s %q is not a plain full path", field, v)
	}
	return abs, nil
}

// withUpdate is a host add's fields again, as host add --update.
func withUpdate(f url.Values) []formField {
	if f.Get("action") != "host-add" {
		return nil
	}
	var out []formField
	for _, ff := range carried(f) {
		if ff.Name != "update" {
			out = append(out, ff)
		}
	}
	return append(out, formField{"update", "1"})
}
