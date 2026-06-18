package github

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Morning Run", "morning-run"},
		{"Leg Day!", "leg-day!"}, // '!' is not in replacer set, preserved
		{"A/B\\C:D", "a-b-c-d"},
		{"It's a (test), really.", "its-a-test-really"},
		{`Quote"Me"`, "quoteme"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"---leading-trailing---", "leading-trailing"},
		{"", ""},
		{"UPPER", "upper"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, sanitizeFileName(tc.in))
		})
	}
}

func TestBuildFilePath(t *testing.T) {
	d := time.Date(2026, 3, 7, 9, 30, 0, 0, time.UTC)
	got := buildFilePath("workouts/", "Morning Run", d)
	assert.Equal(t, "workouts/2026/03/2026-03-07-morning-run/activity.md", got)

	// Folder without trailing slash is used verbatim (caller normalizes).
	got2 := buildFilePath("logs/", "Leg Day", d)
	assert.Equal(t, "logs/2026/03/2026-03-07-leg-day/activity.md", got2)
}

func TestLoadGitHubConfig(t *testing.T) {
	t.Run("valid with default folder", func(t *testing.T) {
		cfg, err := loadGitHubConfig(&pbevents.ActivityPayload{
			Metadata: map[string]string{"github_repo": "octocat/hello"},
		})
		require.NoError(t, err)
		assert.Equal(t, "octocat/hello", cfg.Repo)
		assert.Equal(t, "workouts/", cfg.Folder)
		assert.Equal(t, "octocat", cfg.Owner)
		assert.Equal(t, "hello", cfg.Name)
	})

	t.Run("custom folder gets trailing slash", func(t *testing.T) {
		cfg, err := loadGitHubConfig(&pbevents.ActivityPayload{
			Metadata: map[string]string{"github_repo": "a/b", "github_folder": "runs"},
		})
		require.NoError(t, err)
		assert.Equal(t, "runs/", cfg.Folder)
	})

	t.Run("custom folder with slash preserved", func(t *testing.T) {
		cfg, err := loadGitHubConfig(&pbevents.ActivityPayload{
			Metadata: map[string]string{"github_repo": "a/b", "github_folder": "runs/"},
		})
		require.NoError(t, err)
		assert.Equal(t, "runs/", cfg.Folder)
	})

	t.Run("missing repo errors", func(t *testing.T) {
		_, err := loadGitHubConfig(&pbevents.ActivityPayload{Metadata: map[string]string{}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "github_repo not configured")
	})

	t.Run("invalid repo format errors", func(t *testing.T) {
		_, err := loadGitHubConfig(&pbevents.ActivityPayload{
			Metadata: map[string]string{"github_repo": "noslash"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid repo format")
	})
}

func TestBuildMarkdownContent(t *testing.T) {
	pid := "pipe-1"
	aid := "act-9"
	payload := &pbevents.ActivityPayload{
		ActivityId: &aid,
		PipelineId: &pid,
		Timestamp:  timestamppb.New(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)),
		Metadata: map[string]string{
			"activity_type":       "ACTIVITY_TYPE_RUN",
			"description":         "Felt great",
			"applied_enrichments": "weather,spotify",
			"tags":                "morning,easy",
		},
	}
	out := buildMarkdownContent(payload, "Morning Run", "activity.fit")

	assert.Contains(t, out, `title: "Morning Run"`)
	assert.Contains(t, out, "type: RUN") // ACTIVITY_TYPE_ prefix stripped
	assert.Contains(t, out, "date: 2026-01-02T03:04:05Z")
	assert.Contains(t, out, "activity_id: act-9")
	assert.Contains(t, out, "pipeline_id: pipe-1")
	assert.Contains(t, out, "fit_file: activity.fit")
	assert.Contains(t, out, "enrichments: [weather,spotify]")
	assert.Contains(t, out, "tags: [morning,easy]")
	assert.Contains(t, out, "# Morning Run")
	assert.Contains(t, out, "Felt great")
	assert.Contains(t, out, "<!-- fitglue:end -->")
	// front matter delimiters present
	assert.True(t, strings.HasPrefix(out, "---\n"))
}

func TestBuildMarkdownContent_Minimal(t *testing.T) {
	// No timestamp, no fit file, no optional metadata.
	out := buildMarkdownContent(&pbevents.ActivityPayload{Metadata: map[string]string{}}, "Activity", "")
	assert.NotContains(t, out, "date:")
	assert.NotContains(t, out, "fit_file:")
	assert.NotContains(t, out, "enrichments:")
	assert.NotContains(t, out, "tags:")
	assert.Contains(t, out, "# Activity")
}

func TestMergeWithUserContent(t *testing.T) {
	marker := "<!-- fitglue:end -->"

	t.Run("no marker in existing returns new", func(t *testing.T) {
		got := mergeWithUserContent("new"+marker, "old without marker")
		assert.Equal(t, "new"+marker, got)
	})

	t.Run("empty user content returns new", func(t *testing.T) {
		got := mergeWithUserContent("new"+marker, "old"+marker+"   \n  ")
		assert.Equal(t, "new"+marker, got)
	})

	t.Run("preserves user content after marker", func(t *testing.T) {
		got := mergeWithUserContent("generated"+marker, "old"+marker+"\nUser note")
		assert.Equal(t, "generated"+marker+"\nUser note", got)
	})

	t.Run("new has no marker appends user content", func(t *testing.T) {
		got := mergeWithUserContent("nomarker", "old"+marker+"\nUser note")
		assert.Equal(t, "nomarker\n"+marker+"\nUser note", got)
	})
}

func TestGitHubHeaders(t *testing.T) {
	req, err := http.NewRequest("GET", "https://api.github.com", nil)
	require.NoError(t, err)
	require.NoError(t, gitHubHeaders(context.Background(), req))
	assert.Equal(t, "application/vnd.github.v3+json", req.Header.Get("Accept"))
	assert.Equal(t, "FitGlue/1.0", req.Header.Get("User-Agent"))
	assert.Equal(t, "2022-11-28", req.Header.Get("X-GitHub-Api-Version"))
}

func TestGitHubUploader_Create_NoIntegration(t *testing.T) {
	u := New(&bootstrap.Service{})
	ctx := context.Background()
	payload := &pbevents.ActivityPayload{UserId: "u1"}

	// nil Integrations
	_, err := u.Create(ctx, payload, &user.Record{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no GitHub integration")

	// Integration present but disabled
	rec := &user.Record{Integrations: &pbuser.UserIntegrations{Github: &pbuser.GitHubIntegration{Enabled: false}}}
	_, err = u.Create(ctx, payload, rec)
	require.Error(t, err)
}

func TestGitHubUploader_Update_NoIntegration(t *testing.T) {
	u := New(&bootstrap.Service{})
	err := u.Update(context.Background(), &pbevents.ActivityPayload{UserId: "u1"}, &user.Record{}, &pbpipeline.PipelineRun{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no GitHub integration")
}
