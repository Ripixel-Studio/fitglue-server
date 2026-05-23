// nolint:proto-json
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

type Weather struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewWeather())
}

func NewWeather() *Weather {
	return &Weather{}
}

func (p *Weather) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *Weather) Name() string {
	return "weather"
}

func (p *Weather) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_WEATHER
}

func (p *Weather) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	// Extract GPS coordinates from first record
	var latitude, longitude float64
	var hasGPS bool

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.PositionLat != 0 && record.PositionLong != 0 {
					latitude = record.PositionLat
					longitude = record.PositionLong
					hasGPS = true
					break
				}
			}
			if hasGPS {
				break
			}
		}
		if hasGPS {
			break
		}
	}

	if !hasGPS {
		logger.Info("No GPS data found for weather enricher, skipping")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"weather_status": "skipped",
				"status_detail":  "No GPS data found",
			},
		}, nil
	}

	// Extract activity start time
	startTime := activity.StartTime.AsTime()
	dateStr := startTime.Format("2006-01-02")

	// Call Open-Meteo API
	url := fmt.Sprintf(
		"https://archive-api.open-meteo.com/v1/archive?latitude=%.6f&longitude=%.6f&start_date=%s&end_date=%s&hourly=temperature_2m,weathercode,windspeed_10m,winddirection_10m",
		latitude, longitude, dateStr, dateStr,
	)

	logger.Info("Fetching weather data",
		"latitude", latitude,
		"longitude", longitude,
		"date", dateStr,
		"url", url,
	)

	resp, err := http.Get(url)
	if err != nil {
		logger.Error("Failed to fetch weather data", "error", err)
		if doNotRetry {
			return &providers.EnrichmentResult{
				Metadata: map[string]string{
					"weather_status": "skipped",
					"status_detail":  "Weather API request failed (lag exhausted)",
				},
			}, nil
		}
		return nil, &providers.RetryableError{Err: fmt.Errorf("weather API request failed: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("Weather API returned non-200 status", "status", resp.StatusCode, "body", string(body))
		// Only retry on transient server/rate-limit errors; client errors (e.g. 400 for recent dates) are permanent
		isTransient := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		if isTransient && !doNotRetry {
			return nil, &providers.RetryableError{Err: fmt.Errorf("weather API returned status %d", resp.StatusCode)}
		}
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"weather_status": "skipped",
				"status_detail":  fmt.Sprintf("Weather API returned status %d", resp.StatusCode),
			},
		}, nil
	}

	// Parse response
	var weatherResp OpenMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&weatherResp); err != nil {
		logger.Error("Failed to decode weather response", "error", err)
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"weather_status": "skipped",
				"status_detail":  "Failed to parse weather response",
			},
		}, nil
	}

	// Find closest hourly data to activity start time
	closestIdx := findClosestHourIndex(weatherResp.Hourly.Time, startTime)
	if closestIdx == -1 || closestIdx >= len(weatherResp.Hourly.Temperature) {
		logger.Warn("No weather data found for activity time")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"weather_status": "skipped",
				"status_detail":  "No weather data for activity time",
			},
		}, nil
	}

	// Extract weather data
	temperature := weatherResp.Hourly.Temperature[closestIdx]
	weatherCode := weatherResp.Hourly.WeatherCode[closestIdx]
	windSpeed := weatherResp.Hourly.WindSpeed[closestIdx]
	windDirection := weatherResp.Hourly.WindDirection[closestIdx]

	// Map weather code to description
	weatherDesc := mapWeatherCode(weatherCode)

	// Map wind direction to cardinal
	windCardinal := mapWindDirection(windDirection)

	// Check if wind should be included
	includeWind := inputs["include_wind"] != "false" // Default to true

	// Format summary
	var summaryText string
	if includeWind {
		summaryText = fmt.Sprintf("🌤️ Weather: %.0f°C, %s • Wind: %.0f km/h %s",
			temperature, weatherDesc, windSpeed, windCardinal)
	} else {
		summaryText = fmt.Sprintf("🌤️ Weather: %.0f°C, %s",
			temperature, weatherDesc)
	}

	logger.Info("Weather summary generated",
		"temperature", temperature,
		"weather_code", weatherCode,
		"weather_desc", weatherDesc,
		"wind_speed", windSpeed,
		"wind_direction", windCardinal,
	)

	// Append to existing description
	newDescription := summaryText

	return &providers.EnrichmentResult{
		Description: newDescription,
		Metadata: map[string]string{
			"weather_status":      "success",
			"temperature":         fmt.Sprintf("%.0f", temperature),
			"weather_code":        fmt.Sprintf("%d", weatherCode),
			"weather_description": weatherDesc,
			"wind_speed":          fmt.Sprintf("%.0f", windSpeed),
			"wind_direction":      windCardinal,
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			Weather: &pbactivity.WeatherSummary{
				TempC:              temperature,
				WeatherDescription: weatherDesc,
				WindSpeedKph:       windSpeed,
				WindDirection:      windCardinal,
				WeatherCode:        int32(weatherCode),
			},
		},
	}, nil
}

// OpenMeteoResponse represents the API response structure
type OpenMeteoResponse struct {
	Hourly struct {
		Time          []string  `json:"time"`
		Temperature   []float64 `json:"temperature_2m"`
		WeatherCode   []int     `json:"weathercode"`
		WindSpeed     []float64 `json:"windspeed_10m"`
		WindDirection []float64 `json:"winddirection_10m"`
	} `json:"hourly"`
}

// findClosestHourIndex finds the index of the hour closest to the target time
func findClosestHourIndex(times []string, target time.Time) int {
	if len(times) == 0 {
		return -1
	}

	minDiff := time.Duration(math.MaxInt64)
	closestIdx := -1

	for i, timeStr := range times {
		t, err := time.Parse("2006-01-02T15:04", timeStr)
		if err != nil {
			continue
		}

		diff := target.Sub(t)
		if diff < 0 {
			diff = -diff
		}

		if diff < minDiff {
			minDiff = diff
			closestIdx = i
		}
	}

	return closestIdx
}

// mapWeatherCode maps WMO weather codes to human-readable descriptions
func mapWeatherCode(code int) string {
	switch {
	case code == 0:
		return "Clear"
	case code >= 1 && code <= 3:
		return "Partly Cloudy"
	case code >= 45 && code <= 48:
		return "Fog"
	case code >= 51 && code <= 67:
		return "Rain"
	case code >= 71 && code <= 77:
		return "Snow"
	case code >= 95 && code <= 99:
		return "Thunderstorm"
	default:
		return "Unknown"
	}
}

// mapWindDirection converts degrees to cardinal direction
func mapWindDirection(degrees float64) string {
	// Normalize to 0-360
	degrees = math.Mod(degrees, 360)
	if degrees < 0 {
		degrees += 360
	}

	// 8-point compass rose
	directions := []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	index := int(math.Round(degrees/45.0)) % 8
	return directions[index]
}
