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

	// Embedder is the embedding provider: "vertex-ai", "openai", or "cohere".
	// Default: "vertex-ai"
	Embedder string

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

	return &Config{
		DataDir:  dataDir,
		DBPath:   filepath.Join(dataDir, "echo.db"),
		Mode:     "local",
		Embedder: "vertex-ai",
		LogLevel: "info",
	}
}

// Load loads configuration from environment variables, overriding defaults.
func Load() *Config {
	cfg := Default()

	if v := os.Getenv("ECHO_DATA_DIR"); v != "" {
		cfg.DataDir = v
		cfg.DBPath = filepath.Join(v, "echo.db")
	}

	if v := os.Getenv("ECHO_MODE"); v != "" {
		cfg.Mode = v
	}

	if v := os.Getenv("ECHO_EMBEDDER"); v != "" {
		cfg.Embedder = v
	}

	if v := os.Getenv("ECHO_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	return cfg
}
