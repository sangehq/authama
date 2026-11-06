// Package envnames is the one table of the environment variables authama reads,
// under their current AUTHAMA_* names and the names earlier releases used.
//
// The tool shipped as ssh-manager / sshmgr before it was authama, and its
// variables were SSH_MANAGER_* and SSHMGR_*. Those are still honored - a shell
// profile or CI job written for an earlier release keeps working - but the
// AUTHAMA_* name wins when both are set, and the CLI prints a one-line
// deprecation notice naming the replacement (see Deprecated).
//
// Every read goes through Get or Lookup with the current name, so no call site
// spells a legacy name and none can forget one.
//
// One family is outside this table on purpose: the auth-token-manager names
// ($AUTH_TOKEN_MANAGER_CONFIG, $COMPOSER_AUTH_CONFIG, $AUTH_TOKEN_MANAGER_CATALOG)
// belong to a different tool whose config authama imports rather than reads in
// place, so tokensvc consults them itself and its notice points at
// `token import` instead of naming an AUTHAMA_* replacement.
package envnames

import (
	"os"
	"sort"
)

// The current names. Each constant is the only spelling the code uses.
const (
	ConfigDir        = "AUTHAMA_CONFIG_DIR"
	StateDir         = "AUTHAMA_STATE_DIR"
	DevRoot          = "AUTHAMA_DEV_ROOT"
	NoDotenv         = "AUTHAMA_NO_DOTENV"
	AgeRecipient     = "AUTHAMA_AGE_RECIPIENT"
	AgeIdentityFile  = "AUTHAMA_AGE_IDENTITY_FILE"
	SnapshotRetain   = "AUTHAMA_SNAPSHOT_RETAIN"
	OldKeyMaxAgeDays = "AUTHAMA_OLD_KEY_MAX_AGE_DAYS"
	AutoPin          = "AUTHAMA_AUTO_PIN"
	History          = "AUTHAMA_HISTORY"
	GitMode          = "AUTHAMA_GIT_MODE"
	TokensConfig     = "AUTHAMA_TOKENS_CONFIG"
	TokensCatalog    = "AUTHAMA_TOKENS_CATALOG"
	DockerCredsStore = "AUTHAMA_DOCKER_CREDS_STORE"
	Notifications    = "AUTHAMA_NOTIFICATIONS"
	AskpassMode      = "AUTHAMA_ASKPASS_MODE"
	AskpassSecret    = "AUTHAMA_ASKPASS_SECRET"
	// HelloCheck asks the Windows Hello script for availability only; set by
	// authama for the powershell.exe it runs, never by a user.
	HelloCheck    = "AUTHAMA_HELLO_CHECK"
	VMHypervisors = "AUTHAMA_VM_HYPERVISORS"
	VMPruneGone   = "AUTHAMA_VM_PRUNE_GONE"
	VMRepin       = "AUTHAMA_VM_REPIN"
	WebAddr       = "AUTHAMA_WEB_ADDR"
	WebPort       = "AUTHAMA_WEB_PORT"
	WebBasePath   = "AUTHAMA_WEB_BASE_PATH"
	// OwnerStore set to "file" keeps the owner's sealing key in an owner-only
	// file instead of the OS credential store (see services/owner).
	OwnerStore = "AUTHAMA_OWNER_STORE"
)

// legacy maps a current name to the names earlier releases read, in the order
// they are consulted.
var legacy = map[string][]string{
	ConfigDir:        {"SSH_MANAGER_HOME", "SSH_MANAGER_CONFIG_DIR"},
	DevRoot:          {"SSHMGR_DEV_ROOT"},
	AgeRecipient:     {"SSH_MANAGER_AGE_RECIPIENT"},
	AgeIdentityFile:  {"SSH_MANAGER_AGE_IDENTITY_FILE"},
	SnapshotRetain:   {"SSH_MANAGER_SNAPSHOT_RETAIN"},
	OldKeyMaxAgeDays: {"SSH_MANAGER_OLD_KEY_MAX_AGE_DAYS"},
	AutoPin:          {"SSH_MANAGER_AUTO_PIN"},
	History:          {"SSH_MANAGER_HISTORY"},
	GitMode:          {"SSHMGR_GIT_MODE"},
	TokensConfig:     {"SSH_MANAGER_TOKENS_CONFIG"},
	TokensCatalog:    {"SSH_MANAGER_TOKENS_CATALOG"},
	AskpassMode:      {"SSHMGR_ASKPASS_MODE"},
	AskpassSecret:    {"SSHMGR_ASKPASS_SECRET"},
}

// internal names are set by authama for its own child processes; a legacy
// spelling of one is never something a user typed, so it earns no notice.
var internal = map[string]bool{AskpassMode: true, AskpassSecret: true, HelloCheck: true}

// Legacy returns the earlier names of a current one (nil when it has none).
func Legacy(name string) []string { return legacy[name] }

// Names returns every current name, sorted.
func Names() []string {
	out := []string{
		AgeIdentityFile, AgeRecipient, AskpassMode, AskpassSecret, AutoPin, ConfigDir,
		DevRoot, DockerCredsStore, GitMode, HelloCheck, History, NoDotenv, Notifications,
		OldKeyMaxAgeDays, OwnerStore, SnapshotRetain, StateDir, TokensCatalog, TokensConfig,
		VMHypervisors, VMPruneGone, VMRepin, WebAddr, WebBasePath, WebPort,
	}
	sort.Strings(out)
	return out
}

// AllNames returns every name the tool reads, current and legacy, sorted.
func AllNames() []string {
	out := Names()
	for _, l := range legacy {
		out = append(out, l...)
	}
	sort.Strings(out)
	return out
}

// Getenv is an environment lookup. nil means os.Getenv.
type Getenv func(string) string

// Get returns the value of a current name: the variable itself when non-empty,
// else the first non-empty legacy spelling, else "".
func Get(get Getenv, name string) string {
	v, _ := Source(get, name)
	return v
}

// Source is Get that also says which variable supplied the value ("" when
// none did).
func Source(get Getenv, name string) (value, from string) {
	if get == nil {
		get = os.Getenv
	}
	if v := get(name); v != "" {
		return v, name
	}
	for _, l := range legacy[name] {
		if v := get(l); v != "" {
			return v, l
		}
	}
	return "", ""
}

// IsSet reports whether the setting is present in the environment under any
// of its spellings, even as an empty string. The .env and authama.json loaders
// use it, so a value a user exported - under either name - is never replaced
// by one from a file.
func IsSet(lookup func(string) (string, bool), name string) bool {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	if _, ok := lookup(name); ok {
		return true
	}
	for _, l := range legacy[name] {
		if _, ok := lookup(l); ok {
			return true
		}
	}
	return false
}

// Deprecated lists, for the legacy spellings present in the environment,
// "OLD -> NEW" pairs, sorted. Internal variables are left out.
func Deprecated(get Getenv) []string {
	if get == nil {
		get = os.Getenv
	}
	var out []string
	for cur, olds := range legacy {
		if internal[cur] {
			continue
		}
		for _, o := range olds {
			if get(o) != "" {
				out = append(out, o+" -> "+cur)
			}
		}
	}
	sort.Strings(out)
	return out
}
