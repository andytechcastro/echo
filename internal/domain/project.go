package domain

// ProjectDetector is the port for detecting the current project from the working directory.
type ProjectDetector interface {
	// Detect returns the normalized project ID (e.g., "github.com/company/repo").
	Detect() (string, error)
}

// IdentityDetector is the port for detecting the current user's identity.
type IdentityDetector interface {
	// Detect returns the resolved-by identity (e.g., "andres" or "andres@company.com").
	Detect() (string, error)
}
