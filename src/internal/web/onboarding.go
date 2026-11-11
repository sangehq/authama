package web

// SetupStep is one step of the Overview's "Get started" card: what to do,
// why, where the form for it is, and whether this home has done it already.
type SetupStep struct {
	Title, Why, Href, Icon string
	Done                   bool
}

// Setup is the "Get started" card: each step a new home goes through, judged
// from what the home already holds, so the card fills in by itself and stops
// showing once every step is done.
type Setup struct {
	Steps []SetupStep
	Done  int
}

// Complete reports whether every step is done.
func (s Setup) Complete() bool { return s.Done == len(s.Steps) }

// Percent is how much of the setup is done, 0 to 100.
func (s Setup) Percent() int {
	if len(s.Steps) == 0 {
		return 100
	}
	return s.Done * 100 / len(s.Steps)
}

// setupFor works out the "Get started" steps from the overview's counts.
func setupFor(d dashboard) Setup {
	steps := []SetupStep{
		{Title: "Create a profile", Why: "A profile groups the servers that share a key policy, such as work or personal.",
			Href: "/profiles#add-profile", Icon: "users", Done: d.Profiles > 0},
		{Title: "Add a host", Why: "A server you log in to. It is written into ~/.ssh/config for you.",
			Href: "/hosts#add-host", Icon: "server", Done: d.Hosts > 0},
		{Title: "Mint its keys", Why: "Creates the key each host needs, in its profile's folder.",
			Href: "/keys#mint-keys", Icon: "key", Done: d.KeysOnDisk > 0},
		{Title: "Add a token", Why: "Store the credential npm, Composer, Docker, git or PyPI reads, so it is audited too.",
			Href: "/tokens#add-token", Icon: "cloud-lock", Done: d.Tokens > 0},
		{Title: "Clear what needs attention", Why: "The doctor's checks pass, and the Overview lists nothing to fix.",
			Href: "/doctor", Icon: "stethoscope", Done: d.Profiles > 0 && d.Problems == 0},
	}
	s := Setup{Steps: steps}
	for _, st := range steps {
		if st.Done {
			s.Done++
		}
	}
	return s
}
