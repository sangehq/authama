package change

import (
	"fmt"
	"io"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/editor"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// ServiceUpsert is `ssh service add` or `ssh service edit`.
func ServiceUpsert(spec manifest.ServiceSpec, edit bool) Change {
	cmd := "ssh service add"
	if edit {
		cmd = "ssh service edit"
	}
	args := []string{spec.Ref(), "--kind", spec.Kind}
	if spec.Binary != "" {
		args = append(args, "--binary", spec.Binary)
	}
	if spec.Label != "" {
		args = append(args, "--label", spec.Label)
	}
	return Change{
		Command: cmd, Args: args,
		Effects: func(paths.Paths) ([]string, error) {
			return []string{"updates defaults.services in the manifest; does not start a tunnel or change WARP"}, nil
		},
		Run: func(p paths.Paths, out io.Writer) error {
			if err := editor.New(p).UpsertService(spec); err != nil {
				return err
			}
			if _, err := io.WriteString(out, "updated service "+spec.Ref()+"\n"); err != nil {
				return err
			}
			return ApplyManifestEdit(out, p)
		},
	}
}

// ServiceRemove is `ssh service remove`.
func ServiceRemove(name string) Change {
	return Change{
		Command: "ssh service remove", Args: []string{name},
		Effects: func(paths.Paths) ([]string, error) {
			return []string{"removes catalog service " + name + "; hosts that still attach it need a later edit"}, nil
		},
		Run: func(p paths.Paths, out io.Writer) error {
			if err := editor.New(p).RemoveService(name); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "removed service %s\n", name)
			return ApplyManifestEdit(out, p)
		},
	}
}
