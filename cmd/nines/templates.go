package main

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ishantanu/nines/internal/slotemplates"
)

func templatesCommand(o *options) *cobra.Command {
	group := &cobra.Command{Use: "templates", Short: "Browse SLI categories and generate experimental SLO YAML without a server"}
	var category string
	var lock string
	group.PersistentFlags().StringVar(&lock, "extension-lock", "", "Pinned extension lockfile; no network access")
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		registry, err := slotemplates.Load(lock)
		if err != nil {
			return err
		}
		catalog, err := registry.List(category)
		if err != nil {
			return err
		}
		if o.format("table") != "table" {
			return render(cmd.OutOrStdout(), catalog, o.format("table"), "templates")
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		if _, err = fmt.Fprintln(w, "ID\tCATEGORY\tSLI\tREVISION"); err != nil {
			return err
		}
		for _, t := range catalog.Templates {
			if _, err = fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", t.ID, t.Category, t.SLI, t.Revision); err != nil {
				return err
			}
		}
		return w.Flush()
	}}
	list.Flags().StringVar(&category, "category", "", "SLI category, for example agent or inference")
	group.AddCommand(mark(list, "offline", "read", "List shipped SLO templates by category"))
	show := &cobra.Command{Use: "show ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		registry, err := slotemplates.Load(lock)
		if err != nil {
			return err
		}
		t, err := registry.Get(args[0])
		if err != nil {
			return err
		}
		return render(cmd.OutOrStdout(), t, o.format("yaml"), "template")
	}}
	group.AddCommand(mark(show, "offline", "read", "Read a template, requirements and example measurement"))
	var valuesFile, dest, name, project, service, source string
	generate := &cobra.Command{Use: "generate ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		values, err := readTemplateValues(valuesFile)
		if err != nil {
			return err
		}

		if cmd.Flags().Changed("name") {
			values.Name = name
		}
		if cmd.Flags().Changed("project") {
			values.Project = project
		}
		if cmd.Flags().Changed("service") {
			values.Service = service
		}
		if cmd.Flags().Changed("source") {
			values.Source = source
		}
		registry, err := slotemplates.Load(lock)
		if err != nil {
			return err
		}
		template, err := registry.Get(args[0])
		if err != nil {
			return err
		}
		if values.ExpectedTemplateDigest == "" {
			values.ExpectedTemplateDigest = template.Digest
		}
		draft, err := registry.Render(args[0], values)
		if err != nil {
			return err
		}
		for _, warning := range draft.Warnings {
			if _, err = fmt.Fprintln(cmd.ErrOrStderr(), warning); err != nil {
				return err
			}
		}
		if dest != "" {
			return o.generated(cmd.OutOrStdout(), dest, []byte(draft.YAML))
		}
		if o.Agent || o.Output != "" {
			return render(cmd.OutOrStdout(), draft, o.format("json"), "template-draft")
		}
		_, err = io.WriteString(cmd.OutOrStdout(), draft.YAML)
		return err
	}}
	generate.Flags().StringVar(&valuesFile, "values", "", "YAML file of template values; measurement replaces the entire example measurement")
	generate.Flags().StringVar(&dest, "out", "", "Write SLO YAML to a new file; never overwrite")
	generate.Flags().StringVar(&name, "name", "", "SLO identifier")
	generate.Flags().StringVar(&project, "project", "", "Project name")
	generate.Flags().StringVar(&service, "service", "", "Service and default service population label")
	generate.Flags().StringVar(&source, "source", "", "Configured telemetry connection name")
	group.AddCommand(mark(generate, "offline", "local-write", "Generate editable SLO YAML, or a draft receipt with --agent"))
	return group
}
