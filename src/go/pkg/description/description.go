// Package description provides utilities for section-based description manipulation.
// Sections are identified by header prefixes (typically emoji + text, e.g., "🏃 Parkrun Results:").
// This enables enrichers to define replaceable sections that can be updated during resume flows
// instead of being blindly appended.
package description

import (
	"strings"
	"unicode"
)

// isEmojiOrSpecialStart checks if a string starts with an emoji or special character.
// This is used to detect section boundaries.
func isEmojiOrSpecialStart(s string) bool {
	if len(s) == 0 {
		return false
	}
	r := []rune(s)
	if len(r) == 0 {
		return false
	}
	// Check for common emoji ranges and symbols
	first := r[0]
	return first > 127 || // Non-ASCII (likely emoji or special char)
		unicode.IsSymbol(first) ||
		unicode.In(first, unicode.So) // Symbol, other
}

// FindSection locates a section by its header prefix in a description.
// Returns start index, end index (exclusive), and whether found.
// A section ends at: (a) a blank line followed by an emoji/symbol start, OR (b) end of string.
func FindSection(description, headerPrefix string) (start, end int, found bool) {
	if description == "" || headerPrefix == "" {
		return 0, 0, false
	}

	// Find the start of the section
	start = strings.Index(description, headerPrefix)
	if start == -1 {
		return 0, 0, false
	}

	// Find the end of the section
	// Look for a blank line followed by a line starting with emoji/symbol
	searchFrom := start + len(headerPrefix)
	remaining := description[searchFrom:]

	// Split into lines to find section boundary
	lines := strings.Split(remaining, "\n")
	position := searchFrom

	for i, line := range lines {
		position += len(line)
		if i < len(lines)-1 {
			position++ // Account for newline
		}

		// Check for blank line followed by emoji start (section boundary)
		if strings.TrimSpace(line) == "" && i+1 < len(lines) {
			nextLine := lines[i+1]
			if isEmojiOrSpecialStart(strings.TrimSpace(nextLine)) {
				// Found section boundary - end is at the blank line
				end = start + len(headerPrefix) + strings.Index(remaining, "\n"+nextLine) - 1
				// Trim trailing whitespace from section
				for end > start && (description[end-1] == '\n' || description[end-1] == ' ') {
					end--
				}
				return start, end, true
			}
		}
	}

	// No boundary found - section extends to end of string
	end = len(description)
	// Trim trailing whitespace
	for end > start && (description[end-1] == '\n' || description[end-1] == ' ') {
		end--
	}
	return start, end, true
}

// HasSection checks if a description contains a section with the given header.
func HasSection(description, headerPrefix string) bool {
	_, _, found := FindSection(description, headerPrefix)
	return found
}

// ReplaceSection replaces a section (from header to next section or EOF) with new content.
// If the section doesn't exist, the new content is appended.
func ReplaceSection(description, headerPrefix, newContent string) string {
	start, end, found := FindSection(description, headerPrefix)
	if !found {
		// Section not found - append
		if description != "" {
			return description + "\n\n" + newContent
		}
		return newContent
	}

	// Build result: before + new content + after
	before := description[:start]
	after := description[end:]

	// Clean up spacing
	before = strings.TrimRight(before, "\n ")
	after = strings.TrimLeft(after, "\n ")

	var result strings.Builder
	if before != "" {
		result.WriteString(before)
		result.WriteString("\n\n")
	}
	result.WriteString(newContent)
	if after != "" {
		result.WriteString("\n\n")
		result.WriteString(after)
	}

	return result.String()
}

// ExtractSection extracts the content of a section (from header to next section or EOF).
// Returns the section content if found, empty string otherwise.
func ExtractSection(description, headerPrefix string) string {
	start, end, found := FindSection(description, headerPrefix)
	if !found {
		return ""
	}
	return strings.TrimSpace(description[start:end])
}

// MergeDescription merges a freshly-computed payload description into a destination's
// existing remote description for a cross-source Update() call. If the pipeline declared
// any replaceable sections (payloadMetadata entries keyed "section_header_*") that already
// exist remotely, each of those sections is replaced in place with its payload version.
// Otherwise the payload description is appended — unless it's already present verbatim in
// the existing description, in which case the existing description is returned unchanged.
//
// A cross-source Update() recomputes the WHOLE final description on every pass, so blindly
// appending it re-adds content already present remotely. Two guards prevent this:
//
//   - Section-managed merge: enrichers declaring a section header write a placeholder
//     section at Create time, so those headers are present remotely by the time Update()
//     runs. Every declared section that is present is replaced in place; the surrounding
//     content (and any section not owned by this pipeline) is left untouched, and the
//     payload is never appended wholesale. Handling *all* declared headers — rather than a
//     single, map-iteration-order-dependent one — is what stops a second section from
//     being duplicated on resume.
//   - Verbatim guard: for pipelines with no declared sections (e.g. only branding's
//     "Posted via FitGlue" footer), a resume that recomputes an identical description is a
//     no-op rather than a re-append.
func MergeDescription(existingDescription, payloadDescription string, payloadMetadata map[string]string) string {
	if payloadDescription == "" {
		return existingDescription
	}

	var sectionHeaders []string
	for key, val := range payloadMetadata {
		if strings.HasPrefix(key, "section_header_") && val != "" {
			sectionHeaders = append(sectionHeaders, val)
		}
	}

	// If any declared section is already present remotely, treat this as a section-managed
	// merge: replace each present section in place and never append the payload wholesale.
	merged := existingDescription
	sectionManaged := false
	for _, header := range sectionHeaders {
		if !HasSection(merged, header) {
			continue
		}
		sectionManaged = true
		newSectionContent := ExtractSection(payloadDescription, header)
		if newSectionContent == "" {
			continue
		}
		merged = ReplaceSection(merged, header, newSectionContent)
	}
	if sectionManaged {
		return merged
	}

	if existingDescription == "" {
		return payloadDescription
	}
	if strings.Contains(existingDescription, payloadDescription) {
		return existingDescription
	}
	return existingDescription + "\n\n" + payloadDescription
}

// RemoveSection removes a section entirely from the description.
func RemoveSection(description, headerPrefix string) string {
	start, end, found := FindSection(description, headerPrefix)
	if !found {
		return description
	}

	before := description[:start]
	after := description[end:]

	// Clean up spacing
	before = strings.TrimRight(before, "\n ")
	after = strings.TrimLeft(after, "\n ")

	if before == "" {
		return after
	}
	if after == "" {
		return before
	}
	return before + "\n\n" + after
}
