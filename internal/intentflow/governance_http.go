package intentflow

import (
	"context"
	"net/http"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func mountGovernance(m *http.ServeMux, s *Service) {
	m.HandleFunc("GET /api/intent-conversations/{id}/audit", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			fail(w, model.Invalid("audit query fields are not supported"))
			return
		}
		out, e := s.Audit(r.Context(), r.PathValue("id"))
		reply(w, out, e, 200)
	})
	reviews := map[string]func(context.Context, string, v1.ReviewIntentChangeSet) (v1.IntentConversation, error){
		"reject": s.RejectChangeSet, "rollback": s.PrepareRollback, "rollback/reject": s.RejectRollback,
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
