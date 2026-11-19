package main

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
)

func intentContextCommand(o *options) *cobra.Command {
	in := v1.IntentContextInput{}
	cmd := &cobra.Command{Use: "context", Short: "Read saved service/source context without model or provider calls", Args: cobra.NoArgs}
	f := cmd.Flags()
	f.StringVar(&in.Service, "service", "", "exact service ID")
	f.StringVar(&in.Source, "source", "", "explicit configured telemetry source ID")
	f.StringVar(&in.Project, "project", "", "exact project ID")
	f.StringVar(&in.Journey, "journey", "", "optional exact critical user journey ID")
	f.Int64Var(&in.JourneyRevision, "journey-revision", 0, "required current revision when selecting a journey")
	f.IntVar(&in.Limit, "limit", 5, "matching SLOs per page (1–10); scans at most 100 catalog rows")
	f.StringVar(&in.Cursor, "cursor", "", "nextCursor from the same scope")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if _, err := client.IntentContextPath(in); err != nil {
			return err
		}
		c, err := client.New(client.Config{URL: o.Server, Tenant: o.Tenant, Token: o.Token})
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
		defer cancel()
		out, err := c.IntentContext(ctx, in)
		if err != nil {
			return err
		}
		return render(cmd.OutOrStdout(), out, o.format("json"), "intent-context")
	}
	return mark(cmd, "server", "read", cmd.Short)
}
