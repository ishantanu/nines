package investigation

import (
	"errors"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestPostmortemOnAnIncidentReview(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	s.Platform.IncidentsEnabled = true
	// Postmortems belong to incident reviews.
	slo, err := s.Create(ctx, input())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.PostmortemTemplate(ctx, slo.ID); err == nil {
		t.Fatal("postmortem started on an SLO review")
	}
	declared, err := s.Platform.DeclareIncident(ctx, "ana", "Replica lag", nil)
	if err != nil {
		t.Fatal(err)
	}
	in := input()
	in.ID = "review-incident"
	in.Scope = v1.InvestigationScope{Kind: v1.IncidentScope, ResourceID: declared.ID, ExpectedRevision: 1, At: in.Scope.At}
	review, err := s.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	// No SLO detected it, so the template carries the standing item.
	tmpl, err := s.PostmortemTemplate(ctx, review.ID)
	if err != nil || len(tmpl.Postmortem.ActionItems) != 1 || tmpl.Postmortem.ActionItems[0].ID != v1.DefineSLOAction || tmpl.Revision != "" {
		t.Fatal(tmpl, err)
	}
	p := tmpl.Postmortem
	p.Summary = "The replica fell behind; the on-call engineer should have failed over sooner."
	p.ContributingCauses = []v1.ContributingCause{{Kind: v1.CauseLatent, Text: "Replication lag had no SLO."}, {Kind: v1.CauseTrigger, Text: "A bulk import saturated the replica."}}
	p.ActionItems = append(p.ActionItems, v1.ActionItem{ID: "alert-lag", Title: "Page on replication lag", Owner: "db-team", Due: "2026-12-15", Kind: v1.ActionDetect, SLOs: []string{"checkout"}})
	// Removing the standing item is refused.
	without := p
	without.ActionItems = without.ActionItems[1:]
	if _, _, err = s.SavePostmortem(ctx, review.ID, v1.SavePostmortem{ID: "pm-0", ExpectedVersion: 1, Postmortem: without}); err == nil || !strings.Contains(err.Error(), "Define an SLO") {
		t.Fatal("standing item removed", err)
	}
	saved, findings, err := s.SavePostmortem(ctx, review.ID, v1.SavePostmortem{ID: "pm-1", ExpectedVersion: 1, Postmortem: p})
	if err != nil || saved.Version != 2 {
		t.Fatal(saved.Version, err)
	}
	if len(findings) != 1 || findings[0].Phrase != "should have" || findings[0].Field != "summary" {
		t.Fatalf("findings %+v", findings)
	}
	// A stale save conflicts; a new revision keeps the old one.
	if _, _, err = s.SavePostmortem(ctx, review.ID, v1.SavePostmortem{ID: "pm-2", ExpectedVersion: 1, Postmortem: p}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale save accepted", err)
	}
	p.ActionItems[0].Done = true
	if saved, _, err = s.SavePostmortem(ctx, review.ID, v1.SavePostmortem{ID: "pm-2", ExpectedVersion: 2, Postmortem: p}); err != nil || len(saved.Entries) != 2 {
		t.Fatal(err)
	}
	if tmpl, _ = s.PostmortemTemplate(ctx, review.ID); tmpl.Revision != "pm-2" || !tmpl.Postmortem.ActionItems[0].Done || len(tmpl.Findings) != 1 {
		t.Fatalf("template is not the latest revision: %+v", tmpl)
	}
	// Open items link their SLOs; done items drop off.
	open, err := s.OpenActionItems(ctx, "checkout", "")
	if err != nil || len(open) != 1 || open[0].Item.ID != "alert-lag" || open[0].IncidentID != declared.ID {
		t.Fatal(open, err)
	}
	if all, _ := s.OpenActionItems(ctx, "", ""); len(all) != 1 {
		t.Fatal("the done standing item is still open", all)
	}
	// The incident lists its postmortem, with what is still open.
	listed, err := s.IncidentPostmortems(ctx, declared.ID)
	if err != nil || len(listed) != 1 || listed[0].ReviewID != review.ID || listed[0].Published || listed[0].OpenActions != 1 {
		t.Fatal(listed, err)
	}
	// Invalid content is refused.
	for name, bad := range map[string]func(*v1.Postmortem){
		"cause kind":  func(p *v1.Postmortem) { p.ContributingCauses[0].Kind = "root" },
		"unknown SLO": func(p *v1.Postmortem) { p.ActionItems[1].SLOs = []string{"missing"} },
		"due date":    func(p *v1.Postmortem) { p.ActionItems[1].Due = "next week" },
		"item kind":   func(p *v1.Postmortem) { p.ActionItems[1].Kind = "blame" },
	} {
		bad(&p)
		if _, _, err = s.SavePostmortem(ctx, review.ID, v1.SavePostmortem{ID: "bad", ExpectedVersion: 3, Postmortem: p}); err == nil {
			t.Fatal(name, "accepted")
		}
		tmpl, _ = s.PostmortemTemplate(ctx, review.ID)
		p = tmpl.Postmortem
	}
	// Agents may help in messages, but people write postmortems.
	agent := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Role: "agent", Subject: "agent-1", Kind: "agent"})
	if _, _, err = s.SavePostmortem(agent, review.ID, v1.SavePostmortem{ID: "pm-agent", ExpectedVersion: 3, Postmortem: p}); err == nil {
		t.Fatal("an agent wrote a postmortem")
	}
}
