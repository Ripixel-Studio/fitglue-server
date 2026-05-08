package github

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/description"
	activityPkg "github.com/fitglue/server/src/go/pkg/domain/activity"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/infrastructure/oauth"
	ghclient "github.com/fitglue/server/src/go/pkg/integrations/github"
	"github.com/fitglue/server/src/go/pkg/loopprevention"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Uploader implements destination.Destination for GitHub
type Uploader struct {
	svc *bootstrap.Service
}

// New returns a new GitHub Uploader initialized with dependencies.
func New(svc *bootstrap.Service) *Uploader {
	return &Uploader{
		svc: svc,
	}
}

// Name returns the identifier for this uploader
func (u *Uploader) Name() string {
	return "github"
}

type gitHubConfig struct {
	Repo   string // "owner/repo"
	Folder string // e.g. "workouts/"
	Owner  string // Parsed from Repo
	Name   string // Parsed from Repo
}

func loadGitHubConfig(payload *pbevents.ActivityPayload) (*gitHubConfig, error) {
	repo := ""
	folder := "workouts/"

	if r, ok := payload.Metadata["github_repo"]; ok {
		repo = r
	}
	if f, ok := payload.Metadata["github_folder"]; ok && f != "" {
		folder = f
	}
	if repo == "" {
		return nil, fmt.Errorf("github_repo not configured in metadata")
	}
	if !strings.HasSuffix(folder, "/") {
		folder += "/"
	}

	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s (expected owner/repo)", repo)
	}

	return &gitHubConfig{
		Repo:   repo,
		Folder: folder,
		Owner:  parts[0],
		Name:   parts[1],
	}, nil
}

// Create uploads a new activity to GitHub by committing a Markdown file (and optionally a FIT file).
func (u *Uploader) Create(ctx context.Context, payload *pbevents.ActivityPayload, userRec *user.Record) (string, error) {
	if userRec.Integrations == nil || userRec.Integrations.Github == nil || !userRec.Integrations.Github.Enabled {
		return "", fmt.Errorf("user has no GitHub integration configured")
	}

	tokenSource := oauth.NewFirestoreTokenSource(u.svc, payload.UserId, "github")
	httpClient := oauth.NewClientWithUsageTracking(tokenSource, u.svc, payload.UserId, "github", infra.NewLogger())
	logger := slog.Default()

	ghClient, err := ghclient.NewClientWithResponses("https://api.github.com",
		ghclient.WithHTTPClient(httpClient),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create GitHub client: %w", err)
	}

	config, err := loadGitHubConfig(payload)
	if err != nil {
		return "", fmt.Errorf("failed to load GitHub config: %w", err)
	}

	activityDate := time.Now()
	if payload.Timestamp != nil {
		activityDate = payload.Timestamp.AsTime()
	}

	activityName := payload.Metadata["activity_name"]
	if activityName == "" {
		activityName = "Activity"
	}

	filePath := buildFilePath(config.Folder, activityName, activityDate)

	fitFileName := ""
	if fitFileUri, ok := payload.Metadata["fit_file_uri"]; ok && fitFileUri != "" {
		logger.Info("FIT file URI found, downloading for GitHub commit", "uri", fitFileUri)
		fitData, fitErr := u.downloadFitFile(ctx, fitFileUri)
		if fitErr != nil {
			logger.Warn("Failed to download FIT file, continuing without it", "error", fitErr)
		} else {
			fitFileName = "activity.fit"
			fitPath := path.Join(path.Dir(filePath), fitFileName)
			fitCommitMsg := fmt.Sprintf("Add FIT data for %s", activityName)
			if _, fitCommitErr := u.createOrUpdateBinaryFile(ctx, ghClient, config, fitPath, fitData, fitCommitMsg, nil); fitCommitErr != nil {
				logger.Warn("Failed to commit FIT file, continuing without it", "error", fitCommitErr)
				fitFileName = ""
			} else {
				logger.Info("Committed FIT file to GitHub", "path", fitPath, "size", len(fitData))
			}
		}
	} else {
		logger.Info("No FIT file URI in payload metadata, skipping FIT commit")
	}

	markdownContent := buildMarkdownContent(payload, activityName, fitFileName)

	logger.Info("Creating file in GitHub",
		"repo", config.Repo,
		"path", filePath,
		"content_length", len(markdownContent),
		"has_fit_file", fitFileName != "",
	)

	commitMessage := fmt.Sprintf("Add %s — %s", activityName, activityDate.Format("2006-01-02"))
	externalID, err := u.createOrUpdateFile(ctx, ghClient, config, filePath, markdownContent, commitMessage, nil)
	if err != nil {
		return "", fmt.Errorf("GitHub create failed: %w", err)
	}

	uploadRecord := &pbactivity.UploadedActivityRecord{
		Id:            loopprevention.BuildUploadedActivityID(pbplugin.DestinationType_DESTINATION_GITHUB, externalID),
		UserId:        payload.UserId,
		Source:        payload.Source,
		ExternalId:    payload.StandardizedActivity.GetExternalId(),
		StartTime:     payload.Timestamp,
		Destination:   pbplugin.DestinationType_DESTINATION_GITHUB,
		DestinationId: externalID,
		UploadedAt:    timestamppb.Now(),
	}
	_ = u.svc.DB.SetUploadedActivity(ctx, payload.UserId, uploadRecord)

	_ = u.svc.DB.IncrementSyncCount(ctx, payload.UserId)

	return externalID, nil
}

// Update modifies an existing file in GitHub
func (u *Uploader) Update(ctx context.Context, payload *pbevents.ActivityPayload, userRec *user.Record, pipelineRun *pbpipeline.PipelineRun) error {
	if userRec.Integrations == nil || userRec.Integrations.Github == nil || !userRec.Integrations.Github.Enabled {
		return fmt.Errorf("user has no GitHub integration configured")
	}

	tokenSource := oauth.NewFirestoreTokenSource(u.svc, payload.UserId, "github")
	httpClient := oauth.NewClientWithUsageTracking(tokenSource, u.svc, payload.UserId, "github", infra.NewLogger())
	logger := slog.Default()

	ghClient, err := ghclient.NewClientWithResponses("https://api.github.com",
		ghclient.WithHTTPClient(httpClient),
	)
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}

	config, err := loadGitHubConfig(payload)
	if err != nil {
		return fmt.Errorf("failed to load GitHub config: %w", err)
	}

	var existingFilePath string
	if pipelineRun != nil {
		for _, dest := range pipelineRun.Destinations {
			if dest.Destination == pbplugin.DestinationType_DESTINATION_GITHUB && dest.ExternalId != nil && *dest.ExternalId != "" {
				existingFilePath = *dest.ExternalId
				break
			}
		}
	}

	if existingFilePath == "" {
		return fmt.Errorf("no GitHub destination found in pipeline run")
	}

	existingSHA, existingContent, err := u.getFileContent(ctx, ghClient, config, existingFilePath)
	if err != nil {
		logger.Warn("Failed to fetch existing file for UPDATE", "error", err, "path", existingFilePath)
		existingSHA = nil
		existingContent = ""
	}

	payloadDesc := payload.Metadata["description"]
	if existingContent != "" && payloadDesc != "" {
		sectionHeader := ""
		for key, val := range payload.Metadata {
			if strings.HasPrefix(key, "section_header_") {
				sectionHeader = val
				break
			}
		}

		if sectionHeader != "" && description.HasSection(payloadDesc, sectionHeader) {
			newSectionContent := description.ExtractSection(payloadDesc, sectionHeader)
			if newSectionContent != "" {
				payload.Metadata["description"] = description.ReplaceSection(existingContent, sectionHeader, newSectionContent)
			}
		}
	}

	activityName := payload.Metadata["activity_name"]
	if activityName == "" {
		activityName = "Activity"
	}

	fitFileName := ""
	if fitFileUri, ok := payload.Metadata["fit_file_uri"]; ok && fitFileUri != "" {
		logger.Info("FIT file URI found, downloading for GitHub update commit", "uri", fitFileUri)
		fitData, fitErr := u.downloadFitFile(ctx, fitFileUri)
		if fitErr != nil {
			logger.Warn("Failed to download FIT file for update, continuing without it", "error", fitErr)
		} else {
			fitFileName = "activity.fit"
			fitPath := path.Join(path.Dir(existingFilePath), fitFileName)
			existingFitSHA, _, _ := u.getFileContent(ctx, ghClient, config, fitPath)
			fitCommitMsg := fmt.Sprintf("Update FIT data for %s", activityName)
			if _, fitCommitErr := u.createOrUpdateBinaryFile(ctx, ghClient, config, fitPath, fitData, fitCommitMsg, existingFitSHA); fitCommitErr != nil {
				logger.Warn("Failed to commit FIT file during update", "error", fitCommitErr)
				fitFileName = ""
			} else {
				logger.Info("Updated FIT file in GitHub", "path", fitPath, "size", len(fitData))
			}
		}
	} else {
		logger.Info("No FIT file URI in payload metadata, skipping FIT update commit")
	}

	markdownContent := buildMarkdownContent(payload, activityName, fitFileName)
	if existingContent != "" {
		markdownContent = mergeWithUserContent(markdownContent, existingContent)
	}

	commitMessage := fmt.Sprintf("Update %s", activityName)
	_, err = u.createOrUpdateFile(ctx, ghClient, config, existingFilePath, markdownContent, commitMessage, existingSHA)
	if err != nil {
		return fmt.Errorf("GitHub update failed: %w", err)
	}

	_ = u.svc.DB.IncrementSyncCount(ctx, payload.UserId)

	return nil
}

func buildMarkdownContent(payload *pbevents.ActivityPayload, activityName, fitFileName string) string {
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("title: %q\n", activityName))

	activityTypeStr := payload.Metadata["activity_type"]
	activityTypeStr = strings.TrimPrefix(activityTypeStr, "ACTIVITY_TYPE_")
	sb.WriteString(fmt.Sprintf("type: %s\n", activityTypeStr))

	if payload.Timestamp != nil {
		sb.WriteString(fmt.Sprintf("date: %s\n", payload.Timestamp.AsTime().Format("2006-01-02T15:04:05Z07:00")))
	}

	sb.WriteString(fmt.Sprintf("source: %s\n", payload.Source.String()))
	sb.WriteString(fmt.Sprintf("activity_id: %s\n", payload.GetActivityId()))
	sb.WriteString(fmt.Sprintf("pipeline_id: %s\n", payload.GetPipelineId()))

	if fitFileName != "" {
		sb.WriteString(fmt.Sprintf("fit_file: %s\n", fitFileName))
	}

	appliedEnrichmentsStr := payload.Metadata["applied_enrichments"]
	if appliedEnrichmentsStr != "" {
		sb.WriteString(fmt.Sprintf("enrichments: [%s]\n", appliedEnrichmentsStr))
	}

	tagsStr := payload.Metadata["tags"]
	if tagsStr != "" {
		sb.WriteString(fmt.Sprintf("tags: [%s]\n", tagsStr))
	}
	sb.WriteString("---\n\n")

	sb.WriteString(fmt.Sprintf("# %s\n\n", activityName))

	description := payload.Metadata["description"]
	if description != "" {
		sb.WriteString(description)
		sb.WriteString("\n")
	}

	sb.WriteString("\n<!-- fitglue:end -->\n")

	return sb.String()
}

func buildFilePath(folder string, activityName string, activityDate time.Time) string {
	dateStr := activityDate.Format("2006-01-02")
	safeName := sanitizeFileName(activityName)
	return fmt.Sprintf("%s%s/%s/%s-%s/activity.md",
		folder,
		activityDate.Format("2006"),
		activityDate.Format("01"),
		dateStr,
		safeName,
	)
}

func sanitizeFileName(name string) string {
	lower := strings.ToLower(name)
	replacer := strings.NewReplacer(
		" ", "-", "/", "-", "\\", "-", ":", "-",
		"'", "", "\"", "", "(", "", ")", "",
		".", "-", ",", "",
	)
	result := replacer.Replace(lower)
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return strings.Trim(result, "-")
}

func mergeWithUserContent(newContent, existingContent string) string {
	marker := "<!-- fitglue:end -->"
	idx := strings.Index(existingContent, marker)
	if idx == -1 {
		return newContent
	}

	userContent := existingContent[idx+len(marker):]
	if strings.TrimSpace(userContent) == "" {
		return newContent
	}

	newIdx := strings.Index(newContent, marker)
	if newIdx == -1 {
		return newContent + "\n" + marker + userContent
	}

	return newContent[:newIdx+len(marker)] + userContent
}

func gitHubHeaders(ctx context.Context, req *http.Request) error {
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "FitGlue/1.0")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return nil
}

func (u *Uploader) createOrUpdateFile(ctx context.Context, ghClient *ghclient.ClientWithResponses, config *gitHubConfig, filePath, content, message string, existingSHA *string) (string, error) {
	committerName := "FitGlue Bot"
	committerEmail := "bot@fitglue.com"

	reqBody := ghclient.CreateOrUpdateFileRequest{
		Message: message,
		Content: base64.StdEncoding.EncodeToString([]byte(content)),
		Sha:     existingSHA,
		Committer: &ghclient.CommitAuthor{
			Name:  committerName,
			Email: committerEmail,
		},
	}

	resp, err := ghClient.ReposcreateOrUpdateFileContentsWithResponse(ctx,
		config.Owner, config.Name, filePath, reqBody, gitHubHeaders)
	if err != nil {
		return "", fmt.Errorf("GitHub API request failed: %w", err)
	}

	if resp.StatusCode() >= 400 {
		return "", fmt.Errorf("GitHub API error: status %d, body: %s", resp.StatusCode(), string(resp.Body))
	}

	return filePath, nil
}

func (u *Uploader) createOrUpdateBinaryFile(ctx context.Context, ghClient *ghclient.ClientWithResponses, config *gitHubConfig, filePath string, data []byte, message string, existingSHA *string) (string, error) {
	committerName := "FitGlue Bot"
	committerEmail := "bot@fitglue.com"

	reqBody := ghclient.CreateOrUpdateFileRequest{
		Message: message,
		Content: base64.StdEncoding.EncodeToString(data),
		Sha:     existingSHA,
		Committer: &ghclient.CommitAuthor{
			Name:  committerName,
			Email: committerEmail,
		},
	}

	resp, err := ghClient.ReposcreateOrUpdateFileContentsWithResponse(ctx,
		config.Owner, config.Name, filePath, reqBody, gitHubHeaders)
	if err != nil {
		return "", fmt.Errorf("GitHub API request failed: %w", err)
	}

	if resp.StatusCode() >= 400 {
		return "", fmt.Errorf("GitHub API error: status %d, body: %s", resp.StatusCode(), string(resp.Body))
	}

	return filePath, nil
}

func (u *Uploader) downloadFitFile(ctx context.Context, fitFileUri string) ([]byte, error) {
	bucketName, objectName, ok := activityPkg.ParseGCSURI(fitFileUri)
	if !ok {
		return nil, fmt.Errorf("invalid FIT file URI: %q", fitFileUri)
	}

	data, err := u.svc.Store.Get(ctx, bucketName, objectName)
	if err != nil {
		return nil, fmt.Errorf("GCS read error for FIT file: %w", err)
	}
	return data, nil
}

func (u *Uploader) getFileContent(ctx context.Context, ghClient *ghclient.ClientWithResponses, config *gitHubConfig, filePath string) (*string, string, error) {
	resp, err := ghClient.ReposgetContentWithResponse(ctx,
		config.Owner, config.Name, filePath, nil, gitHubHeaders)
	if err != nil {
		return nil, "", fmt.Errorf("GitHub API request failed: %w", err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return nil, "", nil
	}
	if resp.StatusCode() >= 400 {
		return nil, "", fmt.Errorf("GitHub API error: status %d, body: %s", resp.StatusCode(), string(resp.Body))
	}

	if resp.JSON200 == nil {
		return nil, "", fmt.Errorf("unexpected nil response body")
	}

	file := resp.JSON200
	if file.Content == nil {
		return &file.Sha, "", nil
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(*file.Content, "\n", ""))
	if err != nil {
		return &file.Sha, "", fmt.Errorf("failed to decode file content: %w", err)
	}

	return &file.Sha, string(decoded), nil
}
