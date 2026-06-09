package photo_upload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"log/slog"
	"strings"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/user_input"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"

	pendinginput "github.com/fitglue/server/src/go/pkg/pending_input"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

const (
	maxServerDim = 1600
	jpegQuality  = 85
)

type Provider struct {
	service *bootstrap.Service
}

// Compile-time assertion: photo_upload supports non-blocking mode.
var _ providers.SupportsNonBlocking = (*Provider)(nil)

func init() {
	providers.Register(&Provider{})
}

func (p *Provider) SetService(s *bootstrap.Service) {
	p.service = s
}

func (p *Provider) Name() string { return "photo-upload" }

func (p *Provider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_PHOTO_UPLOAD
}

// Enrich pauses the pipeline to request activity photo uploads from the user.
// If doNotRetry is set (user dismissed), the enricher skips gracefully.
func (p *Provider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, userRec *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	if doNotRetry {
		logger.Debug("photo-upload: skipping — do-not-retry flag set")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"photo_upload_status": "skipped"},
		}, nil
	}

	if p.service == nil {
		return nil, fmt.Errorf("service not initialized")
	}

	stableID := pendinginput.GenerateID(activity.Source.String(), activity.ExternalId, p.Name())

	pending, err := p.service.DB.GetPendingInput(ctx, userRec.UserId, stableID)
	if err == nil && pending != nil {
		if pending.Status == pbpipeline.PendingInput_STATUS_COMPLETED {
			return p.applyCompletedInput(ctx, pending)
		}
		if pending.Status == pbpipeline.PendingInput_STATUS_WAITING {
			logger.Debug("photo-upload: still waiting for photos")
			return nil, buildWaitError(stableID, p.Name())
		}
	}

	logger.Debug("photo-upload: requesting photo upload via pending input", "activity_id", stableID)
	return nil, buildWaitError(stableID, p.Name())
}

// EnrichResume is called when the user has submitted photo URLs via the pending input.
func (p *Provider) EnrichResume(ctx context.Context, _ *pbactivity.StandardizedActivity, _ *user.Record, pendingInput *pbpipeline.PendingInput) (*providers.EnrichmentResult, error) {
	return p.applyCompletedInput(ctx, pendingInput)
}

func (p *Provider) applyCompletedInput(ctx context.Context, pendingInput *pbpipeline.PendingInput) (*providers.EnrichmentResult, error) {
	rawPhotos := pendingInput.InputData["photos"]
	if rawPhotos == "" {
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"photo_upload_status": "no_photos"},
		}, nil
	}

	// Parse the JSON array of GCS public URLs submitted by the client.
	var photoURLs []string
	if err := json.Unmarshal([]byte(rawPhotos), &photoURLs); err != nil {
		return nil, fmt.Errorf("photo-upload: failed to parse photo URLs: %w", err)
	}

	if len(photoURLs) == 0 {
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"photo_upload_status": "no_photos"},
		}, nil
	}

	// Server-side resize: download each photo, resize to maxServerDim, re-upload as JPEG.
	// Falls back to the original URL for any photo that fails (e.g. HEIC, corrupt file).
	processedURLs := p.processPhotos(ctx, photoURLs)

	return &providers.EnrichmentResult{
		Metadata: map[string]string{
			"photo_urls":          strings.Join(processedURLs, ","),
			"photo_upload_status": "applied",
			"photo_count":         fmt.Sprintf("%d", len(processedURLs)),
		},
	}, nil
}

// processPhotos resizes each photo and returns the processed URLs.
// On any per-photo error (unsupported format, network failure), the original URL is used.
func (p *Provider) processPhotos(ctx context.Context, photoURLs []string) []string {
	if p.service == nil {
		return photoURLs
	}
	logger := slog.Default()
	processed := make([]string, 0, len(photoURLs))
	for _, url := range photoURLs {
		resizedURL, err := p.processOnePhoto(ctx, url)
		if err != nil {
			logger.Warn("photo-upload: resize failed, using original", "url", url, "error", err)
			processed = append(processed, url)
			continue
		}
		processed = append(processed, resizedURL)
	}
	return processed
}

// processOnePhoto downloads a GCS photo, resizes it to maxServerDim on the longest side,
// re-encodes as JPEG, and uploads to a "processed" sibling path in the same bucket.
// Returns the original URL unchanged if the image is already within bounds and is JPEG.
func (p *Provider) processOnePhoto(ctx context.Context, publicURL string) (string, error) {
	// Parse bucket + object path from "https://storage.googleapis.com/{bucket}/{path}"
	trimmed := strings.TrimPrefix(publicURL, "https://storage.googleapis.com/")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("unrecognised GCS public URL: %s", publicURL)
	}
	bucket, objectPath := parts[0], parts[1]

	// Download original bytes.
	data, err := p.service.Store.Get(ctx, bucket, objectPath)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}

	// Decode image. Unsupported formats (HEIC, RAW) return an error → caller uses original.
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}

	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Skip re-upload if the image is already a JPEG within target dimensions.
	lowerPath := strings.ToLower(objectPath)
	isJPEG := strings.HasSuffix(lowerPath, ".jpg") || strings.HasSuffix(lowerPath, ".jpeg")
	if isJPEG && w <= maxServerDim && h <= maxServerDim {
		return publicURL, nil
	}

	// Resize if needed.
	resized := scaleToMax(src, maxServerDim)

	// Encode as JPEG.
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return "", fmt.Errorf("jpeg encode: %w", err)
	}

	// Build processed path: strip original extension, append "_p.jpg".
	base := objectPath
	if idx := strings.LastIndexByte(base, '.'); idx >= 0 {
		base = base[:idx]
	}
	processedPath := base + "_p.jpg"

	if err := p.service.Store.Write(ctx, bucket, processedPath, buf.Bytes()); err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}

	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucket, processedPath), nil
}

// scaleToMax returns a scaled-down version of src if either dimension exceeds maxDim.
// Uses ApproxBiLinear for a good quality/speed tradeoff on photo downscaling.
func scaleToMax(src image.Image, maxDim int) image.Image {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= maxDim && h <= maxDim {
		return src
	}
	var newW, newH int
	if w > h {
		newW = maxDim
		newH = int(float64(h) * float64(maxDim) / float64(w))
	} else {
		newH = maxDim
		newW = int(float64(w) * float64(maxDim) / float64(h))
	}
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, bounds, xdraw.Over, nil)
	return dst
}

func buildWaitError(stableID, providerName string) *user_input.WaitForInputError {
	return &user_input.WaitForInputError{
		ActivityID:         stableID,
		RequiredFields:     []string{"photos"},
		EnricherProviderID: providerName,
		Metadata: map[string]string{
			"display.field_labels": `{"photos":"Activity Photos"}`,
			"display.field_types":  `{"photos":"photo_upload"}`,
			"display.summary":      "Add photos from this activity (optional)",
			"display.title":        "Upload Activity Photos",
			"display.max_photos":   "10",
		},
	}
}
