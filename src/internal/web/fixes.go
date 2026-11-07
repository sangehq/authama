package web

// fixAction is how the web UI performs one of the fixes the overview's state
// rows name. A row's Fix is the CLI command that repairs it; on the overview
// that command stays visible, and beside it the page offers the same repair:
// a change previewed in place (Action), or the page that offers it (Page).
type fixAction struct {
	// Action is a change kind (see actionFields) previewed straight from the
	// row, so the diff is shown and nothing is written until it is confirmed.
	Action string
	// Page is the page, with an optional #panel, where the fix is offered,
	// for fixes that need a choice the row cannot make (which key to rotate).
	Page string
	// Label is the button's text.
	Label string
}

// overviewFixes maps each fix command in services/overview to the web UI's way
// of doing it. TestEveryOverviewFixIsPerformedOrCLIOnly holds it to the
// overview's source, so a new fix there fails until it is mapped here or
// listed in cliOnlyFixes.
var overviewFixes = map[string]fixAction{
	"authama ssh reconcile":      {Action: "reconcile", Label: "Preview the fix"},
	"authama doctor --fix":       {Action: "doctor-fix", Label: "Preview the fix"},
	"authama doctor":             {Page: "/doctor", Label: "Open Doctor"},
	"authama ssh rotate":         {Page: "/keys", Label: "Open Keys"},
	"authama ssh knownhosts pin": {Page: "/hosts#knownhosts-init", Label: "Pin host keys"},
}

// cliOnlyFixes are the overview's fixes with no web counterpart, and why. The
// row still shows the command to run.
var cliOnlyFixes = map[string]string{
	"authama init":         "creates the home the web UI itself runs in; the engine asks for it before any page can change anything",
	"authama migrate":      "moves an old config home into place, which is done once from the shell that owns it",
	"not your real ~/.ssh": "a note on the dev-mode row, not a command",
}

// fixFor is the web UI's way of doing fix, or nil when there is none.
func fixFor(fix string) *fixAction {
	if a, ok := overviewFixes[fix]; ok {
		return &a
	}
	return nil
}
