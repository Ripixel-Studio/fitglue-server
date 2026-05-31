package activity

import (
	"github.com/fitglue/server/src/go/internal/infra"
	shared "github.com/fitglue/server/src/go/pkg"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
)

// Service implements the pbsvc.ActivityServiceServer interface.
type Service struct {
	pbsvc.UnimplementedActivityServiceServer
	store                ActivityStore
	blobStore            BlobStore
	publisher            Publisher
	bucketName           string
	showcaseAssetsBucket string
	logger               infra.Logger
	notifications        shared.NotificationService
}

func NewService(store ActivityStore, blobStore BlobStore, publisher Publisher, bucketName string, showcaseAssetsBucket string, logger infra.Logger, notifications shared.NotificationService) *Service {
	return &Service{
		store:                store,
		blobStore:            blobStore,
		publisher:            publisher,
		bucketName:           bucketName,
		showcaseAssetsBucket: showcaseAssetsBucket,
		logger:               logger,
		notifications:        notifications,
	}
}
