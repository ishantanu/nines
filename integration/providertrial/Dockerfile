# syntax=docker/dockerfile:1
FROM golang:1.27.4-alpine AS build
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
COPY integration/providertrial/ ./integration/providertrial/
RUN --mount=type=cache,id=nines-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=nines-go-build,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/providertrial ./integration/providertrial

FROM alpine:3.23 AS runtime
RUN apk add --no-cache ca-certificates
COPY --from=build /out/providertrial /providertrial
ENTRYPOINT ["/providertrial"]
