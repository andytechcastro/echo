package config

import (
	"os"
	"path/filepath"
)

// Config holds the application configuration.
type Config struct {
	// DataDir is the directory where Echo stores its data.
	// Default: ~/.config/echo
	DataDir string

	// DBPath is the full path to the SQLite database file.
	// Default: ~/.config/echo/echo.db
	DBPath string

	// Mode is the operating mode: "local", "embeddings", or "cloud".
	// Default: "local"
	Mode string

	// Embedder is the embedding provider: "local", "vertex-ai", "openai", or "cohere".
	// Default: "local"
	Embedder string

	// ModelPath is the path to the ONNX model file.
	// Default: ~/.config/echo/models/all-MiniLM-L6-v2.onnx
	ModelPath string

	// VocabPath is the path to the WordPiece vocab.txt file.
	// Default: ~/.config/echo/models/vocab.txt
	VocabPath string

	// LogLevel is the logging level: "debug", "info", "warn", "error".
	// Default: "info"
	LogLevel string
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "/tmp"
	}

	dataDir := filepath.Join(homeDir, ".config", "echo")
	modelDir := filepath.Join(dataDir, "models")

	return &Config{
		DataDir:   dataDir,
		DBPath:    filepath.Join(dataDir, "echo.db"),
		Mode:      "local",
		Embedder:  "local",
		ModelPath: filepath.Join(modelDir, "all-MiniLM-L6-v2.onnx"),
		VocabPath: filepath.Join(modelDir, "vocab.txt"),
		LogLevel:  "info",
	}
}

// Load loads configuration from environment variables, overriding defaults.
func Load() *Config {
	cfg := Default()

	if v := os.Getenv("ECHO_DATA_DIR"); v != "" {
		cfg.DataDir = v
		cfg.DBPath = filepath.Join(v, "echo.db")
		modelDir := filepath.Join(v, "models")
		cfg.ModelPath = filepath.Join(modelDir, "all-MiniLM-L6-v2.onnx")
		cfg.VocabPath = filepath.Join(modelDir, "vocab.txt")
	}

	if v := os.Getenv("ECHO_MODE"); v != "" {
		cfg.Mode = v
	}

	if v := os.Getenv("ECHO_EMBEDDER"); v != "" {
		cfg.Embedder = v
	}

	if v := os.Getenv("ECHO_MODEL_PATH"); v != "" {
		cfg.ModelPath = v
	}

	if v := os.Getenv("ECHO_VOCAB_PATH"); v != "" {
		cfg.VocabPath = v
	}

	if v := os.Getenv("ECHO_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	return cfg
}
