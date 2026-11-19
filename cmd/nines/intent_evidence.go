package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/model"
)

func intentEvidenceCommand(o *options) *cobra.Command {
	cmd := &cobra.Command{Use: "evidence", Short: "Prepare bounded measurement evidence and explicit trade-offs", Args: cobra.NoArgs}
	revision := cmd.Flags().Int64("expected-revision", 0, "current saved SLO revision; zero requires a new identity")
	file := cmd.Flags().StringP("file", "f", "", "versioned intent YAML or pinned evidence-request YAML")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if *file == "" {
			return errors.New("provide -f evidence-request.yaml")
		}
		input, err := openInput(*file)
		if err != nil {
			return err
		}
		defer func() { _ = input.Close() }()
		body, err := io.ReadAll(io.LimitReader(input, 32769))
		if err != nil {
			return err
		}
		if len(body) > 32768 {
			return errors.New("evidence request exceeds 32 KiB")
		}
		var in v1.IntentEvidenceInput
		var header struct {
			Kind string `yaml:"kind"`
		}
		if err = yaml.Unmarshal(body, &header); err != nil {
			return err
		}
		if header.Kind == v1.IntentKind {
			goal, err := model.ParseIntent(body)
			if err != nil {
				return err
			}
			in.Plan = v1.PlanIntentInput{Intent: goal}
			in.ExpectedRevision = *revision
		} else {
			if cmd.Flags().Changed("expected-revision") {
				return errors.New("put expectedRevision inside evidence request YAML")
			}
			decoder := yaml.NewDecoder(bytes.NewReader(body))
			decoder.KnownFields(true)
			if err = decoder.Decode(&in); err != nil {
				return err
			}
			if decoder.Decode(new(any)) != io.EOF {
				return errors.New("provide exactly one evidence request")
			}
		}

		c, err := client.New(client.Config{URL: o.Server, Tenant: o.Tenant, Token: o.Token})
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 85*time.Second)
		defer cancel()
		result, err := c.PrepareIntentEvidence(ctx, in)
		if err != nil {
			return err
		}
		return render(cmd.OutOrStdout(), result, o.format("json"), "intent-evidence")
	}
	return mark(cmd, "server", "provider-read", cmd.Short)
}
