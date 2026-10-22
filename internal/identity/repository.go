// Package identity authenticates people and service accounts. Membership is
// resolved on every request; authentication alone grants no tenant access.
package identity

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"
)

var (
	ErrNotFound  = errors.New("identity record not found")
	ErrConflict  = errors.New("identity revision conflict")
	ErrInvalid   = errors.New("invalid identity request")
	ErrForbidden = errors.New("identity operation denied")
)

const maxRows = 1000

type Tx interface {
	Get(kind, scope, id string) (json.RawMessage, error)
	List(kind, scope string) ([]json.RawMessage, error)
	Page(kind, scope, before string, limit int) ([]json.RawMessage, error)
	Put(kind, scope, id string, data json.RawMessage, expires time.Time) error
	Delete(kind, scope, id string) error
	Ensure(kind, scope, id string, data json.RawMessage) error
}
type Repository interface {
	View(context.Context, func(Tx) error) error
	Update(context.Context, string, func(Tx) error) error
	Prune(context.Context) error
	Close()
}

func read[T any](tx Tx, kind, scope, id string) (T, error) {
	var value T
	b, err := tx.Get(kind, scope, id)
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(b, &value)
	return value, err
}
func list[T any](tx Tx, kind, scope string) ([]T, error) {
	rows, err := tx.List(kind, scope)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(rows))
	for _, b := range rows {
		var v T
		if err = json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
func put(tx Tx, kind, scope, id string, value any, expires time.Time) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return tx.Put(kind, scope, id, b, expires)
}

type entry struct {
	data    json.RawMessage
	expires time.Time
}
type Memory struct {
	mu   sync.RWMutex
	rows map[[3]string]entry
}

func NewMemory() *Memory { return &Memory{rows: map[[3]string]entry{}} }
func (m *Memory) Close() {}
func (m *Memory) View(ctx context.Context, f func(Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return f(&memoryTx{rows: m.rows})
}
func (m *Memory) Update(ctx context.Context, _ string, f func(Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rows := make(map[[3]string]entry, len(m.rows))
	for k, v := range m.rows {
		rows[k] = v
	}
	if err := f(&memoryTx{rows: rows, writable: true}); err != nil {
		return err
	}
	m.rows = rows
	return nil
}
func (m *Memory) Prune(ctx context.Context) error {
	return m.Update(ctx, "prune", func(tx Tx) error {
		t := tx.(*memoryTx)
		for k, v := range t.rows {
			if !v.expires.IsZero() && !v.expires.After(time.Now()) {
				delete(t.rows, k)
			}
		}
		return nil
	})
}

type memoryTx struct {
	rows     map[[3]string]entry
	writable bool
}

func (t *memoryTx) Get(kind, scope, id string) (json.RawMessage, error) {
	v, ok := t.rows[[3]string{kind, scope, id}]
	if !ok || !v.expires.IsZero() && !v.expires.After(time.Now()) {
		return nil, ErrNotFound
	}
	return append(json.RawMessage(nil), v.data...), nil
}
func (t *memoryTx) List(kind, scope string) ([]json.RawMessage, error) {
	out := []json.RawMessage{}
	for k, v := range t.rows {
		if k[0] == kind && k[1] == scope && (v.expires.IsZero() || v.expires.After(time.Now())) {
			out = append(out, append(json.RawMessage(nil), v.data...))
			if len(out) > maxRows {
				return nil, errors.New("identity scope exceeds 1000 records")
			}
		}
	}
	return out, nil
}
func (t *memoryTx) Put(kind, scope, id string, b json.RawMessage, expires time.Time) error {
	if !t.writable {
		return ErrForbidden
	}
	t.rows[[3]string{kind, scope, id}] = entry{append(json.RawMessage(nil), b...), expires}
	return nil
}
func (t *memoryTx) Delete(kind, scope, id string) error {
	if !t.writable {
		return ErrForbidden
	}
	delete(t.rows, [3]string{kind, scope, id})
	return nil
}

func (t *memoryTx) Page(kind, scope, before string, limit int) ([]json.RawMessage, error) {
	keys := []string{}
	for k, v := range t.rows {
		if k[0] == kind && k[1] == scope && (before == "" || k[2] < before) && (v.expires.IsZero() || v.expires.After(time.Now())) {
			keys = append(keys, k[2])
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))
	if len(keys) > limit {
		keys = keys[:limit]
	}
	out := []json.RawMessage{}
	for _, id := range keys {
		out = append(out, append(json.RawMessage(nil), t.rows[[3]string{kind, scope, id}].data...))
	}
	return out, nil
}

func (t *memoryTx) Ensure(kind, scope, id string, b json.RawMessage) error {
	if !t.writable {
		return ErrForbidden
	}
	if _, ok := t.rows[[3]string{kind, scope, id}]; !ok {
		return t.Put(kind, scope, id, b, time.Time{})
	}
	return nil
}
