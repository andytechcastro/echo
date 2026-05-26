package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Manifest tracks which chunks have been imported from git-synced memories.
// It lives at .echo/manifest.json in the project root.
type Manifest struct {
	Imported []ImportedChunk `json:"imported"`
}

// ImportedChunk represents a chunk that has been imported from git sync.
type ImportedChunk struct {
	ID        string    `json:"id"`
	ImportedAt time.Time `json:"imported_at"`
}

// LoadManifest reads the manifest from the project directory.
// Returns an empty manifest if it doesn't exist.
func LoadManifest(projectDir string) (*Manifest, error) {
	manifestPath := filepath.Join(projectDir, ".echo", "manifest.json")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{Imported: []ImportedChunk{}}, nil
		}
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	return &manifest, nil
}

// SaveManifest writes the manifest to the project directory.
func SaveManifest(projectDir string, manifest *Manifest) error {
	echoDir := filepath.Join(projectDir, ".echo")
	if err := os.MkdirAll(echoDir, 0o755); err != nil {
		return fmt.Errorf("create .echo directory: %w", err)
	}

	manifestPath := filepath.Join(echoDir, "manifest.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}

// IsImported checks if a chunk has already been imported.
func (m *Manifest) IsImported(id string) bool {
	for _, chunk := range m.Imported {
		if chunk.ID == id {
			return true
		}
	}
	return false
}

// MarkImported adds a chunk to the imported list.
func (m *Manifest) MarkImported(id string) {
	m.Imported = append(m.Imported, ImportedChunk{
		ID:        id,
		ImportedAt: time.Now(),
	})
}
