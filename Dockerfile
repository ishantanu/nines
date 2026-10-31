FROM node:24-alpine AS ui
WORKDIR /ui
ARG VITE_OTEL_EXPORTER_OTLP_ENDPOINT
ENV VITE_OTEL_EXPORTER_OTLP_ENDPOINT=${VITE_OTEL_EXPORTER_OTLP_ENDPOINT}
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
# The UI imports the same replay privacy catalog and test vectors as Go.
COPY internal/replay/public-labels.json /internal/replay/public-labels.json
COPY internal/replay/testdata/ /internal/replay/testdata/
COPY docs/public/brand/ /docs/public/brand/
RUN npm run build

FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /internal/httpapi/web ./internal/httpapi/web
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/nines-server ./cmd/server
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/nines-mcp ./cmd/mcp

FROM build AS notices
RUN apk add --no-cache python3
COPY --from=ui /ui/node_modules ./web/node_modules
RUN python3 scripts/release/licenses.py --out /out/LICENSES

FROM scratch
COPY LICENSE NOTICE THIRD_PARTY_NOTICES.md /licenses/
COPY --from=notices /out/LICENSES /licenses/dependencies/
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/nines-server /nines-server
COPY --from=build /out/nines-mcp /nines-mcp
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/nines-server", "-addr", "0.0.0.0:8080"]
