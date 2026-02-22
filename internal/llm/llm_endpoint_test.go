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
			name:     "http rejected",
			endpoint: "http://api.openai.com/v1",
			wantErr:  true,
			errMsg:   "endpoint must use https scheme",
		},
		{
			name:     "no scheme rejected",
			endpoint: "api.openai.com/v1",
			wantErr:  true,
			errMsg:   "endpoint must use https scheme",
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
