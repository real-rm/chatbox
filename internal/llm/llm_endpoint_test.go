package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid https endpoint",
			endpoint: "https://api.openai.com/v1",
			wantErr:  false,
		},
		{
			name:     "valid https with port",
			endpoint: "https://api.example.com:8443/v1",
			wantErr:  false,
		},
		{
			name:     "http rejected for public host",
			endpoint: "http://api.openai.com/v1",
			wantErr:  true,
			errMsg:   "endpoint must use https scheme",
		},
		{
			name:     "no scheme rejected",
			endpoint: "api.openai.com/v1",
			wantErr:  true,
			errMsg:   "endpoint must have a host",
		},
		{
			name:     "empty host rejected",
			endpoint: "https:///v1",
			wantErr:  true,
			errMsg:   "endpoint must have a host",
		},
		{
			name:     "ftp scheme rejected",
			endpoint: "ftp://files.example.com",
			wantErr:  true,
			errMsg:   "endpoint must use https scheme",
		},
		{
			name:     "internal IP still allowed if https",
			endpoint: "https://192.168.1.1:8443/v1",
			wantErr:  false,
		},
		{
			name:     "http allowed for loopback",
			endpoint: "http://127.0.0.1:8080/v1",
			wantErr:  false,
		},
		{
			name:     "http allowed for localhost",
			endpoint: "http://localhost:8080/v1",
			wantErr:  false,
		},
		{
			name:     "http allowed for RFC 1918 private IP",
			endpoint: "http://10.96.0.50/v1",
			wantErr:  false,
		},
		{
			name:     "http allowed for RFC 1918 192.168 range",
			endpoint: "http://192.168.1.10:8080/v1",
			wantErr:  false,
		},
		{
			name:     "http allowed for Kubernetes short service name",
			endpoint: "http://dify/v1",
			wantErr:  false,
		},
		{
			name:     "http allowed for .svc.cluster.local",
			endpoint: "http://dify-api.default.svc.cluster.local/v1",
			wantErr:  false,
		},
		{
			name:     "http allowed for .internal suffix",
			endpoint: "http://dify.internal/v1",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEndpoint(tt.endpoint)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
