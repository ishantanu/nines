package auditarchive

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// This opt-in check creates one synthetic compliance-retained object. It never
// runs from ambient AWS credentials alone and never modifies bucket policy.
func TestRealS3ObjectLockReadbackAndIdempotency(t *testing.T) {
	path := os.Getenv("TEST_AUDIT_S3_CONFIG")
	if path == "" || os.Getenv("TEST_AUDIT_ARCHIVE_CONFIRM") != "COMPLIANCE" {
		t.Skip("set TEST_AUDIT_S3_CONFIG and TEST_AUDIT_ARCHIVE_CONFIRM=COMPLIANCE for explicit archive acceptance")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err = decoder.Decode(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.RetentionDays != 1 {
		t.Fatal("acceptance requires retentionDays: 1 and an isolated bucket/prefix")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	sink, err := NewS3(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	job := Job{Record: v1.AuditRecord{APIVersion: v1.APIVersion, ID: model.NewID(), Tenant: "archive-acceptance", Conversation: model.NewID(), Event: v1.IntentAuditEvent{Action: "acceptance.synthetic", Outcome: "succeeded", Reason: "Synthetic archive acceptance; contains no user configuration."}}, RetainUntil: time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)}
	job.Record.Event.Sequence = 1
	job.Record.Event.At = time.Now().UTC()
	job.Record.Event.Hash = model.DefinitionDigest(job.Record.Event)
	first, err := sink.Store(ctx, job)
	if err != nil {
		t.Fatal("archive readback failed:", ErrorCode(err))
	}
	second, err := sink.Store(ctx, job)
	if err != nil || second.VersionID != first.VersionID || second.SHA256 != first.SHA256 || second.Key != first.Key {
		t.Fatal("retry did not recover the identical object version", err)
	}
	t.Log("Synthetic object read back with COMPLIANCE retention; exact retry reused its version. The object remains retained for one day.")
}
