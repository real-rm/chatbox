package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Validates: Requirements 1.1, 1.3**
// Property 1: JWT Token Validation
// For any JWT token, the WebSocket_Server should accept the token if and only if
// it has a valid signature and has not expired, and should reject all invalid or
// expired tokens with an unauthorized error.
func TestProperty_JWTTokenValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	validator := NewJWTValidator(testSecret)

	// Property: Valid tokens (correct signature + not expired) should be accepted
	properties.Property("valid tokens with correct signature and not expired should be accepted", prop.ForAll(
		func(userID string, roles []string, expiresInMinutes int) bool {
			// Generate a valid token that expires in the future
			tokenString := createTestToken(userID, roles, time.Duration(expiresInMinutes)*time.Minute)

			claims, err := validator.ValidateToken(tokenString)

			// Should succeed
			if err != nil {
				return false
			}

			// Claims should match
			return claims.UserID == userID && len(claims.Roles) == len(roles)
		},
		genNonEmptyString(),
		genRoles(),
		gen.IntRange(1, 120), // 1 to 120 minutes in the future
	))

	// Property: Expired tokens should be rejected
	properties.Property("expired tokens should be rejected with error", prop.ForAll(
		func(userID string, roles []string, expiredMinutesAgo int) bool {
			// Generate a token that expired in the past
			tokenString := createTestToken(userID, roles, -time.Duration(expiredMinutesAgo)*time.Minute)

			_, err := validator.ValidateToken(tokenString)

			// Should fail with expired error
			return err != nil
		},
		genNonEmptyString(),
		genRoles(),
		gen.IntRange(1, 120), // 1 to 120 minutes ago
	))

	// Property: Tokens with invalid signature should be rejected
	properties.Property("tokens with invalid signature should be rejected with error", prop.ForAll(
		func(userID string, roles []string) bool {
			// Generate a token with wrong secret (invalid signature)
			tokenString := createTokenWithInvalidSignature(userID, roles)

			_, err := validator.ValidateToken(tokenString)

			// Should fail with signature error
			return err != nil
		},
		genNonEmptyString(),
		genRoles(),
	))

	// Property: Malformed tokens should be rejected
	properties.Property("malformed tokens should be rejected with error", prop.ForAll(
		func(malformedToken string) bool {
			// Skip valid JWT-like patterns to ensure we're testing malformed tokens
			if len(malformedToken) > 100 && countDots(malformedToken) == 2 {
				return true // Skip this case
			}

			_, err := validator.ValidateToken(malformedToken)

			// Should fail
			return err != nil
		},
		gen.AnyString(),
	))

	// Property: Empty tokens should be rejected
	properties.Property("empty tokens should be rejected with error", prop.ForAll(
		func() bool {
			_, err := validator.ValidateToken("")

			// Should fail
			return err != nil
		},
	))

	// Property: Tokens missing user_id should be rejected
	properties.Property("tokens missing user_id claim should be rejected", prop.ForAll(
		func(roles []string) bool {
			// Create token without user_id claim
			claims := jwt.MapClaims{
				"roles": roles,
				"exp":   time.Now().Add(time.Hour).Unix(),
				"iat":   time.Now().Unix(),
			}
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
			tokenString, _ := token.SignedString([]byte(testSecret))

			_, err := validator.ValidateToken(tokenString)

			// Should fail
			return err != nil
		},
		genRoles(),
	))

	// Property: Tokens missing roles should be rejected
	properties.Property("tokens missing roles claim should be rejected", prop.ForAll(
		func(userID string) bool {
			// Create token without roles claim
			claims := jwt.MapClaims{
				"user_id": userID,
				"exp":     time.Now().Add(time.Hour).Unix(),
				"iat":     time.Now().Unix(),
			}
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
			tokenString, _ := token.SignedString([]byte(testSecret))

			_, err := validator.ValidateToken(tokenString)

			// Should fail
			return err != nil
		},
		genNonEmptyString(),
	))

	properties.TestingRun(t)
}

// Generator for non-empty strings (user IDs)
func genNonEmptyString() gopter.Gen {
	return gen.Identifier().SuchThat(func(s string) bool {
		return len(s) > 0
	})
}

// Generator for roles (array of strings)
func genRoles() gopter.Gen {
	return gen.SliceOf(gen.Identifier())
}

// Helper function to count dots in a string
func countDots(s string) int {
	count := 0
	for _, c := range s {
		if c == '.' {
			count++
		}
	}
	return count
}

// **Validates: Requirements 1.2**
// Property 2: JWT Claims Extraction Round Trip
// For any valid JWT token with user ID and roles claims, extracting the claims
// from the token should produce values that match the original encoded values.
func TestProperty_JWTClaimsExtractionRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	validator := NewJWTValidator(testSecret)

	// Property: Extracting claims from a valid token should return the original values
	properties.Property("extracting claims from valid token returns original user_id and roles", prop.ForAll(
		func(userID string, roles []string) bool {
			// Create a valid token with the given user ID and roles
			tokenString := createTestToken(userID, roles, time.Hour)

			// Extract claims from the token
			claims, err := validator.ValidateToken(tokenString)

			// Should succeed
			if err != nil {
				return false
			}

			// Extracted user ID should match original
			if claims.UserID != userID {
				return false
			}

			// Extracted roles should match original
			if len(claims.Roles) != len(roles) {
				return false
			}

			for i, role := range roles {
				if claims.Roles[i] != role {
					return false
				}
			}

			return true
		},
		genNonEmptyString(),
		genRoles(),
	))

	// Property: Round trip with empty roles array
	properties.Property("extracting claims with empty roles array preserves empty array", prop.ForAll(
		func(userID string) bool {
			emptyRoles := []string{}
			tokenString := createTestToken(userID, emptyRoles, time.Hour)

			claims, err := validator.ValidateToken(tokenString)

			if err != nil {
				return false
			}

			return claims.UserID == userID && len(claims.Roles) == 0
		},
		genNonEmptyString(),
	))

	// Property: Round trip with multiple roles
	properties.Property("extracting claims with multiple roles preserves all roles in order", prop.ForAll(
		func(userID string, role1 string, role2 string, role3 string) bool {
			roles := []string{role1, role2, role3}
			tokenString := createTestToken(userID, roles, time.Hour)

			claims, err := validator.ValidateToken(tokenString)

			if err != nil {
				return false
			}

			if claims.UserID != userID {
				return false
			}

			if len(claims.Roles) != 3 {
				return false
			}

			return claims.Roles[0] == role1 && claims.Roles[1] == role2 && claims.Roles[2] == role3
		},
		genNonEmptyString(),
		gen.Identifier(),
		gen.Identifier(),
		gen.Identifier(),
	))

	// Property: Round trip with special characters in user ID
	properties.Property("extracting claims with special characters in user_id preserves exact value", prop.ForAll(
		func(prefix string, suffix string, roles []string) bool {
			// Create user ID with special characters
			userID := prefix + "-" + suffix + "@example.com"
			tokenString := createTestToken(userID, roles, time.Hour)

			claims, err := validator.ValidateToken(tokenString)

			if err != nil {
				return false
			}

			return claims.UserID == userID && len(claims.Roles) == len(roles)
		},
		gen.Identifier(),
		gen.Identifier(),
		genRoles(),
	))

	// Property: Round trip preserves role order
	properties.Property("extracting claims preserves exact role order", prop.ForAll(
		func(userID string, roles []string) bool {
			// Skip if roles is empty to focus on order preservation
			if len(roles) == 0 {
				return true
			}

			tokenString := createTestToken(userID, roles, time.Hour)

			claims, err := validator.ValidateToken(tokenString)

			if err != nil {
				return false
			}

			// Check exact order
			for i := range roles {
				if i >= len(claims.Roles) || claims.Roles[i] != roles[i] {
					return false
				}
			}

			return true
		},
		genNonEmptyString(),
		genRoles(),
	))

	properties.TestingRun(t)
}
