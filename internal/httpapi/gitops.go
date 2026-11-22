package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/gitops"
	"github.com/ishantanu/nines/internal/model"
)

// GitDelivery is the optional Git delivery adapter configured for a tenant.
type GitDelivery = gitops.Provider

var gitLimitations = []string{
	"Nines opens a pull request. Merging it does not apply the SLO in Nines; your pipeline must deploy the definition.",
	"A proposal is reported as applied only when the live definition's digest equals the proposed digest.",
	"Status is read from the Git provider on request. Nines stores no copy of it.",
}

func (a *Server) gitops() (GitDelivery, bool) {
	g := a.Workspace.GitOps
	return g, g != nil
}

func (a *Server) gitopsInfo(w http.ResponseWriter, r *http.Request) {
	info := gitops.Info{}
	if g, ok := a.gitops(); ok {
		info = g.Info()
	}
	write(w, 200, v1.GitInfo{APIVersion: model.APIVersion, Tenant: a.Workspace.ID, Configured: info.Configured, Provider: info.Provider, Repository: info.Repository, BaseBranch: info.BaseBranch, Path: info.Path})
}

// gitopsPropose opens a pull request for a reviewed SLO definition. The server
// re-plans the definition and refuses unless it equals what the caller
// reviewed, so the repository receives exactly the reviewed bytes' meaning.
func (a *Server) gitopsPropose(w http.ResponseWriter, r *http.Request) {
	g, ok := a.gitops()
	if !ok {
		write(w, 404, map[string]string{"error": "Git delivery is not configured for this workspace"})
		return
	}
	d, err := definitionBody(w, r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	kind, id := gitops.KindSLO, ""
	switch {
	case d.SLO != nil:
		id = d.SLO.Metadata.Name
	case d.Journey != nil:
		kind, id = gitops.KindJourney, d.Journey.Metadata.Name
	default:
		a.fail(w, r, model.Invalid("Git proposals support SLO and journey definitions only"))
		return
	}
	value := r.Header.Get("If-Match")
	expected, err := strconv.ParseInt(strings.Trim(value, `"`), 10, 64)
	if err != nil || expected < 0 || value != `"`+strconv.FormatInt(expected, 10)+`"` {
		a.fail(w, r, model.Invalid("If-Match must contain the quoted expected revision, e.g. \"0\" to create"))
		return
	}
	plan, err := a.Service.PlanDefinition(r.Context(), d)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if plan.Action == "unchanged" {
		a.fail(w, r, model.Invalid("the definition is unchanged from the saved SLO; there is nothing to propose"))
		return
	}
	if plan.ExpectedRevision != expected || r.Header.Get("X-Nines-Plan-Digest") != plan.Digest {
		a.fail(w, r, model.ErrRevisionConflict)
		return
	}
	// Propose the definition as v1beta1. The pull request records the stored
	// form's digest, so drift and applied-state checks are unaffected.
	resolved, err := a.Service.ResolveJourney(r.Context(), d)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	manifest, err := a.Service.ExportYAML(r.Context(), resolved, "")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	receipt, err := g.Open(ctx, gitops.Proposal{Tenant: a.Workspace.ID, Kind: kind, ID: id, Action: plan.Action, ExpectedRevision: plan.ExpectedRevision, Digest: plan.Digest, Manifest: manifest, Warnings: plan.Warnings})
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, gitops.ErrConflict) {
			status = http.StatusConflict
		}
		write(w, status, map[string]string{"error": err.Error()})
		return
	}
	status := http.StatusOK
	if receipt.Created {
		status = http.StatusCreated
	}
	write(w, status, v1.GitProposalResult{Receipt: receipt, DefinitionDigest: plan.Digest, ExpectedRevision: plan.ExpectedRevision, Limitations: gitLimitations})
}

// liveDigest returns the digest of the saved definition for a kind and id.
func (a *Server) liveDigest(ctx context.Context, kind, id string) (string, bool, error) {
	if kind == gitops.KindJourney {
		if a.Service.Journeys == nil {
			return "", false, model.ErrUnavailable
		}
		rev, err := a.Service.Journeys.JourneyRevision(ctx, id, 0)
		if errors.Is(err, model.ErrJourneyNotFound) {
			return "", false, nil
		}
		return rev.Digest, err == nil, err
	}
	rec, err := a.Service.Store.Read(ctx, id)
	if errors.Is(err, model.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return model.DefinitionDigest(model.DefinitionFromSLO(rec.SLO)), true, nil
}

// repositoryDigest parses a repository file and digests it the way a plan does.
func repositoryDigest(kind, id string, content []byte) (string, bool) {
	d, err := model.ParseDefinition(content)
	if err != nil {
		return "", false
	}
	if kind == gitops.KindJourney {
		if d.Journey == nil || d.Journey.Metadata.Name != id {
			return "", false
		}
		return model.JourneyDigest(*d.Journey), true
	}
	if d.SLO == nil || d.SLO.Metadata.Name != id {
		return "", false
	}
	return model.DefinitionDigest(d.Manifest()), true
}

func (a *Server) gitopsProposals(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		g, ok := a.gitops()
		if !ok {
			write(w, 200, v1.GitProposalList{APIVersion: model.APIVersion, Tenant: a.Workspace.ID, Proposals: []v1.GitProposalStatus{}, Limitations: gitLimitations})
			return
		}
		id := r.PathValue("id")
		live, exists, err := a.liveDigest(r.Context(), kind, id)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		if !exists {
			a.fail(w, r, model.ErrNotFound)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		pulls, err := g.Proposals(ctx, a.Workspace.ID, kind, id)
		if err != nil {
			write(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		out := make([]v1.GitProposalStatus, 0, len(pulls))
		for _, p := range pulls {
			out = append(out, v1.GitProposalStatus{GitPullRequest: p, Applied: p.State == "merged" && p.DefinitionDigest != "" && p.DefinitionDigest == live})
		}
		write(w, 200, v1.GitProposalList{APIVersion: model.APIVersion, Tenant: a.Workspace.ID, Configured: true, LiveDigest: live, Proposals: out, Limitations: gitLimitations})
	}
}

// gitopsDrift compares the file on the base branch with the live definition by
// digest. Phase 1 reads one file; it never writes and never decides which side
// should win.
func (a *Server) gitopsDrift(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		g, ok := a.gitops()
		if !ok {
			write(w, 404, map[string]string{"error": "Git delivery is not configured for this workspace"})
			return
		}
		id := r.PathValue("id")
		out := v1.GitDrift{APIVersion: model.APIVersion, Tenant: a.Workspace.ID, Limitations: []string{
			"Compares the file on the base branch with the saved definition by digest. A difference is reported, not resolved: either side may be the intended one.",
			"Only the definition is compared. Deployed rules and observed SLI are separate evidence.",
			"The repository is read on request from the base branch only; pull requests that are not merged are not considered.",
		}}
		live, exists, err := a.liveDigest(r.Context(), kind, id)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		out.LiveDigest = live
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		content, path, found, err := g.File(ctx, kind, id)
		out.Path = path
		switch {
		case err != nil && !found:
			write(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		case !found:
			out.State = "not_in_repo"
			if !exists {
				a.fail(w, r, model.ErrNotFound)
				return
			}
		case err != nil:
			out.State = "unreadable"
		default:
			digest, parsed := repositoryDigest(kind, id, content)
			switch {
			case !parsed:
				out.State = "unreadable"
			case !exists:
				out.RepositoryDigest, out.State = digest, "not_applied"
			case digest == live:
				out.RepositoryDigest, out.State = digest, "in_sync"
			default:
				out.RepositoryDigest, out.State = digest, "drifted"
			}
		}
		write(w, 200, out)
	}
}
