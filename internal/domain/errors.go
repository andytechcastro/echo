package domain

import "errors"

// Domain-level errors used across all layers.
var (
	ErrNotFound       = errors.New("learning not found")
	ErrDuplicate      = errors.New("duplicate learning detected")
	ErrSecretDetected = errors.New("potential secret detected in input")
	ErrInvalidType    = errors.New("invalid learning type")
	ErrInvalidScope   = errors.New("invalid scope")
	ErrScopeForbidden = errors.New("scope 'organization' is admin-only")
	ErrEmptyField     = errors.New("required field is empty")
)

// SecretError wraps a secret detection error with field and pattern info.
type SecretError struct {
	Field   string
	Pattern string
}

func (e *SecretError) Error() string {
	return "potential secret detected in '" + e.Field + "' field (pattern: " + e.Pattern + ")"
}

func (e *SecretError) Is(target error) bool {
	_, ok := target.(*SecretError)
	return ok
}
