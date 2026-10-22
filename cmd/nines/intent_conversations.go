package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
)

func conversationFile(path string, out any) error {
	f, e := openInput(path)
	if e != nil {
		return e
	}
	defer func() {
		_ = f.Close()
	}()
	b, e := io.ReadAll(io.LimitReader(f, 32769))
	if e != nil {
		return e
	}
	if len(b) > 32768 {
		return fmt.Errorf("request exceeds 32 KiB")
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	var value any
	if e = d.Decode(&value); e != nil {
		return e
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("provide exactly one YAML request")
	}
	data, e := json.Marshal(value)
	if e != nil {
		return e
	}
	jd := json.NewDecoder(bytes.NewReader(data))
	jd.DisallowUnknownFields()
	return jd.Decode(out)
}
func conversationCommand(action string, o *options) *cobra.Command {
	cmd := &cobra.Command{Use: action, Args: cobra.NoArgs}
	id := ""
	f := cmd.Flags()
	server, workspace := &o.Server, &o.Tenant
	file := f.StringP("file", "f", "", "operational YAML request file")
	namespace := f.String("namespace", "", "explicit Kubernetes export namespace")
	digest := f.String("digest", "", "exact reviewed change-set digest")
	cursor := f.String("cursor", "", "metadata page cursor")
	format := new(string)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		if len(args) > 0 {
			id = args[0]
		}
		*format = o.format("json")

		if *format != "json" && *format != "yaml" {
			return fmt.Errorf("unexpected arguments or output format")
		}
		c, e := client.New(client.Config{URL: *server, Tenant: *workspace, Token: o.Token})
		if e != nil {
			return e
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
		defer cancel()
		var result any
		if action != "list" && action != "show" && action != "verify" && action != "kubernetes" && *file == "" {
			return fmt.Errorf("provide -f with an explicit operational YAML request")
		}
		switch action {
		case "kubernetes":
			exported, err := c.ExportIntentKubernetes(ctx, id, *namespace, *digest)
			if err != nil {
				return err
			}
			_, err = io.WriteString(out, exported.YAML)
			return err

		case "changes":
			var in struct {
				ExpectedVersion int64 `json:"expectedVersion"`
			}
			if e = conversationFile(*file, &in); e == nil {
				result, e = c.PrepareIntentChangeSet(ctx, id, in.ExpectedVersion)
			}
		case "approve", "apply":
			var in v1.ReviewIntentChangeSet
			if e = conversationFile(*file, &in); e == nil {
				if action == "approve" {
					result, e = c.ApproveIntentChangeSet(ctx, id, in)
				} else {
					result, e = c.ApplyIntentChangeSet(ctx, id, in)
				}
			}
		case "verify":
			result, e = c.VerifyIntentChangeSet(ctx, id)

		case "list":
			result, e = c.IntentConversations(ctx, *cursor)
		case "show":
			result, e = c.IntentConversation(ctx, id)
		case "create":
			var in v1.CreateIntentConversation
			if e = conversationFile(*file, &in); e == nil {
				result, e = c.CreateIntentConversation(ctx, in)
			}
		case "message":
			var in v1.AppendIntentConversation
			if e = conversationFile(*file, &in); e == nil {
				result, e = c.AppendIntentConversation(ctx, id, in)
			}
		case "run":
			var in v1.CreateIntentFlowRun
			if e = conversationFile(*file, &in); e == nil {
				result, e = c.StartIntentFlowRun(ctx, id, in)
			}
		case "confirm":
			var in v1.ConfirmIntentBinding
			if e = conversationFile(*file, &in); e == nil {
				result, e = c.ConfirmIntentBinding(ctx, id, in)
			}
		case "cancel":
			var in struct {
				ExpectedVersion int64 `json:"expectedVersion"`
			}
			if e = conversationFile(*file, &in); e == nil {
				result, e = c.CancelIntentFlowRun(ctx, id, in.ExpectedVersion)
			}
		default:
			return fmt.Errorf("use intent conversations list|show|create|message|run|confirm|cancel|changes|approve|apply|verify|kubernetes")
		}
		if e != nil {
			return e
		}
		return render(out, result, *format, action)
	}
	return cmd
}
