package service

import (
	"context"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

func (s *Service) SLORevisions(ctx context.Context, id string, before int64, limit int) ([]v1.SLORevision, error) {
	if limit < 1 || limit > 100 || before < 0 {
		return nil, model.Invalid("use limit 1–100 and a nonnegative beforeRevision")
	}
	if _, err := s.Store.Read(ctx, id); err != nil {
		return nil, err
	}
	history, ok := s.Store.(store.SLORevisionStore)
	if !ok {
		return nil, model.ErrUnavailable
	}
	return history.SLORevisions(ctx, id, before, limit)
}
func (s *Service) SLORevision(ctx context.Context, id string, revision int64) (store.SLOArchive, error) {
	if revision < 1 {
		return store.SLOArchive{}, model.Invalid("revision must be positive")
	}
	history, ok := s.Store.(store.SLORevisionStore)
	if !ok {
		return store.SLOArchive{}, model.ErrUnavailable
	}
	return history.SLORevision(ctx, id, revision)
}
func (s *Service) HistoryForRevision(ctx context.Context, id string, revision int64, from, to time.Time, resolution int64, policyName string, span ...int64) (engine.History, error) {
	archive, err := s.SLORevision(ctx, id, revision)
	if err != nil {
		return engine.History{}, err
	}
	return s.historyFromRecord(ctx, archive.Record, archive.Summary.ArchivedAt, from, to, resolution, policyName, span...)
}
