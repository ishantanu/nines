# syntax=docker/dockerfile:1
FROM golang:1.27.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY examples/service-graph ./examples/service-graph
RUN --mount=type=cache,id=nines-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=nines-go-build,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/service-graph ./examples/service-graph
FROM scratch
COPY --from=build /out/service-graph /service-graph
USER 65532:65532
ENTRYPOINT ["/service-graph"]
