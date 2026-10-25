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
	after := f.Int64("after", 0, "Continue conversation audit after this sequence")
	through := f.Int64("through", 0, "Freeze audit export at this sequence (zero: current)")
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
		if action != "list" && action != "show" && action != "verify" && action != "kubernetes" && action != "rollback-kubernetes" && action != "audit" && *file == "" {
			return fmt.Errorf("provide -f with an explicit operational YAML request")
		}
		switch action {
		case "kubernetes", "rollback-kubernetes":
			var exported v1.IntentKubernetesExport
			var err error
			if action == "kubernetes" {
				exported, err = c.ExportIntentKubernetes(ctx, id, *namespace, *digest)
			} else {
				exported, err = c.ExportIntentRollbackKubernetes(ctx, id, *namespace, *digest)
			}
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
		case "audit":
			result, e = c.IntentAuditPage(ctx, id, *after, *through)
		case "reject", "rollback-plan", "rollback-approve", "rollback-apply", "rollback-reject":
			var in v1.ReviewIntentChangeSet
			if e = conversationFile(*file, &in); e == nil {
				switch action {
				case "reject":
					result, e = c.RejectIntentChangeSet(ctx, id, in)
				case "rollback-plan":
					result, e = c.PrepareIntentRollback(ctx, id, in)
				case "rollback-approve":
					result, e = c.ApproveIntentRollback(ctx, id, in)
				case "rollback-apply":
					result, e = c.ApplyIntentRollback(ctx, id, in)
				case "rollback-reject":
					result, e = c.RejectIntentRollback(ctx, id, in)
				}
			}
		case "approve", "apply", "observe":
			var in v1.ReviewIntentChangeSet
			if e = conversationFile(*file, &in); e == nil {
				switch action {
				case "observe":
					result, e = c.ObserveIntentChangeSet(ctx, id, in)
				case "approve":
					result, e = c.ApproveIntentChangeSet(ctx, id, in)
				default:
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
			return fmt.Errorf("use intent conversations list|show|create|message|run|confirm|cancel|changes|approve|reject|apply|audit|verify|observe|rollback-plan|rollback-approve|rollback-apply|rollback-reject|kubernetes|rollback-kubernetes")
		}
		if e != nil {
			return e
		}
		return render(out, result, *format, action)
	}
	return cmd
}
