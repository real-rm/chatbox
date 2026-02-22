package util

import (
	"strings"
	"testing"
	"time"
)

func TestValidateNotEmpty(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		fieldName string
		wantErr   bool
	}{
		{"valid string", "test", "field", false},
		{"empty string", "", "field", true},
		{"whitespace", "   ", "field", false}, // Whitespace is not empty
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNotEmpty(tt.value, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNotEmpty() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), tt.fieldName) {
				t.Errorf("Error message should contain field name %q", tt.fieldName)
			}
		})
	}
}

func TestValidateNotNil(t *testing.T) {
	var nilPtr *string
	nonNilPtr := new(string)

	tests := []struct {
		name      string
		value     interface{}
		fieldName string
		wantErr   bool
	}{
		{"non-nil pointer", nonNilPtr, "field", false},
		{"nil pointer", nilPtr, "field", true},
		{"nil interface", nil, "field", true},
		{"non-nil value", "test", "field", false},
		{"nil *int pointer", (*int)(nil), "field", true},
		{"non-nil *int pointer", new(int), "field", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNotNil(tt.value, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNotNil() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRange(t *testing.T) {
	tests := []struct {
		name      string
		value     int
		min       int
		max       int
		fieldName string
		wantErr   bool
	}{
		{"within range", 50, 1, 100, "field", false},
		{"at minimum", 1, 1, 100, "field", false},
		{"at maximum", 100, 1, 100, "field", false},
		{"below minimum", 0, 1, 100, "field", true},
		{"above maximum", 101, 1, 100, "field", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRange(tt.value, tt.min, tt.max, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRange() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateMinLength(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		minLength int
		fieldName string
		wantErr   bool
	}{
		{"meets minimum", "test", 4, "field", false},
		{"exceeds minimum", "testing", 4, "field", false},
		{"below minimum", "tes", 4, "field", true},
		{"empty string", "", 1, "field", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMinLength(tt.value, tt.minLength, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMinLength() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateExactLength(t *testing.T) {
	tests := []struct {
		name        string
		value       []byte
		exactLength int
		fieldName   string
		wantErr     bool
	}{
		{"exact length", []byte{1, 2, 3, 4}, 4, "field", false},
		{"empty allowed", []byte{}, 4, "field", false}, // Empty is allowed
		{"wrong length", []byte{1, 2, 3}, 4, "field", true},
		{"too long", []byte{1, 2, 3, 4, 5}, 4, "field", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExactLength(tt.value, tt.exactLength, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateExactLength() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePositive(t *testing.T) {
	tests := []struct {
		name      string
		value     int
		fieldName string
		wantErr   bool
	}{
		{"positive", 1, "field", false},
		{"large positive", 1000, "field", false},
		{"zero", 0, "field", true},
		{"negative", -1, "field", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePositive(tt.value, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePositive() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFileURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"empty URL allowed", "", false},
		{"valid HTTPS URL", "https://example.com/files/photo.jpg", false},
		{"valid HTTP URL", "http://cdn.example.com/file.pdf", false},
		{"javascript scheme rejected", "javascript:alert(1)", true},
		{"data scheme rejected", "data:text/html,<h1>Hello</h1>", true},
		{"file scheme rejected", "file:///etc/passwd", true},
		{"ftp scheme rejected", "ftp://example.com/file.txt", true},
		{"private IP 10.x rejected", "http://10.0.0.1/file.txt", true},
		{"private IP 172.16.x rejected", "http://172.16.0.1/file.txt", true},
		{"private IP 192.168.x rejected", "http://192.168.1.1/file.txt", true},
		{"localhost rejected", "http://127.0.0.1/file.txt", true},
		{"link-local rejected", "http://169.254.1.1/file.txt", true},
		{"no hostname rejected", "http:///path", true},
		{"URL too long", "https://example.com/" + strings.Repeat("a", 2048), true},
		{"valid domain with path", "https://storage.googleapis.com/bucket/file.png", false},
		{"valid URL with query params", "https://cdn.example.com/file?token=abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFileURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

// TestValidateTimeRange verifies time range validation with various inputs.
func TestValidateTimeRange(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name    string
		start   interface{}
		end     interface{}
		wantErr bool
	}{
		{
			name:    "valid range",
			start:   now.Add(-time.Hour),
			end:     now,
			wantErr: false,
		},
		{
			name:    "nil start — not a time.Time",
			start:   nil,
			end:     nil,
			wantErr: true,
		},
		{
			name:    "string values — not a time.Time",
			start:   "2024-01-01",
			end:     "2024-12-31",
			wantErr: true,
		},
		{
			name:    "integer values — not a time.Time",
			start:   1,
			end:     100,
			wantErr: true,
		},
		{
			name:    "zero start time",
			start:   time.Time{},
			end:     now,
			wantErr: true,
		},
		{
			name:    "zero end time",
			start:   now,
			end:     time.Time{},
			wantErr: true,
		},
		{
			name:    "inverted range — end before start",
			start:   now,
			end:     now.Add(-time.Hour),
			wantErr: true,
		},
		{
			name:    "equal times — end not after start",
			start:   now,
			end:     now,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimeRange(tt.start, tt.end)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTimeRange() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
