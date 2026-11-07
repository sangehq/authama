package selection

import (
	"fmt"
	"strings"

	"github.com/simtabi/authama/src/internal/services/dataset"
	"github.com/simtabi/authama/src/internal/util/encode"
)

// HostSource is the manifest as a host format reads it, and the order its
// hosts come in.
type HostSource interface {
	encode.HostSource
	ManifestOrder(aliases []string) ([]string, error)
}

// aliasField is the hidden column HostRows adds to learn each row's host.
const aliasField = "alias"

// HostRows runs s, a host selection, and is the aliases of the rows it kept
// with the result as the selection itself would write it. With no sort, the
// aliases come in manifest order, the order `config render` writes them in;
// with one, in the selection's order.
func HostRows(ds *dataset.Dataset, s Selection, src HostSource) ([]string, Result, error) {
	if s.Grain != GrainHost {
		return nil, Result{}, fmt.Errorf("a host format writes hosts; this selection's rows are %ss (export hosts instead)", s.Grain)
	}
	if len(s.Transforms) > 0 {
		return nil, Result{}, fmt.Errorf("a host format writes the hosts themselves, so it takes no transforms; use a table format such as csv or json")
	}
	res, err := Run(ds, s)
	if err != nil {
		return nil, Result{}, err
	}
	withAlias := s
	cols := s.Columns
	if len(cols) == 0 {
		cols = Defaults[s.Grain]
	}
	withAlias.Columns = append(append([]Column{}, cols...), Column{Field: aliasField})
	all, err := Run(ds, withAlias)
	if err != nil {
		return nil, Result{}, err
	}
	aliases := make([]string, 0, len(all.Rows))
	last := len(withAlias.Columns) - 1
	for _, r := range all.Rows {
		aliases = append(aliases, Cell(r[last]))
	}
	if len(s.Sort) == 0 {
		if aliases, err = src.ManifestOrder(aliases); err != nil {
			return nil, Result{}, err
		}
	}
	return aliases, res, nil
}

// HostExport writes the hosts s keeps in the host format f.
func HostExport(ds *dataset.Dataset, s Selection, f encode.Format, src HostSource, o encode.Options) (*encode.ClientExport, Result, error) {
	if !f.IsHosts() {
		return nil, Result{}, fmt.Errorf("%s is a table format, not a host format", f.Name)
	}
	aliases, res, err := HostRows(ds, s, src)
	if err != nil {
		return nil, Result{}, err
	}
	exp, err := f.Hosts(src, aliases, o)
	if err != nil {
		return nil, Result{}, err
	}
	return exp, res, nil
}

// Report is what a host export says beside its file: the hosts left out and
// what is left to do in the client, one line each.
func Report(exp *encode.ClientExport) string {
	var b strings.Builder
	for _, s := range exp.Skipped {
		b.WriteString("skipped: " + s + "\n")
	}
	for _, n := range exp.Notes {
		b.WriteString(n + "\n")
	}
	return b.String()
}
