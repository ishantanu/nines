package observability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type PGTracer struct{}
type pgKey struct{}
type pgQuery struct {
	span      trace.Span
	start     time.Time
	operation string
}

func (PGTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	operation := "OTHER"
	fields := strings.Fields(data.SQL)
	if len(fields) > 0 {
		switch strings.ToUpper(fields[0]) {
		case "SELECT", "INSERT", "UPDATE", "DELETE", "BEGIN", "COMMIT", "ROLLBACK", "CREATE", "ALTER", "DROP", "SET":
			operation = strings.ToUpper(fields[0])
		}
	}
	ctx, span := otel.Tracer(scope).Start(ctx, "postgresql "+operation, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attribute.String("db.system.name", "postgresql"), attribute.String("db.system", "postgresql"), attribute.String("peer.service", "nines-postgresql"), attribute.String("db.operation.name", operation)))
	return context.WithValue(ctx, pgKey{}, pgQuery{span: span, start: time.Now(), operation: operation})
}
func (PGTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	q, ok := ctx.Value(pgKey{}).(pgQuery)
	if !ok {
		return
	}
	defer q.span.End()
	outcome := "succeeded"
	if data.Err != nil {
		outcome = "failed"
		q.span.SetStatus(codes.Error, "database operation failed")
		q.span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%T", data.Err)))
	}
	histogram, _ := otel.Meter(scope).Float64Histogram("db.client.operation.duration", metric.WithUnit("s"))
	histogram.Record(ctx, time.Since(q.start).Seconds(), metric.WithAttributes(attribute.String("db.system.name", "postgresql"), attribute.String("db.operation.name", q.operation), attribute.String("outcome", outcome)))
}
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.Tracer = PGTracer{}
	return pgxpool.NewWithConfig(ctx, cfg)
}

func (t PGTracer) TraceBatchStart(ctx context.Context, conn *pgx.Conn, _ pgx.TraceBatchStartData) context.Context {
	ctx = t.TraceQueryStart(ctx, conn, pgx.TraceQueryStartData{SQL: "BATCH"})
	if q, ok := ctx.Value(pgKey{}).(pgQuery); ok {
		q.operation = "BATCH"
		q.span.SetName("postgresql BATCH")
		q.span.SetAttributes(attribute.String("db.operation.name", "BATCH"))
		return context.WithValue(ctx, pgKey{}, q)
	}
	return ctx
}
func (PGTracer) TraceBatchQuery(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchQueryData) {
	if data.Err != nil {
		if q, ok := ctx.Value(pgKey{}).(pgQuery); ok {
			q.span.SetStatus(codes.Error, "database batch operation failed")
		}
	}
}
func (t PGTracer) TraceBatchEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceBatchEndData) {
	t.TraceQueryEnd(ctx, conn, pgx.TraceQueryEndData{Err: data.Err})
}
