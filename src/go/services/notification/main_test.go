package main

import (
	"context"
	"errors"
	"testing"

	firebaseAuth "firebase.google.com/go/v4/auth"
	"github.com/fitglue/server/src/go/internal/infra"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
)

// fakeAuth is a test double for authEmailLookup.
type fakeAuth struct {
	email  string
	err    error
	called bool
}

func (f *fakeAuth) GetUser(_ context.Context, _ string) (*firebaseAuth.UserRecord, error) {
	f.called = true
	if f.err != nil {
		return nil, f.err
	}
	return &firebaseAuth.UserRecord{UserInfo: &firebaseAuth.UserInfo{Email: f.email}}, nil
}

func TestResolveEmail(t *testing.T) {
	logger := infra.NewLoggerWithComponent("test")

	t.Run("doc email present is preferred and auth is not consulted", func(t *testing.T) {
		auth := &fakeAuth{email: "fromauth@example.com"}
		s := &notificationService{auth: auth, logger: logger}
		if got := s.resolveEmail(context.Background(), "u1", "fromdoc@example.com"); got != "fromdoc@example.com" {
			t.Errorf("resolveEmail() = %q, want %q", got, "fromdoc@example.com")
		}
		if auth.called {
			t.Error("auth was consulted even though the doc email was present")
		}
	})

	t.Run("falls back to auth when doc email is empty", func(t *testing.T) {
		auth := &fakeAuth{email: "fromauth@example.com"}
		s := &notificationService{auth: auth, logger: logger}
		if got := s.resolveEmail(context.Background(), "u1", ""); got != "fromauth@example.com" {
			t.Errorf("resolveEmail() = %q, want %q", got, "fromauth@example.com")
		}
		if !auth.called {
			t.Error("auth was not consulted for an empty doc email")
		}
	})

	t.Run("returns empty when auth lookup fails", func(t *testing.T) {
		auth := &fakeAuth{err: errors.New("user not found")}
		s := &notificationService{auth: auth, logger: logger}
		if got := s.resolveEmail(context.Background(), "u1", ""); got != "" {
			t.Errorf("resolveEmail() = %q, want empty string", got)
		}
	})

	t.Run("returns empty when auth client is unavailable", func(t *testing.T) {
		s := &notificationService{auth: nil, logger: logger}
		if got := s.resolveEmail(context.Background(), "u1", ""); got != "" {
			t.Errorf("resolveEmail() = %q, want empty string", got)
		}
	})
}

func TestPushDeepLinkPath(t *testing.T) {
	tests := []struct {
		name string
		typ  pbnotification.NotificationType
		data map[string]string
		want string
	}{
		{
			name: "pending input with activity id links to run detail",
			typ:  pbnotification.NotificationType_NOTIFICATION_TYPE_PENDING_INPUT,
			data: map[string]string{"activity_id": "act-123"},
			want: "/activities/act-123",
		},
		{
			name: "pending input without activity id falls back to inputs list",
			typ:  pbnotification.NotificationType_NOTIFICATION_TYPE_PENDING_INPUT,
			data: map[string]string{},
			want: "/inputs",
		},
		{
			name: "pipeline success links to activity detail",
			typ:  pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_SUCCESS,
			data: map[string]string{"activity_id": "act-9"},
			want: "/activities/act-9",
		},
		{
			name: "pipeline failure without id falls back to activities list",
			typ:  pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_FAILURE,
			data: map[string]string{},
			want: "/activities",
		},
		{
			name: "connection action links to the connection detail",
			typ:  pbnotification.NotificationType_NOTIFICATION_TYPE_CONNECTION_ACTION,
			data: map[string]string{"sourceId": "strava"},
			want: "/connections/strava",
		},
		{
			name: "connection action without source falls back to connections list",
			typ:  pbnotification.NotificationType_NOTIFICATION_TYPE_CONNECTION_ACTION,
			data: map[string]string{},
			want: "/connections",
		},
		{
			name: "pipeline cancelled links to activities list",
			typ:  pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_CANCELLED,
			data: map[string]string{},
			want: "/activities",
		},
		{
			name: "trial expiring links to subscription settings",
			typ:  pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRING,
			data: map[string]string{},
			want: "/settings/subscription",
		},
		{
			name: "showcase roundup has no SPA deep link",
			typ:  pbnotification.NotificationType_NOTIFICATION_TYPE_SHOWCASE_ROUNDUP,
			data: map[string]string{"slug": "jane", "period": "2026-W24"},
			want: "",
		},
		{
			name: "email-only billing type has no deep link",
			typ:  pbnotification.NotificationType_NOTIFICATION_TYPE_PAYMENT_FAILED,
			data: map[string]string{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pushDeepLinkPath(tt.typ, tt.data); got != tt.want {
				t.Errorf("pushDeepLinkPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
