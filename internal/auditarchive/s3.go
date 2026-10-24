package auditarchive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

type s3API interface {
	GetObjectLockConfiguration(context.Context, *s3.GetObjectLockConfigurationInput, ...func(*s3.Options)) (*s3.GetObjectLockConfigurationOutput, error)
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	GetObjectRetention(context.Context, *s3.GetObjectRetentionInput, ...func(*s3.Options)) (*s3.GetObjectRetentionOutput, error)
}
type S3 struct {
	config Config
	client s3API
}

func NewS3(ctx context.Context, c Config) (*S3, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(c.Region), config.WithRetryMaxAttempts(2), config.WithHTTPClient(&http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}))
	if err != nil {
		return nil, ErrUnavailable
	}
	// Archive destinations come exclusively from the owner configuration, not SDK endpoint environment variables.
	cfg.BaseEndpoint = nil
	cfg.ConfigSources = nil
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if c.Endpoint != "" {
			o.BaseEndpoint = aws.String(c.Endpoint)
			o.UsePathStyle = true
		}
	})
	return &S3{config: c, client: client}, nil
}

var objectPart = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func (s *S3) Store(ctx context.Context, job Job) (v1.AuditArchiveReceipt, error) {
	var receipt v1.AuditArchiveReceipt
	if !objectPart.MatchString(job.Record.Tenant) || !objectPart.MatchString(job.Record.ID) || !objectPart.MatchString(job.Record.Conversation) {
		return receipt, ErrIntegrity
	}
	unsigned := job.Record.Event
	unsigned.Hash = ""
	if job.Record.APIVersion != v1.APIVersion || job.Record.Event.Sequence < 1 || job.Record.Event.Hash != model.DefinitionDigest(unsigned) {
		return receipt, ErrIntegrity
	}
	if !job.RetainUntil.After(time.Now()) {
		return receipt, ErrRetention
	}
	payload, err := json.Marshal(job.Record)
	if err != nil || len(payload) > 3<<20 {
		return receipt, ErrIntegrity
	}
	digest := sha256.Sum256(payload)
	checksum := base64.StdEncoding.EncodeToString(digest[:])
	bucket := aws.String(s.config.Bucket)
	// AWS account ownership is verified only in s3 mode. Do not send an empty
	// owner header or imply that other gateways enforce this AWS-specific check.
	var owner *string
	if s.config.Provider != "s3-compatible" {
		owner = aws.String(s.config.ExpectedBucketOwner)
	}
	lock, err := s.client.GetObjectLockConfiguration(ctx, &s3.GetObjectLockConfigurationInput{Bucket: bucket, ExpectedBucketOwner: owner})
	if err != nil {
		var api smithy.APIError
		if errors.As(err, &api) && api.ErrorCode() == "ObjectLockConfigurationNotFoundError" {
			return receipt, ErrNotLocked
		}
		return receipt, ErrUnavailable
	}
	if lock == nil || lock.ObjectLockConfiguration == nil || lock.ObjectLockConfiguration.ObjectLockEnabled != types.ObjectLockEnabledEnabled {
		return receipt, ErrNotLocked
	}
	key := s.config.Prefix + "/" + job.Record.Tenant + "/" + s.config.Target() + "/" + job.Record.Conversation + "/" + job.Record.ID + "/" + strconv.FormatInt(job.RetainUntil.Unix(), 10) + ".json"
	input := &s3.PutObjectInput{Bucket: bucket, Key: aws.String(key), ExpectedBucketOwner: owner, Body: bytes.NewReader(payload), ContentLength: aws.Int64(int64(len(payload))), ContentType: aws.String("application/json"), ChecksumAlgorithm: types.ChecksumAlgorithmSha256, ChecksumSHA256: aws.String(checksum), IfNoneMatch: aws.String("*"), ObjectLockMode: types.ObjectLockModeCompliance, ObjectLockRetainUntilDate: &job.RetainUntil, ServerSideEncryption: types.ServerSideEncryptionAes256}
	if s.config.KMSKeyID != "" {
		input.ServerSideEncryption = types.ServerSideEncryptionAwsKms
		input.SSEKMSKeyId = aws.String(s.config.KMSKeyID)
	}
	_, err = s.client.PutObject(ctx, input)
	if err != nil {
		var api smithy.APIError
		if !errors.As(err, &api) || (api.ErrorCode() != "PreconditionFailed" && api.ErrorCode() != "ConditionalRequestConflict") {
			return receipt, ErrUnavailable
		}
	}
	// A successful PUT (or an earlier ambiguous PUT) is insufficient: read the exact version and bytes back.
	object, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: bucket, Key: aws.String(key), ExpectedBucketOwner: owner, ChecksumMode: types.ChecksumModeEnabled})
	if err != nil {
		return receipt, ErrUnavailable
	}
	if object == nil || object.Body == nil {
		return receipt, ErrIntegrity
	}
	defer func() { _ = object.Body.Close() }()
	h := sha256.New()
	count, err := io.Copy(h, io.LimitReader(object.Body, int64(len(payload))+1))
	version := aws.ToString(object.VersionId)
	if err != nil || count != int64(len(payload)) || !bytes.Equal(h.Sum(nil), digest[:]) || version == "" || version == "null" {
		return receipt, ErrIntegrity
	}
	retention, err := s.client.GetObjectRetention(ctx, &s3.GetObjectRetentionInput{Bucket: bucket, Key: aws.String(key), VersionId: object.VersionId, ExpectedBucketOwner: owner})
	if err != nil {
		return receipt, ErrUnavailable
	}
	if retention == nil || retention.Retention == nil || retention.Retention.Mode != types.ObjectLockRetentionModeCompliance || retention.Retention.RetainUntilDate == nil || retention.Retention.RetainUntilDate.Before(job.RetainUntil) {
		return receipt, ErrRetention
	}
	if object.ServerSideEncryption != input.ServerSideEncryption || s.config.KMSKeyID != "" && aws.ToString(object.SSEKMSKeyId) != s.config.KMSKeyID {
		return receipt, ErrIntegrity
	}
	return v1.AuditArchiveReceipt{Bucket: s.config.Bucket, Key: key, VersionID: version, SHA256: hex.EncodeToString(digest[:]), Mode: "COMPLIANCE", RetainUntil: *retention.Retention.RetainUntilDate, VerifiedAt: time.Now().UTC()}, nil
}
