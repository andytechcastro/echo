package detector

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupGitRepo creates a temporary directory with a git repo and the given remote URL.
func setupGitRepo(t *testing.T, remoteURL string) string {
	t.Helper()

	dir := t.TempDir()

	// Initialize git repo.
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	// Add remote.
	if remoteURL != "" {
		runGit(t, dir, "remote", "add", "origin", remoteURL)
	}

	return dir
}

// runGit runs a git command in the given directory.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
}

func TestGitProjectDetector_Detect_SSH(t *testing.T) {
	dir := setupGitRepo(t, "git@github.com:empresa/repo-x.git")

	detector := NewGitProjectDetector(dir)
	result, err := detector.Detect()
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}

	want := "github.com/empresa/repo-x"
	if result != want {
		t.Errorf("Detect() = %v, want %v", result, want)
	}
}

func TestGitProjectDetector_Detect_HTTPS(t *testing.T) {
	dir := setupGitRepo(t, "https://github.com/empresa/repo-x.git")

	detector := NewGitProjectDetector(dir)
	result, err := detector.Detect()
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}

	want := "github.com/empresa/repo-x"
	if result != want {
		t.Errorf("Detect() = %v, want %v", result, want)
	}
}

func TestGitProjectDetector_Detect_HTTPS_WithToken(t *testing.T) {
	dir := setupGitRepo(t, "https://user:ghp_123@github.com/empresa/repo-x.git")

	detector := NewGitProjectDetector(dir)
	result, err := detector.Detect()
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}

	want := "github.com/empresa/repo-x"
	if result != want {
		t.Errorf("Detect() = %v, want %v", result, want)
	}
}

func TestGitProjectDetector_Detect_GitProtocol(t *testing.T) {
	dir := setupGitRepo(t, "git://github.com/empresa/repo-x.git")

	detector := NewGitProjectDetector(dir)
	result, err := detector.Detect()
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}

	want := "github.com/empresa/repo-x"
	if result != want {
		t.Errorf("Detect() = %v, want %v", result, want)
	}
}

func TestGitProjectDetector_Detect_SelfHosted(t *testing.T) {
	dir := setupGitRepo(t, "git@gitlab.internal:team/backend.git")

	detector := NewGitProjectDetector(dir)
	result, err := detector.Detect()
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}

	want := "gitlab.internal/team/backend"
	if result != want {
		t.Errorf("Detect() = %v, want %v", result, want)
	}
}

func TestGitProjectDetector_Detect_NoRemote(t *testing.T) {
	dir := setupGitRepo(t, "") // No remote.

	detector := NewGitProjectDetector(dir)
	_, err := detector.Detect()
	if err == nil {
		t.Error("Detect() should return error when no remote exists")
	}
}

func TestGitProjectDetector_Detect_NotGitRepo(t *testing.T) {
	dir := t.TempDir() // Not a git repo.

	detector := NewGitProjectDetector(dir)
	_, err := detector.Detect()
	if err == nil {
		t.Error("Detect() should return error when not in a git repo")
	}
}

func TestNormalizeProjectURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "SSH URL",
			input: "git@github.com:empresa/repo-x.git",
			want:  "github.com/empresa/repo-x",
		},
		{
			name:  "HTTPS URL",
			input: "https://github.com/empresa/repo-x.git",
			want:  "github.com/empresa/repo-x",
		},
		{
			name:  "HTTPS with token",
			input: "https://user:ghp_123@github.com/empresa/repo-x",
			want:  "github.com/empresa/repo-x",
		},
		{
			name:  "Git protocol",
			input: "git://github.com/empresa/repo-x.git",
			want:  "github.com/empresa/repo-x",
		},
		{
			name:  "Self-hosted GitLab",
			input: "git@gitlab.internal:team/backend.git",
			want:  "gitlab.internal/team/backend",
		},
		{
			name:  "Already normalized",
			input: "github.com/empresa/repo-x",
			want:  "github.com/empresa/repo-x",
		},
		{
			name:  "Uppercase gets lowercased",
			input: "https://github.com/Empresa/Repo-X.git",
			want:  "github.com/empresa/repo-x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeProjectURL(tt.input)
			if got != tt.want {
				t.Errorf("normalizeProjectURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGitIdentityDetector_Detect(t *testing.T) {
	dir := setupGitRepo(t, "https://github.com/test/repo.git")
	// setupGitRepo already sets user.name = "Test User".

	detector := NewGitIdentityDetector(dir)
	result, err := detector.Detect()
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}

	want := "Test User"
	if result != want {
		t.Errorf("Detect() = %v, want %v", result, want)
	}
}

func TestGitIdentityDetector_Detect_Email(t *testing.T) {
	dir := setupGitRepo(t, "https://github.com/test/repo.git")

	// Override user.name to empty, so it falls back to email.
	runGit(t, dir, "config", "user.name", "")

	detector := NewGitIdentityDetector(dir)
	result, err := detector.Detect()
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}

	want := "test@example.com"
	if result != want {
		t.Errorf("Detect() = %v, want %v", result, want)
	}
}

func TestGitIdentityDetector_Detect_Whoami(t *testing.T) {
	dir := t.TempDir() // Not a git repo.

	detector := NewGitIdentityDetector(dir)
	result, err := detector.Detect()

	// Should fall back to whoami, which should not be "unknown".
	if err != nil {
		// If whoami also fails, we get a DetectError.
		t.Logf("Detect() returned error (expected if whoami fails): %v", err)
	}

	if result == "" {
		t.Error("Detect() should return a non-empty result")
	}
}

func TestGitIdentityDetector_Detect_NotGitRepo(t *testing.T) {
	dir := t.TempDir() // Not a git repo.

	// Create a fake whoami command by ensuring the system has one.
	// This test verifies that even without git config, it falls back gracefully.
	detector := NewGitIdentityDetector(dir)
	result, err := detector.Detect()

	// Should not error if whoami works.
	if err != nil {
		// Error is acceptable if whoami is not available.
		t.Logf("Detect() returned error: %v", err)
	}

	// Result should be either a valid identity or "unknown".
	if result == "" {
		t.Error("Detect() should return a non-empty result or error")
	}
}

// Test that the detector implements the interface.
var _ interface {
	Detect() (string, error)
} = (*GitProjectDetector)(nil)

var _ interface {
	Detect() (string, error)
} = (*GitIdentityDetector)(nil)

// TestDetectError tests the error type.
func TestDetectError(t *testing.T) {
	err := &DetectError{Component: "project", Reason: "test reason"}

	want := "detect project: test reason"
	if err.Error() != want {
		t.Errorf("Error() = %v, want %v", err.Error(), want)
	}
}
}

// TestMkdirAll tests that the store creates directories.
func TestMkdirAll(t *testing.T) {
	// Create a path that doesn't exist.
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "subdir", "nested", "echo.db")

	store, err := NewSQLiteFTS5Store(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteFTS5Store() error: %v", err)
	}
	defer store.Close()

	// Verify the directory was created.
	if _, err := os.Stat(filepath.Dir(dbPath)); os.IsNotExist(err) {
		t.Error("Directory should have been created")
	}
}
