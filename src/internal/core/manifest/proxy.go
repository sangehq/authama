package manifest

import (
	"fmt"
	"regexp"
	"strings"
)

// ProxyCloudflared reaches a host through Cloudflare Access: ssh runs
// `cloudflared access ssh --hostname %h`, which opens the tunnel after a
// browser sign-in, so no SSH port has to be open to the internet.
const ProxyCloudflared = "cloudflared"

// ProxyNone renders `ProxyCommand none`: connect straight to the HostName,
// even when a broader block (a Host * of the user's own, say) sets a proxy.
// A WARP or VPN route to a private address uses it.
const ProxyNone = "none"

// ProxyCommandKind runs a command of the user's own. It is held as an argv,
// never as a shell string: the renderer quotes each argument, so nothing in
// one can start a second command (no injection through ';', '$(...)', '|').
const ProxyCommandKind = "command"

// ProxyKinds are the proxies authama can render. raw_options may not set a
// ProxyCommand (an import could carry one in unseen), so every proxy is a
// field authama writes itself, as it does for VM hosts.
var ProxyKinds = []string{ProxyCloudflared, ProxyNone, ProxyCommandKind}

// ProxySpec routes a host's connections through a proxy command.
type ProxySpec struct {
	Kind string `json:"kind"`
	// Command is the argv a "command" proxy runs; empty for every other kind.
	Command []string `json:"command,omitempty"`
}

// proxyToken is a % token ssh expands in a ProxyCommand. An unknown one makes
// ssh refuse to connect ("unknown key %x"), so it is refused here instead.
var proxyToken = regexp.MustCompile(`%.?`)

var proxyTokens = map[string]bool{"%%": true, "%C": true, "%d": true, "%h": true, "%i": true,
	"%j": true, "%k": true, "%L": true, "%l": true, "%n": true, "%p": true, "%r": true, "%u": true}

// CheckProxyCommand validates a custom proxy's argv: something to run, no
// control characters (a newline would end the config line), no leading '-'
// on the program, and only the % tokens ssh knows.
func CheckProxyCommand(argv []string) error {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return fmt.Errorf("a command proxy needs a command to run")
	}
	if strings.HasPrefix(argv[0], "-") {
		return fmt.Errorf("proxy command %q must not start with '-'", argv[0])
	}
	if strings.EqualFold(argv[0], "none") && len(argv) == 1 {
		return fmt.Errorf(`"none" is not a command: use the proxy kind none`)
	}
	for _, a := range argv {
		if err := rejectControl("proxy command", a); err != nil {
			return err
		}
		for _, tok := range proxyToken.FindAllString(a, -1) {
			if !proxyTokens[tok] {
				return fmt.Errorf("proxy command argument %q has %q, which ssh does not expand "+
					"(write %%%% for a literal %%)", a, tok)
			}
		}
	}
	return nil
}

func checkProxySpec(where string, p *ProxySpec) error {
	if p == nil {
		return nil
	}
	known := false
	for _, k := range ProxyKinds {
		known = known || p.Kind == k
	}
	if !known {
		return fmt.Errorf("%s: proxy.kind %q is not supported (supported: %s)", where, p.Kind,
			strings.Join(ProxyKinds, ", "))
	}
	if p.Kind != ProxyCommandKind {
		if len(p.Command) > 0 {
			return fmt.Errorf("%s: proxy.command is only for kind %q", where, ProxyCommandKind)
		}
		return nil
	}
	if err := CheckProxyCommand(p.Command); err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}
	return nil
}

func checkProxy(profile string, h Host) error {
	where := profile + "/" + h.Alias
	if h.Proxy != nil && h.VM != nil {
		return fmt.Errorf("%s: a host is either a VM or behind a proxy, not both: each renders the "+
			"one ProxyCommand ssh uses", where)
	}
	if err := checkProxySpec(where, h.Proxy); err != nil {
		return err
	}
	if h.Proxy != nil && hasRawKey(h.RawOptions, "ProxyJump") {
		return fmt.Errorf("%s: set a proxy or a ProxyJump, not both: ssh uses whichever comes first", where)
	}
	return nil
}

// ShellQuote quotes one ProxyCommand argument for the shell ssh runs it with.
// A plain word, a path or a %-token stays as it is (so the Cloudflare block
// reads `/opt/homebrew/bin/cloudflared access ssh --hostname %h`); anything
// else is single-quoted, with each ' written as '\”.
func ShellQuote(s string) string {
	if s != "" && plainShellWord.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var plainShellWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./~-]+$`)

// ShellJoin renders an argv as one ProxyCommand line. A leading '~' is quoted
// unless it opens a path, where the shell expands it as the user means.
func ShellJoin(argv []string) string {
	out := make([]string, len(argv))
	for i, a := range argv {
		if strings.HasPrefix(a, "~") && !strings.HasPrefix(a, "~/") {
			out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
			continue
		}
		out[i] = ShellQuote(a)
	}
	return strings.Join(out, " ")
}

// SplitCommand splits a command line the way a POSIX shell splits words:
// whitespace separates, '...' is literal, "..." keeps spaces and honours \"
// and \\, and a backslash outside quotes escapes the next character. Shell
// operators are refused rather than run: a proxy is one program and its
// arguments, never a pipeline.
func SplitCommand(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inWord := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, fmt.Errorf("unterminated ' in %q", s)
			}
			cur.WriteString(s[i+1 : i+1+j])
			i += j + 1
			inWord = true
		case c == '"':
			i++
			for ; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
					i++
				}
				if s[i] == '$' || s[i] == '`' {
					return nil, fmt.Errorf("%q: $ and ` expand in the shell; a proxy command cannot use them", s)
				}
				cur.WriteByte(s[i])
			}
			if i >= len(s) {
				return nil, fmt.Errorf(`unterminated " in %q`, s)
			}
			inWord = true
		case c == '\\':
			if i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
				inWord = true
			}
		case strings.IndexByte(";&|<>()$`", c) >= 0:
			return nil, fmt.Errorf("%q: %q is shell syntax; a proxy command is one program and its "+
				"arguments (quote it if it is part of an argument)", s, string(c))
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out, nil
}

// ProxyFromCommand reads a ProxyCommand value back into a ProxySpec: "none",
// the Cloudflare Access command (any path to cloudflared), or a command of
// the user's own.
func ProxyFromCommand(value string) (*ProxySpec, error) {
	v := strings.TrimSpace(value)
	if strings.EqualFold(v, "none") {
		return &ProxySpec{Kind: ProxyNone}, nil
	}
	argv, err := SplitCommand(v)
	if err != nil {
		return nil, err
	}
	if len(argv) > 0 && strings.EqualFold(argv[0], "exec") {
		argv = argv[1:]
	}
	if IsCloudflaredAccess(argv) {
		return &ProxySpec{Kind: ProxyCloudflared}, nil
	}
	if err := CheckProxyCommand(argv); err != nil {
		return nil, err
	}
	return &ProxySpec{Kind: ProxyCommandKind, Command: argv}, nil
}

// IsCloudflaredAccess reports whether argv is `<...>/cloudflared access ssh
// --hostname %h`, the command the cloudflared kind renders.
func IsCloudflaredAccess(argv []string) bool {
	if len(argv) != 5 {
		return false
	}
	bin := argv[0][strings.LastIndexAny(argv[0], `/\`)+1:]
	bin = strings.TrimSuffix(strings.ToLower(bin), ".exe")
	return bin == "cloudflared" && argv[1] == "access" && argv[2] == "ssh" &&
		argv[3] == "--hostname" && argv[4] == "%h"
}

func hasRawKey(o OrderedOptions, key string) bool {
	for _, k := range o.Keys() {
		if strings.EqualFold(k, key) {
			return true
		}
	}
	return false
}
