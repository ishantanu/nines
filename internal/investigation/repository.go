// Package investigation stores tenant-scoped conversations and measured evidence.
// It does not execute models, approve changes or mutate reliability definitions.
package investigation

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

var ErrNotFound = errors.New("investigation not found")
var ErrConflict = errors.New("investigation version or request conflicts with stored state")

type Repository interface {
	RunRepository
	Create(context.Context, v1.Investigation) (v1.Investigation, error)
	Read(context.Context, string) (v1.Investigation, error)
	List(context.Context, string, int) ([]v1.InvestigationSummary, error)
	Update(context.Context, string, func(*v1.Investigation) error) (v1.Investigation, error)
	Close()
}

func clone(v v1.Investigation) v1.Investigation {
	b, _ := json.Marshal(v)
	var out v1.Investigation
	_ = json.Unmarshal(b, &out)
	return out
}
func sameCreate(a, b v1.Investigation) bool {
	return a.Tenant == b.Tenant && a.Title == b.Title && a.Scope.Kind == b.Scope.Kind && a.Scope.ResourceID == b.Scope.ResourceID && a.Scope.ExpectedRevision == b.Scope.ExpectedRevision && a.Scope.At.Equal(b.Scope.At) && a.CreatedBy.Subject == b.CreatedBy.Subject
}

type memory struct {
	mu     sync.Mutex
	values map[string]v1.Investigation
	runs   map[[2]string]runRecord
}

func NewMemory() Repository {
	return &memory{values: map[string]v1.Investigation{}, runs: map[[2]string]runRecord{}}
}
func (m *memory) Close() {}
func (m *memory) Create(ctx context.Context, v v1.Investigation) (v1.Investigation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return v, err
	}
	if old, ok := m.values[v.ID]; ok {
		if !sameCreate(old, v) {
			return v, ErrConflict
		}
		return clone(old), nil
	}
	m.values[v.ID] = clone(v)
	return clone(v), nil
}
func (m *memory) Read(ctx context.Context, id string) (v1.Investigation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return v1.Investigation{}, err
	}
	v, ok := m.values[id]
	if !ok {
		return v, ErrNotFound
	}
	return clone(v), nil
}
func (m *memory) List(ctx context.Context, after string, limit int) ([]v1.InvestigationSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := []v1.InvestigationSummary{}
	for _, v := range m.values {
		if v.ID > after {
			out = append(out, v.InvestigationSummary)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (m *memory) Update(ctx context.Context, id string, fn func(*v1.Investigation) error) (v1.Investigation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return v1.Investigation{}, err
	}
	old, ok := m.values[id]
	if !ok {
		return old, ErrNotFound
	}
	v := clone(old)
	if err := fn(&v); err != nil {
		return v1.Investigation{}, err
	}
	m.values[id] = clone(v)
	return clone(v), nil
}
