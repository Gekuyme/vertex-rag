package ingest

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/Gekuyme/vertex-rag/apps/worker/internal/store"
)

const (
	defaultParentSectionSize = 1600
	defaultChildChunkSize    = 320
	defaultChildOverlap      = 60
)

type normalizeOptions struct {
	PreserveIndent bool
}

var markdownHeadingRE = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
var sentenceSplitRE = regexp.MustCompile(`[.!?]\s+`)

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

type paragraphSpan struct {
	Start       int
	End         int
	Text        string
	HeadingPath string
	StartPage   int
	EndPage     int
}

func chunkDocumentText(rawText, mime, filename string) store.ChunkPlan {
	lowerMIME := strings.ToLower(strings.TrimSpace(mime))
	extension := strings.ToLower(filepathExt(filename))

	isPDF := lowerMIME == "application/pdf" || extension == ".pdf"
	isMarkdown := strings.Contains(lowerMIME, "markdown") || extension == ".md"
	normalizeOpts := normalizeOptions{PreserveIndent: isPDF}

	normalizedText, boundaries := normalizeWithPageBoundaries(rawText, isPDF, normalizeOpts)
	if strings.TrimSpace(normalizedText) == "" {
		return store.ChunkPlan{}
	}

	var headings []headingBoundary
	if isMarkdown {
		headings = extractMarkdownHeadings(normalizedText)
	}

	paragraphs := extractParagraphSpans(normalizedText, headings, boundaries)
	if len(paragraphs) == 0 {
		return store.ChunkPlan{}
	}

	plan := store.ChunkPlan{
		Sections: make([]store.SectionInput, 0),
		Chunks:   make([]store.ChunkInput, 0),
	}

	currentSection := make([]paragraphSpan, 0)
	appendSection := func() {
		if len(currentSection) == 0 {
			return
		}

		sectionIndex := len(plan.Sections)
		section := buildSectionInput(sectionIndex, currentSection)
		plan.Sections = append(plan.Sections, section)
		childChunks := buildChildChunksForSection(section, currentSection)
		for index := range childChunks {
			childChunks[index].Index = len(plan.Chunks) + index
		}
		plan.Chunks = append(plan.Chunks, childChunks...)
		currentSection = currentSection[:0]
	}

	currentHeading := paragraphs[0].HeadingPath
	currentSize := 0
	for _, paragraph := range paragraphs {
		paragraphLen := len([]rune(paragraph.Text))
		headingChanged := currentHeading != "" && paragraph.HeadingPath != "" && paragraph.HeadingPath != currentHeading
		if len(currentSection) > 0 && (headingChanged || (currentSize >= 900 && currentSize+paragraphLen > defaultParentSectionSize)) {
			appendSection()
			currentHeading = paragraph.HeadingPath
			currentSize = 0
		}

		if currentHeading == "" {
			currentHeading = paragraph.HeadingPath
		}
		currentSection = append(currentSection, paragraph)
		currentSize += paragraphLen
	}
	appendSection()

	return plan
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

func extractParagraphSpans(text string, headings []headingBoundary, boundaries []pageBoundary) []paragraphSpan {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	runes := []rune(text)
	out := make([]paragraphSpan, 0)
	start := 0
	for start < len(runes) {
		for start < len(runes) && unicode.IsSpace(runes[start]) {
			start++
		}
		if start >= len(runes) {
			break
		}

		end := start
		for end < len(runes) {
			if end+1 < len(runes) && runes[end] == '\n' && runes[end+1] == '\n' {
				break
			}
			end++
		}
		if end <= start {
			break
		}

		paragraphText := strings.TrimSpace(string(runes[start:end]))
		if paragraphText != "" {
			startPage, endPage := resolvePages(boundaries, start, end)
			out = append(out, paragraphSpan{
				Start:       start,
				End:         end,
				Text:        paragraphText,
				HeadingPath: markdownHeadingForOffset(headings, start),
				StartPage:   startPage,
				EndPage:     endPage,
			})
		}

		start = end + 2
	}

	return out
}

func buildSectionInput(index int, paragraphs []paragraphSpan) store.SectionInput {
	start := paragraphs[0].Start
	end := paragraphs[len(paragraphs)-1].End
	startPage := paragraphs[0].StartPage
	endPage := paragraphs[len(paragraphs)-1].EndPage
	headingPath := strings.TrimSpace(paragraphs[len(paragraphs)-1].HeadingPath)
	if headingPath == "" {
		headingPath = strings.TrimSpace(paragraphs[0].HeadingPath)
	}

	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		lines = append(lines, paragraph.Text)
	}
	content := strings.TrimSpace(strings.Join(lines, "\n\n"))
	metadata := map[string]any{
		"char_start": start,
		"char_end":   end,
	}
	if headingPath != "" {
		metadata["section"] = headingPath
		metadata["heading_path"] = headingPath
	}
	if startPage > 0 {
		metadata["page"] = startPage
	}
	if endPage > 0 && endPage != startPage {
		metadata["page_end"] = endPage
	}

	return store.SectionInput{
		Index:       index,
		HeadingPath: headingPath,
		Content:     content,
		Metadata:    metadata,
	}
}

func buildChildChunksForSection(section store.SectionInput, paragraphs []paragraphSpan) []store.ChunkInput {
	if strings.TrimSpace(section.Content) == "" {
		return nil
	}

	runes := []rune(section.Content)
	baseStart, _ := metadataIntFromMap(section.Metadata, "char_start")
	basePage, _ := metadataIntFromMap(section.Metadata, "page")
	basePageEnd, _ := metadataIntFromMap(section.Metadata, "page_end")
	headingPath, _ := section.Metadata["heading_path"].(string)

	chunks := make([]store.ChunkInput, 0)
	start := snapStart(runes, 0)
	for start < len(runes) {
		end := start + defaultChildChunkSize
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

		content := strings.TrimSpace(string(runes[start:end]))
		if content != "" {
			metadata := map[string]any{
				"char_start":   baseStart + start,
				"char_end":     baseStart + end,
				"chunk_kind":   classifyChunkKind(content),
				"parent_index": section.Index,
			}
			if headingPath != "" {
				metadata["section"] = headingPath
				metadata["heading_path"] = headingPath
			}
			if basePage > 0 {
				metadata["page"] = basePage
			}
			if basePageEnd > 0 && basePageEnd != basePage {
				metadata["page_end"] = basePageEnd
			}

			chunks = append(chunks, store.ChunkInput{
				ParentIndex: section.Index,
				Content:     content,
				Metadata:    metadata,
			})
		}

		if end == len(runes) {
			break
		}

		nextStart := end - defaultChildOverlap
		if nextStart <= start {
			nextStart = end
		}
		start = snapStart(runes, nextStart)
	}

	return chunks
}

func metadataIntFromMap(metadata map[string]any, key string) (int, bool) {
	value, ok := metadata[key]
	if !ok {
		return 0, false
	}

	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
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

func classifyChunkKind(content string) string {
	trimmed := strings.TrimSpace(strings.ToLower(content))
	if trimmed == "" {
		return "general"
	}

	scores := map[string]int{
		"general":    0,
		"definition": 0,
		"example":    0,
		"procedure":  0,
		"policy":     0,
		"reference":  0,
	}

	firstLine := trimmed
	if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
		firstLine = strings.TrimSpace(firstLine[:idx])
	}

	for _, pattern := range []string{
		"что такое ", "это ", "— это", "– это", "определяется как",
		"называется ", "is a ", "is an ", "refers to ", "defined as ",
	} {
		if strings.Contains(firstLine, pattern) || strings.Contains(trimmed, pattern) {
			scores["definition"] += 3
		}
	}

	for _, pattern := range []string{
		"например", "for example", "example:", "пример:", "например:",
		"например ", "such as ",
	} {
		if strings.Contains(trimmed, pattern) {
			scores["example"] += 3
		}
	}

	for _, pattern := range []string{
		"шаг ", "шаги", "step ", "steps", "процедур", "порядок", "инструкц",
		"как ", "how to", "follow these steps", "выполните", "сначала", "затем",
	} {
		if strings.Contains(trimmed, pattern) {
			scores["procedure"] += 2
		}
	}

	for _, pattern := range []string{
		"должен", "должны", "обязан", "обязаны", "запрещ", "разрешено", "необходимо",
		"требуется", "policy", "правило", "регламент", "must ", "must\n", "shall ",
		"required", "prohibited", "allowed",
	} {
		if strings.Contains(trimmed, pattern) {
			scores["policy"] += 2
		}
	}

	for _, pattern := range []string{
		"таблица", "сводка", "reference", "справка", "faq", "вопрос:", "ответ:",
		"q:", "a:",
	} {
		if strings.Contains(trimmed, pattern) {
			scores["reference"] += 2
		}
	}

	lines := strings.Split(content, "\n")
	listLines := 0
	for _, line := range lines {
		if looksLikeListItem(line) {
			listLines++
		}
	}
	if listLines >= 2 {
		scores["procedure"] += 3
	}
	if len(lines) >= 3 && listLines == 0 && len(sentenceSplitRE.Split(trimmed, -1)) >= 4 {
		scores["general"] += 1
	}

	bestKind := "general"
	bestScore := scores[bestKind]
	for _, kind := range []string{"definition", "procedure", "policy", "reference", "example", "general"} {
		if scores[kind] > bestScore {
			bestKind = kind
			bestScore = scores[kind]
		}
	}

	return bestKind
}
