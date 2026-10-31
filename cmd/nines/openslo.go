package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/openslo"
)

func opensloCommand(o *options) *cobra.Command {
	group := &cobra.Command{Use: "openslo", Short: "Export the supported OpenSLO v1 subset without contacting providers"}
	var file string
	var acknowledge bool
	export := &cobra.Command{Use: "export", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if file == "" {
			return fmt.Errorf("--file is required")
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		def, err := model.ParseSLODefinition(data)
		if err != nil {
			return err
		}
		result, err := openslo.Export(v1.ServiceLevelObjective(def))
		if err != nil {
			return err
		}
		if o.Agent {
			return render(cmd.OutOrStdout(), result, o.format("json"), "openslo")
		}
		if !acknowledge {
			return fmt.Errorf("OpenSLO export omits Nines-specific semantics; review --agent output, then explicitly use --acknowledge-omissions for a bare document")
		}
		for _, warning := range result.Warnings {
			if _, err = fmt.Fprintln(cmd.ErrOrStderr(), warning); err != nil {
				return err
			}
		}
		return render(cmd.OutOrStdout(), result.Document, o.format("yaml"), "openslo")
	}}
	export.Flags().StringVarP(&file, "file", "f", "", "Nines SLO YAML file")
	export.Flags().BoolVar(&acknowledge, "acknowledge-omissions", false, "Acknowledge omitted Nines policies and measurement semantics before exporting a bare document")
	group.AddCommand(mark(export, "offline", "read", "Export a bounded OpenSLO v1 ratio-counter definition with explicit omissions"))
	return group
}
