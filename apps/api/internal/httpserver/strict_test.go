package httpserver

import (
	"strings"
	"testing"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/store"
)

func TestIsStrictCompletionValid_AllowsFallback(t *testing.T) {
	if !isStrictCompletionValid("Недостаточно данных в базе знаний.", nil) {
		t.Fatalf("expected strict validation to allow fallback message")
	}
}

func TestIsStrictCompletionValid_RequiresCitationMarker(t *testing.T) {
	retrieved := []store.RetrievalChunk{{ChunkID: "c1", Content: "Источник"}}
	if isStrictCompletionValid("Ответ без ссылок.", retrieved) {
		t.Fatalf("expected strict validation to reject answer without [N] citations")
	}
}

func TestIsStrictCompletionValid_AllowsValidCitationMarker(t *testing.T) {
	retrieved := []store.RetrievalChunk{{ChunkID: "c1", Content: "Цитата из документа. Еще слова."}}
	answer := "Краткий ответ: Ответ со ссылкой [1].\n\nЦитаты:\n- \"Цитата из документа\" [1]"
	if !isStrictCompletionValid(answer, retrieved) {
		t.Fatalf("expected strict validation to allow answer with [1]")
	}
}

func TestIsStrictCompletionValid_RejectsOutOfRangeCitationMarker(t *testing.T) {
	retrieved := []store.RetrievalChunk{{ChunkID: "c1", Content: "Цитата из документа."}}
	answer := "Краткий ответ: Ответ со ссылкой [2].\n\nЦитаты:\n- \"Цитата из документа\" [2]"
	if isStrictCompletionValid(answer, retrieved) {
		t.Fatalf("expected strict validation to reject out-of-range citation index")
	}
	answer = "Краткий ответ: Ответ со ссылкой [0].\n\nЦитаты:\n- \"Цитата из документа\" [0]"
	if isStrictCompletionValid(answer, retrieved) {
		t.Fatalf("expected strict validation to reject zero citation index")
	}
}

func TestIsStrictCompletionValid_RequiresStrictFormatHeadings(t *testing.T) {
	retrieved := []store.RetrievalChunk{{ChunkID: "c1", Content: "Цитата из документа."}}
	if isStrictCompletionValid("Ответ со ссылкой [1].", retrieved) {
		t.Fatalf("expected strict validation to reject answers without required headings")
	}
}

func TestIsStrictCompletionValid_RequiresDirectQuotesFromSnippet(t *testing.T) {
	retrieved := []store.RetrievalChunk{{ChunkID: "c1", Content: "Только этот текст доступен."}}
	answer := "Краткий ответ: Ответ со ссылкой [1].\n\nЦитаты:\n- \"Несуществующая цитата\" [1]"
	if isStrictCompletionValid(answer, retrieved) {
		t.Fatalf("expected strict validation to reject quotes that do not exist in snippet")
	}
}

func TestNormalizeForQuoteMatch_RemovesSoftHyphenArtifacts(t *testing.T) {
	normalized := normalizeForQuoteMatch("со\u00ad\nдержащихся")
	if normalized != "содержащихся" {
		t.Fatalf("expected normalized quote text without soft hyphen artifacts, got %q", normalized)
	}
}

func TestBuildRetrievalQueries_UsesStemmedPrefixToken(t *testing.T) {
	embedQuery, textQuery := buildRetrievalQueries("что такое строка")
	if embedQuery != "что такое строка" {
		t.Fatalf("unexpected embed query: %q", embedQuery)
	}
	if textQuery != "строк" {
		t.Fatalf("expected text query to be stemmed prefix, got %q", textQuery)
	}
}

func TestDetectQueryIntent(t *testing.T) {
	testCases := []struct {
		query string
		want  string
	}{
		{query: "что такое строка", want: "definition"},
		{query: "как настроить sso", want: "procedure"},
		{query: "можно ли отправлять пароль в чат", want: "policy"},
		{query: "в чем разница между oauth и sso", want: "comparison"},
		{query: "контакты бухгалтерии", want: "general"},
	}

	for _, testCase := range testCases {
		if got := detectQueryIntent(testCase.query); got != testCase.want {
			t.Fatalf("detectQueryIntent(%q) = %q, want %q", testCase.query, got, testCase.want)
		}
	}
}

func TestBuildLLMContext_IncludesChunkKindMetadata(t *testing.T) {
	context := buildLLMContext([]store.RetrievalChunk{
		{
			ChunkID:     "c1",
			DocTitle:    "Glossary",
			DocFilename: "glossary.txt",
			ChunkIndex:  0,
			Content:     "Строка — это последовательность байтов.",
			Metadata: map[string]any{
				"chunk_kind": "definition",
			},
		},
	}, 500)

	if context == "" || !containsAll(context, "kind:definition", "Glossary", "Строка — это последовательность байтов.") {
		t.Fatalf("expected context to include chunk kind metadata, got %q", context)
	}
}

func TestFocusRetrievedChunks_DefinitionFiltersOffTopicChunks(t *testing.T) {
	retrieved := []store.RetrievalChunk{
		{
			ChunkID:     "goroutines",
			DocTitle:    "Go concurrency",
			DocFilename: "go.txt",
			Content:     "Горутины — это легковесные потоки управления в Go. Горутины запускаются словом go. Каналы используются между горутинами.",
		},
		{
			ChunkID:     "strings",
			DocTitle:    "Go strings",
			DocFilename: "strings.txt",
			Content:     "Учебник Go: строки, массивы, горутины и каналы.",
		},
	}

	filtered := focusRetrievedChunks("что такое горутины", "definition", retrieved)
	if len(filtered) != 1 {
		t.Fatalf("expected one focused chunk, got %d", len(filtered))
	}
	if filtered[0].ChunkID != "goroutines" {
		t.Fatalf("expected goroutines chunk to remain, got %q", filtered[0].ChunkID)
	}
}

func TestFocusRetrievedChunks_FallsBackWhenNoTopicalMatch(t *testing.T) {
	retrieved := []store.RetrievalChunk{
		{
			ChunkID:     "strings",
			DocTitle:    "Go strings",
			DocFilename: "strings.txt",
			Content:     "Строка в Go — это неизменяемая последовательность байтов.",
		},
	}

	filtered := focusRetrievedChunks("что такое горутины", "definition", retrieved)
	if len(filtered) != len(retrieved) {
		t.Fatalf("expected original retrieval to be preserved when filter is empty")
	}
}

func TestChunkTopicScore_PrefersDenseTopicalChunk(t *testing.T) {
	focusTokens := topicalQueryTokens("что такое горутины")
	if len(focusTokens) == 0 {
		t.Fatalf("expected focus tokens for goroutines query")
	}

	dense := store.RetrievalChunk{
		ChunkID:  "dense",
		Content:  "Горутины — это легковесные потоки. Каналы синхронизируют горутины. Горутины запускаются словом go.",
		DocTitle: "Go concurrency",
	}
	sparse := store.RetrievalChunk{
		ChunkID:  "sparse",
		Content:  "Учебник Go: строки, массивы, горутины и каналы.",
		DocTitle: "Go basics",
	}

	if chunkTopicScore(dense, focusTokens) <= chunkTopicScore(sparse, focusTokens) {
		t.Fatalf("expected dense goroutines chunk to score higher than mixed-topic chunk")
	}
}

func TestExpandTopicalVariants_HandlesRussianPlural(t *testing.T) {
	variants := expandTopicalVariants("строки")
	if !containsAll(strings.Join(variants, " "), "строки", "строк", "строка") {
		t.Fatalf("expected plural variants for строки, got %#v", variants)
	}
}

func TestChunkTopicScore_PrefersDefinitionPatternForStrings(t *testing.T) {
	focusTokens := topicalQueryTokens("что такое строки")
	good := store.RetrievalChunk{
		ChunkID: "good",
		Content: "В Go строка — это неизменяемая последовательность байтов (обычно UTF-8).",
		Metadata: map[string]any{
			"chunk_kind": "definition",
		},
		DocTitle: "go strings",
	}
	bad := store.RetrievalChunk{
		ChunkID: "bad",
		Content: "Неизменяемость означает, что две копии строки могут безопасно разделять одну и ту же память.",
		Metadata: map[string]any{
			"chunk_kind": "procedure",
		},
		DocTitle: "Donovan Go",
	}

	if chunkTopicScore(good, focusTokens) <= chunkTopicScore(bad, focusTokens) {
		t.Fatalf("expected definition pattern chunk to score higher for strings query")
	}
}

func TestDefinitionSourceScore_PrefersTxtOverNoisyPDF(t *testing.T) {
	txt := store.RetrievalChunk{
		DocFilename: "go.txt",
		Content:     "Строка — это неизменяемая последовательность байтов (обычно UTF-8).",
	}
	pdf := store.RetrievalChunk{
		DocFilename: "book.pdf",
		Content:     "0 до 7), не превышающими значение \\3 7 7 . Обе они обозначают один байт с указанным значением. Неформатированный строковый литерал ( ' . . . ' ) ...",
	}

	if definitionSourceScore(txt) <= definitionSourceScore(pdf) {
		t.Fatalf("expected txt definition source to score higher than noisy pdf")
	}
}

func TestLooksOCRNoisy_DetectsArtifacts(t *testing.T) {
	if !looksOCRNoisy("0 до 7), не превышающими значение \\3 7 7 . Неформатированный литерал ( ' . . . ' )") {
		t.Fatalf("expected noisy OCR-like chunk to be detected")
	}
	if looksOCRNoisy("Строка — это неизменяемая последовательность байтов (обычно UTF-8).") {
		t.Fatalf("did not expect clean definition chunk to be marked noisy")
	}
}

func TestBuildStrictHeuristicAnswer_UsesTopChunkBullets(t *testing.T) {
	retrieved := []store.RetrievalChunk{
		{
			ChunkID: "c1",
			Content: "3. ГОРУТИНЫ\n- Это легковесные потоки управления, выполняемые планировщиком Go.\n- Запускаются ключевым словом 'go'.\n- Потребляют очень мало памяти.",
		},
	}

	answer := buildStrictHeuristicAnswer(retrieved, responseLanguageRU)
	if answer == "" {
		t.Fatalf("expected heuristic answer")
	}
	if !containsAll(answer,
		"Краткий ответ:",
		"Это легковесные потоки управления, выполняемые планировщиком Go. [1]",
		"\"Запускаются ключевым словом 'go'.\" [1]",
	) {
		t.Fatalf("unexpected heuristic answer: %q", answer)
	}
	if !isStrictCompletionValid(answer, retrieved) {
		t.Fatalf("expected heuristic answer to satisfy strict validation")
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
