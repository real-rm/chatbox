package websocket

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/real-rm/chatbox/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestToken creates a valid HS256-signed JWT for use in websocket handler tests.
func generateTestToken(t *testing.T, secret, userID string, roles []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"user_id": userID,
		"roles":   roles,
		"exp":     time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

// TestDeprecateJWTQueryParam_RejectsQueryTokenWhenEnabled verifies that when
// SetDeprecateJWTQueryParam(true) is called, tokens provided via ?token= are
// rejected with 401 and a clear message directing clients to the header.
func TestDeprecateJWTQueryParam_RejectsQueryTokenWhenEnabled(t *testing.T) {
	secret := "test-secret-32-bytes-padding-ok!"
	validator := auth.NewJWTValidator(secret)
	handler := NewHandler(validator, nil, testLogger(), 1048576)
	handler.SetDeprecateJWTQueryParam(true)

	token := generateTestToken(t, secret, "user-jwt-dep-test", []string{"user"})

	req := httptest.NewRequest(http.MethodGet, "/ws?token="+token, nil)
	w := httptest.NewRecorder()
	handler.HandleWebSocket(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"query-param JWT must be rejected when DeprecateJWTQueryParam is enabled")
	assert.Contains(t, w.Body.String(), "Authorization",
		"response must direct clients to use the Authorization header")
}

// TestDeprecateJWTQueryParam_AcceptsQueryTokenByDefault verifies that the
// default behaviour (flag false) still accepts tokens via query parameter,
// preserving backwards compatibility for existing clients.
func TestDeprecateJWTQueryParam_AcceptsQueryTokenByDefault(t *testing.T) {
	secret := "test-secret-32-bytes-padding-ok!"
	validator := auth.NewJWTValidator(secret)
	handler := NewHandler(validator, nil, testLogger(), 1048576)
	// No SetDeprecateJWTQueryParam call — default is false.

	token := generateTestToken(t, secret, "user-jwt-dep-default", []string{"user"})

	// The upgrade itself will fail (not a real WS request), but the important thing
	// is that the query-param token is not rejected with the new 401 deprecation error.
	req := httptest.NewRequest(http.MethodGet, "/ws?token="+token, nil)
	w := httptest.NewRecorder()
	handler.HandleWebSocket(w, req)

	assert.NotEqual(t, http.StatusUnauthorized, w.Code,
		"query-param JWT must be accepted when DeprecateJWTQueryParam is false (default)")
}
