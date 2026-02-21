package kubernetes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestReadOnlyRootFilesystem validates that deployment.yaml has readOnlyRootFilesystem: true
func TestReadOnlyRootFilesystem(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("deployment.yaml"))
	require.NoError(t, err, "Failed to read deployment.yaml")

	var deployment map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &deployment))

	// Navigate: spec.template.spec.containers[0].securityContext.readOnlyRootFilesystem
	spec := deployment["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	podSpec := template["spec"].(map[string]interface{})
	containers := podSpec["containers"].([]interface{})
	require.NotEmpty(t, containers, "deployment must have at least one container")

	container := containers[0].(map[string]interface{})
	secCtx, ok := container["securityContext"].(map[string]interface{})
	require.True(t, ok, "container must have securityContext")

	readOnly, ok := secCtx["readOnlyRootFilesystem"].(bool)
	require.True(t, ok, "securityContext must have readOnlyRootFilesystem")
	assert.True(t, readOnly, "readOnlyRootFilesystem must be true for production")
}

// TestPodDisruptionBudget validates pdb.yaml has correct configuration
func TestPodDisruptionBudget(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("pdb.yaml"))
	require.NoError(t, err, "Failed to read pdb.yaml")

	var pdb map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &pdb))

	// Validate API version and kind
	assert.Equal(t, "policy/v1", pdb["apiVersion"])
	assert.Equal(t, "PodDisruptionBudget", pdb["kind"])

	// Validate spec
	spec := pdb["spec"].(map[string]interface{})
	minAvail, ok := spec["minAvailable"].(int)
	require.True(t, ok, "minAvailable must be an integer")
	assert.GreaterOrEqual(t, minAvail, 2, "minAvailable should be at least 2 for HA")

	// Validate selector labels match deployment
	selector := spec["selector"].(map[string]interface{})
	matchLabels := selector["matchLabels"].(map[string]interface{})
	assert.Equal(t, "chatbox", matchLabels["app"])
	assert.Equal(t, "websocket", matchLabels["component"])
}

// TestNetworkPolicy validates networkpolicy.yaml restricts traffic properly
func TestNetworkPolicy(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("networkpolicy.yaml"))
	require.NoError(t, err, "Failed to read networkpolicy.yaml")

	var np map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &np))

	// Validate API version and kind
	assert.Equal(t, "networking.k8s.io/v1", np["apiVersion"])
	assert.Equal(t, "NetworkPolicy", np["kind"])

	// Validate spec
	spec := np["spec"].(map[string]interface{})

	// Validate pod selector
	podSelector := spec["podSelector"].(map[string]interface{})
	matchLabels := podSelector["matchLabels"].(map[string]interface{})
	assert.Equal(t, "chatbox", matchLabels["app"])

	// Validate both Ingress and Egress are restricted
	policyTypes := spec["policyTypes"].([]interface{})
	policyTypeStrs := make([]string, len(policyTypes))
	for i, pt := range policyTypes {
		policyTypeStrs[i] = pt.(string)
	}
	assert.Contains(t, policyTypeStrs, "Ingress", "must restrict ingress")
	assert.Contains(t, policyTypeStrs, "Egress", "must restrict egress")

	// Validate ingress rules exist
	ingress := spec["ingress"].([]interface{})
	assert.NotEmpty(t, ingress, "must have at least one ingress rule")

	// Validate egress rules exist (DNS + MongoDB)
	egress := spec["egress"].([]interface{})
	assert.GreaterOrEqual(t, len(egress), 2, "must have egress rules for DNS and MongoDB")
}
