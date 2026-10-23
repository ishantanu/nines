package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/rules"
)

func telemetryCommand(group, action string, o *options) *cobra.Command {
	cmd := &cobra.Command{Use: action, Args: cobra.NoArgs}
	id := ""
	f := cmd.Flags()
	server, workspace := &o.Server, &o.Tenant
	prefix := f.String("prefix", "", "literal metric prefix")
	metric := f.String("metric", "", "exact metric name")
	labelsFile := f.String("labels", "", "YAML equality-label mapping file")
	limit := f.Int("limit", 25, "page size 1–100")
	cursor := f.String("cursor", "", "next cursor")
	expected := f.Int64("expected-revision", 0, "reviewed binding or map revision")
	name := f.String("name", "", "new SLO name")
	display := f.String("display-name", "", "SLO display name")
	target := f.Float64("target", .999, "target fraction")
	window := f.String("window", "28d", "rolling window")
	format := new(string)
	file := f.StringP("file", "f", "", "self-contained SLO YAML to verify")
	dest := f.String("out", "", "write compiled SLO YAML to a new file")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		if len(args) > 0 {
			id = args[0]
		}
		*format = o.format("json")

		c, err := client.New(client.Config{URL: *server, Tenant: *workspace, Token: o.Token})
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
		defer cancel()
		var result any
		switch {
		case group == "telemetry" && action == "verify":
			if *file == "" {
				return fmt.Errorf("provide -f slo.yaml")
			}
			input, e := openInput(*file)
			if e != nil {
				return e
			}
			body, e := io.ReadAll(io.LimitReader(input, (1<<20)+1))
			_ = input.Close()
			if e != nil {
				return e
			}
			compiled, e := c.CompileMeasurement(ctx, body)
			if e != nil {
				return e
			}
			if *dest != "" {
				return o.generated(out, *dest, []byte(compiled.YAML))
			}
			result = compiled
		case group == "telemetry" && action == "names":
			result, err = c.DiscoverMetricNames(ctx, id, *prefix)
		case group == "telemetry" && action == "inspect":
			labels := map[string]string{}
			if *labelsFile != "" {
				file, e := openInput(*labelsFile)
				if e != nil {
					return e
				}
				b, e := io.ReadAll(io.LimitReader(file, 4097))
				_ = file.Close()
				if e != nil || len(b) > 4096 {
					return fmt.Errorf("labels exceed 4096 bytes")
				}
				if e = yaml.Unmarshal(b, &labels); e != nil {
					return e
				}
			}
			result, err = c.DiscoverTelemetry(ctx, id, *metric, labels)
		case (group == "bindings" || group == "dependencies") && action == "list":
			kind := v1.BindingKind
			if group == "dependencies" {
				kind = v1.DependencyMapKind
			}
			result, err = c.TopologyResources(ctx, kind, *cursor, *limit)
		case group == "dependencies" && action == "show":
			result, err = c.DependencyMap(ctx, id, *expected)
		case group == "bindings" && action == "compile":
			seconds, e := rules.DurationSeconds(*window)
			if e != nil {
				return e
			}
			if *display == "" {
				*display = *name
			}
			compiled, e := c.CompileBinding(ctx, id, v1.CompileBindingInput{ExpectedRevision: *expected, Name: *name, DisplayName: *display, Target: *target, WindowSeconds: seconds})
			if e != nil {
				return e
			}
			if *dest != "" {
				return o.generated(out, *dest, []byte(compiled.YAML))
			}
			result = compiled
		default:
			return fmt.Errorf("unknown telemetry, bindings or dependencies command")
		}
		if err != nil {
			return err
		}
		if *dest != "" {
			return fmt.Errorf("-out applies only to binding compilation")
		}
		return render(out, result, *format, action)
	}
	return cmd
}
