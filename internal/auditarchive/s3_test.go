package auditarchive

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func testConfig() Config {
	return Config{Bucket: "nines-audit-test", Region: "us-east-1", ExpectedBucketOwner: "123456789012", Prefix: "nines/audit", RetentionDays: 30}
}
func testJob() Job {
	event := v1.IntentAuditEvent{Sequence: 1, At: time.Now().UTC(), Action: "test.synthetic", Outcome: "succeeded"}
	event.Hash = model.DefinitionDigest(event)
	return Job{Record: v1.AuditRecord{Event: event, APIVersion: v1.APIVersion, ID: "ea87ab60-03c5-4ad5-af0a-d814a493d38c", Tenant: "alpha", Conversation: "chat"}, RetainUntil: time.Now().UTC().Truncate(time.Second).Add(30 * 24 * time.Hour)}
}
func TestS3ObjectLockVerificationAndAmbiguousRetry(t *testing.T) {
	for _, provider := range []string{"s3", "s3-compatible"} {
		for _, scenario := range []string{"success", "precondition", "unlocked", "lock-not-configured", "governance", "short-retention", "wrong-content", "missing-version", "wrong-encryption", "redirect"} {
			t.Run(provider+"/"+scenario, func(t *testing.T) {
				t.Setenv("AWS_ACCESS_KEY_ID", "synthetic-access")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "synthetic-secret")
				t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
				t.Setenv("AWS_SESSION_TOKEN", "")
				t.Setenv("AWS_ENDPOINT_URL_S3", "https://ignored-endpoint.invalid")
				var body []byte
				puts := 0
				gets := 0
				job := testJob()
				endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					expectedOwner := "123456789012"
					if provider == "s3-compatible" {
						expectedOwner = ""
						if _, sent := r.Header["X-Amz-Expected-Bucket-Owner"]; sent {
							t.Error("non-AWS request sent an owner header")
						}
					}
					if r.Header.Get("X-Amz-Expected-Bucket-Owner") != expectedOwner || !strings.Contains(r.Header.Get("Authorization"), "Credential=synthetic-access/") {
						t.Error("missing owner binding or signature")
					}
					q := r.URL.Query()
					if q.Has("object-lock") {
						if scenario == "lock-not-configured" {
							w.WriteHeader(http.StatusNotFound)
							_, _ = fmt.Fprint(w, `<Error><Code>ObjectLockConfigurationNotFoundError</Code></Error>`)
							return
						}
						enabled := "Enabled"
						if scenario == "unlocked" {
							enabled = ""
						}
						_, _ = fmt.Fprintf(w, `<ObjectLockConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><ObjectLockEnabled>%s</ObjectLockEnabled></ObjectLockConfiguration>`, enabled)
						return
					}
					if r.Method == "PUT" {
						puts++
						body, _ = io.ReadAll(r.Body)
						sum := sha256.Sum256(body)
						if !strings.HasPrefix(r.URL.Path, "/nines-audit-test/nines/audit/alpha/") || r.Header.Get("X-Amz-Server-Side-Encryption") != "AES256" || r.Header.Get("If-None-Match") != "*" || r.Header.Get("X-Amz-Object-Lock-Mode") != "COMPLIANCE" || r.Header.Get("X-Amz-Checksum-Sha256") != base64.StdEncoding.EncodeToString(sum[:]) {
							t.Error("write lacked conditional integrity/retention constraints")
						}
						if scenario == "precondition" {
							w.WriteHeader(412)
							_, _ = fmt.Fprint(w, `<Error><Code>PreconditionFailed</Code></Error>`)
							return
						}
						w.Header().Set("X-Amz-Version-Id", "version-1")
						return
					}
					if q.Has("retention") {
						if q.Get("versionId") != "version-1" {
							t.Error("retention read was not pinned to object version")
						}
						mode := "COMPLIANCE"
						until := job.RetainUntil
						if scenario == "governance" {
							mode = "GOVERNANCE"
						}
						if scenario == "short-retention" {
							until = until.Add(-time.Hour)
						}
						_, _ = fmt.Fprintf(w, `<Retention xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Mode>%s</Mode><RetainUntilDate>%s</RetainUntilDate></Retention>`, mode, until.Format(time.RFC3339))
						return
					}
					gets++
					if scenario == "redirect" {
						http.Redirect(w, r, "https://example.invalid/", http.StatusFound)
						return
					}
					if scenario != "missing-version" {
						w.Header().Set("X-Amz-Version-Id", "version-1")
					}
					if scenario != "wrong-encryption" {
						w.Header().Set("X-Amz-Server-Side-Encryption", "AES256")
					}
					if scenario == "wrong-content" {
						_, _ = fmt.Fprint(w, "wrong")
						return
					}
					_, _ = w.Write(body)
				}))
				defer endpoint.Close()
				cfg := testConfig()
				cfg.Endpoint = endpoint.URL
				if provider == "s3-compatible" {
					cfg.Provider = "s3-compatible"
					cfg.ExpectedBucketOwner = ""
				}
				sink, err := NewS3(context.Background(), cfg)
				if err != nil {
					t.Fatal(err)
				}
				receipt, err := sink.Store(context.Background(), job)
				if scenario == "success" || scenario == "precondition" {
					if err != nil || receipt.VersionID != "version-1" || receipt.Mode != "COMPLIANCE" || len(receipt.SHA256) != 64 || receipt.RetainUntil.Before(job.RetainUntil) {
						t.Fatal(receipt, err)
					}
				} else if err == nil {
					t.Fatal("unverified object accepted", scenario)
				}
				if scenario == "unlocked" || scenario == "lock-not-configured" {
					if puts != 0 || !errors.Is(err, ErrNotLocked) {
						t.Fatal("unlocked bucket was written or reported as another failure", puts, err)
					}
				}
				if scenario == "success" && (puts != 1 || gets != 1) {
					t.Fatal(puts, gets)
				}
			})
		}
	}
}
func TestArchiveConfigRejectsUnsafeDestinations(t *testing.T) {
	base := testConfig()
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"http://example.com", "https://user:secret@example.com", "https://example.com/?token=secret", "https://example.com/path", "file:///tmp/objects"} {
		c := base
		c.Endpoint = endpoint
		if c.Validate() == nil {
			t.Fatal(endpoint)
		}
	}
	for _, change := range []func(*Config){func(c *Config) { c.RetentionDays = 0 }, func(c *Config) { c.RetentionDays = 3651 }, func(c *Config) { c.Prefix = "../other" }, func(c *Config) { c.ExpectedBucketOwner = "" }, func(c *Config) { c.Region = "../region" }} {
		c := base
		change(&c)
		if c.Validate() == nil {
			t.Fatal(c)
		}
	}
	changed := base
	changed.RetentionDays++
	if changed.Target() == base.Target() {
		t.Fatal("policy changes reused archive target")
	}
}

func TestCompatibleStorageConfigBoundaries(t *testing.T) {
	base := testConfig()
	base.Provider = "s3-compatible"
	base.ExpectedBucketOwner = ""
	base.Endpoint = "https://archive.example.com"
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Config){
		func(c *Config) { c.Endpoint = "" },
		func(c *Config) { c.ExpectedBucketOwner = "123456789012" },
		func(c *Config) { c.KMSKeyID = "arn:aws:kms:us-east-1:123456789012:key/test" },
		func(c *Config) { c.Provider = "unknown" },
		func(c *Config) { c.Provider = "minio" },
		func(c *Config) { c.Provider = "s3" },
		func(c *Config) { c.Endpoint = "http://archive.example.com" },
	} {
		cfg := base
		change(&cfg)
		if cfg.Validate() == nil {
			t.Fatal("accepted invalid provider configuration", cfg)
		}
	}
	changed := base
	changed.Provider = "s3"
	changed.ExpectedBucketOwner = "123456789012"
	if changed.Target() == base.Target() {
		t.Fatal("providers reused the same delivery policy")
	}
}
