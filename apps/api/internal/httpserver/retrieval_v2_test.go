package httpserver

import (
	"strings"
	"testing"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/config"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/store"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/websearch"
)

func TestAnalyzeQuery_ComparisonNeedsCoverage(t *testing.T) {
	analysis := analyzeQuery("сравни oauth и sso")
	if analysis.QueryType != "comparison" {
		t.Fatalf("expected comparison query type, got %q", analysis.QueryType)
	}
	if !analysis.NeedsRewrite || !analysis.NeedsExpansion || !analysis.NeedsMultiEntityCoverage {
		t.Fatalf("expected comparison query to require rewrite, expansion, and multi-entity coverage: %#v", analysis)
	}
	if !analysis.IsShortQuery {
		t.Fatalf("expected comparison query to be treated as short for rewrite heuristics")
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

func TestBuildLLMContext_FallsBackToChildWithoutOffsets(t *testing.T) {
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

	if !strings.Contains(context, "short child") {
		t.Fatalf("expected llm context to fall back to child content without offsets, got %q", context)
	}
	if !strings.Contains(context, "section:Security") {
		t.Fatalf("expected llm context to include parent section, got %q", context)
	}
}

func TestHeuristicQueryVariants_AddsComparisonForms(t *testing.T) {
	analysis := queryAnalysis{
		QueryType:                "comparison",
		NeedsRewrite:             true,
		NeedsExpansion:           true,
		NeedsMultiEntityCoverage: true,
		IsShortQuery:             true,
		Entities:                 []string{"oauth", "sso"},
	}

	variants := heuristicQueryVariants("сравни oauth и sso", analysis)
	joined := strings.Join(variants, " | ")
	if !strings.Contains(joined, "oauth vs sso") {
		t.Fatalf("expected comparison heuristic variant, got %#v", variants)
	}
	if !strings.Contains(joined, "difference between oauth and sso") {
		t.Fatalf("expected english comparison variant, got %#v", variants)
	}
}

func TestFilterQueryVariants_DropsVariantsWithoutCriticalTerms(t *testing.T) {
	analysis := queryAnalysis{
		QueryType:    "comparison",
		IsShortQuery: true,
		Entities:     []string{"oauth", "sso"},
	}

	filtered := filterQueryVariants("сравни oauth и sso", analysis, []string{
		"oauth vs sso",
		"difference between oauth and sso",
		"compare identity systems",
		"what is authentication",
	})

	if len(filtered) != 2 {
		t.Fatalf("expected only critical-term-preserving variants, got %#v", filtered)
	}
}

func TestBuildQueryVariants_UsesHeuristicFallbacksWithoutLLM(t *testing.T) {
	server := &Server{}
	analysis := analyzeQuery("sso")
	variants := server.buildQueryVariants(t.Context(), "sso", analysis)
	joined := strings.Join(variants, " | ")

	if len(variants) == 0 {
		t.Fatalf("expected non-empty query variants")
	}
	if !strings.Contains(joined, "что такое sso") {
		t.Fatalf("expected heuristic definition fallback, got %#v", variants)
	}
}

func TestBuildDenseQueryVariants_KeepsBaseVariantsWhenHyDENotEnabled(t *testing.T) {
	server := &Server{hydeEnabled: false}
	analysis := analyzeQuery("sso")
	base := []string{"sso", "что такое sso"}

	denseVariants := server.buildDenseQueryVariants(t.Context(), "sso", analysis, base)
	if strings.Join(denseVariants, "|") != strings.Join(base, "|") {
		t.Fatalf("expected dense variants to stay unchanged without HyDE, got %#v", denseVariants)
	}
}

func TestPlanQueryRoute_UsesWebForFreshnessQuery(t *testing.T) {
	searchClient, err := websearch.NewClient(config.SearchConfig{
		Enabled:    true,
		Provider:   "brave",
		APIKey:     "test-key",
		MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("expected search client, got error: %v", err)
	}
	server := &Server{
		search: searchClient,
	}
	user := store.User{
		Permissions: []string{store.PermissionToggleWebSearch},
	}

	plan := server.planQueryRoute(user, store.ModeUnstrict, "latest sso news")
	if plan.Route != "web" || !plan.UseWebSearch || plan.UseRetrieval {
		t.Fatalf("expected web route plan, got %#v", plan)
	}
}

func TestPlanQueryRoute_SkipsRetrievalForSmallTalk(t *testing.T) {
	server := &Server{}
	user := store.User{}

	plan := server.planQueryRoute(user, store.ModeUnstrict, "привет")
	if plan.Route != "no-retrieval" || plan.UseRetrieval {
		t.Fatalf("expected no-retrieval plan, got %#v", plan)
	}
}

func TestEnsureMultiDocumentCoverage_PrefersEntityCoverageAcrossDocuments(t *testing.T) {
	analysis := queryAnalysis{
		QueryType:                "comparison",
		NeedsMultiEntityCoverage: true,
		Entities:                 []string{"oauth", "sso"},
	}

	chunks := []store.RetrievalChunk{
		{
			ChunkID:       "c1",
			DocumentID:    "doc-oauth",
			DocTitle:      "OAuth guide",
			ParentContent: "OAuth is used for delegated authorization in distributed systems.",
			RRFScore:      0.09,
		},
		{
			ChunkID:       "c2",
			DocumentID:    "doc-oauth-2",
			DocTitle:      "OAuth setup",
			ParentContent: "OAuth clients use access tokens and refresh tokens.",
			RRFScore:      0.08,
		},
		{
			ChunkID:       "c3",
			DocumentID:    "doc-sso",
			DocTitle:      "SSO overview",
			ParentContent: "SSO allows one login session to span multiple applications.",
			RRFScore:      0.07,
		},
	}

	covered := ensureMultiDocumentCoverage(chunks, analysis)
	if len(covered) != 3 {
		t.Fatalf("expected all chunks to remain, got %d", len(covered))
	}
	if covered[0].DocumentID == covered[1].DocumentID {
		t.Fatalf("expected top two chunks to cover multiple documents, got %#v", covered[:2])
	}
	if covered[0].DocumentID != "doc-oauth" || covered[1].DocumentID != "doc-sso" {
		t.Fatalf("expected oauth then sso coverage, got %#v", []string{covered[0].DocumentID, covered[1].DocumentID})
	}
}

func TestEnsureMultiDocumentCoverage_FallsBackToSecondDocumentWhenEntitiesMissing(t *testing.T) {
	analysis := queryAnalysis{
		QueryType:                "comparison",
		NeedsMultiEntityCoverage: true,
		Entities:                 []string{"oauth", "sso"},
	}

	chunks := []store.RetrievalChunk{
		{
			ChunkID:       "c1",
			DocumentID:    "doc-a",
			DocTitle:      "Identity architecture",
			ParentContent: "Authorization concepts and login flow.",
			RRFScore:      0.11,
		},
		{
			ChunkID:       "c2",
			DocumentID:    "doc-a",
			DocTitle:      "Identity architecture appendix",
			ParentContent: "Token exchange details.",
			RRFScore:      0.09,
		},
		{
			ChunkID:       "c3",
			DocumentID:    "doc-b",
			DocTitle:      "Access guide",
			ParentContent: "Session federation details.",
			RRFScore:      0.08,
		},
	}

	covered := ensureMultiDocumentCoverage(chunks, analysis)
	if len(covered) != 3 {
		t.Fatalf("expected all chunks to remain, got %d", len(covered))
	}
	if covered[0].DocumentID != "doc-a" || covered[1].DocumentID != "doc-b" {
		t.Fatalf("expected fallback multi-doc coverage, got %#v", []string{covered[0].DocumentID, covered[1].DocumentID})
	}
}
