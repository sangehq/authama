package web

import (
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/services/svchealth"
)

// servicesPage is /services: the catalog, attachments, and read-only health.
type servicesPage struct{ s *Server }

// ServicesView is the /services page.
type ServicesView struct {
	Reports   []svchealth.Report
	Catalog   []manifest.ServiceSpec
	Kinds     []string
	Attached  []ServiceAttachment
	ListCmd   string
	HealthCmd string
	HealthOf  string
}

// ServiceAttachment is a host or variant that names a service.
type ServiceAttachment struct {
	Profile, Host, Variant, Service, Kind string
}

func (sp servicesPage) handleServices(w http.ResponseWriter, r *http.Request) {
	p := sp.s.opts.Engine.Home()
	v := ServicesView{
		ListCmd:   brand.Command + " ssh service list",
		HealthCmd: brand.Command + " ssh service health",
		Kinds:     manifest.BuiltinServiceKinds(),
		HealthOf:  strings.TrimSpace(r.URL.Query().Get("health")),
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil && !os.IsNotExist(err) {
		sp.s.renderError(w, r, http.StatusInternalServerError, "Could not read the manifest", err.Error())
		return
	}
	if err == nil {
		v.Catalog = m.Defaults.Services
		if v.HealthOf != "" {
			v.Reports = []svchealth.Report{svchealth.Check(m, v.HealthOf)}
		} else {
			v.Reports = svchealth.CheckAll(m)
		}
		for _, pname := range m.ProfileNames() {
			for _, h := range m.Profiles[pname].Hosts {
				for _, n := range h.Services {
					v.Attached = append(v.Attached, ServiceAttachment{Profile: pname, Host: h.Alias, Service: n, Kind: manifest.ServiceKind(m, n)})
				}
				for _, vari := range h.Variants {
					for _, n := range vari.Services {
						v.Attached = append(v.Attached, ServiceAttachment{Profile: pname, Host: h.Alias, Variant: vari.Name, Service: n, Kind: manifest.ServiceKind(m, n)})
					}
				}
			}
		}
	}
	sp.s.render(w, http.StatusOK, "services", sp.s.newView(r, "services", "Services", v))
}

func serviceUpsertChange(f url.Values, errs []error, edit bool) (change.Change, []error) {
	name := strings.TrimSpace(f.Get("name"))
	kind := strings.TrimSpace(f.Get("kind"))
	if name == "" || kind == "" {
		return change.Change{}, append(errs, badForm("a service needs a name and a kind"))
	}
	spec := manifest.ServiceSpec{Name: name, Kind: kind, Binary: strings.TrimSpace(f.Get("binary")), Label: strings.TrimSpace(f.Get("label"))}
	return change.ServiceUpsert(spec, edit), errs
}

func serviceRemoveChange(f url.Values, errs []error) (change.Change, []error) {
	name := strings.TrimSpace(f.Get("name"))
	if name == "" {
		return change.Change{}, append(errs, badForm("name the service to remove"))
	}
	return change.ServiceRemove(name), errs
}
