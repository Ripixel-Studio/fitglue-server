package pubsub

import (
	"context"
	"encoding/json"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/fitglue/server/src/go/internal/infra"
)

// publishTimeout bounds how long a single publish (batching + RPC) may take.
// It is applied to a context that is detached from the inbound request's
// deadline/cancellation — see publishWithAttrs for why.
const publishTimeout = 30 * time.Second

// PubSubAdapter provides message publishing using Google Cloud Pub/Sub
type PubSubAdapter struct {
	Client *pubsub.Client
	Logger infra.Logger
}

func (a *PubSubAdapter) logger() infra.Logger {
	return a.Logger.With("component", "publisher")
}

func (a *PubSubAdapter) PublishCloudEvent(ctx context.Context, topicID string, e event.Event) (string, error) {
	bytes, err := json.Marshal(e)
	if err != nil {
		a.logger().Error(ctx, "Failed to marshal CloudEvent", "topic", topicID, "error", err)
		return "", err
	}
	a.logger().Info(ctx, "Publishing CloudEvent",
		"topic", topicID,
		"event_type", e.Type(),
		"event_id", e.ID(),
		"source", e.Source(),
		"size_bytes", len(bytes))
	return a.publish(ctx, topicID, bytes)
}

func (a *PubSubAdapter) publish(ctx context.Context, topicID string, data []byte) (string, error) {
	return a.publishWithAttrs(ctx, topicID, data, nil)
}

func (a *PubSubAdapter) PublishJSON(ctx context.Context, topicID string, data []byte) error {
	_, err := a.publish(ctx, topicID, data)
	return err
}

func (a *PubSubAdapter) publishWithAttrs(ctx context.Context, topicID string, data []byte, attributes map[string]string) (string, error) {
	topic := a.Client.Topic(topicID)
	msg := &pubsub.Message{
		Data: data,
	}
	if attributes != nil {
		msg.Attributes = attributes
	}

	// Decouple the publish from the caller's request context. Handlers run as
	// Pub/Sub push targets whose context deadline is the message ack deadline;
	// slow upstream work (GCS offload, enrichment, provider calls) can leave
	// almost no budget by the time we publish. With the request context, the
	// batching + publish RPC then trips context.DeadlineExceeded on res.Get —
	// even though the event is valid and the message has usually already been
	// handed to the server. The event represents completed work that downstream
	// must receive, so give the publish its own bounded budget while retaining
	// context values (trace, etc.) via WithoutCancel.
	pubCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), publishTimeout)
	defer cancel()

	res := topic.Publish(pubCtx, msg)
	msgID, err := res.Get(pubCtx)
	if err != nil {
		a.logger().Error(ctx, "Failed to publish message", "topic", topicID, "error", err)
		return "", err
	}
	a.logger().Info(ctx, "Message published successfully", "topic", topicID, "message_id", msgID, "size_bytes", len(data))
	return msgID, nil
}
