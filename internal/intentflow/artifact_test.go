package intentflow

import (
	"testing"

	"github.com/ishantanu/nines/internal/tenancy"
)

func TestReviewArtifactIsCompleteScopedAndRejectsTampering(t *testing.T) {
	s, ctx, v := updatedChangeFixture(t, false)
	v, err := s.PrepareChangeSet(ctx, v.ID, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.ReviewArtifact(ctx, v.ID)
	if err != nil || out.Tenant != "alpha" || out.ChangeSet.Digest != v.ChangeSet.Digest || out.AuditIntegrity != "verified" || out.AuditThrough != int64(len(out.Audit)) || len(out.Audit) == 0 || len(out.ChangeSet.Changes) == 0 || out.Status.NextAction != "approve" {
		t.Fatal(out, err)
	}
	wrong := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "other", Subject: "other", Role: "owner"})
	if _, err = s.ReviewArtifact(wrong, v.ID); err == nil {
		t.Fatal("cross tenant read accepted")
	}
	m := s.Repository.(*memory)
	m.mu.Lock()
	r := m.values[v.ID]
	r.Session.ChangeSet.Recommendation.Summary = "tampered"
	m.values[v.ID] = r
	m.mu.Unlock()
	if _, err = s.ReviewArtifact(ctx, v.ID); err == nil {
		t.Fatal("tampered proposal exported")
	}
}
