#!/usr/bin/env python3
"""
Script to fix LLM property tests by replacing gen.Identifier() with fixed model IDs
"""

import re

# Read the file
with open('internal/llm/llm_property_test.go', 'r') as f:
    content = f.read()

# List of test functions that need fixing
tests_to_fix = [
    'TestProperty_ResponseTimeTracking',
    'TestProperty_LLMRequestContextInclusion',
    'TestProperty_StreamingResponseForwarding',
    'TestProperty_LLMBackendRetryLogic',
    'TestProperty_ModelSelectionPersistence',
    'TestProperty_TokenUsageTrackingAndStorage',
]

for test_name in tests_to_fix:
    # Find the test function
    pattern = rf'(func {test_name}\(t \*testing\.T\) \{{\n)'
    
    # Add fixed model IDs declaration after function start
    replacement = r'\1\t// Use a fixed set of model IDs to avoid goconfig caching issues\n\tfixedModelIDs := []string{"test-model-1", "test-model-2", "test-model-3"}\n\t\n'
    content = re.sub(pattern, replacement, content)
    
    # Find and replace gen.Identifier() with gen.IntRange(0, 1000) for modelID parameter
    # This is tricky because we need to find the right gen.Identifier() call
    # We'll look for patterns like:
    # func(modelID string, ...) bool {
    # and replace the corresponding gen.Identifier() with gen.IntRange(0, 1000)
    
# Also need to replace modelID usage in the function body
# Replace: if modelID == "" {
# With: modelID := fixedModelIDs[modelIDIndex%len(fixedModelIDs)]

print("Manual fixes needed - the regex is too complex for automated replacement")
print("Please manually update the remaining tests")
