package manifest

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// RiskyOption is a raw SSH option that can redirect or weaken connections:
// legitimate when the owner set it, and exactly what a forged bundle would
// plant (age does not authenticate a bundle's sender, review R-SEC-04). A
// bundle restore refuses a manifest holding one unless told to allow it
// (audit A-SEC-13).
type RiskyOption struct {
	// Where is defaults.global_options or <profile>/<alias>.raw_options.
	Where, Key, Value, Why string
}

func (r RiskyOption) String() string {
	return fmt.Sprintf("%s: %s %s (%s)", r.Where, r.Key, r.Value, r.Why)
}

// RiskyOptionsInJSON lists the risky options in a manifest document, read
// leniently: only the options are looked at, so a document this build would
// refuse for another reason is still scanned. It errors only on JSON that
// does not parse.
func RiskyOptionsInJSON(data []byte) ([]RiskyOption, error) {
	var doc struct {
		Defaults struct {
			GlobalOptions OrderedOptions `json:"global_options"`
		} `json:"defaults"`
		Profiles map[string]struct {
			Hosts []struct {
				Alias      string         `json:"alias"`
				RawOptions OrderedOptions `json:"raw_options"`
			} `json:"hosts"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	out := riskyIn("defaults.global_options", doc.Defaults.GlobalOptions, true)
	names := make([]string, 0, len(doc.Profiles))
	for n := range doc.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		for _, h := range doc.Profiles[n].Hosts {
			out = append(out, riskyIn(n+"/"+h.Alias+".raw_options", h.RawOptions, false)...)
		}
	}
	return out, nil
}

// riskyIn lists the risky options in one block. global is the Host * block,
// where forwarding the agent or dropping host-key checks applies to every
// host; on one host they are an ordinary choice.
func riskyIn(where string, opts OrderedOptions, global bool) []RiskyOption {
	var out []RiskyOption
	for _, k := range opts.Keys() {
		v := opts.Get(k)
		why := ""
		switch lv := strings.ToLower(strings.TrimSpace(v)); strings.ToLower(k) {
		case "controlpath":
			if !underSafeDir(v) {
				why = "a shared-connection socket outside ~/.ssh or $XDG_RUNTIME_DIR, which another local user could host"
			}
		case "identityagent":
			if !safeAgent(v) {
				why = "an agent socket other than your own, which would sign as you with keys it chooses"
			}
		case "forwardagent":
			if global && lv != "no" {
				why = "forwards your agent to every host, so any of them can use your keys while you are connected"
			}
		case "stricthostkeychecking":
			if global && (lv == "no" || lv == "off") {
				why = "accepts any host key for every host, so a man in the middle goes unnoticed"
			}
		}
		if why != "" {
			out = append(out, RiskyOption{Where: where, Key: k, Value: v, Why: why})
		}
	}
	return out
}

// safeDirs are the places a socket path may point without being risky: the
// user's own ~/.ssh, and the per-user runtime directory.
var safeDirs = []string{"~/.ssh/", "%d/.ssh/", "$XDG_RUNTIME_DIR/", "${XDG_RUNTIME_DIR}/"}

func underSafeDir(v string) bool {
	v = strings.TrimSpace(v)
	if strings.EqualFold(v, "none") {
		return true
	}
	for _, d := range safeDirs {
		if rest, ok := strings.CutPrefix(v, d); ok && rest != "" && !hasDotDot(rest) {
			return true
		}
	}
	return false
}

// hasDotDot is deliberately crude: any "..", even inside a name, makes the
// path not plainly under the directory, so it counts as outside.
func hasDotDot(p string) bool { return strings.Contains(p, "..") }

// safeAgent reports an IdentityAgent that is the user's own agent: none,
// $SSH_AUTH_SOCK, the 1Password agent authama renders, or a socket under a
// safe directory.
func safeAgent(v string) bool {
	switch strings.TrimSpace(v) {
	case "SSH_AUTH_SOCK", "$SSH_AUTH_SOCK", "${SSH_AUTH_SOCK}",
		onePasswordAgentFor("darwin"), onePasswordAgentFor("linux"):
		return true
	}
	return underSafeDir(v)
}
