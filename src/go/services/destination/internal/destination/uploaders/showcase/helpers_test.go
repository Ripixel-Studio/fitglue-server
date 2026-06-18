package showcase

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Morning Run", "morning-run"},
		{"Leg_Day", "leg-day"},
		{"Hello, World!", "hello-world"},
		{"  spaced  out  ", "spaced-out"},
		{"Multiple---Dashes", "multiple-dashes"},
		{"UPPER CASE", "upper-case"},
		{"émojis 🏃 stripped", "mojis-stripped"},
		{"---", ""},
		{"", ""},
		{"123 numbers", "123-numbers"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, slugify(tc.in))
		})
	}
}

func TestSlugify_TruncatesTo30(t *testing.T) {
	long := "this-is-a-very-long-activity-title-that-exceeds-thirty"
	got := slugify(long)
	assert.LessOrEqual(t, len(got), 30)
	// no trailing dash after truncation
	assert.NotEqual(t, byte('-'), got[len(got)-1])
}

func TestGenerateRandomSuffix(t *testing.T) {
	s := generateRandomSuffix()
	// 3 bytes hex-encoded = 6 chars
	assert.Len(t, s, 6)
	_, err := hex.DecodeString(s)
	require.NoError(t, err)

	// Highly likely to differ across calls.
	other := generateRandomSuffix()
	assert.NotEqual(t, s, other)
}
