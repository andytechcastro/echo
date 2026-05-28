// Package embedder provides text-to-vector embedding implementations.
package embedder

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
	"github.com/buckhx/gobert/tokenize"
	"github.com/buckhx/gobert/tokenize/vocab"
)

const onnxModelDimensions = 384
const maxSeqLen = 256

// ONNXEmbedder implements domain.Embedder using all-MiniLM-L6-v2 ONNX model.
// Requires:
//  1. ONNX Runtime shared library (libonnxruntime.so)
//  2. all-MiniLM-L6-v2 ONNX model file
//  3. vocab.txt WordPiece vocabulary file
type ONNXEmbedder struct {
	tk                  tokenize.VocabTokenizer
	session             *ort.AdvancedSession
	inputTensor         *ort.Tensor[int64]
	attentionMaskTensor *ort.Tensor[int64]
	tokenTypeIdsTensor  *ort.Tensor[int64]
	outputTensor        *ort.Tensor[float32]
	dimensions          int
	provider            string
	mu                  sync.Mutex
}

// NewONNXEmbedder initializes the ONNX Runtime and loads the model.
// modelPath: path to all-MiniLM-L6-v2 ONNX model file.
// vocabPath: path to vocab.txt tokenizer vocabulary.
// runtimePath: path to libonnxruntime.so (optional, empty uses default search).
func NewONNXEmbedder(modelPath, vocabPath, runtimePath string) (*ONNXEmbedder, error) {
	// Validate files exist.
	for name, path := range map[string]string{
		"model": modelPath,
		"vocab": vocabPath,
	} {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("%s file not found at %s: %w", name, path, err)
		}
	}

	// Set ONNX Runtime shared library path.
	if runtimePath != "" {
		ort.SetSharedLibraryPath(runtimePath)
	}

	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("initialize ONNX runtime: %w", err)
	}

	// Load WordPiece vocabulary and tokenizer.
	voc, err := vocab.FromFile(vocabPath)
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("load vocab: %w", err)
	}
	tk := tokenize.NewTokenizer(voc, tokenize.WithLower(true))

	// Create input tensors: shape [1, maxSeqLen].
	inputShape := ort.NewShape(int64(1), int64(maxSeqLen))

	inputTensor, err := ort.NewTensor[int64](inputShape, nil)
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create input tensor: %w", err)
	}

	attentionMaskTensor, err := ort.NewTensor[int64](inputShape, nil)
	if err != nil {
		inputTensor.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create attention mask tensor: %w", err)
	}

	tokenTypeIdsTensor, err := ort.NewTensor[int64](inputShape, nil)
	if err != nil {
		inputTensor.Destroy()
		attentionMaskTensor.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create token type ids tensor: %w", err)
	}

	// Output tensor: shape [1, maxSeqLen, 384].
	outputShape := ort.NewShape(int64(1), int64(maxSeqLen), int64(onnxModelDimensions))
	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		inputTensor.Destroy()
		attentionMaskTensor.Destroy()
		tokenTypeIdsTensor.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create output tensor: %w", err)
	}

	// Create ONNX session.
	session, err := ort.NewAdvancedSession(modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		[]ort.Value{inputTensor, attentionMaskTensor, tokenTypeIdsTensor},
		[]ort.Value{outputTensor},
		nil,
	)
	if err != nil {
		inputTensor.Destroy()
		attentionMaskTensor.Destroy()
		tokenTypeIdsTensor.Destroy()
		outputTensor.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create ONNX session: %w", err)
	}

	slog.Info("ONNX embedder initialized",
		"model", "all-MiniLM-L6-v2",
		"dimensions", onnxModelDimensions,
		"max_seq_len", maxSeqLen,
	)

	return &ONNXEmbedder{
		tk:                  tk,
		session:             session,
		inputTensor:         inputTensor,
		attentionMaskTensor: attentionMaskTensor,
		tokenTypeIdsTensor:  tokenTypeIdsTensor,
		outputTensor:        outputTensor,
		dimensions:          onnxModelDimensions,
		provider:            "local-onnx",
	}, nil
}

// Embed generates a 384-dimensional L2-normalized vector for the given text.
func (e *ONNXEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("empty text")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Tokenize text.
	tokens := e.tk.Tokenize(text)
	voc := e.tk.Vocab()

	// Build input IDs: [CLS] + tokens + [SEP].
	clsID := int64(voc.GetID("[CLS]"))
	sepID := int64(voc.GetID("[SEP]"))
	ids := make([]int64, 0, len(tokens)+2)
	ids = append(ids, clsID)
	for _, token := range tokens {
		id := voc.GetID(token)
		ids = append(ids, int64(id))
	}
	ids = append(ids, sepID)

	// Truncate to max sequence length.
	if len(ids) > maxSeqLen {
		ids = ids[:maxSeqLen]
	}

	seqLen := len(ids)

	// Build attention mask and token type IDs.
	attentionMask := make([]int64, maxSeqLen)
	for i := 0; i < seqLen; i++ {
		attentionMask[i] = 1
	}
	tokenTypeIds := make([]int64, maxSeqLen) // all zeros for single sentence

	// Fill input tensors.
	inputData := e.inputTensor.GetData()
	attentionData := e.attentionMaskTensor.GetData()
	typeData := e.tokenTypeIdsTensor.GetData()

	for i := range inputData {
		inputData[i] = 0
		attentionData[i] = 0
		typeData[i] = 0
	}

	for i, id := range ids {
		inputData[i] = id
	}
	for i, mask := range attentionMask {
		attentionData[i] = mask
	}
	for i, typeID := range tokenTypeIds {
		typeData[i] = typeID
	}

	// Run ONNX inference.
	if err := e.session.Run(); err != nil {
		return nil, fmt.Errorf("run ONNX inference: %w", err)
	}

	// Output shape: [1, maxSeqLen, 384]. Mean-pool over actual seqLen.
	outputData := e.outputTensor.GetData()

	embedding := make([]float32, onnxModelDimensions)
	var maskSum float64

	for i := 0; i < seqLen; i++ {
		if attentionMask[i] == 0 {
			continue
		}
		maskSum++
		offset := i * onnxModelDimensions
		for j := 0; j < onnxModelDimensions; j++ {
			embedding[j] += outputData[offset+j]
		}
	}

	if maskSum > 0 {
		for j := range embedding {
			embedding[j] = float32(float64(embedding[j]) / maskSum)
		}
	}

	// L2 normalize to unit vector.
	l2Normalize(embedding)

	return embedding, nil
}

// Dimensions returns the embedding vector size (384 for all-MiniLM-L6-v2).
func (e *ONNXEmbedder) Dimensions() int {
	return e.dimensions
}

// Provider returns the human-readable provider name.
func (e *ONNXEmbedder) Provider() string {
	return e.provider
}

// Close releases all ONNX Runtime resources.
// Safe to call multiple times or on a partially initialized embedder.
func (e *ONNXEmbedder) Close() error {
	if e.session != nil {
		e.session.Destroy()
		e.session = nil
	}
	if e.inputTensor != nil {
		e.inputTensor.Destroy()
		e.inputTensor = nil
	}
	if e.attentionMaskTensor != nil {
		e.attentionMaskTensor.Destroy()
		e.attentionMaskTensor = nil
	}
	if e.tokenTypeIdsTensor != nil {
		e.tokenTypeIdsTensor.Destroy()
		e.tokenTypeIdsTensor = nil
	}
	if e.outputTensor != nil {
		e.outputTensor.Destroy()
		e.outputTensor = nil
	}
	ort.DestroyEnvironment()
	return nil
}

// l2Normalize normalizes a vector to unit length in-place.
func l2Normalize(v []float32) {
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if norm == 0 {
		return
	}
	norm = math.Sqrt(norm)
	for i := range v {
		v[i] = float32(float64(v[i]) / norm)
	}
}
