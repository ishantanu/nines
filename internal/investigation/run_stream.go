package investigation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
)

// SSE carries committed public snapshots only. Provider transcripts and partial
// tool arguments are private. Reconnect resumes from the durable run version.
func (s *Service) streamRun(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		fail(w, model.Invalid("query fields are not supported"))
		return
	}
	last := int64(0)
	if value := r.Header.Get("Last-Event-ID"); value != "" {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil || n < 0 {
			fail(w, model.Invalid("invalid Last-Event-ID"))
			return
		}
		last = n
	}
	run, err := s.ReadRun(r.Context(), r.PathValue("id"), r.PathValue("run"))
	if err != nil {
		fail(w, err)
		return
	}
	if last > run.Version {
		fail(w, model.Invalid("Last-Event-ID is ahead of the run"))
		return
	}
	principal, ok := tenancy.FromContext(r.Context())
	if !ok || s.AuthorizeWatch == nil {
		fail(w, ErrForbidden)
		return
	}
	actor := v1.InvestigationActor{Subject: principal.Subject, Kind: principal.Kind, Name: principal.Name}
	if err = s.AuthorizeWatch(r.Context(), actor); err != nil {
		fail(w, ErrForbidden)
		return
	}
	select {
	case s.watchSlots <- struct{}{}:
		defer func() { <-s.watchSlots }()
	default:
		http.Error(w, "too many active review streams", http.StatusTooManyRequests)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	send := func(event string, value any, version int64) bool {
		_ = controller.SetWriteDeadline(time.Now().Add(3 * time.Second))
		data, _ := json.Marshal(value)
		if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", version, event, data); err != nil {
			return false
		}
		return controller.Flush() == nil
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if run.Version > last {
			if !send("run", run, run.Version) {
				return
			}
			last = run.Version
		}
		if terminal(run.Status) {
			send("done", map[string]string{"reason": "terminal"}, last)
			return
		}
		select {
		case <-ctx.Done():
			if r.Context().Err() == nil {
				send("done", map[string]string{"reason": "reconnect"}, last)
			}
			return
		case <-ticker.C:
		}
		if err = s.AuthorizeWatch(ctx, actor); err != nil {
			send("access_revoked", map[string]string{"reason": "access_changed"}, last)
			return
		}
		run, err = s.ReadRun(ctx, run.InvestigationID, run.ID)
		if err != nil {
			send("unavailable", map[string]string{"reason": "read_failed"}, last)
			return
		}
		if run.Version == last {
			_ = controller.SetWriteDeadline(time.Now().Add(3 * time.Second))
			if _, err = fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if controller.Flush() != nil {
				return
			}
		}
	}
}
