package main

import (
	"context"
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/githubreview"
)

func githubReviewCommand(o *options, publish bool) *cobra.Command {
	name := "github-plan"
	effect := "read"
	short := "Prepare a GitHub PR review snapshot without contacting GitHub"
	if publish {
		name = "github-publish"
		effect = "external-write"
		short = "Publish an explicitly reviewed snapshot as an immutable GitHub PR comment"
	}
	target := githubreview.Target{}
	var version int64
	var digest, reportDigest string
	var sharing bool
	cmd := &cobra.Command{Use: name + " ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if o.Output != "" && o.Output != "json" && o.Output != "yaml" {
			return usage("GitHub review output must be json or yaml")
		}
		if err := target.Validate(); err != nil {
			return usage(err.Error())
		}
		if publish && (!sharing || version < 1 || len(digest) != 64 || len(reportDigest) != 64) {
			return usage("provide --confirm-sharing and the reviewed --expected-version, --expected-digest and --expected-report-digest")
		}
		c, err := client.New(client.Config{URL: o.Server, Tenant: o.Tenant, Token: o.Token})
		if err != nil {
			return err
		}
		artifact, err := c.IntentReviewArtifact(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		plan, err := githubreview.Prepare(artifact, target)
		if err != nil {
			return err
		}
		if githubreview.ContainsSecret(plan.Markdown, o.Token) {
			return errors.New("review contains the Nines credential")
		}
		if !publish {
			return render(cmd.OutOrStdout(), plan, o.format("json"), "github-review")
		}
		if plan.Version != version || plan.ChangeSetDigest != digest || plan.ReportDigest != reportDigest {
			return &cliError{"conflict", "review changed; prepare and inspect a fresh GitHub review plan", 5}
		}
		token := os.Getenv("NINES_GITHUB_TOKEN")
		publisher, err := githubreview.New(token, o.githubTransport)
		if err != nil {
			return err
		}
		receipt, err := publisher.Publish(cmd.Context(), plan, func(ctx context.Context) error {
			status, e := c.IntentWorkflowStatus(ctx, args[0])
			if e != nil {
				return e
			}
			if status.Tenant != o.Tenant || status.Version != version || status.ChangeSetDigest != digest {
				return githubreview.ErrConflict
			}
			return nil
		})
		if errors.Is(err, githubreview.ErrConflict) {
			return &cliError{"conflict", err.Error(), 5}
		}
		if err != nil {
			return err
		}
		return render(cmd.OutOrStdout(), receipt, o.format("json"), "github-review")
	}}
	f := cmd.Flags()
	f.StringVar(&target.Repository, "repository", "", "Explicit GitHub owner/repository receiving the review")
	f.IntVar(&target.PullRequest, "pull-request", 0, "Explicit pull request number")
	f.StringVar(&target.Head, "head", "", "Full reviewed PR head commit SHA")
	if publish {
		f.Int64Var(&version, "expected-version", 0, "Conversation version in the reviewed plan")
		f.StringVar(&digest, "expected-digest", "", "Exact reviewed change-set digest")
		f.StringVar(&reportDigest, "expected-report-digest", "", "Exact digest of the GitHub review plan")
		f.BoolVar(&sharing, "confirm-sharing", false, "Confirm sharing this report and its sensitive evidence with the repository")
	}
	return mark(cmd, "server", effect, short)
}
