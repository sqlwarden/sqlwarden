FROM golang:1.24-alpine AS builder

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
    addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /app

COPY --from=builder /build/sqlwarden .

RUN chown -R appuser:appuser /app

USER appuser

EXPOSE 6020

CMD ["./sqlwarden"]
