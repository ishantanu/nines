package auditarchive

import (
	"context"
	"errors"
	"log/slog"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
)

var ErrEmpty = errors.New("no audit archive work")
var ErrLease = errors.New("audit archive lease lost")
var ErrUnavailable = errors.New("archive destination unavailable")
var ErrNotLocked = errors.New("archive bucket Object Lock not enabled")
var ErrIntegrity = errors.New("archive object content mismatch")
var ErrRetention = errors.New("archive retention not verified")

type Job struct {
	Record      v1.AuditRecord
	Token       string
	RetainUntil time.Time
	Attempts    int
}
type Repository interface {
	ClaimArchive(context.Context, string, string, int, string) (Job, error)
	FinishArchive(context.Context, string, string, string, string, *v1.AuditArchiveReceipt, string) error
	ArchiveStatus(context.Context, string, string, string) (v1.AuditArchiveStatus, error)
}
type Sink interface {
	Store(context.Context, Job) (v1.AuditArchiveReceipt, error)
}
type Worker struct {
	Tenant     string
	Config     Config
	Repository Repository
	Sink       Sink
}

func ErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrNotLocked):
		return "bucket_not_locked"
	case errors.Is(err, ErrIntegrity):
		return "object_integrity_failed"
	case errors.Is(err, ErrRetention):
		return "retention_not_verified"
	default:
		return "destination_unavailable"
	}
}
func (w *Worker) WorkOne(ctx context.Context) (bool, error) {
	job, err := w.Repository.ClaimArchive(ctx, w.Tenant, w.Config.Target(), w.Config.RetentionDays, model.NewID())
	if errors.Is(err, ErrEmpty) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	work, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	work, finish := observability.Operation(work, "intent.audit_archive", w.Tenant)
	var receipt v1.AuditArchiveReceipt
	var deliveryErr error
	if job.Record.Tenant != w.Tenant {
		deliveryErr = ErrIntegrity
	} else {
		receipt, deliveryErr = w.Sink.Store(work, job)
	}
	save, done := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer done()
	var verified *v1.AuditArchiveReceipt
	code := ""
	if deliveryErr == nil {
		verified = &receipt
	} else {
		code = ErrorCode(deliveryErr)
	}
	err = w.Repository.FinishArchive(save, w.Tenant, w.Config.Target(), job.Record.ID, job.Token, verified, code)
	result := errors.Join(deliveryErr, err)
	finish(result)
	return true, result
}
func (w *Worker) Run(ctx context.Context, log *slog.Logger) {
	timer := time.NewTicker(5 * time.Second)
	defer timer.Stop()
	for {
		for i := 0; i < 25 && ctx.Err() == nil; i++ {
			found, err := w.WorkOne(ctx)
			if err != nil && ctx.Err() == nil {
				log.Warn("Audit archive delivery not verified", "code", ErrorCode(err))
			}
			if !found {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}
