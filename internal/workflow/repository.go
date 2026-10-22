package workflow

import (
	"context"
	"sort"
	"sync"
)

// Update serializes decisions for one workflow across all replicas. Callbacks
// must not call this repository recursively. Platform CAS application uses a
// separate store transaction; the durable approved state permits crash repair.
type Repository interface {
	Create(context.Context, Workflow) (Workflow, error)
	Get(context.Context, string) (Workflow, error)
	List(context.Context) ([]Workflow, error)
	Recoverable(context.Context) ([]Workflow, error)
	Update(context.Context, string, func(*Workflow) error) (Workflow, error)
	Close()
}
type memory struct {
	mu     sync.Mutex
	values map[string]Workflow
}

func NewMemory() Repository { return &memory{values: map[string]Workflow{}} }
func (m *memory) Close()    {}
func (m *memory) Create(ctx context.Context, w Workflow) (Workflow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Workflow{}, err
	}
	if old, ok := m.values[w.ID]; ok {
		if old.RequestDigest != w.RequestDigest {
			return Workflow{}, ErrConflict
		}
		return clone(old), nil
	}
	m.values[w.ID] = clone(w)
	return clone(w), nil
}
func (m *memory) Get(ctx context.Context, id string) (Workflow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Workflow{}, err
	}
	w, ok := m.values[id]
	if !ok {
		return w, ErrNotFound
	}
	return clone(w), nil
}
func (m *memory) List(ctx context.Context) ([]Workflow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]Workflow, 0, len(m.values))
	for _, w := range m.values {
		out = append(out, clone(w))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > 200 {
		out = out[:200]
	}
	return out, nil
}
func (m *memory) Recoverable(ctx context.Context) ([]Workflow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := []Workflow{}
	for _, w := range m.values {
		if w.State == "approved" {
			out = append(out, clone(w))
			if len(out) == 200 {
				break
			}
		}
	}
	return out, nil
}
func (m *memory) Update(ctx context.Context, id string, fn func(*Workflow) error) (Workflow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Workflow{}, err
	}
	old, ok := m.values[id]
	if !ok {
		return Workflow{}, ErrNotFound
	}
	w := clone(old)
	if err := fn(&w); err != nil {
		return clone(old), err
	}
	if err := ctx.Err(); err != nil {
		return clone(old), err
	}
	m.values[id] = clone(w)
	return clone(w), nil
}
