package detector

import (
	"os/exec"
	"regexp"
	"strings"
)

// GitProjectDetector implements domain.ProjectDetector by reading the git remote URL.
type GitProjectDetector struct {
	// workDir is the directory where git commands are executed.
	// If empty, uses the current working directory.
	workDir string
}

// NewGitProjectDetector creates a new detector.
func NewGitProjectDetector(workDir string) *GitProjectDetector {
	return &GitProjectDetector{workDir: workDir}
}

// Detect returns the normalized project ID from `git remote get-url origin`.
// It strips protocols, auth credentials, and .git suffixes.
//
// Examples:
//
//	git@github.com:empresa/repo-x.git  →  github.com/empresa/repo-x
//	https://github.com/empresa/repo-x.git  →  github.com/empresa/repo-x
//	https://user:token@github.com/empresa/repo-x  →  github.com/empresa/repo-x
//	git@gitlab.internal:team/backend.git  →  gitlab.internal/team/backend
func (d *GitProjectDetector) Detect() (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = d.workDir

	output, err := cmd.Output()
	if err != nil {
		return "", &DetectError{
			Component: "project",
			Reason:    "failed to run git remote get-url origin: " + err.Error(),
		}
	}

	rawURL := strings.TrimSpace(string(output))
	return normalizeProjectURL(rawURL), nil
}

// normalizeProjectURL converts a git remote URL to a canonical project ID.
func normalizeProjectURL(rawURL string) string {
	// Strip protocols and auth.
	// Matches: git@, https://, http://, git://, ssh://, user:pass@
	re := regexp.MustCompile(`^(?:git@|https?://|git://|ssh://)(?:[^@/]+@)?`)
	cleaned := re.ReplaceAllString(rawURL, "")

	// Replace : with / (SSH URLs like git@github.com:org/repo.git)
	cleaned = strings.Replace(cleaned, ":", "/", 1)

	// Remove trailing .git
	cleaned = strings.TrimSuffix(cleaned, ".git")

	// Lowercase for consistency.
	cleaned = strings.ToLower(cleaned)

	return cleaned
}

// DetectError is returned when detection fails.
type DetectError struct {
	Component string
	Reason    string
}

func (e *DetectError) Error() string {
	return "detect " + e.Component + ": " + e.Reason
}
