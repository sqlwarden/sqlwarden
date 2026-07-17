FROM oven/bun:1.3.10-alpine AS frontend-builder

WORKDIR /build

COPY frontend/package.json frontend/bun.lock ./frontend/
RUN cd frontend && bun install --frozen-lockfile

COPY frontend ./frontend

# One argument selects both halves of the artifact. This prevents an
# enterprise backend from accidentally embedding a community frontend.
ARG EDITION=community
RUN case "$EDITION" in \
      community) cd frontend && bun run build ;; \
      enterprise) cd frontend && bun run build:enterprise ;; \
      *) echo "unsupported EDITION: $EDITION" >&2; exit 1 ;; \
    esac

FROM golang:1.26.5-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend-builder /build/assets/static ./assets/static

ARG EDITION=community
RUN case "$EDITION" in \
      community) build_tags="" ;; \
      enterprise) build_tags="enterprise" ;; \
      *) echo "unsupported EDITION: $EDITION" >&2; exit 1 ;; \
    esac && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
      -tags "$build_tags" \
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
