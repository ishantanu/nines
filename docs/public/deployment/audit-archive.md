# Immutable audit archival

Nines can deliver each tenant's decision journal to an owner-configured Amazon S3 or S3-compatible bucket with Object Lock. It is disabled by default.

- The writer uses **COMPLIANCE** retention and verifies the object bytes, version, encryption and retention readback before persisting a delivery receipt.
- A successful upload response alone does not count as verified delivery.
- This protects the delivered object version for its retention period.
  - It does not make the Nines database immutable or guarantee that an event reaches S3 during an outage. Pending delivery remains visible.
  - Database administrators can alter local records and receipts; independently inspect the S3 versions and preserve separate access controls.

## Prepare the destination

- Provision a dedicated bucket with versioning and Object Lock enabled.
  - Choose the account, region and retention period deliberately.
  - In [S3 compliance mode](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html), protected versions cannot be overwritten or deleted during retention, and retention cannot be shortened.
  - Nines does not create buckets, change bucket policy, delete objects, enable legal holds or request governance bypass.
- Give the Nines server an IAM role scoped to the bucket and its tenant prefix. The bucket-level permission is `s3:GetBucketObjectLockConfiguration`.
  - Object permissions are `s3:PutObject`, `s3:PutObjectRetention`, `s3:GetObject` and `s3:GetObjectRetention`.
  - An independent verifier may also need `s3:GetObjectVersion`.
  - Do not grant this writer object deletion, bucket policy administration or retention bypass.
  - With a customer KMS key, also configure its key policy and the required `kms:GenerateDataKey`/`kms:Decrypt` permissions.
- Use separate tenant prefixes or buckets and apply corresponding IAM boundaries.
  - Nines puts the tenant ID and a destination-policy digest below the configured prefix.
  - A shared process IAM identity can access every prefix permitted to it; Nines configuration does not replace IAM isolation.

## Configure the tenant

Add `auditArchive` to the tenant in the server's `TENANTS_FILE`:

```yaml
tenants:
  - id: production
    name: Production
    access:
      - tokenEnv: PRODUCTION_API_TOKEN
        role: owner
    source:
      kind: prometheus
      url: http://prometheus:9090
    auditArchive:
      bucket: example-nines-audit
      region: eu-west-2
      expectedBucketOwner: "123456789012"
      prefix: nines/audit
      retentionDays: 365
      # Optional: full KMS key ARN, not an alias.
      # kmsKeyID: arn:aws:kms:eu-west-2:123456789012:key/00000000-0000-0000-0000-000000000000
```

- Replace the example bucket/account with your own. `retentionDays` is required and accepts 1–3650.
  - Credentials are obtained by the AWS SDK's default credential provider chain, including workload roles and standard AWS environment/profile configuration.
  - Keep access keys out of tenant YAML, manifests, CLI arguments, browser forms and model prompts. Prefer a workload IAM role.
- Archive object destinations are taken from this server-owned configuration.
  - SDK S3 endpoint environment overrides do not select another archive destination.
  - Set `provider: s3-compatible` with an explicit `endpoint` for a deliberately configured non-AWS HTTPS gateway; HTTP is accepted only for loopback acceptance fixtures.
  - Compatibility tests do not certify that another service provides Amazon S3's retention guarantees.
- Restart the server after configuring the tenant.
  - Persistent server deployments require PostgreSQL; configured archival is rejected with production `-memory` mode.
  - An explicit in-memory demo can exercise the API but loses its journal, queue and receipts when the process stops.

## Open-source object storage

- External storage remains optional.
  - The built-in journal and exports work without an object store.
  - Use `provider: s3-compatible` for an owner-configured non-AWS S3 endpoint.
  - This is a capability-based integration, not certification of every S3 implementation.
- Ceph RGW is the documented open-source target.
  - Its maintained Tentacle release documents [S3 compatibility](https://docs.ceph.com/en/tentacle/radosgw/s3/), [Object Lock](https://docs.ceph.com/en/tentacle/radosgw/s3/bucketops/) and [server-side encryption](https://docs.ceph.com/en/tentacle/radosgw/encryption/).
  - Ceph requires its own cluster administration; Nines does not install it.
  - The disposable container acceptance below has passed against Ceph 20.2.4 with OpenBao 2.5.0.
  - Your production storage, permissions and key-management setup still require independent acceptance.

```yaml
auditArchive:
  provider: s3-compatible
  endpoint: https://rgw.example.com
  bucket: nines-audit
  region: us-east-1
  prefix: nines/audit
  retentionDays: 365
```

- Set the region to the value configured for your gateway.
  - Use the HTTPS S3 API endpoint with a trusted certificate. Explicit endpoints use path-style bucket addressing.
  - HTTP is allowed only on loopback for acceptance fixtures.
  - Loopback in a container refers to that container. Do not disable TLS verification.
- Omit `expectedBucketOwner` and `kmsKeyID` in this mode; these fields select AWS account ownership and an AWS KMS key.
  - The default `s3` provider continues to require the 12-digit expected owner.
  - Configure SSE-S3 on the gateway itself, including its key-management backend.
  - Nines requests and verifies `AES256`; missing encryption does not produce a verified receipt.
- Provision the bucket with versioning and Object Lock before configuring delivery.
  - Supply a dedicated gateway access key to the Nines server through `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` using your deployment's secret mechanism.
  - These names belong to the S3 credential protocol; the explicit endpoint selects your gateway.
  - Keep secrets out of tenant YAML, URLs, browser forms and model inputs.
  - Limit the credentials to the bucket/prefix and required S3 operations above.
  - A process-wide credential chain is shared by configured tenants; use separate deployments if independent storage credentials are required.
- Before production use, run the provider acceptance check below with an isolated bucket/prefix and `retentionDays: 1`.
  - The gateway must enforce conditional object creation, checksum validation, versioning, encryption and COMPLIANCE retention. An S3-compatible label alone is insufficient.
  - No verification requirement is silently relaxed for non-AWS storage.
- Object Lock protects versions through the storage API.
  - Storage administrators with disk, encryption-key or backup access remain outside that boundary.
  - Keep storage administration separate and retain independent backups.

## Inspect delivery

Open **Audit log → External archive** or run:

```sh
nines audit status --context production --agent
nines audit status --record RECORD_UUID --context production --agent
```

- The same evidence is available through `GET /api/audit/archive` and `GET /api/audit/archive?record=RECORD_UUID`. Owners/editors can read it; agents and viewers cannot read tenant-wide archive status. The Go client exposes `AuditArchiveStatus`.

* **Disabled:** no external destination is configured. Local journal retention and NDJSON exports still work.
* **Pending:** a journal record has no verified receipt for the current destination policy.
* **Retrying:** one or more records failed verification/delivery and will retry. Codes distinguish unavailable destinations, missing bucket locking, content mismatch and unverified retention.
* **Verified:** the record was read back with its exact bytes, object version, encryption and compliance retention. Its receipt contains bucket/key/version, SHA-256, retain-until and verified-at times.
* The portal reports verified objects still within retention separately from elapsed retention periods.
  - Status is the last verified readback, not continuous provider monitoring.
  - Use provider-side monitoring for later access, lifecycle operations and account-level changes.
  - For an empty journal, zero verified records is not proof of an external write.
* The journal itself is the durable outbox.
  - Conversation updates and journal insertion commit together; a background worker claims undelivered records.
  - A claim expires after 90 seconds, remote work is bounded to 45 seconds, and failed deliveries back off from 10 seconds to one hour. Restarting preserves PostgreSQL work and receipts.
  - Conditional object creation plus readback recovers ambiguous upload responses without overwriting an existing object. A content mismatch never authorizes replacement.
* Retention starts at the current delivery generation's first claim. Retries keep that retain-until value.
  - If a pending generation expires during a prolonged outage, a later claim creates a fresh retention generation under a new object key.
  - Previously verified records are not automatically renewed after their configured retention expires.
* Changing provider, bucket, owner, region, prefix, retention, encryption or explicit endpoint creates a new destination-policy digest and independently queues retained journal records for that policy.
  - Previous objects and receipts remain; Nines does not delete or shorten their retention.
  - Credential rotation alone does not change the destination identity.
* Archival is asynchronous.
  - An archive outage does not undo or automatically block an approved SLO change.
  - Inspect pending counts and errors, alert on the `intent.audit_archive` operation outcome, and define your organization's outage/approval policy.
  - Configuration rollback still cannot restore consumed error budget.

## Disposable Ceph acceptance

Run from the repository root:

```sh
make test-ceph
```

- Requires Docker, Go and OpenSSL with `req -addext` support.
  - The runner uses digest-pinned official Ceph 20.2.4 and OpenBao 2.5.0 images.
  - It starts a single-node Ceph cluster backed by a 2 GiB container-local file and an ephemeral OpenBao transit engine for SSE-S3.
  - Ceph is capped at 1536 MiB/2 CPUs; OpenBao at 256 MiB/1 CPU.
  - Allow additional disk space for image downloads.
- The runner generates credentials and a two-day fixture certificate, trusts that certificate explicitly, and publishes only RGW's TLS endpoint on a random host loopback port.
  - The key service listens only inside the fixture network namespace.
  - It uses no privileged container, host block devices, production credentials or existing playground configuration.
- It creates synthetic locked and unprotected buckets, then checks:

* A Nines archive upload and independent exact-version JSON/encryption readback.
* Conditional retry returning the same checksum/key/version, with exactly one stored version.
* Successful deletion of an unprotected encrypted control object.
* Denial of protected-version deletion and COMPLIANCE retention shortening.
* Nines rejecting an unlocked bucket as `bucket_not_locked` and unchanged retention after the denied mutations.
* An exit trap removes only its uniquely named containers and temporary credentials/certificate directory, including on failure or interruption.
  - The synthetic retained data is destroyed with the container filesystem; this is an application-protocol test, not disk-level immutability, disaster recovery, cluster HA or production key-management certification. Downloaded image caches remain.
  - The fixture uses broad synthetic storage credentials and an ephemeral development key service, neither of which is a production deployment template.

## Provider acceptance

- Local tests cover signed SDK requests, checksum/content/version checks, conditional retry recovery, retention rejection, leases, PostgreSQL restart, tenant isolation and backoff. They do not establish a real AWS account's IAM, KMS, region or retention configuration.
- An opt-in acceptance test can write **one synthetic object retained for one day** into an isolated, pre-provisioned Object Lock bucket/prefix.
  - It never runs from ambient AWS credentials alone.
  - Create a private YAML file containing the `auditArchive` fields above, using `retentionDays: 1` and a dedicated acceptance prefix; configure credentials outside that file. Then explicitly run:

```sh
TEST_AUDIT_S3_CONFIG=/private/path/acceptance.yaml \
TEST_AUDIT_ARCHIVE_CONFIRM=COMPLIANCE \
go test -v ./internal/auditarchive -run '^TestRealS3ObjectLockReadbackAndIdempotency$' -count=1
```

- The test does not create a bucket or modify retention policy.
  - It verifies content/retention readback and that an exact retry recovers the same version.
  - It leaves the synthetic object retained for one day; cleanup cannot shorten compliance retention.
  - Independently inspect that version with an authorized verifier.
  - A passing S3-compatible test is separate from AWS acceptance.
