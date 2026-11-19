# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine AS build
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,id=nines-go-mod,target=/go/pkg/mod \
    go mod download
COPY api/ ./api/
COPY client/ ./client/
COPY internal/ ./internal/
COPY openslo/ ./openslo/
COPY provider/ ./provider/
COPY reviewskills/ ./reviewskills/
COPY skills/ ./skills/
COPY cmd/operator/ ./cmd/operator/
RUN --mount=type=cache,id=nines-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=nines-go-build,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X github.com/ishantanu/nines/internal/buildinfo.Version=$VERSION -X github.com/ishantanu/nines/internal/buildinfo.Commit=$COMMIT -X github.com/ishantanu/nines/internal/buildinfo.Date=$BUILD_DATE" \
    -o /nines-operator ./cmd/operator
FROM build AS notices
RUN apk add --no-cache python3
COPY scripts/release/licenses.py ./scripts/release/licenses.py
COPY third_party/ ./third_party/
COPY web/public/shadcn-license.txt ./web/public/shadcn-license.txt
COPY web/public/beautiful-ui-license.txt ./web/public/beautiful-ui-license.txt
RUN --mount=type=cache,id=nines-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=nines-go-build,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH python3 scripts/release/licenses.py --go-only --package ./cmd/operator --out /operator-licenses
FROM gcr.io/distroless/static:nonroot
COPY --from=build --chmod=0555 /nines-operator /nines-operator
COPY --chmod=0444 LICENSE NOTICE THIRD_PARTY_NOTICES.md /licenses/
COPY --from=notices --chmod=0555 /operator-licenses /licenses/dependencies/
USER 65532:65532
ENTRYPOINT ["/nines-operator"]
