package auth

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

var (
	// ErrInvalidToken is returned when the token is malformed or invalid
	ErrInvalidToken = errors.New("invalid token")
	// ErrExpiredToken is returned when the token has expired
	ErrExpiredToken = errors.New("token has expired")
	// ErrInvalidSignature is returned when the token signature is invalid
	ErrInvalidSignature = errors.New("invalid token signature")
	// ErrMissingClaims is returned when required claims are missing
	ErrMissingClaims = errors.New("missing required claims")
)

// Claims represents the JWT claims extracted from a token
type Claims struct {
	UserID string
	Name   string
	Roles  []string
}

// JWTValidator handles JWT token validation
type JWTValidator struct {
	secret []byte
}

// NewJWTValidator creates a new JWT validator with the given secret
func NewJWTValidator(secret string) *JWTValidator {
	return &JWTValidator{
		secret: []byte(secret),
	}
}

// ValidateToken validates a JWT token and extracts the claims
// It verifies:
// - Token signature
// - Token expiration
// - Required claims (user_id, roles)
func (v *JWTValidator) ValidateToken(tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, fmt.Errorf("%w: empty token", ErrInvalidToken)
	}

	// Parse and validate the token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		// No else needed: early return pattern (guard clause)
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method: %v", ErrInvalidSignature, token.Header["alg"])
		}
		return v.secret, nil
	})

	// No else needed: early return pattern (guard clause)
	if err != nil {
		// Check for specific error types
		// No else needed: early return pattern (guard clause)
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, fmt.Errorf("%w: %v", ErrExpiredToken, err)
		}
		// No else needed: early return pattern (guard clause)
		if errors.Is(err, jwt.ErrSignatureInvalid) {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSignature, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	// No else needed: early return pattern (guard clause)
	if !token.Valid {
		return nil, fmt.Errorf("%w: token is not valid", ErrInvalidToken)
	}

	// Extract claims
	mapClaims, ok := token.Claims.(jwt.MapClaims)
	// No else needed: early return pattern (guard clause)
	if !ok {
		return nil, fmt.Errorf("%w: unable to parse claims", ErrInvalidToken)
	}

	// Extract user_id
	userID, ok := mapClaims["user_id"].(string)
	// No else needed: early return pattern (guard clause)
	if !ok || userID == "" {
		return nil, fmt.Errorf("%w: user_id claim missing or invalid", ErrMissingClaims)
	}

	// Extract name (optional field)
	name, _ := mapClaims["name"].(string)
	// No else needed: optional operation (set default value)
	// If name is not present or empty, default to user_id
	if name == "" {
		name = userID
	}

	// Extract roles
	rolesInterface, ok := mapClaims["roles"]
	// No else needed: early return pattern (guard clause)
	if !ok {
		return nil, fmt.Errorf("%w: roles claim missing", ErrMissingClaims)
	}

	// Convert roles to []string
	roles, err := extractRoles(rolesInterface)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMissingClaims, err)
	}

	return &Claims{
		UserID: userID,
		Name:   name,
		Roles:  roles,
	}, nil
}

// extractRoles converts the roles claim to a string slice
func extractRoles(rolesInterface interface{}) ([]string, error) {
	// Handle []interface{} (common JWT claim format)
	// No else needed: type assertion with specific handling, continues to next check if false
	if rolesSlice, ok := rolesInterface.([]interface{}); ok {
		roles := make([]string, len(rolesSlice))
		for i, role := range rolesSlice {
			roleStr, ok := role.(string)
			// No else needed: early return pattern (guard clause)
			if !ok {
				return nil, fmt.Errorf("roles array contains non-string value at index %d", i)
			}
			roles[i] = roleStr
		}
		return roles, nil
	}

	// Handle []string (less common but possible)
	// No else needed: type assertion with specific handling, continues to error if false
	if rolesSlice, ok := rolesInterface.([]string); ok {
		return rolesSlice, nil
	}

	return nil, fmt.Errorf("roles claim must be an array of strings")
}
