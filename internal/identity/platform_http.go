package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type platformMutation struct {
	Reason          string            `json:"reason"`
	Tenant          string            `json:"tenant,omitempty"`
	UserID          string            `json:"userId,omitempty"`
	Provider        string            `json:"provider,omitempty"`
	Subject         string            `json:"subject,omitempty"`
	Role            string            `json:"role,omitempty"`
	ExpectedVersion int64             `json:"expectedVersion"`
	User            *LocalUserInput   `json:"user,omitempty"`
	Policy          *PlatformAIPolicy `json:"policy,omitempty"`
}

func (s *Service) PlatformHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, found, err := s.Resolve(r)
		if err != nil {
			authFailure(w, err)
			return
		}
		if !found || !a.Session || a.Kind != "user" {
			authJSON(w, 401, map[string]string{"error": "Sign in with a platform administrator account"})
			return
		}
		err = s.repo.View(r.Context(), func(tx Tx) error { return s.platformAdminTx(tx, a) })
		if err != nil {
			authFailure(w, err)
			return
		}
		if !s.ValidOrigin(r) || len(r.Header.Values("X-Nines-Tenant")) != 0 {
			authFailure(w, ErrForbidden)
			return
		}
		if r.Method != http.MethodGet && (r.Method != http.MethodPost || !s.RequireCSRF(r, a)) {
			authFailure(w, ErrForbidden)
			return
		}
		a.platformOperation = true
		suffix := strings.TrimPrefix(r.URL.Path, "/api/platform/")
		if r.Method == http.MethodGet {
			if len(r.URL.Query()) > 1 || len(r.URL.Query().Get("before")) > 100 || len(r.URL.Query()["before"]) > 1 || (len(r.URL.Query()) == 1 && !r.URL.Query().Has("before")) {
				authFailure(w, ErrInvalid)
				return
			}
			before := r.URL.Query().Get("before")
			var out any
			switch {
			case suffix == "overview":
				tenants := []PlatformTenant{}
				for _, t := range s.platformTenants {
					tenants = append(tenants, t)
				}
				sort.Slice(tenants, func(i, j int) bool { return tenants[i].ID < tenants[j].ID })
				out = map[string]any{"tenants": tenants, "providers": s.Providers(), "administrators": s.config.PlatformAdmins, "administratorRolesManagedBy": "server configuration", "tenantProvisioning": "server configuration", "auditRetentionDays": 90}
			case suffix == "users":
				out, err = s.platformUsers(r.Context(), a, before)
			case suffix == "audit":
				events := []PlatformAudit{}
				err = s.repo.View(r.Context(), func(tx Tx) error {
					if e := s.platformAdminTx(tx, a); e != nil {
						return e
					}
					rows, e := tx.Page("platform_audit", "global", before, 101)
					if e != nil {
						return e
					}
					for _, raw := range rows {
						var item PlatformAudit
						if e = json.Unmarshal(raw, &item); e != nil {
							return e
						}
						events = append(events, item)
					}
					return nil
				})
				next := ""
				if len(events) > 100 {
					events = events[:100]
					next = events[99].Cursor
				}
				out = map[string]any{"items": events, "nextCursor": next}
			case strings.HasPrefix(suffix, "tenants/") && strings.HasSuffix(suffix, "/system-one"):
				tenant := strings.TrimSuffix(strings.TrimPrefix(suffix, "tenants/"), "/system-one")
				out, err = s.platformAI(r.Context(), a, tenant)
			default:
				authFailure(w, ErrNotFound)
				return
			}
			if err != nil {
				authFailure(w, err)
				return
			}
			authJSON(w, 200, out)
			return
		}
		var in platformMutation
		if err = decode(w, r, &in); err != nil {
			authFailure(w, err)
			return
		}
		if !utf8.ValidString(in.Reason) || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 1000 || strings.ContainsRune(in.Reason, 0) || !s.platformReasonSafe(in.Reason) {
			authFailure(w, ErrInvalid)
			return
		}
		a.platformReason = in.Reason
		var out any = map[string]bool{"ok": true}
		switch suffix {
		case "users/create":
			if in.User == nil {
				err = ErrInvalid
				break
			}
			out, err = s.CreateLocalUser(r.Context(), a, in.Tenant, *in.User)
		case "users/update":
			if in.User == nil {
				err = ErrInvalid
				break
			}
			out, err = s.UpdateLocalUser(r.Context(), a, in.Tenant, in.UserID, *in.User)
		case "users/revoke-sessions":
			err = s.revokePlatformSessions(r.Context(), a, in.UserID, in.ExpectedVersion)
		case "memberships/save":
			out, err = s.Grant(r.Context(), a, in.Tenant, in.Provider, in.Subject, in.Role, in.ExpectedVersion)
		case "memberships/remove":
			err = s.RemoveMember(r.Context(), a, in.Tenant, in.UserID, in.ExpectedVersion)
		case "system-one/save":
			if in.Policy == nil {
				err = ErrInvalid
				break
			}
			err = s.savePlatformAI(r.Context(), a, in.Tenant, *in.Policy)
		default:
			authFailure(w, ErrNotFound)
			return
		}
		if err != nil {
			cause := err
			auditCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer cancel()
			auditErr := s.repo.Update(auditCtx, "platform-audit", func(tx Tx) error {
				if e := s.platformAdminTx(tx, a); e != nil {
					return e
				}
				return s.recordPlatform(tx, a, suffix, in.Tenant+"/"+in.UserID, "failed", map[string]any{"conflict": errors.Is(cause, ErrConflict)})
			})
			if auditErr != nil {
				authJSON(w, 503, map[string]string{"error": "Platform audit unavailable"})
				return
			}
			authFailure(w, err)
			return
		}
		authJSON(w, 200, out)
	})
}
