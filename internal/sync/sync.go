package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/company/echo/internal/domain"
)

// Chunk represents a git-synced memory chunk.
// These are exported by other Echo instances and committed to git.
type Chunk struct {
	ID        string             `json:"id"`
	Project   string             `json:"project"`
	Type      domain.LearningType `json:"type"`
	Question  string             `json:"question"`
	Answer    string             `json:"answer"`
	Reasoning string             `json:"reasoning"`
	Location  string             `json:"location"`
	Notes     string             `json:"notes"`
	Tags      []string           `json:"tags"`
}

// Syncer handles importing/exporting chunks from git-synced memories.
type Syncer struct {
	store  Store
	logger Logger
}

// Store is the interface for saving learnings.
type Store interface {
	Save(ctx context.Context, input *domain.Learning) (string, error)
}

// Logger is the interface for logging.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}

// NewSyncer creates a new Syncer.
func NewSyncer(store Store, logger Logger) *Syncer {
	return &Syncer{
		store:  store,
		logger: logger,
	}
}

// ImportChunks imports chunks from the .echo/chunks directory.
// It skips chunks that have already been imported (tracked in manifest).
func (s *Syncer) ImportChunks(ctx context.Context, projectDir string) (int, error) {
	manifest, err := LoadManifest(projectDir)
	if err != nil {
		return 0, fmt.Errorf("load manifest: %w", err)
	}

	chunksDir := filepath.Join(projectDir, ".echo", "chunks")
	entries, err := os.ReadDir(chunksDir)
	if err != nil {
		if os.IsNotExist(err) {
			s.logger.Info("no chunks directory, skipping import")
			return 0, nil
		}
		return 0, fmt.Errorf("read chunks directory: %w", err)
	}

	imported := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		chunkPath := filepath.Join(chunksDir, entry.Name())
		data, err := os.ReadFile(chunkPath)
		if err != nil {
			s.logger.Error("failed to read chunk", "file", entry.Name(), "error", err)
			continue
		}

		var chunk Chunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			s.logger.Error("failed to parse chunk", "file", entry.Name(), "error", err)
			continue
		}

		if manifest.IsImported(chunk.ID) {
			continue
		}

		// Save the chunk as a learning.
		learning := &domain.Learning{
			Type:      chunk.Type,
			Question:  chunk.Question,
			Answer:    chunk.Answer,
			Reasoning: chunk.Reasoning,
			Location:  chunk.Location,
			Notes:     chunk.Notes,
			Tags:      chunk.Tags,
		}

		if _, err := s.store.Save(ctx, learning); err != nil {
			s.logger.Error("failed to save chunk", "id", chunk.ID, "error", err)
			continue
		}

		manifest.MarkImported(chunk.ID)
		imported++
	}

	// Save updated manifest.
	if err := SaveManifest(projectDir, manifest); err != nil {
		return imported, fmt.Errorf("save manifest: %w", err)
	}

	s.logger.Info("imported chunks", "count", imported)
	return imported, nil
}

// ExportChunk exports a learning as a chunk file for git sync.
func (s *Syncer) ExportChunk(projectDir string, learning *domain.Learning) error {
	chunksDir := filepath.Join(projectDir, ".echo", "chunks")
	if err := os.MkdirAll(chunksDir, 0o755); err != nil {
		return fmt.Errorf("create chunks directory: %w", err)
	}

	chunk := Chunk{
		ID:        learning.ID,
		Project:   learning.Project,
		Type:      learning.Type,
		Question:  learning.Question,
		Answer:    learning.Answer,
		Reasoning: learning.Reasoning,
		Location:  learning.Location,
		Notes:     learning.Notes,
		Tags:      learning.Tags,
	}

	data, err := json.MarshalIndent(chunk, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal chunk: %w", err)
	}

	chunkPath := filepath.Join(chunksDir, learning.ID+".json")
	if err := os.WriteFile(chunkPath, data, 0o644); err != nil {
		return fmt.Errorf("write chunk: %w", err)
	}

	return nil
}
