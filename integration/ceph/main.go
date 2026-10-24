// Command ceph runs synthetic Object Lock checks against the disposable container fixture.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/auditarchive"
	"github.com/ishantanu/nines/internal/model"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	endpoint := os.Getenv("NINES_CEPH_ENDPOINT")
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() || os.Getenv("NINES_CEPH_CONTAINER") != "1" || os.Getenv("SSL_CERT_FILE") == "" {
		return errors.New("requires the explicit disposable Ceph container fixture and its loopback TLS endpoint/CA")
	}
	access, secret := os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY")
	if len(access) < 24 || len(secret) < 40 {
		return errors.New("fixture credentials are missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	client := s3.New(s3.Options{Region: "us-east-1", BaseEndpoint: aws.String(endpoint), UsePathStyle: true, Credentials: credentials.NewStaticCredentialsProvider(access, secret, ""), HTTPClient: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}})
	bucket := "nines-audit-" + model.NewID()
	control := "nines-control-" + model.NewID()
	if _, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket), ObjectLockEnabledForBucket: aws.Bool(true)}); err != nil {
		return fmt.Errorf("create locked fixture bucket: %w", err)
	}
	if _, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(control)}); err != nil {
		return fmt.Errorf("create control fixture bucket: %w", err)
	}
	cfg := auditarchive.Config{Provider: "s3-compatible", Endpoint: endpoint, Bucket: bucket, Region: "us-east-1", Prefix: "nines/audit", RetentionDays: 1}
	sink, err := auditarchive.NewS3(ctx, cfg)
	if err != nil {
		return err
	}
	event := v1.IntentAuditEvent{Sequence: 1, At: time.Now().UTC(), Action: "acceptance.synthetic", Outcome: "succeeded", Reason: "Synthetic local Ceph container acceptance; no user configuration."}
	event.Hash = model.DefinitionDigest(event)
	job := auditarchive.Job{Record: v1.AuditRecord{APIVersion: v1.APIVersion, ID: model.NewID(), Tenant: "ceph-acceptance", Conversation: model.NewID(), Event: event}, RetainUntil: time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)}
	first, err := sink.Store(ctx, job)
	if err != nil {
		return fmt.Errorf("encrypted archive write/readback: %w", err)
	}
	second, err := sink.Store(ctx, job)
	if err != nil || second.VersionID != first.VersionID || second.Key != first.Key || second.SHA256 != first.SHA256 {
		return fmt.Errorf("conditional retry did not recover identical version: %w", err)
	}
	versions, err := client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: aws.String(bucket), Prefix: aws.String(first.Key)})
	if err != nil || len(versions.Versions) != 1 || aws.ToString(versions.Versions[0].VersionId) != first.VersionID {
		return fmt.Errorf("retry created extra versions or version listing failed: %w", err)
	}
	object, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(first.Key), VersionId: aws.String(first.VersionID)})
	if err != nil {
		return fmt.Errorf("independent version read: %w", err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(object.Body, 3<<20))
	_ = object.Body.Close()
	expected, _ := json.Marshal(job.Record)
	if readErr != nil || !bytes.Equal(raw, expected) || object.ServerSideEncryption != types.ServerSideEncryptionAes256 {
		return errors.New("independent encrypted version readback mismatch")
	}
	// A successful control deletion demonstrates that retention denials below are
	// enforced by Object Lock, rather than credentials lacking delete permission.
	_, err = client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(control), Key: aws.String("control"), Body: bytes.NewReader([]byte("synthetic")), ServerSideEncryption: types.ServerSideEncryptionAes256})
	if err != nil {
		return fmt.Errorf("encrypted control write: %w", err)
	}
	if _, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(control), Key: aws.String("control")}); err != nil {
		return fmt.Errorf("control delete permission: %w", err)
	}
	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(first.Key), VersionId: aws.String(first.VersionID)})
	if !denied(err) {
		return fmt.Errorf("COMPLIANCE version deletion was not denied: %w", err)
	}
	shorter := job.RetainUntil.Add(-time.Hour)
	_, err = client.PutObjectRetention(ctx, &s3.PutObjectRetentionInput{Bucket: aws.String(bucket), Key: aws.String(first.Key), VersionId: aws.String(first.VersionID), Retention: &types.ObjectLockRetention{Mode: types.ObjectLockRetentionModeCompliance, RetainUntilDate: &shorter}})
	if !denied(err) {
		return fmt.Errorf("COMPLIANCE retention shortening was not denied: %w", err)
	}
	cfg.Bucket = control
	unlocked, err := auditarchive.NewS3(ctx, cfg)
	if err != nil {
		return err
	}
	if _, err = unlocked.Store(ctx, job); !errors.Is(err, auditarchive.ErrNotLocked) {
		return fmt.Errorf("unlocked bucket was not classified as bucket_not_locked: %w", err)
	}
	retention, err := client.GetObjectRetention(ctx, &s3.GetObjectRetentionInput{Bucket: aws.String(bucket), Key: aws.String(first.Key), VersionId: aws.String(first.VersionID)})
	if err != nil || retention.Retention == nil || retention.Retention.Mode != types.ObjectLockRetentionModeCompliance || retention.Retention.RetainUntilDate == nil || retention.Retention.RetainUntilDate.Before(job.RetainUntil) {
		return errors.New("retention changed after denied mutations")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "passed", "provider": "ceph-rgw", "tls": true, "encryption": "AES256", "retention": "COMPLIANCE", "retentionDays": 1, "retryVersions": 1, "versionDeleteDenied": true, "shortenRetentionDenied": true, "controlDeletePassed": true, "unlockedBucketRejected": true})
}
func denied(err error) bool {
	var api smithy.APIError
	return errors.As(err, &api) && api.ErrorCode() == "AccessDenied"
}
