package embeddings

import "context"

type Provider interface {
	Embed(context.Context, []string) ([][]float32, error)
}
