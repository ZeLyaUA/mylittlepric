package services

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

type PromptManager struct {
	prompts map[string]string
	mu      sync.RWMutex
}

func NewPromptManager() *PromptManager {
	pm := &PromptManager{
		prompts: make(map[string]string),
	}
	pm.loadPrompts()
	return pm
}

func (pm *PromptManager) loadPrompts() {
	// Get base path for prompts
	basePath := getPromptBasePath()

	promptFiles := map[string]string{
		"master":        basePath + "internal/services/prompts/master_prompt.txt",
		"electronics":   basePath + "internal/services/prompts/specialized_electronics.txt",
		"parametric":    basePath + "internal/services/prompts/specialized_parametric.txt",
		"generic_model": basePath + "internal/services/prompts/specialized_generic_model.txt",
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	for key, path := range promptFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("⚠️  Failed to load prompt %s from %s: %v\n", key, path, err)
			continue
		}
		pm.prompts[key] = string(content)
	}

	fmt.Printf("✅ Loaded %d prompts\n", len(pm.prompts))
}

func (pm *PromptManager) GetPrompt(key, country, language, category string) string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	prompt, exists := pm.prompts[key]
	if !exists {
		return ""
	}

	prompt = strings.ReplaceAll(prompt, "{country}", country)
	prompt = strings.ReplaceAll(prompt, "{language}", language)
	prompt = strings.ReplaceAll(prompt, "{category}", category)

	return prompt
}

func (pm *PromptManager) GetPromptKey(category string) string {
	switch category {
	case "electronics":
		return "electronics"
	case "generic_model":
		return "generic_model"
	case "clothing", "furniture", "kitchen", "sports", "tools", "decor", "textiles":
		return "parametric"
	default:
		return "master"
	}
}
