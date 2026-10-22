package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ishantanu/nines/client"
)

func reviewSkillCommand(action string, o *options) *cobra.Command {
	cmd := &cobra.Command{Use: action, Args: cobra.NoArgs}
	id := ""
	f := cmd.Flags()
	server, workspace := &o.Server, &o.Tenant
	version := f.String("version", "", "exact skill version")
	format := new(string)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		if len(args) > 0 {
			id = args[0]
		}
		*format = o.format("json")

		if (*format != "json" && *format != "yaml") || action == "list" && *version != "" {
			return fmt.Errorf("unexpected arguments or output format")
		}
		c, err := client.New(client.Config{URL: *server, Tenant: *workspace, Token: o.Token})
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
		defer cancel()
		var result any
		if action == "list" {
			result, err = c.ReviewSkills(ctx)
		} else {
			result, err = c.ReviewSkill(ctx, id, *version)
		}
		if err != nil {
			return err
		}
		return render(out, result, *format, action)
	}
	return cmd
}
