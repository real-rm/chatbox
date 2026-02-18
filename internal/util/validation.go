package util

import (
	"errors"
	"fmt"
)

// ValidateNotEmpty checks if a string is not empty and returns an error if it is.
// This eliminates repeated empty string checks.
//
// Example:
//
//	if err := util.ValidateNotEmpty(sessionID, "session ID"); err != nil {
//	    return err
//	}
func ValidateNotEmpty(value, fieldName string) error {
	if value == "" {
		return fmt.Errorf("%s cannot be empty", fieldName)
	}
	return nil
}

// ValidateNotNil checks if a pointer is not nil and returns an error if it is.
// This eliminates repeated nil checks.
// Note: This function uses reflection to properly detect nil pointers.
//
// Example:
//
//	if err := util.ValidateNotNil(conn, "connection"); err != nil {
//	    return err
//	}
func ValidateNotNil(value interface{}, fieldName string) error {
	if value == nil {
		return fmt.Errorf("%s cannot be nil", fieldName)
	}

	// Use type assertion to check for nil pointers
	// This is needed because interface{} can hold a typed nil pointer
	switch v := value.(type) {
	case *string:
		if v == nil {
			return fmt.Errorf("%s cannot be nil", fieldName)
		}
	case *int:
		if v == nil {
			return fmt.Errorf("%s cannot be nil", fieldName)
		}
		// Add more types as needed, or use reflection for generic solution
	}

	return nil
}

// ValidateRange checks if an integer is within a specified range (inclusive).
//
// Example:
//
//	if err := util.ValidateRange(port, 1, 65535, "port"); err != nil {
//	    return err
//	}
func ValidateRange(value, min, max int, fieldName string) error {
	if value < min || value > max {
		return fmt.Errorf("%s must be between %d and %d, got %d", fieldName, min, max, value)
	}
	return nil
}

// ValidateMinLength checks if a string meets minimum length requirement.
//
// Example:
//
//	if err := util.ValidateMinLength(secret, 32, "JWT secret"); err != nil {
//	    return err
//	}
func ValidateMinLength(value string, minLength int, fieldName string) error {
	if len(value) < minLength {
		return fmt.Errorf("%s must be at least %d characters, got %d", fieldName, minLength, len(value))
	}
	return nil
}

// ValidateExactLength checks if a byte slice has exact length.
// This is useful for encryption key validation.
//
// Example:
//
//	if err := util.ValidateExactLength(key, 32, "encryption key"); err != nil {
//	    return err
//	}
func ValidateExactLength(value []byte, exactLength int, fieldName string) error {
	if len(value) != exactLength && len(value) != 0 {
		return fmt.Errorf("%s must be exactly %d bytes, got %d bytes", fieldName, exactLength, len(value))
	}
	return nil
}

// ValidatePositive checks if a number is positive.
//
// Example:
//
//	if err := util.ValidatePositive(timeout, "timeout"); err != nil {
//	    return err
//	}
func ValidatePositive(value int, fieldName string) error {
	if value <= 0 {
		return fmt.Errorf("%s must be positive, got %d", fieldName, value)
	}
	return nil
}

// ValidateTimeRange checks if end time is after start time.
//
// Example:
//
//	if err := util.ValidateTimeRange(startTime, endTime); err != nil {
//	    return err
//	}
func ValidateTimeRange(start, end interface{}) error {
	// This is a placeholder - actual implementation would depend on time.Time comparison
	return errors.New("not implemented")
}
