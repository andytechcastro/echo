package embedder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureModel_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	modelDir := filepath.Join(tmpDir, "models", "nested")

	modelPath, vocabPath, err := EnsureModel(modelDir)
	// Will fail to download in test env (no network or wrong checksums), but directory should be created.
	if _, statErr := os.Stat(modelDir); os.IsNotExist(statErr) {
		t.Error("model directory should have been created")
	}

	// Paths should be set correctly even if download fails.
	expectedModel := filepath.Join(modelDir, modelFileName)
	expectedVocab := filepath.Join(modelDir, vocabFileName)
	if modelPath != expectedModel {
		t.Errorf("modelPath = %s, want %s", modelPath, expectedModel)
	}
	if vocabPath != expectedVocab {
		t.Errorf("vocabPath = %s, want %s", vocabPath, expectedVocab)
	}

	// Suppress unused variable warning.
	_ = err
}

func TestVerifyChecksum_CorrectFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")

	// Create a file with known content.
	content := []byte("hello world")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Calculate expected checksum.
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if err := verifyChecksum(path, expected); err != nil {
		t.Errorf("expected checksum to match: %v", err)
	}
}

func TestVerifyChecksum_WrongChecksum(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")

	content := []byte("hello world")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := verifyChecksum(path, "wrongchecksum"); err == nil {
		t.Error("expected checksum mismatch error")
	}
}

func TestNeedsDownload_MissingFile(t *testing.T) {
	if !needsDownload("/nonexistent/file.txt", "any") {
		t.Error("should return true for missing file")
	}
}
