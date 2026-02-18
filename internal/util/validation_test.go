package util

import (
	"strings"
	"testing"
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
