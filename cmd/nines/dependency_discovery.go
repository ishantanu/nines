package main

import (
	"io"

	"github.com/spf13/cobra"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/kubediscovery"
)

func dependencyDiscoveryCommand(o *options, live bool) *cobra.Command {
	name, mode, effect := "import-kubernetes", "offline", "local-write"
	if live {
		name, mode, effect = "discover", "server", "provider-read"
	}
	c := &cobra.Command{Use: name, Args: cobra.NoArgs}
	namespace := c.Flags().String("namespace", "", "Kubernetes namespace; required (no cluster-wide reads)")
	id := c.Flags().String("name", "kubernetes-services", "dependency map resource name")
	project := c.Flags().String("project", "production", "Nines project")
	display := c.Flags().String("display-name", "Kubernetes services", "dependency map display name")
	dest := c.Flags().String("out", "", "write candidate map YAML to a new file; never overwrite")
	file := ""
	if !live {
		c.Flags().StringVarP(&file, "file", "f", "", "local Kubernetes List or YAML stream; '-' reads stdin")
	}
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		if !kubediscovery.ValidNamespace(*namespace) {
			return usage("provide a valid --namespace")
		}
		in := v1.DependencyDiscoveryInput{Mode: "cluster", Namespace: *namespace, Name: *id, Project: *project, DisplayName: *display}
		var out v1.DependencyDiscovery
		var err error
		if live {
			var api *client.Client
			api, err = client.New(client.Config{URL: o.Server, Tenant: o.Tenant, Token: o.Token})
			if err == nil {
				out, err = api.DiscoverDependencies(cmd.Context(), in)
			}
		} else {
			if file == "" {
				return usage("provide --file for local Kubernetes resources")
			}
			input, e := openInput(file)
			if e != nil {
				return e
			}
			body, e := io.ReadAll(io.LimitReader(input, kubediscovery.MaxBytes+1))
			_ = input.Close()
			if e != nil {
				return e
			}
			inventory, e := kubediscovery.Project(body, *namespace)
			if e != nil {
				return e
			}
			in.Mode, in.Inventory = "resources", &inventory
			out, err = kubediscovery.Discover(in)
		}
		if err != nil {
			return err
		}
		if *dest != "" {
			return o.generated(cmd.OutOrStdout(), *dest, []byte(out.YAML))
		}
		return render(cmd.OutOrStdout(), out, o.format("json"), "dependency discovery")
	}
	return mark(c, mode, effect, "Preview Kubernetes dependency candidates without saving a map")
}
