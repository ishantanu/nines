package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
)

func replayCommand(action string, o *options) *cobra.Command {
	cmd := &cobra.Command{Use: action, Args: cobra.NoArgs}
	id := ""
	f := cmd.Flags()
	server, workspace := &o.Server, &o.Tenant
	kind := f.String("resource-kind", "", "linked resource kind")
	resource := f.String("resource-id", "", "linked resource ID")
	format := new(string)
	file := f.StringP("file", "f", "", "masked recording JSON request file")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		if len(args) > 0 {
			id = args[0]
		}
		*format = o.format("json")

		if *format != "json" && *format != "yaml" {
			return fmt.Errorf("unexpected arguments or output format")
		}
		c, err := client.New(client.Config{URL: *server, Tenant: *workspace, Token: o.Token})
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
		defer cancel()
		var result any
		switch action {
		case "list":
			result, err = c.SessionReplays(ctx, *kind, *resource)
		case "show":
			result, err = c.SessionReplay(ctx, id)
		case "events":
			result, err = c.ReplayEvents(ctx, id)
		case "ingest":
			if *file == "" {
				return fmt.Errorf("provide -f with a consented masked recording JSON request")
			}
			r, e := openInput(*file)
			if e != nil {
				return e
			}
			defer func() {
				_ = r.Close()
			}()
			b, e := io.ReadAll(io.LimitReader(r, (3<<20)+1))
			if e != nil {
				return e
			}
			if len(b) > 3<<20 {
				return fmt.Errorf("recording request exceeds three MiB")
			}
			var in v1.CreateSessionReplay
			if e = json.Unmarshal(b, &in); e != nil {
				return e
			}
			result, err = c.CreateSessionReplay(ctx, in)
		default:
			return fmt.Errorf("use replays list|show|events|ingest")
		}
		if err != nil {
			return err
		}
		return render(out, result, *format, action)
	}
	return cmd
}
