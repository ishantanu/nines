FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/providertrial ./integration/providertrial

FROM alpine:3.23 AS runtime
RUN apk add --no-cache ca-certificates
COPY --from=build /out/providertrial /providertrial
ENTRYPOINT ["/providertrial"]
