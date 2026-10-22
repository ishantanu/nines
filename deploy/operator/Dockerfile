FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /nines-operator ./cmd/operator
FROM gcr.io/distroless/static:nonroot
COPY --from=build /nines-operator /nines-operator
USER 65532:65532
ENTRYPOINT ["/nines-operator"]
