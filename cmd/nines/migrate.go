package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ishantanu/nines/internal/migrate"
)

// importCommand converts other tools' SLO definitions offline. It writes
// files or prints YAML; it never contacts a server, saves or applies anything.
func importCommand(o *options) *cobra.Command {
	group := &cobra.Command{Use: "import", Short: "Convert Sloth or Pyrra SLO definitions into Nines definitions without contacting a server"}
	for _, tool := range []string{"sloth", "pyrra"} {
		var file, outDir, project, source, lifecycle, window string
		var force bool
		c := &cobra.Command{Use: tool, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			if file == "" {
				return usage("--file is required")
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			opts := migrate.Options{Project: project, Source: source, Lifecycle: lifecycle}
			if window != "" {
				seconds, err := windowSeconds(window)
				if err != nil {
					return usage(err.Error())
				}
				opts.WindowSeconds = seconds
			}
			result, err := migrate.Convert(tool, data, opts)
			if err != nil {
				return err
			}
			if o.Agent {
				return render(cmd.OutOrStdout(), result, o.format("json"), "migration")
			}
			if outDir == "" {
				for _, d := range result.Definitions {
					if _, err = fmt.Fprintf(cmd.OutOrStdout(), "---\n# from %s\n%s", d.Origin, d.YAML); err != nil {
						return err
					}
				}
			} else {
				if err = os.MkdirAll(outDir, 0o755); err != nil {
					return err
				}
				for _, d := range result.Definitions {
					flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
					if force {
						flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
					}
					f, err := os.OpenFile(filepath.Join(outDir, d.Name+".slo.yaml"), flags, 0o644)
					if err != nil {
						return fmt.Errorf("%v (use --force to overwrite)", err)
					}
					_, werr := f.WriteString(d.YAML)
					if cerr := f.Close(); werr == nil {
						werr = cerr
					}
					if werr != nil {
						return werr
					}
				}
			}
			stderr := cmd.ErrOrStderr()
			for _, n := range result.Notes {
				_, _ = fmt.Fprintf(stderr, "note: %s\n", n.Message)
			}
			for _, d := range result.Definitions {
				for _, n := range d.Notes {
					if n.Level != "info" {
						_, _ = fmt.Fprintf(stderr, "%s [%s] %s: %s\n", d.Name, n.Level, n.Field, n.Message)
					}
				}
			}
			for _, s := range result.Skipped {
				_, _ = fmt.Fprintf(stderr, "skipped %s: %s\n", s.Origin, s.Reason)
			}
			_, _ = fmt.Fprintf(stderr, "%d converted, %d skipped. Review each definition, then nines plan and nines apply.\n", len(result.Definitions), len(result.Skipped))
			return nil
		}}
		c.Flags().StringVarP(&file, "file", "f", "", "YAML file with one or more "+strings.ToUpper(tool[:1])+tool[1:]+" definitions")
		c.Flags().StringVar(&outDir, "out-dir", "", "write one <name>.slo.yaml per SLO here instead of printing")
		c.Flags().BoolVar(&force, "force", false, "overwrite existing files in --out-dir")
		c.Flags().StringVar(&project, "project", "", "Nines project (default production)")
		c.Flags().StringVar(&source, "source", "", "Nines connection that serves the queries (default primary)")
		c.Flags().StringVar(&lifecycle, "lifecycle", "", "experimental (default, measures without paging), aspirational or committed")
		c.Flags().StringVar(&window, "window", "", "SLO window, for example 30d (Sloth only; Pyrra carries its own)")
		group.AddCommand(mark(c, "offline", "read", "Convert "+strings.ToUpper(tool[:1])+tool[1:]+" definitions; review notes and skipped items before applying"))
	}
	return group
}

// windowSeconds reads durations such as 30d, 7d or 720h.
func windowSeconds(s string) (int64, error) {
	var n int64
	var unit string
	if _, err := fmt.Sscanf(s, "%d%s", &n, &unit); err != nil || n <= 0 {
		return 0, fmt.Errorf("--window must look like 30d, 7d or 720h")
	}
	switch unit {
	case "d":
		return n * 86400, nil
	case "h":
		return n * 3600, nil
	case "w":
		return n * 7 * 86400, nil
	}
	return 0, fmt.Errorf("--window must look like 30d, 7d or 720h")
}
