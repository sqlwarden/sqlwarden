// Package invalidate defines the cache-invalidation port and an in-process
// implementation.
//
// Messages carry only a topic and a key, never payloads. Delivery is
// best-effort, so every subscriber must keep a TTL ceiling that heals a lost
// message.
//
// TopicPolicy (never published) and TopicCredentials (published on session and
// credential revocation) have no subscribers today: policy
// evaluation and authentication read the database on every request, so
// nothing caches state that these topics would drop. The topics are declared
// so a future cache can subscribe without changing the publishers.
package invalidate
