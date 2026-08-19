FROM oven/bun:1.3.10-alpine AS frontend-builder

WORKDIR /build/frontend

COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile

COPY frontend/ ./
RUN bun run build && test -s /build/assets/static/index.html

FROM golang:1.26.6-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

COPY --from=frontend-builder /build/assets/static ./assets/static

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o sqlwarden \
    ./cmd/api

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -g 1000 sqlwarden && \
    adduser -D -u 1000 -G sqlwarden -h /var/lib/sqlwarden sqlwarden && \
    mkdir -p /var/lib/sqlwarden

WORKDIR /app

COPY --from=builder /build/sqlwarden .

RUN chown -R sqlwarden:sqlwarden /app /var/lib/sqlwarden

USER sqlwarden

EXPOSE 6020

CMD ["./sqlwarden"]
