package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"
)

func consoleHandler(out io.Writer, res *resource.Resource) slog.Handler {
	h := slog.NewJSONHandler(out, &slog.HandlerOptions{ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) == 0 {
			switch a.Key {
			case slog.TimeKey:
				a.Key = "timestamp"
			case slog.LevelKey:
				a.Key = "severity_text"
			case slog.MessageKey:
				a.Key = "body"
			}
		}
		return a
	}})
	return h.WithAttrs([]slog.Attr{slog.GroupAttrs("resource", serviceAttributes(res)...)})
}

type logHandler struct{ handlers []slog.Handler }

func (h *logHandler) Enabled(ctx context.Context, l slog.Level) bool {
	for _, v := range h.handlers {
		if v.Enabled(ctx, l) {
			return true
		}
	}
	return false
}
func safeAttr(a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	for _, secret := range []string{"authorization", "cookie", "token", "password", "secret", "dsn", "query", "statement", "body", "url", "requestid"} {
		if strings.Contains(key, secret) {
			return slog.Attr{}
		}
	}
	if a.Key == "error" {
		return slog.String("error.type", fmt.Sprintf("%T", a.Value.Any()))
	}
	if a.Value.Kind() == slog.KindGroup {
		attrs := []slog.Attr{}
		for _, v := range a.Value.Group() {
			if cleaned := safeAttr(v); !cleaned.Equal(slog.Attr{}) {
				attrs = append(attrs, cleaned)
			}
		}
		return slog.GroupAttrs(a.Key, attrs...)
	}
	return a
}
func (h *logHandler) Handle(ctx context.Context, record slog.Record) error {
	r := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(a slog.Attr) bool {
		if a = safeAttr(a); !a.Equal(slog.Attr{}) {
			r.AddAttrs(a)
		}
		return true
	})
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			if err := handler.Handle(ctx, r.Clone()); err != nil {
				return err
			}
		}
	}
	return nil
}
func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cleaned := []slog.Attr{}
	for _, a := range attrs {
		if a = safeAttr(a); !a.Equal(slog.Attr{}) {
			cleaned = append(cleaned, a)
		}
	}
	next := &logHandler{}
	for _, v := range h.handlers {
		next.handlers = append(next.handlers, v.WithAttrs(cleaned))
	}
	return next
}
func (h *logHandler) WithGroup(name string) slog.Handler {
	next := &logHandler{}
	for _, v := range h.handlers {
		next.handlers = append(next.handlers, v.WithGroup(name))
	}
	return next
}
