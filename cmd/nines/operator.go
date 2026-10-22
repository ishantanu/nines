package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"gopkg.in/yaml.v3"

	k "github.com/ishantanu/nines/api/kubernetes/v1alpha1"
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/operator"
)

func runOperator(args []string, out io.Writer) error {
	return run(append([]string{"operator"}, args...), out)
}
func operatorCommand(o *options) *cobra.Command {
	root := &cobra.Command{Use: "operator", Short: "Render Kubernetes resources offline"}
	root.AddCommand(mark(&cobra.Command{Use: "crds", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		var out bytes.Buffer
		for i, crd := range operator.CRDs() {
			if i > 0 {
				fmt.Fprintln(&out, "---")
			}
			b, e := operator.YAML(crd)
			if e != nil {
				return e
			}
			out.Write(b)
		}
		_, e := cmd.OutOrStdout().Write(out.Bytes())
		return e
	}}, "offline", "read", "Print CRD schemas"))
	cmd := mark(&cobra.Command{Use: "render", Args: cobra.NoArgs}, "offline", "local-write", "Render local YAML for GitOps review")
	root.AddCommand(cmd)
	flags := cmd.Flags()
	file := flags.StringP("file", "f", "", "native versioned YAML definition")
	namespace := flags.String("namespace", "", "Kubernetes namespace")
	tenant := &o.Tenant
	rulesOnly := flags.Bool("rules-only", false, "emit a GitOps-owned PrometheusRule instead of a Nines CRD")
	binding := flags.String("binding", "", "explicit binding selection for an intent CRD")
	generation := flags.Int64("expected-binding-generation", 0, "reviewed Kubernetes binding generation")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		var buffer bytes.Buffer
		out := io.Writer(&buffer)
		if *file == "" || *namespace == "" {
			return fmt.Errorf("provide -f native.yaml and --namespace")
		}
		if *rulesOnly && !cmd.Flags().Changed("tenant") && !cmd.InheritedFlags().Changed("tenant") && os.Getenv("NINES_TENANT") == "" {
			return fmt.Errorf("rules-only requires an explicit --tenant or NINES_TENANT")
		}
		f, e := openInput(*file)
		if e != nil {
			return e
		}
		defer func() {
			_ = f.Close()
		}()
		b, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
		if e != nil {
			return e
		}
		var header struct {
			Kind string `yaml:"kind"`
		}
		if e = yaml.Unmarshal(b, &header); e != nil {
			return e
		}
		var output any
		if header.Kind == v1.IntentKind {
			if *rulesOnly {
				return fmt.Errorf("compile and verify an intent measurement before rendering rules")
			}
			goal, e := model.ParseIntent(b)
			if e != nil {
				return e
			}
			resource, e := operator.FromIntent(goal, *namespace, k.MeasurementSelection{Name: *binding, ExpectedGeneration: *generation})
			if e != nil {
				return e
			}
			output = resource
		} else {
			d, e := model.ParseDefinition(b)
			if e != nil {
				return e
			}
			if *rulesOnly {
				if d.SLO == nil {
					return fmt.Errorf("rules-only requires an SLO")
				}
				resource, e := operator.PrometheusRule(*d.SLO, *tenant, *namespace, "", nil, 1)
				if e != nil {
					return e
				}
				output = resource
			} else {
				resource, e := operator.FromDefinition(d, *namespace)
				if e != nil {
					return e
				}
				output = resource
			}
		}
		b, e = operator.YAML(output)
		if e != nil {
			return e
		}
		_, e = out.Write(b)
		if e != nil {
			return e
		}
		_, e = cmd.OutOrStdout().Write(buffer.Bytes())
		return e
	}
	return root
}
