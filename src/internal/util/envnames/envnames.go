// Package envnames is the one table of the environment variables authama reads.
//
// Every name is AUTHAMA_*, and every read goes through Get, Source or IsSet
// with one of these constants, so no call site spells a variable itself. The
// SSH_MANAGER_* and SSHMGR_* names releases before 0.1.1 read were removed on
// 2026-11-07, after a deprecation notice; they are not read at all now.
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
	HelloCheck     = "AUTHAMA_HELLO_CHECK"
	VMHypervisors  = "AUTHAMA_VM_HYPERVISORS"
	VMMultiplex    = "AUTHAMA_VM_MULTIPLEX"
	VMPruneGone    = "AUTHAMA_VM_PRUNE_GONE"
	VMRepin        = "AUTHAMA_VM_REPIN"
	WebAddr        = "AUTHAMA_WEB_ADDR"
	WebPort        = "AUTHAMA_WEB_PORT"
	WebBasePath    = "AUTHAMA_WEB_BASE_PATH"
	WebAutoRestart = "AUTHAMA_WEB_AUTO_RESTART"
	WebMenuExport  = "AUTHAMA_WEB_MENU_EXPORT"
	// ExportSaveTo is where a saved export selection goes: user or manifest.
	ExportSaveTo = "AUTHAMA_EXPORT_SAVE_TO"
	// ServeHandoff is set by a restarting `serve` for the one replacing it,
	// never by a user: the token, port and old instance, as JSON.
	ServeHandoff = "AUTHAMA_SERVE_HANDOFF"
	// OwnerStore set to "file" keeps the owner's sealing key in an owner-only
	// file instead of the OS credential store (see services/owner).
	OwnerStore = "AUTHAMA_OWNER_STORE"
)

// Names returns every name authama reads, sorted.
func Names() []string {
	out := []string{
		AgeIdentityFile, AgeRecipient, AskpassMode, AskpassSecret, AutoPin, ConfigDir,
		DevRoot, DockerCredsStore, ExportSaveTo, GitMode, HelloCheck, History, NoDotenv, Notifications,
		OldKeyMaxAgeDays, OwnerStore, ServeHandoff, SnapshotRetain, StateDir, TokensCatalog, TokensConfig,
		VMHypervisors, VMMultiplex, VMPruneGone, VMRepin, WebAddr, WebAutoRestart, WebBasePath, WebMenuExport, WebPort,
	}
	sort.Strings(out)
	return out
}

// Getenv is an environment lookup. nil means os.Getenv.
type Getenv func(string) string

// Get returns the variable's value ("" when it is unset or empty).
func Get(get Getenv, name string) string {
	v, _ := Source(get, name)
	return v
}

// Source is Get that also says which variable supplied the value: name when
// it is set and non-empty, "" otherwise.
func Source(get Getenv, name string) (value, from string) {
	if get == nil {
		get = os.Getenv
	}
	if v := get(name); v != "" {
		return v, name
	}
	return "", ""
}

// IsSet reports whether the variable is present in the environment, even as
// an empty string. The .env and authama.json loaders use it, so a value a user
// exported is never replaced by one from a file.
func IsSet(lookup func(string) (string, bool), name string) bool {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	_, ok := lookup(name)
	return ok
}
