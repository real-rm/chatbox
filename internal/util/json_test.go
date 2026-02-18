package util

import (
	"encoding/json"
	"testing"
)

type testStruct struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestMarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
	}{
		{
			name: "valid struct",
			input: testStruct{
				Name:  "test",
				Value: 42,
			},
			wantErr: false,
		},
		{
			name:    "valid map",
			input:   map[string]string{"key": "value"},
			wantErr: false,
		},
		{
			name:    "valid slice",
			input:   []int{1, 2, 3},
			wantErr: false,
		},
		{
			name:    "nil value",
			input:   nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := MarshalJSON(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && data == nil {
				t.Error("Expected non-nil data")
			}

			// Verify it's valid JSON
			if !tt.wantErr {
				var result interface{}
				if err := json.Unmarshal(data, &result); err != nil {
					t.Errorf("Result is not valid JSON: %v", err)
				}
			}
		})
	}
}

func TestUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		target  interface{}
		wantErr bool
	}{
		{
			name:    "valid JSON to struct",
			input:   `{"name":"test","value":42}`,
			target:  &testStruct{},
			wantErr: false,
		},
		{
			name:    "valid JSON to map",
			input:   `{"key":"value"}`,
			target:  &map[string]string{},
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   `{invalid}`,
			target:  &testStruct{},
			wantErr: true,
		},
		{
			name:    "empty JSON",
			input:   ``,
			target:  &testStruct{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UnmarshalJSON([]byte(tt.input), tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	original := testStruct{
		Name:  "roundtrip",
		Value: 123,
	}

	// Marshal
	data, err := MarshalJSON(original)
	if err != nil {
		t.Fatalf("MarshalJSON() failed: %v", err)
	}

	// Unmarshal
	var result testStruct
	if err := UnmarshalJSON(data, &result); err != nil {
		t.Fatalf("UnmarshalJSON() failed: %v", err)
	}

	// Compare
	if result.Name != original.Name || result.Value != original.Value {
		t.Errorf("Round trip failed: got %+v, want %+v", result, original)
	}
}
