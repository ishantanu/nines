package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ishantanu/nines/client"
)

// gitCommand exposes optional Git delivery. Proposing opens a pull request; it
// never applies, merges or deploys.
func gitCommand(o *options) *cobra.Command {
	group := &cobra.Command{Use: "git", Short: "Propose reviewed SLO and journey definitions as pull or merge requests (optional Git delivery)"}
	connect := func() (*client.Client, error) {
		return client.New(client.Config{URL: o.Server, Tenant: o.Tenant, Token: o.Token})
	}
	info := &cobra.Command{Use: "info", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		api, err := connect()
		if err != nil {
			return err
		}
		out, err := api.GitInfo(c.Context())
		if err != nil {
			return err
		}
		return render(c.OutOrStdout(), out, o.format("json"), "git")
	}}
	group.AddCommand(mark(info, "server", "read", "Show whether Git delivery is configured, with its repository, branch and path"))

	var journey bool
	kindOf := func() string {
		if journey {
			return "journey"
		}
		return "slo"
	}
	proposals := &cobra.Command{Use: "proposals ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		api, err := connect()
		if err != nil {
			return err
		}
		out, err := api.GitProposalsFor(c.Context(), kindOf(), args[0])
		if err != nil {
			return err
		}
		return render(c.OutOrStdout(), out, o.format("json"), "git")
	}}
	proposals.Flags().BoolVar(&journey, "journey", false, "the identifier names a journey, not an SLO")
	group.AddCommand(mark(proposals, "server", "read", "List pull requests opened for an SLO or journey and whether each is applied"))

	drift := &cobra.Command{Use: "drift ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		api, err := connect()
		if err != nil {
			return err
		}
		out, err := api.GitDriftFor(c.Context(), kindOf(), args[0])
		if err != nil {
			return err
		}
		return render(c.OutOrStdout(), out, o.format("json"), "git")
	}}
	drift.Flags().BoolVar(&journey, "journey", false, "the identifier names a journey, not an SLO")
	group.AddCommand(mark(drift, "server", "read", "Compare the repository file with the live definition by digest; reports a difference, not which side is right"))

	var file, digest string
	var expected int64
	propose := &cobra.Command{Use: "propose", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if file == "" || digest == "" {
			return usage("--file and --digest (from nines plan) are required; use --expected-revision from the same plan")
		}
		if !c.Flags().Changed("expected-revision") {
			return usage("--expected-revision is required and must come from the plan you reviewed")
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		api, err := connect()
		if err != nil {
			return err
		}
		out, err := api.ProposeYAML(c.Context(), data, expected, digest)
		if err != nil {
			return err
		}
		if !o.Agent {
			_, _ = fmt.Fprintln(c.ErrOrStderr(), "Pull request opened. The SLO is not applied; your pipeline must deploy it after merge.")
		}
		return render(c.OutOrStdout(), out, o.format("json"), "git")
	}}
	propose.Flags().StringVarP(&file, "file", "f", "", "reviewed SLO YAML file")
	propose.Flags().Int64Var(&expected, "expected-revision", 0, "revision from the reviewed plan (required)")
	propose.Flags().StringVar(&digest, "digest", "", "plan digest from the reviewed plan (required)")
	group.AddCommand(mark(propose, "server", "write", "Open a pull request for a reviewed SLO definition without applying it"))
	return group
}
