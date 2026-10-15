package schema

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/platform/invalidate"
)

// Keys published on invalidate.TopicSchema and invalidate.TopicCompletion:
//
//	<connectionID>/<originID>
//
// originID names the publishing Navigator instance. A subscriber skips keys
// carrying its own origin: the publisher has already done the local work, and
// applying it again would drop memory a Refresh has just filled. A key that
// does not parse clears the topic's caches for every connection, so a key
// shape the subscriber does not understand never leaves stale entries behind.
func invalidationKey(connID int64, origin string) string {
	return strconv.FormatInt(connID, 10) + "/" + origin
}

func parseInvalidationKey(key string) (int64, string, bool) {
	conn, origin, ok := strings.Cut(key, "/")
	if !ok || origin == "" {
		return 0, "", false
	}
	connID, err := strconv.ParseInt(conn, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return connID, origin, true
}

func newOriginID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// publishInvalidation must be called without n.mu held: in-process buses
// deliver synchronously and subscribers take n.mu.
func (n *Navigator) publishInvalidation(connID int64) {
	if n.bus == nil {
		return
	}
	key := invalidationKey(connID, n.origin)
	for _, topic := range []invalidate.Topic{invalidate.TopicSchema, invalidate.TopicCompletion} {
		if err := n.bus.Publish(context.Background(), topic, key); err != nil {
			n.logger.Warn("schema navigator invalidation publish failed",
				slog.String("topic", string(topic)),
				slog.Int64("connection_id", connID),
				slog.Any("error", err),
			)
		}
	}
}

func (n *Navigator) applySchemaInvalidation(key string) {
	connID, origin, ok := parseInvalidationKey(key)
	if !ok {
		n.logger.Warn("unrecognized schema invalidation key; clearing navigator memory",
			slog.String("topic", string(invalidate.TopicSchema)),
			slog.String("key", key),
		)
		n.mu.Lock()
		clear(n.memory)
		n.mu.Unlock()
		return
	}
	if origin == n.origin {
		return
	}
	n.mu.Lock()
	delete(n.memory, connID)
	n.mu.Unlock()
	n.logger.Debug("schema navigator memory invalidated", slog.Int64("connection_id", connID))
}

func (n *Navigator) applyCompletionInvalidation(key string) {
	connID, origin, ok := parseInvalidationKey(key)
	if !ok {
		n.logger.Warn("unrecognized completion invalidation key; clearing completion caches",
			slog.String("topic", string(invalidate.TopicCompletion)),
			slog.String("key", key),
		)
		n.mu.Lock()
		clear(n.sessionScopes)
		n.mu.Unlock()
		n.decoded.clear()
		return
	}
	if origin == n.origin {
		return
	}
	n.mu.Lock()
	delete(n.sessionScopes, connID)
	n.mu.Unlock()
	n.decoded.forget(connID)
	n.logger.Debug("schema completion caches invalidated", slog.Int64("connection_id", connID))
}
