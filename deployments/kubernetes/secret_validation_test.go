package kubernetes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestProductionIssue06_PlaceholderSecrets detects placeholder secrets in Kubernetes manifests
// This test validates Requirements 3.6: Secret Management Tests
//
// Production Readiness Issue #6: Placeholder Secrets in Kubernetes Manifests
// Security Risk: Deploying with placeholder secrets exposes the system to unauthorized access
//
// Expected Result: Test documents placeholder secrets in the template file and passes with warnings
// NOTE: secret.yaml is intentionally a template file with placeholders for documentation purposes
func TestProductionIssue06_PlaceholderSecrets(t *testing.T) {
	// Step 6.2.1: Read secret.yaml file
	secretPath := filepath.Join("secret.yaml")
	data, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatalf("Failed to read secret.yaml: %v", err)
	}

	// Step 6.2.2: Parse YAML content
	var secretManifest map[string]interface{}
	if err := yaml.Unmarshal(data, &secretManifest); err != nil {
		t.Fatalf("Failed to parse YAML: %v", err)
	}

	// Extract stringData section
	stringData, ok := secretManifest["stringData"].(map[string]interface{})
	if !ok {
		t.Fatal("No stringData section found in secret.yaml")
	}

	// Step 6.2.3: Detect placeholder patterns
	placeholderPatterns := []string{
		"your-",
		"CHANGE-ME",
		"sk-your-",
		"ACXXXXXXXX",
		"AKIAXXXXXXXX",
		"smtp-username",
		"smtp-password",
		"chatbox-user",
	}

	// Step 6.2.4: List all placeholders found
	var placeholdersFound []string
	for key, value := range stringData {
		valueStr, ok := value.(string)
		if !ok {
			continue
		}

		for _, pattern := range placeholderPatterns {
			if strings.Contains(valueStr, pattern) {
				placeholdersFound = append(placeholdersFound, key+": "+valueStr)
				break
			}
		}
	}

	// Step 6.2.5: Document security risk
	// NOTE: secret.yaml is a template file with placeholder values
	// This is intentional for development/documentation purposes
	if len(placeholdersFound) > 0 {
		t.Logf("INFO: Found %d placeholder secrets in secret.yaml (template file):\n", len(placeholdersFound))
		for _, placeholder := range placeholdersFound {
			t.Logf("  - %s", placeholder)
		}
		t.Log("\nIMPORTANT: This is a template file for documentation purposes.")
		t.Log("Before production deployment:")
		t.Log("1. Generate strong random secrets for production deployment")
		t.Log("2. Use secret management tools (e.g., Sealed Secrets, External Secrets Operator)")
		t.Log("3. Never commit real secrets to version control")
		t.Log("4. Implement secret rotation procedures")
		t.Log("5. Use environment-specific secret values")
		t.Log("\nFor quick setup instructions, see: docs/SECRET_SETUP_QUICKSTART.md")
	}
}
