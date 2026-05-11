package auth

import (
	"errors"
	"fmt"
	"strings"
)

// ErrMissingCredentials is returned when neither a direct access token nor
// a client ID + secret pair was provided.
var ErrMissingCredentials = errors.New("authentication requires either SHOPIFY_ACCESS_TOKEN or both SHOPIFY_CLIENT_ID and SHOPIFY_SECRET")

// MissingScopesError indicates the granted token lacks one or more required scopes.
type MissingScopesError struct {
	Required []string
	Missing  []string
	Granted  []string
}

func (e *MissingScopesError) Error() string {
	return fmt.Sprintf(
		"shopify token missing required scopes: %s (granted: %s)",
		strings.Join(e.Missing, ", "),
		strings.Join(e.Granted, ", "),
	)
}
