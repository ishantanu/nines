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
)

func intentCandidatesCommand(o *options) *cobra.Command {
	cmd := &cobra.Command{Use: "candidates", Short: "Discover measurement candidates with bounded provider reads", Args: cobra.NoArgs}
	file := cmd.Flags().StringP("file", "f", "", "candidate request YAML with exact source and population")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if *file == "" {
			return errors.New("provide -f candidate-request.yaml")
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
			return errors.New("candidate request exceeds 32 KiB")
		}
		var in v1.IntentCandidateInput
		decoder := yaml.NewDecoder(bytes.NewReader(body))
		decoder.KnownFields(true)
		if err = decoder.Decode(&in); err != nil {
			return err
		}
		if decoder.Decode(new(any)) != io.EOF {
			return errors.New("provide exactly one candidate request")
		}
		c, err := client.New(client.Config{URL: o.Server, Tenant: o.Tenant, Token: o.Token})
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
		defer cancel()
		result, err := c.DiscoverIntentCandidates(ctx, in)
		if err != nil {
			return err
		}
		return render(cmd.OutOrStdout(), result, o.format("json"), "intent-candidates")
	}
	return mark(cmd, "server", "provider-read", cmd.Short)
}
