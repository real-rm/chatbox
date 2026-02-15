#!/usr/bin/env python3
import re

def fix_property_test_file():
    with open('internal/llm/llm_property_test.go', 'r') as f:
        content = f.read()
    
    # Remove the old import
    content = content.replace('	"github.com/real-rm/chatbox/internal/config"', '')
    
    # Replace config.LLMConfig with createTestConfig call
    # Pattern: cfg := &config.LLMConfig{\n\t\t\tProviders: []config.LLMProviderConfig{\n\t\t\t\t{\n\t\t\t\t\tID:
    pattern = r'cfg := &config\.LLMConfig\{\s+Providers: \[\]config\.LLMProviderConfig\{'
    replacement = 'cfg := createTestConfig([]LLMProviderConfig{'
    content = re.sub(pattern, replacement, content)
    
    # Replace the closing pattern: },\n\t\t\t},\n\t\t\t}
    # with: },\n\t\t\t})
    pattern = r'(\t+)\},\n\t+\},\n\t+\}\n\n(\t+)service, err := NewLLMService\(cfg\)'
    replacement = r'\1},\n\1})\n\n\1logger := createTestLogger()\n\2service, err := NewLLMService(cfg, logger)'
    content = re.sub(pattern, replacement, content)
    
    with open('internal/llm/llm_property_test.go', 'w') as f:
        f.write(content)
    
    print("Fixed property test file")

def fix_unit_test_file():
    with open('internal/llm/llm_test.go', 'r') as f:
        content = f.read()
    
    # Remove the old import
    content = content.replace('	"github.com/real-rm/chatbox/internal/config"', '')
    
    # Already fixed, just verify
    print("Unit test file already fixed")

if __name__ == '__main__':
    fix_property_test_file()
    fix_unit_test_file()
