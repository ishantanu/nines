package rules

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

// Retire removes only owned configuration at the frozen, trusted destination.
// A successful DELETE/unlink is insufficient: the evaluator must confirm absence.
func (m *Manager) Retire(ctx context.Context, archived model.SLO) (bool, error) {
	state := archived.Retirement
	if state == nil {
		return false, model.Invalid("SLO is not archived")
	}
	original := archived
	original.Revision = state.OriginalRevision
	original.Retirement = nil
	target, ok := m.TargetFor(original)
	if !ok || RouteDigest(target) != state.RouteDigest {
		return false, model.Invalid("retirement destination changed; restore the original trusted connection before retrying cleanup")
	}
	if target.Kind == "operator" {
		plan, e := m.PlanContext(ctx, original)
		if e != nil {
			return false, e
		}
		if plan.PriorUID != "" && plan.PriorUID != state.ResourceUID {
			return false, model.Invalid("operator publication UID changed; retirement will not remove a replacement resource")
		}
		result, e := m.Withdraw(ctx, original, plan.Digest)
		if e != nil {
			return false, e
		}
		// Foreground deletion proves the owned publication and finalizers completed.
		// Provider runtime still needs a separate absence check below.
		if result.State != "withdrawn" {
			return false, nil
		}
		target.Kind = target.ProviderKind
		if target.Kind == "prometheus-operator" {
			target.Kind = "prometheus"
		}
		stage := InspectEvaluation(ctx, original, plan.Group, target, time.Now().UTC())
		return stage.State == "not_loaded" && stage.Code == "runtime_group_missing", nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	plan, e := m.PlanContext(ctx, original)
	if e != nil {
		return false, e
	}
	if plan.Group.Name != state.Group || RouteDigest(plan.Target) != state.RouteDigest {
		return false, model.ErrRevisionConflict
	}
	if e = m.authorizeWrite(ctx, original, plan.Digest); e != nil {
		return false, e
	}
	switch target.Kind {
	case "prometheus":
		file := filepath.Join(target.Directory, target.Namespace+"-"+plan.Group.Name+".yaml")
		existing, e := readRuleFile(file)
		if e == nil {
			if !Owned(existing.Groups[0], m.Tenant, target.Connection, original.ID) || !ownedUID(existing.Groups[0], m.OwnerUID) {
				return false, model.Invalid("refusing to remove foreign rules")
			}
			if e = os.Remove(file); e != nil {
				return false, errors.New("owned rule file could not be removed")
			}
			directory, e := os.Open(target.Directory)
			if e != nil {
				return false, e
			}
			syncErr := directory.Sync()
			_ = directory.Close()
			if syncErr != nil {
				return false, syncErr
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return false, model.Invalid("rule file cannot be safely inspected")
		}
		if target.Reload {
			code, e := reloadPrometheus(ctx, target)
			if e != nil || code != http.StatusOK {
				return false, model.ErrUnavailable
			}
		}
	case "mimir":
		path := "/config/v1/rules/" + url.PathEscape(target.Namespace) + "/" + url.PathEscape(plan.Group.Name)
		code, body, e := rulerRequest(ctx, target, http.MethodGet, path, nil)
		if e != nil {
			return false, model.ErrUnavailable
		}
		if code == http.StatusOK {
			group, e := decodeRuleGroup(body)
			if e != nil || !Owned(group, m.Tenant, target.Connection, original.ID) || !ownedUID(group, m.OwnerUID) {
				return false, model.Invalid("refusing to remove foreign rules")
			}
			code, _, e = rulerRequest(ctx, target, http.MethodDelete, path, nil)
			if e != nil || code != http.StatusAccepted && code != http.StatusOK && code != http.StatusNoContent && code != http.StatusNotFound {
				return false, model.ErrUnavailable
			}
		} else if code != http.StatusNotFound {
			return false, model.ErrUnavailable
		}
	default:
		return false, model.Invalid("retirement destination is unsupported")
	}
	stage := InspectEvaluation(ctx, original, plan.Group, target, time.Now().UTC())
	return stage.State == "not_loaded" && stage.Code == "runtime_group_missing", nil
}
