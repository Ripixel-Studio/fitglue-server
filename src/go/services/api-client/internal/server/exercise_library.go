package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	hevyapi "github.com/fitglue/server/src/go/pkg/api/hevy"
	"github.com/fitglue/server/src/go/pkg/infrastructure/oauth"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
)

type exerciseLibraryEntry struct {
	Name          string `json:"name"`
	Category      string `json:"category"`
	PrimaryMuscle string `json:"primary_muscle"`
	Source        string `json:"source"`
}

func (s *APIServer) handleGetExerciseLibrary(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	intRes, err := s.userService.GetIntegration(r.Context(), &userpb.GetIntegrationRequest{
		UserId:   token.UID,
		Provider: "hevy",
	})
	if err != nil || intRes.GetIntegrations().GetHevy() == nil || intRes.GetIntegrations().GetHevy().GetApiKey() == "" {
		WriteJSON(w, map[string]interface{}{"exercises": []exerciseLibraryEntry{}})
		return
	}

	apiKey := intRes.GetIntegrations().GetHevy().GetApiKey()
	entries, err := fetchHevyExerciseLibrary(r.Context(), apiKey, q)
	if err != nil {
		s.logger.Error(r.Context(), "failed to fetch Hevy exercise library", "error", err)
		WriteJSON(w, map[string]interface{}{"exercises": []exerciseLibraryEntry{}})
		return
	}

	WriteJSON(w, map[string]interface{}{"exercises": entries})
}

func fetchHevyExerciseLibrary(ctx context.Context, apiKey, q string) ([]exerciseLibraryEntry, error) {
	httpClient := oauth.NewClientWithErrorLogging(slog.Default(), "hevy", 30*time.Second)

	var all []exerciseLibraryEntry
	page := 1
	pageSize := 100

	for {
		url := fmt.Sprintf("https://api.hevyapp.com/v1/exercise_templates?page=%d&page_size=%d", page, pageSize)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("api-key", apiKey)

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("API request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			var buf bytes.Buffer
			buf.ReadFrom(resp.Body)
			return nil, fmt.Errorf("Hevy API error (%d): %s", resp.StatusCode, buf.String())
		}

		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}

		var result struct {
			ExerciseTemplates []hevyapi.ExerciseTemplate `json:"exercise_templates"`
			Page              int                        `json:"page"`
			PageCount         int                        `json:"page_count"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}

		for _, t := range result.ExerciseTemplates {
			if t.Title == nil {
				continue
			}
			name := *t.Title
			if q != "" && !strings.Contains(strings.ToLower(name), q) {
				continue
			}
			entry := exerciseLibraryEntry{
				Name:   name,
				Source: "hevy",
			}
			if t.PrimaryMuscleGroup != nil {
				entry.PrimaryMuscle = *t.PrimaryMuscleGroup
				entry.Category = muscleGroupToCategory(*t.PrimaryMuscleGroup)
			}
			all = append(all, entry)
		}

		if page >= result.PageCount || len(result.ExerciseTemplates) == 0 {
			break
		}
		page++
	}

	return all, nil
}

// muscleGroupToCategory converts Hevy muscle group strings to display category names
func muscleGroupToCategory(mg string) string {
	switch mg {
	case "chest":
		return "Chest"
	case "back", "lower_back":
		return "Back"
	case "shoulders":
		return "Shoulders"
	case "biceps", "triceps", "forearm":
		return "Arms"
	case "quadriceps", "hamstrings", "glutes", "calves":
		return "Legs"
	case "abdominals":
		return "Core"
	case "full_body":
		return "Full Body"
	case "cardio":
		return "Cardio"
	default:
		return "Other"
	}
}
