package service

import (
	"context"
	"encoding/json"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

// IntentDefinitionBatch validates/applies a frozen proposal, never calls a model
// or telemetry provider, and never deploys rules. The SLO and its CUJ link commit
// together or neither does. expected revisions are checked inside that transaction.
func (s *Service) IntentDefinitionBatch(ctx context.Context, key, digest string, commit bool, goal v1.SLOIntent, binding v1.IntentSelection, plans []v1.IntentDefinitionChange, expectedSource string, evidence ...v1.IntentOperationEvidence) (v1.IntentApplyReceipt, error) {
	if len(plans) < 1 || len(plans) > 2 {
		return v1.IntentApplyReceipt{}, model.Invalid("an intent change set requires one SLO and at most one CUJ")
	}
	st, ok := s.Store.(store.DefinitionBatchStore)
	if !ok {
		return v1.IntentApplyReceipt{}, model.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	body, err := st.DefinitionBatch(ctx, key, digest, commit, func(st store.Store) (json.RawMessage, error) {
		if expectedSource != s.SourceFingerprint(goal.Spec.Source) {
			return nil, model.ErrRevisionConflict
		}
		scope := New(st, s.Collector)
		scope.TenantID = s.TenantID
		scope.Sources = s.Sources
		scope.DynamicSource = s.DynamicSource
		scope.Now = s.Now
		var row model.TopologyRevision
		var e error
		if goal.Spec.Measurement == nil {
			row, e = scope.ReadTopology(ctx, v1.BindingKind, binding.Name, 0)
			if e != nil {
				return nil, e
			}
			if row.Revision != binding.ExpectedRevision {
				return nil, model.ErrRevisionConflict
			}
			var measured model.MeasurementBinding
			if e = json.Unmarshal(row.Manifest, &measured); e != nil {
				return nil, e
			}
			if !model.IntentMatchesBinding(goal, measured) {
				return nil, model.Invalid("selected measurement no longer matches the intent")
			}

		}
		out := []v1.IntentAppliedResource{}
		if goal.Spec.Journey != nil && len(plans) != 2 {
			return nil, model.Invalid("the selected CUJ link must be included in the change set")
		}
		for i, change := range plans {
			def, e := model.ParseDefinition(change.Manifest)
			if e != nil {
				return nil, e
			}
			if i == 0 && (def.SLO == nil || def.Metadata().Name != goal.Metadata.Name || def.Metadata().Project != goal.Metadata.Project || model.DefinitionDigest(def.SLO.Spec.Intent) != model.DefinitionDigest(goal.Spec)) {
				return nil, model.Invalid("first change must be the compiled intent SLO")
			}
			if i == 1 && (def.Journey == nil || goal.Spec.Journey == nil || def.Metadata().Name != goal.Spec.Journey.Name || def.Metadata().Project != goal.Metadata.Project) {
				return nil, model.Invalid("second change must be the selected critical user journey")
			}
			if i == 0 {
				ref := def.SLO.Spec.MeasurementBinding
				bindingInvalid := ref == nil || ref.Name != row.ID || ref.Revision != row.Revision || ref.Digest != row.Digest
				if goal.Spec.Measurement != nil {
					bindingInvalid = ref != nil || model.DefinitionDigest(def.SLO.Spec.Measurement) != model.DefinitionDigest(goal.Spec.Measurement) || def.SLO.Spec.StepSeconds != goal.Spec.StepSeconds
				}
				if bindingInvalid || def.SLO.Spec.Target != goal.Spec.Target || def.SLO.Spec.WindowSeconds != goal.Spec.WindowSeconds || (model.DefinitionDigest(def.SLO.Spec.Policy) != model.DefinitionDigest(goal.Spec.Policy) || model.DefinitionDigest(def.SLO.Spec.Policies) != model.DefinitionDigest(goal.Spec.Policies)) {
					return nil, model.Invalid("compiled SLO differs from the reviewed intent or measurement")
				}
			}
			plan, e := scope.PlanDefinition(ctx, def)
			if e != nil {
				return nil, e
			}
			if plan.ExpectedRevision != change.ExpectedRevision || plan.Digest != change.Digest {
				return nil, model.ErrRevisionConflict
			}
			applied, e := scope.ApplyDefinition(ctx, def, change.ExpectedRevision)
			if e != nil {
				return nil, e
			}
			out = append(out, v1.IntentAppliedResource{Kind: def.Kind(), Name: def.Metadata().Name, Revision: applied.Revision, Digest: applied.Digest})
		}
		principal, _ := tenancy.FromContext(ctx)
		receipt := v1.IntentApplyReceipt{Resources: out, Actor: v1.InvestigationActor{Subject: principal.Subject, Name: principal.Name, Kind: principal.Kind}, At: s.Now().UTC()}
		if len(evidence) > 0 {
			receipt.Evidence = &evidence[0]
		}
		return json.Marshal(receipt)
	})
	if err != nil {
		return v1.IntentApplyReceipt{}, err
	}
	var out v1.IntentApplyReceipt
	err = json.Unmarshal(body, &out)
	return out, err
}
