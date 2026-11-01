package tokensvc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/simtabi/authama/src/v3/internal/core/tokens"
	"github.com/simtabi/authama/src/v3/internal/util/log"
)

// Docker reads a registry's credentials from a credential store (credsStore,
// e.g. "desktop" or "osxkeychain") or a per-registry helper (credHelpers)
// before it looks at inline "auths". Writing an inline auth for such a host
// does nothing and looks like it worked. The Python tool did exactly that;
// here the user picks what happens instead.

// The docker credential-store modes.
const (
	// DockerLogin runs `docker login --password-stdin`, which stores the
	// credential where docker will actually read it.
	DockerLogin = "login"
	// DockerRefuse skips the host with a warning.
	DockerRefuse = "refuse"
)

// DockerModes lists the valid modes.
func DockerModes() []string { return []string{DockerLogin, DockerRefuse} }

// ValidDockerMode reports whether m is "", login or refuse.
func ValidDockerMode(m string) bool { return m == "" || m == DockerLogin || m == DockerRefuse }

// Delegated reports which of hosts docker reads from a credential store or
// helper given the config text, and the helper that holds each.
func Delegated(configText string, hosts []string) map[string]string {
	out := map[string]string{}
	store, helpers := tokens.DockerDelegation(configText)
	for _, h := range hosts {
		switch {
		case helpers[h] != "":
			out[h] = "credHelpers:" + helpers[h]
		case store != "":
			out[h] = "credsStore:" + store
		}
	}
	return out
}

// DockerLoginRunner runs docker login; tests replace it.
var DockerLoginRunner = func(configDir, host, user, pass string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "--config", configDir, "login", host, "--username", user, "--password-stdin")
	cmd.Stdin = strings.NewReader(pass + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker login %s: %v: %s", host, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// DockerResult says what happened to each docker host.
type DockerResult struct {
	Written  []string // inline auths merged into config.json
	LoggedIn []string // stored through docker login
	Skipped  []string // delegated and refused
	Warnings []string
}

// SyncDocker writes the docker entries. Hosts docker reads inline are merged
// into config.json; hosts delegated to a credential store follow mode, which
// must already be resolved to login or refuse.
func (e Env) SyncDocker(set *tokens.Set, path, mode string) (DockerResult, error) {
	var res DockerResult
	if err := e.Guard(path); err != nil {
		return res, err
	}
	existing, _, err := readExisting(path)
	if err != nil {
		return res, err
	}
	var hosts []string
	for _, c := range set.Entries(tokens.Docker) {
		hosts = append(hosts, c.Host)
	}
	delegated := Delegated(existing, hosts)
	inline := tokens.NewSet()
	for _, c := range set.Entries(tokens.Docker) {
		via, ok := delegated[c.Host]
		if !ok {
			inline.Add(c)
			continue
		}
		if mode != DockerLogin {
			res.Skipped = append(res.Skipped, c.Host)
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"skipped docker:%s: docker reads it from %s, so an inline auth would be ignored "+
					"(use --creds-store-mode login to store it with docker login)", c.Host, via))
			continue
		}
		if err := e.Paths.RefuseOutsideSandbox("docker login"); err != nil {
			return res, err
		}
		if err := DockerLoginRunner(filepath.Dir(path), c.Host, c.Username, c.Password); err != nil {
			return res, err
		}
		res.LoggedIn = append(res.LoggedIn, c.Host)
		e.audit("token.docker-login", log.Field{Key: "host", Value: c.Host}, log.Field{Key: "via", Value: via})
	}
	if inline.Len("") > 0 {
		warns, err := e.WriteTarget(inline, tokens.Docker, path)
		if err != nil {
			return res, err
		}
		res.Warnings = append(res.Warnings, warns...)
		for _, c := range inline.Entries("") {
			res.Written = append(res.Written, c.Host)
		}
	}
	return res, nil
}

// HasDockerDelegation reports whether the docker config at path delegates
// any of set's docker hosts, so a caller can ask for a mode before writing.
func HasDockerDelegation(set *tokens.Set, path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var hosts []string
	for _, c := range set.Entries(tokens.Docker) {
		hosts = append(hosts, c.Host)
	}
	return len(Delegated(string(b), hosts)) > 0
}
