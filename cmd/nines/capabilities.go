package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/ishantanu/nines/internal/providers"
	"github.com/ishantanu/nines/skills"
)

type commandCapability struct {
	Command     string           `json:"command"`
	Usage       string           `json:"usage"`
	Description string           `json:"description"`
	Mode        string           `json:"mode"`
	Effect      string           `json:"effect"`
	Flags       []flagCapability `json:"flags"`
	Artifact    string           `json:"artifact,omitempty"`
}
type flagCapability struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Description string `json:"description"`
}

func commandCatalog(root *cobra.Command) any {
	items := []commandCapability{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c.RunE != nil && c.Annotations["mode"] != "" {
			v := commandCapability{Command: c.CommandPath(), Usage: c.UseLine(), Description: c.Short, Mode: c.Annotations["mode"], Effect: c.Annotations["effect"], Flags: []flagCapability{}}
			fs := pflag.NewFlagSet("catalog", pflag.ContinueOnError)
			fs.AddFlagSet(c.InheritedFlags())
			fs.AddFlagSet(c.Flags())
			fs.VisitAll(func(f *pflag.Flag) {
				if !f.Hidden {
					v.Flags = append(v.Flags, flagCapability{f.Name, f.Shorthand, f.Value.Type(), f.DefValue, f.Usage})
				}
			})
			path := c.CommandPath()
			if strings.HasPrefix(path, "nines init ") || path == "nines rules generate" || strings.HasPrefix(path, "nines operator ") || strings.HasPrefix(path, "nines export ") || strings.HasSuffix(path, "conversations kubernetes") {
				v.Artifact = "yaml"
			}
			if path == "nines audit export" {
				v.Artifact = "ndjson"
			}
			if path == "nines skills show" {
				v.Artifact = "markdown"
			}
			items = append(items, v)
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(root)
	return map[string]any{"apiVersion": "nines.dev/v1alpha1", "kind": "CLICapabilities", "commands": items, "telemetryProviders": providers.Default.List(), "authentication": map[string]any{"environmentKeys": []string{"NINES_API_KEY", "API_TOKEN"}, "browser": "Nines authorization code with S256 PKCE; existing OIDC/LDAP identity", "tenantHeader": "X-Nines-Tenant", "nativeSAML": false}, "integrations": map[string]any{"github": map[string]any{"credentialEnvironment": "NINES_GITHUB_TOKEN", "effect": "PR comment publication only; no Nines approval or application"}}, "exitCodes": map[string]string{"0": "success", "1": "execution or validation", "2": "usage", "3": "unauthenticated", "4": "forbidden", "5": "conflict", "6": "unavailable"}}
}
func skillsCommand(o *options) *cobra.Command {
	root := &cobra.Command{Use: "skills", Short: "Read bundled provider-neutral agent guidance offline"}
	root.AddCommand(mark(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		all := append(skills.Catalog(), skills.CLISkill())
		for i := range all {
			all[i].Instructions = ""
		}
		return render(c.OutOrStdout(), all, o.format("json"), "skills")
	}}, "offline", "read", "List embedded skill versions and digests"), mark(&cobra.Command{Use: "show ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		for _, s := range append(skills.Catalog(), skills.CLISkill()) {
			if s.ID == args[0] {
				if o.Output != "" || o.Agent {
					return render(c.OutOrStdout(), s, o.format("json"), "skills")
				}
				_, e := fmt.Fprint(c.OutOrStdout(), s.Instructions)
				return e
			}
		}
		return usage("unknown bundled skill")
	}}, "offline", "read", "Print a bundled SKILL.md or explicit structured output"))
	return root
}
