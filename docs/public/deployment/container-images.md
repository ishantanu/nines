# Container images

Each release publishes two images to the GitHub Container Registry, built from the same tested commit as the release archives.

| Image                              | Contains                                              | Platforms                   |
| ---------------------------------- | ----------------------------------------------------- | --------------------------- |
| `ghcr.io/ishantanu/nines`          | `nines-server` with the embedded portal, `nines-mcp`  | `linux/amd64`, `linux/arm64` |
| `ghcr.io/ishantanu/nines-operator` | `nines-operator`                                      | `linux/amd64`, `linux/arm64` |

- Images are tagged with the release version only, such as `v0.1.0-alpha.4`. There is no `latest` tag while Nines is a pre-release.
- The release notes list each image's digest. Pin the digest in anything you deploy, so a later push cannot change what runs.
- Each image is signed with keyless [Cosign](https://docs.sigstore.dev/) by the release workflow, and carries GitHub build provenance and an SBOM.
- Dependency licenses are in `/licenses` inside each image.

## Try the demo

A container listens on all interfaces, so the server refuses to start without a random `API_TOKEN` of at least 24 characters, even in demo mode.

```sh
export API_TOKEN="$(openssl rand -hex 24)"
echo "$API_TOKEN"
docker run --rm -p 127.0.0.1:8080:8080 -e API_TOKEN ghcr.io/ishantanu/nines:v0.1.0-alpha.4 -demo
```

Open `http://127.0.0.1:8080`, choose **Set API token** and paste the token, then sign in with `admin` / `admin`. The demo keeps its data in memory and resets on restart.

## Run a persistent server

Supply a PostgreSQL database and the same settings as any other deployment. See [server configuration](../configuration.md).

```sh
docker run -d --name nines -p 127.0.0.1:8080:8080 \
  --env-file /secure/path/nines.env \
  ghcr.io/ishantanu/nines:v0.1.0-alpha.4
```

- The environment file holds `DATABASE_URL`, `API_TOKEN`, `NINES_PUBLIC_URL` and `NINES_BOOTSTRAP_PASSWORD`, as in the [Kubernetes guide](kubernetes.md#database-and-credentials).
- The image runs as user `65532` and listens on port 8080. The Compose setup in [Docker Compose](docker-compose.md) builds from source instead.

## Verify an image

Check the signature. The identity is the release workflow at the release tag:

```sh
cosign verify ghcr.io/ishantanu/nines:v0.1.0-alpha.4 \
  --certificate-identity "https://github.com/ishantanu/nines/.github/workflows/alpha-release.yml@refs/tags/v0.1.0-alpha.4" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Check the build provenance with the GitHub CLI:

```sh
gh attestation verify oci://ghcr.io/ishantanu/nines:v0.1.0-alpha.4 --repo ishantanu/nines
```

Use the same commands with `nines-operator` for the operator image.

From v0.1.0-alpha.5, releases also publish the [Helm chart](kubernetes.md#install-with-helm) as `oci://ghcr.io/ishantanu/charts/nines`, pinned to the release's image digests and signed by the same workflow. Verify it the same way, using the chart digest from the release notes:

```sh
cosign verify ghcr.io/ishantanu/charts/nines@sha256:CHART_DIGEST_FROM_THE_RELEASE_NOTES \
  --certificate-identity "https://github.com/ishantanu/nines/.github/workflows/alpha-release.yml@refs/tags/v0.1.0-alpha.5" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Build your own

The images use the repository `Dockerfile` and `deploy/operator/Dockerfile`. Both cross-compile, so one machine can build both platforms:

```sh
docker buildx build --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=v0.1.0-alpha.4 -t registry.example.com/nines:reviewed .
```
