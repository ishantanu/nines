package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
)

func intentCommand(action string, o *options) *cobra.Command {
	cmd := &cobra.Command{Use: action, Args: cobra.NoArgs}
	f := cmd.Flags()
	file := f.StringP("file", "f", "", "versioned intent YAML or draft-request file")
	from := f.String("from", "", "backtest start RFC3339")
	to := f.String("to", "", "backtest end RFC3339")
	resolution := f.String("resolution", "5m", "backtest sampling resolution")
	server, workspace := &o.Server, &o.Tenant
	binding := f.String("binding", "", "explicit binding selection")
	rev := f.Int64("expected-revision", 0, "reviewed binding revision")
	dest := f.String("out", "", "write generated SLO YAML to a new file")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		if *file == "" {
			return fmt.Errorf("provide -f intent.yaml and no positional arguments")
		}
		r, err := openInput(*file)
		if err != nil {
			return err
		}
		defer func() {
			_ = r.Close()
		}()
		body, err := io.ReadAll(io.LimitReader(r, 32769))
		if err != nil {
			return err
		}
		if len(body) > 32768 {
			return fmt.Errorf("intent exceeds 32 KiB")
		}
		c, err := client.New(client.Config{URL: *server, Tenant: *workspace, Token: o.Token})
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 65*time.Second)
		defer cancel()

		if action == "draft" {
			if *binding != "" || *rev != 0 || *from != "" || *to != "" {
				return fmt.Errorf("draft requests do not accept bindings or historical ranges")
			}
			var in v1.DraftIntentInput
			d := yaml.NewDecoder(bytes.NewReader(body))
			d.KnownFields(true)
			if err = d.Decode(&in); err != nil {
				return err
			}
			var extra any
			if d.Decode(&extra) != io.EOF {
				return fmt.Errorf("provide one draft request")
			}
			result, err := c.DraftIntent(ctx, in)
			if err != nil {
				return err
			}
			if *dest != "" {
				if result.Intent == nil {
					return fmt.Errorf("draft needs clarification; inspect without -out")
				}
				b, err := yaml.Marshal(result.Intent)
				if err != nil {
					return err
				}
				return o.generated(out, *dest, b)
			}
			return render(out, result, o.format("json"), action)
		}
		intent, err := model.ParseIntent(body)
		if err != nil {
			return err
		}
		in := v1.PlanIntentInput{Intent: intent}
		if *binding != "" || *rev != 0 {
			if *binding == "" || *rev < 1 {
				return fmt.Errorf("binding and positive expected-revision must be provided together")
			}
			in.Binding = &v1.IntentSelection{Name: *binding, ExpectedRevision: *rev}
		}

		if action == "backtest" {
			if *dest != "" {
				return fmt.Errorf("backtest outputs JSON; -out is only for definition exports")
			}
			start, err := time.Parse(time.RFC3339, *from)
			if err != nil {
				return fmt.Errorf("provide -from RFC3339")
			}
			end, err := time.Parse(time.RFC3339, *to)
			if err != nil {
				return fmt.Errorf("provide -to RFC3339")
			}
			seconds, err := rules.DurationSeconds(*resolution)
			if err != nil {
				return err
			}
			result, err := c.BacktestIntent(ctx, v1.IntentBacktestInput{Plan: in, From: start, To: end, ResolutionSeconds: seconds})
			if err != nil {
				return err
			}
			return render(out, result, o.format("json"), action)
		}
		if *from != "" || *to != "" {
			return fmt.Errorf("historical ranges require intent backtest")
		}
		plan, err := c.PlanIntent(ctx, in)
		if err != nil {
			return err
		}
		if *dest != "" {
			if plan.Status != "ready" || plan.Compiled == nil {
				return fmt.Errorf("intent is %s; inspect the plan without -out to resolve clarifications", plan.Status)
			}
			if plan.Journey != nil {
				return fmt.Errorf("plan includes a CUJ change; inspect the full JSON plan and save both reviewed artifacts")
			}
			return o.generated(out, *dest, []byte(plan.Compiled.YAML))
		}
		return render(out, plan, o.format("json"), action)
	}
	return cmd
}
