package httpserver

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/store"
)

var numericTokenRE = regexp.MustCompile(`\b\d{2,}\b`)
var yearTokenRE = regexp.MustCompile(`\b(?:19|20)\d{2}\b`)

type groundingDocument struct {
	DocumentID    string `json:"document_id"`
	DocTitle      string `json:"doc_title"`
	DocFilename   string `json:"doc_filename"`
	CitationCount int    `json:"citation_count"`
}

type contradictionSignal struct {
	Type        string   `json:"type"`
	Summary     string   `json:"summary"`
	DocumentIDs []string `json:"document_ids"`
}

type groundingSummary struct {
	Confidence        float64               `json:"confidence"`
	ConfidenceLabel   string                `json:"confidence_label"`
	ConfidenceReasons []string              `json:"confidence_reasons"`
	CoverageRatio     float64               `json:"coverage_ratio"`
	SourceAgreement   float64               `json:"source_agreement"`
	RerankerMargin    float64               `json:"reranker_margin"`
	MultiDocument     bool                  `json:"multi_document"`
	DocumentCount     int                   `json:"document_count"`
	Documents         []groundingDocument   `json:"documents"`
	Contradictions    []contradictionSignal `json:"contradictions"`
}

func buildGroundingSummary(retrieved []store.RetrievalChunk, citations []retrievalCitation) groundingSummary {
	docMap := make(map[string]*groundingDocument)
	scoreSum := 0.0
	scoreCount := 0
	rerankCount := 0
	rerankScores := make([]float64, 0, len(citations))

	for _, citation := range citations {
		docID := strings.TrimSpace(citation.DocumentID)
		if docID == "" {
			continue
		}
		entry, exists := docMap[docID]
		if !exists {
			entry = &groundingDocument{
				DocumentID:  docID,
				DocTitle:    citation.DocTitle,
				DocFilename: citation.DocFilename,
			}
			docMap[docID] = entry
		}
		entry.CitationCount++
		if citation.Score > 0 {
			scoreSum += citation.Score
			scoreCount++
		}
		if citation.RerankScore > 0 {
			rerankCount++
			rerankScores = append(rerankScores, citation.RerankScore)
		}
	}

	documents := make([]groundingDocument, 0, len(docMap))
	for _, document := range docMap {
		documents = append(documents, *document)
	}
	sort.SliceStable(documents, func(i, j int) bool {
		if documents[i].CitationCount == documents[j].CitationCount {
			return documents[i].DocTitle < documents[j].DocTitle
		}
		return documents[i].CitationCount > documents[j].CitationCount
	})

	coverageRatio := 0.0
	if len(retrieved) > 0 {
		coverageRatio = minFloat(1.0, float64(len(citations))/float64(len(retrieved)))
	}
	sourceAgreement := computeSourceAgreement(documents)
	rerankerMargin := computeRerankerMargin(rerankScores)

	confidence := 0.12
	reasons := make([]string, 0, 5)
	if len(citations) > 0 {
		confidence += 0.18
		reasons = append(reasons, "retrieved cited evidence")
	}
	if coverageRatio >= 0.9 {
		confidence += 0.18
		reasons = append(reasons, "high citation coverage")
	} else if coverageRatio >= 0.6 {
		confidence += 0.1
		reasons = append(reasons, "partial citation coverage")
	}
	if len(documents) > 0 {
		confidence += 0.1
		reasons = append(reasons, "document coverage available")
	}
	if len(documents) > 1 && sourceAgreement >= 0.45 {
		confidence += 0.12
		reasons = append(reasons, "multiple documents support the answer")
	} else if len(documents) == 1 {
		confidence += 0.04
	}
	if scoreCount > 0 {
		avg := scoreSum / float64(scoreCount)
		switch {
		case avg >= 0.75:
			confidence += 0.2
			reasons = append(reasons, "high retrieval relevance")
		case avg >= 0.55:
			confidence += 0.12
			reasons = append(reasons, "moderate retrieval relevance")
		default:
			confidence += 0.04
		}
	}
	if rerankCount > 0 && rerankerMargin >= 0.08 {
		confidence += 0.1
		reasons = append(reasons, "reranker confirmed top context")
	} else if rerankCount > 0 {
		confidence += 0.04
	}

	contradictions := detectContradictions(retrieved)
	if len(contradictions) > 0 {
		confidence -= 0.18
		reasons = append(reasons, "contradicting evidence detected")
	}

	if confidence < 0 {
		confidence = 0
	}
	if confidence > 0.99 {
		confidence = 0.99
	}

	return groundingSummary{
		Confidence:        confidence,
		ConfidenceLabel:   confidenceLabel(confidence),
		ConfidenceReasons: reasons,
		CoverageRatio:     coverageRatio,
		SourceAgreement:   sourceAgreement,
		RerankerMargin:    rerankerMargin,
		MultiDocument:     len(documents) > 1,
		DocumentCount:     len(documents),
		Documents:         documents,
		Contradictions:    contradictions,
	}
}

func confidenceLabel(score float64) string {
	switch {
	case score >= 0.8:
		return "high"
	case score >= 0.55:
		return "medium"
	default:
		return "low"
	}
}

func computeSourceAgreement(documents []groundingDocument) float64 {
	if len(documents) == 0 {
		return 0
	}
	total := 0
	top := 0
	for _, document := range documents {
		total += document.CitationCount
		if document.CitationCount > top {
			top = document.CitationCount
		}
	}
	if total == 0 {
		return 0
	}
	if len(documents) == 1 {
		return 1
	}
	return 1.0 - (float64(top) / float64(total))
}

func computeRerankerMargin(scores []float64) float64 {
	if len(scores) < 2 {
		return 0
	}
	sorted := append([]float64(nil), scores...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i] > sorted[j]
	})
	margin := sorted[0] - sorted[1]
	if margin < 0 {
		return 0
	}
	return margin
}

func detectContradictions(retrieved []store.RetrievalChunk) []contradictionSignal {
	signals := make([]contradictionSignal, 0)
	if len(retrieved) < 2 {
		return signals
	}

	for i := 0; i < len(retrieved); i++ {
		for j := i + 1; j < len(retrieved); j++ {
			left := retrieved[i]
			right := retrieved[j]
			if strings.TrimSpace(left.DocumentID) == "" || strings.TrimSpace(right.DocumentID) == "" {
				continue
			}
			if left.DocumentID == right.DocumentID {
				continue
			}

			leftSection := firstNonEmptyString(metadataString(left.Metadata, "section"), metadataString(left.ParentMetadata, "section"))
			rightSection := firstNonEmptyString(metadataString(right.Metadata, "section"), metadataString(right.ParentMetadata, "section"))
			if leftSection == "" || rightSection == "" || !strings.EqualFold(leftSection, rightSection) {
				continue
			}

			if signal, ok := detectPolicyConflict(left, right); ok {
				signals = append(signals, signal)
				continue
			}
			if signal, ok := detectYearConflict(left, right); ok {
				signals = append(signals, signal)
				continue
			}
			if signal, ok := detectBooleanConflict(left, right); ok {
				signals = append(signals, signal)
				continue
			}
			if signal, ok := detectNumericConflict(left, right); ok {
				signals = append(signals, signal)
			}
		}
	}

	return dedupeContradictionSignals(signals)
}

func detectPolicyConflict(left, right store.RetrievalChunk) (contradictionSignal, bool) {
	leftText := strings.ToLower(firstNonEmptyString(left.ParentContent, left.Content))
	rightText := strings.ToLower(firstNonEmptyString(right.ParentContent, right.Content))
	if leftText == "" || rightText == "" {
		return contradictionSignal{}, false
	}

	leftPositive := containsAny(leftText, "must", "required", "allowed", "permitted", "разрешено", "можно", "должен", "обязан")
	rightPositive := containsAny(rightText, "must", "required", "allowed", "permitted", "разрешено", "можно", "должен", "обязан")
	leftNegative := containsAny(leftText, "must not", "prohibited", "forbidden", "not allowed", "запрещено", "нельзя", "не должен", "запрещается")
	rightNegative := containsAny(rightText, "must not", "prohibited", "forbidden", "not allowed", "запрещено", "нельзя", "не должен", "запрещается")

	if (leftPositive && rightNegative) || (leftNegative && rightPositive) {
		return contradictionSignal{
			Type:        "policy_conflict",
			Summary:     "documents disagree on the same policy area",
			DocumentIDs: []string{left.DocumentID, right.DocumentID},
		}, true
	}

	return contradictionSignal{}, false
}

func detectNumericConflict(left, right store.RetrievalChunk) (contradictionSignal, bool) {
	leftNumbers := numericTokenRE.FindAllString(firstNonEmptyString(left.ParentContent, left.Content), -1)
	rightNumbers := numericTokenRE.FindAllString(firstNonEmptyString(right.ParentContent, right.Content), -1)
	if len(leftNumbers) == 0 || len(rightNumbers) == 0 {
		return contradictionSignal{}, false
	}

	leftSet := make(map[string]struct{}, len(leftNumbers))
	for _, number := range leftNumbers {
		leftSet[number] = struct{}{}
	}
	for _, number := range rightNumbers {
		if _, exists := leftSet[number]; exists {
			return contradictionSignal{}, false
		}
	}

	leftSample, _ := strconv.Atoi(leftNumbers[0])
	rightSample, _ := strconv.Atoi(rightNumbers[0])
	if leftSample == 0 || rightSample == 0 || absInt(leftSample-rightSample) < 2 {
		return contradictionSignal{}, false
	}

	return contradictionSignal{
		Type:        "numeric_conflict",
		Summary:     "documents mention different numeric values in the same section",
		DocumentIDs: []string{left.DocumentID, right.DocumentID},
	}, true
}

func detectYearConflict(left, right store.RetrievalChunk) (contradictionSignal, bool) {
	leftYears := yearTokenRE.FindAllString(firstNonEmptyString(left.ParentContent, left.Content), -1)
	rightYears := yearTokenRE.FindAllString(firstNonEmptyString(right.ParentContent, right.Content), -1)
	if len(leftYears) == 0 || len(rightYears) == 0 {
		return contradictionSignal{}, false
	}

	leftSet := make(map[string]struct{}, len(leftYears))
	for _, year := range leftYears {
		leftSet[year] = struct{}{}
	}
	for _, year := range rightYears {
		if _, exists := leftSet[year]; exists {
			return contradictionSignal{}, false
		}
	}

	return contradictionSignal{
		Type:        "date_conflict",
		Summary:     "documents mention different years in the same section",
		DocumentIDs: []string{left.DocumentID, right.DocumentID},
	}, true
}

func detectBooleanConflict(left, right store.RetrievalChunk) (contradictionSignal, bool) {
	leftText := strings.ToLower(firstNonEmptyString(left.ParentContent, left.Content))
	rightText := strings.ToLower(firstNonEmptyString(right.ParentContent, right.Content))
	if leftText == "" || rightText == "" {
		return contradictionSignal{}, false
	}

	leftAffirmative := containsAny(leftText,
		"yes", "enabled", "supported", "available", "true",
		"да", "включено", "поддерживается", "доступно", "истина",
	)
	rightAffirmative := containsAny(rightText,
		"yes", "enabled", "supported", "available", "true",
		"да", "включено", "поддерживается", "доступно", "истина",
	)
	leftNegative := containsAny(leftText,
		"no", "disabled", "unsupported", "unavailable", "false",
		"нет", "отключено", "не поддерживается", "недоступно", "ложь",
	)
	rightNegative := containsAny(rightText,
		"no", "disabled", "unsupported", "unavailable", "false",
		"нет", "отключено", "не поддерживается", "недоступно", "ложь",
	)

	if (leftAffirmative && rightNegative) || (leftNegative && rightAffirmative) {
		return contradictionSignal{
			Type:        "boolean_conflict",
			Summary:     "documents disagree on a yes/no or enabled/disabled statement",
			DocumentIDs: []string{left.DocumentID, right.DocumentID},
		}, true
	}

	return contradictionSignal{}, false
}

func dedupeContradictionSignals(signals []contradictionSignal) []contradictionSignal {
	seen := make(map[string]struct{})
	out := make([]contradictionSignal, 0, len(signals))
	for _, signal := range signals {
		key := signal.Type + "|" + strings.Join(signal.DocumentIDs, ",")
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, signal)
	}
	return out
}

func containsAny(value string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}
