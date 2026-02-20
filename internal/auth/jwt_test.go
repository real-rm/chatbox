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

func TestValidateToken_WithName(t *testing.T) {
	validator := NewJWTValidator(testSecret)

	// Create token with name claim
	claims := jwt.MapClaims{
		"user_id": "admin-123",
		"name":    "John Admin",
		"roles":   []string{"admin"},
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testSecret))

	extractedClaims, err := validator.ValidateToken(tokenString)

	require.NoError(t, err)
	assert.Equal(t, "admin-123", extractedClaims.UserID)
	assert.Equal(t, "John Admin", extractedClaims.Name)
	assert.Equal(t, []string{"admin"}, extractedClaims.Roles)
}

func TestValidateToken_WithoutName(t *testing.T) {
	validator := NewJWTValidator(testSecret)

	// Create token without name claim - should default to user_id
	tokenString := createTestToken("user-456", []string{"user"}, time.Hour)

	extractedClaims, err := validator.ValidateToken(tokenString)

	require.NoError(t, err)
	assert.Equal(t, "user-456", extractedClaims.UserID)
	assert.Equal(t, "user-456", extractedClaims.Name) // Should default to user_id
	assert.Equal(t, []string{"user"}, extractedClaims.Roles)
}

// TestExtractRoles covers all branches of the extractRoles internal function.
// Since extractRoles is package-private, we test it directly here.
func TestExtractRoles(t *testing.T) {
	tests := []struct {
		name      string
		input     interface{}
		wantRoles []string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "[]interface{} with strings — normal JWT claim format",
			input:     []interface{}{"user", "admin"},
			wantRoles: []string{"user", "admin"},
			wantErr:   false,
		},
		{
			name:      "empty []interface{}",
			input:     []interface{}{},
			wantRoles: []string{},
			wantErr:   false,
		},
		{
			name:    "[]interface{} with non-string element",
			input:   []interface{}{"user", 42},
			wantErr: true,
			errMsg:  "non-string value at index 1",
		},
		{
			name:      "[]string — direct string slice (less common)",
			input:     []string{"admin", "moderator"},
			wantRoles: []string{"admin", "moderator"},
			wantErr:   false,
		},
		{
			name:    "string — invalid type",
			input:   "user",
			wantErr: true,
			errMsg:  "must be an array of strings",
		},
		{
			name:    "nil — invalid type",
			input:   nil,
			wantErr: true,
			errMsg:  "must be an array of strings",
		},
		{
			name:    "integer — invalid type",
			input:   42,
			wantErr: true,
			errMsg:  "must be an array of strings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roles, err := extractRoles(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantRoles, roles)
			}
		})
	}
}

func TestValidateToken_WithEmptyName(t *testing.T) {
	validator := NewJWTValidator(testSecret)

	// Create token with empty name claim - should default to user_id
	claims := jwt.MapClaims{
		"user_id": "user-789",
		"name":    "",
		"roles":   []string{"user"},
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testSecret))

	extractedClaims, err := validator.ValidateToken(tokenString)

	require.NoError(t, err)
	assert.Equal(t, "user-789", extractedClaims.UserID)
	assert.Equal(t, "user-789", extractedClaims.Name) // Should default to user_id when empty
	assert.Equal(t, []string{"user"}, extractedClaims.Roles)
}
