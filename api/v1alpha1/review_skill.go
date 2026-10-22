package v1alpha1

// ReviewSkill is a server-owned, versioned execution contract. It cannot contain code or credentials.
type ReviewSkill struct {
	APIVersion string              `json:"apiVersion" yaml:"apiVersion"`
	Kind       string              `json:"kind" yaml:"kind"`
	Metadata   ReviewSkillMetadata `json:"metadata" yaml:"metadata"`
	Spec       ReviewSkillSpec     `json:"spec" yaml:"spec"`
	Digest     string              `json:"digest" yaml:"-"`
}
type ReviewSkillMetadata struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version" yaml:"version"`
}
type ReviewSkillSpec struct {
	DisplayName         string         `json:"displayName" yaml:"displayName"`
	Description         string         `json:"description" yaml:"description"`
	Mode                string         `json:"mode" yaml:"mode"`
	ScopeKinds          []string       `json:"scopeKinds" yaml:"scopeKinds"`
	ToolAllowlist       []string       `json:"toolAllowlist" yaml:"toolAllowlist"`
	RequiredTools       []string       `json:"requiredTools" yaml:"requiredTools"`
	EvidenceObligations []string       `json:"evidenceObligations" yaml:"evidenceObligations"`
	InputSchema         map[string]any `json:"inputSchema" yaml:"inputSchema"`
	OutputSchema        map[string]any `json:"outputSchema" yaml:"outputSchema"`
	Instructions        string         `json:"instructions" yaml:"instructions"`
}
type ReviewSkillRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}
type ReviewSkillResult struct {
	InputDigest     string   `json:"inputDigest"`
	EvidenceDigests []string `json:"evidenceDigests"`
	CompletedTools  []string `json:"completedTools"`
	OutputDigest    string   `json:"outputDigest"`
}
type ReviewSkillList struct {
	APIVersion string        `json:"apiVersion"`
	Tenant     string        `json:"tenant"`
	Items      []ReviewSkill `json:"items"`
}

type ReviewSkillDocument struct {
	APIVersion string      `json:"apiVersion"`
	Tenant     string      `json:"tenant"`
	Item       ReviewSkill `json:"item"`
}
