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
// Expected Result: Test should fail if placeholder patterns are detected, documenting the security risk
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
	if len(placeholdersFound) > 0 {
		t.Errorf("SECURITY RISK: Found %d placeholder secrets in secret.yaml:\n", len(placeholdersFound))
		for _, placeholder := range placeholdersFound {
			t.Logf("  - %s", placeholder)
		}
		t.Error("\nRecommendations:")
		t.Error("1. Generate strong random secrets for production deployment")
		t.Error("2. Use secret management tools (e.g., Sealed Secrets, External Secrets Operator)")
		t.Error("3. Never commit real secrets to version control")
		t.Error("4. Implement secret rotation procedures")
		t.Error("5. Use environment-specific secret values")
		t.Error("\nFor quick setup instructions, see: docs/SECRET_SETUP_QUICKSTART.md")
	}
}
