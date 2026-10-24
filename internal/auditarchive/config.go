// Package auditarchive delivers tenant decision records to externally retained storage.
package auditarchive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"
)

type Config struct {
	// Provider defaults to s3. Other S3 endpoints require explicit configuration.
	Provider            string `json:"provider,omitempty" yaml:"provider,omitempty"`
	Bucket              string `json:"bucket" yaml:"bucket"`
	Region              string `json:"region" yaml:"region"`
	ExpectedBucketOwner string `json:"expectedBucketOwner" yaml:"expectedBucketOwner"`
	Prefix              string `json:"prefix" yaml:"prefix"`
	RetentionDays       int    `json:"retentionDays" yaml:"retentionDays"`
	Endpoint            string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	KMSKeyID            string `json:"kmsKeyID,omitempty" yaml:"kmsKeyID,omitempty"`
}

var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
var regionPattern = regexp.MustCompile(`^[a-z0-9-]{3,32}$`)
var ownerPattern = regexp.MustCompile(`^[0-9]{12}$`)
var kmsPattern = regexp.MustCompile(`^arn:(aws|aws-cn|aws-us-gov):kms:[a-z0-9-]+:[0-9]{12}:key/[a-zA-Z0-9-]+$`)
var prefixPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9/-]{0,127}$`)

func (c Config) Validate() error {
	if !bucketPattern.MatchString(c.Bucket) || strings.Contains(c.Bucket, "..") || net.ParseIP(c.Bucket) != nil || !regionPattern.MatchString(c.Region) || !prefixPattern.MatchString(c.Prefix) || strings.HasSuffix(c.Prefix, "/") || strings.Contains(c.Prefix, "//") || c.RetentionDays < 1 || c.RetentionDays > 3650 || len(c.KMSKeyID) > 2048 || c.KMSKeyID != "" && !kmsPattern.MatchString(c.KMSKeyID) {
		return errors.New("auditArchive requires a valid bucket, region, safe prefix, retentionDays (1–3650), and a key ARN when kmsKeyID is set")
	}
	switch c.Provider {
	case "", "s3":
		if !ownerPattern.MatchString(c.ExpectedBucketOwner) {
			return errors.New("S3 auditArchive requires a 12-digit expectedBucketOwner")
		}
	case "s3-compatible":
		if c.Endpoint == "" || c.ExpectedBucketOwner != "" || c.KMSKeyID != "" {
			return errors.New("S3-compatible storage auditArchive requires an explicit endpoint; omit AWS expectedBucketOwner and kmsKeyID, and configure SSE-S3 on the storage server")
		}
	default:
		return errors.New("auditArchive provider must be s3 or s3-compatible")
	}
	if c.Endpoint != "" {
		u, e := url.Parse(c.Endpoint)
		if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return errors.New("invalid auditArchive endpoint")
		}
		loopback := u.Hostname() == "localhost"
		if ip := net.ParseIP(u.Hostname()); ip != nil {
			loopback = ip.IsLoopback()
		}
		if u.Scheme != "https" && (u.Scheme != "http" || !loopback) {
			return errors.New("auditArchive endpoint requires HTTPS, except loopback acceptance fixtures")
		}
	}
	return nil
}
func (c Config) Target() string {
	b, _ := json.Marshal(c)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
