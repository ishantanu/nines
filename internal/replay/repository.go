package replay

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

var ErrCorrupt = errors.New("session replay integrity check failed")
var ErrNotFound = errors.New("session replay not found")
var ErrConflict = errors.New("session replay version or request conflicts")
var ErrLimit = errors.New("tenant replay capacity reached; recordings expire after seven days")

const MaxSessions = 50
const RetentionDays = 7

type record struct {
	Meta          v1.SessionReplay  `json:"meta"`
	Events        []json.RawMessage `json:"events"`
	RequestDigest string            `json:"requestDigest"`
}
type Repository interface {
	Create(context.Context, record) (record, error)
	Read(context.Context, string) (record, error)
	List(context.Context) ([]v1.SessionReplay, error)
	Update(context.Context, string, func(*record) error) (record, error)
	Prune(context.Context) error
	Close()
}

func clone(r record) record {
	b, _ := json.Marshal(r)
	var out record
	_ = json.Unmarshal(b, &out)
	return out
}

type memory struct {
	mu     sync.Mutex
	values map[string]record
}

func NewMemory() Repository { return &memory{values: map[string]record{}} }
func (m *memory) Close()    {}
func (m *memory) prune() {
	for id, r := range m.values {
		if !r.Meta.ExpiresAt.After(time.Now()) {
			delete(m.values, id)
		}
	}
}
func (m *memory) Prune(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prune()
	return ctx.Err()
}
func (m *memory) Create(ctx context.Context, r record) (record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		return r, ctx.Err()
	}
	m.prune()
	if old, ok := m.values[r.Meta.ID]; ok {
		if old.RequestDigest != r.RequestDigest {
			return r, ErrConflict
		}
		return clone(old), nil
	}
	if len(m.values) >= MaxSessions {
		return r, ErrLimit
	}
	m.values[r.Meta.ID] = clone(r)
	return clone(r), nil
}
func (m *memory) Read(ctx context.Context, id string) (record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		return record{}, ctx.Err()
	}
	r, ok := m.values[id]
	if !ok || !r.Meta.ExpiresAt.After(time.Now()) {
		return record{}, ErrNotFound
	}
	return clone(r), nil
}
func (m *memory) List(ctx context.Context) ([]v1.SessionReplay, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	out := []v1.SessionReplay{}
	for _, r := range m.values {
		if r.Meta.ExpiresAt.After(time.Now()) {
			out = append(out, clone(r).Meta)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (m *memory) Update(ctx context.Context, id string, fn func(*record) error) (record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		return record{}, ctx.Err()
	}
	r, ok := m.values[id]
	if !ok || !r.Meta.ExpiresAt.After(time.Now()) {
		return r, ErrNotFound
	}
	r = clone(r)
	if err := fn(&r); err != nil {
		return r, err
	}
	m.values[id] = clone(r)
	return clone(r), nil
}
