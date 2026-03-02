package embeddings

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
)

const localEmbeddingDimension = 256

type localProvider struct{}

func (p *localProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vectors = append(vectors, localEmbed(text))
	}

	return vectors, nil
}

func localEmbed(text string) []float32 {
	vector := make([]float32, localEmbeddingDimension)
	normalizedText := strings.ToLower(strings.TrimSpace(text))
	if normalizedText == "" {
		return vector
	}

	words := strings.Fields(normalizedText)
	for _, word := range words {
		hasher := fnv.New32a()
		_, _ = hasher.Write([]byte(word))
		index := int(hasher.Sum32() % uint32(localEmbeddingDimension))
		vector[index] += 1
	}

	var sumSquares float64
	for _, value := range vector {
		sumSquares += float64(value * value)
	}
	if sumSquares == 0 {
		return vector
	}

	norm := float32(math.Sqrt(sumSquares))
	for index, value := range vector {
		vector[index] = value / norm
	}

	return vector
}
