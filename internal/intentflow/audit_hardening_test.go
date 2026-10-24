package intentflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
)

func independentApplier(ctx context.Context) context.Context {
	p, _ := tenancy.FromContext(ctx)
	p.Subject = "release-engineer"
	p.Kind = "user"
	p.Name = "Release engineer"
	p.Role = "editor"
	return tenancy.WithPrincipal(ctx, p)
}
func TestProposalPreparationFailuresAreAudited(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := readyChangeFixture(t, pg)
			if _, err := s.PrepareChangeSet(ctx, v.ID, v.Version+1); !errors.Is(err, ErrConflict) {
				t.Fatal(err)
			}
			log, err := s.Audit(ctx, v.ID)
			if err != nil || len(log.Events) != 1 || log.Events[0].Action != "change_set.proposed" || log.Events[0].Outcome != "conflict" {
				t.Fatal(log, err)
			}
			p, _ := tenancy.FromContext(ctx)
			p.Role = "viewer"
			p.Subject = "read-only"
			if _, err = s.PrepareChangeSet(tenancy.WithPrincipal(ctx, p), v.ID, v.Version); !errors.Is(err, ErrForbidden) {
				t.Fatal(err)
			}
			log, err = s.Audit(ctx, v.ID)
			if err != nil || len(log.Events) != 2 || log.Events[1].Outcome != "denied" {
				t.Fatal(log, err)
			}
		})
	}
}
func TestApprovalRequiresReasonAndIndependentApplier(t *testing.T) {
	s, ctx, v := readyChangeFixture(t, false)
	v, err := s.PrepareChangeSet(ctx, v.ID, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"", "  \n\t"} {
		if _, err = s.ApproveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, reason)); err == nil {
			t.Fatal("approval accepted without a reason")
		}
	}
	v, err = s.ApproveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Reviewed the measurement population and target impact."))
	if err != nil {
		t.Fatal(err)
	}
	in := reviewInput(v, v.ChangeSet.Digest, "")
	if _, err = s.ApplyChangeSet(ctx, v.ID, in); !errors.Is(err, ErrForbidden) {
		t.Fatal("reviewer applied own approval", err)
	}
	if _, err = s.Platform.Store.Read(ctx, "intent-created"); !errors.Is(err, model.ErrNotFound) {
		t.Fatal("denied apply changed definitions", err)
	}
	v, err = s.ApplyChangeSet(independentApplier(ctx), v.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if v.ChangeSet.AppliedBy.Subject == v.ChangeSet.ApprovedBy.Subject {
		t.Fatal("independent identity was not retained")
	}
	log, err := s.Audit(ctx, v.ID)
	if err != nil || log.Integrity != "verified" {
		t.Fatal(log, err)
	}
	if !strings.Contains(log.Events[len(log.Events)-1].Action, "applied") {
		t.Fatal(log.Events)
	}
}
