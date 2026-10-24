// Package intentflow implements bounded, durable intent preparation. It cannot apply resources.
package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/auditarchive"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

var ErrNotFound = errors.New("intent conversation not found")
var ErrConflict = errors.New("intent conversation version or request conflicts")
var ErrLimit = errors.New("intent conversation capacity reached")
var ErrForbidden = errors.New("intent workflow access denied")
var errLease = errors.New("intent workflow lease lost")

const maxDocument = 2 << 20
const maxSessions = 200

type record struct {
	MessageDigests map[string]string       `json:"messageDigests,omitempty"`
	Session        v1.IntentConversation   `json:"session"`
	RequestDigest  string                  `json:"requestDigest"`
	Token          string                  `json:"token,omitempty"`
	LeaseUntil     time.Time               `json:"leaseUntil"`
	Native         []rm.Message            `json:"native,omitempty"`
	Pending        *rm.Call                `json:"pending,omitempty"`
	RunRequest     *v1.CreateIntentFlowRun `json:"runRequest,omitempty"`
	InputDigest    string                  `json:"inputDigest,omitempty"`
}
type Repository interface {
	auditarchive.Repository
	Audit(context.Context, AuditSearch) ([]v1.AuditRecord, error)
	Create(context.Context, record) (record, error)
	Read(context.Context, string) (record, error)
	List(context.Context, string, int) ([]v1.IntentConversationSummary, error)
	Update(context.Context, string, func(*record, time.Time) error) (record, error)
	Claim(context.Context, string) (record, error)
	Close()
}

func clone[T any](v T) T { b, _ := json.Marshal(v); var out T; _ = json.Unmarshal(b, &out); return out }
func active(r record) bool {
	n := len(r.Session.Runs)
	return n > 0 && (r.Session.Runs[n-1].Status == "queued" || r.Session.Runs[n-1].Status == "running")
}
func current(r *record) *v1.IntentFlowRun { return &r.Session.Runs[len(r.Session.Runs)-1] }
func validSize(r record) error {
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	if len(b) > maxDocument {
		return ErrLimit
	}
	return nil
}
func claim(r *record, token string, now time.Time) error {
	if !active(*r) || r.LeaseUntil.After(now) {
		return ErrConflict
	}
	run := current(r)
	if !run.DeadlineAt.After(now) || run.Attempts >= 18 {
		finish(r, "failed", "execution_limit", "The preparation run reached its execution limit. Review the saved evidence before starting another run.", now)
		return nil
	}
	r.Token = token
	r.LeaseUntil = now.Add(90 * time.Second)
	run.Attempts++
	run.Status = "running"
	run.UpdatedAt = now
	r.Session.State = "running"
	touch(r, now)
	return nil
}
func touch(r *record, now time.Time) { r.Session.Version++; r.Session.UpdatedAt = now }

type memory struct {
	mu       sync.Mutex
	values   map[string]record
	journal  []v1.AuditRecord
	archives map[string]archiveState
}

func NewMemory() Repository { return &memory{values: map[string]record{}} }
func (m *memory) Close()    {}
func (m *memory) Create(ctx context.Context, r record) (record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		return record{}, ctx.Err()
	}
	if old, ok := m.values[r.Session.ID]; ok {
		if old.RequestDigest != r.RequestDigest {
			return record{}, ErrConflict
		}
		return clone(old), nil
	}
	if len(m.values) >= maxSessions {
		return record{}, ErrLimit
	}
	if err := validSize(r); err != nil {
		return record{}, err
	}
	m.values[r.Session.ID] = clone(r)
	return clone(r), nil
}
func (m *memory) Read(ctx context.Context, id string) (record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		return record{}, ctx.Err()
	}
	r, ok := m.values[id]
	if !ok {
		return record{}, ErrNotFound
	}
	return clone(r), nil
}
func (m *memory) List(ctx context.Context, after string, limit int) ([]v1.IntentConversationSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	out := []v1.IntentConversationSummary{}
	for id, r := range m.values {
		if id > after {
			out = append(out, clone(r.Session.IntentConversationSummary))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (m *memory) Update(ctx context.Context, id string, fn func(*record, time.Time) error) (record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		return record{}, ctx.Err()
	}
	old, ok := m.values[id]
	if !ok {
		return record{}, ErrNotFound
	}
	var head *v1.AuditRecord
	sequence, _ := lastAudit(old.Session.Audit)
	for i := len(m.journal) - 1; i >= 0 && sequence > 0; i-- {
		item := &m.journal[i]
		if item.Tenant == old.Session.Tenant && item.Conversation == id && item.Event.Sequence == sequence {
			head = item
			break
		}
	}
	if err := validJournalHead(old, head); err != nil {
		return record{}, err
	}
	r := clone(old)
	if err := fn(&r, time.Now().UTC()); err != nil {
		return record{}, err
	}
	if err := validSize(r); err != nil {
		return record{}, err
	}
	if ctx.Err() != nil {
		return record{}, ctx.Err()
	}
	added, err := journalAppend(old, r)
	if err != nil {
		return record{}, err
	}
	m.journal = append(m.journal, clone(added)...)
	m.values[id] = clone(r)
	return clone(r), nil
}
func (m *memory) Audit(ctx context.Context, query AuditSearch) ([]v1.AuditRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	out := []v1.AuditRecord{}
	for _, event := range m.journal {
		if auditMatches(event, query) {
			item := clone(event)
			if query.ID == "" {
				item.Proposal = nil
			}
			out = append(out, item)
		}
	}
	sortAudit(out, query.Ascending)
	if len(out) > query.Query.Limit {
		out = out[:query.Query.Limit]
	}
	return out, nil
}
func (m *memory) Claim(ctx context.Context, token string) (record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		return record{}, ctx.Err()
	}
	now := time.Now().UTC()
	ids := []string{}
	for id, r := range m.values {
		if active(r) && !r.LeaseUntil.After(now) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return record{}, ErrNotFound
	}
	r := clone(m.values[ids[0]])
	if err := claim(&r, token, now); err != nil {
		return record{}, err
	}
	m.values[ids[0]] = clone(r)
	return clone(r), nil
}
