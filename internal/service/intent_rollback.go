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

// RestoreIntentDefinitions appends revisions; it neither deletes resources nor
// rewinds observation history. Network operations never run inside this batch.
func (s *Service) RestoreIntentDefinitions(ctx context.Context, key, digest string, commit bool, plans []v1.IntentDefinitionChange, expected []v1.IntentAppliedResource, source, sourceDigest string, evidence v1.IntentOperationEvidence) (v1.IntentApplyReceipt, error) {
	s.DefinitionMu.Lock()
	defer s.DefinitionMu.Unlock()
	st, ok := s.Store.(store.DefinitionBatchStore)
	if !ok {
		return v1.IntentApplyReceipt{}, model.ErrUnavailable
	}
	if len(plans) < 1 || len(plans) > 2 || len(expected) < 1 || len(expected) > 2 {
		return v1.IntentApplyReceipt{}, model.Invalid("rollback requires a bounded saved SLO/CUJ change set")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	body, err := st.DefinitionBatch(ctx, key, digest, commit, func(st store.Store) (json.RawMessage, error) {
		if s.SourceFingerprint(source) != sourceDigest {
			return nil, model.ErrRevisionConflict
		}
		scope := New(st, s.Collector)
		scope.RuleManager = s.RuleManager
		scope.TenantID = s.TenantID
		scope.Sources = s.Sources
		scope.DynamicSource = s.DynamicSource
		scope.Now = s.Now
		// Fence the whole applied bundle, including resources that were unchanged.
		for _, want := range expected {
			switch want.Kind {
			case v1.SLOKind:
				actual, e := st.Read(ctx, want.Name)
				if e != nil {
					return nil, e
				}
				if model.ConfigRevision(actual.SLO) != want.Revision || model.DefinitionDigest(model.DefinitionFromSLO(actual.SLO)) != want.Digest {
					return nil, model.ErrRevisionConflict
				}
			case v1.JourneyKind:
				actual, e := scope.Journeys.JourneyRevision(ctx, want.Name, 0)
				if e != nil {
					return nil, e
				}
				if actual.Revision != want.Revision || actual.Digest != want.Digest {
					return nil, model.ErrRevisionConflict
				}
			default:
				return nil, model.Invalid("unsupported rollback resource")
			}
		}
		receipt := v1.IntentApplyReceipt{Resources: []v1.IntentAppliedResource{}, At: s.Now().UTC(), Evidence: &evidence}
		principal, _ := tenancy.FromContext(ctx)
		receipt.Actor = v1.InvestigationActor{Subject: principal.Subject, Name: principal.Name, Kind: principal.Kind}
		for _, change := range plans {
			def, e := model.ParseDefinition(change.Manifest)
			if e != nil {
				return nil, e
			}
			matched := false
			for _, want := range expected {
				matched = matched || (want.Name == def.Metadata().Name && want.Kind == def.Kind() && want.Revision == change.ExpectedRevision)
			}
			if !matched || def.Kind() != change.Kind || def.Metadata().Name != change.Name {
				return nil, model.Invalid("rollback must restore only the original applied resources")
			}
			plan, e := scope.PlanDefinition(ctx, def)
			if e != nil {
				return nil, e
			}
			if plan.ExpectedRevision != change.ExpectedRevision || plan.Digest != change.Digest {
				return nil, model.ErrRevisionConflict
			}
			result, e := scope.ApplyDefinition(ctx, def, change.ExpectedRevision)
			if e != nil {
				return nil, e
			}
			receipt.Resources = append(receipt.Resources, v1.IntentAppliedResource{Kind: def.Kind(), Name: def.Metadata().Name, Revision: result.Revision, Digest: result.Digest})
		}
		return json.Marshal(receipt)
	})
	if err != nil {
		return v1.IntentApplyReceipt{}, err
	}
	var receipt v1.IntentApplyReceipt
	err = json.Unmarshal(body, &receipt)
	return receipt, err
}
