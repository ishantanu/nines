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
COPY integration/aiplayground/ ./integration/aiplayground/
RUN --mount=type=cache,id=nines-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=nines-go-build,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ai-playground ./integration/aiplayground

FROM scratch
COPY LICENSE NOTICE THIRD_PARTY_NOTICES.md /licenses/
COPY --from=build /out/ai-playground /ai-playground
USER 65532:65532
EXPOSE 8001
ENTRYPOINT ["/ai-playground"]
CMD ["serve"]
