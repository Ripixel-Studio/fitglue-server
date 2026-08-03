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
// and existing descriptions are reconciled block-by-block (see mergeBlocks).
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

	return mergeBlocks(existingDescription, payloadDescription)
}

// mergeBlocks reconciles two FitGlue-authored descriptions at the granularity of
// "\n\n"-delimited blocks rather than whole strings.
//
// FitGlue composes the payload description from ordered blocks: the source's original
// upload text in slot 0, one block per enricher section, then the branding footer (see
// buildDescriptionFromSlots in the enricher orchestrator). The destination's remote
// description is that same block set from an earlier pass — but a non-blocking pending
// input that had not yet resolved on that pass leaves its slot empty, so its block is
// MISSING remotely and lands in the MIDDLE of the payload once it resolves. The remote
// description may also carry extra text the user typed on the destination platform that
// FitGlue never authored.
//
// The previous implementation compared whole strings: it only skipped the re-append when
// one description wholly contained the other. A freshly-resolved section inserted mid-way
// shifts every block after it (e.g. the branding footer), so neither string contains the
// other and the entire payload was appended — duplicating the whole description on every
// non-blocking resume. Reconciling by block fixes this: any remote block also present in
// the payload is dropped (the payload re-supplies it in the right place), and only remote
// blocks FitGlue did NOT author are preserved, ahead of the payload.
//
// Byte-equality alone is not enough for sections that REGENERATE between runs (e.g. the
// AI summary rewrites itself once more data is available, effort/streak/HR numbers change
// as inputs resolve). The regenerated remote block is no longer byte-identical to the
// payload's, so it fell through the equality check and was preserved as if it were user
// text — leaving a stale copy of the section at the top of the description. So we also
// match by section header: a remote block whose header (see blockHeaderKey) the payload
// re-supplies is dropped, letting the payload's fresh version stand in its place.
func mergeBlocks(existingDescription, payloadDescription string) string {
	payloadBlocks := splitBlocks(payloadDescription)
	inPayload := make(map[string]bool, len(payloadBlocks))
	payloadHeaders := make(map[string]bool, len(payloadBlocks))
	for _, b := range payloadBlocks {
		inPayload[b] = true
		if h := blockHeaderKey(b); h != "" {
			payloadHeaders[h] = true
		}
	}

	var preserved []string
	for _, b := range splitBlocks(existingDescription) {
		if inPayload[b] {
			continue // payload re-supplies this exact block in the right place
		}
		if h := blockHeaderKey(b); h != "" && payloadHeaders[h] {
			continue // payload re-supplies this section (content regenerated) in the right place
		}
		preserved = append(preserved, b)
	}

	if len(preserved) == 0 {
		return payloadDescription
	}
	return strings.Join(append(preserved, payloadDescription), "\n\n")
}

// blockHeaderKey returns the stable, content-independent identity of a FitGlue section
// block, or "" if the block is not a recognisable section. FitGlue sections open with an
// emoji/symbol and a "Header:" label on their first line — either alone ("✨ AI Summary:")
// or inline with a value ("🔥 Calories: 303 kcal"). The key is that label up to and
// including the first colon, so a section still matches after its content regenerates.
// Blocks without this shape (the user's own upload text, the "Posted via FitGlue" footer,
// free text typed on the destination) return "" and are matched only by exact content.
func blockHeaderKey(block string) string {
	firstLine := block
	if idx := strings.IndexByte(block, '\n'); idx != -1 {
		firstLine = block[:idx]
	}
	firstLine = strings.TrimSpace(firstLine)
	if !isEmojiOrSpecialStart(firstLine) {
		return ""
	}
	colon := strings.IndexByte(firstLine, ':')
	if colon == -1 {
		return ""
	}
	return firstLine[:colon+1]
}

// splitBlocks splits a description into its "\n\n"-delimited blocks, trimming surrounding
// whitespace from each and dropping empties. Trimming keeps block matching stable against
// the minor whitespace normalisation some destinations apply when they store and return a
// description.
func splitBlocks(s string) []string {
	raw := strings.Split(s, "\n\n")
	blocks := make([]string, 0, len(raw))
	for _, b := range raw {
		if t := strings.TrimSpace(b); t != "" {
			blocks = append(blocks, t)
		}
	}
	return blocks
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
