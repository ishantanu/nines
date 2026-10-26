FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ai-playground ./integration/aiplayground

FROM scratch
COPY LICENSE NOTICE THIRD_PARTY_NOTICES.md /licenses/
COPY --from=build /out/ai-playground /ai-playground
USER 65532:65532
EXPOSE 8001
ENTRYPOINT ["/ai-playground"]
CMD ["serve"]
