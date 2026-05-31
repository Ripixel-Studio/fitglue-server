// Package notificationpub provides a thin publisher for notification requests.
// Services call Enqueue to fire a notification; the dedicated notification service
// consumes the message and fans out to each channel the user has configured.
package notificationpub

import (
	"context"
	"fmt"

	shared "github.com/fitglue/server/src/go/pkg"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
	"google.golang.org/protobuf/encoding/protojson"
)

const TopicNotification = "topic-notification"

// Enqueue publishes a NotificationRequest to topic-notification.
// The notification service decides which channels (push, email, …) to use
// based on the user's stored preferences. This call returns as soon as the
// message is durably enqueued — delivery is best-effort and asynchronous.
func Enqueue(ctx context.Context, pub shared.Publisher, req *pbnotification.NotificationRequest) error {
	data, err := protojson.Marshal(req)
	if err != nil {
		return fmt.Errorf("notificationpub: marshal: %w", err)
	}
	return pub.PublishJSON(ctx, TopicNotification, data)
}
