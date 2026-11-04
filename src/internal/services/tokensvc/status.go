package tokensvc

import (
	"fmt"
	"os"

	"github.com/simtabi/authama/src/internal/core/tokens"
)

// Status is `authama token status`: the config that would be used, and per
// tool its file, mode, installed count and config entries. The JSON tags are
// `token status --json`'s.
type Status struct {
	Config   StatusConfig   `json:"config"`
	Targets  []StatusTarget `json:"targets"`
	Warnings []string       `json:"warnings"`
}

// StatusConfig is the token config in use.
type StatusConfig struct {
	Path    string `json:"path"`
	Builtin bool   `json:"builtin"`
	Legacy  bool   `json:"legacy"`
}

// StatusTarget is one tool's row.
type StatusTarget struct {
	Target     string `json:"target"`
	File       string `json:"file"`
	Exists     bool   `json:"exists"`
	Mode       string `json:"mode"`
	Installed  int    `json:"installed"`
	Configured int    `json:"configured"`
	Resolved   int    `json:"resolved"`
}

// StatusOf gathers `token status` for config (the discovered one when empty)
// and project (the user files when empty). It writes nothing. A function, not
// an Env method: Env is held to a method ceiling against becoming a facade.
func StatusOf(e Env, config, project string) (Status, error) {
	cfg, src, err := e.ResolveConfig(config)
	if err != nil {
		return Status{}, err
	}
	st := Status{
		Config:   StatusConfig{Path: src.Path, Builtin: src.Path == "", Legacy: src.Legacy},
		Warnings: []string{},
	}
	set := cfg.Resolve(e.Lookup)
	st.Warnings = append(st.Warnings, set.Warnings...)
	configured := map[string]int{}
	for _, sp := range cfg.Specs() {
		configured[sp.Target]++
	}
	installed := map[string]int{}
	for _, it := range e.Inventory(project, false) {
		installed[it.Target]++
	}
	for _, t := range tokens.Targets() {
		row := StatusTarget{Target: t, File: e.TargetPath(t, project), Mode: "-",
			Installed: installed[t], Configured: configured[t], Resolved: set.Len(t)}
		if fi, err := os.Stat(row.File); err == nil {
			row.Exists = true
			if !e.Windows {
				row.Mode = fmt.Sprintf("%04o", fi.Mode().Perm())
			}
		}
		st.Targets = append(st.Targets, row)
	}
	return st, nil
}
