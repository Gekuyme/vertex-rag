package ingest

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/Gekuyme/vertex-rag/apps/worker/internal/store"
)

const (
	defaultChunkSize    = 1200
	defaultChunkOverlap = 180
)

type normalizeOptions struct {
	PreserveIndent bool
}

var markdownHeadingRE = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

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

func snapEndParagraph(runes []rune, start int, end int) int {
	if end >= len(runes) {
		end = len(runes)
	}
	if end <= start {
		return start
	}

	const lookback = 320
	const minLen = 260
	minCut := start + minLen
	if minCut > end {
		return end
	}
	low := end - lookback
	if low < minCut {
		low = minCut
	}

	for i := end - 2; i >= low; i-- {
		if runes[i] == '\n' && runes[i+1] == '\n' {
			if i > start {
				return i
			}
			break
		}
	}

	return end
}

type pageBoundary struct {
	Page  int
	Start int // rune offset (inclusive) in normalizedText
	End   int // rune offset (exclusive) in normalizedText
}

func chunkDocumentText(rawText, mime, filename string) []store.ChunkInput {
	lowerMIME := strings.ToLower(strings.TrimSpace(mime))
	extension := strings.ToLower(filepathExt(filename))

	isPDF := lowerMIME == "application/pdf" || extension == ".pdf"
	isMarkdown := strings.Contains(lowerMIME, "markdown") || extension == ".md"
	normalizeOpts := normalizeOptions{PreserveIndent: isPDF}

	normalizedText, boundaries := normalizeWithPageBoundaries(rawText, isPDF, normalizeOpts)
	if strings.TrimSpace(normalizedText) == "" {
		return nil
	}

	runes := []rune(normalizedText)
	var headings []headingBoundary
	if isMarkdown {
		headings = extractMarkdownHeadings(normalizedText)
	}
	chunks := make([]store.ChunkInput, 0)
	start := snapStart(runes, 0)
	index := 0

	for start < len(runes) {
		end := start + defaultChunkSize
		if end > len(runes) {
			end = len(runes)
		}
		end = snapEnd(runes, start, end)
		paragraphEnd := snapEndParagraph(runes, start, end)
		if paragraphEnd > start {
			end = paragraphEnd
		}
		if end <= start {
			break
		}

		chunkRunes := runes[start:end]
		chunkContent := strings.TrimSpace(string(chunkRunes))
		if chunkContent != "" {
			metadata := map[string]any{
				"char_start": start,
				"char_end":   end,
			}

			if len(boundaries) > 0 {
				startPage, endPage := resolvePages(boundaries, start, end)
				if startPage > 0 {
					metadata["page"] = startPage
				}
				if endPage > 0 && endPage != startPage {
					metadata["page_end"] = endPage
				}
			}
			if len(headings) > 0 {
				if section := markdownHeadingForOffset(headings, start); section != "" {
					metadata["section"] = section
				}
			}

			chunks = append(chunks, store.ChunkInput{
				Index:    index,
				Content:  chunkContent,
				Metadata: metadata,
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

func filepathExt(filename string) string {
	if filename == "" {
		return ""
	}
	if idx := strings.LastIndexByte(filename, '.'); idx >= 0 {
		return filename[idx:]
	}
	return ""
}

func normalizeWithPageBoundaries(raw string, isPDF bool, opts normalizeOptions) (string, []pageBoundary) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	// PDFs extracted via pdftotext commonly contain form-feed (\f) page breaks.
	if isPDF && strings.Contains(raw, "\f") {
		parts := strings.Split(raw, "\f")
		boundaries := make([]pageBoundary, 0, len(parts))
		runeCursor := 0
		var builder strings.Builder

		for i, part := range parts {
			pageText := normalizeText(part, opts)
			if strings.TrimSpace(pageText) == "" {
				continue
			}
			if builder.Len() > 0 {
				// Separate pages by blank line to preserve structure.
				builder.WriteString("\n\n")
				runeCursor += 2
			}

			start := runeCursor
			pageRunes := []rune(pageText)
			builder.WriteString(pageText)
			runeCursor += len(pageRunes)
			end := runeCursor

			boundaries = append(boundaries, pageBoundary{
				Page:  i + 1,
				Start: start,
				End:   end,
			})
		}

		normalized := strings.TrimSpace(builder.String())
		return normalized, boundaries
	}

	return normalizeText(raw, opts), nil
}

func resolvePages(boundaries []pageBoundary, start, end int) (int, int) {
	if len(boundaries) == 0 {
		return 0, 0
	}
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}

	startPage := pageForOffset(boundaries, start)
	// Use end-1 when possible to avoid attributing exact boundary to next page.
	endOffset := end
	if endOffset > start {
		endOffset = endOffset - 1
	}
	endPage := pageForOffset(boundaries, endOffset)
	return startPage, endPage
}

func pageForOffset(boundaries []pageBoundary, offset int) int {
	if len(boundaries) == 0 {
		return 0
	}
	if offset < 0 {
		offset = 0
	}

	index := sort.Search(len(boundaries), func(i int) bool {
		return boundaries[i].End > offset
	})
	if index >= len(boundaries) {
		return boundaries[len(boundaries)-1].Page
	}
	if offset < boundaries[index].Start {
		return boundaries[index].Page
	}
	return boundaries[index].Page
}

type headingBoundary struct {
	Start int
	Title string
}

func extractMarkdownHeadings(text string) []headingBoundary {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	lines := strings.Split(text, "\n")
	out := make([]headingBoundary, 0)
	runeCursor := 0

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if match := markdownHeadingRE.FindStringSubmatch(line); len(match) == 3 {
			title := strings.TrimSpace(match[2])
			if title != "" {
				out = append(out, headingBoundary{
					Start: runeCursor,
					Title: title,
				})
			}
		}
		// +1 for the newline rune.
		runeCursor += len([]rune(rawLine)) + 1
	}

	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

func markdownHeadingForOffset(headings []headingBoundary, offset int) string {
	if len(headings) == 0 {
		return ""
	}
	if offset < 0 {
		offset = 0
	}

	index := sort.Search(len(headings), func(i int) bool {
		return headings[i].Start > offset
	})
	if index <= 0 {
		return headings[0].Title
	}
	return headings[index-1].Title
}

func normalizeText(text string, opts normalizeOptions) string {
	// Normalize line endings first.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\u00ad\n", "")
	text = strings.ReplaceAll(text, "\u00ad", "")
	text = strings.ReplaceAll(text, "\u200b", "")
	text = strings.ReplaceAll(text, "\ufeff", "")
	text = strings.ReplaceAll(text, "\u00a0", " ")

	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))

	inFence := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Toggle fenced blocks (markdown-ish).
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			out = append(out, trimmed)
			continue
		}

		if trimmed == "" {
			// Keep at most one blank line.
			if len(out) == 0 || out[len(out)-1] == "" {
				continue
			}
			out = append(out, "")
			continue
		}

		if inFence {
			// Preserve indentation inside code fences.
			out = append(out, strings.TrimRightFunc(line, unicode.IsSpace))
			continue
		}

		// For layout-heavy sources (like PDFs), keep indentation.
		if opts.PreserveIndent {
			out = append(out, strings.TrimRightFunc(line, unicode.IsSpace))
		} else {
			// Preserve code-block-by-indentation (4 spaces or tab).
			if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
				out = append(out, strings.TrimRightFunc(line, unicode.IsSpace))
			} else {
				out = append(out, strings.TrimSpace(line))
			}
		}
	}

	// Now join wrapped lines into paragraphs, but keep obvious structure (headings/lists/tables).
	joined := make([]string, 0, len(out))
	for i := 0; i < len(out); i++ {
		line := out[i]
		if line == "" {
			if len(joined) == 0 || joined[len(joined)-1] == "" {
				continue
			}
			joined = append(joined, "")
			continue
		}

		if len(joined) == 0 || joined[len(joined)-1] == "" {
			joined = append(joined, line)
			continue
		}

		prev := joined[len(joined)-1]
		if shouldPreserveLineBreak(prev, line) {
			joined = append(joined, line)
			continue
		}

		// Join hyphenated line breaks: "exam-\nple" -> "example".
		if endsWithHyphenatedWord(prev) && startsWithWord(line) {
			joined[len(joined)-1] = strings.TrimRight(prev, "-") + strings.TrimLeft(line, " \t")
			continue
		}

		joined[len(joined)-1] = prev + " " + strings.TrimLeft(line, " \t")
	}

	normalized := strings.TrimSpace(strings.Join(joined, "\n"))

	// Join hyphenated line breaks that survived normalization.
	hyphenBreak := regexp.MustCompile(`([\\p{L}])-\n([\\p{L}])`)
	normalized = hyphenBreak.ReplaceAllString(normalized, `$1$2`)

	// Collapse excessive blank lines.
	for strings.Contains(normalized, "\n\n\n") {
		normalized = strings.ReplaceAll(normalized, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(normalized)
}

func shouldPreserveLineBreak(prevLine, nextLine string) bool {
	prev := strings.TrimSpace(prevLine)
	next := strings.TrimSpace(nextLine)
	if prev == "" || next == "" {
		return true
	}

	// Markdown headings, list items, blockquotes, tables.
	if strings.HasPrefix(prev, "#") || strings.HasPrefix(next, "#") {
		return true
	}
	if strings.HasPrefix(prev, ">") || strings.HasPrefix(next, ">") {
		return true
	}
	if looksLikeListItem(prev) || looksLikeListItem(next) {
		return true
	}
	if strings.Contains(prev, "|") || strings.Contains(next, "|") {
		return true
	}
	// Keep lines that look like columnar layout.
	if strings.Contains(prevLine, "   ") || strings.Contains(nextLine, "   ") {
		return true
	}

	return false
}

func looksLikeListItem(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
		return true
	}
	// Ordered list: "1. ..."
	if len(trimmed) >= 3 && unicode.IsDigit(rune(trimmed[0])) {
		if idx := strings.IndexByte(trimmed, '.'); idx == 1 || idx == 2 {
			return idx+1 < len(trimmed) && trimmed[idx+1] == ' '
		}
	}
	return false
}

func endsWithHyphenatedWord(line string) bool {
	trimmed := strings.TrimRightFunc(line, unicode.IsSpace)
	if len(trimmed) == 0 {
		return false
	}
	if !strings.HasSuffix(trimmed, "-") {
		return false
	}
	runes := []rune(trimmed)
	if len(runes) < 2 {
		return false
	}
	return isWordRune(runes[len(runes)-2])
}

func startsWithWord(line string) bool {
	trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
	if trimmed == "" {
		return false
	}
	runes := []rune(trimmed)
	return isWordRune(runes[0])
}
