package user

import (
	"context"
	"fmt"
	"strings"

	"github.com/fitglue/server/src/go/internal/infra"
)

// PrefixStorage is the slice of the storage adapter the purger needs.
type PrefixStorage interface {
	ListByPrefix(ctx context.Context, bucketName, prefix string) ([]string, error)
	DeleteByPrefix(ctx context.Context, bucketName, prefix string) (int, error)
}

// ArtifactPurger removes every GCS object belonging to a user when their
// account is deleted. Since the artifacts bucket stopped lifecycle-deleting
// (activity data must never degrade), account deletion is the only mechanism
// that removes this data — right-to-erasure depends on it running.
type ArtifactPurger struct {
	storage             PrefixStorage
	artifactsBucket     string
	showcaseAssetBucket string
	logger              infra.Logger
}

func NewArtifactPurger(storage PrefixStorage, artifactsBucket, showcaseAssetBucket string, logger infra.Logger) *ArtifactPurger {
	return &ArtifactPurger{
		storage:             storage,
		artifactsBucket:     artifactsBucket,
		showcaseAssetBucket: showcaseAssetBucket,
		logger:              logger,
	}
}

// PurgeUser deletes the user's objects from both buckets. showcaseExecIDs are
// pipeline execution IDs gathered from the user's showcase documents *before*
// those documents are deleted — route thumbnails are stored under
// {execution_id}/ in the showcase-assets bucket, which is not user-keyed.
// Execution IDs are also recovered from enriched_events object names, which
// covers activities that were never showcased.
func (p *ArtifactPurger) PurgeUser(ctx context.Context, userID string, showcaseExecIDs []string) error {
	if userID == "" {
		return fmt.Errorf("userID is required")
	}

	execIDs := map[string]bool{}
	for _, id := range showcaseExecIDs {
		if id != "" {
			execIDs[id] = true
		}
	}

	// enriched_events/{uid}/{execID}.json — collect exec IDs before the
	// prefix is deleted below.
	enrichedPrefix := fmt.Sprintf("enriched_events/%s/", userID)
	names, err := p.storage.ListByPrefix(ctx, p.artifactsBucket, enrichedPrefix)
	if err != nil {
		return fmt.Errorf("listing enriched events: %w", err)
	}
	for _, name := range names {
		base := strings.TrimSuffix(strings.TrimPrefix(name, enrichedPrefix), ".json")
		if base != "" && !strings.Contains(base, "/") {
			execIDs[base] = true
		}
	}

	// User-keyed prefixes in both buckets.
	targets := []struct{ bucket, prefix string }{
		{p.artifactsBucket, fmt.Sprintf("activities/%s/", userID)},
		{p.artifactsBucket, enrichedPrefix},
		{p.artifactsBucket, fmt.Sprintf("payloads/%s/", userID)},
		{p.showcaseAssetBucket, fmt.Sprintf("showcase_data/%s/", userID)},
		{p.showcaseAssetBucket, fmt.Sprintf("activity_photos/%s/", userID)},
	}
	// Route thumbnails: {execID}/route-thumbnail.svg in showcase-assets.
	for id := range execIDs {
		targets = append(targets, struct{ bucket, prefix string }{p.showcaseAssetBucket, id + "/"})
	}

	total := 0
	for _, t := range targets {
		n, err := p.storage.DeleteByPrefix(ctx, t.bucket, t.prefix)
		total += n
		if err != nil {
			return fmt.Errorf("purging gs://%s/%s (deleted %d so far): %w", t.bucket, t.prefix, total, err)
		}
	}
	p.logger.Info(ctx, "purged user artifacts", "user_id", userID, "objects_deleted", total, "exec_prefixes", len(execIDs))
	return nil
}
