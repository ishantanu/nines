package intentflow

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func mountGovernance(m *http.ServeMux, s *Service) {
	m.HandleFunc("GET /api/audit/archive", func(w http.ResponseWriter, r *http.Request) {
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			fail(w, model.Invalid("invalid archive status query"))
			return
		}
		for key, values := range q {
			if key != "record" || len(values) != 1 {
				fail(w, model.Invalid("provide only one record ID"))
				return
			}
		}
		out, err := s.ArchiveStatus(r.Context(), q.Get("record"))
		reply(w, out, err, 200)
	})
	m.HandleFunc("GET /api/audit", func(w http.ResponseWriter, r *http.Request) {
		q, err := parseAuditQuery(r.URL.RawQuery)
		if err != nil {
			fail(w, err)
			return
		}
		out, err := s.ListAudit(r.Context(), q)
		reply(w, out, err, 200)
	})
	m.HandleFunc("GET /api/audit/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			fail(w, model.Invalid("query fields are not supported"))
			return
		}
		out, err := s.AuditRecord(r.Context(), r.PathValue("id"))
		reply(w, out, err, 200)
	})
	m.HandleFunc("GET /api/intent-conversations/{id}/audit", func(w http.ResponseWriter, r *http.Request) {
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			fail(w, model.Invalid("invalid audit query"))
			return
		}
		for key, values := range q {
			if (key != "after" && key != "through") || len(values) != 1 {
				fail(w, model.Invalid("provide only after and through"))
				return
			}
		}
		var after, through int64
		for key, value := range map[string]*int64{"after": &after, "through": &through} {
			if q.Has(key) {
				*value, err = strconv.ParseInt(q.Get(key), 10, 64)
				if err != nil {
					fail(w, model.Invalid("invalid audit sequence"))
					return
				}
			}
		}
		out, e := s.AuditPage(r.Context(), r.PathValue("id"), after, through)
		reply(w, out, e, 200)
	})
	reviews := map[string]func(context.Context, string, v1.ReviewIntentChangeSet) (v1.IntentConversation, error){
		"observe": s.ObserveChangeSet, "reject": s.RejectChangeSet, "rollback": s.PrepareRollback, "rollback/reject": s.RejectRollback,
		"rollback/approve": func(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (v1.IntentConversation, error) {
			return s.ReviewRollback(ctx, id, in, false)
		},
		"rollback/apply": func(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (v1.IntentConversation, error) {
			return s.ReviewRollback(ctx, id, in, true)
		},
	}
	for path, review := range reviews {
		m.HandleFunc("POST /api/intent-conversations/{id}/change-set/"+path, func(w http.ResponseWriter, r *http.Request) {
			var in v1.ReviewIntentChangeSet
			if e := decode(w, r, &in); e != nil {
				action := map[string]string{"observe": "change_set.observed", "reject": "change_set.rejected", "rollback": "rollback.proposed", "rollback/reject": "rollback.rejected", "rollback/approve": "rollback.approved", "rollback/apply": "rollback.applied"}[path]
				s.auditFailure(r.Context(), r.PathValue("id"), action, "", e)
				fail(w, e)
				return
			}
			out, e := review(r.Context(), r.PathValue("id"), in)
			reply(w, out, e, 200)
		})
	}
	m.HandleFunc("GET /api/intent-conversations/{id}/change-set/rollback/kubernetes", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for key, values := range q {
			if (key != "namespace" && key != "digest") || len(values) != 1 {
				fail(w, model.Invalid("provide only namespace and digest"))
				return
			}
		}
		out, e := s.ExportRollbackKubernetes(r.Context(), r.PathValue("id"), q.Get("namespace"), q.Get("digest"))
		reply(w, out, e, 200)
	})
}

func parseAuditQuery(raw string) (v1.AuditQuery, error) {
	var out v1.AuditQuery
	q, err := url.ParseQuery(raw)
	if err != nil {
		return out, model.Invalid("invalid audit query")
	}
	fields := map[string]*string{"conversation": &out.Conversation, "actor": &out.Actor, "action": &out.Action, "outcome": &out.Outcome, "resource": &out.Resource, "cursor": &out.Cursor}
	for key, values := range q {
		if len(values) != 1 {
			return out, model.Invalid("duplicate audit query field")
		}
		if dst, ok := fields[key]; ok {
			*dst = values[0]
			continue
		}
		switch key {
		case "limit":
			out.Limit, err = strconv.Atoi(values[0])
			if out.Limit == 0 {
				err = model.Invalid("audit limit must be positive")
			}
		case "from":
			out.From, err = time.Parse(time.RFC3339Nano, values[0])
		case "to":
			out.To, err = time.Parse(time.RFC3339Nano, values[0])
		default:
			return out, model.Invalid("unsupported audit query field")
		}
		if err != nil {
			return out, model.Invalid("invalid audit date or limit")
		}
	}
	return out, nil
}
