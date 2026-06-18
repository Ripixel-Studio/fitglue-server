package hdrop

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessHDropData_Empty(t *testing.T) {
	result, err := processHDropData("")
	require.NoError(t, err)
	assert.Equal(t, "no_data", result.Metadata["hdrop_status"])
	assert.Nil(t, result.Enrichments)
}

func TestProcessHDropData_InvalidJSON(t *testing.T) {
	_, err := processHDropData("{not json}")
	assert.Error(t, err)
}

// TestProcessHDropData_StringNumericFields covers the real hDrop export format where
// sweatRate and sodiumConcentration are JSON strings rather than numbers.
func TestProcessHDropData_StringNumericFields(t *testing.T) {
	raw := `{
		"metadata": {
			"totalSweatLoss": 1.618,
			"sweatRate": "1.26",
			"totalSodium": 1250.568,
			"totalPotassium": 163.118,
			"sodiumConcentration": "772.9",
			"averagehDropScore": 76.08,
			"minhDropScore": 39,
			"bodyLocation": "Upper Arm (Default)",
			"minTemperature": 27.04,
			"maxTemperature": 43.74
		},
		"timeseriesData": []
	}`

	result, err := processHDropData(raw)
	require.NoError(t, err)
	assert.Equal(t, "applied", result.Metadata["hdrop_status"])

	s := result.Enrichments.Hdrop
	assert.InDelta(t, 1.618, s.TotalFluidLossL, 0.001)
	assert.InDelta(t, 1.26, s.SweatRateLPerHr, 0.001)
	assert.InDelta(t, 772.9, s.SodiumConcentrationMgPerL, 0.1)
	assert.Equal(t, "Upper Arm (Default)", s.BodyLocation)
}

// TestProcessHDropData_NullTimeseriesValues covers warm-up data points where
// sweatRate and fluidLoss are JSON null before the sensor detects sweat.
func TestProcessHDropData_NullTimeseriesValues(t *testing.T) {
	raw := `{
		"metadata": {
			"totalSweatLoss": 0.5,
			"sweatRate": "0.8",
			"totalSodium": 400.0,
			"totalPotassium": 50.0,
			"sodiumConcentration": "600.0",
			"averagehDropScore": 70.0,
			"minhDropScore": 50.0,
			"bodyLocation": "Upper Arm (Default)",
			"minTemperature": 28.0,
			"maxTemperature": 40.0
		},
		"timeseriesData": [
			{"timeMinutes": 0, "sweatRate": null, "fluidLoss": null, "temperature": 28.0, "sodiumConcentration": 55.0},
			{"timeMinutes": 3, "sweatRate": null, "fluidLoss": null, "temperature": 30.0, "sodiumConcentration": 55.0},
			{"timeMinutes": 6, "sweatRate": 0.34, "fluidLoss": 0.017, "temperature": 33.0, "sodiumConcentration": 55.0}
		]
	}`

	result, err := processHDropData(raw)
	require.NoError(t, err)

	pts := result.Enrichments.Hdrop.Timeseries
	require.Len(t, pts, 3)
	assert.Equal(t, float64(0), pts[0].SweatRate)
	assert.Equal(t, float64(0), pts[0].FluidLossCumulative)
	assert.Equal(t, float64(0), pts[1].SweatRate)
	assert.InDelta(t, 0.34, pts[2].SweatRate, 0.001)
	assert.InDelta(t, 0.017, pts[2].FluidLossCumulative, 0.001)
}

func TestParseFloatString(t *testing.T) {
	assert.InDelta(t, 1.26, parseFloatString("1.26"), 0.001)
	assert.InDelta(t, 772.9, parseFloatString("772.9"), 0.1)
	assert.Equal(t, float64(0), parseFloatString(""))
}

func TestDerefF64(t *testing.T) {
	v := 3.14
	assert.Equal(t, 3.14, derefF64(&v))
	assert.Equal(t, float64(0), derefF64(nil))
}
