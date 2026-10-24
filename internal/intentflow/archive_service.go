package intentflow

import (
	"context"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
)

func (s *Service) ArchiveStatus(ctx context.Context, id string) (v1.AuditArchiveStatus, error) {
	out := v1.AuditArchiveStatus{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, State: "disabled", RecordID: id}
	p, ok := tenancy.FromContext(ctx)
	if !ok || p.TenantID != s.Platform.TenantID || p.Subject == "" || (p.Role != "owner" && p.Role != "editor") {
		return out, ErrForbidden
	}
	if id != "" && !auditRecordID.MatchString(id) {
		return out, model.Invalid("invalid audit record ID")
	}
	if s.Archive == nil {
		return out, nil
	}
	return s.Repository.ArchiveStatus(ctx, p.TenantID, s.Archive.Config.Target(), id)
}
