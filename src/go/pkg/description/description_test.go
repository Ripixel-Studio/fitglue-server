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
			// Two enrichers each declare a section. Both are present remotely (written as
			// placeholders at Create time). A resume must replace BOTH in place without
			// duplicating the base line or the branding footer. The previous
			// implementation replaced only one map-iteration-order-dependent section and,
			// when it happened to pick a section not yet present, appended the whole
			// payload — the source of the cross-source duplication.
			name:     "multiple present sections are all replaced without duplication",
			existing: "🏃 Base\n\n🏃 Parkrun Results:\nWaiting...\n\n💧 hDrop Sweat Analysis:\nPending...\n\nPosted via FitGlue 💪",
			payload:  "🏃 Base\n\n🏃 Parkrun Results:\n42nd place, 23:45\n\n💧 hDrop Sweat Analysis:\n2.1L lost\n\nPosted via FitGlue 💪",
			metadata: map[string]string{
				"section_header_parkrun": "🏃 Parkrun Results:",
				"section_header_hdrop":   "💧 hDrop Sweat Analysis:",
			},
			expected: "🏃 Base\n\n🏃 Parkrun Results:\n42nd place, 23:45\n\n💧 hDrop Sweat Analysis:\n2.1L lost\n\nPosted via FitGlue 💪",
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

// TestMergeDescription_MultiSectionResumeDoesNotDuplicate is a regression test for
// cross-source duplication where a pipeline declares more than one replaceable section.
// The previous implementation only handled a single, map-iteration-order-dependent
// section header, so a resume could re-append the entire description. This drives two
// sequential resumes (one section resolving at a time) and asserts nothing accumulates.
func TestMergeDescription_MultiSectionResumeDoesNotDuplicate(t *testing.T) {
	metadata := map[string]string{
		"section_header_parkrun": "🏃 Parkrun Results:",
		"section_header_hdrop":   "💧 hDrop Sweat Analysis:",
	}
	remote := "🏃 Base\n\n🏃 Parkrun Results:\nWaiting...\n\n💧 hDrop Sweat Analysis:\nPending...\n\nPosted via FitGlue 💪"

	// First resume: parkrun resolves, hdrop still pending in the recomputed payload.
	payload1 := "🏃 Base\n\n🏃 Parkrun Results:\n42nd place\n\n💧 hDrop Sweat Analysis:\nPending...\n\nPosted via FitGlue 💪"
	remote = MergeDescription(remote, payload1, metadata)
	want1 := "🏃 Base\n\n🏃 Parkrun Results:\n42nd place\n\n💧 hDrop Sweat Analysis:\nPending...\n\nPosted via FitGlue 💪"
	if remote != want1 {
		t.Fatalf("after first resume = %q, want %q", remote, want1)
	}

	// Second resume: hdrop now resolves too. Both sections must be current, with the
	// base line and branding footer each appearing exactly once.
	payload2 := "🏃 Base\n\n🏃 Parkrun Results:\n42nd place\n\n💧 hDrop Sweat Analysis:\n2.1L lost\n\nPosted via FitGlue 💪"
	remote = MergeDescription(remote, payload2, metadata)
	if remote != payload2 {
		t.Fatalf("after second resume = %q, want %q", remote, payload2)
	}

	// A redundant resume with identical payload must be a no-op.
	remote = MergeDescription(remote, payload2, metadata)
	if remote != payload2 {
		t.Fatalf("redundant resume duplicated content, got %q", remote)
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
