package change

import (
	"fmt"
	"io"
	"strconv"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/editor"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// RouteInput is how ssh reaches a host or a route variant, as the web sets
// it: the flags host add, host edit and host variant set share. nil and empty
// leave each as it is, as an unset flag does.
type RouteInput struct {
	// Proxy is --proxy: cloudflared, none (ProxyCommand none) or off.
	Proxy *string
	// ProxyCommand is --proxy-command; "" removes it.
	ProxyCommand *string
	// ProxyJump is --proxy-jump; "" removes it.
	ProxyJump *string
	// ConnectTimeout is --connect-timeout in seconds; 0 removes it.
	ConnectTimeout *int
	// Options are --option KEY=VALUE; Unset are --unset-option KEY.
	Options [][2]string
	Unset   []string
	// Section and Comment are --section and --comment; "" removes them.
	Section, Comment *string
	// Services are --service names to attach; RemoveServices are --remove-service.
	Services, RemoveServices []string
}

func (r RouteInput) args() []string {
	var a []string
	if r.Proxy != nil {
		a = append(a, "--proxy="+*r.Proxy)
	}
	if r.ProxyCommand != nil {
		a = append(a, "--proxy-command="+*r.ProxyCommand)
	}
	if r.ProxyJump != nil {
		a = append(a, "--proxy-jump="+*r.ProxyJump)
	}
	if r.ConnectTimeout != nil {
		a = append(a, "--connect-timeout="+strconv.Itoa(*r.ConnectTimeout))
	}
	for _, kv := range r.Options {
		a = append(a, "--option="+kv[0]+"="+kv[1])
	}
	for _, k := range r.Unset {
		a = append(a, "--unset-option="+k)
	}
	if r.Section != nil {
		a = append(a, "--section="+*r.Section)
	}
	if r.Comment != nil {
		a = append(a, "--comment="+*r.Comment)
	}
	for _, n := range r.Services {
		a = append(a, "--service="+n)
	}
	for _, n := range r.RemoveServices {
		a = append(a, "--remove-service="+n)
	}
	return a
}

// fields is the editor's form of the input.
func (r RouteInput) fields() editor.RouteFields {
	f := editor.RouteFields{Proxy: r.Proxy, ProxyCommand: r.ProxyCommand, Section: r.Section, Comment: r.Comment,
		UnsetOptions: r.Unset, Services: r.Services, RemoveServices: r.RemoveServices}
	if r.ProxyJump != nil {
		f.SetOptions = append(f.SetOptions, [2]string{"ProxyJump", *r.ProxyJump})
	}
	if r.ConnectTimeout != nil {
		v := ""
		if *r.ConnectTimeout > 0 {
			v = strconv.Itoa(*r.ConnectTimeout)
		}
		f.SetOptions = append(f.SetOptions, [2]string{"ConnectTimeout", v})
	}
	f.SetOptions = append(f.SetOptions, r.Options...)
	return f
}

// VariantInput is what the web sets on a route variant.
type VariantInput struct {
	Hostname *string
	Port     *int
	RouteInput
}

// HostVariantSet is `authama ssh host variant set <profile> <alias> <name> [...]`.
func HostVariantSet(profile, alias, name string, in VariantInput) Change {
	args := []string{profile, alias, name}
	if in.Hostname != nil {
		args = append(args, "--hostname="+*in.Hostname)
	}
	if in.Port != nil {
		args = append(args, fmt.Sprintf("--port=%d", *in.Port))
	}
	args = append(args, in.args()...)
	return Change{
		Command: "host variant set", Args: args,
		Subjects: []history.Subject{{Kind: history.KindProfile, Name: profile}, {Kind: history.KindHost, Name: alias}},
		Run: func(p paths.Paths, out io.Writer) error {
			created, err := editor.RoutesOf(editor.New(p)).SetVariant(profile, alias, name,
				editor.VariantFields{Hostname: in.Hostname, Port: in.Port, RouteFields: in.fields()})
			if err != nil {
				return err
			}
			verb := "updated"
			if created {
				verb = "added"
			}
			_, _ = fmt.Fprintf(out, "%s variant %s of %s: Host %s\n", verb, name, alias, manifest.VariantAlias(name, alias))
			return ApplyManifestEdit(out, p)
		},
	}
}

// HostVariantRemove is `authama ssh host variant remove <profile> <alias> <name>`.
func HostVariantRemove(profile, alias, name string) Change {
	return Change{
		Command: "host variant remove", Args: []string{profile, alias, name},
		Subjects: []history.Subject{{Kind: history.KindProfile, Name: profile}, {Kind: history.KindHost, Name: alias}},
		Run: func(p paths.Paths, out io.Writer) error {
			if err := editor.RoutesOf(editor.New(p)).RemoveVariant(profile, alias, name); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "removed variant %s of %s\n", name, alias)
			return ApplyManifestEdit(out, p)
		},
	}
}

// ConfigKeepEdits is `authama ssh config keep-edits --yes`: what was typed
// into the managed config by hand goes into the manifest, and the config is
// rendered again when every edited block was taken in.
func ConfigKeepEdits() Change {
	return Change{
		Command: "ssh config keep-edits", Args: []string{"--yes"},
		Run: func(p paths.Paths, out io.Writer) error {
			res, err := editor.RoutesOf(editor.New(p)).KeepEdits(platform.EmitUseKeychain(), false)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, res.Format(false))
			if len(res.Left) > 0 {
				_, _ = fmt.Fprintln(out, "the config was not rendered: the blocks left above are still hand edits")
				return nil
			}
			if len(res.Adopted) == 0 {
				return nil
			}
			return ApplyManifestEdit(out, p)
		},
	}
}
