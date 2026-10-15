package invalidate

import "context"

// Topic names an invalidation channel. Each cache subscribes to its own topic.
type Topic string

const (
	TopicAuthz            Topic = "authz"
	TopicCredentials      Topic = "credentials"
	TopicPolicy           Topic = "policy"
	TopicInstanceSettings Topic = "instance_settings"
	TopicSchema           Topic = "schema"
	TopicCompletion       Topic = "completion"
)

// Invalidator broadcasts cache invalidations. The key identifies the
// invalidated entry and its meaning is defined by each topic.
//
// Publish returns an error only when the message could not be handed to the
// transport; subscriber failures are never reported to the publisher.
// Subscribe returns an idempotent function that stops delivery.
type Invalidator interface {
	Publish(ctx context.Context, topic Topic, key string) error
	Subscribe(topic Topic, fn func(key string)) (unsubscribe func())
}
