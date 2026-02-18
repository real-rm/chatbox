package util

import (
	"errors"
	"strings"
)

var (
	// ErrMissingAuthHeader is returned when the Authorization header is missing
	ErrMissingAuthHeader = errors.New("missing Authorization header")
	// ErrInvalidAuthHeader is returned when the Authorization header format is invalid
	ErrInvalidAuthHeader = errors.New("invalid Authorization header format")
)

// ExtractBearerToken extracts the JWT token from an Authorization header.
// It expects the format "Bearer <token>" and returns the token part.
//
// Returns:
//   - token string if successful
//   - error if header is missing or malformed
//
// Example:
//
//	token, err := util.ExtractBearerToken(authHeader)
//	if err != nil {
//	    return err
//	}
func ExtractBearerToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", ErrMissingAuthHeader
	}

	// Check for "Bearer " prefix
	const bearerPrefix = "Bearer "
	const bearerPrefixLen = 7

	if len(authHeader) <= bearerPrefixLen || authHeader[:bearerPrefixLen] != bearerPrefix {
		return "", ErrInvalidAuthHeader
	}

	token := authHeader[bearerPrefixLen:]
	if token == "" {
		return "", ErrInvalidAuthHeader
	}

	return token, nil
}

// HasRole checks if a user has any of the specified roles.
// This is useful for authorization checks.
//
// Example:
//
//	if util.HasRole(claims.Roles, "admin", "chat_admin") {
//	    // User has admin access
//	}
func HasRole(userRoles []string, requiredRoles ...string) bool {
	roleMap := make(map[string]bool, len(userRoles))
	for _, role := range userRoles {
		roleMap[role] = true
	}

	for _, required := range requiredRoles {
		if roleMap[required] {
			return true
		}
	}

	return false
}

// ContainsWeakPattern checks if a string contains any weak patterns.
// This is used for password and secret validation.
//
// Example:
//
//	if util.ContainsWeakPattern(secret, weakSecrets) {
//	    return errors.New("secret contains weak pattern")
//	}
func ContainsWeakPattern(s string, weakPatterns []string) (bool, string) {
	lowerS := strings.ToLower(s)
	for _, pattern := range weakPatterns {
		if strings.Contains(lowerS, pattern) {
			return true, pattern
		}
	}
	return false, ""
}
