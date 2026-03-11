package httpserver

import (
	"strings"
	"testing"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/store"
)

func TestBuildGroundingSummary_IncludesCoverageAndConfidence(t *testing.T) {
	retrieved := []store.RetrievalChunk{
		{ChunkID: "c1", DocumentID: "d1"},
		{ChunkID: "c2", DocumentID: "d2"},
	}
	citations := []retrievalCitation{
		{
			ChunkID:        "c1",
			DocumentID:     "d1",
			DocTitle:       "Security guide",
			DocFilename:    "security.pdf",
			Score:          0.84,
			RerankScore:    0.91,
			RetrieversUsed: []string{"dense", "sparse"},
		},
		{
			ChunkID:        "c2",
			DocumentID:     "d2",
			DocTitle:       "Identity guide",
			DocFilename:    "identity.pdf",
			Score:          0.78,
			RerankScore:    0.88,
			RetrieversUsed: []string{"dense"},
		},
	}

	summary := buildGroundingSummary(retrieved, citations)

	if summary.DocumentCount != 2 || !summary.MultiDocument {
		t.Fatalf("expected multi-document grounding summary, got %#v", summary)
	}
	if summary.ConfidenceLabel != "high" {
		t.Fatalf("expected high confidence label, got %#v", summary)
	}
	if len(summary.ConfidenceReasons) == 0 {
		t.Fatalf("expected confidence reasons, got %#v", summary)
	}
	if len(summary.Documents) != 2 {
		t.Fatalf("expected supporting documents in grounding summary, got %#v", summary.Documents)
	}
	if summary.CoverageRatio < 0.9 {
		t.Fatalf("expected strong coverage ratio, got %#v", summary)
	}
	if summary.SourceAgreement <= 0 {
		t.Fatalf("expected positive source agreement, got %#v", summary)
	}
	if summary.RerankerMargin <= 0 {
		t.Fatalf("expected positive reranker margin, got %#v", summary)
	}
}

func TestBuildGroundingSummary_DetectsPolicyContradiction(t *testing.T) {
	retrieved := []store.RetrievalChunk{
		{
			ChunkID:       "c1",
			DocumentID:    "d1",
			ParentContent: "Users must enable MFA before access is granted.",
			Metadata: map[string]any{
				"section": "Access policy",
			},
		},
		{
			ChunkID:       "c2",
			DocumentID:    "d2",
			ParentContent: "Users must not enable MFA before temporary access is granted.",
			Metadata: map[string]any{
				"section": "Access policy",
			},
		},
	}
	citations := []retrievalCitation{
		{ChunkID: "c1", DocumentID: "d1", DocTitle: "Policy A", Score: 0.71},
		{ChunkID: "c2", DocumentID: "d2", DocTitle: "Policy B", Score: 0.74},
	}

	summary := buildGroundingSummary(retrieved, citations)

	if len(summary.Contradictions) == 0 {
		t.Fatalf("expected contradiction signal, got %#v", summary)
	}
	if summary.Contradictions[0].Type != "policy_conflict" {
		t.Fatalf("expected policy conflict, got %#v", summary.Contradictions)
	}
	if summary.Confidence >= 0.8 {
		t.Fatalf("expected contradiction to reduce confidence, got %#v", summary)
	}
}

func TestBuildGroundingSummary_DetectsYearContradiction(t *testing.T) {
	retrieved := []store.RetrievalChunk{
		{
			ChunkID:       "c1",
			DocumentID:    "d1",
			ParentContent: "The retention schedule was approved in 2023 for this policy area.",
			Metadata: map[string]any{
				"section": "Retention",
			},
		},
		{
			ChunkID:       "c2",
			DocumentID:    "d2",
			ParentContent: "The retention schedule was approved in 2025 for this policy area.",
			Metadata: map[string]any{
				"section": "Retention",
			},
		},
	}
	citations := []retrievalCitation{
		{ChunkID: "c1", DocumentID: "d1", DocTitle: "Retention A", Score: 0.73},
		{ChunkID: "c2", DocumentID: "d2", DocTitle: "Retention B", Score: 0.75},
	}

	summary := buildGroundingSummary(retrieved, citations)

	if len(summary.Contradictions) == 0 {
		t.Fatalf("expected contradiction signal, got %#v", summary)
	}
	if summary.Contradictions[0].Type != "date_conflict" {
		t.Fatalf("expected date conflict, got %#v", summary.Contradictions)
	}
}

func TestBuildGroundingSummary_DetectsBooleanContradiction(t *testing.T) {
	retrieved := []store.RetrievalChunk{
		{
			ChunkID:       "c1",
			DocumentID:    "d1",
			ParentContent: "SSO is enabled for all enterprise tenants in this environment.",
			Metadata: map[string]any{
				"section": "SSO",
			},
		},
		{
			ChunkID:       "c2",
			DocumentID:    "d2",
			ParentContent: "SSO is disabled for all enterprise tenants in this environment.",
			Metadata: map[string]any{
				"section": "SSO",
			},
		},
	}
	citations := []retrievalCitation{
		{ChunkID: "c1", DocumentID: "d1", DocTitle: "SSO A", Score: 0.69},
		{ChunkID: "c2", DocumentID: "d2", DocTitle: "SSO B", Score: 0.68},
	}

	summary := buildGroundingSummary(retrieved, citations)

	if len(summary.Contradictions) == 0 {
		t.Fatalf("expected contradiction signal, got %#v", summary)
	}
	if summary.Contradictions[0].Type != "boolean_conflict" {
		t.Fatalf("expected boolean conflict, got %#v", summary.Contradictions)
	}
}

func TestBuildRetrievalCitations_ExposesParentAndRankMetadata(t *testing.T) {
	retrieved := []store.RetrievalChunk{
		{
			ChunkID:         "c1",
			DocumentID:      "d1",
			DocTitle:        "Security policy",
			DocFilename:     "policy.md",
			Content:         "child snippet",
			ParentContent:   "parent snippet with broader context",
			ParentSectionID: "p1",
			VectorScore:     0.63,
			TextScore:       12.4,
			Score:           0.88,
			DenseRank:       4,
			SparseRank:      2,
			RRFScore:        0.031,
			RerankScore:     0.94,
			QueryVariant:    "mfa policy",
			RetrieversUsed:  []string{"dense", "sparse"},
			Metadata: map[string]any{
				"page":       3,
				"section":    "Security",
				"char_start": 12,
				"char_end":   44,
			},
			ParentMetadata: map[string]any{
				"char_start": 12,
			},
		},
	}

	citations := buildRetrievalCitations(retrieved)
	if len(citations) != 1 {
		t.Fatalf("expected one citation, got %d", len(citations))
	}

	citation := citations[0]
	if citation.ParentSectionID != "p1" {
		t.Fatalf("expected parent section ID, got %#v", citation)
	}
	if citation.Offsets["start"] != 12 || citation.Offsets["end"] != 44 {
		t.Fatalf("expected offsets in citation, got %#v", citation.Offsets)
	}
	if citation.DenseRank != 4 || citation.SparseRank != 2 {
		t.Fatalf("expected dense/sparse rank metadata, got %#v", citation)
	}
	if citation.RerankScore != 0.94 || citation.RRFScore != 0.031 {
		t.Fatalf("expected rerank and rrf scores, got %#v", citation)
	}
	if citation.Snippet != "parent snippet with broader context" {
		t.Fatalf("expected citation snippet to prefer parent content, got %#v", citation)
	}
	if !strings.HasPrefix(citation.EvidenceSpan, "parent snippet with broader") {
		t.Fatalf("expected evidence span extracted from parent offsets, got %#v", citation.EvidenceSpan)
	}
}

func TestBuildGroundingSummary_LowersConfidenceForWeakCoverage(t *testing.T) {
	retrieved := []store.RetrievalChunk{
		{ChunkID: "c1", DocumentID: "d1"},
		{ChunkID: "c2", DocumentID: "d1"},
		{ChunkID: "c3", DocumentID: "d2"},
		{ChunkID: "c4", DocumentID: "d2"},
	}
	citations := []retrievalCitation{
		{
			ChunkID:     "c1",
			DocumentID:  "d1",
			DocTitle:    "Doc A",
			Score:       0.58,
			RerankScore: 0.61,
		},
	}

	summary := buildGroundingSummary(retrieved, citations)

	if summary.CoverageRatio >= 0.6 {
		t.Fatalf("expected weak coverage ratio, got %#v", summary)
	}
	if summary.ConfidenceLabel == "high" {
		t.Fatalf("expected weak coverage to avoid high confidence, got %#v", summary)
	}
}
