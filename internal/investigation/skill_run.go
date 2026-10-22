package investigation

import (
	"encoding/json"
	"slices"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
	"github.com/ishantanu/nines/reviewskills"
)

func skillInput(r v1.InvestigationRun) any {
	if r.Mode == "model" {
		return map[string]string{"question": r.Question}
	}
	return map[string]string{}
}
func skillInputDigest(r v1.InvestigationRun) string {
	b, _ := json.Marshal(skillInput(r))
	return reviewskills.Digest(b)
}
func checkSkill(r v1.InvestigationRun) error {
	if r.Skill == nil {
		return nil
	} // Pre-skill durable runs retain their exact execution contract.
	s, err := reviewskills.Get(*r.Skill)
	if err != nil {
		return rm.Failure("skill_unavailable")
	}
	if s.Spec.Mode != runMode(r.Mode) || !slices.Contains(s.Spec.ScopeKinds, r.Scope.Kind) || r.SkillInputDigest != skillInputDigest(r) {
		return rm.Failure("skill_input_invalid")
	}
	if reviewskills.Validate(*r.Skill, false, skillInput(r)) != nil {
		return rm.Failure("skill_input_invalid")
	}
	return nil
}
func modelPolicyFor(r v1.InvestigationRun) string {
	if r.Skill == nil {
		return modelPolicy()
	}
	return reviewskills.Digest([]byte(modelPolicy() + ":" + r.Skill.Digest))
}
func skillTools(r v1.InvestigationRun) ([]rm.Tool, string, error) {
	if r.Skill == nil {
		return reviewTools, reviewInstruction, nil
	}
	skill, err := reviewskills.Get(*r.Skill)
	if err != nil {
		return nil, "", rm.Failure("skill_unavailable")
	}
	tools := []rm.Tool{}
	for _, t := range reviewTools {
		if slices.Contains(skill.Spec.ToolAllowlist, t.Name) {
			if t.Name == "finish_review" {
				t.Schema, _ = json.Marshal(skill.Spec.OutputSchema)
			}
			tools = append(tools, t)
		}
	}
	return tools, reviewInstruction + "\nServer-owned skill instructions:\n" + skill.Spec.Instructions, nil
}
func skillAllows(r v1.InvestigationRun, tool string) bool {
	if r.Skill == nil {
		return true
	}
	s, err := reviewskills.Get(*r.Skill)
	return err == nil && slices.Contains(s.Spec.ToolAllowlist, tool)
}
func completedToolNames(r v1.InvestigationRun) []string {
	out := []string{}
	for _, step := range r.Steps {
		if step.Status == "completed" && !slices.Contains(out, step.Tool) {
			out = append(out, step.Tool)
		}
	}
	return out
}
func skillEvidence(r v1.InvestigationRun) error {
	if len(r.Steps) < 2 || r.Steps[0].Evidence == nil || r.Steps[1].Assessment == nil {
		return rm.Failure("skill_evidence_missing")
	}
	e := r.Steps[0].Evidence
	if reviewskills.Digest(e.Context) != e.Digest || r.Steps[1].Assessment.EvidenceDigest != e.Digest {
		return rm.Failure("skill_evidence_missing")
	}
	return nil
}
func validateSkillOutput(r v1.InvestigationRun, value any) error {
	if r.Skill == nil {
		return nil
	}
	if err := skillEvidence(r); err != nil {
		return err
	}
	skill, err := reviewskills.Get(*r.Skill)
	if err != nil {
		return rm.Failure("skill_unavailable")
	}
	completed := completedToolNames(r)
	for _, tool := range skill.Spec.RequiredTools {
		if !slices.Contains(completed, tool) {
			return rm.Failure("skill_evidence_missing")
		}
	}
	if reviewskills.Validate(*r.Skill, true, value) != nil {
		return rm.Failure("skill_output_invalid")
	}
	return nil
}
func completeSkill(r *runRecord) error {
	if r.Run.Skill == nil {
		return nil
	}
	var output any = r.Run.Steps[1].Assessment
	if r.Run.Mode == "model" {
		output = r.Run.ModelReview
	}
	if err := validateSkillOutput(r.Run, output); err != nil {
		return err
	}
	b, err := json.Marshal(output)
	if err != nil {
		return rm.Failure("skill_output_invalid")
	}
	r.Run.SkillResult = &v1.ReviewSkillResult{InputDigest: r.Run.SkillInputDigest, EvidenceDigests: []string{r.Run.Steps[0].Evidence.Digest}, CompletedTools: completedToolNames(r.Run), OutputDigest: reviewskills.Digest(b)}
	return nil
}
