package httperrors

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRespondUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	RespondUnauthorized(c, "")

	assert.Equal(t, 401, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, MsgUnauthorized, response.Error)
	assert.Equal(t, CodeUnauthorized, response.Code)
}

func TestRespondInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	RespondInvalidToken(c)

	assert.Equal(t, 401, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, MsgInvalidToken, response.Error)
	assert.Equal(t, CodeInvalidToken, response.Code)
}

func TestRespondForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	RespondForbidden(c)

	assert.Equal(t, 403, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, MsgForbidden, response.Error)
	assert.Equal(t, CodeForbidden, response.Code)
}

func TestRespondBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	RespondBadRequest(c, "Custom message")

	assert.Equal(t, 400, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Custom message", response.Error)
	assert.Equal(t, CodeBadRequest, response.Code)
}

func TestRespondInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	RespondInternalError(c)

	assert.Equal(t, 500, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, MsgInternalError, response.Error)
	assert.Equal(t, CodeInternalError, response.Code)
}

func TestRespondServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	RespondServiceUnavailable(c)

	assert.Equal(t, 503, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, MsgServiceUnavailable, response.Error)
	assert.Equal(t, CodeServiceUnavailable, response.Code)
}

func TestRespondNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	RespondNotFound(c, "")

	assert.Equal(t, 404, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, MsgResourceNotFound, response.Error)
	assert.Equal(t, CodeNotFound, response.Code)
}

func TestErrorResponseDoesNotLeakInternalDetails(t *testing.T) {
	// This test verifies that error messages are generic and don't contain
	// internal implementation details like stack traces, database queries, etc.

	tests := []struct {
		name    string
		message string
	}{
		{"Unauthorized", MsgUnauthorized},
		{"InvalidToken", MsgInvalidToken},
		{"Forbidden", MsgForbidden},
		{"InternalError", MsgInternalError},
		{"ServiceUnavailable", MsgServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify messages don't contain common internal detail indicators
			assert.NotContains(t, tt.message, "stack trace")
			assert.NotContains(t, tt.message, "query")
			assert.NotContains(t, tt.message, "database")
			assert.NotContains(t, tt.message, "SQL")
			assert.NotContains(t, tt.message, "MongoDB")
			assert.NotContains(t, tt.message, "exception")
			assert.NotContains(t, tt.message, "panic")
			assert.NotContains(t, tt.message, "nil pointer")
			assert.NotContains(t, tt.message, "file path")
			assert.NotContains(t, tt.message, "/internal/")
		})
	}
}
