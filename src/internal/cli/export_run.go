package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/renderer"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/dataset"
	"github.com/sangehq/authama/src/internal/services/savedsel"
	"github.com/sangehq/authama/src/internal/services/selection"
	"github.com/sangehq/authama/src/internal/util/encode"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// newExportCmd is `export`: any list of the home, filtered, with the columns
// chosen, in any format. The same pipeline as the web UI's Export page and
// /exports/run: a built-in list (or a saved selection), then the filters,
// columns, sort and limit given, then the format.
func newExportCmd() *cobra.Command {
	var (
		format, out, selFile, shape         string
		columns, sortBy                     []string
		profiles, keys, envs, roles, tags   []string
		statuses, targets, kinds, areas     []string
		sources, commands                   []string
		transforms, templates, dedupe       []string
		groupBy, aggregates                 []string
		pivot                               string
		include, exclude, since, until      string
		limit                               int
		narrow, noHeader, compact, listFlds bool
		includeGit, totals                  bool
	)
	names := make([]string, 0, len(selection.Presets))
	for n := range selection.Presets {
		names = append(names, n)
	}
	sort.Strings(names)
	cmd := &cobra.Command{
		Use:   "export [" + strings.Join(names, "|") + "|<saved selection>]",
		Short: "Write any list as CSV, JSON, YAML, a PHP or JS array, Markdown or HTML, or hosts for an SSH client, filtered",
		Long: "Write one of the home's lists, filtered and with the columns chosen, in any format: the\n" +
			"same files the web UI's Export page makes. Nothing in an export holds a secret.\n\n" +
			"The list is one of " + strings.Join(names, ", ") + ", or a saved selection's name (`export selections`\n" +
			"keeps them), or --selection names a selection as a JSON file (the Export page's Copy link holds one). Filters narrow it: --profile, --key (a key's name or\n" +
			"profile/name), --env, --role, --tag, --status, --tool, --kind, --area, --source, --command,\n" +
			"--since and --until (a time or a prefix such as 2026-11), --include and --exclude text.\n" +
			"--columns chooses the columns, by field (--fields lists them; key.fingerprint follows the\n" +
			"host's key), each optionally renamed as field=heading. --sort orders the rows (-field\n" +
			"descends). --format is one of " + strings.Join(encode.TableNames(), ", ") + ".\n\n" +
			"Transforms run after the filters and before the columns, in this order: each --transform\n" +
			"(a step as JSON, the Export page's shape), each --template (as=text naming fields in\n" +
			"braces), --dedupe (keep the first row of each), then --group-by with --aggregate\n" +
			"(count, or op:field for sum, min, max, list or first) and --totals, or --pivot rows:column\n" +
			"(a row per value of rows, a column per value of column, cells by the first --aggregate,\n" +
			"count by default) with --totals. A column can name\n" +
			"the fields they write. The SSH client formats below take none.\n\n" +
			"Hosts also go out as an SSH client imports them: --format " + strings.Join(encode.HostNames(), ", ") + ".\n" +
			"ssh_config is the managed config of the hosts kept, byte for byte what `ssh config export --flat`\n" +
			"writes when every host is kept; termius is the Termius CSV; hosts-csv is a generic CSV; the rest\n" +
			"are the clients' own files. Those formats write hosts in manifest order unless --sort is\n" +
			"given, ignore --columns, leave out git hosts unless --include-git and, except ssh_config,\n" +
			"windows-terminal and iterm2, hosts behind a ProxyCommand. remmina writes a zip of one file per\n" +
			"host. What was left out, and what is left to do in the client, goes to stderr.\n\n" +
			"The file goes to stdout, or to --out.",
		Example: "  " + brand.Command + " export hosts --profile simtabi --env prd --format json\n" +
			"  " + brand.Command + " export keys --columns ref=key,fingerprint,expires_on --sort expires_on --format markdown\n" +
			"  " + brand.Command + " export profiles --key simtabi_simbi-prd-web-00-ed25519 --narrow --format yaml\n" +
			"  " + brand.Command + " export hosts --fields\n" +
			"  " + brand.Command + " export hosts --template 'ssh=ssh {user}@{hostname} -p {port}' --columns alias,ssh\n" +
			"  " + brand.Command + " export hosts --group-by profile --aggregate count,list:env --totals --format markdown\n" +
			"  " + brand.Command + " export hosts --pivot profile:env --totals\n" +
			"  " + brand.Command + " export --selection sel.json --format php --out hosts.php\n" +
			"  " + brand.Command + " export prd-hosts --format json\n" +
			"  " + brand.Command + " export hosts --profile simtabi --format ssh_config --out simtabi.ssh_config\n" +
			"  " + brand.Command + " export hosts --env prd --format putty --out prd.reg",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			sel, err := exportSelection(p, args, selFile)
			if err != nil {
				return err
			}
			ds, err := dataset.Build(p, dataset.Options{Parts: selection.Parts(sel.Grain)})
			if err != nil {
				return err
			}
			steps, err := exportTransforms(transforms, templates, dedupe, groupBy, aggregates, pivot, totals)
			if err != nil {
				return err
			}
			sel.Transforms = append(sel.Transforms, steps...)
			if listFlds {
				res, err := selection.Run(ds, selection.Selection{Version: selection.Version, Grain: sel.Grain, Transforms: sel.Transforms})
				if err != nil {
					return err
				}
				for _, f := range res.Fields {
					_, _ = fmt.Fprintln(c.OutOrStdout(), f)
				}
				return nil
			}
			w := &sel.Where
			for flag, to := range map[string]*[]string{
				"profile": &w.Profile, "key": &w.Key, "env": &w.Env, "role": &w.Role, "tag": &w.Tag,
				"status": &w.Status, "tool": &w.Target, "kind": &w.Kind, "area": &w.Area, "source": &w.Source, "command": &w.Command,
			} {
				if c.Flags().Changed(flag) {
					v, _ := c.Flags().GetStringSlice(flag)
					*to = v
				}
			}
			for flag, to := range map[string]*string{"include": &w.Include, "exclude": &w.Exclude, "since": &w.Since, "until": &w.Until} {
				if c.Flags().Changed(flag) {
					v, _ := c.Flags().GetString(flag)
					*to = v
				}
			}
			if c.Flags().Changed("narrow") {
				sel.Narrow = narrow
			}
			if len(columns) > 0 {
				sel.Columns = nil
				for _, col := range columns {
					field, as, _ := strings.Cut(col, "=")
					sel.Columns = append(sel.Columns, selection.Column{Field: strings.TrimSpace(field), As: strings.TrimSpace(as)})
				}
			}
			if len(sortBy) > 0 {
				sel.Sort = nil
				for _, s := range sortBy {
					s = strings.TrimSpace(s)
					sel.Sort = append(sel.Sort, selection.Sort{Field: strings.TrimPrefix(s, "-"), Desc: strings.HasPrefix(s, "-")})
				}
			}
			if c.Flags().Changed("limit") {
				sel.Limit = limit
			}
			f, err := encode.Get(format)
			if err != nil {
				return err
			}
			if f.IsHosts() {
				return runHostExport(c, p, ds, sel, f, out, encode.Options{IncludeGit: includeGit})
			}
			res, err := selection.Run(ds, sel)
			if err != nil {
				return err
			}
			var buf bytes.Buffer
			if err := f.Write(&buf, encode.Table{Columns: res.Columns, Rows: res.Rows},
				encode.Options{NoHeader: noHeader, Compact: compact, Shape: shape, Title: brand.Command + " " + sel.Grain + "s"}); err != nil {
				return err
			}
			if out == "" || out == stdoutTarget {
				_, err := c.OutOrStdout().Write(buf.Bytes())
				return err
			}
			if err := fs.WriteTextAtomic(out, buf.String(), 0o600); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.ErrOrStderr(), "wrote %d row(s) to %s\n", len(res.Rows), out)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&format, "format", "csv", "csv, tsv, json, jsonl, yaml, php, js, markdown or html; for hosts also "+strings.Join(encode.HostNames(), ", "))
	f.StringVarP(&out, "out", "o", "", "write to this file (mode 0600) instead of stdout")
	f.StringVar(&selFile, "selection", "", "a selection as a JSON file (the Export page's), or a saved selection's name, instead of a list name")
	f.StringSliceVar(&columns, "columns", nil, "the columns, by field, in order; field=heading renames one")
	f.StringSliceVar(&sortBy, "sort", nil, "sort by these fields; -field descends")
	f.IntVar(&limit, "limit", 0, "at most this many rows (0: all)")
	f.StringSliceVar(&profiles, "profile", nil, "only these profiles")
	f.StringSliceVar(&keys, "key", nil, "only rows holding or using these keys (name or profile/name)")
	f.StringSliceVar(&envs, "env", nil, "only these environments")
	f.StringSliceVar(&roles, "role", nil, "only these roles")
	f.StringSliceVar(&tags, "tag", nil, "only rows with one of these tags")
	f.StringSliceVar(&statuses, "status", nil, "only these statuses (a host's status, a key's state)")
	f.StringSliceVar(&targets, "tool", nil, "tokens: only these tools")
	f.StringSliceVar(&kinds, "kind", nil, "events and tokens: only these kinds")
	f.StringSliceVar(&areas, "area", nil, "events: only these areas (ssh, config, auth)")
	f.StringSliceVar(&sources, "source", nil, "history: only these sources (cli, web, ...)")
	f.StringSliceVar(&commands, "command", nil, "history: only these commands, or their subcommands")
	f.StringVar(&include, "include", "", "only rows containing this text, ignoring case")
	f.StringVar(&exclude, "exclude", "", "leave out rows containing this text, ignoring case")
	f.StringVar(&since, "since", "", "only rows at or after this time (a prefix such as 2026-11 works)")
	f.StringVar(&until, "until", "", "only rows at or before this time")
	f.BoolVar(&narrow, "narrow", false, "profiles: count and list only the keys --key chose")
	f.BoolVar(&noHeader, "no-header", false, "csv, tsv, markdown, html: leave the heading row out")
	f.BoolVar(&compact, "compact", false, "json, js, php: no indentation")
	f.StringVar(&shape, "shape", "", "json, js, php, yaml: \"table\" writes {columns, rows} instead of one object per row")
	f.BoolVar(&listFlds, "fields", false, "list every field the list's rows have, after any transforms, and write nothing")
	f.StringArrayVar(&transforms, "transform", nil, `a transform step as JSON, e.g. {"kind":"map","field":"status","values":{"needs-redeploy":"pending"}}`)
	f.StringArrayVar(&templates, "template", nil, "a column from a template: as={profile}/{alias}")
	f.StringSliceVar(&dedupe, "dedupe", nil, "keep the first row of each distinct value of these fields")
	f.StringSliceVar(&groupBy, "group-by", nil, "a row per distinct value of these fields")
	f.StringSliceVar(&aggregates, "aggregate", nil, "with --group-by or --pivot: count, or op:field[=as] for sum, min, max, list, first")
	f.BoolVar(&totals, "totals", false, "with --group-by: a last row over every group; with --pivot, also a total column")
	f.StringVar(&pivot, "pivot", "", "rows:column - a row per value of the rows fields (comma-separated), a column per value of column")
	f.BoolVar(&includeGit, "include-git", false, "host formats: also export git hosts (user git, or a github or gitlab provider)")
	cmd.AddCommand(newExportSelectionsCmd(), newExportScheduleCmd())
	return cmd
}

// exportTransforms are the transform steps the flags name, in the order the
// help gives.
func exportTransforms(raw, templates, dedupe, groupBy, aggregates []string, pivot string, totals bool) ([]selection.Transform, error) {
	var out []selection.Transform
	for _, r := range raw {
		var t selection.Transform
		dec := json.NewDecoder(strings.NewReader(r))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&t); err != nil {
			return nil, fmt.Errorf("--transform %s: %w", r, err)
		}
		out = append(out, t)
	}
	for _, tp := range templates {
		as, text, ok := strings.Cut(tp, "=")
		if !ok || strings.TrimSpace(as) == "" {
			return nil, fmt.Errorf("--template %q: give as=template, e.g. where={profile}/{alias}", tp)
		}
		out = append(out, selection.Transform{Kind: selection.TransformTemplate, As: strings.TrimSpace(as), Template: text})
	}
	if len(dedupe) > 0 {
		out = append(out, selection.Transform{Kind: selection.TransformDedupe, By: dedupe})
	}
	var g selection.Transform
	switch {
	case len(groupBy) > 0 && pivot != "":
		return nil, errors.New("give --group-by or --pivot, not both")
	case len(groupBy) > 0:
		g = selection.Transform{Kind: selection.TransformGroup, By: groupBy, Totals: totals}
	case pivot != "":
		rows, column, ok := strings.Cut(pivot, ":")
		if !ok || strings.TrimSpace(rows) == "" || strings.TrimSpace(column) == "" {
			return nil, fmt.Errorf("--pivot %q: give rows:column, e.g. profile:env", pivot)
		}
		g = selection.Transform{Kind: selection.TransformPivot, Field: strings.TrimSpace(column), Totals: totals}
		for _, r := range strings.Split(rows, ",") {
			g.By = append(g.By, strings.TrimSpace(r))
		}
	default:
		if len(aggregates) > 0 || totals {
			return nil, errors.New("--aggregate and --totals need --group-by or --pivot")
		}
		return out, nil
	}
	for _, a := range aggregates {
		spec, as, _ := strings.Cut(a, "=")
		op, field, _ := strings.Cut(spec, ":")
		g.Aggregates = append(g.Aggregates, selection.Aggregate{Op: strings.TrimSpace(op), Field: strings.TrimSpace(field), As: strings.TrimSpace(as)})
	}
	return append(out, g), nil
}

// exportSelection is the selection a command line names: a built-in list, a
// saved selection by name, or --selection, which is a file or, when no file
// has that name, a saved selection; not both, and one of them.
func exportSelection(p paths.Paths, args []string, file string) (selection.Selection, error) {
	switch {
	case len(args) == 1 && file != "":
		return selection.Selection{}, errors.New("give a list name or --selection, not both")
	case len(args) == 1:
		if s, ok := selection.Presets[args[0]]; ok {
			return s, nil
		}
		if manifest.CheckSelectionName(args[0]) == nil {
			if s, err := savedSelection(p, args[0]); !errors.Is(err, savedsel.ErrNotFound) {
				return s, err
			}
		}
		return selection.Selection{}, fmt.Errorf("no list or saved selection named %q (`%s export selections list` lists those saved)", args[0], brand.Command)
	case file != "":
		b, err := os.ReadFile(file) //nolint:gosec // a path the user named
		if errors.Is(err, os.ErrNotExist) && manifest.CheckSelectionName(file) == nil {
			return savedSelection(p, file)
		}
		if err != nil {
			return selection.Selection{}, err
		}
		return selection.Parse(b)
	}
	return selection.Selection{}, errors.New("name a list (hosts, keys, ...) or give --selection")
}

// savedSelection is the selection saved as name, in either store.
func savedSelection(p paths.Paths, name string) (selection.Selection, error) {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		m = nil // the user's own store is still read
	}
	s, err := savedsel.Get(p, m, name)
	return s.Selection, err
}

// runHostExport writes the hosts sel keeps in the host format f: one file to
// stdout or --out (remmina's is a zip), with what was left out and what is
// left to do in the client on stderr, so a pipe gets a clean file.
func runHostExport(c *cobra.Command, p paths.Paths, ds *dataset.Dataset, sel selection.Selection, f encode.Format, out string, o encode.Options) error {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return err
	}
	src := renderer.Source{M: m, P: exportPaths(p), EmitUseKeychain: platform.EmitUseKeychain()}
	exp, res, err := selection.HostExport(ds, sel, f, src, o)
	if err != nil {
		return err
	}
	data, err := encode.Bundle(f, exp)
	if err != nil {
		return err
	}
	if out == "" || out == stdoutTarget {
		if _, err := c.OutOrStdout().Write(data); err != nil {
			return err
		}
	} else {
		if out, err = exportTarget(p, out, ""); err != nil {
			return err
		}
		if err := fs.WriteTextAtomic(out, string(data), 0o600); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(c.ErrOrStderr(), "wrote %d of %d host(s) to %s\n", len(res.Rows)-len(exp.Skipped), len(res.Rows), out)
	}
	_, _ = fmt.Fprint(c.ErrOrStderr(), selection.Report(exp))
	return nil
}
