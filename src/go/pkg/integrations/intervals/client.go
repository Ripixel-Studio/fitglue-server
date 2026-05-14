package intervals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	baseURL = "https://intervals.icu/api/v1"
)

// VerifyCredentials checks that the given API key and athlete ID form a valid
// Intervals.icu credential pair by fetching the athlete's own profile.
// Returns an error with a user-friendly message if the credentials are invalid.
func VerifyCredentials(ctx context.Context, apiKey, athleteID string) error {
	url := fmt.Sprintf("%s/athlete/%s", baseURL, athleteID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth(apiKey, "")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach Intervals.icu: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("invalid API key or athlete ID")
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("athlete ID %q not found", athleteID)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// Client is an API client for Intervals.icu
type Client struct {
	apiKey    string
	athleteID string
	client    *http.Client
}

// NewClient creates a new Intervals.icu API client
func NewClient(apiKey, athleteID string) *Client {
	return &Client{
		apiKey:    apiKey,
		athleteID: athleteID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Activity represents an Intervals.icu activity
type Activity struct {
	ID                 int64   `json:"id,omitempty"`
	StartDateLocal     string  `json:"start_date_local"` // ISO 8601 format
	Type               string  `json:"type"`
	Name               string  `json:"name"`
	Description        string  `json:"description,omitempty"`
	Distance           float64 `json:"distance,omitempty"`             // meters
	MovingTime         int     `json:"moving_time,omitempty"`          // seconds
	ElapsedTime        int     `json:"elapsed_time,omitempty"`         // seconds
	TotalElevationGain float64 `json:"total_elevation_gain,omitempty"` // meters

	// Performance metrics
	AvgHR      int     `json:"average_hr,omitempty"`
	MaxHR      int     `json:"max_hr,omitempty"`
	AvgWatts   int     `json:"average_watts,omitempty"`
	MaxWatts   int     `json:"max_watts,omitempty"`
	AvgCadence int     `json:"average_cadence,omitempty"`
	MaxCadence int     `json:"max_cadence,omitempty"`
	AvgSpeed   float64 `json:"average_speed,omitempty"` // m/s
	MaxSpeed   float64 `json:"max_speed,omitempty"`     // m/s

	// GPS data
	StartLat float64 `json:"start_lat,omitempty"`
	StartLng float64 `json:"start_lng,omitempty"`

	// File attachment
	FileID string `json:"file_id,omitempty"` // ID of uploaded FIT file
}

// ListActivitiesParams are parameters for listing activities
type ListActivitiesParams struct {
	Oldest string // ISO 8601 date
	Newest string // ISO 8601 date
}

// doRequest performs an HTTP request with Basic Auth
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	url := fmt.Sprintf("%s/athlete/%s%s", baseURL, c.athleteID, path)
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Basic Auth with API key as username, no password
	req.SetBasicAuth(c.apiKey, "")

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}

// ListActivities retrieves activities for the athlete
func (c *Client) ListActivities(ctx context.Context, params ListActivitiesParams) ([]Activity, error) {
	path := "/activities"
	if params.Oldest != "" || params.Newest != "" {
		path += "?"
		if params.Oldest != "" {
			path += fmt.Sprintf("oldest=%s&", params.Oldest)
		}
		if params.Newest != "" {
			path += fmt.Sprintf("newest=%s", params.Newest)
		}
	}

	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var activities []Activity
	if err := json.NewDecoder(resp.Body).Decode(&activities); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return activities, nil
}

// GetActivity retrieves a single activity by ID
func (c *Client) GetActivity(ctx context.Context, activityID int64) (*Activity, error) {
	path := fmt.Sprintf("/activities/%d", activityID)

	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var activity Activity
	if err := json.NewDecoder(resp.Body).Decode(&activity); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &activity, nil
}

// CreateActivity creates a new activity
func (c *Client) CreateActivity(ctx context.Context, activity *Activity) (*Activity, error) {
	resp, err := c.doRequest(ctx, "POST", "/activities", activity)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var created Activity
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &created, nil
}

// UpdateActivity updates an existing activity
func (c *Client) UpdateActivity(ctx context.Context, activityID int64, activity *Activity) (*Activity, error) {
	path := fmt.Sprintf("/activities/%d", activityID)

	resp, err := c.doRequest(ctx, "PUT", path, activity)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var updated Activity
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &updated, nil
}

// DownloadFITFile downloads the raw FIT file for an activity by ID.
func (c *Client) DownloadFITFile(ctx context.Context, activityID int64) ([]byte, error) {
	url := fmt.Sprintf("%s/athlete/%s/activities/%d.fit", baseURL, c.athleteID, activityID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth(c.apiKey, "")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download error %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// UploadFITFile uploads a FIT file and returns the file ID
func (c *Client) UploadFITFile(ctx context.Context, fitData []byte) (string, error) {
	url := fmt.Sprintf("%s/athlete/%s/activities", baseURL, c.athleteID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(fitData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.SetBasicAuth(c.apiKey, "")
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result Activity
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return fmt.Sprintf("%d", result.ID), nil
}
