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

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
