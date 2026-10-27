package main

import (
	"github.com/spf13/cobra"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
)

func retirementCommand(o *options) *cobra.Command {
	root := &cobra.Command{Use: "retire", Short: "Review SLO archival, owned-rule cleanup and permanent deletion"}
	for _, action := range []string{"plan", "apply", "status", "retry"} {
		c := &cobra.Command{Use: action + " ID", Args: cobra.ExactArgs(1)}
		in := v1.RetirementInput{}
		if action == "plan" || action == "apply" {
			c.Flags().StringVar(&in.Action, "action", "archive", "archive or delete")
		}
		if action == "apply" {
			c.Flags().Int64Var(&in.ExpectedRevision, "expected-revision", 0, "exact reviewed revision (required)")
			c.Flags().StringVar(&in.Digest, "digest", "", "exact retirement plan digest (required)")
			c.Flags().StringVar(&in.ConfirmName, "confirm-name", "", "SLO identifier confirmation for permanent deletion")
		}
		c.RunE = func(c *cobra.Command, args []string) error {
			api, e := client.New(client.Config{URL: o.Server, Tenant: o.Tenant, Token: o.Token})
			if e != nil {
				return e
			}
			var out any
			switch action {
			case "plan":
				out, e = api.PlanRetirement(c.Context(), args[0], in.Action)
			case "apply":
				out, e = api.ApplyRetirement(c.Context(), args[0], in)
			case "status":
				out, e = api.Retirement(c.Context(), args[0])
			case "retry":
				out, e = api.RetryRetirement(c.Context(), args[0])
			}
			if e != nil {
				return e
			}
			return render(c.OutOrStdout(), out, o.format("json"), "retirement")
		}
		effect := "read"
		if action == "apply" || action == "retry" {
			effect = "write"
		}
		root.AddCommand(mark(c, "server", effect, action+" an SLO retirement"))
	}
	return root
}
