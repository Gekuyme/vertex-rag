package ingest

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/Gekuyme/vertex-rag/apps/worker/internal/store"
)

const (
	defaultChunkSize    = 1200
	defaultChunkOverlap = 180
)

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func snapStart(runes []rune, start int) int {
	if start <= 0 {
		return 0
	}
	if start >= len(runes) {
		return len(runes)
	}

	// If we start in the middle of a token, move to the next boundary.
	if isWordRune(runes[start]) && isWordRune(runes[start-1]) {
		for start < len(runes) && isWordRune(runes[start]) {
			start++
		}
	}
	for start < len(runes) && unicode.IsSpace(runes[start]) {
		start++
	}
	return start
}

func snapEnd(runes []rune, start int, end int) int {
	if end >= len(runes) {
		return len(runes)
	}
	if end <= start {
		return start
	}

	// Prefer to end on whitespace near the boundary.
	const lookback = 80
	const minLen = 300
	minCut := start + minLen
	if minCut > end {
		minCut = start
	}
	low := end - lookback
	if low < minCut {
		low = minCut
	}
	for i := end - 1; i >= low; i-- {
		if unicode.IsSpace(runes[i]) {
			return i
		}
	}

	// If we end in the middle of a token, extend slightly to finish the token.
	if end < len(runes) && end-1 >= 0 && isWordRune(runes[end-1]) && isWordRune(runes[end]) {
		limit := end + 50
		if limit > len(runes) {
			limit = len(runes)
		}
		for end < limit && isWordRune(runes[end]) {
			end++
		}
	}

	return end
}

func chunkText(text string) []store.ChunkInput {
	cleanText := normalizeText(text)
	if cleanText == "" {
		return nil
	}

	runes := []rune(cleanText)
	chunks := make([]store.ChunkInput, 0)
	start := snapStart(runes, 0)
	index := 0

	for start < len(runes) {
		end := start + defaultChunkSize
		if end > len(runes) {
			end = len(runes)
		}
		end = snapEnd(runes, start, end)
		if end <= start {
			break
		}

		chunkRunes := runes[start:end]
		chunkContent := strings.TrimSpace(string(chunkRunes))
		if chunkContent != "" {
			chunks = append(chunks, store.ChunkInput{
				Index:   index,
				Content: chunkContent,
				Metadata: map[string]any{
					"char_start": start,
					"char_end":   end,
				},
			})
			index++
		}

		if end == len(runes) {
			break
		}

		nextStart := end - defaultChunkOverlap
		if nextStart <= start {
			nextStart = end
		}
		start = snapStart(runes, nextStart)
	}

	return chunks
}

func normalizeText(text string) string {
	// Normalize line endings first.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimSpace(line)
	}
	text = strings.Join(lines, "\n")

	// Join hyphenated line breaks (common in PDFs).
	hyphenBreak := regexp.MustCompile(`([\\p{L}])-\n([\\p{L}])`)
	text = hyphenBreak.ReplaceAllString(text, `$1$2`)

	// Convert single newlines inside paragraphs to spaces, keep blank lines as paragraph breaks.
	singleNL := regexp.MustCompile(`([^\n])\n([^\n])`)
	text = singleNL.ReplaceAllString(text, `$1 $2`)

	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(text)
}
