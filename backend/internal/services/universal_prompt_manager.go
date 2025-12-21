package services

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"mylittleprice/internal/models"
	"mylittleprice/internal/utils"
)

const (
	PromptIDUniversal = "UniversalPrompt v1.0.1"
	MaxIterations     = 6
)

// UniversalPromptManager manages the Universal Prompt system with mini-kernel approach
type UniversalPromptManager struct {
	universalPrompt string
	miniKernel      string
	promptHasher    *utils.PromptHasher
	promptHash      string
	mu              sync.RWMutex
}

// NewUniversalPromptManager creates a new Universal Prompt Manager
func NewUniversalPromptManager() *UniversalPromptManager {
	upm := &UniversalPromptManager{
		promptHasher: utils.NewPromptHasher(),
	}
	upm.loadPrompts()
	return upm
}

// loadPrompts loads the universal prompt and mini-kernel from files
func (upm *UniversalPromptManager) loadPrompts() {
	upm.mu.Lock()
	defer upm.mu.Unlock()

	// Get base path - try current directory first, then root
	basePath := getPromptBasePath()

	// Load universal prompt (sent once on session start)
	universalPath := basePath + "internal/services/prompts/universal_prompt.txt"
	universalContent, err := os.ReadFile(universalPath)
	if err != nil {
		panic(fmt.Errorf("CRITICAL: Failed to load universal prompt from %s: %w", universalPath, err))
	}
	upm.universalPrompt = string(universalContent)

	// Load mini-kernel (sent on every turn)
	kernelPath := basePath + "internal/services/prompts/mini_kernel.txt"
	kernelContent, err := os.ReadFile(kernelPath)
	if err != nil {
		panic(fmt.Errorf("CRITICAL: Failed to load mini-kernel from %s: %w", kernelPath, err))
	}
	upm.miniKernel = string(kernelContent)

	// Generate hash for drift detection
	upm.promptHash = upm.promptHasher.HashPrompt(upm.universalPrompt)

	fmt.Printf("✅ Universal Prompt System loaded (hash: %s)\n", upm.promptHasher.HashPromptShort(upm.universalPrompt))
}

// GetSystemPrompt returns the full system prompt for NEW sessions
// This is sent ONCE when the session starts
func (upm *UniversalPromptManager) GetSystemPrompt(
	feLocation, feLanguage, feCurrency string,
) string {
	upm.mu.RLock()
	defer upm.mu.RUnlock()

	// Get current date and year
	now := time.Now()
	currentDate := now.Format("January 2, 2006")
	currentYear := fmt.Sprintf("%d", now.Year())
	previousYear := fmt.Sprintf("%d", now.Year()-1)

	prompt := upm.universalPrompt
	prompt = strings.ReplaceAll(prompt, "{fe_location}", feLocation)
	prompt = strings.ReplaceAll(prompt, "{fe_language}", feLanguage)
	prompt = strings.ReplaceAll(prompt, "{fe_currency}", feCurrency)
	prompt = strings.ReplaceAll(prompt, "{current_date}", currentDate)
	prompt = strings.ReplaceAll(prompt, "{current_year}", currentYear)
	prompt = strings.ReplaceAll(prompt, "{previous_year}", previousYear)

	return prompt
}

// GetMiniKernel returns the mini-kernel for EVERY turn
// This ensures the rules are always in context
func (upm *UniversalPromptManager) GetMiniKernel(
	feLocation, feLanguage, feCurrency string,
	cycleState *models.CycleState,
) string {
	upm.mu.RLock()
	defer upm.mu.RUnlock()

	// Get current date and year
	now := time.Now()
	currentDate := now.Format("January 2, 2006")
	currentYear := fmt.Sprintf("%d", now.Year())
	previousYear := fmt.Sprintf("%d", now.Year()-1)

	kernel := upm.miniKernel
	kernel = strings.ReplaceAll(kernel, "{fe_location}", feLocation)
	kernel = strings.ReplaceAll(kernel, "{fe_language}", feLanguage)
	kernel = strings.ReplaceAll(kernel, "{fe_currency}", feCurrency)
	kernel = strings.ReplaceAll(kernel, "{cycle_id}", fmt.Sprintf("%d", cycleState.CycleID))
	kernel = strings.ReplaceAll(kernel, "{iteration}", fmt.Sprintf("%d", cycleState.Iteration))
	kernel = strings.ReplaceAll(kernel, "{category}", getCategory(cycleState))
	kernel = strings.ReplaceAll(kernel, "{current_date}", currentDate)
	kernel = strings.ReplaceAll(kernel, "{current_year}", currentYear)
	kernel = strings.ReplaceAll(kernel, "{previous_year}", previousYear)

	return kernel
}

// BuildStateContext builds the state context object sent with each turn
// This includes cycle_history, last_cycle_context, and last_defined
func (upm *UniversalPromptManager) BuildStateContext(
	session *models.ChatSession,
) string {
	cycleState := &session.CycleState

	var sb strings.Builder

	sb.WriteString("=== CURRENT STATE ===\n")
	sb.WriteString(fmt.Sprintf("CYCLE_ID: %d\n", cycleState.CycleID))
	sb.WriteString(fmt.Sprintf("ITERATION: %d/%d\n", cycleState.Iteration, MaxIterations))
	sb.WriteString(fmt.Sprintf("CURRENT_CATEGORY: %s\n", getCategory(cycleState)))
	sb.WriteString("\n")

	// Cycle history (limited to last 6 messages to match MaxIterations)
	sb.WriteString("=== CYCLE_HISTORY (Current Cycle) ===\n")
	if len(cycleState.CycleHistory) == 0 {
		sb.WriteString("(empty - first message in cycle)\n")
	} else {
		// Show only last 6 messages to match max iterations per cycle
		// This ensures full visibility of current cycle while managing token usage
		maxRecentMessages := MaxIterations
		startIdx := len(cycleState.CycleHistory) - maxRecentMessages
		if startIdx < 0 {
			startIdx = 0
		}

		// If we're skipping messages, add a summary note
		if startIdx > 0 {
			sb.WriteString(fmt.Sprintf("(showing last %d of %d messages)\n",
				len(cycleState.CycleHistory)-startIdx, len(cycleState.CycleHistory)))
		}

		for i := startIdx; i < len(cycleState.CycleHistory); i++ {
			msg := cycleState.CycleHistory[i]
			sb.WriteString(fmt.Sprintf("%d. %s: %s\n", i+1, msg.Role, msg.Content))
		}
	}
	sb.WriteString("\n")

	// Last cycle context
	if cycleState.LastCycleContext != nil {
		sb.WriteString("=== LAST_CYCLE_CONTEXT ===\n")
		if len(cycleState.LastCycleContext.Groups) > 0 {
			sb.WriteString(fmt.Sprintf("Groups: %s\n", strings.Join(cycleState.LastCycleContext.Groups, ", ")))
		}
		if len(cycleState.LastCycleContext.Subgroups) > 0 {
			sb.WriteString(fmt.Sprintf("Subgroups: %s\n", strings.Join(cycleState.LastCycleContext.Subgroups, ", ")))
		}
		if len(cycleState.LastCycleContext.Products) > 0 {
			sb.WriteString("Products from last cycle:\n")
			for _, p := range cycleState.LastCycleContext.Products {
				sb.WriteString(fmt.Sprintf("  - %s (%.2f)\n", p.Name, p.Price))
			}
		}
		if cycleState.LastCycleContext.LastRequest != "" {
			sb.WriteString(fmt.Sprintf("Last request: %s\n", cycleState.LastCycleContext.LastRequest))
		}
		sb.WriteString("\n")
	}

	// Last defined products
	if len(cycleState.LastDefined) > 0 {
		sb.WriteString("=== LAST_DEFINED (confirmed products) ===\n")
		sb.WriteString(strings.Join(cycleState.LastDefined, ", ") + "\n")
		sb.WriteString("\n")
	}

	return sb.String()
}

// InitializeCycleState creates a new cycle state for a session
func (upm *UniversalPromptManager) InitializeCycleState() models.CycleState {
	return models.CycleState{
		CycleID:          1,
		Iteration:        1,
		CycleHistory:     []models.CycleMessage{},
		LastCycleContext: nil,
		LastDefined:      []string{},
		PromptID:         PromptIDUniversal,
		PromptHash:       upm.GetPromptHash(),
	}
}

// IncrementIteration increments the iteration counter
// Returns true if we should continue in the same cycle, false if we need a new cycle
func (upm *UniversalPromptManager) IncrementIteration(cycleState *models.CycleState) bool {
	// Check BEFORE incrementing to properly handle iteration 6
	if cycleState.Iteration >= MaxIterations {
		fmt.Printf("⚠️ Max iterations reached (%d), need new cycle\n", MaxIterations)
		return false // Need new cycle
	}

	cycleState.Iteration++
	fmt.Printf("📊 Cycle %d, Iteration %d/%d\n", cycleState.CycleID, cycleState.Iteration, MaxIterations)

	return true
}

// StartNewCycle starts a new cycle, carrying over context
func (upm *UniversalPromptManager) StartNewCycle(
	cycleState *models.CycleState,
	lastRequest string,
	products []models.ProductInfo,
) {
	// Save current cycle context
	lastContext := &models.LastCycleContext{
		Groups:      extractGroups(cycleState.CycleHistory),
		Subgroups:   extractSubgroups(cycleState.CycleHistory),
		Products:    products,
		LastRequest: lastRequest,
	}

	// Increment cycle ID
	cycleState.CycleID++
	cycleState.Iteration = 1
	cycleState.CycleHistory = []models.CycleMessage{}
	cycleState.LastCycleContext = lastContext

	fmt.Printf("🔄 Starting new Cycle %d (carried over context from previous cycle)\n", cycleState.CycleID)
}

// AddToCycleHistory adds a message to the current cycle history
func (upm *UniversalPromptManager) AddToCycleHistory(
	cycleState *models.CycleState,
	role, content string,
) {
	msg := models.CycleMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}
	cycleState.CycleHistory = append(cycleState.CycleHistory, msg)
}

// GetPromptHash returns the SHA-256 hash of the universal prompt
func (upm *UniversalPromptManager) GetPromptHash() string {
	upm.mu.RLock()
	defer upm.mu.RUnlock()
	return upm.promptHash
}

// GetPromptHashShort returns a short version of the hash for logging
func (upm *UniversalPromptManager) GetPromptHashShort() string {
	return upm.promptHasher.HashPromptShort(upm.universalPrompt)
}

// GetPromptID returns the prompt version identifier
func (upm *UniversalPromptManager) GetPromptID() string {
	return PromptIDUniversal
}

// Helper functions

func getCategory(cycleState *models.CycleState) string {
	// Try to extract category from cycle history
	// Look for the latest category mentioned in assistant messages
	for i := len(cycleState.CycleHistory) - 1; i >= 0; i-- {
		msg := cycleState.CycleHistory[i]
		if msg.Role == "assistant" {
			// Try to parse JSON to extract category
			var resp map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Content), &resp); err == nil {
				if cat, ok := resp["category"].(string); ok && cat != "" {
					return cat
				}
			}
		}
	}
	return "unknown"
}

func extractGroups(history []models.CycleMessage) []string {
	groups := make(map[string]bool)
	// Simple extraction - in practice, you'd parse assistant responses for group mentions
	// For now, return empty as this is implementation-dependent
	result := make([]string, 0, len(groups))
	for g := range groups {
		result = append(result, g)
	}
	return result
}

func extractSubgroups(history []models.CycleMessage) []string {
	subgroups := make(map[string]bool)
	// Simple extraction - in practice, you'd parse assistant responses for subgroup mentions
	result := make([]string, 0, len(subgroups))
	for s := range subgroups {
		result = append(result, s)
	}
	return result
}

// ==================== Smart Context Management Methods ====================

// BuildMinimalContext builds minimal context for simple queries (e.g., "cheaper?")
// Only includes last product and essential state
func (upm *UniversalPromptManager) BuildMinimalContext(
	session *models.ChatSession,
) string {
	var sb strings.Builder

	sb.WriteString("=== MINIMAL CONTEXT ===\n")
	sb.WriteString(fmt.Sprintf("CYCLE: %d, ITERATION: %d\n", session.CycleState.CycleID, session.CycleState.Iteration))

	// Last 1-2 messages only
	history := session.CycleState.CycleHistory
	if len(history) > 0 {
		sb.WriteString("\nRecent exchange:\n")
		startIdx := len(history) - 2
		if startIdx < 0 {
			startIdx = 0
		}
		for i := startIdx; i < len(history); i++ {
			msg := history[i]
			// Truncate long messages
			content := msg.Content
			if len(content) > 150 {
				content = content[:147] + "..."
			}
			sb.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, content))
		}
	}

	// Last product shown
	if session.SearchState.LastProduct != nil {
		sb.WriteString(fmt.Sprintf("\nLast product: %s (%.2f %s)\n",
			session.SearchState.LastProduct.Name,
			session.SearchState.LastProduct.Price,
			session.Currency))
	}

	// Conversation context if available
	if session.ConversationContext != nil {
		if session.ConversationContext.LastSearch != nil {
			sb.WriteString(fmt.Sprintf("Last search: %s\n", session.ConversationContext.LastSearch.Query))
		}
	}

	return sb.String()
}

// BuildCompactStateContext builds compact context with configurable depth
// maxRecentMessages: how many recent messages to include (2-6)
func (upm *UniversalPromptManager) BuildCompactStateContext(
	session *models.ChatSession,
	maxRecentMessages int,
) string {
	var sb strings.Builder

	sb.WriteString("=== STATE CONTEXT ===\n")
	sb.WriteString(fmt.Sprintf("CYCLE: %d, ITERATION: %d/%d, CATEGORY: %s\n",
		session.CycleState.CycleID,
		session.CycleState.Iteration,
		MaxIterations,
		getCategory(&session.CycleState)))

	// Include conversation summary if available
	if session.ConversationContext != nil && session.ConversationContext.Summary != "" {
		sb.WriteString("\n=== CONVERSATION SUMMARY ===\n")
		sb.WriteString(session.ConversationContext.Summary + "\n")

		// Include structured preferences
		prefs := session.ConversationContext.Preferences
		if prefs.PriceRange != nil {
			sb.WriteString(fmt.Sprintf("\nPrice range: %.0f-%.0f %s\n",
				ptrFloat64Value(prefs.PriceRange.Min),
				ptrFloat64Value(prefs.PriceRange.Max),
				prefs.PriceRange.Currency))
		}
		if len(prefs.Brands) > 0 {
			sb.WriteString(fmt.Sprintf("Preferred brands: %s\n", strings.Join(prefs.Brands, ", ")))
		}
		if len(prefs.Features) > 0 {
			sb.WriteString(fmt.Sprintf("Required features: %s\n", strings.Join(prefs.Features, ", ")))
		}
		if len(session.ConversationContext.Exclusions) > 0 {
			sb.WriteString(fmt.Sprintf("Exclusions: %s\n", strings.Join(session.ConversationContext.Exclusions, ", ")))
		}
	}

	// Recent messages (limited)
	sb.WriteString("\n=== RECENT MESSAGES ===\n")
	history := session.CycleState.CycleHistory
	if len(history) == 0 {
		sb.WriteString("(no messages yet)\n")
	} else {
		startIdx := len(history) - maxRecentMessages
		if startIdx < 0 {
			startIdx = 0
		}

		if startIdx > 0 {
			sb.WriteString(fmt.Sprintf("(showing last %d of %d)\n", len(history)-startIdx, len(history)))
		}

		for i := startIdx; i < len(history); i++ {
			msg := history[i]
			// Truncate very long messages
			content := msg.Content
			if len(content) > 300 {
				content = content[:297] + "..."
			}
			sb.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, content))
		}
	}

	// Last product if available
	if session.SearchState.LastProduct != nil {
		sb.WriteString(fmt.Sprintf("\nLast product shown: %s (%.2f %s)\n",
			session.SearchState.LastProduct.Name,
			session.SearchState.LastProduct.Price,
			session.Currency))
	}

	return sb.String()
}

// BuildFullContext builds complete context (original behavior)
// This is the same as BuildStateContext but kept for clarity
func (upm *UniversalPromptManager) BuildFullContext(
	session *models.ChatSession,
) string {
	// Use the original BuildStateContext for full context
	return upm.BuildStateContext(session)
}

// Helper function to safely get float64 value from pointer
func ptrFloat64Value(ptr *float64) float64 {
	if ptr == nil {
		return 0
	}
	return *ptr
}

// getPromptBasePath determines the base path for loading prompt files
// Tries multiple locations to support both development and Docker environments
func getPromptBasePath() string {
	// First, check if PROMPT_BASE_PATH env var is set
	if basePath := os.Getenv("PROMPT_BASE_PATH"); basePath != "" {
		return basePath
	}

	// Try different possible locations
	possiblePaths := []string{
		"",           // Current directory (for development)
		"./",         // Explicit current directory
		"/app/",      // Docker deployment
	}

	for _, path := range possiblePaths {
		testPath := path + "internal/services/prompts/universal_prompt.txt"
		if _, err := os.Stat(testPath); err == nil {
			return path
		}
	}

	// Default to current directory
	return ""
}
