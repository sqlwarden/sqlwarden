FROM golang:1.26.4-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

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
