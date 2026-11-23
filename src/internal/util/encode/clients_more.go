package encode

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// More SSH and file-transfer clients' import formats. Like the others, they
// carry a host, port, user and the key's path on this machine, never a
// password or a private key, and say in their notes what the client cannot
// take from the file (a ProxyJump, a key it keeps in its own store).

// moreClients join Clients when the package's variables are set, before any
// init function reads them.
var moreClients = map[string]Client{
	"royalts": {Name: "royalts", Client: "Royal TS and Royal TSX (Royal JSON for a dynamic folder)", Render: renderRoyalTS},
	"rdm":     {Name: "rdm", Client: "Devolutions Remote Desktop Manager (a CSV for its CSV import wizard)", Render: renderRDM},
	"tabby":   {Name: "tabby", Client: "Tabby (an ssh-profiles.yaml beside its config.yaml)", Render: renderTabby},
	"mremoteng": {Name: "mremoteng",
		Client: "mRemoteNG (a confCons.xml for File > Import)", Render: renderMRemoteNG},
	"xshell": {Name: "xshell", Client: "Xshell (one .xsh session file per host)", Render: renderXshell},
	"blink": {Name: "blink",
		Client: "Blink Shell (an ssh config of the hosts, for its config editor)", Render: snippetRenderer("Blink Shell", "authama-blink.ssh_config", blinkNotes)},
	"prompt3": {Name: "prompt3",
		Client: "Prompt 3 (an ssh config of the hosts, to enter or paste in Prompt)", Render: snippetRenderer("Prompt 3", "authama-prompt3.ssh_config", promptNotes)},
	"winscp":    {Name: "winscp", Client: "WinSCP (WinSCP.ini sessions)", Render: renderWinSCP},
	"filezilla": {Name: "filezilla", Client: "FileZilla (a Site Manager XML for File > Import)", Render: renderFileZilla},
	"cyberduck": {Name: "cyberduck", Client: "Cyberduck and Mountain Duck (one .duck bookmark per host)", Render: renderCyberduck},
	"transmit": {Name: "transmit",
		Client: "Transmit 5 (a FileZilla Site Manager XML, the import Transmit reads)", Render: renderTransmit},
	"ansible": {Name: "ansible", Client: "Ansible (a YAML inventory)", Render: renderAnsible},
}

// jumpNotes says, for each host with a ProxyJump, what the client needs done
// by hand: the file connects straight to the host otherwise.
func jumpNotes(hosts []Host, how string) []string {
	var out []string
	for _, h := range hosts {
		if h.ProxyJump != "" {
			out = append(out, "  "+h.Alias+" reaches its host through ProxyJump "+h.ProxyJump+", which this file cannot carry: "+how)
		}
	}
	if len(out) > 0 {
		out = append([]string{"Jump hosts:"}, out...)
	}
	return out
}

// agentNotes names the hosts whose key is in 1Password: the file names no
// key for them, so the client must use the agent.
func agentNotes(hosts []Host) []string {
	var out []string
	for _, h := range hosts {
		if h.KeyFile == "" {
			out = append(out, "  "+h.Alias+" keeps its key in 1Password: point the client at the 1Password SSH agent")
		}
	}
	return out
}

func jsonFile(name string, v any) (ClientFile, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ClientFile{}, err
	}
	return ClientFile{Name: name, Data: append(b, '\n')}, nil
}

// renderRoyalTS writes a Royal JSON document: a folder per group and an SSH
// terminal connection per host, the key named by KeyFilePath. Royal JSON is
// what a dynamic folder's script returns, and its JSON script type takes it
// pasted as is.
func renderRoyalTS(hosts []Host) (*ClientExport, error) {
	type conn struct {
		Type                   string `json:"Type"`
		TerminalConnectionType string `json:"TerminalConnectionType"`
		Name                   string `json:"Name"`
		ComputerName           string `json:"ComputerName"`
		Port                   int    `json:"Port"`
		Username               string `json:"Username"`
		KeyFilePath            string `json:"KeyFilePath,omitempty"`
		Description            string `json:"Description,omitempty"`
	}
	type folder struct {
		Type    string `json:"Type"`
		Name    string `json:"Name"`
		Objects []conn `json:"Objects"`
	}
	doc := struct {
		Objects []folder `json:"Objects"`
	}{Objects: []folder{}}
	names, by := groups(hosts)
	for _, g := range names {
		f := folder{Type: "Folder", Name: g, Objects: []conn{}}
		for _, h := range by[g] {
			f.Objects = append(f.Objects, conn{Type: "TerminalConnection", TerminalConnectionType: "SSH", Name: h.Alias,
				ComputerName: h.Hostname, Port: h.Port, Username: h.User, KeyFilePath: h.KeyFile, Description: strings.Join(h.Tags, ", ")})
		}
		doc.Objects = append(doc.Objects, f)
	}
	file, err := jsonFile("authama-royalts.json", doc)
	if err != nil {
		return nil, err
	}
	notes := []string{"In Royal TS or Royal TSX, add a Dynamic Folder, set its script type to JSON, paste authama-royalts.json as the script and Reload it. " +
		"Key paths are this machine's; edit them if Royal TS runs elsewhere."}
	notes = append(notes, jumpNotes(hosts, "set a Secure Gateway for the connection in Royal TS")...)
	return &ClientExport{Files: []ClientFile{file}, Notes: append(notes, agentNotes(hosts)...)}, nil
}

// RDMColumns are the Remote Desktop Manager CSV's columns, named as its CSV
// import wizard's fields so each maps by name.
var RDMColumns = []string{"Name", "Group", "ConnectionType", "Host", "Port", "Username", "PrivateKeyPath", "Description"}

// renderRDM writes a CSV for Remote Desktop Manager's CSV import: one SSH
// Shell entry per host, its group as the folder.
func renderRDM(hosts []Host) (*ClientExport, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(RDMColumns); err != nil {
		return nil, err
	}
	for _, h := range hosts {
		if err := w.Write([]string{cell(h.Alias), cell(h.Group), "SSHShell", cell(h.Hostname), strconv.Itoa(h.Port),
			cell(h.User), cell(h.KeyFile), cell(strings.Join(h.Tags, ", "))}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	notes := []string{"In Remote Desktop Manager, File > Import > Import from CSV (the CSV wizard), choose authama-rdm.csv and map each column to the field of the same name. " +
		"PrivateKeyPath is this machine's; set the entry's private key source to a file."}
	notes = append(notes, jumpNotes(hosts, "set the entry's VPN/SSH Gateway in Remote Desktop Manager")...)
	return &ClientExport{Files: []ClientFile{{Name: "authama-rdm.csv", Data: buf.Bytes()}}, Notes: append(notes, agentNotes(hosts)...)}, nil
}

// renderTabby writes ssh-profiles.yaml, the profile list Tabby reads beside
// its config.yaml: an SSH profile per host, grouped, with its key file and,
// for a ProxyJump that names another exported host, Tabby's jumpHost.
func renderTabby(hosts []Host) (*ClientExport, error) {
	// Tabby gives each profile of ssh-profiles.yaml the id file-config:
	// and the SHA-256 of its name, whatever the file says, so a jump host is
	// named that way.
	ids := map[string]string{}
	for _, h := range hosts {
		sum := sha256.Sum256([]byte(h.Alias))
		ids[h.Alias] = "file-config:" + hex.EncodeToString(sum[:])
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode}
	var unmapped []Host
	for _, h := range hosts {
		opts := mapping("host", h.Hostname, "port", h.Port, "user", h.User)
		if h.KeyFile != "" {
			opts.Content = append(opts.Content, scalar("auth"), scalar("publicKey"), scalar("privateKeys"),
				&yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{scalar(h.KeyFile)}})
		} else {
			opts.Content = append(opts.Content, scalar("auth"), scalar("agent"))
		}
		if h.ProxyJump != "" {
			if id, ok := ids[h.ProxyJump]; ok {
				opts.Content = append(opts.Content, scalar("jumpHost"), scalar(id))
			} else {
				unmapped = append(unmapped, h)
			}
		}
		p := mapping("name", h.Alias, "type", "ssh", "group", h.Group)
		p.Content = append(p.Content, scalar("options"), opts)
		seq.Content = append(seq.Content, p)
	}
	data, err := yamlBytes(seq)
	if err != nil {
		return nil, err
	}
	notes := []string{"Copy ssh-profiles.yaml next to Tabby's config.yaml (Settings > Config file > Show config file) and restart Tabby; the profiles appear in its profile list. Each names its authama group in group, which Tabby shows as a group only when one with that id exists in Settings > Profiles; otherwise it lists them as ungrouped. " +
		"Key paths are this machine's."}
	notes = append(notes, jumpNotes(unmapped, "its jump host is not among the exported hosts; set Jump host on the profile in Tabby")...)
	return &ClientExport{Files: []ClientFile{{Name: "ssh-profiles.yaml", Data: data}}, Notes: append(notes, agentNotes(hosts)...)}, nil
}

// renderMRemoteNG writes a confCons.xml: a container per group and an SSH2
// connection per host. mRemoteNG takes keys from a PuTTY saved session, so
// each connection names the PuTTY session the putty export makes for it.
func renderMRemoteNG(hosts []Host) (*ClientExport, error) {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n")
	b.WriteString("<mrng:Connections xmlns:mrng=\"http://mremoteng.org\" Name=\"Connections\" Export=\"false\" EncryptionEngine=\"AES\" BlockCipherMode=\"GCM\" KdfIterations=\"1000\" FullFileEncryption=\"false\" ConfVersion=\"2.6\">\n")
	names, by := groups(hosts)
	for _, g := range names {
		fmt.Fprintf(&b, "  <Node Name=\"%s\" Type=\"Container\" Expanded=\"true\">\n", xmlText(g))
		for _, h := range by[g] {
			fmt.Fprintf(&b, "    <Node Name=\"%s\" Type=\"Connection\" Descr=\"%s\" Hostname=\"%s\" Port=\"%d\" Username=\"%s\" Protocol=\"SSH2\" PuttySession=\"%s\" />\n",
				xmlText(h.Alias), xmlText(strings.Join(h.Tags, ", ")), xmlText(h.Hostname), h.Port, xmlText(h.User), xmlText(h.Alias))
		}
		b.WriteString("  </Node>\n")
	}
	b.WriteString("</mrng:Connections>\n")
	notes := []string{"In mRemoteNG, File > Import > Import from File and choose confCons.xml. mRemoteNG takes a key from a PuTTY saved session: " +
		"each connection names the session `export hosts --format putty` makes for its alias, so import that .reg too (or set each connection's PuTTY session by hand)."}
	notes = append(notes, jumpNotes(hosts, "set a proxy on its PuTTY session")...)
	return &ClientExport{Files: []ClientFile{{Name: "confCons.xml", Data: []byte(b.String())}}, Notes: notes}, nil
}

// iniValue keeps a value on its line in an INI file.
func iniValue(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

// keyName is a key's file name, the name a client that keeps its own key
// store files it under.
func keyName(h Host) string {
	if h.KeyFile == "" {
		return ""
	}
	return filepath.Base(h.KeyFile)
}

// renderXshell writes one .xsh session per host, named after it. Xshell
// keeps keys in its own User Key Manager and a session names one there, so
// each session names its key by file name and the notes list what to import.
func renderXshell(hosts []Host) (*ClientExport, error) {
	exp := &ClientExport{}
	var keys []string
	seen := map[string]bool{}
	for _, h := range hosts {
		var b strings.Builder
		b.WriteString("[SessionInfo]\r\nVersion=7.0\r\nDescription=" + iniValue(strings.Join(append([]string{h.Group}, h.Tags...), ", ")) + "\r\n")
		b.WriteString("[CONNECTION]\r\nProtocol=SSH\r\n")
		fmt.Fprintf(&b, "Host=%s\r\nPort=%d\r\n", iniValue(h.Hostname), h.Port)
		b.WriteString("[CONNECTION:AUTHENTICATION]\r\n")
		fmt.Fprintf(&b, "UserName=%s\r\n", iniValue(h.User))
		if k := keyName(h); k != "" {
			// Method 1 is Public Key.
			fmt.Fprintf(&b, "Method=1\r\nUserKey=%s\r\n", iniValue(k))
			if !seen[h.KeyFile] {
				seen[h.KeyFile] = true
				keys = append(keys, "  "+h.KeyFile+"  as "+k)
			}
		}
		exp.Files = append(exp.Files, ClientFile{Name: fileSafe(h.Alias) + ".xsh", Data: []byte(b.String())})
	}
	exp.Notes = []string{"Copy the .xsh files into a folder under Xshell's Sessions (Documents\\NetSarang Computer\\<version>\\Xshell\\Sessions) " +
		"or open the zip's folder from Xshell's Sessions dialog. Each session names its key by file name; import each into Tools > User Key Manager under that name:"}
	exp.Notes = append(exp.Notes, keys...)
	exp.Notes = append(exp.Notes, jumpNotes(hosts, "set the session's Proxy, or a tunnelling session, in Xshell")...)
	exp.Notes = append(exp.Notes, agentNotes(hosts)...)
	return exp, nil
}

// snippet is an ssh config of the hosts for a client that keeps its own key
// store: IdentityFile names each key by file name, the name the key is
// imported under, and ProxyJump stays, since both clients read it.
func snippet(hosts []Host, client string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Hosts for %s, written by authama. Each IdentityFile is the name to import that key under.\n", client)
	for _, h := range hosts {
		fmt.Fprintf(&b, "\n# %s\nHost %s\n  HostName %s\n  Port %d\n  User %s\n", h.Group, h.Alias, h.Hostname, h.Port, h.User)
		if k := keyName(h); k != "" {
			fmt.Fprintf(&b, "  IdentityFile %s\n", k)
		}
		if h.ProxyJump != "" {
			fmt.Fprintf(&b, "  ProxyJump %s\n", h.ProxyJump)
		}
	}
	return b.String()
}

func keyList(hosts []Host) []string {
	var out []string
	seen := map[string]bool{}
	for _, h := range hosts {
		if h.KeyFile != "" && !seen[h.KeyFile] {
			seen[h.KeyFile] = true
			out = append(out, "  "+h.KeyFile+"  as "+keyName(h))
		}
	}
	return out
}

func snippetRenderer(client, file string, notes func() []string) func([]Host) (*ClientExport, error) {
	return func(hosts []Host) (*ClientExport, error) {
		n := append(notes(), keyList(hosts)...)
		return &ClientExport{Files: []ClientFile{{Name: file, Data: []byte(snippet(hosts, client))}}, Notes: append(n, agentNotes(hosts)...)}, nil
	}
}

func blinkNotes() []string {
	return []string{"Blink Shell keeps its own keys and hosts. Import each key under Settings > Keys with the name below, then " +
		"paste authama-blink.ssh_config into Settings > Hosts > SSH Config (or `config` in a Blink session); Blink reads Host, HostName, Port, User, IdentityFile and ProxyJump from it. Keys to import:"}
}

func promptNotes() []string {
	return []string{"Prompt 3 has no file import: add each server under Servers > + with the values from authama-prompt3.ssh_config " +
		"(the jump host goes in the server's Jump Server field), and import each key under Settings > Keys with the name below. " +
		"Panic Sync carries them to Prompt on other devices. Keys to import:"}
}

// winscpName escapes a session name as WinSCP stores it: anything but a
// letter, digit or one of -_.~ is %XX; '/' stays, since it nests folders.
func winscpName(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("-_.~/", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// renderWinSCP writes the [Sessions\...] sections of a WinSCP.ini: an SFTP
// site per host in a folder per group. WinSCP reads PuTTY .ppk keys, so each
// names <key>.ppk, as the putty export does.
func renderWinSCP(hosts []Host) (*ClientExport, error) {
	var b strings.Builder
	var convert []string
	for _, h := range hosts {
		name := strings.ReplaceAll(h.Group, "/", "-") + "/" + strings.ReplaceAll(h.Alias, "/", "-")
		fmt.Fprintf(&b, "[Sessions\\%s]\r\n", winscpName(name))
		fmt.Fprintf(&b, "HostName=%s\r\nPortNumber=%d\r\nUserName=%s\r\nFSProtocol=2\r\n", iniValue(h.Hostname), h.Port, iniValue(h.User))
		if h.KeyFile != "" {
			fmt.Fprintf(&b, "PublicKeyFile=%s\r\n", iniValue(h.KeyFile+".ppk"))
			convert = append(convert, "puttygen "+h.KeyFile+" -o "+h.KeyFile+".ppk")
		}
		b.WriteString("\r\n")
	}
	notes := []string{"WinSCP reads these sections from WinSCP.ini: with WinSCP closed, append authama-winscp.ini to the WinSCP.ini beside WinSCP.exe (portable) " +
		"or the one Tools > Preferences > Storage names, then start it. (Tools > Import Sites also takes the filezilla export.) Key paths are this machine's."}
	if len(convert) > 0 {
		notes = append(notes, "WinSCP reads .ppk keys; convert each once (WinSCP offers to when you pick an OpenSSH key):")
		notes = append(notes, convert...)
	}
	notes = append(notes, jumpNotes(hosts, "set Connection > Tunnel on the site in WinSCP")...)
	return &ClientExport{Files: []ClientFile{{Name: "authama-winscp.ini", Data: []byte(b.String())}}, Notes: append(notes, agentNotes(hosts)...)}, nil
}

// fileZillaXML is a FileZilla Site Manager export: a folder per group and an
// SFTP server per host, key file logon (Logontype 5) when it has a key file.
func fileZillaXML(hosts []Host) []byte {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<FileZilla3 version=\"3\">\n  <Servers>\n")
	names, by := groups(hosts)
	for _, g := range names {
		fmt.Fprintf(&b, "    <Folder expanded=\"1\">%s\n", xmlText(g))
		for _, h := range by[g] {
			b.WriteString("      <Server>\n")
			fmt.Fprintf(&b, "        <Host>%s</Host>\n        <Port>%d</Port>\n        <Protocol>1</Protocol>\n        <Type>0</Type>\n", xmlText(h.Hostname), h.Port)
			fmt.Fprintf(&b, "        <User>%s</User>\n", xmlText(h.User))
			if h.KeyFile != "" {
				fmt.Fprintf(&b, "        <Logontype>5</Logontype>\n        <Keyfile>%s</Keyfile>\n", xmlText(h.KeyFile))
			} else {
				b.WriteString("        <Logontype>2</Logontype>\n") // ask: the agent answers first
			}
			fmt.Fprintf(&b, "        <Name>%s</Name>\n        <Comments>%s</Comments>\n", xmlText(h.Alias), xmlText(strings.Join(h.Tags, ", ")))
			b.WriteString("      </Server>\n")
		}
		b.WriteString("    </Folder>\n")
	}
	b.WriteString("  </Servers>\n</FileZilla3>\n")
	return []byte(b.String())
}

func renderFileZilla(hosts []Host) (*ClientExport, error) {
	notes := []string{"In FileZilla, File > Import and choose authama-filezilla.xml; the sites appear in the Site Manager under their groups. " +
		"Key paths are this machine's; FileZilla reads OpenSSH keys."}
	notes = append(notes, jumpNotes(hosts, "FileZilla has no jump host; reach it through a tunnel (ssh -L) or use WinSCP")...)
	return &ClientExport{Files: []ClientFile{{Name: "authama-filezilla.xml", Data: fileZillaXML(hosts)}}, Notes: append(notes, agentNotes(hosts)...)}, nil
}

// renderTransmit writes the FileZilla Site Manager XML under Transmit's own
// name: Transmit 5 imports FileZilla's sites, and reads no format that holds
// a key path, so the notes say what to finish by hand.
func renderTransmit(hosts []Host) (*ClientExport, error) {
	notes := []string{"In Transmit 5, File > Import… and choose authama-transmit.xml as a FileZilla file; the servers appear under their groups. " +
		"Transmit keeps keys in its own store and does not take the key path from the file: add each key under Settings > Keys (or use the SSH agent), then pick it on each server. Keys:"}
	notes = append(notes, keyList(hosts)...)
	notes = append(notes, jumpNotes(hosts, "Transmit has no jump host; reach it through a tunnel (ssh -L)")...)
	return &ClientExport{Files: []ClientFile{{Name: "authama-transmit.xml", Data: fileZillaXML(hosts)}}, Notes: append(notes, agentNotes(hosts)...)}, nil
}

// plistString escapes a value for a property list.
func plistString(s string) string { return xmlText(s) }

// renderCyberduck writes one .duck bookmark per host: an SFTP bookmark, a
// property list, with its key file. The UUID is fixed per alias, so opening
// a re-export updates the bookmark rather than adding one.
func renderCyberduck(hosts []Host) (*ClientExport, error) {
	exp := &ClientExport{}
	for _, h := range hosts {
		var b strings.Builder
		b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\">\n<dict>\n")
		kv := func(k, v string) { fmt.Fprintf(&b, "\t<key>%s</key>\n\t<string>%s</string>\n", k, plistString(v)) }
		kv("Protocol", "sftp")
		kv("Nickname", h.Alias)
		kv("UUID", "authama-"+h.Alias)
		kv("Hostname", h.Hostname)
		kv("Port", strconv.Itoa(h.Port))
		kv("Username", h.User)
		if h.KeyFile != "" {
			kv("Private Key File", h.KeyFile)
		}
		if len(h.Tags) > 0 || h.Group != "" {
			kv("Notes", strings.Join(append([]string{h.Group}, h.Tags...), ", "))
		}
		b.WriteString("</dict>\n</plist>\n")
		exp.Files = append(exp.Files, ClientFile{Name: fileSafe(h.Alias) + ".duck", Data: []byte(b.String())})
	}
	exp.Notes = []string{"Unzip and drag the .duck files onto Cyberduck's Bookmarks (or copy them to ~/Library/Group Containers/G69SCX94XU.duck/Library/Application Support/duck/Bookmarks); " +
		"Mountain Duck reads the same files. Key paths are this machine's."}
	exp.Notes = append(exp.Notes, jumpNotes(hosts, "Cyberduck uses the ProxyJump in ~/.ssh/config when the bookmark's host matches a Host there; otherwise tunnel with ssh -L")...)
	exp.Notes = append(exp.Notes, agentNotes(hosts)...)
	return exp, nil
}

var ansibleUnsafe = regexp.MustCompile(`[^A-Za-z0-9_]`)

// ansibleGroup is a name Ansible takes as a group: letters, digits and _,
// not starting with a digit.
func ansibleGroup(s string) string {
	s = ansibleUnsafe.ReplaceAllString(s, "_")
	if s == "" || s[0] >= '0' && s[0] <= '9' {
		s = "g_" + s
	}
	return s
}

// renderAnsible writes a YAML inventory: under all, a group per category
// holding a group per profile holding its hosts, and a group per environment
// (env_<env>) naming the hosts again. Each host carries ansible_host, port,
// user and its key file; a ProxyJump goes in ansible_ssh_common_args, which
// Ansible hands to ssh.
func renderAnsible(hosts []Host) (*ClientExport, error) {
	cats := map[string]map[string][]Host{}
	envs := map[string][]Host{}
	for _, h := range hosts {
		c := h.Category
		if c == "" {
			c = "ungrouped_profiles"
		}
		if cats[c] == nil {
			cats[c] = map[string][]Host{}
		}
		cats[c][h.Group] = append(cats[c][h.Group], h)
		if h.Env != "" {
			envs[h.Env] = append(envs[h.Env], h)
		}
	}
	hostVars := func(h Host) *yaml.Node {
		v := mapping("ansible_host", h.Hostname, "ansible_port", h.Port, "ansible_user", h.User)
		if h.KeyFile != "" {
			v.Content = append(v.Content, scalar("ansible_ssh_private_key_file"), scalar(h.KeyFile))
		}
		if h.ProxyJump != "" {
			v.Content = append(v.Content, scalar("ansible_ssh_common_args"), scalar("-o ProxyJump="+h.ProxyJump))
		}
		return v
	}
	children := &yaml.Node{Kind: yaml.MappingNode}
	for _, c := range sortedKeys(cats) {
		profiles := &yaml.Node{Kind: yaml.MappingNode}
		for _, p := range sortedKeys(cats[c]) {
			hs := &yaml.Node{Kind: yaml.MappingNode}
			for _, h := range cats[c][p] {
				hs.Content = append(hs.Content, scalar(h.Alias), hostVars(h))
			}
			pg := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalar("hosts"), hs}}
			name := ansibleGroup(p)
			if name == ansibleGroup(c) { // a group cannot be its own child
				name += "_hosts"
			}
			profiles.Content = append(profiles.Content, scalar(name), pg)
		}
		children.Content = append(children.Content, scalar(ansibleGroup(c)),
			&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalar("children"), profiles}})
	}
	for _, e := range sortedKeys(envs) {
		hs := &yaml.Node{Kind: yaml.MappingNode}
		for _, h := range envs[e] {
			hs.Content = append(hs.Content, scalar(h.Alias), &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle})
		}
		children.Content = append(children.Content, scalar("env_"+ansibleGroup(e)),
			&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalar("hosts"), hs}})
	}
	doc := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalar("all"),
		{Kind: yaml.MappingNode, Content: []*yaml.Node{scalar("children"), children}}}}
	data, err := yamlBytes(doc)
	if err != nil {
		return nil, err
	}
	notes := []string{"Use it as an inventory: ansible-inventory -i inventory.yml --graph, or ansible all -i inventory.yml -m ping. " +
		"Groups are categories, then profiles (a profile with no category is under ungrouped_profiles), and env_<env> for each environment. Key paths are this machine's."}
	return &ClientExport{Files: []ClientFile{{Name: "inventory.yml", Data: data}}, Notes: append(notes, agentNotes(hosts)...)}, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// scalar is a YAML string, tagged so "yes", "0600" or "22" stay strings.
func scalar(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }

// mapping builds a mapping from key, value pairs: a string or an int value.
func mapping(kv ...any) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode}
	for i := 0; i+1 < len(kv); i += 2 {
		k, _ := kv[i].(string)
		var v *yaml.Node
		switch x := kv[i+1].(type) {
		case int:
			v = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(x)}
		case string:
			v = scalar(x)
		}
		m.Content = append(m.Content, scalar(k), v)
	}
	return m
}

func yamlBytes(n *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
