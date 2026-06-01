package billing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compile-time check that mockTrialPublisher satisfies the shared.Publisher interface
var _ interface {
	PublishCloudEvent(context.Context, string, event.Event) (string, error)
	PublishJSON(context.Context, string, []byte) error
} = (*mockTrialPublisher)(nil)

// mockTrialStore implements the Store interface subset needed by TrialChecker.
// Only the trial-checker methods are implemented; others panic to surface misuse.
type mockTrialStore struct {
	MockStore
	trialUsers        []TrialUser
	listErr           error
	warningSentCalls  []string
	expiredNotifCalls []string
}

func (m *mockTrialStore) ListUsersWithTrialsEndingBetween(_ context.Context, from, to time.Time) ([]TrialUser, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var result []TrialUser
	for _, u := range m.trialUsers {
		if !u.TrialEndsAt.Before(from) && !u.TrialEndsAt.After(to) {
			result = append(result, u)
		}
	}
	return result, nil
}

func (m *mockTrialStore) SetTrialWarningSent(_ context.Context, userID string, _ time.Time) error {
	m.warningSentCalls = append(m.warningSentCalls, userID)
	return nil
}

func (m *mockTrialStore) SetTrialExpiredNotified(_ context.Context, userID string, _ time.Time) error {
	m.expiredNotifCalls = append(m.expiredNotifCalls, userID)
	return nil
}

// mockPublisher records published messages and satisfies shared.Publisher
type mockTrialPublisher struct {
	publishedCount int
}

func (m *mockTrialPublisher) PublishCloudEvent(_ context.Context, _ string, _ event.Event) (string, error) {
	m.publishedCount++
	return "msg-id", nil
}

func (m *mockTrialPublisher) PublishJSON(_ context.Context, _ string, _ []byte) error {
	m.publishedCount++
	return nil
}

func newTrialChecker(store *mockTrialStore) (*TrialChecker, *mockTrialPublisher) {
	pub := &mockTrialPublisher{}
	checker := NewTrialChecker(store, pub, mockLogger{})
	return checker, pub
}

func TestHandleTrialCheck_NoUsersNoop(t *testing.T) {
	store := &mockTrialStore{}
	checker, pub := newTrialChecker(store)

	req := httptest.NewRequest(http.MethodPost, "/pubsub/trial-check", nil)
	rr := httptest.NewRecorder()
	checker.HandleTrialCheck(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, 0, pub.publishedCount)
}

func TestHandleTrialCheck_ExpiredUserGetsNotified(t *testing.T) {
	now := time.Now().UTC()
	store := &mockTrialStore{
		trialUsers: []TrialUser{
			{UserID: "user1", TrialEndsAt: now.Add(-24 * time.Hour)}, // expired yesterday
		},
	}
	checker, pub := newTrialChecker(store)

	req := httptest.NewRequest(http.MethodPost, "/pubsub/trial-check", nil)
	rr := httptest.NewRecorder()
	checker.HandleTrialCheck(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, store.expiredNotifCalls, "user1")
	assert.Greater(t, pub.publishedCount, 0)
}

func TestHandleTrialCheck_AlreadyNotifiedUserSkipped(t *testing.T) {
	now := time.Now().UTC()
	sentinel := now.Add(-1 * time.Hour) // already set
	store := &mockTrialStore{
		trialUsers: []TrialUser{
			{UserID: "user2", TrialEndsAt: now.Add(-24 * time.Hour), ExpiredNotifiedAt: &sentinel},
		},
	}
	checker, pub := newTrialChecker(store)

	req := httptest.NewRequest(http.MethodPost, "/pubsub/trial-check", nil)
	rr := httptest.NewRecorder()
	checker.HandleTrialCheck(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, store.expiredNotifCalls, "should not re-notify already-notified user")
	assert.Equal(t, 0, pub.publishedCount)
}

func TestHandleTrialCheck_ExpiringUserGetsWarning(t *testing.T) {
	now := time.Now().UTC()
	// Trial ends in 7 days (within the 6.5–7.5 day warning window)
	store := &mockTrialStore{
		trialUsers: []TrialUser{
			{UserID: "user3", TrialEndsAt: now.Add(7 * 24 * time.Hour)},
		},
	}
	checker, pub := newTrialChecker(store)

	req := httptest.NewRequest(http.MethodPost, "/pubsub/trial-check", nil)
	rr := httptest.NewRecorder()
	checker.HandleTrialCheck(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, store.warningSentCalls, "user3")
	assert.Greater(t, pub.publishedCount, 0)
}

func TestHandleTrialCheck_AlreadyWarnedUserSkipped(t *testing.T) {
	now := time.Now().UTC()
	warnedAt := now.Add(-1 * time.Hour)
	store := &mockTrialStore{
		trialUsers: []TrialUser{
			{UserID: "user4", TrialEndsAt: now.Add(7 * 24 * time.Hour), WarningSentAt: &warnedAt},
		},
	}
	checker, pub := newTrialChecker(store)

	req := httptest.NewRequest(http.MethodPost, "/pubsub/trial-check", nil)
	rr := httptest.NewRecorder()
	checker.HandleTrialCheck(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, store.warningSentCalls)
	assert.Equal(t, 0, pub.publishedCount)
}

func TestHandleTrialCheck_StoreErrorReturns500(t *testing.T) {
	store := &mockTrialStore{listErr: assert.AnError}
	checker, _ := newTrialChecker(store)

	req := httptest.NewRequest(http.MethodPost, "/pubsub/trial-check", nil)
	rr := httptest.NewRecorder()
	checker.HandleTrialCheck(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}
