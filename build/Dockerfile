# syntax=docker/dockerfile:1.2.1
FROM golang:1.19.4-alpine AS builder
WORKDIR /go/src/github.com/healthjoy/mimir-rules-controller
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    cd cmd/controller \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -o /go/bin/mimir-rules-controller

FROM alpine:3.17.0
COPY --from=builder /go/bin/mimir-rules-controller /bin/mimir-rules-controller
ENTRYPOINT ["/bin/mimir-rules-controller"]
