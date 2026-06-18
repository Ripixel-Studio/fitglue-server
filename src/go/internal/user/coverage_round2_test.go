package user

import (
	"context"
	"errors"
	"testing"

	firebaseAuth "firebase.google.com/go/v4/auth"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/iterator"
)

// pagingUserIterator is a UserIterator with a real iterator.PageInfo so that it
// works correctly with iterator.NewPager (the simple mockUserIterator in
// service_test.go only supports Next(), not paging). It serves all buffered
// records in a single fetch.
type pagingUserIterator struct {
	pageInfo *iterator.PageInfo
	nextFunc func() error
	buf      []*firebaseAuth.ExportedUserRecord
	fetchErr error
}

func newPagingUserIterator(users []*firebaseAuth.ExportedUserRecord, fetchErr error) *pagingUserIterator {
	it := &pagingUserIterator{fetchErr: fetchErr}
	it.pageInfo, it.nextFunc = iterator.NewPageInfo(
		func(int, string) (string, error) {
			if it.fetchErr != nil {
				return "", it.fetchErr
			}
			it.buf = append(it.buf, users...)
			return "", nil // empty token => no more pages
		},
		func() int { return len(it.buf) },
		func() interface{} { b := it.buf; it.buf = nil; return b },
	)
	return it
}

func (it *pagingUserIterator) PageInfo() *iterator.PageInfo { return it.pageInfo }

func (it *pagingUserIterator) Next() (*firebaseAuth.ExportedUserRecord, error) {
	if err := it.nextFunc(); err != nil {
		return nil, err
	}
	rec := it.buf[0]
	it.buf = it.buf[1:]
	return rec, nil
}

// TestRevokeStravaToken_NetworkPaths exercises revokeStravaToken without a real
// network call. A canceled context forces http.DefaultClient.Do to fail fast,
// covering the request-construction success branch and the Do-error branch.
func TestRevokeStravaToken_NetworkPaths(t *testing.T) {
	t.Run("CanceledContextDoError", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately so the HTTP round trip never reaches the network
		err := revokeStravaToken(ctx, "some-access-token")
		assert.Error(t, err)
	})

	t.Run("InvalidRequestConstruction", func(t *testing.T) {
		// A nil context makes http.NewRequestWithContext return an error,
		// covering the early return branch.
		//nolint:staticcheck // intentionally passing nil context to hit error branch
		err := revokeStravaToken(nil, "token")
		assert.Error(t, err)
	})
}

// TestListUsers_PagerPaths covers the ListUsers happy path (iterator returns
// users, profiles fetched per user) plus the pager-error path. The nil-iterator
// path is already covered by TestListUsers in service_test.go.
func TestListUsers_PagerPaths(t *testing.T) {
	t.Run("SuccessWithUsersAndDefaultLimit", func(t *testing.T) {
		svc, store, _, auth := setupTest()
		store.profile = &pbuser.UserProfile{UserId: "u1"}
		auth.usersIter = newPagingUserIterator([]*firebaseAuth.ExportedUserRecord{
			{UserRecord: &firebaseAuth.UserRecord{
				UserInfo: &firebaseAuth.UserInfo{UID: "u1", Email: "u1@example.com", DisplayName: "User One"},
			}},
			{UserRecord: &firebaseAuth.UserRecord{
				UserInfo: &firebaseAuth.UserInfo{UID: "u2", Email: "u2@example.com"},
			}},
		}, nil)
		// Limit <= 0 should default to 50.
		resp, err := svc.ListUsers(context.Background(), &pbsvc.ListUsersRequest{})
		assert.NoError(t, err)
		assert.Len(t, resp.Users, 2)
		for _, u := range resp.Users {
			assert.NotNil(t, u)
			assert.NotEmpty(t, u.Email)
		}
	})

	t.Run("ProfileMissingFallsBackToStub", func(t *testing.T) {
		svc, store, _, auth := setupTest()
		// store returns nil profile + error; ListUsers warns then builds a stub.
		store.profile = nil
		store.err = errors.New("no profile")
		auth.usersIter = newPagingUserIterator([]*firebaseAuth.ExportedUserRecord{
			{UserRecord: &firebaseAuth.UserRecord{
				UserInfo: &firebaseAuth.UserInfo{UID: "u3", Email: "u3@example.com"},
			}},
		}, nil)
		resp, err := svc.ListUsers(context.Background(), &pbsvc.ListUsersRequest{Limit: 10})
		assert.NoError(t, err)
		assert.Len(t, resp.Users, 1)
		assert.Equal(t, "u3", resp.Users[0].UserId)
		assert.Equal(t, pbuser.UserTier_USER_TIER_HOBBYIST, resp.Users[0].Tier)
		assert.Equal(t, "u3@example.com", resp.Users[0].Email)
	})

	t.Run("PagerError", func(t *testing.T) {
		svc, _, _, auth := setupTest()
		auth.usersIter = newPagingUserIterator(nil, errors.New("iterator boom"))
		_, err := svc.ListUsers(context.Background(), &pbsvc.ListUsersRequest{Limit: 5})
		assert.Error(t, err)
	})
}
