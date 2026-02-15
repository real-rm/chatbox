package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret-key-for-jwt-validation"

// Helper function to create a valid JWT token for testing
func createTestToken(userID string, roles []string, expiresIn time.Duration) string {
	claims := jwt.MapClaims{
		"user_id": userID,
		"roles":   roles,
		"exp":     time.Now().Add(expiresIn).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testSecret))
	return tokenString
}

// Helper function to create a token with invalid signature
func createTokenWithInvalidSignature(userID string, roles []string) string {
	claims := jwt.MapClaims{
		"user_id": userID,
		"roles":   roles,
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte("wrong-secret"))
	return tokenString
}

func TestValidateToken_ValidToken(t *testing.T) {
	validator := NewJWTValidator(testSecret)

	tokenString := createTestToken("user-123", []string{"user"}, time.Hour)

	claims, err := validator.ValidateToken(tokenString)

	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
	assert.Equal(t, []string{"user"}, claims.Roles)
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	validator := NewJWTValidator(testSecret)

	// Create token that expired 1 hour ago
	tokenString := createTestToken("user-123", []string{"user"}, -time.Hour)

	_, err := validator.ValidateToken(tokenString)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestValidateToken_InvalidSignature(t *testing.T) {
	validator := NewJWTValidator(testSecret)

	tokenString := createTokenWithInvalidSignature("user-123", []string{"user"})

	_, err := validator.ValidateToken(tokenString)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature")
}

func TestValidateToken_MalformedToken(t *testing.T) {
	validator := NewJWTValidator(testSecret)

	_, err := validator.ValidateToken("not-a-valid-jwt-token")

	require.Error(t, err)
}

func TestValidateToken_EmptyToken(t *testing.T) {
	validator := NewJWTValidator(testSecret)

	_, err := validator.ValidateToken("")

	require.Error(t, err)
}

func TestValidateToken_MissingUserID(t *testing.T) {
	validator := NewJWTValidator(testSecret)

	// Create token without user_id claim
	claims := jwt.MapClaims{
		"roles": []string{"user"},
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testSecret))

	_, err := validator.ValidateToken(tokenString)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_id")
}

func TestValidateToken_MissingRoles(t *testing.T) {
	validator := NewJWTValidator(testSecret)

	// Create token without roles claim
	claims := jwt.MapClaims{
		"user_id": "user-123",
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testSecret))

	_, err := validator.ValidateToken(tokenString)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "roles")
}

func TestValidateToken_InvalidRolesType(t *testing.T) {
	validator := NewJWTValidator(testSecret)

	// Create token with roles as string instead of array
	claims := jwt.MapClaims{
		"user_id": "user-123",
		"roles":   "user", // Should be []string
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testSecret))

	_, err := validator.ValidateToken(tokenString)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "roles")
}

func TestValidateToken_MultipleRoles(t *testing.T) {
	validator := NewJWTValidator(testSecret)

	tokenString := createTestToken("admin-456", []string{"user", "admin"}, time.Hour)

	claims, err := validator.ValidateToken(tokenString)

	require.NoError(t, err)
	assert.Equal(t, "admin-456", claims.UserID)
	assert.Equal(t, []string{"user", "admin"}, claims.Roles)
}

func TestValidateToken_EmptyRolesArray(t *testing.T) {
	validator := NewJWTValidator(testSecret)

	tokenString := createTestToken("user-789", []string{}, time.Hour)

	claims, err := validator.ValidateToken(tokenString)

	require.NoError(t, err)
	assert.Equal(t, "user-789", claims.UserID)
	assert.Equal(t, []string{}, claims.Roles)
}

func TestExtractClaims_RoundTrip(t *testing.T) {
	validator := NewJWTValidator(testSecret)

	originalUserID := "user-999"
	originalRoles := []string{"user", "moderator"}

	tokenString := createTestToken(originalUserID, originalRoles, time.Hour)

	claims, err := validator.ValidateToken(tokenString)

	require.NoError(t, err)
	assert.Equal(t, originalUserID, claims.UserID)
	assert.Equal(t, originalRoles, claims.Roles)
}
