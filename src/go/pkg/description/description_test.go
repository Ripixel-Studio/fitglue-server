package description

import (
	"testing"
)

func TestHasSection(t *testing.T) {
	tests := []struct {
		name         string
		description  string
		headerPrefix string
		expected     bool
	}{
		{
			name:         "Section found at start",
			description:  "🏃 Parkrun Results:\nWaiting for results...",
			headerPrefix: "🏃 Parkrun Results:",
			expected:     true,
		},
		{
			name:         "Section found in middle",
			description:  "Original\n\n🏃 Parkrun Results:\nWaiting...\n\n❤️ Heart Rate:",
			headerPrefix: "🏃 Parkrun Results:",
			expected:     true,
		},
		{
			name:         "Section not found",
			description:  "Some description without the section",
			headerPrefix: "🏃 Parkrun Results:",
			expected:     false,
		},
		{
			name:         "Empty description",
			description:  "",
			headerPrefix: "🏃 Parkrun Results:",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasSection(tt.description, tt.headerPrefix)
			if result != tt.expected {
				t.Errorf("HasSection() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestReplaceSection(t *testing.T) {
	tests := []struct {
		name         string
		description  string
		headerPrefix string
		newContent   string
		expected     string
	}{
		{
			name:         "Replace section at start",
			description:  "🏃 Parkrun Results:\nWaiting for results...",
			headerPrefix: "🏃 Parkrun Results:",
			newContent:   "🏃 Parkrun Results:\n42nd place, 23:45",
			expected:     "🏃 Parkrun Results:\n42nd place, 23:45",
		},
		{
			name:         "Replace section with content before",
			description:  "Original description\n\n🏃 Parkrun Results:\nWaiting for results...",
			headerPrefix: "🏃 Parkrun Results:",
			newContent:   "🏃 Parkrun Results:\n42nd place, 23:45",
			expected:     "Original description\n\n🏃 Parkrun Results:\n42nd place, 23:45",
		},
		{
			name:         "Replace section with content after",
			description:  "🏃 Parkrun Results:\nWaiting...\n\n❤️ Heart Rate:\n150 bpm avg",
			headerPrefix: "🏃 Parkrun Results:",
			newContent:   "🏃 Parkrun Results:\n42nd place",
			expected:     "🏃 Parkrun Results:\n42nd place\n\n❤️ Heart Rate:\n150 bpm avg",
		},
		{
			name:         "Section not found - append",
			description:  "Some description",
			headerPrefix: "🏃 Parkrun Results:",
			newContent:   "🏃 Parkrun Results:\n42nd place",
			expected:     "Some description\n\n🏃 Parkrun Results:\n42nd place",
		},
		{
			name:         "Empty description - set",
			description:  "",
			headerPrefix: "🏃 Parkrun Results:",
			newContent:   "🏃 Parkrun Results:\n42nd place",
			expected:     "🏃 Parkrun Results:\n42nd place",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReplaceSection(tt.description, tt.headerPrefix, tt.newContent)
			if result != tt.expected {
				t.Errorf("ReplaceSection() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractSection(t *testing.T) {
	tests := []struct {
		name         string
		description  string
		headerPrefix string
		expected     string
	}{
		{
			name:         "Extract section at start",
			description:  "🏃 Parkrun Results:\nWaiting for results...",
			headerPrefix: "🏃 Parkrun Results:",
			expected:     "🏃 Parkrun Results:\nWaiting for results...",
		},
		{
			name:         "Extract section from middle",
			description:  "Original description\n\n🏃 Parkrun Results:\n42nd place, 23:45\n\n❤️ Heart Rate:\n150 bpm avg",
			headerPrefix: "🏃 Parkrun Results:",
			expected:     "🏃 Parkrun Results:\n42nd place, 23:45",
		},
		{
			name:         "Section not found",
			description:  "Some description without the section",
			headerPrefix: "🏃 Parkrun Results:",
			expected:     "",
		},
		{
			name:         "Empty description",
			description:  "",
			headerPrefix: "🏃 Parkrun Results:",
			expected:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractSection(tt.description, tt.headerPrefix)
			if result != tt.expected {
				t.Errorf("ExtractSection() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMergeDescription(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		payload  string
		metadata map[string]string
		expected string
	}{
		{
			name:     "empty payload leaves existing untouched",
			existing: "Some existing description",
			payload:  "",
			expected: "Some existing description",
		},
		{
			name:     "empty existing is set to payload",
			existing: "",
			payload:  "New description",
			expected: "New description",
		},
		{
			name:     "no section header, new content appended",
			existing: "Original description",
			payload:  "🏃 Parkrun Results:\n42nd place",
			expected: "Original description\n\n🏃 Parkrun Results:\n42nd place",
		},
		{
			name:     "no section header, payload already present is not re-appended",
			existing: "Original description\n\nPosted via FitGlue 💪",
			payload:  "Original description\n\nPosted via FitGlue 💪",
			expected: "Original description\n\nPosted via FitGlue 💪",
		},
		{
			name:     "matching section header replaces only that section",
			existing: "Original\n\n🏃 Parkrun Results:\nWaiting...\n\n❤️ Heart Rate:\n150 bpm",
			payload:  "🏃 Parkrun Results:\n42nd place, 23:45",
			metadata: map[string]string{"section_header_parkrun": "🏃 Parkrun Results:"},
			expected: "Original\n\n🏃 Parkrun Results:\n42nd place, 23:45\n\n❤️ Heart Rate:\n150 bpm",
		},
		{
			name:     "section header set but not present in existing falls back to append",
			existing: "Original description",
			payload:  "🏃 Parkrun Results:\n42nd place",
			metadata: map[string]string{"section_header_parkrun": "🏃 Parkrun Results:"},
			expected: "Original description\n\n🏃 Parkrun Results:\n42nd place",
		},
		{
			// Cross-source update where the destination still holds only the user's
			// original upload text. The payload carries that same text at its head
			// followed by enrichment sections, so we replace over it rather than
			// appending a duplicate copy of the user's words.
			name:     "existing user description carried in payload is replaced not duplicated",
			existing: "Great run today!",
			payload:  "Great run today!\n\n🏃 Parkrun Results:\n42nd place",
			expected: "Great run today!\n\n🏃 Parkrun Results:\n42nd place",
		},
		{
			// The destination holds text FitGlue didn't author (not present in the
			// payload), so it must be preserved by appending the payload after it.
			name:     "unrelated existing description is preserved by appending",
			existing: "Manually written on Strava",
			payload:  "🏃 Parkrun Results:\n42nd place",
			expected: "Manually written on Strava\n\n🏃 Parkrun Results:\n42nd place",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeDescription(tt.existing, tt.payload, tt.metadata)
			if result != tt.expected {
				t.Errorf("MergeDescription() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestMergeDescription_ResumeDoesNotDuplicate is a regression test for a bug where
// resolving a non-blocking pending input (or any resume that recomputes an identical
// final description) caused destinations without a configured section header to
// duplicate the entire description block on every resume.
func TestMergeDescription_ResumeDoesNotDuplicate(t *testing.T) {
	remoteDescription := "🏃 My workout\n\nPosted via FitGlue 💪"
	payloadDescription := "🏃 My workout\n\nPosted via FitGlue 💪"

	// First resume: identical content is already present remotely - no duplication.
	merged := MergeDescription(remoteDescription, payloadDescription, nil)
	if merged != remoteDescription {
		t.Fatalf("expected no change on repeated identical content, got %q", merged)
	}

	// A second resume with the same (still-identical) payload must also not duplicate,
	// simulating multiple non-blocking pending inputs resolving in sequence.
	merged = MergeDescription(merged, payloadDescription, nil)
	if merged != remoteDescription {
		t.Fatalf("description duplicated across repeated resumes, got %q", merged)
	}
}

// TestMergeDescription_NonBlockingResolveInsertsMidBlock is a regression test for the
// reported bug: an activity from a non-Strava source runs a pipeline with Strava as a
// destination and a non-blocking pending input. The non-blocking enricher's description
// block is absent on the first upload (its slot is empty while it waits) and lands in the
// MIDDLE of the recomputed description once the input resolves — shifting the branding
// footer down. Whole-string containment missed this and appended the entire description
// again; block-level reconciliation must instead replace in place.
func TestMergeDescription_NonBlockingResolveInsertsMidBlock(t *testing.T) {
	// First Strava upload: the non-blocking section is still pending, so only the
	// original text, one resolved section, and the branding footer are present.
	remote := "Morning run\n\n🏃 Pace: 5:12/km\n\nPosted via FitGlue 💪"

	// The non-blocking input resolves; the recomputed description inserts its block
	// between the pace section and the footer.
	payload := "Morning run\n\n🏃 Pace: 5:12/km\n\n📸 Photos: 3 added\n\nPosted via FitGlue 💪"

	merged := MergeDescription(remote, payload, nil)
	if merged != payload {
		t.Fatalf("mid-block insert should replace in place, not append.\n got: %q\nwant: %q", merged, payload)
	}

	// A second non-blocking input resolves and inserts another mid-block section. This
	// must not re-append the earlier copy either.
	payload2 := "Morning run\n\n🏃 Pace: 5:12/km\n\n📸 Photos: 3 added\n\n💧 hDrop: 0.8L\n\nPosted via FitGlue 💪"
	merged2 := MergeDescription(merged, payload2, nil)
	if merged2 != payload2 {
		t.Fatalf("second mid-block insert duplicated the description.\n got: %q\nwant: %q", merged2, payload2)
	}
}

// TestMergeDescription_PreservesUnrelatedRemoteText ensures the block merge still keeps
// text the user typed on the destination platform (a block FitGlue never authored) while
// dropping the blocks the payload re-supplies.
func TestMergeDescription_PreservesUnrelatedRemoteText(t *testing.T) {
	remote := "Hand-written on Strava\n\n🏃 Pace: 5:12/km\n\nPosted via FitGlue 💪"
	payload := "🏃 Pace: 5:12/km\n\n📸 Photos: 3 added\n\nPosted via FitGlue 💪"

	merged := MergeDescription(remote, payload, nil)
	want := "Hand-written on Strava\n\n" + payload
	if merged != want {
		t.Fatalf("unrelated remote text not preserved.\n got: %q\nwant: %q", merged, want)
	}
}

// TestMergeDescription_RegeneratedSectionNotDuplicated is a regression test for the
// reported bug: a section that regenerates with DIFFERENT content between runs (e.g. the
// AI summary rewrites itself once more data is available) is no longer byte-identical to
// its remote copy. Whole-block equality treated the stale remote copy as user text and
// preserved it at the top of the description, leaving two copies of the section. Header
// matching must instead drop the stale copy so the payload's fresh version stands alone.
func TestMergeDescription_RegeneratedSectionNotDuplicated(t *testing.T) {
	// Remote holds an earlier AI summary above the user's own upload text.
	remote := "✨ AI Summary:\nAn early guess with little data.\n\n" +
		"The Monday morning double done 💪\n\n" +
		"🔥 Calories: 250 kcal ≈ 0.9 slice of pizza 🍕\n\n" +
		"Posted via FitGlue 💪"

	// The pipeline reruns with more data: the AI summary is rewritten and the calorie
	// figure changes. The payload is the full recomputed description.
	payload := "The Monday morning double done 💪\n\n" +
		"✨ AI Summary:\nA solid 52-minute Tower Pilates session with controlled heart rate.\n\n" +
		"🔥 Calories: 303 kcal ≈ 1.1 slice of pizza 🍕\n\n" +
		"Posted via FitGlue 💪"

	merged := MergeDescription(remote, payload, nil)
	if merged != payload {
		t.Fatalf("regenerated sections should replace in place, not duplicate.\n got: %q\nwant: %q", merged, payload)
	}
}

// TestMergeDescription_HeaderMatchPreservesUserText ensures header matching does not
// clobber free text the user typed on the destination — even when that text happens to
// contain a colon — because such text does not open with an emoji/symbol section header.
func TestMergeDescription_HeaderMatchPreservesUserText(t *testing.T) {
	remote := "Note: felt strong today\n\n✨ AI Summary:\nOld summary.\n\nPosted via FitGlue 💪"
	payload := "✨ AI Summary:\nFresh summary.\n\nPosted via FitGlue 💪"

	merged := MergeDescription(remote, payload, nil)
	want := "Note: felt strong today\n\n" + payload
	if merged != want {
		t.Fatalf("user free text should be preserved.\n got: %q\nwant: %q", merged, want)
	}
}

func TestBlockHeaderKey(t *testing.T) {
	tests := []struct {
		name  string
		block string
		want  string
	}{
		{"header line only", "✨ AI Summary:\nsome content", "✨ AI Summary:"},
		{"header inline with value", "🔥 Calories: 303 kcal ≈ 1.1 slice 🍕", "🔥 Calories:"},
		{"emoji start no colon", "🏃 My workout", ""},
		{"plain text with colon", "Note: felt strong", ""},
		{"branding footer", "Posted via FitGlue 💪", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := blockHeaderKey(tt.block); got != tt.want {
				t.Errorf("blockHeaderKey(%q) = %q, want %q", tt.block, got, tt.want)
			}
		})
	}
}

func TestRemoveSection(t *testing.T) {
	tests := []struct {
		name         string
		description  string
		headerPrefix string
		expected     string
	}{
		{
			name:         "Remove only section",
			description:  "🏃 Parkrun Results:\nWaiting for results...",
			headerPrefix: "🏃 Parkrun Results:",
			expected:     "",
		},
		{
			name:         "Remove section with content before",
			description:  "Original description\n\n🏃 Parkrun Results:\nWaiting...",
			headerPrefix: "🏃 Parkrun Results:",
			expected:     "Original description",
		},
		{
			name:         "Remove section with content after",
			description:  "🏃 Parkrun Results:\nWaiting...\n\n❤️ Heart Rate:\n150 bpm",
			headerPrefix: "🏃 Parkrun Results:",
			expected:     "❤️ Heart Rate:\n150 bpm",
		},
		{
			name:         "Section not found - no change",
			description:  "Some description",
			headerPrefix: "🏃 Parkrun Results:",
			expected:     "Some description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveSection(tt.description, tt.headerPrefix)
			if result != tt.expected {
				t.Errorf("RemoveSection() = %q, want %q", result, tt.expected)
			}
		})
	}
}
