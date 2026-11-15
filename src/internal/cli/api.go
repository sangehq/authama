package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/services/apitokens"
	"github.com/sangehq/authama/src/internal/services/settings"
	"github.com/sangehq/authama/src/internal/util/envnames"
)

// newAPICmd is `authama api`: the tokens programs call the read API with.
func newAPICmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "api",
		Short: "Manage the read API's tokens",
		Long: "The read API is served by `authama serve` at /api/v1/ when api.enabled is on (it is off by default).\n" +
			"Programs call it with an API token: `Authorization: Bearer ama_…`. A token reads only, and is shown\n" +
			"once when it is made; authama keeps only its hash.",
	}
	tok := &cobra.Command{Use: "token", Short: "Make, list and revoke API tokens"}
	tok.AddCommand(newAPITokenCreateCmd(), newAPITokenListCmd(), newAPITokenRevokeCmd())
	c.AddCommand(tok)
	return c
}

func newAPITokenCreateCmd() *cobra.Command {
	var days int
	c := &cobra.Command{
		Use:   "create <name>",
		Short: "Make an API token, shown once",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			lockForMutation(p)
			secret, t, err := apitokens.Create(p, args[0], days, time.Now())
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			_, _ = fmt.Fprintln(out, secret)
			w := c.ErrOrStderr()
			_, _ = fmt.Fprintf(w, "made API token %q (reads only)", t.Name)
			if !t.Expires.IsZero() {
				_, _ = fmt.Fprintf(w, ", expires %s", t.Expires.Format("2006-01-02"))
			}
			_, _ = fmt.Fprintln(w, ".\nIt is shown once: keep it where the program that uses it reads secrets. authama keeps only its hash.")
			if !settingOn(envnames.API, false) {
				_, _ = fmt.Fprintln(w, "The API is off: `authama settings set api.enabled true`, then restart `authama serve`.")
			}
			return nil
		},
	}
	c.Flags().IntVar(&days, "expires", 90, "days until it expires; 0 for never")
	return c
}

func newAPITokenListCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List API tokens (never the tokens themselves)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			toks, err := apitokens.List(p)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if asJSON {
				type row struct {
					Name     string   `json:"name"`
					Scopes   []string `json:"scopes"`
					Created  string   `json:"created"`
					Expires  string   `json:"expires,omitempty"`
					LastUsed string   `json:"last_used,omitempty"`
					Expired  bool     `json:"expired"`
				}
				rows := []row{}
				for _, t := range toks {
					rows = append(rows, row{Name: t.Name, Scopes: t.Scopes, Created: day(t.Created), Expires: day(t.Expires),
						LastUsed: day(t.LastUsed), Expired: t.Expired(time.Now())})
				}
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			if len(toks) == 0 {
				_, _ = fmt.Fprintln(out, "no API tokens: make one with `authama api token create <name>`")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "NAME\tSCOPES\tCREATED\tEXPIRES\tLAST USED")
			for _, t := range toks {
				exp := orNever(day(t.Expires))
				if t.Expired(time.Now()) {
					exp += " (expired)"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%v\t%s\t%s\t%s\n", t.Name, t.Scopes, day(t.Created), exp, orDash(day(t.LastUsed)))
			}
			return tw.Flush()
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "a JSON array")
	return c
}

func newAPITokenRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <name>",
		Short: "Revoke an API token: it stops working at once",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			lockForMutation(p)
			if err := apitokens.Revoke(p, args[0]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "revoked API token %q\n", args[0])
			return nil
		},
	}
}

// settingOn reads a boolean setting as its value in effect: a value set in
// authama.json reaches the environment as 1 or 0, and one set by hand may be
// true, yes or on, so a setting is never compared with the word "true". An
// unset one is def.
func settingOn(name string, def bool) bool {
	v := strings.TrimSpace(envnames.Get(nil, name))
	if v == "" {
		return def
	}
	return settings.Truthy(v)
}

func day(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func orNever(s string) string {
	if s == "" {
		return "never"
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
