package embedder

import (
	"context"
	"math"
	"os"
	"testing"
)

func TestNewONNXEmbedder_MissingFiles(t *testing.T) {
	// Without actual model/vocab files, should fail with file-not-found error.
	_, err := NewONNXEmbedder("/nonexistent/model.onnx", "/nonexistent/vocab.txt", "")
	if err == nil {
		t.Fatal("expected error when files not found")
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestONNXEmbedder_Embed_EmptyText(t *testing.T) {
	// Empty text check happens before any tokenizer access, so nil fields are safe.
	e := &ONNXEmbedder{dimensions: 384, provider: "local-onnx"}

	_, err := e.Embed(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty text")
	}
}

func TestONNXEmbedder_Embed_ContextCanceled(t *testing.T) {
	e := &ONNXEmbedder{dimensions: 384, provider: "local-onnx"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := e.Embed(ctx, "hello")
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

func TestONNXEmbedder_Dimensions(t *testing.T) {
	e := &ONNXEmbedder{dimensions: 384, provider: "local-onnx"}
	if e.Dimensions() != 384 {
		t.Errorf("expected 384 dimensions, got %d", e.Dimensions())
	}
}

func TestONNXEmbedder_Provider(t *testing.T) {
	e := &ONNXEmbedder{dimensions: 384, provider: "local-onnx"}
	if e.Provider() != "local-onnx" {
		t.Errorf("expected 'local-onnx', got %s", e.Provider())
	}
}

func TestONNXEmbedder_Close_NilSession(t *testing.T) {
	// Close with zero-value embedder (no session, no tensors) should not panic.
	e := &ONNXEmbedder{dimensions: 384, provider: "local-onnx"}
	err := e.Close()
	if err != nil {
		t.Errorf("expected no error when closing nil session: %v", err)
	}
}

// TestONNXEmbedder_Integration tests actual embedding with ONNX Runtime.
// Skip if ONNXRUNTIME_LIB_PATH is not set.
func TestONNXEmbedder_Integration(t *testing.T) {
	runtimePath := os.Getenv("ONNXRUNTIME_LIB_PATH")
	if runtimePath == "" {
		t.Skip("ONNXRUNTIME_LIB_PATH not set, skipping integration test")
	}

	modelPath := os.Getenv("ONNX_MODEL_PATH")
	if modelPath == "" {
		modelPath = defaultModelPath()
	}
	vocabPath := os.Getenv("ONNX_VOCAB_PATH")
	if vocabPath == "" {
		vocabPath = defaultVocabPath()
	}

	e, err := NewONNXEmbedder(modelPath, vocabPath, runtimePath)
	if err != nil {
		t.Fatalf("create embedder: %v", err)
	}
	defer e.Close()

	// Test embedding dimensions.
	embedding, err := e.Embed(context.Background(), "The dog is running in the park")
	if err != nil {
		t.Fatalf("embed: %v", err)
	}

	if len(embedding) != 384 {
		t.Errorf("expected 384 dimensions, got %d", len(embedding))
	}

	// Verify L2 norm is ~1.0.
	norm := 0.0
	for _, v := range embedding {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm < 0.999 || norm > 1.001 {
		t.Errorf("expected L2 norm ≈ 1.0, got %f", norm)
	}

	// Different texts produce different embeddings.
	embedding2, err := e.Embed(context.Background(), "I love eating pizza for dinner")
	if err != nil {
		t.Fatalf("embed2: %v", err)
	}

	same := true
	for i := range embedding {
		if embedding[i] != embedding2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different texts should produce different embeddings")
	}
}

// TestONNXEmbedder_CosineSimilarity tests that similar texts have higher cosine
// similarity than dissimilar texts.
func TestONNXEmbedder_CosineSimilarity(t *testing.T) {
	runtimePath := os.Getenv("ONNXRUNTIME_LIB_PATH")
	if runtimePath == "" {
		t.Skip("ONNXRUNTIME_LIB_PATH not set, skipping integration test")
	}

	modelPath := os.Getenv("ONNX_MODEL_PATH")
	if modelPath == "" {
		modelPath = defaultModelPath()
	}
	vocabPath := os.Getenv("ONNX_VOCAB_PATH")
	if vocabPath == "" {
		vocabPath = defaultVocabPath()
	}

	e, err := NewONNXEmbedder(modelPath, vocabPath, runtimePath)
	if err != nil {
		t.Fatalf("create embedder: %v", err)
	}
	defer e.Close()

	base, err := e.Embed(context.Background(), "The dog is running in the park")
	if err != nil {
		t.Fatalf("embed base: %v", err)
	}

	similar, err := e.Embed(context.Background(), "A dog runs through the park")
	if err != nil {
		t.Fatalf("embed similar: %v", err)
	}

	dissimilar, err := e.Embed(context.Background(), "I love eating pizza for dinner")
	if err != nil {
		t.Fatalf("embed dissimilar: %v", err)
	}

	simToSimilar := cosineSimilarity(base, similar)
	simToDissimilar := cosineSimilarity(base, dissimilar)

	t.Logf("similarity(similar)  = %.4f", simToSimilar)
	t.Logf("similarity(dissimilar) = %.4f", simToDissimilar)

	if simToSimilar <= simToDissimilar {
		t.Errorf("similar text should have higher cosine similarity: similar=%.4f, dissimilar=%.4f",
			simToSimilar, simToDissimilar)
	}
}

// defaultModelPath returns the default model path for testing.
func defaultModelPath() string {
	home, _ := os.UserHomeDir()
	return home + "/.config/echo/models/all-MiniLM-L6-v2.onnx"
}

// defaultVocabPath returns the default vocab path for testing.
func defaultVocabPath() string {
	home, _ := os.UserHomeDir()
	return home + "/.config/echo/models/vocab.txt"
}

// cosineSimilarity computes the cosine similarity between two L2-normalized vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	// Vectors are already L2-normalized, so cosine = dot product.
	return dot
}
