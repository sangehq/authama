package vmsync

import (
	"fmt"
	"sort"

	"github.com/simtabi/authama/src/internal/services/dockerctx"
)

// ContextStatus is one Docker context that belongs to a VM host: whether it
// still points where it should, the VM's state, and (when asked) whether the
// engine behind it answers. doctor and the web UI's VMs page show these.
type ContextStatus struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	// Managed: authama made it (its description), or it points exactly
	// where an authama context for its name would.
	Managed bool `json:"managed"`
	Current bool `json:"current"`
	// Host is profile/alias of the VM host by that name, "" when none.
	Host string `json:"host,omitempty"`
	// VMState is the hypervisor's word for the host's VM (running, stopped,
	// gone, ...), "" when there is no host.
	VMState string `json:"vm_state,omitempty"`
	// Docker is the engine check; nil when it was not made (not asked for,
	// or the VM is not running: a check never starts a VM).
	Docker *dockerctx.Diagnosis `json:"docker,omitempty"`
	// Problem says why the context needs attention, "" when it does not;
	// Fix says what to do.
	Problem string `json:"problem,omitempty"`
	Fix     string `json:"fix,omitempty"`
}

// Checker asks the engine behind a context; dockerctx.Check is the real one.
type Checker func(name, user string) dockerctx.Diagnosis

// ContextStatuses lists the Docker contexts that belong to VM hosts: every
// one authama manages, and any other named after a VM host. Other contexts
// (default, orbstack, desktop-linux, ...) are not authama's and are left out.
// With check, the engine behind each context of a running VM is asked; without
// it nothing connects to a guest.
func ContextStatuses(plan Plan, contexts []dockerctx.Context, check Checker) []ContextStatus {
	hosts := map[string]Host{}
	for _, h := range plan.Hosts {
		hosts[h.Alias] = h
	}
	var out []ContextStatus
	for _, c := range contexts {
		h, isHost := hosts[c.Name]
		if !c.Managed() && !isHost {
			continue
		}
		st := ContextStatus{Name: c.Name, Endpoint: c.Endpoint, Managed: c.Managed(), Current: c.Current}
		if isHost {
			st.Host, st.VMState = h.Profile+"/"+h.Alias, h.State
		}
		want := dockerctx.Endpoint(c.Name)
		switch {
		case !isHost:
			st.Problem = "no VM host " + c.Name + " is left"
			if c.Description == dockerctx.Marker && c.Endpoint == want {
				st.Fix = "authama ssh vm sync removes it (exported to a backup first)"
			} else {
				st.Fix = "remove it if you no longer use it: docker context rm " + c.Name
			}
		case h.State == "gone":
			st.Problem = "the VM is gone"
			st.Fix = "authama ssh vm sync removes the host and this context"
		case c.Endpoint != want && c.Description == dockerctx.Marker:
			st.Problem = fmt.Sprintf("points at %s, not %s", c.Endpoint, want)
			st.Fix = "authama ssh vm sync refreshes it"
		case c.Endpoint != want:
			// Named after a host, made by someone else: shown, never judged.
		case h.Running && check != nil:
			d := check(c.Name, h.User)
			st.Docker = &d
			if !d.Healthy() {
				st.Problem = "the Docker engine does not answer (" + d.State + ")"
				if d.Detail != "" {
					st.Problem += ": " + d.Detail
				}
				st.Fix = d.Fix
			}
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
