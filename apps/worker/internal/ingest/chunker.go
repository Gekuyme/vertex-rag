package ingest

import (
	"strings"

	"github.com/Gekuyme/vertex-rag/apps/worker/internal/store"
)

const (
	defaultChunkSize    = 1200
	defaultChunkOverlap = 180
)

func chunkText(text string) []store.ChunkInput {
	cleanText := normalizeText(text)
	if cleanText == "" {
		return nil
	}

	runes := []rune(cleanText)
	chunks := make([]store.ChunkInput, 0)
	start := 0
	index := 0

	for start < len(runes) {
		end := start + defaultChunkSize
		if end > len(runes) {
			end = len(runes)
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
		start = nextStart
	}

	return chunks
}

func normalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimSpace(line)
	}
	text = strings.Join(lines, "\n")

	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(text)
}
