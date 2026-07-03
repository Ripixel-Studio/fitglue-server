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
// a replaceable section (a payloadMetadata entry keyed "section_header_*") and that
// section already exists remotely, only that section is replaced. Otherwise the payload
// description is appended — unless it's already present verbatim in the existing
// description, in which case the existing description is returned unchanged.
//
// The "already present" guard exists because a resumed pipeline (e.g. resolving an
// unrelated non-blocking pending input) recomputes the same final description and calls
// Update() again. Without it, every such resume blindly re-appends the whole
// description, producing duplicated content in destinations that don't have a matching
// section header configured.
func MergeDescription(existingDescription, payloadDescription string, payloadMetadata map[string]string) string {
	if payloadDescription == "" {
		return existingDescription
	}

	sectionHeader := ""
	for key, val := range payloadMetadata {
		if strings.HasPrefix(key, "section_header_") {
			sectionHeader = val
			break
		}
	}

	if sectionHeader != "" && HasSection(existingDescription, sectionHeader) {
		newSectionContent := ExtractSection(payloadDescription, sectionHeader)
		if newSectionContent != "" {
			return ReplaceSection(existingDescription, sectionHeader, newSectionContent)
		}
		return existingDescription
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
