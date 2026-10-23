FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS build
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X github.com/ishantanu/nines/internal/buildinfo.Version=$VERSION -X github.com/ishantanu/nines/internal/buildinfo.Commit=$COMMIT -X github.com/ishantanu/nines/internal/buildinfo.Date=$BUILD_DATE" \
    -o /nines-operator ./cmd/operator
FROM build AS notices
RUN apk add --no-cache python3
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH python3 scripts/release/licenses.py --go-only --package ./cmd/operator --out /operator-licenses
FROM gcr.io/distroless/static:nonroot
COPY --from=build --chmod=0555 /nines-operator /nines-operator
COPY --chmod=0444 LICENSE NOTICE THIRD_PARTY_NOTICES.md /licenses/
COPY --from=notices --chmod=0555 /operator-licenses /licenses/dependencies/
USER 65532:65532
ENTRYPOINT ["/nines-operator"]
