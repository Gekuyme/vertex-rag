package llm

import (
	"context"
	"strings"
)

type localProvider struct{}

func (p *localProvider) Complete(_ context.Context, request CompletionRequest) (string, error) {
	if len(request.Messages) == 0 {
		return "Недостаточно данных в базе знаний.", nil
	}

	last := request.Messages[len(request.Messages)-1].Content
	last = strings.TrimSpace(last)
	if last == "" {
		return "Недостаточно данных в базе знаний.", nil
	}

	runes := []rune(last)
	if len(runes) > 700 {
		last = string(runes[:700]) + "…"
	}

	return "Локальный режим ответа. Основано на контексте:\n\n" + last, nil
}
