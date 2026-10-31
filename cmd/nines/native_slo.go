package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ishantanu/nines/client"
)

func nativeCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "native", Short: "Read native vendor SLOs and prepare reviewed import drafts"}
	for _, action := range []string{"results", "list", "draft"} {
		cmd := &cobra.Command{Use: action + " ID", Args: cobra.ExactArgs(1)}
		cmd.Short = map[string]string{"results": "Read current owned native SLO results over explicit bounds", "list": "List native SLOs on a configured connection", "draft": "Prepare experimental Nines YAML; no import or vendor mutation"}[action]
		var from, to, nativeID, name, project, service string
		if action == "results" {
			cmd.Flags().StringVar(&from, "from", "", "RFC3339 start (required)")
			cmd.Flags().StringVar(&to, "to", "", "RFC3339 end (required)")
		}
		if action == "draft" {
			cmd.Flags().StringVar(&nativeID, "native-id", "", "Vendor SLO identifier")
			cmd.Flags().StringVar(&name, "name", "", "New Nines SLO name")
			cmd.Flags().StringVar(&project, "project", "", "Nines project")
			cmd.Flags().StringVar(&service, "service", "", "Nines service")
		}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			c, e := client.New(client.Config{URL: o.Server, Tenant: o.Tenant, Token: o.Token})
			if e != nil {
				return e
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			var out any
			switch action {
			case "results":
				f, err := time.Parse(time.RFC3339, from)
				if err != nil {
					return usage("--from must be RFC3339")
				}
				t, err := time.Parse(time.RFC3339, to)
				if err != nil {
					return usage("--to must be RFC3339")
				}
				out, e = c.NativeSLOResults(ctx, args[0], f, t)
			case "list":
				out, e = c.NativeSLOCatalog(ctx, args[0])
			case "draft":
				if nativeID == "" || name == "" || project == "" || service == "" {
					return usage("draft requires --native-id, --name, --project and --service")
				}
				draft, err := c.NativeSLODraft(ctx, args[0], nativeID, name, project, service)
				if err != nil {
					return err
				}
				if !o.Agent && o.format("yaml") == "yaml" {
					for _, warning := range draft.Warnings {
						if _, e := fmt.Fprintln(cmd.ErrOrStderr(), warning); e != nil {
							return e
						}
					}
					_, err = fmt.Fprint(cmd.OutOrStdout(), draft.Manifest)
					return err
				}
				out = draft
			}
			if e != nil {
				return e
			}
			return render(cmd.OutOrStdout(), out, o.format("json"), "native-slo")
		}
		group.AddCommand(mark(cmd, "server", "provider-read", cmd.Short))
	}
	return group
}
