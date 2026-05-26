package detector

import (
	"os/exec"
	"strings"
)

// GitIdentityDetector implements domain.IdentityDetector by reading git config.
type GitIdentityDetector struct {
	// workDir is the directory where git commands are executed.
	// If empty, uses the current working directory.
	workDir string
}

// NewGitIdentityDetector creates a new detector.
func NewGitIdentityDetector(workDir string) *GitIdentityDetector {
	return &GitIdentityDetector{workDir: workDir}
}

// Detect returns the current user's identity.
// It tries `git config user.name` first, falls back to `git config user.email`.
// If neither is available, falls back to the OS username.
func (d *GitIdentityDetector) Detect() (string, error) {
	// Try user.name first.
	name, err := d.gitConfig("user.name")
	if err == nil && name != "" {
		return name, nil
	}

	// Fallback to user.email.
	email, err := d.gitConfig("user.email")
	if err == nil && email != "" {
		return email, nil
	}

	// Fallback to OS username.
	whoami, err := exec.Command("whoami").Output()
	if err == nil {
		return strings.TrimSpace(string(whoami)), nil
	}

	return "unknown", &DetectError{
		Component: "identity",
		Reason:    "could not detect user identity (git config and whoami both failed)",
	}
}

// gitConfig runs `git config --get <key>` and returns the trimmed value.
func (d *GitIdentityDetector) gitConfig(key string) (string, error) {
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = d.workDir

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}
