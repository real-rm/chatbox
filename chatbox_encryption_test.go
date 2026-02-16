package chatbox

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidateEncryptionKey tests the encryption key validation function
func TestValidateEncryptionKey(t *testing.T) {
	t.Run("valid 32-byte key", func(t *testing.T) {
		// Create a 32-byte key
		key := make([]byte, 32)
		for i := range key {
			key[i] = byte(i)
		}

		err := validateEncryptionKey(key)
		assert.NoError(t, err, "32-byte key should be valid")
	})

	t.Run("empty key (encryption disabled)", func(t *testing.T) {
		// Empty key should be valid (encryption disabled)
		key := []byte{}

		err := validateEncryptionKey(key)
		assert.NoError(t, err, "empty key should be valid (encryption disabled)")
	})

	t.Run("nil key (encryption disabled)", func(t *testing.T) {
		// Nil key should be valid (encryption disabled)
		var key []byte = nil

		err := validateEncryptionKey(key)
		assert.NoError(t, err, "nil key should be valid (encryption disabled)")
	})

	t.Run("invalid 16-byte key", func(t *testing.T) {
		// Create a 16-byte key (too short)
		key := make([]byte, 16)

		err := validateEncryptionKey(key)
		assert.Error(t, err, "16-byte key should be invalid")
		assert.Contains(t, err.Error(), "32 bytes", "error should mention required length")
		assert.Contains(t, err.Error(), "16 bytes", "error should mention actual length")
	})

	t.Run("invalid 31-byte key", func(t *testing.T) {
		// Create a 31-byte key (one byte short)
		key := make([]byte, 31)

		err := validateEncryptionKey(key)
		assert.Error(t, err, "31-byte key should be invalid")
		assert.Contains(t, err.Error(), "32 bytes", "error should mention required length")
		assert.Contains(t, err.Error(), "31 bytes", "error should mention actual length")
	})

	t.Run("invalid 33-byte key", func(t *testing.T) {
		// Create a 33-byte key (one byte too long)
		key := make([]byte, 33)

		err := validateEncryptionKey(key)
		assert.Error(t, err, "33-byte key should be invalid")
		assert.Contains(t, err.Error(), "32 bytes", "error should mention required length")
		assert.Contains(t, err.Error(), "33 bytes", "error should mention actual length")
	})

	t.Run("invalid 64-byte key", func(t *testing.T) {
		// Create a 64-byte key (too long)
		key := make([]byte, 64)

		err := validateEncryptionKey(key)
		assert.Error(t, err, "64-byte key should be invalid")
		assert.Contains(t, err.Error(), "32 bytes", "error should mention required length")
		assert.Contains(t, err.Error(), "64 bytes", "error should mention actual length")
	})

	t.Run("error message format", func(t *testing.T) {
		// Test that error message has the correct format
		key := make([]byte, 16)

		err := validateEncryptionKey(key)
		assert.Error(t, err)

		errMsg := err.Error()

		// Error message should contain:
		// 1. "encryption key must be exactly 32 bytes"
		// 2. "AES-256"
		// 3. "got 16 bytes"
		// 4. Guidance about providing valid key or removing it
		assert.Contains(t, errMsg, "encryption key must be exactly 32 bytes", "error should state requirement")
		assert.Contains(t, errMsg, "AES-256", "error should mention AES-256")
		assert.Contains(t, errMsg, "got 16 bytes", "error should state actual length")
		assert.True(t,
			strings.Contains(errMsg, "provide a valid 32-byte key") ||
				strings.Contains(errMsg, "remove the key"),
			"error should provide guidance")
	})

	t.Run("error message contains both required and actual lengths", func(t *testing.T) {
		// Test various invalid lengths to ensure error message always contains both lengths
		testCases := []struct {
			length       int
			expectedText string
		}{
			{1, "got 1 bytes"},
			{8, "got 8 bytes"},
			{15, "got 15 bytes"},
			{16, "got 16 bytes"},
			{24, "got 24 bytes"},
			{31, "got 31 bytes"},
			{33, "got 33 bytes"},
			{48, "got 48 bytes"},
			{64, "got 64 bytes"},
			{100, "got 100 bytes"},
		}

		for _, tc := range testCases {
			key := make([]byte, tc.length)
			err := validateEncryptionKey(key)

			assert.Error(t, err, "key of length %d should be invalid", tc.length)
			assert.Contains(t, err.Error(), "32 bytes", "error should mention required length (32)")
			assert.Contains(t, err.Error(), tc.expectedText, "error should mention actual length (%d)", tc.length)
		}
	})
}
