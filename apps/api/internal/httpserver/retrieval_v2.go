package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/llm"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/reranker"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/sparsesearch"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/store"
)

const rrfK = 60.0

type queryAnalysis struct {
	QueryType                string   `json:"query_type"`
	NeedsRewrite             bool     `json:"needs_rewrite"`
	NeedsExpansion           bool     `json:"needs_expansion"`
	NeedsMultiEntityCoverage bool     `json:"needs_multi_entity_coverage"`
	IsShortQuery             bool     `json:"is_short_query"`
	Entities                 []string `json:"entities,omitempty"`
}

type retrievalV2Debug struct {
	Analysis      queryAnalysis `json:"query_analysis"`
	QueryVariants []string      `json:"query_variants"`
	DenseVariants []string      `json:"dense_variants,omitempty"`
}

type fusedCandidate struct {
	chunkID     string
	documentID  string
	vectorScore float64
	textScore   float64
	denseRank   int
	sparseRank  int
	rrfScore    float64
	rerankScore float64
	variants    map[string]struct{}
	retrievers  map[string]struct{}
}

func (s *Server) shouldUseRetrievalV2() bool {
	return strings.EqualFold(strings.TrimSpace(s.retrievalVersion), "v2")
}

func (s *Server) retrieveForChatV2(
	ctx context.Context,
	user store.User,
	query string,
	topK int,
	candidateK int,
) ([]store.RetrievalChunk, retrievalV2Debug, error) {
	analysis := analyzeQuery(query)
	variants := s.buildQueryVariants(ctx, query, analysis)
	denseVariants := s.buildDenseQueryVariants(ctx, query, analysis, variants)

	denseCandidates, err := s.collectDenseCandidates(ctx, user, denseVariants, candidateK)
	if err != nil {
		return nil, retrievalV2Debug{}, err
	}

	sparseCandidates, err := s.collectSparseCandidates(ctx, user, query, variants, analysis, candidateK)
	if err != nil {
		return nil, retrievalV2Debug{}, err
	}

	candidates := fuseCandidates(denseCandidates, sparseCandidates)
	if len(candidates) == 0 {
		return nil, retrievalV2Debug{Analysis: analysis, QueryVariants: variants, DenseVariants: denseVariants}, nil
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].rrfScore == candidates[j].rrfScore {
			if candidates[i].denseRank == candidates[j].denseRank {
				return candidates[i].sparseRank < candidates[j].sparseRank
			}
			return rankOrMax(candidates[i].denseRank) < rankOrMax(candidates[j].denseRank)
		}
		return candidates[i].rrfScore > candidates[j].rrfScore
	})

	hydrationIDs := make([]string, 0, minInt(len(candidates), 80))
	for index, candidate := range candidates {
		if index == 80 {
			break
		}
		hydrationIDs = append(hydrationIDs, candidate.chunkID)
	}

	hydratedChunks, err := s.store.GetRetrievalChunksByIDs(ctx, user.OrgID, user.RoleID, hydrationIDs)
	if err != nil {
		return nil, retrievalV2Debug{}, err
	}
	chunksByID := make(map[string]store.RetrievalChunk, len(hydratedChunks))
	for _, chunk := range hydratedChunks {
		chunksByID[chunk.ChunkID] = chunk
	}

	ranked := make([]store.RetrievalChunk, 0, len(hydrationIDs))
	for _, candidate := range candidates {
		chunk, ok := chunksByID[candidate.chunkID]
		if !ok {
			continue
		}
		chunk.VectorScore = candidate.vectorScore
		chunk.TextScore = candidate.textScore
		chunk.DenseRank = candidate.denseRank
		chunk.SparseRank = candidate.sparseRank
		chunk.RRFScore = candidate.rrfScore
		chunk.Score = candidate.rrfScore
		chunk.QueryVariant = joinVariantSet(candidate.variants)
		chunk.RetrieversUsed = sortedSetValues(candidate.retrievers)
		ranked = append(ranked, chunk)
		if len(ranked) == len(hydrationIDs) {
			break
		}
	}

	ranked = ensureMultiDocumentCoverage(ranked, analysis)
	ranked = s.applyReranker(ctx, query, ranked)
	ranked = focusRetrievedChunks(query, analysis.QueryType, ranked)
	if len(ranked) > topK {
		ranked = ranked[:topK]
	}

	return ranked, retrievalV2Debug{Analysis: analysis, QueryVariants: variants, DenseVariants: denseVariants}, nil
}

func analyzeQuery(query string) queryAnalysis {
	queryType := detectQueryIntent(query)
	tokens := tokenizeQuery(query)
	filteredTokens := make([]string, 0, len(tokens))
	seen := make(map[string]struct{})
	for _, token := range tokens {
		token = strings.TrimSpace(strings.ToLower(token))
		if token == "" || isLikelyStopword(token) {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		filteredTokens = append(filteredTokens, token)
	}

	analysis := queryAnalysis{
		QueryType:                queryType,
		NeedsRewrite:             len(filteredTokens) <= 4 || queryType == "comparison",
		NeedsExpansion:           len(filteredTokens) <= 3 || queryType == "comparison" || queryType == "policy",
		NeedsMultiEntityCoverage: queryType == "comparison",
		IsShortQuery:             len(filteredTokens) <= 3,
		Entities:                 filteredTokens,
	}
	if len(analysis.Entities) > 4 {
		analysis.Entities = analysis.Entities[:4]
	}
	return analysis
}

func (s *Server) buildQueryVariants(ctx context.Context, query string, analysis queryAnalysis) []string {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return nil
	}

	variants := []string{trimmedQuery}
	variants = append(variants, heuristicQueryVariants(trimmedQuery, analysis)...)

	if s.queryRewriteEnabled && analysis.NeedsRewrite {
		if rewritten := s.generateLLMQueryVariants(ctx, trimmedQuery, analysis, 1); len(rewritten) > 0 {
			variants = append(variants, rewritten...)
		}
	}
	if s.queryExpandEnabled && analysis.NeedsExpansion {
		if expanded := s.generateLLMQueryVariants(ctx, trimmedQuery, analysis, 3); len(expanded) > 0 {
			variants = append(variants, expanded...)
		}
	}

	return filterQueryVariants(trimmedQuery, analysis, variants)
}

func (s *Server) generateLLMQueryVariants(ctx context.Context, query string, analysis queryAnalysis, limit int) []string {
	if s.llm == nil || limit <= 0 {
		return nil
	}

	provider, option, ok := s.llm.Resolve("")
	if !ok || provider == nil {
		return nil
	}

	prompt := fmt.Sprintf(
		"Rewrite the user query for retrieval. Return JSON only: {\"queries\":[\"...\"]}. Limit: %d. Query type: %s. Original query: %s",
		limit,
		analysis.QueryType,
		query,
	)
	response, err := provider.Complete(ctx, llm.CompletionRequest{
		Mode: "strict",
		Messages: []llm.Message{
			{Role: "system", Content: "You generate retrieval queries. Preserve meaning, entities, dates, names, and factual constraints. Return valid JSON only."},
			{Role: "user", Content: prompt},
		},
		Model:       option.DefaultModel,
		MaxTokens:   220,
		Temperature: 0.1,
	})
	if err != nil {
		return nil
	}

	var parsed struct {
		Queries []string `json:"queries"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(response)), &parsed); err != nil {
		return nil
	}
	if len(parsed.Queries) > limit {
		parsed.Queries = parsed.Queries[:limit]
	}
	return filterQueryVariants(query, analysis, parsed.Queries)
}

func (s *Server) buildDenseQueryVariants(ctx context.Context, query string, analysis queryAnalysis, variants []string) []string {
	denseVariants := append([]string(nil), variants...)
	if !s.hydeEnabled {
		return denseVariants
	}

	if hydeVariant := s.generateHyDEVariant(ctx, query, analysis); hydeVariant != "" {
		denseVariants = append(denseVariants, hydeVariant)
	}
	return dedupeStrings(denseVariants)
}

func (s *Server) generateHyDEVariant(ctx context.Context, query string, analysis queryAnalysis) string {
	if s.llm == nil {
		return ""
	}
	provider, option, ok := s.llm.Resolve("")
	if !ok || provider == nil {
		return ""
	}

	prompt := fmt.Sprintf(
		"Write a short hypothetical answer for retrieval expansion. Preserve names, dates, numbers, and entities. Query type: %s. Question: %s",
		analysis.QueryType,
		query,
	)
	response, err := provider.Complete(ctx, llm.CompletionRequest{
		Mode: "strict",
		Messages: []llm.Message{
			{Role: "system", Content: "Generate one concise hypothetical answer paragraph for dense retrieval only. Do not mention that it is hypothetical."},
			{Role: "user", Content: prompt},
		},
		Model:       option.DefaultModel,
		MaxTokens:   180,
		Temperature: 0.2,
	})
	if err != nil {
		return ""
	}

	return strings.TrimSpace(response)
}

func (s *Server) collectDenseCandidates(ctx context.Context, user store.User, variants []string, candidateK int) ([]fusedCandidate, error) {
	if len(variants) == 0 {
		return nil, nil
	}

	vectors, err := s.embeddings.Embed(ctx, variants)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(variants) {
		return nil, fmt.Errorf("embedding provider returned %d vectors for %d variants", len(vectors), len(variants))
	}

	collected := make([]fusedCandidate, 0)
	for index, variant := range variants {
		hits, err := s.store.DenseRetrieveChunks(ctx, store.DenseRetrievalOptions{
			OrgID:          user.OrgID,
			RoleID:         user.RoleID,
			QueryEmbedding: vectors[index],
			MaxResults:     candidateK,
			MaxPerDoc:      minInt(6, candidateK),
			QueryVariant:   variant,
		})
		if err != nil {
			return nil, err
		}
		for _, hit := range hits {
			collected = append(collected, fusedCandidate{
				chunkID:     hit.ChunkID,
				documentID:  hit.DocumentID,
				vectorScore: hit.VectorScore,
				denseRank:   hit.DenseRank,
				variants:    map[string]struct{}{variant: {}},
				retrievers:  map[string]struct{}{"dense": {}},
			})
		}
	}

	return collected, nil
}

func (s *Server) collectSparseCandidates(ctx context.Context, user store.User, query string, variants []string, analysis queryAnalysis, candidateK int) ([]fusedCandidate, error) {
	if s.sparseSearch == nil || !s.sparseSearch.Enabled() {
		return nil, nil
	}

	hits, err := s.sparseSearch.Search(ctx, sparsesearch.SearchRequest{
		OrgID:           user.OrgID,
		RoleID:          user.RoleID,
		Query:           query,
		Variants:        variants,
		MaxResults:      candidateK,
		QueryType:       analysis.QueryType,
		RequireMultiDoc: analysis.NeedsMultiEntityCoverage,
	})
	if err != nil {
		return nil, err
	}

	collected := make([]fusedCandidate, 0, len(hits))
	for _, hit := range hits {
		collected = append(collected, fusedCandidate{
			chunkID:    hit.ChunkID,
			documentID: hit.DocumentID,
			textScore:  hit.Score,
			sparseRank: hit.Rank,
			variants:   map[string]struct{}{hit.Query: {}},
			retrievers: map[string]struct{}{"sparse": {}},
		})
	}
	return collected, nil
}

func fuseCandidates(dense []fusedCandidate, sparse []fusedCandidate) []fusedCandidate {
	byID := make(map[string]*fusedCandidate)
	merge := func(items []fusedCandidate, source string) {
		for _, item := range items {
			existing, ok := byID[item.chunkID]
			if !ok {
				item.rrfScore = 0
				itemCopy := item
				byID[item.chunkID] = &itemCopy
				existing = &itemCopy
			}
			if item.denseRank > 0 && (existing.denseRank == 0 || item.denseRank < existing.denseRank) {
				existing.denseRank = item.denseRank
				existing.vectorScore = maxFloat(existing.vectorScore, item.vectorScore)
			}
			if item.sparseRank > 0 && (existing.sparseRank == 0 || item.sparseRank < existing.sparseRank) {
				existing.sparseRank = item.sparseRank
				existing.textScore = maxFloat(existing.textScore, item.textScore)
			}
			for variant := range item.variants {
				existing.variants[variant] = struct{}{}
			}
			for retriever := range item.retrievers {
				existing.retrievers[retriever] = struct{}{}
			}
			switch source {
			case "dense":
				existing.rrfScore += rrfScore(item.denseRank)
			case "sparse":
				existing.rrfScore += rrfScore(item.sparseRank)
			}
		}
	}

	merge(dense, "dense")
	merge(sparse, "sparse")

	out := make([]fusedCandidate, 0, len(byID))
	for _, candidate := range byID {
		out = append(out, *candidate)
	}
	return out
}

func (s *Server) applyReranker(ctx context.Context, query string, chunks []store.RetrievalChunk) []store.RetrievalChunk {
	if s.reranker == nil || !s.reranker.Enabled() || len(chunks) == 0 {
		sortChunksByScore(chunks)
		return chunks
	}

	limit := minInt(30, len(chunks))
	docs := make([]reranker.Document, 0, limit)
	for _, chunk := range chunks[:limit] {
		content := strings.TrimSpace(chunk.ParentContent)
		if content == "" {
			content = chunk.Content
		}
		docs = append(docs, reranker.Document{ID: chunk.ChunkID, Content: content})
	}

	results, err := s.reranker.Rerank(ctx, query, docs)
	if err != nil || len(results) == 0 {
		sortChunksByScore(chunks)
		return chunks
	}

	scores := make(map[string]reranker.Result, len(results))
	for _, result := range results {
		scores[result.ID] = result
	}

	sort.SliceStable(chunks, func(i, j int) bool {
		left, leftOK := scores[chunks[i].ChunkID]
		right, rightOK := scores[chunks[j].ChunkID]
		if leftOK && rightOK {
			if left.Score == right.Score {
				return chunks[i].RRFScore > chunks[j].RRFScore
			}
			return left.Score > right.Score
		}
		if leftOK != rightOK {
			return leftOK
		}
		return chunks[i].RRFScore > chunks[j].RRFScore
	})

	for index := range chunks {
		if result, ok := scores[chunks[index].ChunkID]; ok {
			chunks[index].RerankScore = result.Score
			chunks[index].Score = result.Score
		} else {
			chunks[index].Score = chunks[index].RRFScore
		}
	}

	return chunks
}

func ensureMultiDocumentCoverage(chunks []store.RetrievalChunk, analysis queryAnalysis) []store.RetrievalChunk {
	if !analysis.NeedsMultiEntityCoverage || len(chunks) < 2 {
		return chunks
	}

	selected := make([]store.RetrievalChunk, 0, len(chunks))
	used := make([]bool, len(chunks))

	entityTargets := normalizedCoverageEntities(analysis.Entities)
	docSeen := make(map[string]struct{})
	for _, entity := range entityTargets {
		bestIndex := -1
		bestScore := -1
		for index, chunk := range chunks {
			if used[index] {
				continue
			}
			score := chunkEntityCoverageScore(chunk, entity)
			if score > bestScore {
				bestScore = score
				bestIndex = index
			}
		}
		if bestIndex >= 0 && bestScore > 0 {
			selected = append(selected, chunks[bestIndex])
			used[bestIndex] = true
			if docID := strings.TrimSpace(chunks[bestIndex].DocumentID); docID != "" {
				docSeen[docID] = struct{}{}
			}
		}
	}

	if len(docSeen) < 2 {
		for index, chunk := range chunks {
			if used[index] {
				continue
			}
			docID := strings.TrimSpace(chunk.DocumentID)
			if docID == "" {
				continue
			}
			if _, exists := docSeen[docID]; exists {
				continue
			}
			selected = append(selected, chunk)
			used[index] = true
			docSeen[docID] = struct{}{}
			if len(docSeen) >= 2 {
				break
			}
		}
	}

	if len(selected) == 0 {
		return chunks
	}

	for index, chunk := range chunks {
		if used[index] {
			continue
		}
		selected = append(selected, chunk)
	}

	return selected
}

func normalizedCoverageEntities(values []string) []string {
	entities := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		entity := strings.TrimSpace(strings.ToLower(value))
		if entity == "" || isLikelyStopword(entity) {
			continue
		}
		if _, exists := seen[entity]; exists {
			continue
		}
		seen[entity] = struct{}{}
		entities = append(entities, entity)
	}
	if len(entities) > 3 {
		entities = entities[:3]
	}
	return entities
}

func heuristicQueryVariants(query string, analysis queryAnalysis) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	variants := make([]string, 0, 6)
	entityTargets := normalizedCoverageEntities(analysis.Entities)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || strings.EqualFold(value, query) {
			return
		}
		variants = append(variants, value)
	}

	switch analysis.QueryType {
	case "comparison":
		if len(entityTargets) >= 2 {
			left := entityTargets[0]
			right := entityTargets[1]
			add(fmt.Sprintf("%s vs %s", left, right))
			add(fmt.Sprintf("difference between %s and %s", left, right))
			add(fmt.Sprintf("сравнение %s и %s", left, right))
		}
	case "definition":
		if analysis.IsShortQuery && len(entityTargets) > 0 {
			entity := entityTargets[0]
			add(fmt.Sprintf("что такое %s", entity))
			add(fmt.Sprintf("%s definition", entity))
		}
	case "procedure":
		if len(entityTargets) > 0 {
			entity := entityTargets[0]
			add(fmt.Sprintf("how to configure %s", entity))
			add(fmt.Sprintf("как настроить %s", entity))
		}
	case "policy":
		if len(entityTargets) > 0 {
			entity := entityTargets[0]
			add(fmt.Sprintf("policy for %s", entity))
			add(fmt.Sprintf("правила %s", entity))
		}
	case "general":
		if analysis.IsShortQuery && len(entityTargets) > 0 {
			entity := entityTargets[0]
			add(fmt.Sprintf("что такое %s", entity))
			add(fmt.Sprintf("%s definition", entity))
			add(fmt.Sprintf("как работает %s", entity))
		}
	}

	if analysis.IsShortQuery {
		for _, entity := range entityTargets {
			add(entity)
			for _, variant := range expandTopicalVariants(entity) {
				add(variant)
			}
		}
	}

	return dedupeStrings(variants)
}

func filterQueryVariants(query string, analysis queryAnalysis, variants []string) []string {
	baseTokens := criticalQueryTokens(query, analysis)
	filtered := make([]string, 0, len(variants))
	seen := make(map[string]struct{})

	for _, variant := range variants {
		normalized := strings.TrimSpace(variant)
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if _, exists := seen[key]; exists {
			continue
		}
		if !variantPreservesCriticalTerms(normalized, baseTokens) {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, normalized)
		if len(filtered) == 5 {
			break
		}
	}

	if len(filtered) == 0 {
		return []string{strings.TrimSpace(query)}
	}
	return filtered
}

func criticalQueryTokens(query string, analysis queryAnalysis) []string {
	out := make([]string, 0, 8)
	seen := make(map[string]struct{})
	add := func(value string) {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	for _, entity := range normalizedCoverageEntities(analysis.Entities) {
		add(entity)
	}
	for _, token := range tokenizeQuery(query) {
		token = strings.TrimSpace(strings.ToLower(token))
		if token == "" {
			continue
		}
		hasDigit := strings.IndexFunc(token, func(r rune) bool { return r >= '0' && r <= '9' }) >= 0
		if hasDigit || len([]rune(token)) >= 4 {
			add(token)
		}
	}
	return out
}

func variantPreservesCriticalTerms(variant string, criticalTokens []string) bool {
	if len(criticalTokens) == 0 {
		return true
	}
	normalized := strings.ToLower(strings.TrimSpace(variant))
	matched := 0
	for _, token := range criticalTokens {
		if strings.Contains(normalized, token) {
			matched++
		}
	}
	required := minInt(2, len(criticalTokens))
	if required <= 0 {
		required = 1
	}
	return matched >= required
}

func chunkEntityCoverageScore(chunk store.RetrievalChunk, entity string) int {
	if strings.TrimSpace(entity) == "" {
		return 0
	}

	score := 0
	content := strings.ToLower(firstNonEmptyString(chunk.ParentContent, chunk.Content))
	title := strings.ToLower(chunk.DocTitle)
	filename := strings.ToLower(chunk.DocFilename)
	section := strings.ToLower(firstNonEmptyString(metadataString(chunk.Metadata, "section"), metadataString(chunk.ParentMetadata, "section")))

	score += countOccurrences(content, entity) * 3
	score += countOccurrences(title, entity) * 2
	score += countOccurrences(section, entity) * 2
	score += countOccurrences(filename, entity)
	if score == 0 {
		return 0
	}

	if chunk.RerankScore > 0 {
		score += 2
	}
	if chunk.RRFScore > 0 {
		score += 1
	}
	return score
}

func sortChunksByScore(chunks []store.RetrievalChunk) {
	sort.SliceStable(chunks, func(i, j int) bool {
		if chunks[i].Score == chunks[j].Score {
			return chunks[i].RRFScore > chunks[j].RRFScore
		}
		return chunks[i].Score > chunks[j].Score
	})
}

func rrfScore(rank int) float64 {
	if rank <= 0 {
		return 0
	}
	return 1.0 / (rrfK + float64(rank))
}

func rankOrMax(rank int) int {
	if rank <= 0 {
		return 1 << 30
	}
	return rank
}

func joinVariantSet(values map[string]struct{}) string {
	return strings.Join(sortedSetValues(values), " | ")
}

func sortedSetValues(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
