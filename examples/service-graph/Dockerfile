FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY examples/service-graph ./examples/service-graph
RUN CGO_ENABLED=0 go build -trimpath -o /out/service-graph ./examples/service-graph
FROM scratch
COPY --from=build /out/service-graph /service-graph
USER 65532:65532
ENTRYPOINT ["/service-graph"]
