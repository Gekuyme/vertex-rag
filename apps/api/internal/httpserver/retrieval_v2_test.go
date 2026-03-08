package httpserver

import (
	"strings"
	"testing"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/store"
)

func TestAnalyzeQuery_ComparisonNeedsCoverage(t *testing.T) {
	analysis := analyzeQuery("сравни oauth и sso")
	if analysis.QueryType != "comparison" {
		t.Fatalf("expected comparison query type, got %q", analysis.QueryType)
	}
	if !analysis.NeedsRewrite || !analysis.NeedsExpansion || !analysis.NeedsMultiEntityCoverage {
		t.Fatalf("expected comparison query to require rewrite, expansion, and multi-entity coverage: %#v", analysis)
	}
}

func TestFuseCandidates_AccumulatesDenseAndSparseRRF(t *testing.T) {
	fused := fuseCandidates(
		[]fusedCandidate{{
			chunkID:     "c1",
			denseRank:   2,
			vectorScore: 0.9,
			variants:    map[string]struct{}{"oauth setup": {}},
			retrievers:  map[string]struct{}{"dense": {}},
		}},
		[]fusedCandidate{{
			chunkID:    "c1",
			sparseRank: 1,
			textScore:  12.5,
			variants:   map[string]struct{}{"oauth configuration": {}},
			retrievers: map[string]struct{}{"sparse": {}},
		}},
	)
	if len(fused) != 1 {
		t.Fatalf("expected one fused candidate, got %d", len(fused))
	}
	expected := rrfScore(2) + rrfScore(1)
	if fused[0].rrfScore != expected {
		t.Fatalf("expected rrf score %f, got %f", expected, fused[0].rrfScore)
	}
	if len(fused[0].variants) != 2 {
		t.Fatalf("expected merged variants, got %#v", fused[0].variants)
	}
}

func TestBuildLLMContext_UsesParentContentWhenAvailable(t *testing.T) {
	context := buildLLMContext([]store.RetrievalChunk{{
		ChunkID:       "c1",
		DocTitle:      "Policy",
		DocFilename:   "policy.md",
		ChunkIndex:    3,
		Content:       "short child",
		ParentContent: "parent section content with more detail",
		Metadata: map[string]any{
			"chunk_kind": "policy",
		},
		ParentMetadata: map[string]any{
			"section": "Security",
		},
	}}, 500)

	if !strings.Contains(context, "parent section content with more detail") {
		t.Fatalf("expected llm context to use parent content, got %q", context)
	}
	if !strings.Contains(context, "section:Security") {
		t.Fatalf("expected llm context to include parent section, got %q", context)
	}
}
