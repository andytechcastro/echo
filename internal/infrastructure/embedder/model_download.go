package embedder

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
)

const (
	// Model download URLs.
	modelURL  = "https://huggingface.co/onnx-models/all-MiniLM-L6-v2-onnx/resolve/main/model.onnx"
	vocabURL  = "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/vocab.txt"

	// Expected SHA256 checksums for verification.
	modelSHA256  = "994a58868f7abacacbf2192aa0aae8f56da8c4505dbde2740c861b24426ede6b"
	vocabSHA256  = "07eced375cec144d27c900241f3e339478dec958f92fddbc551f295c992038a3"

	// Default filenames.
	modelFileName  = "all-MiniLM-L6-v2.onnx"
	vocabFileName  = "vocab.txt"
)

// EnsureModel downloads the ONNX model and vocab.txt if they don't exist.
// modelDir is the directory where files should be stored (e.g., ~/.config/echo/models).
// Returns the paths to the model and vocab files.
func EnsureModel(modelDir string) (modelPath, vocabPath string, err error) {
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create model directory: %w", err)
	}

	modelPath = filepath.Join(modelDir, modelFileName)
	vocabPath = filepath.Join(modelDir, vocabFileName)

	// Download model if not present or checksum mismatch.
	if needsDownload(modelPath, modelSHA256) {
		slog.Info("downloading embedding model", "url", modelURL, "path", modelPath)
		if err := downloadFile(modelURL, modelPath); err != nil {
			return "", "", fmt.Errorf("download model: %w", err)
		}
		if err := verifyChecksum(modelPath, modelSHA256); err != nil {
			return "", "", fmt.Errorf("model checksum verification failed: %w", err)
		}
		slog.Info("model downloaded and verified", "path", modelPath)
	}

	// Download vocab if not present or checksum mismatch.
	if needsDownload(vocabPath, vocabSHA256) {
		slog.Info("downloading tokenizer vocab", "url", vocabURL, "path", vocabPath)
		if err := downloadFile(vocabURL, vocabPath); err != nil {
			return "", "", fmt.Errorf("download vocab: %w", err)
		}
		if err := verifyChecksum(vocabPath, vocabSHA256); err != nil {
			return "", "", fmt.Errorf("vocab checksum verification failed: %w", err)
		}
		slog.Info("vocab downloaded and verified", "path", vocabPath)
	}

	return modelPath, vocabPath, nil
}

// needsDownload returns true if the file doesn't exist or checksum doesn't match.
func needsDownload(path, expectedSHA256 string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true
	}
	// Verify checksum in case file was corrupted.
	return verifyChecksum(path, expectedSHA256) != nil
}

// downloadFile downloads a file from url to path with progress logging.
func downloadFile(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// Create temporary file first, then rename on success.
	tmpPath := path + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		out.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write file: %w", err)
	}
	out.Close()

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename file: %w", err)
	}

	slog.Info("downloaded", "path", path, "bytes", written)
	return nil
}

// verifyChecksum checks the SHA256 of a file against the expected value.
func verifyChecksum(path, expectedSHA256 string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expectedSHA256 {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedSHA256, actual)
	}

	return nil
}
