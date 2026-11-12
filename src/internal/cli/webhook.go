package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/services/webhooks"
	"github.com/simtabi/authama/src/internal/util/envnames"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// webhooksAllowPrivate is webhooks.allow_private.
func webhooksAllowPrivate() bool { return settingOn(envnames.WebhooksAllowPrivate, false) }

// newWebhookCmd is `authama webhook`: the endpoints the event feed is sent to.
func newWebhookCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "webhook",
		Short: "Send events to your endpoints as they happen",
		Long: "A webhook sends each new event (a change finished, a watched file changed) to an endpoint you add,\n" +
			"signed as Standard Webhooks do. `authama serve` delivers them when webhooks.enabled is on (it is off by\n" +
			"default); `authama webhook deliver` sends what is due by hand. Endpoints must be https to a public\n" +
			"address unless webhooks.allow_private is on.",
	}
	c.AddCommand(newWebhookAddCmd(), newWebhookListCmd(), newWebhookRemoveCmd(), newWebhookToggleCmd(true),
		newWebhookToggleCmd(false), newWebhookTestCmd(), newWebhookDeliverCmd())
	return c
}

func newWebhookAddCmd() *cobra.Command {
	var events []string
	c := &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add an endpoint; its signing secret is shown once",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			lockForMutation(p)
			ep, err := webhooks.Add(p, args[0], args[1], events, webhooksAllowPrivate(), time.Now())
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(c.OutOrStdout(), ep.Secret)
			w := c.ErrOrStderr()
			_, _ = fmt.Fprintf(w, "added webhook %q -> %s; it is sent what happens from now on.\n", ep.Name, ep.URL)
			_, _ = fmt.Fprintln(w, "The line above is its signing secret, shown once: give it to the receiver to verify the webhook-signature header.")
			if !settingOn(envnames.Webhooks, false) {
				_, _ = fmt.Fprintln(w, "Webhooks are off: `authama settings set webhooks.enabled true` for `serve` to send them, or run `authama webhook deliver`.")
			}
			return nil
		},
	}
	c.Flags().StringSliceVar(&events, "event", nil, "send only events whose type starts with this, whole or without com.simtabi.authama. (repeatable): operation.finished, file.")
	return c
}

func newWebhookListCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List endpoints and how their delivery stands (never their secrets)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			eps, err := webhooks.List(p)
			if err != nil {
				return err
			}
			for i := range eps {
				eps[i].Secret = ""
			}
			out := c.OutOrStdout()
			if asJSON {
				if eps == nil {
					eps = []webhooks.Endpoint{}
				}
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(eps)
			}
			if len(eps) == 0 {
				_, _ = fmt.Fprintln(out, "no webhooks: add one with `authama webhook add <name> <url>`")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "NAME\tURL\tEVENTS\tSTATE\tLAST SENT")
			for _, ep := range eps {
				state := "on"
				switch {
				case ep.Disabled && ep.DisabledWhy != "":
					state = "off: " + ep.DisabledWhy
				case ep.Disabled:
					state = "off"
				case ep.Failures > 0:
					state = fmt.Sprintf("retrying (%d failed: %s)", ep.Failures, ep.LastError)
				}
				evs := strings.Join(ep.Events, ",")
				if evs == "" {
					evs = "all"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", ep.Name, ep.URL, evs, state, orDash(day(ep.LastSuccess)))
			}
			return tw.Flush()
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "a JSON array")
	return c
}

func newWebhookRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an endpoint and its secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			lockForMutation(p)
			if err := webhooks.Remove(p, args[0]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "removed webhook %q\n", args[0])
			return nil
		},
	}
}

func newWebhookToggleCmd(on bool) *cobra.Command {
	use, short := "disable <name>", "Stop sending to an endpoint; it keeps its place"
	if on {
		use, short = "enable <name>", "Send to an endpoint again, from where it stopped"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			lockForMutation(p)
			if err := webhooks.SetDisabled(p, args[0], !on); err != nil {
				return err
			}
			word := "disabled"
			if on {
				word = "enabled"
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "%s webhook %q\n", word, args[0])
			return nil
		},
	}
}

func newWebhookTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <name>",
		Short: "Send one webhook.test event now",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			status, err := webhooks.Test(c.Context(), p, webhooks.Client(webhooksAllowPrivate()), args[0], time.Now())
			if err != nil {
				return fmt.Errorf("webhook %q: %w", args[0], err)
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "webhook %q answered %d\n", args[0], status)
			return nil
		},
	}
}

func newWebhookDeliverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deliver",
		Short: "Send every endpoint the events due to it now",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			lockForMutation(p)
			res, err := webhooks.Deliver(c.Context(), p, webhooks.Client(webhooksAllowPrivate()), time.Now())
			if err != nil {
				return err
			}
			if len(res) == 0 {
				_, _ = fmt.Fprintln(c.OutOrStdout(), "no webhook is on")
				return nil
			}
			failed := false
			for _, r := range res {
				reportDelivery(c.OutOrStdout(), r)
				failed = failed || r.Err != nil
			}
			if failed {
				return errNotClean
			}
			return nil
		},
	}
}

func reportDelivery(w io.Writer, r webhooks.Result) {
	switch {
	case r.Postponed:
		_, _ = fmt.Fprintf(w, "%s: waiting out a backoff after a failure\n", r.Name)
	case r.Disabled:
		_, _ = fmt.Fprintf(w, "%s: sent %d, then turned off: %v\n", r.Name, r.Sent, r.Err)
	case r.Err != nil:
		_, _ = fmt.Fprintf(w, "%s: sent %d, then failed (tried again later): %v\n", r.Name, r.Sent, r.Err)
	default:
		_, _ = fmt.Fprintf(w, "%s: sent %d\n", r.Name, r.Sent)
	}
}

// DeliverEvery is how often `serve` sends what is due, with webhooks on.
var DeliverEvery = 15 * time.Second

// serveWebhooks delivers the feed every DeliverEvery until ctx ends, when
// webhooks.enabled is on. A failure is said once per endpoint and state
// change, not on every tick.
func serveWebhooks(ctx context.Context, p paths.Paths, errOut io.Writer) {
	if !settingOn(envnames.Webhooks, false) {
		return
	}
	client := webhooks.Client(webhooksAllowPrivate())
	go func() {
		said := map[string]string{}
		tick := time.NewTicker(DeliverEvery)
		defer tick.Stop()
		for {
			res, err := webhooks.Deliver(ctx, p, client, time.Now())
			if err != nil {
				_, _ = fmt.Fprintf(errOut, "webhooks: %v\n", err)
			}
			for _, r := range res {
				msg := ""
				if r.Err != nil {
					msg = r.Err.Error()
				}
				if msg != said[r.Name] && !r.Postponed {
					said[r.Name] = msg
					if msg != "" {
						reportDelivery(errOut, r)
					}
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
}
