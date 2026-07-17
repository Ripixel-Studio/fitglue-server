package showcase

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"github.com/stretchr/testify/assert"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestShowcaseUploader_Name(t *testing.T) {
	u := New(&bootstrap.Service{}, nil, "test-showcase-assets")
	assert.Equal(t, "showcase", u.Name())
}

func TestPersistDurableActivityData_CopiesToShowcaseAssetsBucket(t *testing.T) {
	var writtenBucket, writtenObject string
	var writtenData []byte

	store := &mocks.MockBlobStore{
		GetFunc: func(ctx context.Context, bucket, object string) ([]byte, error) {
			assert.Equal(t, "gs://artifacts-bucket/payloads/user-1/act-1.json", object)
			return []byte(`{"activityId":"act-1"}`), nil
		},
		WriteFunc: func(ctx context.Context, bucket, object string, data []byte) error {
			writtenBucket, writtenObject, writtenData = bucket, object, data
			return nil
		},
	}

	u := New(&bootstrap.Service{Store: store}, nil, "showcase-assets-bucket")
	logger := testLogger()

	uri := u.persistDurableActivityData(context.Background(), logger, "user-1", "showcase-1", "gs://artifacts-bucket/payloads/user-1/act-1.json")

	assert.Equal(t, "gs://showcase-assets-bucket/showcase_data/user-1/showcase-1_data.json", uri)
	assert.Equal(t, "showcase-assets-bucket", writtenBucket)
	assert.Equal(t, "showcase_data/user-1/showcase-1_data.json", writtenObject)
	assert.Equal(t, []byte(`{"activityId":"act-1"}`), writtenData)
}

func TestPersistDurableActivityData_ReturnsEmptyOnReadFailure(t *testing.T) {
	store := &mocks.MockBlobStore{
		GetFunc: func(ctx context.Context, bucket, object string) ([]byte, error) {
			return nil, errors.New("object not found")
		},
	}

	u := New(&bootstrap.Service{Store: store}, nil, "showcase-assets-bucket")
	logger := testLogger()

	uri := u.persistDurableActivityData(context.Background(), logger, "user-1", "showcase-1", "gs://artifacts-bucket/payloads/user-1/act-1.json")

	assert.Empty(t, uri)
}

func TestPersistDurableActivityData_ReturnsEmptyOnWriteFailure(t *testing.T) {
	store := &mocks.MockBlobStore{
		GetFunc: func(ctx context.Context, bucket, object string) ([]byte, error) {
			return []byte(`{}`), nil
		},
		WriteFunc: func(ctx context.Context, bucket, object string, data []byte) error {
			return errors.New("write denied")
		},
	}

	u := New(&bootstrap.Service{Store: store}, nil, "showcase-assets-bucket")
	logger := testLogger()

	uri := u.persistDurableActivityData(context.Background(), logger, "user-1", "showcase-1", "gs://artifacts-bucket/payloads/user-1/act-1.json")

	assert.Empty(t, uri)
}

func TestPersistDurableFitFile_CopiesToShowcaseAssetsBucket(t *testing.T) {
	var writtenObject string

	store := &mocks.MockBlobStore{
		GetFunc: func(ctx context.Context, bucket, object string) ([]byte, error) {
			return []byte("fit-bytes"), nil
		},
		WriteFunc: func(ctx context.Context, bucket, object string, data []byte) error {
			writtenObject = object
			return nil
		},
	}

	u := New(&bootstrap.Service{Store: store}, nil, "showcase-assets-bucket")
	logger := testLogger()

	uri := u.persistDurableFitFile(context.Background(), logger, "user-1", "showcase-1", "gs://artifacts-bucket/activities/user-1/act-1.fit")

	assert.Equal(t, "gs://showcase-assets-bucket/showcase_data/user-1/showcase-1.fit", uri)
	assert.Equal(t, "showcase_data/user-1/showcase-1.fit", writtenObject)
}

func TestCalculateExpiration(t *testing.T) {
	now := time.Now()

	// Hobbyist test
	hobbyRec := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId: "hobbyist",
		},
	}
	expHobby := calculateExpiration(hobbyRec, now)
	assert.NotNil(t, expHobby)
	assert.True(t, expHobby.After(now.AddDate(0, 0, 29)))

	// Athlete test
	athleteRec := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId: "athlete",
			Tier:   pbuser.UserTier_USER_TIER_ATHLETE,
		},
	}
	expAthlete := calculateExpiration(athleteRec, now)
	assert.Nil(t, expAthlete)
}
