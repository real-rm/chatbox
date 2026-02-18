package chatbox

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
)

// TestMetricsEndpoint verifies that the /metrics endpoint is accessible and returns Prometheus metrics
func TestMetricsEndpoint(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Create a test router
	r := gin.New()

	// Register the metrics endpoint
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Create a test request
	req, err := http.NewRequest("GET", "/metrics", nil)
	assert.NoError(t, err)

	// Create a response recorder
	w := httptest.NewRecorder()

	// Serve the request
	r.ServeHTTP(w, req)

	// Verify the response
	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200 OK")
	assert.Contains(t, w.Body.String(), "# HELP", "Expected Prometheus metrics format")
	assert.Contains(t, w.Body.String(), "# TYPE", "Expected Prometheus metrics format")
}

// TestMetricsEndpointContentType verifies that the /metrics endpoint returns the correct content type
func TestMetricsEndpointContentType(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Create a test router
	r := gin.New()

	// Register the metrics endpoint
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Create a test request
	req, err := http.NewRequest("GET", "/metrics", nil)
	assert.NoError(t, err)

	// Create a response recorder
	w := httptest.NewRecorder()

	// Serve the request
	r.ServeHTTP(w, req)

	// Verify the content type
	contentType := w.Header().Get("Content-Type")
	assert.Contains(t, contentType, "text/plain", "Expected text/plain content type for Prometheus metrics")
}
