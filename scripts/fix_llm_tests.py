#!/usr/bin/env python3
import re

# Read the file
with open('internal/llm/llm_property_test.go', 'r') as f:
    content = f.read()

# Pattern to match the old config creation
old_pattern = r'''cfg := createTestConfig\(\[\]LLMProviderConfig\{
\s+\{
\s+ID:\s+"([^"]+)",
\s+Name:\s+"Test Model",
\s+Type:\s+"openai",
\s+Endpoint:\s+"https://api\.test\.com",
\s+APIKey:\s+"test-key",
\s+\},
\s+\}\)'''

# Replacement pattern
new_pattern = r'''// Create config with all 4 models to avoid goconfig caching issues
			allProviders := make([]LLMProviderConfig, len(fixedModelIDs))
			for i, id := range fixedModelIDs {
				allProviders[i] = LLMProviderConfig{
					ID:       id,
					Name:     fmt.Sprintf("Test Model %d", i+1),
					Type:     "openai",
					Endpoint: "https://api.test.com",
					APIKey:   "test-key",
				}
			}
			cfg := createTestConfig(allProviders)'''

# Replace all occurrences
content = re.sub(old_pattern, new_pattern, content)

# Write back
with open('internal/llm/llm_property_test.go', 'w') as f:
    f.write(content)

print("Fixed all occurrences")
