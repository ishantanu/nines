package main

import (
	"encoding/base64"
	"errors"
	"io"
	"os"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
)

func visualCommand(o *options) *cobra.Command {
	root := &cobra.Command{Use: "visual", Short: "Assess a consented journey screenshot and review the model receipt"}
	for _, action := range []string{"capabilities", "list", "assess", "review"} {
		c := &cobra.Command{Use: action + " JOURNEY", Args: cobra.ExactArgs(1)}
		if action == "capabilities" {
			c.Use = "capabilities"
			c.Args = cobra.NoArgs
		}
		in := v1.CreateVisualEvaluation{NoticeVersion: v1.VisualConsentNotice}
		review := v1.ReviewVisualEvaluation{}
		imagePath := ""
		if action == "assess" {
			c.Flags().StringVar(&in.RouteDigest, "route-digest", "", "provider/model contract digest from visual capabilities, reviewed before sharing")
			c.Flags().StringVar(&in.ID, "id", "", "request UUIDv4; reuse the same ID and input for a network retry")
			c.Flags().Int64Var(&in.JourneyRevision, "revision", 0, "exact journey revision")
			c.Flags().StringVar(&in.SuccessCriterion, "criterion", "", "visible success criterion (shared with provider)")
			c.Flags().StringVar(&imagePath, "image", "", "redacted PNG or JPEG file, at most 2 MiB")
			c.Flags().BoolVar(&in.Consent, "consent", false, "attest permission to send this image and criterion to the configured provider for one assessment")
		}
		if action == "review" {
			c.Use = "review JOURNEY EVALUATION"
			c.Args = cobra.ExactArgs(2)
			c.Flags().StringVar(&review.RequestDigest, "digest", "", "exact visual receipt request digest")
			c.Flags().StringVar(&review.Outcome, "outcome", "", "success, failure or inconclusive")
			c.Flags().StringVar(&review.Reason, "reason", "", "human review reason; stored with the immutable review")
		}
		c.RunE = func(c *cobra.Command, args []string) error {
			api, e := client.New(client.Config{URL: o.Server, Tenant: o.Tenant, Token: o.Token})
			if e != nil {
				return e
			}
			var out any
			switch action {
			case "capabilities":
				out, e = api.VisualCapabilities(c.Context())
			case "list":
				out, e = api.VisualEvaluations(c.Context(), args[0])
			case "assess":
				id, err := uuid.Parse(in.ID)
				if err != nil || id.Version() != 4 || id.String() != in.ID || in.JourneyRevision < 1 || !in.Consent || len(in.RouteDigest) != 64 || in.SuccessCriterion == "" || imagePath == "" {
					return usage("provide --id UUIDv4, --revision, --image, --criterion --route-digest and explicit --consent; agents cannot grant consent themselves")
				}
				file, err := os.Open(imagePath)
				if err != nil {
					return errors.New("image file could not be opened")
				}
				defer func() { _ = file.Close() }()
				pixels, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
				if err != nil || len(pixels) > 2<<20 {
					return errors.New("image file must be at most two MiB")
				}
				in.ImageBase64 = base64.StdEncoding.EncodeToString(pixels)
				out, e = api.AssessVisual(c.Context(), args[0], in)
			case "review":
				if review.RequestDigest == "" || review.Reason == "" || review.Outcome == "" {
					return usage("provide --digest, --outcome and --reason")
				}
				out, e = api.ReviewVisual(c.Context(), args[0], args[1], review)
			}
			if e != nil {
				return e
			}
			return render(c.OutOrStdout(), out, o.format("json"), "visual evidence")
		}
		effect := "read"
		if action != "list" {
			effect = "write"
		}
		root.AddCommand(mark(c, "server", effect, action+" journey visual evidence"))
	}
	return root
}
func replayDeleteCommand(o *options) *cobra.Command {
	var version int64
	confirmation := ""
	c := &cobra.Command{Use: "delete ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		api, e := client.New(client.Config{URL: o.Server, Tenant: o.Tenant, Token: o.Token})
		if e != nil {
			return e
		}
		if e = api.DeleteSessionReplay(c.Context(), args[0], version, confirmation); e != nil {
			return e
		}
		return render(c.OutOrStdout(), map[string]bool{"deleted": true}, o.format("json"), "replay deletion")
	}}
	c.Flags().Int64Var(&version, "expected-version", 0, "exact recording version")
	c.Flags().StringVar(&confirmation, "confirm-id", "", "exact recording ID; deletion is irreversible")
	return mark(c, "server", "write", "Permanently delete an owner-authorized recording")
}
