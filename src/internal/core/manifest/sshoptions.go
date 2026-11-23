package manifest

import (
	"fmt"
	"sort"
	"strings"
)

// sshClientOptions are the ssh_config keywords OpenSSH's client reads (9.x and
// 10.x), plus Apple's UseKeychain. A keyword ssh does not know makes it refuse
// the whole file ("Bad configuration option"), taking every host offline, so
// an option typed into authama is checked against this list before it is
// written. Lower-case: ssh reads keywords case-insensitively.
var sshClientOptions = setOf(
	"addkeystoagent", "addressfamily", "batchmode", "bindaddress", "bindinterface",
	"canonicaldomains", "canonicalizefallbacklocal", "canonicalizehostname", "canonicalizemaxdots",
	"canonicalizepermittedcnames", "casignaturealgorithms", "certificatefile", "challengeresponseauthentication",
	"channeltimeout", "checkhostip", "ciphers", "clearallforwardings", "compression", "connectionattempts",
	"connecttimeout", "controlmaster", "controlpath", "controlpersist", "dynamicforward",
	"enableescapecommandline", "enablesshkeysign", "escapechar", "exitonforwardfailure", "fingerprinthash",
	"forkafterauthentication", "forwardagent", "forwardx11", "forwardx11timeout", "forwardx11trusted",
	"gatewayports", "globalknownhostsfile", "gssapiauthentication", "gssapidelegatecredentials",
	"hashknownhosts", "host", "hostbasedacceptedalgorithms", "hostbasedauthentication",
	"hostbasedkeytypes", "hostkeyalgorithms", "hostkeyalias", "hostname", "identitiesonly",
	"identityagent", "identityfile", "ignoreunknown", "include", "ipqos", "kbdinteractiveauthentication",
	"kbdinteractivedevices", "kexalgorithms", "knownhostscommand", "localcommand", "localforward",
	"loglevel", "logverbose", "macs", "match", "nohostauthenticationforlocalhost",
	"numberofpasswordprompts", "obscurekeystroketiming", "passwordauthentication", "permitlocalcommand",
	"permitremoteopen", "pkcs11provider", "port", "preferredauthentications", "proxycommand", "proxyjump",
	"proxyusefdpass", "pubkeyacceptedalgorithms", "pubkeyacceptedkeytypes", "pubkeyauthentication",
	"refuseconnection", "rekeylimit", "remotecommand", "remoteforward", "requesttty", "requiredrsasize",
	"revokedhostkeys", "securitykeyprovider", "sendenv", "serveralivecountmax", "serveraliveinterval",
	"sessiontype", "setenv", "stdinnull", "streamlocalbindmask", "streamlocalbindunlink",
	"stricthostkeychecking", "syslogfacility", "tag", "tcpkeepalive", "tunnel", "tunneldevice",
	"updatehostkeys", "usekeychain", "user", "userknownhostsfile", "verifyhostkeydns", "visualhostkey",
	"warnweakcrypto", "xauthlocation",
)

func setOf(ks ...string) map[string]bool {
	m := make(map[string]bool, len(ks))
	for _, k := range ks {
		m[k] = true
	}
	return m
}

// IsSSHOption reports whether ssh's client knows the keyword.
func IsSSHOption(key string) bool { return sshClientOptions[strings.ToLower(key)] }

// optionFields are the keywords a host sets through a field of its own, each
// with the flag that sets it.
var optionFields = map[string]string{
	"hostname": "--hostname", "user": "--user", "port": "--port", "proxycommand": "--proxy or --proxy-command",
	"identityfile": "--key-name", "userknownhostsfile": "authama ssh knownhosts pin",
}

// CheckHostOption validates one option a person sets on a host (the CLI's
// --option, the web form's extra options): a keyword ssh knows, not one that
// runs a command or opens a block, not one a field of the host already sets,
// and a value ssh will parse - one line, balanced quotes.
func CheckHostOption(key, value string) error {
	if !optionKeyword.MatchString(key) {
		return fmt.Errorf("option %q is not an ssh keyword (letters and digits only)", key)
	}
	if !IsSSHOption(key) {
		return fmt.Errorf("option %q is not an ssh_config keyword ssh knows (see man ssh_config)%s", key,
			suggestOption(key))
	}
	if use, ok := optionFields[strings.ToLower(key)]; ok {
		return fmt.Errorf("option %s is set by the host itself - use %s", key, use)
	}
	if IsDangerousOption(key) {
		return fmt.Errorf("option %s is not allowed (it can run a command, load config or start a new block)", key)
	}
	return CheckOptionValue(key, value)
}

// CheckOptionValue refuses a value ssh would misread: empty, a control
// character (a newline would start a second line), an unbalanced quote (ssh
// refuses the whole file), or a leading '#' or '=' (a comment, a separator).
func CheckOptionValue(key, value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return fmt.Errorf("option %s needs a value", key)
	}
	if err := rejectControl("option "+key, value); err != nil {
		return err
	}
	if strings.HasPrefix(v, "#") || strings.HasPrefix(v, "=") {
		return fmt.Errorf("option %s value %q must not start with %q", key, value, v[:1])
	}
	for _, q := range []string{`"`, `'`} {
		if strings.Count(v, q)%2 != 0 {
			return fmt.Errorf("option %s value %q has an unbalanced %s quote (ssh would refuse the file)", key, value, q)
		}
	}
	return nil
}

// CanonicalOption is the keyword in OpenSSH's own spelling when the input
// differs only in case (connecttimeout -> ConnectTimeout), else unchanged.
func CanonicalOption(key string) string {
	for _, k := range canonicalSpellings {
		if strings.EqualFold(k, key) {
			return k
		}
	}
	return key
}

var canonicalSpellings = []string{
	"AddKeysToAgent", "AddressFamily", "BatchMode", "BindAddress", "BindInterface", "CanonicalDomains",
	"CanonicalizeFallbackLocal", "CanonicalizeHostname", "CanonicalizeMaxDots", "CanonicalizePermittedCNAMEs",
	"CASignatureAlgorithms", "CertificateFile", "ChannelTimeout", "CheckHostIP", "Ciphers", "ClearAllForwardings",
	"Compression", "ConnectionAttempts", "ConnectTimeout", "ControlMaster", "ControlPath", "ControlPersist",
	"DynamicForward", "EnableEscapeCommandline", "EscapeChar", "ExitOnForwardFailure", "FingerprintHash",
	"ForkAfterAuthentication", "ForwardAgent", "ForwardX11", "ForwardX11Timeout", "ForwardX11Trusted",
	"GatewayPorts", "GlobalKnownHostsFile", "GSSAPIAuthentication", "GSSAPIDelegateCredentials", "HashKnownHosts",
	"HostbasedAcceptedAlgorithms", "HostbasedAuthentication", "HostKeyAlgorithms", "HostKeyAlias",
	"IdentitiesOnly", "IdentityAgent", "IgnoreUnknown", "IPQoS", "KbdInteractiveAuthentication", "KexAlgorithms",
	"LocalForward", "LogLevel", "LogVerbose", "MACs", "NoHostAuthenticationForLocalhost", "NumberOfPasswordPrompts",
	"ObscureKeystrokeTiming", "PasswordAuthentication", "PermitRemoteOpen", "PreferredAuthentications",
	"ProxyJump", "ProxyUseFdpass", "PubkeyAcceptedAlgorithms", "PubkeyAuthentication", "RekeyLimit",
	"RemoteForward", "RequestTTY", "RequiredRSASize", "RevokedHostKeys", "SendEnv", "ServerAliveCountMax",
	"ServerAliveInterval", "SessionType", "SetEnv", "StdinNull", "StreamLocalBindMask", "StreamLocalBindUnlink",
	"StrictHostKeyChecking", "SyslogFacility", "Tag", "TCPKeepAlive", "Tunnel", "TunnelDevice", "UpdateHostKeys",
	"UseKeychain", "VerifyHostKeyDNS", "VisualHostKey",
}

// suggestOption names the closest known keyword for a typo, as ", did you
// mean X?", or "".
func suggestOption(key string) string {
	k := strings.ToLower(key)
	best, bestD := "", 3
	names := make([]string, 0, len(canonicalSpellings))
	names = append(names, canonicalSpellings...)
	sort.Strings(names)
	for _, n := range names {
		if d := editDistance(k, strings.ToLower(n)); d < bestD {
			best, bestD = n, d
		}
	}
	if best == "" {
		return ""
	}
	return ", did you mean " + best + "?"
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
