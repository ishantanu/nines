package investigation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
	"github.com/ishantanu/nines/reviewskills"
)

func measurementSkill() v1.ReviewSkillRef {
	for _, s := range reviewskills.Catalog() {
		if s.Metadata.Name == "measurement-review" {
			return reviewskills.Ref(s)
		}
	}
	panic("missing fixture skill")
}
func skillFixture(t *testing.T, pg bool) (*Service, context.Context, string, v1.CreateInvestigationRun) {
	t.Helper()
	s, ctx, dsn := fixture(t, pg)
	s.AuthorizeRun = func(context.Context, v1.InvestigationActor) error { return nil }
	if _, err := s.Create(ctx, input()); err != nil {
		t.Fatal(err)
	}
	ref := measurementSkill()
	req := v1.CreateInvestigationRun{ID: "skill-run", ExpectedVersion: 1, Mode: "model", Question: "What prevents a reliable comparison?", Skill: &ref}
	return s, ctx, dsn, req
}
func skillModel() *fixtureModel {
	m := &fixtureModel{route: "skill-fixture"}
	m.turn = func(_ context.Context, in rm.Request, _ func(rm.Progress) error) (rm.Response, error) {
		for _, tool := range in.Tools {
			if tool.Name != "read_assessment" && tool.Name != "finish_review" {
				return rm.Response{}, errors.New("undeclared tool was exposed")
			}
		}
		if len(in.Messages) == 1 {
			return fixtureResponse("read", "read_assessment", map[string]any{}), nil
		}
		var intro struct {
			EvidenceDigest string `json:"evidenceDigest"`
		}
		_ = json.Unmarshal([]byte(in.Messages[0].Text), &intro)
		return fixtureResponse("finish", "finish_review", v1.ModelReview{Summary: "Captured evidence does not establish success.", EvidenceDigests: []string{intro.EvidenceDigest}, Limitations: []string{"No fresh provider query or causal diagnosis was performed."}}), nil
	}
	return m
}
func TestExecutableSkillPinsRecoveryAndReceipt(t *testing.T) {
	runModes(t, func(t *testing.T, pg bool) {
		s, ctx, dsn, in := skillFixture(t, pg)
		s.Model = skillModel()
		r, err := s.CreateRun(ctx, input().ID, in)
		if err != nil || r.Skill == nil || *r.Skill != *in.Skill || !r.SkillExplicit || len(r.SkillInputDigest) != 64 {
			t.Fatal(r, err)
		}
		if pg {
			var state string
			repo := s.Repository.(*postgres)
			if err := repo.pool.QueryRow(ctx, `SELECT state FROM reliability_review_runs WHERE id=$1`, in.ID).Scan(&state); err != nil || state != "skill_queued" {
				t.Fatal(state, err)
			}
		}
		runSteps(t, s, ctx, 4)
		if pg {
			s.Repository.Close()
			repo, err := NewPostgres(ctx, dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			s = New(s.Platform, repo)
			s.AuthorizeRun = func(context.Context, v1.InvestigationActor) error { return nil }
			s.Model = skillModel()
		}
		runSteps(t, s, ctx, 2)
		r, err = s.ReadRun(ctx, input().ID, in.ID)
		if err != nil || r.Status != "completed" || r.SkillResult == nil || r.SkillResult.InputDigest != r.SkillInputDigest || r.Steps[0].Attempts != 1 || r.Steps[3].Attempts != 1 || !slices.Contains(r.SkillResult.CompletedTools, "read_assessment") {
			t.Fatal(r, err)
		}
		b, _ := json.Marshal(r.ModelReview)
		if r.SkillResult.OutputDigest != reviewskills.Digest(b) || r.SkillResult.EvidenceDigests[0] != r.Steps[0].Evidence.Digest {
			t.Fatal("receipt does not bind exact output and evidence")
		}
		s.Model = nil
		again, err := s.CreateRun(ctx, input().ID, in)
		if err != nil || again.Version != r.Version {
			t.Fatal("idempotent retry changed after configuration removal", err)
		}
		changed := *in.Skill
		changed.Digest = strings.Repeat("f", 64)
		in.Skill = &changed
		if _, err = s.CreateRun(ctx, input().ID, in); !errors.Is(err, ErrConflict) {
			t.Fatal("changed skill retry accepted", err)
		}
	})
}
func TestSkillRejectsUndeclaredToolsAndMissingObligations(t *testing.T) {
	for _, tc := range []struct {
		name, tool, code string
		limits           []string
	}{{"undeclared", "read_objective", "unsafe_tool_call", nil}, {"finish_before_read", "finish_review", "skill_evidence_missing", []string{"limit"}}, {"required_limitation", "finish_review", "skill_output_invalid", []string{}}} {
		t.Run(tc.name, func(t *testing.T) {
			s, ctx, _, in := skillFixture(t, false)
			m := skillModel()
			m.turn = func(_ context.Context, request rm.Request, _ func(rm.Progress) error) (rm.Response, error) {
				var intro struct {
					EvidenceDigest string `json:"evidenceDigest"`
				}
				_ = json.Unmarshal([]byte(request.Messages[0].Text), &intro)
				if tc.name == "required_limitation" && len(request.Messages) == 1 {
					return fixtureResponse("read", "read_assessment", map[string]any{}), nil
				}
				if tc.tool == "read_objective" {
					return fixtureResponse("bad", tc.tool, map[string]string{"resourceId": "checkout"}), nil
				}
				return fixtureResponse("bad", tc.tool, v1.ModelReview{Summary: "Unsupported claim", EvidenceDigests: []string{intro.EvidenceDigest}, Limitations: tc.limits}), nil
			}
			s.Model = m
			if _, err := s.CreateRun(ctx, input().ID, in); err != nil {
				t.Fatal(err)
			}
			n := 3
			if tc.name == "required_limitation" {
				n = 5
			}
			runSteps(t, s, ctx, n)
			r, _ := s.ReadRun(ctx, input().ID, in.ID)
			if r.Status != "failed" || r.ErrorCode != tc.code || r.ModelReview != nil || r.SkillResult != nil {
				t.Fatal(r)
			}
		})
	}
}
func TestSkillDefaultRetryAndVersionMismatch(t *testing.T) {
	s, ctx, _ := runFixture(t, false)
	r, _ := s.ReadRun(ctx, input().ID, "run-1")
	if r.Skill == nil || r.SkillExplicit {
		t.Fatal("default skill not pinned")
	}
	retry := v1.CreateInvestigationRun{ID: r.ID, ExpectedVersion: 1}
	again, err := s.CreateRun(ctx, input().ID, retry)
	if err != nil || again.Version != r.Version {
		t.Fatal(err)
	}
	ref := *r.Skill
	retry.Skill = &ref
	if _, err = s.CreateRun(ctx, input().ID, retry); !errors.Is(err, ErrConflict) {
		t.Fatal("implicit/explicit request change accepted", err)
	}
	_, err = s.Repository.updateRun(ctx, input().ID, r.ID, func(current *runRecord) error { current.Run.Skill.Version = "999.0.0"; return nil })
	if err != nil {
		t.Fatal(err)
	}
	runSteps(t, s, ctx, 1)
	r, _ = s.ReadRun(ctx, input().ID, r.ID)
	if r.Status != "failed" || r.ErrorCode != "skill_unavailable" || r.Steps[0].Attempts != 0 {
		t.Fatal(r)
	}
}
func TestPreSkillRunRetainsLegacyModelPolicy(t *testing.T) {
	m := &fixtureModel{route: "legacy-fixture"}
	m.turn = func(_ context.Context, in rm.Request, _ func(rm.Progress) error) (rm.Response, error) {
		var intro struct {
			EvidenceDigest string `json:"evidenceDigest"`
		}
		_ = json.Unmarshal([]byte(in.Messages[0].Text), &intro)
		return fixtureResponse("final", "finish_review", v1.ModelReview{Summary: "Legacy review", EvidenceDigests: []string{intro.EvidenceDigest}, Limitations: []string{}}), nil
	}
	s, ctx, _ := modelFixture(t, false, m)
	_, err := s.Repository.updateRun(ctx, input().ID, "model-run", func(r *runRecord) error {
		r.Run.Skill = nil
		r.Run.SkillInputDigest = ""
		r.Run.ModelPolicy = modelPolicy()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	runSteps(t, s, ctx, 4)
	r, _ := s.ReadRun(ctx, input().ID, "model-run")
	if r.Status != "completed" || r.Skill != nil || r.SkillResult != nil {
		t.Fatal(r)
	}
}
func TestSkillAssessmentFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/review-skills.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name    string         `yaml:"name"`
		Context map[string]any `yaml:"context"`
		Want    string         `yaml:"want"`
	}
	if err = yaml.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			b, _ := json.Marshal(tc.Context)
			e := v1.InvestigationEvidence{Digest: reviewskills.Digest(b), Context: b}
			a, err := assess(v1.InvestigationScope{Kind: v1.SLOKind}, e)
			if err != nil || a.State != tc.Want {
				t.Fatal(a, err)
			}
			ref := reviewskills.Default("deterministic")
			r := runRecord{Run: v1.InvestigationRun{Skill: &ref, SkillInputDigest: reviewskills.Digest([]byte(`{}`)), Steps: []v1.InvestigationRunStep{{Tool: "read_context", Status: "completed", Evidence: &e}, {Tool: "assess_evidence", Status: "completed", Assessment: a}}}}
			if err = completeSkill(&r); err != nil || r.Run.SkillResult == nil {
				t.Fatal(err)
			}
		})
	}
}
