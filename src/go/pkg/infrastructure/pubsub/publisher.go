package pubsub

import (
	"context"
	"encoding/json"

	"cloud.google.com/go/pubsub"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/fitglue/server/src/go/internal/infra"
)

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
	res := topic.Publish(ctx, msg)
	msgID, err := res.Get(ctx)
	if err != nil {
		a.logger().Error(ctx, "Failed to publish message", "topic", topicID, "error", err)
		return "", err
	}
	a.logger().Info(ctx, "Message published successfully", "topic", topicID, "message_id", msgID, "size_bytes", len(data))
	return msgID, nil
}
