package investigation

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

// Private lease tokens are never returned by HTTP or SDK responses.
type runRecord struct {
	Model modelState
	Run   v1.InvestigationRun
	Token string
	Until time.Time
	Now   time.Time
}
type RunRepository interface {
	CreateRun(context.Context, v1.InvestigationRun) (v1.InvestigationRun, error)
	ReadRun(context.Context, string, string) (v1.InvestigationRun, error)
	ListRuns(context.Context, string) ([]v1.InvestigationRun, error)
	claimRun(context.Context, string) (runRecord, error)
	updateRun(context.Context, string, string, func(*runRecord) error) (v1.InvestigationRun, error)
}

const leaseDuration = 30 * time.Second

func terminal(status string) bool {
	return status == "completed" || status == "cancelled" || status == "failed"
}
func copyRun(v v1.InvestigationRun) v1.InvestigationRun {
	b, _ := json.Marshal(v)
	var out v1.InvestigationRun
	_ = json.Unmarshal(b, &out)
	return out
}
func sameRun(a, b v1.InvestigationRun) bool {
	return a.Tenant == b.Tenant && a.InvestigationID == b.InvestigationID && a.ReviewVersion == b.ReviewVersion && a.RequestedBy.Subject == b.RequestedBy.Subject && a.RequestedBy.Kind == b.RequestedBy.Kind && a.Scope == b.Scope && runMode(a.Mode) == runMode(b.Mode) && a.Question == b.Question && sameSkillRequest(a, b)
}
func finish(r *runRecord, status, code string) {
	r.Run.Status = status
	r.Run.ErrorCode = code
	r.Run.FinishedAt = &r.Now
	r.Run.UpdatedAt = r.Now
	r.Run.Version++
	r.Token = ""
	r.Until = r.Now
}
func acquire(r *runRecord, token string) {
	if !r.Run.DeadlineAt.After(r.Now) {
		finish(r, "failed", "deadline_exceeded")
		return
	}
	limit := 6
	if r.Run.Mode == "model" {
		limit = 24
	}
	if r.Run.Attempts >= limit {
		finish(r, "failed", "attempt_limit")
		return
	}
	r.Token = token
	duration := leaseDuration
	if r.Run.Mode == "model" {
		duration = 90 * time.Second
	}
	r.Until = r.Now.Add(duration)
	r.Run.Attempts++
	r.Run.Status = "running"
	r.Run.UpdatedAt = r.Now
	r.Run.Version++
	if r.Run.StartedAt == nil {
		at := r.Now
		r.Run.StartedAt = &at
	}
}
func (m *memory) CreateRun(ctx context.Context, v v1.InvestigationRun) (v1.InvestigationRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return v, err
	}
	key := [2]string{v.InvestigationID, v.ID}
	if old, ok := m.runs[key]; ok {
		if !sameRun(old.Run, v) {
			return v, ErrConflict
		}
		return copyRun(old.Run), nil
	}
	parent, ok := m.values[v.InvestigationID]
	if !ok || parent.Tenant != v.Tenant {
		return v, ErrNotFound
	}
	if parent.Version != v.ReviewVersion || parent.Scope != v.Scope {
		return v, ErrConflict
	}
	count := 0
	for _, r := range m.runs {
		if r.Run.InvestigationID == v.InvestigationID {
			count++
			if !terminal(r.Run.Status) {
				return v, ErrConflict
			}
		}
	}
	if count >= 20 {
		return v, model.Invalid("review run limit reached; create a new review")
	}
	m.runs[key] = runRecord{Run: copyRun(v)}
	return copyRun(v), nil
}
func (m *memory) ReadRun(ctx context.Context, review, id string) (v1.InvestigationRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return v1.InvestigationRun{}, err
	}
	r, ok := m.runs[[2]string{review, id}]
	if !ok {
		return r.Run, ErrNotFound
	}
	return copyRun(r.Run), nil
}
func (m *memory) ListRuns(ctx context.Context, review string) ([]v1.InvestigationRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := []v1.InvestigationRun{}
	for _, r := range m.runs {
		if r.Run.InvestigationID == review {
			out = append(out, copyRun(r.Run))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt) || out[i].CreatedAt.Equal(out[j].CreatedAt) && out[i].ID < out[j].ID
	})
	return out, nil
}
func (m *memory) updateRun(ctx context.Context, review, id string, fn func(*runRecord) error) (v1.InvestigationRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return v1.InvestigationRun{}, err
	}
	key := [2]string{review, id}
	r, ok := m.runs[key]
	if !ok {
		return r.Run, ErrNotFound
	}
	r = copyRunRecord(r)
	r.Now = time.Now().UTC()
	if err := fn(&r); err != nil {
		return v1.InvestigationRun{}, err
	}
	if err := validRunRecord(r); err != nil {
		return v1.InvestigationRun{}, err
	}
	m.runs[key] = r
	return copyRun(r.Run), nil
}
func (m *memory) claimRun(ctx context.Context, token string) (runRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return runRecord{}, err
	}
	now := time.Now().UTC()
	var key [2]string
	var found bool
	for k, r := range m.runs {
		if !terminal(r.Run.Status) && !r.Until.After(now) && (!found || r.Run.CreatedAt.Before(m.runs[key].Run.CreatedAt)) {
			key = k
			found = true
		}
	}
	if !found {
		return runRecord{}, ErrNotFound
	}
	r := m.runs[key]
	r = copyRunRecord(r)
	r.Now = now
	acquire(&r, token)
	m.runs[key] = r
	return r, nil
}

type modelState struct {
	Messages []rm.Message    `json:"messages,omitempty"`
	Calls    map[int]rm.Call `json:"calls,omitempty"`
}

func copyRunRecord(r runRecord) runRecord {
	r.Run = copyRun(r.Run)
	b, _ := json.Marshal(r.Model)
	var private modelState
	_ = json.Unmarshal(b, &private)
	r.Model = private
	return r
}
func validRunRecord(r runRecord) error {
	public, err := json.Marshal(r.Run)
	if err != nil {
		return err
	}
	private, err := json.Marshal(r.Model)
	if err != nil {
		return err
	}
	if len(public) > 128<<10 || len(private) > 256<<10 {
		return rm.Failure("model_budget_exceeded")
	}
	return nil
}
func runMode(mode string) string {
	if mode == "" {
		return "deterministic"
	}
	return mode
}

func sameSkillRequest(a, b v1.InvestigationRun) bool {
	if !b.SkillExplicit {
		return !a.SkillExplicit
	}
	return a.SkillExplicit && a.Skill != nil && *a.Skill == *b.Skill
}
