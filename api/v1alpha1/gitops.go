package v1alpha1

import "time"

// GitInfo describes a tenant's optional Git delivery. It never holds a
// credential.
type GitInfo struct {
	APIVersion string `json:"apiVersion"`
	Tenant     string `json:"tenant"`
	Configured bool   `json:"configured"`
	Provider   string `json:"provider,omitempty"`
	Repository string `json:"repository,omitempty"`
	BaseBranch string `json:"baseBranch,omitempty"`
	Path       string `json:"path,omitempty"`
}

// GitPullRequest is a pull or merge request Nines opened for a definition.
type GitPullRequest struct {
	Number           int       `json:"number"`
	URL              string    `json:"url"`
	Branch           string    `json:"branch"`
	State            string    `json:"state"` // open, merged or closed
	Title            string    `json:"title"`
	DefinitionDigest string    `json:"definitionDigest,omitempty"`
	ExpectedRevision int64     `json:"expectedRevision,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
}

// GitReceipt records where a proposal went. Created is false when an identical
// proposal already had a request.
type GitReceipt struct {
	APIVersion  string         `json:"apiVersion"`
	Created     bool           `json:"created"`
	Path        string         `json:"path"`
	PullRequest GitPullRequest `json:"pullRequest"`
}

// GitProposalResult is the response to opening a proposal. Proposing never
// applies the definition in Nines.
type GitProposalResult struct {
	Receipt          GitReceipt `json:"receipt"`
	DefinitionDigest string     `json:"definitionDigest"`
	ExpectedRevision int64      `json:"expectedRevision"`
	Limitations      []string   `json:"limitations"`
}

// GitProposalStatus adds whether the merged definition equals the live one.
// A merge alone never means applied.
type GitProposalStatus struct {
	GitPullRequest
	Applied bool `json:"applied"`
}

// GitProposalList lists recent proposals for one definition.
type GitProposalList struct {
	APIVersion  string              `json:"apiVersion"`
	Tenant      string              `json:"tenant"`
	Configured  bool                `json:"configured"`
	LiveDigest  string              `json:"liveDigest,omitempty"`
	Proposals   []GitProposalStatus `json:"proposals"`
	Limitations []string            `json:"limitations"`
}

// GitDrift compares the definition file on the base branch with the live
// definition. It reports a difference; it does not say which side is right.
type GitDrift struct {
	APIVersion       string   `json:"apiVersion"`
	Tenant           string   `json:"tenant"`
	State            string   `json:"state"` // in_sync, drifted, not_in_repo, not_applied or unreadable
	Path             string   `json:"path"`
	RepositoryDigest string   `json:"repositoryDigest,omitempty"`
	LiveDigest       string   `json:"liveDigest,omitempty"`
	Limitations      []string `json:"limitations"`
}
