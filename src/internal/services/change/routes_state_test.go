package change

import (
	"path/filepath"
	"testing"

	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/mutation"
)

// A home whose state lives apart from its config (XDG_STATE_HOME) previews
// and applies one route change after another without the lines authama wrote
// being taken for hand edits: the preview's copy carries the render record.
func TestRouteChangesInASeparateStateHome(t *testing.T) {
	p := home(t)
	p.StateRoot = filepath.Join(t.TempDir(), "state")
	do := func(ch Change) {
		t.Helper()
		mutation.Release()
		plan, err := Preview(p, ch)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Err != "" {
			t.Fatalf("%s: the preview says it would fail: %s", ch.Command, plan.Err)
		}
		mutation.Release()
		if _, _, err := Apply(p, ch, plan.Hash, 10, history.SourceWeb); err != nil {
			t.Fatalf("%s: %v", ch.Command, err)
		}
	}
	do(ConfigRender())
	do(HostAdd("work", "web", HostInput{Hostname: ptr("web.example"), User: ptr("deploy"), AddAliases: []string{"web-cf"},
		RouteInput: RouteInput{Proxy: ptr("none"), Options: [][2]string{{"ServerAliveInterval", "30"}}}}))
	do(HostAdd("work", "web", HostInput{Hostname: ptr("web2.example"), User: ptr("deploy"), Update: true}))
	do(HostEdit("work", "web", HostInput{AddAliases: []string{"web-b"}, RemoveAliases: []string{"web-cf"},
		RouteInput: RouteInput{ConnectTimeout: ptr(15), Unset: []string{"ServerAliveInterval"}}}))
	do(HostVariantSet("work", "web", "warp", VariantInput{Hostname: ptr("10.0.0.3"), RouteInput: RouteInput{Proxy: ptr("none")}}))
	do(HostVariantSet("work", "web", "warp", VariantInput{Port: ptr(2222)}))
	do(HostVariantRemove("work", "web", "warp"))
	do(HostDelete("work", "web"))
}
