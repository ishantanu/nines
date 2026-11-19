# syntax=docker/dockerfile:1
# Build stages run on the builder's own platform and cross-compile, so
# multi-architecture images do not emulate Node or the Go compiler.
FROM --platform=$BUILDPLATFORM node:24-alpine AS ui
WORKDIR /ui
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,id=nines-npm,target=/root/.npm \
    npm ci --no-audit --no-fund
COPY web/src/ ./src/
COPY web/public/ ./public/
COPY web/site/fonts/ ./site/fonts/
COPY web/index.html web/replay-player.html web/vite.config.ts web/site-brand.ts web/tsconfig.json ./
# The UI imports the same replay privacy catalog and test vectors as Go.
COPY internal/replay/public-labels.json /internal/replay/public-labels.json
COPY internal/replay/testdata/ /internal/replay/testdata/
COPY docs/public/brand/*.svg /docs/public/brand/
ARG VITE_OTEL_EXPORTER_OTLP_ENDPOINT
ENV VITE_OTEL_EXPORTER_OTLP_ENDPOINT=${VITE_OTEL_EXPORTER_OTLP_ENDPOINT}
RUN npm run build

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
COPY cmd/ ./cmd/
COPY examples/*.go examples/*.yaml ./examples/
COPY internal/ ./internal/
COPY openslo/ ./openslo/
COPY provider/ ./provider/
COPY reviewskills/ ./reviewskills/
COPY skills/ ./skills/
COPY --from=ui /internal/httpapi/web ./internal/httpapi/web
RUN --mount=type=cache,id=nines-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=nines-go-build,target=/root/.cache/go-build \
    flags="-s -w -X github.com/ishantanu/nines/internal/buildinfo.Version=$VERSION -X github.com/ishantanu/nines/internal/buildinfo.Commit=$COMMIT -X github.com/ishantanu/nines/internal/buildinfo.Date=$BUILD_DATE" && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false -ldflags="$flags" -o /out/nines-server ./cmd/server && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false -ldflags="$flags" -o /out/nines-mcp ./cmd/mcp

FROM build AS notices
RUN apk add --no-cache python3
COPY scripts/release/licenses.py ./scripts/release/licenses.py
COPY third_party/ ./third_party/
COPY web/package-lock.json ./web/package-lock.json
COPY web/public/shadcn-license.txt ./web/public/shadcn-license.txt
COPY web/public/beautiful-ui-license.txt ./web/public/beautiful-ui-license.txt
RUN --mount=type=cache,id=nines-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=nines-go-build,target=/root/.cache/go-build \
    --mount=type=bind,from=ui,source=/ui/node_modules,target=/src/web/node_modules \
    GOOS=$TARGETOS GOARCH=$TARGETARCH python3 scripts/release/licenses.py --out /out/LICENSES

FROM alpine:3.23
RUN apk add --no-cache wget ca-certificates
COPY LICENSE NOTICE THIRD_PARTY_NOTICES.md /licenses/
COPY --from=notices /out/LICENSES /licenses/dependencies/
COPY --from=build /out/nines-server /nines-server
COPY --from=build /out/nines-mcp /nines-mcp
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/nines-server", "-addr", "0.0.0.0:8080"]
