package main

import (
	"context"
	"testing"

	"github.com/fitglue/server/src/go/internal/infra"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrimPrefix(t *testing.T) {
	assert.Equal(t, "World", trimPrefix("Hello World", "Hello "))
	assert.Equal(t, "Hello World", trimPrefix("Hello World", "Goodbye "))
	assert.Equal(t, "", trimPrefix("abc", "abc"))
}

func TestTrimSuffix(t *testing.T) {
	assert.Equal(t, "Hello", trimSuffix("Hello World", " World"))
	assert.Equal(t, "Hello World", trimSuffix("Hello World", " Goodbye"))
	assert.Equal(t, "", trimSuffix("abc", "abc"))
}

func TestNotificationTypeString(t *testing.T) {
	tests := []struct {
		typ  pbnotification.NotificationType
		want string
	}{
		{pbnotification.NotificationType_NOTIFICATION_TYPE_PENDING_INPUT, "PENDING_INPUT"},
		{pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_SUCCESS, "PIPELINE_SUCCESS"},
		{pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_FAILURE, "PIPELINE_FAILED"},
		{pbnotification.NotificationType_NOTIFICATION_TYPE_CONNECTION_ACTION, "CONNECTION_ACTION"},
		{pbnotification.NotificationType_NOTIFICATION_TYPE_SHOWCASE_ROUNDUP, "SHOWCASE_ROUNDUP"},
		{pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_CANCELLED, "PIPELINE_CANCELLED"},
		{pbnotification.NotificationType_NOTIFICATION_TYPE_SUBSCRIPTION_STARTED, "SUBSCRIPTION_STARTED"},
		{pbnotification.NotificationType_NOTIFICATION_TYPE_PAYMENT_FAILED, "PAYMENT_FAILED"},
		{pbnotification.NotificationType_NOTIFICATION_TYPE_SUBSCRIPTION_ENDED, "SUBSCRIPTION_ENDED"},
		{pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRING, "TRIAL_EXPIRING"},
		{pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRED, "TRIAL_EXPIRED"},
		{pbnotification.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED, "UNKNOWN"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, notificationTypeString(tc.typ))
		})
	}
}

func TestActiveChannels_TransactionalAlwaysEmail(t *testing.T) {
	// Even with prefs that would suppress, transactional billing types go to email.
	prefs := &pbuser.NotificationPreferences{}
	for _, typ := range []pbnotification.NotificationType{
		pbnotification.NotificationType_NOTIFICATION_TYPE_SUBSCRIPTION_STARTED,
		pbnotification.NotificationType_NOTIFICATION_TYPE_PAYMENT_FAILED,
		pbnotification.NotificationType_NOTIFICATION_TYPE_SUBSCRIPTION_ENDED,
		pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRED,
	} {
		ch := activeChannels(prefs, typ)
		require.Len(t, ch, 1)
		assert.Equal(t, pbuser.NotificationChannel_NOTIFICATION_CHANNEL_EMAIL, ch[0])
	}
}

func TestActiveChannels_TrialExpiringPushAndEmail(t *testing.T) {
	ch := activeChannels(&pbuser.NotificationPreferences{}, pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRING)
	assert.ElementsMatch(t, []pbuser.NotificationChannel{
		pbuser.NotificationChannel_NOTIFICATION_CHANNEL_PUSH,
		pbuser.NotificationChannel_NOTIFICATION_CHANNEL_EMAIL,
	}, ch)
}

func TestActiveChannels_DefaultsToPushWhenNoPref(t *testing.T) {
	// Pipeline types with no per-type preference set default to PUSH.
	ch := activeChannels(&pbuser.NotificationPreferences{}, pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_SUCCESS)
	require.Len(t, ch, 1)
	assert.Equal(t, pbuser.NotificationChannel_NOTIFICATION_CHANNEL_PUSH, ch[0])
}

func TestActiveChannels_RespectsPerTypePreference(t *testing.T) {
	prefs := &pbuser.NotificationPreferences{
		PipelineFailure: &pbuser.NotificationTypePreference{
			Channels: []pbuser.NotificationChannel{
				pbuser.NotificationChannel_NOTIFICATION_CHANNEL_EMAIL,
			},
		},
	}
	ch := activeChannels(prefs, pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_FAILURE)
	require.Len(t, ch, 1)
	assert.Equal(t, pbuser.NotificationChannel_NOTIFICATION_CHANNEL_EMAIL, ch[0])
}

func TestActiveChannels_EmptyChannelsSuppresses(t *testing.T) {
	// A per-type pref present but with no channels suppresses the notification.
	prefs := &pbuser.NotificationPreferences{
		PipelineSuccess: &pbuser.NotificationTypePreference{
			Channels: []pbuser.NotificationChannel{},
		},
	}
	ch := activeChannels(prefs, pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_SUCCESS)
	assert.Empty(t, ch)
}

func TestRenderEmail_BillingTypes(t *testing.T) {
	s := &notificationService{baseURL: "https://fitglue.tech", logger: infra.NewLoggerWithComponent("test")}

	tests := []struct {
		name        string
		req         *pbnotification.NotificationRequest
		wantSubject string
	}{
		{
			name:        "subscription started",
			req:         &pbnotification.NotificationRequest{Type: pbnotification.NotificationType_NOTIFICATION_TYPE_SUBSCRIPTION_STARTED},
			wantSubject: "You're now a FitGlue Athlete!",
		},
		{
			name:        "payment failed",
			req:         &pbnotification.NotificationRequest{Type: pbnotification.NotificationType_NOTIFICATION_TYPE_PAYMENT_FAILED},
			wantSubject: "Action required: your FitGlue payment failed",
		},
		{
			name:        "subscription ended",
			req:         &pbnotification.NotificationRequest{Type: pbnotification.NotificationType_NOTIFICATION_TYPE_SUBSCRIPTION_ENDED},
			wantSubject: "Your FitGlue Athlete subscription has ended",
		},
		{
			name:        "trial expired",
			req:         &pbnotification.NotificationRequest{Type: pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRED},
			wantSubject: "Your FitGlue Athlete trial has ended",
		},
		{
			name:        "pipeline cancelled",
			req:         &pbnotification.NotificationRequest{Type: pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_CANCELLED},
			wantSubject: "A FitGlue pipeline run was cancelled",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			subject, html := s.renderEmail(tc.req)
			assert.Equal(t, tc.wantSubject, subject)
			assert.NotEmpty(t, html)
		})
	}
}

func TestRenderEmail_TrialExpiring_Pluralization(t *testing.T) {
	s := &notificationService{baseURL: "https://fitglue.tech", logger: infra.NewLoggerWithComponent("test")}

	t.Run("multiple days plural", func(t *testing.T) {
		subject, html := s.renderEmail(&pbnotification.NotificationRequest{
			Type: pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRING,
			Data: map[string]string{"days_left": "3"},
		})
		assert.Equal(t, "Your Athlete trial ends in 3 days", subject)
		assert.NotEmpty(t, html)
	})

	t.Run("one day singular", func(t *testing.T) {
		subject, _ := s.renderEmail(&pbnotification.NotificationRequest{
			Type: pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRING,
			Data: map[string]string{"days_left": "1"},
		})
		assert.Equal(t, "Your Athlete trial ends in 1 day", subject)
	})

	t.Run("default 7 days when unset", func(t *testing.T) {
		subject, _ := s.renderEmail(&pbnotification.NotificationRequest{
			Type: pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRING,
			Data: map[string]string{},
		})
		assert.Equal(t, "Your Athlete trial ends in 7 days", subject)
	})
}

func TestRenderEmail_DataExportReady(t *testing.T) {
	s := &notificationService{baseURL: "https://fitglue.tech", logger: infra.NewLoggerWithComponent("test")}
	subject, html := s.renderEmail(&pbnotification.NotificationRequest{
		Type: pbnotification.NotificationType_NOTIFICATION_TYPE_DATA_EXPORT_READY,
		Data: map[string]string{"download_url": "https://example.com/export.zip"},
	})
	assert.Equal(t, "Your FitGlue data export is ready", subject)
	assert.NotEmpty(t, html)
}

func TestRenderEmail_PipelineSuccess(t *testing.T) {
	s := &notificationService{baseURL: "https://fitglue.tech", logger: infra.NewLoggerWithComponent("test")}
	subject, html := s.renderEmail(&pbnotification.NotificationRequest{
		Type:  pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_SUCCESS,
		Title: "Activity Synced: Morning Run",
		Body:  "Successfully synced to: Strava, TrainingPeaks",
		Data:  map[string]string{"activity_id": "act-1"},
	})
	assert.Equal(t, "Morning Run synced to FitGlue", subject)
	assert.NotEmpty(t, html)
}

func TestRenderEmail_PipelineFailure(t *testing.T) {
	s := &notificationService{baseURL: "https://fitglue.tech", logger: infra.NewLoggerWithComponent("test")}

	t.Run("activity failed prefix", func(t *testing.T) {
		subject, _ := s.renderEmail(&pbnotification.NotificationRequest{
			Type:  pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_FAILURE,
			Title: "Activity Failed: Long Ride",
			Data:  map[string]string{"activity_id": "act-2"},
		})
		assert.Equal(t, "Sync error: Long Ride", subject)
	})

	t.Run("partial sync prefix", func(t *testing.T) {
		subject, _ := s.renderEmail(&pbnotification.NotificationRequest{
			Type:  pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_FAILURE,
			Title: "Partial Sync: Long Ride",
			Data:  map[string]string{"activity_id": "act-2"},
		})
		assert.Equal(t, "Sync error: Long Ride", subject)
	})
}

func TestRenderEmail_PendingInputAndConnection(t *testing.T) {
	s := &notificationService{baseURL: "https://fitglue.tech", logger: infra.NewLoggerWithComponent("test")}

	subject, html := s.renderEmail(&pbnotification.NotificationRequest{
		Type: pbnotification.NotificationType_NOTIFICATION_TYPE_PENDING_INPUT,
		Data: map[string]string{"activity_id": "act-3"},
	})
	assert.Equal(t, "Action required: your pipeline needs input", subject)
	assert.NotEmpty(t, html)

	subject, html = s.renderEmail(&pbnotification.NotificationRequest{
		Type:  pbnotification.NotificationType_NOTIFICATION_TYPE_CONNECTION_ACTION,
		Title: "Reconnect Strava",
	})
	assert.Equal(t, "Action required: reconnect Strava", subject)
	assert.NotEmpty(t, html)
}

func TestRenderEmail_ShowcaseRoundup(t *testing.T) {
	s := &notificationService{baseURL: "https://fitglue.tech", logger: infra.NewLoggerWithComponent("test")}
	subject, html := s.renderEmail(&pbnotification.NotificationRequest{
		Type:  pbnotification.NotificationType_NOTIFICATION_TYPE_SHOWCASE_ROUNDUP,
		Title: "Weekly Showcase Roundup Ready",
		Body:  "42 activities",
		Data:  map[string]string{"slug": "jane", "period": "2026-W24"},
	})
	assert.Equal(t, "Your weekly showcase roundup is ready", subject)
	assert.NotEmpty(t, html)
}

func TestRenderEmail_UnknownTypeReturnsEmpty(t *testing.T) {
	s := &notificationService{baseURL: "https://fitglue.tech", logger: infra.NewLoggerWithComponent("test")}
	subject, html := s.renderEmail(&pbnotification.NotificationRequest{
		Type: pbnotification.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED,
	})
	assert.Empty(t, subject)
	assert.Empty(t, html)
}

func TestDispatchEmail_NoSenderIsNoOp(t *testing.T) {
	// emailSender nil -> no-op, no panic.
	s := &notificationService{logger: infra.NewLoggerWithComponent("test")}
	s.dispatchEmail(context.Background(), &pbnotification.NotificationRequest{
		Type:   pbnotification.NotificationType_NOTIFICATION_TYPE_PAYMENT_FAILED,
		UserId: "u1",
	}, "to@example.com")
}

func TestDispatchPush_NoTokensIsNoOp(t *testing.T) {
	// No tokens -> returns before touching the (nil) FCM adapter.
	s := &notificationService{logger: infra.NewLoggerWithComponent("test")}
	s.dispatchPush(context.Background(), &pbnotification.NotificationRequest{
		Type:   pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_SUCCESS,
		UserId: "u1",
	}, nil)
}
