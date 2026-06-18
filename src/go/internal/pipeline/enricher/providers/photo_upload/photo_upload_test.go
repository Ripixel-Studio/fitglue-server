package photo_upload

import (
	"context"
	"image"
	"io"
	"log/slog"
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

func puLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestPhotoUpload_Metadata(t *testing.T) {
	p := &Provider{}
	if p.Name() != "photo-upload" {
		t.Errorf("name %q", p.Name())
	}
	if p.ProviderType() != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_PHOTO_UPLOAD {
		t.Errorf("type %v", p.ProviderType())
	}
	p.SetService(nil)
}

func TestPhotoUpload_Enrich_SkipOnDoNotRetry(t *testing.T) {
	res, err := (&Provider{}).Enrich(context.Background(), puLogger(), &pbactivity.StandardizedActivity{}, nil, nil, true)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Metadata["photo_upload_status"] != "skipped" {
		t.Errorf("expected skipped, got %v", res.Metadata)
	}
}

func TestPhotoUpload_Enrich_ErrorsWhenServiceNil(t *testing.T) {
	_, err := (&Provider{}).Enrich(context.Background(), puLogger(), &pbactivity.StandardizedActivity{}, nil, nil, false)
	if err == nil {
		t.Error("expected error when service not initialised")
	}
}

func TestScaleToMax(t *testing.T) {
	t.Run("downscales large image preserving aspect", func(t *testing.T) {
		src := image.NewRGBA(image.Rect(0, 0, 2000, 1000))
		out := scaleToMax(src, 500)
		b := out.Bounds()
		if b.Dx() != 500 {
			t.Errorf("expected width 500, got %d", b.Dx())
		}
		if b.Dy() != 250 {
			t.Errorf("expected height 250 (aspect preserved), got %d", b.Dy())
		}
	})

	t.Run("tall image scales by height", func(t *testing.T) {
		src := image.NewRGBA(image.Rect(0, 0, 1000, 2000))
		out := scaleToMax(src, 500)
		if out.Bounds().Dy() != 500 {
			t.Errorf("expected height 500, got %d", out.Bounds().Dy())
		}
	})

	t.Run("small image returned unchanged", func(t *testing.T) {
		src := image.NewRGBA(image.Rect(0, 0, 100, 100))
		out := scaleToMax(src, 500)
		if out.Bounds().Dx() != 100 || out.Bounds().Dy() != 100 {
			t.Errorf("small image should be unchanged, got %v", out.Bounds())
		}
	})
}

func TestBuildWaitError(t *testing.T) {
	we := buildWaitError("act-1", "photo-upload")
	if we.ActivityID != "act-1" || we.EnricherProviderID != "photo-upload" {
		t.Errorf("unexpected: %+v", we)
	}
}
