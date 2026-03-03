package httpserver

import "testing"

func TestIsStrictCompletionValid_AllowsFallback(t *testing.T) {
	if !isStrictCompletionValid("Недостаточно данных в базе знаний.", nil) {
		t.Fatalf("expected strict validation to allow fallback message")
	}
}

func TestIsStrictCompletionValid_RequiresCitationMarker(t *testing.T) {
	citations := []retrievalCitation{{ChunkID: "c1"}}
	if isStrictCompletionValid("Ответ без ссылок.", citations) {
		t.Fatalf("expected strict validation to reject answer without [N] citations")
	}
}

func TestIsStrictCompletionValid_AllowsValidCitationMarker(t *testing.T) {
	citations := []retrievalCitation{{ChunkID: "c1"}}
	answer := "Краткий ответ: Ответ со ссылкой [1].\n\nЦитаты:\n- \"Цитата\" [1]"
	if !isStrictCompletionValid(answer, citations) {
		t.Fatalf("expected strict validation to allow answer with [1]")
	}
}

func TestIsStrictCompletionValid_RejectsOutOfRangeCitationMarker(t *testing.T) {
	citations := []retrievalCitation{{ChunkID: "c1"}}
	answer := "Краткий ответ: Ответ со ссылкой [2].\n\nЦитаты:\n- \"Цитата\" [2]"
	if isStrictCompletionValid(answer, citations) {
		t.Fatalf("expected strict validation to reject out-of-range citation index")
	}
	answer = "Краткий ответ: Ответ со ссылкой [0].\n\nЦитаты:\n- \"Цитата\" [0]"
	if isStrictCompletionValid(answer, citations) {
		t.Fatalf("expected strict validation to reject zero citation index")
	}
}

func TestIsStrictCompletionValid_RequiresStrictFormatHeadings(t *testing.T) {
	citations := []retrievalCitation{{ChunkID: "c1"}}
	if isStrictCompletionValid("Ответ со ссылкой [1].", citations) {
		t.Fatalf("expected strict validation to reject answers without required headings")
	}
}
