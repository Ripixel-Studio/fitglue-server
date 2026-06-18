package notificationpub

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudevents/sdk-go/v2/event"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
)

type fakePublisher struct {
	topic string
	data  []byte
	err   error
}

func (f *fakePublisher) PublishCloudEvent(_ context.Context, _ string, _ event.Event) (string, error) {
	return "", nil
}
func (f *fakePublisher) PublishJSON(_ context.Context, topic string, data []byte) error {
	f.topic = topic
	f.data = data
	return f.err
}

func TestEnqueue(t *testing.T) {
	pub := &fakePublisher{}
	req := &pbnotification.NotificationRequest{UserId: "u1"}
	if err := Enqueue(context.Background(), pub, req); err != nil {
		t.Fatalf("err: %v", err)
	}
	if pub.topic != TopicNotification {
		t.Errorf("topic = %q, want %q", pub.topic, TopicNotification)
	}
	if len(pub.data) == 0 {
		t.Error("expected marshalled payload")
	}
}

func TestEnqueue_PublishError(t *testing.T) {
	pub := &fakePublisher{err: errors.New("pubsub down")}
	if err := Enqueue(context.Background(), pub, &pbnotification.NotificationRequest{}); err == nil {
		t.Error("expected publish error to propagate")
	}
}
