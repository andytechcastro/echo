package domain

import "context"

// Embedder is the port for text-to-vector embedding generation.
// Phase 1: NoEmbedder (noop, returns nil/0).
// Phase 2+: Configurable provider (Vertex AI, OpenAI, Cohere, etc.).
type Embedder interface {
	// Embed generates a vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// Dimensions returns the vector size for this embedder.
	Dimensions() int

	// Provider returns the human-readable provider name (e.g., "vertex-ai", "openai").
	Provider() string
}
