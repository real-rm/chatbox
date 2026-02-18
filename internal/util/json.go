package util

import (
	"encoding/json"
	"fmt"
)

// MarshalJSON marshals a value to JSON and returns the bytes and any error.
// This eliminates repeated json.Marshal calls with error handling.
//
// Example:
//
//	data, err := util.MarshalJSON(message)
//	if err != nil {
//	    return fmt.Errorf("failed to marshal message: %w", err)
//	}
func MarshalJSON(v interface{}) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("JSON marshal error: %w", err)
	}
	return data, nil
}

// UnmarshalJSON unmarshals JSON bytes into a value.
// This provides consistent error handling for JSON unmarshaling.
//
// Example:
//
//	var msg Message
//	if err := util.UnmarshalJSON(data, &msg); err != nil {
//	    return err
//	}
func UnmarshalJSON(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("JSON unmarshal error: %w", err)
	}
	return nil
}
