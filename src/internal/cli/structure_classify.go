package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/services/structure"
)

func newStructureClassifyCmd() *cobra.Command {
	var asJSON, dryRun bool
	cmd := &cobra.Command{
		Use:   "classify",
		Short: "Refresh purpose and leftover-copy suggestions (writes nothing with --dry-run)",
		Long: "Lists the purpose moves (keys that belong in tools/<tool>/) and leftover-copy\n" +
			"removals authama would suggest. --dry-run only prints the plan; without it the\n" +
			"suggestions are recorded like `ssh structure check`. Nothing is moved.",
		Example: "  " + brand.Command + " ssh structure classify --dry-run\n  " + brand.Command + " ssh structure classify --json",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			ss, err := structure.Suggest(p)
			if err != nil {
				return err
			}
			var plan []structure.Suggestion
			for _, s := range ss {
				if s.Kind == structure.KindPurpose || s.Kind == structure.KindCopyRemove {
					plan = append(plan, s)
				}
			}
			if asJSON {
				if plan == nil {
					plan = []structure.Suggestion{}
				}
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(plan)
			}
			if len(plan) == 0 {
				say(c, "no purpose moves or leftover copies to suggest")
				return nil
			}
			printSuggestions(c.OutOrStdout(), plan)
			if dryRun {
				say(c, "dry run: nothing was recorded or moved")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the suggestions as JSON")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "print the plan; write nothing")
	return cmd
}

func newStructureRulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rules",
		Short: "List, add, change or remove learned purpose rules",
	}
	cmd.AddCommand(newStructureRulesListCmd(), newStructureRulesAddCmd(), newStructureRulesSetCmd(), newStructureRulesRemoveCmd())
	return cmd
}

func newStructureRulesListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List purpose rules",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			rs, err := structure.LoadRules(p)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				if rs == nil {
					rs = []structure.Rule{}
				}
				return enc.Encode(rs)
			}
			if len(rs) == 0 {
				say(c, "no purpose rules")
				return nil
			}
			for _, r := range rs {
				auto := ""
				if r.AutoApply {
					auto = " auto-apply"
				}
				say(c, "%s  tool=%s kind=%s profile=%s name=%s%s", r.ID, r.Tool, r.ToolKind, r.Profile, r.NameGlob, auto)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the rules as JSON")
	return cmd
}

func newStructureRulesAddCmd() *cobra.Command {
	var r structure.Rule
	var yes bool
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a purpose rule (auto-apply is off unless --auto is set)",
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if r.Tool == "" {
				return fmt.Errorf("name the destination tool with --tool (git, docker, …)")
			}
			if err := confirmChange(c, "add a purpose rule for tool "+r.Tool, yes); err != nil {
				return err
			}
			id, err := structure.AddRule(p, r)
			if err != nil {
				return err
			}
			say(c, "added rule %s", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&r.Profile, "profile", "", "limit the rule to this profile")
	cmd.Flags().StringVar(&r.NameGlob, "name", "", "a filename glob (git-signing, *-tunnel)")
	cmd.Flags().StringVar(&r.CommentRe, "comment", "", "a regular expression matched against the .pub comment")
	cmd.Flags().StringVar(&r.Fingerprint, "fingerprint", "", "a SHA256:... public-key fingerprint")
	cmd.Flags().StringVar(&r.Tool, "tool", "", "the tools/<tool>/ slug")
	cmd.Flags().StringVar(&r.ToolKind, "kind", "", "the tool kind (git-signing, docker, …)")
	cmd.Flags().BoolVar(&r.AutoApply, "auto", false, "apply later identical suggestions automatically (off by default; still snapshotted)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "add without confirming")
	return cmd
}

func newStructureRulesSetCmd() *cobra.Command {
	var auto bool
	var autoSet, yes bool
	cmd := &cobra.Command{
		Use:   "set <id>",
		Short: "Change a rule (currently --auto / --auto=false)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if !c.Flags().Changed("auto") {
				return fmt.Errorf("set --auto or --auto=false")
			}
			if err := confirmChange(c, "change rule "+args[0], yes); err != nil {
				return err
			}
			if err := structure.SetRuleAuto(p, args[0], auto); err != nil {
				return err
			}
			say(c, "updated rule %s", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&auto, "auto", false, "turn auto-apply on or off")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "change without confirming")
	_ = autoSet
	return cmd
}

func newStructureRulesRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a purpose rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if err := confirmChange(c, "remove rule "+args[0], yes); err != nil {
				return err
			}
			if err := structure.RemoveRule(p, args[0]); err != nil {
				return err
			}
			say(c, "removed rule %s", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "remove without confirming")
	return cmd
}
