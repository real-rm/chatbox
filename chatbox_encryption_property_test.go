package chatbox

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: critical-security-fixes, Property 1: Error message completeness for invalid keys
// **Validates: Requirements 1.7**
//
// For any encryption key that is not exactly 32 bytes in length, the error message
// returned during validation should contain both the required length (32) and the
// actual length of the provided key.
func TestProperty_ErrorMessageCompletenessForInvalidKeys(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("error message contains both required and actual key lengths", prop.ForAll(
		func(keyLength uint8) bool {
			// Skip valid lengths (0 and 32 bytes)
			if keyLength == 0 || keyLength == 32 {
				return true
			}

			// Create a key with the generated length
			key := make([]byte, keyLength)

			// Validate the key
			err := validateEncryptionKey(key)

			// Should return an error for invalid lengths
			if err == nil {
				t.Logf("Expected error for key length %d, but got nil", keyLength)
				return false
			}

			errorMsg := err.Error()

			// Verify error message contains the required length (32)
			if !strings.Contains(errorMsg, "32") {
				t.Logf("Error message does not contain required length (32): %s", errorMsg)
				return false
			}

			// Verify error message contains the actual length
			actualLengthStr := strconv.Itoa(int(keyLength))
			if !strings.Contains(errorMsg, actualLengthStr) {
				t.Logf("Error message does not contain actual length (%d): %s", keyLength, errorMsg)
				return false
			}

			// Additional verification: check for the specific format
			expectedSubstring := fmt.Sprintf("32 bytes for AES-256, got %d bytes", keyLength)
			if !strings.Contains(errorMsg, expectedSubstring) {
				t.Logf("Error message does not contain expected format: %s", errorMsg)
				return false
			}

			return true
		},
		gen.UInt8Range(1, 255), // Generate key lengths from 1 to 255 bytes
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
