package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
)

func auditCommand(o *options) *cobra.Command {
	root := &cobra.Command{Use: "audit", Short: "Search and export tenant decision records"}
	for _, action := range []string{"list", "show", "export", "status"} {
		q := v1.AuditQuery{}
		var from, to string
		cmd := &cobra.Command{Use: action, Args: cobra.NoArgs}
		switch action {
		case "status":
			cmd.Flags().StringVar(&q.Resource, "record", "", "Optional audit record UUID")
		case "show":
			cmd.Use = "show ID"
			cmd.Args = cobra.ExactArgs(1)
		default:
			f := cmd.Flags()
			f.StringVar(&q.Conversation, "conversation", "", "Exact conversation ID")
			f.StringVar(&q.Actor, "actor", "", "Exact authenticated actor subject")
			f.StringVar(&q.Resource, "resource", "", "Exact SLO or journey name")
			f.StringVar(&q.Action, "action", "", "Decision action, for example change_set.approved")
			f.StringVar(&q.Outcome, "outcome", "", "succeeded, failed, denied, conflict, blocked or uncertain")
			f.StringVar(&from, "from", "", "Inclusive RFC3339 start (default: 30 days ago)")
			f.StringVar(&to, "to", "", "Inclusive RFC3339 end (default: now)")
			f.StringVar(&q.Cursor, "cursor", "", "Continue a page with identical filters")
			f.IntVar(&q.Limit, "limit", 50, "Records per page (1–100)")
		}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			c, err := client.New(client.Config{URL: o.Server, Tenant: o.Tenant, Token: o.Token})
			if err != nil {
				return err
			}
			if action == "status" {
				value, err := c.AuditArchiveStatus(cmd.Context(), q.Resource)
				if err != nil {
					return err
				}
				return render(cmd.OutOrStdout(), value, o.format("json"), "audit")
			}
			if action == "show" {
				value, err := c.AuditRecord(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				return render(cmd.OutOrStdout(), value, o.format("json"), "audit")
			}
			if from != "" {
				q.From, err = time.Parse(time.RFC3339Nano, from)
				if err != nil {
					return usage("from must be RFC3339")
				}
			}
			if to != "" {
				q.To, err = time.Parse(time.RFC3339Nano, to)
				if err != nil {
					return usage("to must be RFC3339")
				}
			}
			if action == "list" {
				value, err := c.Audit(cmd.Context(), q)
				if err != nil {
					return err
				}
				return render(cmd.OutOrStdout(), value, o.format("json"), "audit")
			}
			if o.Output != "" && o.Output != "json" {
				return usage("audit export emits newline-delimited JSON; omit output or use json")
			}
			// Streaming makes interruption visible as a nonzero exit; callers must check it before retaining an archive.
			encoder := json.NewEncoder(cmd.OutOrStdout())
			seen := map[string]bool{}
			for {
				page, err := c.Audit(cmd.Context(), q)
				if err != nil {
					return err
				}
				for _, item := range page.Items {
					full, err := c.AuditRecord(cmd.Context(), item.ID)
					if err != nil {
						return err
					}
					if err = encoder.Encode(full); err != nil {
						return err
					}
				}
				if page.NextCursor == "" {
					return nil
				}
				if seen[page.NextCursor] {
					return fmt.Errorf("audit cursor did not advance")
				}
				seen[page.NextCursor] = true
				q.Cursor = page.NextCursor
			}
		}
		root.AddCommand(mark(cmd, "server", "read", map[string]string{"list": "Search tenant decision records", "show": "Read one decision and its frozen proposal", "export": "Stream matching full records as NDJSON", "status": "Read external archive delivery and retention evidence"}[action]))
	}
	return root
}
