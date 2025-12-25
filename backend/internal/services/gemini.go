// backend/internal/services/gemini.go
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"

	"mylittleprice/internal/config"
	"mylittleprice/internal/models"
	"mylittleprice/internal/utils"
)

type GeminiService struct {
	client             *genai.Client
	keyRotator         *utils.KeyRotator
	config             *config.Config
	promptManager      *PromptManager
	universalPromptMgr *UniversalPromptManager
	groundingStats     *GroundingStats
	groundingStrategy  *GroundingStrategy
	tokenStats         *TokenStats
	embedding          *EmbeddingService
	contextOptimizer   *ContextOptimizerService // NEW: Determines optimal context depth
	contextExtractor   *ContextExtractorService // NEW: Extracts preferences and summaries
	ctx                context.Context
	currentKeyIndex    int // Track current API key index
	mu                 sync.RWMutex
}

type TokenStats struct {
	mu                    sync.RWMutex
	TotalRequests         int
	TotalInputTokens      int64
	TotalOutputTokens     int64
	TotalTokens           int64
	RequestsWithGrounding int
	AverageInputTokens    float64
	AverageOutputTokens   float64
}

type GroundingStats struct {
	TotalDecisions    int
	GroundingEnabled  int
	GroundingDisabled int
	ReasonCounts      map[string]int
	AverageConfidence float32
}

func NewGeminiService(keyRotator *utils.KeyRotator, cfg *config.Config, embedding *EmbeddingService) *GeminiService {
	ctx := context.Background()

	apiKey, keyIndex, err := keyRotator.GetNextKey()
	if err != nil {
		panic(fmt.Errorf("failed to get initial API key: %w", err))
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		panic(fmt.Errorf("failed to create Gemini client: %w", err))
	}

	return &GeminiService{
		client:             client,
		keyRotator:         keyRotator,
		config:             cfg,
		promptManager:      NewPromptManager(),
		universalPromptMgr: NewUniversalPromptManager(),
		groundingStats:     &GroundingStats{ReasonCounts: make(map[string]int)},
		groundingStrategy:  NewGroundingStrategy(embedding, cfg),
		tokenStats:         &TokenStats{},
		embedding:          embedding,
		contextOptimizer:   NewContextOptimizerService(embedding),                       // NEW
		contextExtractor:   NewContextExtractorService(client, cfg.GeminiFallbackModel), // NEW - Use fallback model for lightweight tasks
		ctx:                ctx,
		currentKeyIndex:    keyIndex,
	}
}

func (g *GeminiService) rotateClient(markCurrentAsExhausted bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Mark current key as exhausted if requested
	if markCurrentAsExhausted {
		fmt.Printf("   ⚠️ Marking Gemini key %d as exhausted\n", g.currentKeyIndex)
		if err := g.keyRotator.MarkKeyAsExhausted(g.currentKeyIndex); err != nil {
			fmt.Printf("   ⚠️ Failed to mark key as exhausted: %v\n", err)
		}
	}

	apiKey, keyIndex, err := g.keyRotator.GetNextKey()
	if err != nil {
		return fmt.Errorf("failed to get API key: %w", err)
	}

	client, err := genai.NewClient(g.ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return fmt.Errorf("failed to create Gemini client: %w", err)
	}

	g.client = client
	g.currentKeyIndex = keyIndex
	fmt.Printf("   🔄 Gemini API key rotated to key %d\n", keyIndex)
	return nil
}

func (g *GeminiService) ProcessMessageWithContext(
	userMessage string,
	conversationHistory []map[string]string,
	country string,
	language string,
	currentCategory string,
	lastProduct *models.ProductInfo,
) (*models.GeminiResponse, int, error) {

	if currentCategory == "" {
		detectedCategory := g.embedding.DetectCategory(userMessage)
		if detectedCategory != "" {
			currentCategory = detectedCategory
		}
	}

	promptKey := g.promptManager.GetPromptKey(currentCategory)
	systemPrompt := g.promptManager.GetPrompt(promptKey, country, language, currentCategory)

	lastProductStr := ""
	if lastProduct != nil {
		lastProductStr = fmt.Sprintf("%s (%.2f)", lastProduct.Name, lastProduct.Price)
	}

	systemPrompt = strings.ReplaceAll(systemPrompt, "{last_product}", lastProductStr)
	conversationContext := g.buildConversationContext(conversationHistory)

	prompt := systemPrompt + "\n\n# CONVERSATION HISTORY:\n" + conversationContext +
		"\n\nCurrent user message: " + userMessage +
		"\n\nCRITICAL INSTRUCTIONS:\n- You MUST respond with valid JSON only\n- If using grounding/search results, incorporate the information naturally\n- ALWAYS end your response with valid JSON in this exact format:\n{\"response_type\":\"dialogue\",\"output\":\"...\",\"quick_replies\":[...],\"category\":\"...\"}\nOR\n{\"response_type\":\"search\",\"search_phrase\":\"...\",\"search_type\":\"...\",\"category\":\"...\"}\n\nAnalyze the conversation history above. If the last assistant question was similar to what the current situation requires, provide a DIFFERENT question to move the conversation forward."

	temp := g.config.GeminiTemperature
	generateConfig := &genai.GenerateContentConfig{
		Temperature:     &temp,
		MaxOutputTokens: int32(g.config.GeminiMaxOutputTokens),
	}

	useGrounding := g.shouldUseGrounding(userMessage, conversationHistory, currentCategory)
	if useGrounding {
		generateConfig.Tools = []*genai.Tool{
			{GoogleSearch: &genai.GoogleSearch{}},
		}
	} else {
		generateConfig.ResponseMIMEType = "application/json"
	}

	g.mu.RLock()
	client := g.client
	g.mu.RUnlock()

	resp, err := client.Models.GenerateContent(
		g.ctx,
		g.config.GeminiModel,
		genai.Text(prompt),
		generateConfig,
	)

	// Если ошибка - пробуем ротировать ключ и повторить
	if err != nil {
		// Проверяем если это quota/rate limit ошибка
		if strings.Contains(err.Error(), "quota") ||
			strings.Contains(err.Error(), "429") ||
			strings.Contains(err.Error(), "RESOURCE_EXHAUSTED") {

			// Ротируем клиент (помечаем текущий ключ как exhausted)
			if rotateErr := g.rotateClient(true); rotateErr != nil {
				return nil, 0, fmt.Errorf("Gemini API error: %w, rotation failed: %v", err, rotateErr)
			}

			// Повторяем запрос с новым клиентом
			g.mu.RLock()
			client = g.client
			g.mu.RUnlock()

			resp, err = client.Models.GenerateContent(
				g.ctx,
				g.config.GeminiModel,
				genai.Text(prompt),
				generateConfig,
			)

			if err != nil {
				return nil, 0, fmt.Errorf("Gemini API error after rotation: %w", err)
			}
		} else {
			return nil, 0, fmt.Errorf("Gemini API error: %w", err)
		}
	}

	if resp == nil {
		return nil, 0, fmt.Errorf("Gemini returned nil response")
	}

	if resp.UsageMetadata != nil {
		g.updateTokenStats(resp.UsageMetadata, useGrounding)
	}

	if len(resp.Candidates) == 0 {
		return nil, 0, fmt.Errorf("no candidates in Gemini response")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return nil, 0, fmt.Errorf("no content in Gemini response")
	}

	responseText := ""
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			responseText += part.Text
		}
	}

	responseText = strings.TrimSpace(responseText)

	// Если использовали grounding, извлекаем JSON из текста
	if useGrounding {
		responseText = g.extractJSONFromText(responseText)
	}

	responseText = strings.Trim(responseText, "`")
	responseText = strings.TrimPrefix(responseText, "json")
	responseText = strings.TrimSpace(responseText)

	if responseText == "" {
		return nil, 0, fmt.Errorf("empty response text from Gemini")
	}

	// DEBUG: Log raw JSON response from Gemini
	fmt.Printf("🔍 RAW GEMINI JSON:\n%s\n", responseText)

	var geminiResp models.GeminiResponse
	if err := json.Unmarshal([]byte(responseText), &geminiResp); err != nil {
		return nil, 0, fmt.Errorf("failed to parse Gemini JSON response: %w (response: %s)", err, responseText)
	}

	// DEBUG: Log parsed product_description
	fmt.Printf("🔍 PARSED product_description: '%s'\n", geminiResp.ProductDescription)

	if geminiResp.ResponseType == "" {
		return nil, 0, fmt.Errorf("missing response_type in Gemini response")
	}

	return &geminiResp, 0, nil
}

func (g *GeminiService) buildConversationContext(history []map[string]string) string {
	if len(history) == 0 {
		return "No previous messages"
	}

	var context strings.Builder
	for i, msg := range history {
		context.WriteString(fmt.Sprintf("%d. %s: %s\n", i+1, msg["role"], msg["content"]))
	}
	return context.String()
}

// shouldUseGrounding determines if Google Search grounding should be enabled
func (g *GeminiService) shouldUseGrounding(userMessage string, history []map[string]string, category string) bool {
	if !g.config.GeminiUseGrounding {
		return false
	}

	// ALWAYS enable grounding for better product information
	// This ensures AI has up-to-date product data, prices, and availability
	// The smart strategy was too conservative and missed important cases
	g.groundingStats.TotalDecisions++
	g.groundingStats.GroundingEnabled++
	g.groundingStats.ReasonCounts["always_enabled"]++
	g.groundingStats.AverageConfidence = 1.0

	return true
}

func (g *GeminiService) extractJSONFromText(text string) string {
	// When grounding is used, Gemini sometimes duplicates the response
	// We need to extract only the FIRST complete JSON object

	text = strings.TrimSpace(text)

	// Check for duplicate JSON (when text contains multiple "{")
	firstBraceIdx := strings.Index(text, "{")
	if firstBraceIdx == -1 {
		return text
	}

	// Find the matching closing brace for the FIRST JSON object
	braceCount := 0
	inString := false
	escapeNext := false

	for i := firstBraceIdx; i < len(text); i++ {
		char := text[i]

		if escapeNext {
			escapeNext = false
			continue
		}

		if char == '\\' {
			escapeNext = true
			continue
		}

		if char == '"' && !escapeNext {
			inString = !inString
			continue
		}

		if !inString {
			if char == '{' {
				braceCount++
			} else if char == '}' {
				braceCount--
				if braceCount == 0 {
					// Found the end of the first complete JSON object
					firstJSON := text[firstBraceIdx : i+1]
					fmt.Printf("🔍 Extracted first JSON object (%d chars)\n", len(firstJSON))
					return firstJSON
				}
			}
		}
	}

	// Fallback: try to find JSON in code blocks
	if strings.Contains(text, "```json") {
		start := strings.Index(text, "```json")
		end := strings.Index(text[start+7:], "```")
		if end != -1 {
			return strings.TrimSpace(text[start+7 : start+7+end])
		}
	}

	// If we couldn't find a complete JSON, return original
	return text
}

func (g *GeminiService) updateTokenStats(metadata *genai.GenerateContentResponseUsageMetadata, withGrounding bool) {
	g.tokenStats.mu.Lock()
	defer g.tokenStats.mu.Unlock()

	g.tokenStats.TotalRequests++
	g.tokenStats.TotalInputTokens += int64(metadata.PromptTokenCount)

	outputTokens := int64(0)
	if metadata.TotalTokenCount > 0 && metadata.PromptTokenCount > 0 {
		outputTokens = int64(metadata.TotalTokenCount - metadata.PromptTokenCount)
	}
	g.tokenStats.TotalOutputTokens += outputTokens
	g.tokenStats.TotalTokens += int64(metadata.TotalTokenCount)

	if withGrounding {
		g.tokenStats.RequestsWithGrounding++
	}

	g.tokenStats.AverageInputTokens = float64(g.tokenStats.TotalInputTokens) / float64(g.tokenStats.TotalRequests)
	g.tokenStats.AverageOutputTokens = float64(g.tokenStats.TotalOutputTokens) / float64(g.tokenStats.TotalRequests)
}

func (g *GeminiService) GetTokenStats() *TokenStats {
	g.tokenStats.mu.RLock()
	defer g.tokenStats.mu.RUnlock()

	stats := *g.tokenStats
	return &stats
}

func (g *GeminiService) GetGroundingStats() *GroundingStats {
	return g.groundingStats
}

// executeWithRetry performs Gemini API call with exponential backoff retry logic
func (g *GeminiService) executeWithRetry(
	prompt string,
	config *genai.GenerateContentConfig,
	maxRetries int,
) (*genai.GenerateContentResponse, error) {
	return g.executeWithRetryAndModel(prompt, config, maxRetries, g.config.GeminiModel, false)
}

// executeWithRetryAndModel performs Gemini API call with specific model and fallback support
func (g *GeminiService) executeWithRetryAndModel(
	prompt string,
	config *genai.GenerateContentConfig,
	maxRetries int,
	modelName string,
	isFallback bool,
) (*genai.GenerateContentResponse, error) {
	// Track metrics for AI request
	start := time.Now()
	var lastErr error

	defer func() {
		duration := time.Since(start).Seconds()
		status := "success"
		if lastErr != nil {
			status = "error"
		}
		// Metrics tracking removed - request duration: %.2fs, status: %s
		_ = duration
		_ = status
	}()

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s, 8s...
			backoffDuration := time.Duration(1<<uint(attempt-1)) * time.Second
			fmt.Printf("⏳ Retry attempt %d/%d after %v...\n", attempt+1, maxRetries, backoffDuration)
			time.Sleep(backoffDuration)
		}

		// Get current client
		g.mu.RLock()
		client := g.client
		g.mu.RUnlock()

		// Log which model we're using
		if isFallback {
			fmt.Printf("🔄 Using fallback model: %s (attempt %d/%d)\n", modelName, attempt+1, maxRetries)
		}

		// Execute API call with timeout context
		ctx, cancel := context.WithTimeout(g.ctx, 30*time.Second)
		resp, err := client.Models.GenerateContent(
			ctx,
			modelName,
			genai.Text(prompt),
			config,
		)
		cancel()

		// Success case
		if err == nil && resp != nil {
			if attempt > 0 {
				fmt.Printf("✅ Request succeeded on attempt %d/%d\n", attempt+1, maxRetries)
			}
			return resp, nil
		}

		lastErr = err

		// Handle different error types
		if err != nil {
			errMsg := err.Error()

			// Quota/Rate limit errors - rotate key and retry
			if strings.Contains(errMsg, "quota") ||
				strings.Contains(errMsg, "429") ||
				strings.Contains(errMsg, "RESOURCE_EXHAUSTED") {

				fmt.Printf("⚠️ Quota exceeded, rotating API key...\n")
				if rotateErr := g.rotateClient(true); rotateErr != nil {
					fmt.Printf("❌ Key rotation failed: %v\n", rotateErr)
					// Continue to next retry anyway
				}
				continue
			}

			// Overload/503 errors - retry with backoff
			if strings.Contains(errMsg, "503") ||
				strings.Contains(errMsg, "UNAVAILABLE") ||
				strings.Contains(errMsg, "overloaded") {

				fmt.Printf("⚠️ Service overloaded (503), will retry...\n")
				continue
			}

			// Timeout errors - retry
			if strings.Contains(errMsg, "timeout") ||
				strings.Contains(errMsg, "deadline exceeded") {

				fmt.Printf("⚠️ Request timeout, will retry...\n")
				continue
			}

			// Other errors - don't retry
			fmt.Printf("❌ Non-retryable error: %v\n", err)
			return nil, fmt.Errorf("Gemini API error: %w", err)
		}
	}

	// All retries exhausted
	fmt.Printf("❌ All %d retry attempts failed\n", maxRetries)
	if lastErr != nil {
		return nil, fmt.Errorf("Gemini API failed after %d retries: %w", maxRetries, lastErr)
	}
	return nil, fmt.Errorf("Gemini API failed after %d retries with unknown error", maxRetries)
}

// ProcessWithUniversalPrompt processes a message using the Universal Prompt system
// This is the NEW method that should be used instead of ProcessMessageWithContext
func (g *GeminiService) ProcessWithUniversalPrompt(
	userMessage string,
	session *models.ChatSession,
) (*models.GeminiResponse, error) {

	// Build the prompt using Universal Prompt Manager
	upm := g.universalPromptMgr

	// Get the mini-kernel with current state
	miniKernel := upm.GetMiniKernel(
		session.CountryCode,
		session.LanguageCode,
		session.Currency,
		&session.CycleState,
	)

	// NEW: Determine optimal context depth based on user message
	contextDepth := g.contextOptimizer.DecideContextDepth(userMessage, session)

	// NEW: Build state context based on depth
	var stateContext string
	switch contextDepth {
	case ContextDepthMinimal:
		stateContext = upm.BuildMinimalContext(session)
		fmt.Printf("💡 Using MINIMAL context (token efficient)\n")
	case ContextDepthMedium:
		stateContext = upm.BuildCompactStateContext(session, 3) // Last 3 messages
		fmt.Printf("💡 Using MEDIUM context (3 messages)\n")
	case ContextDepthFull:
		stateContext = upm.BuildFullContext(session)
		fmt.Printf("💡 Using FULL context (complete history)\n")
	default:
		stateContext = upm.BuildCompactStateContext(session, 3)
		fmt.Printf("💡 Using MEDIUM context (default)\n")
	}

	// On first iteration of FIRST cycle only, include full system prompt
	// For subsequent cycles, rely on mini-kernel + compact context
	var systemPrompt string
	if session.CycleState.CycleID == 1 && session.CycleState.Iteration == 1 {
		systemPrompt = upm.GetSystemPrompt(
			session.CountryCode,
			session.LanguageCode,
			session.Currency,
		)
		fmt.Printf("📝 Sending full Universal Prompt (first message in session)\n")
	}

	// Build the full prompt: (system prompt if first) + mini-kernel + state + user message
	// Note: When grounding is disabled, ResponseSchema ensures JSON output.
	// When grounding is enabled, we rely on mini_kernel prompt for JSON format (API limitation).
	var prompt string
	if systemPrompt != "" {
		prompt = fmt.Sprintf("%s\n\n%s\n\n%s\n\nUser message: %s",
			systemPrompt,
			miniKernel,
			stateContext,
			userMessage,
		)
	} else {
		prompt = fmt.Sprintf("%s\n\n%s\n\nUser message: %s",
			miniKernel,
			stateContext,
			userMessage,
		)
	}

	// Log telemetry
	fmt.Printf("📊 Prompt Telemetry: ID=%s, Hash=%s, Cycle=%d, Iteration=%d\n",
		session.CycleState.PromptID,
		upm.GetPromptHashShort(),
		session.CycleState.CycleID,
		session.CycleState.Iteration,
	)

	temp := g.config.GeminiTemperature
	generateConfig := &genai.GenerateContentConfig{
		Temperature:     &temp,
		MaxOutputTokens: int32(g.config.GeminiMaxOutputTokens),
	}

	// Grounding is ALWAYS enabled (configured in shouldUseGrounding method)
	// This ensures AI always has access to current product data, prices, and models
	historyMap := convertCycleHistoryToMap(session.CycleState.CycleHistory)
	useGrounding := g.shouldUseGrounding(userMessage, historyMap, session.SearchState.Category)

	if useGrounding {
		fmt.Printf("🌐 Grounding enabled (smart strategy)\n")
		generateConfig.Tools = []*genai.Tool{
			{GoogleSearch: &genai.GoogleSearch{}},
		}
		// When using grounding/tools, we CANNOT use ResponseSchema or ResponseMIMEType
		// The API returns error: "Unsupported response mime type when response schema is set"
		// We rely on the prompt to request JSON format instead
		fmt.Printf("📋 Relying on prompt for JSON structure (Tools mode)\n")
	} else {
		fmt.Printf("📝 Grounding disabled (not needed for this query)\n")
		// Without grounding, we can use both MIME type and schema
		generateConfig.ResponseMIMEType = "application/json"
		generateConfig.ResponseSchema = GetUniversalResponseSchema()
	}

	// Execute API call with retry logic (max 3 attempts with exponential backoff)
	resp, err := g.executeWithRetry(prompt, generateConfig, 3)

	// If primary model failed and we have a fallback model configured, try fallback
	if err != nil && g.config.GeminiFallbackModel != "" && g.config.GeminiFallbackModel != g.config.GeminiModel {
		fmt.Printf("⚠️ Primary model (%s) failed, trying fallback model (%s)\n",
			g.config.GeminiModel, g.config.GeminiFallbackModel)

		resp, err = g.executeWithRetryAndModel(prompt, generateConfig, 2, g.config.GeminiFallbackModel, true)

		if err != nil {
			fmt.Printf("❌ Fallback model also failed: %v\n", err)
			return nil, fmt.Errorf("both primary and fallback models failed: %w", err)
		}

		fmt.Printf("✅ Fallback model succeeded\n")
	} else if err != nil {
		return nil, err
	}

	if resp == nil {
		return nil, fmt.Errorf("Gemini returned nil response")
	}

	if resp.UsageMetadata != nil {
		g.updateTokenStats(resp.UsageMetadata, useGrounding)
	}

	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in Gemini response")
	}

	candidate := resp.Candidates[0]

	// Check for MAX_TOKENS finish reason - this means response was truncated
	if candidate.FinishReason == genai.FinishReasonMaxTokens {
		fmt.Printf("⚠️ Response truncated due to MAX_TOKENS, retrying without grounding...\n")

		// Retry without grounding to get shorter response
		retryConfig := &genai.GenerateContentConfig{
			Temperature:      generateConfig.Temperature,
			MaxOutputTokens:  generateConfig.MaxOutputTokens,
			ResponseMIMEType: "application/json",
			ResponseSchema:   GetUniversalResponseSchema(),
		}

		retryResp, retryErr := g.executeWithRetry(prompt, retryConfig, 2)
		if retryErr == nil && retryResp != nil && len(retryResp.Candidates) > 0 {
			resp = retryResp
			candidate = resp.Candidates[0]
			fmt.Printf("✅ Retry without grounding succeeded\n")
			useGrounding = false // Update flag for stats
		} else {
			fmt.Printf("⚠️ Retry without grounding also failed, continuing with truncated response...\n")
		}
	}

	if candidate.Content == nil {
		fmt.Printf("⚠️ Candidate has no content. Finish reason: %v\n", candidate.FinishReason)
		return nil, fmt.Errorf("no content in Gemini response (finish reason: %v)", candidate.FinishReason)
	}
	if len(candidate.Content.Parts) == 0 {
		fmt.Printf("⚠️ Candidate content has no parts. Finish reason: %v\n", candidate.FinishReason)
		return nil, fmt.Errorf("no parts in Gemini response content (finish reason: %v)", candidate.FinishReason)
	}

	responseText := ""
	hasGroundingMetadata := false

	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			responseText += part.Text
		}
	}

	// Check if there's grounding metadata (search results)
	if resp.Candidates[0].GroundingMetadata != nil {
		hasGroundingMetadata = true
		if resp.Candidates[0].GroundingMetadata.GroundingChunks != nil {
			chunks := resp.Candidates[0].GroundingMetadata.GroundingChunks
			fmt.Printf("✅ Grounding metadata found (%d chunks)\n", len(chunks))

			// Log first 3 chunks for debugging price/model accuracy
			for i, chunk := range chunks {
				if i >= 3 {
					break
				}
				if chunk.Web != nil && chunk.Web.Title != "" {
					fmt.Printf("   📄 Chunk %d: %s\n", i+1, chunk.Web.Title)
					if chunk.Web.URI != "" {
						fmt.Printf("      🔗 Source: %s\n", chunk.Web.URI)
					}
				}
			}
		} else {
			fmt.Printf("✅ Grounding metadata present\n")
		}
	}

	responseText = strings.TrimSpace(responseText)

	// If grounding was used but no text yet, the model might still be processing
	if responseText == "" && hasGroundingMetadata {
		fmt.Printf("⚠️ Grounding search completed but no text response yet\n")
	}

	// Extract JSON if grounding was used
	if useGrounding {
		responseText = g.extractJSONFromText(responseText)
	}

	responseText = strings.Trim(responseText, "`")
	responseText = strings.TrimPrefix(responseText, "json")
	responseText = strings.TrimSpace(responseText)

	if responseText == "" {
		return nil, fmt.Errorf("empty response text from Gemini")
	}

	// DEBUG: Log raw JSON response from Gemini
	fmt.Printf("🔍 RAW GEMINI JSON:\n%s\n", responseText)

	var geminiResp models.GeminiResponse
	err = json.Unmarshal([]byte(responseText), &geminiResp)

	// DEBUG: Log parsed product_description after unmarshal
	if err == nil {
		fmt.Printf("🔍 PARSED product_description: '%s'\n", geminiResp.ProductDescription)
	}

	if err != nil {
		fmt.Printf("❌ Failed to parse JSON response. Raw response:\n%s\n", responseText)
		fmt.Printf("❌ Parse error: %v\n", err)

		// Multi-stage JSON repair pipeline
		repairedText := responseText

		// Stage 1: Remove duplicates (common with grounding)
		repairedText = removeDuplicateJSON(repairedText)
		if repairedText != responseText {
			fmt.Printf("🔧 Removed duplicate JSON objects\n")
			if err2 := json.Unmarshal([]byte(repairedText), &geminiResp); err2 == nil {
				fmt.Printf("✅ Successfully parsed after removing duplicates\n")
				goto success
			}
		}

		// Stage 2: Fix truncated JSON by closing open structures
		repairedText = attemptJSONRepair(repairedText)
		if repairedText != responseText {
			fmt.Printf("🔧 Attempting to repair truncated JSON...\n")
			if err2 := json.Unmarshal([]byte(repairedText), &geminiResp); err2 == nil {
				fmt.Printf("✅ Successfully repaired truncated JSON\n")
				goto success
			}
		}

		// Stage 3: Try to extract valid JSON from markdown/text
		extractedJSON := extractFirstValidJSON(responseText)
		if extractedJSON != "" && extractedJSON != responseText {
			fmt.Printf("🔧 Extracted JSON from text/markdown...\n")
			if err2 := json.Unmarshal([]byte(extractedJSON), &geminiResp); err2 == nil {
				fmt.Printf("✅ Successfully extracted valid JSON\n")
				goto success
			}
		}

		// All repair attempts failed
		return nil, fmt.Errorf("failed to parse Gemini JSON response after all repair attempts: %w (original response: %s)", err, responseText)
	}

success:

	if geminiResp.ResponseType == "" {
		fmt.Printf("❌ Missing response_type. Parsed response: %+v\nRaw text:\n%s\n", geminiResp, responseText)

		// Attempt intelligent fallback based on fields present
		if geminiResp.Output != "" || geminiResp.QuickReplies != nil {
			// Has dialogue-like fields
			fmt.Printf("🔧 Auto-correcting response_type to 'dialogue' based on fields present\n")
			geminiResp.ResponseType = "dialogue"
		} else if geminiResp.SearchPhrase != "" {
			// Has search-like fields
			fmt.Printf("🔧 Auto-correcting response_type to 'search' based on fields present\n")
			geminiResp.ResponseType = "search"
		} else if geminiResp.API != "" || geminiResp.Params != nil {
			// Has API-like fields
			fmt.Printf("🔧 Auto-correcting response_type to 'api_request' based on fields present\n")
			geminiResp.ResponseType = "api_request"
		} else {
			// Try to extract from raw text - handle common Gemini mistakes
			var rawJSON map[string]interface{}
			if err := json.Unmarshal([]byte(responseText), &rawJSON); err == nil {
				// Check for "response" field (common mistake)
				if responseField, ok := rawJSON["response"].(string); ok && responseField != "" {
					fmt.Printf("🔧 Found 'response' field, mapping to 'output' and setting type to 'dialogue'\n")
					geminiResp.Output = responseField
					geminiResp.ResponseType = "dialogue"
				} else if queryField, ok := rawJSON["query"].(string); ok && queryField != "" {
					// Check for "query" field instead of proper structure
					fmt.Printf("🔧 Found 'query' field, mapping to 'output' and setting type to 'dialogue'\n")
					geminiResp.Output = queryField
					geminiResp.ResponseType = "dialogue"
				} else if promptTextField, ok := rawJSON["prompt_text"].(string); ok && promptTextField != "" {
					// Check for "prompt_text" field
					fmt.Printf("🔧 Found 'prompt_text' field, mapping to 'output' and setting type to 'dialogue'\n")
					geminiResp.Output = promptTextField
					geminiResp.ResponseType = "dialogue"
				} else if responseObj, ok := rawJSON["response"].(map[string]interface{}); ok {
					// Check for nested "response" object with "query_refinement"
					if queryRefinement, ok := responseObj["query_refinement"].(string); ok && queryRefinement != "" {
						fmt.Printf("🔧 Found nested 'response.query_refinement', mapping to 'output' and setting type to 'dialogue'\n")
						geminiResp.Output = queryRefinement
						geminiResp.ResponseType = "dialogue"
					}
				} else if assistantResp, ok := rawJSON["ASSISTANT_RESPONSE"].(string); ok && assistantResp != "" {
					// Check for "ASSISTANT_RESPONSE" field
					fmt.Printf("🔧 Found 'ASSISTANT_RESPONSE' field, mapping to 'output' and setting type to 'dialogue'\n")
					geminiResp.Output = assistantResp
					geminiResp.ResponseType = "dialogue"
				}

				// Extract category if present
				if geminiResp.Category == "" {
					if cat, ok := rawJSON["category"].(string); ok && cat != "" {
						geminiResp.Category = cat
					} else if cat, ok := rawJSON["query_category"].(string); ok && cat != "" {
						geminiResp.Category = cat
					} else if cat, ok := rawJSON["CURRENT_CATEGORY"].(string); ok && cat != "" {
						geminiResp.Category = cat
					}
				}
			}
		}

		// If still no response_type, fail
		if geminiResp.ResponseType == "" {
			return nil, fmt.Errorf("missing response_type in Gemini response and could not infer from fields")
		}
	}

	// Log category routing
	fmt.Printf("🏷️  Category routing: %s → %s\n", session.SearchState.Category, geminiResp.Category)

	return &geminiResp, nil
}

// Helper to convert CycleHistory to the old format for compatibility
func convertCycleHistoryToMap(history []models.CycleMessage) []map[string]string {
	result := make([]map[string]string, len(history))
	for i, msg := range history {
		result[i] = map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}
	return result
}

// TranslateToEnglish переводит поисковый запрос на английский язык
func (g *GeminiService) TranslateToEnglish(query string) (string, error) {
	// Если запрос уже на английском, возвращаем как есть
	if isEnglish(query) {
		return query, nil
	}

	prompt := fmt.Sprintf(`Translate this product search query to English. Keep it concise and optimized for Google Shopping.
Only return the translated query, nothing else.

Query: %s

Translated query:`, query)

	temp := g.config.GeminiTranslationTemperature
	generateConfig := &genai.GenerateContentConfig{
		Temperature:     &temp,
		MaxOutputTokens: int32(g.config.GeminiTranslationMaxTokens),
	}

	// Try with primary model first (2 retries)
	resp, err := g.executeWithRetryAndModel(prompt, generateConfig, 2, g.config.GeminiModel, false)

	// If primary model failed, try fallback model
	if err != nil && g.config.GeminiFallbackModel != "" && g.config.GeminiFallbackModel != g.config.GeminiModel {
		fmt.Printf("⚠️ Translation with primary model failed, trying fallback (%s)\n", g.config.GeminiFallbackModel)
		resp, err = g.executeWithRetryAndModel(prompt, generateConfig, 2, g.config.GeminiFallbackModel, true)

		if err != nil {
			fmt.Printf("❌ Translation with fallback model also failed: %v\n", err)
			return query, fmt.Errorf("translation failed with both models: %w", err)
		}

		fmt.Printf("✅ Translation succeeded with fallback model\n")
	} else if err != nil {
		return query, fmt.Errorf("translation failed: %w", err)
	}

	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return query, fmt.Errorf("empty translation response")
	}

	translatedText := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.Text != "" {
			translatedText += part.Text
		}
	}

	translatedText = strings.TrimSpace(translatedText)
	translatedText = strings.Trim(translatedText, `"'`)

	if translatedText == "" {
		return query, nil
	}

	return translatedText, nil
}

// isEnglish проверяет, является ли текст английским (простая эвристика)
func isEnglish(text string) bool {
	// Подсчитываем не-ASCII символы
	nonAsciiCount := 0
	for _, r := range text {
		if r > 127 {
			nonAsciiCount++
		}
	}

	// Если больше 20% не-ASCII символов, скорее всего не английский
	if len(text) > 0 && float64(nonAsciiCount)/float64(len(text)) > 0.2 {
		return false
	}

	return true
}

// GetContextExtractor returns the context extractor service
func (g *GeminiService) GetContextExtractor() *ContextExtractorService {
	return g.contextExtractor
}

// GetContextOptimizer returns the context optimizer service
func (g *GeminiService) GetContextOptimizer() *ContextOptimizerService {
	return g.contextOptimizer
}

// removeDuplicateJSON removes duplicate JSON objects that sometimes appear with grounding
// It handles both complete duplicates and duplicates that start mid-structure (like in arrays)
func removeDuplicateJSON(text string) string {
	text = strings.TrimSpace(text)

	firstBraceIdx := strings.Index(text, "{")
	if firstBraceIdx == -1 {
		return text
	}

	// Find the end of the first complete JSON object
	braceCount := 0
	bracketCount := 0
	inString := false
	escapeNext := false
	lastCompletePos := -1

	for i := firstBraceIdx; i < len(text); i++ {
		char := text[i]

		if escapeNext {
			escapeNext = false
			continue
		}

		if char == '\\' {
			escapeNext = true
			continue
		}

		if char == '"' {
			inString = !inString
			continue
		}

		if !inString {
			if char == '{' {
				braceCount++
			} else if char == '}' {
				braceCount--
				if braceCount == 0 && bracketCount == 0 {
					// Found end of complete JSON object
					lastCompletePos = i + 1

					// Check if there's more content after
					if lastCompletePos < len(text) {
						remaining := strings.TrimSpace(text[lastCompletePos:])

						// If remaining starts with '{', it's a duplicate - cut here
						if strings.HasPrefix(remaining, "{") {
							fmt.Printf("🔍 Complete duplicate JSON detected at position %d\n", lastCompletePos)
							return text[firstBraceIdx:lastCompletePos]
						}
					}

					// No duplicate found, return complete JSON
					return text[firstBraceIdx:lastCompletePos]
				}
			} else if char == '[' {
				bracketCount++
			} else if char == ']' {
				bracketCount--
			}
		}
	}

	// JSON is incomplete - check if there's a duplicate that caused truncation
	// Look for another '{' that starts a duplicate object
	if lastCompletePos == -1 {
		// Try to find where duplication might have started
		// Common pattern: `"text": "value"{"response_type"...`
		secondBraceIdx := strings.Index(text[firstBraceIdx+1:], "{")
		if secondBraceIdx != -1 {
			actualSecondIdx := firstBraceIdx + 1 + secondBraceIdx

			// Check if this second brace is NOT inside a string value
			// by counting quotes before it
			beforeSecond := text[firstBraceIdx:actualSecondIdx]
			quoteCount := 0
			escapeCount := 0
			for _, ch := range beforeSecond {
				if ch == '\\' {
					escapeCount++
				} else if ch == '"' && escapeCount%2 == 0 {
					quoteCount++
				} else {
					escapeCount = 0
				}
			}

			// If odd number of quotes, the { is inside a string, skip it
			// If even, it's a structural brace - might be a duplicate
			if quoteCount%2 == 0 {
				// This looks like a duplicate starting mid-structure
				// Find the last complete statement before this duplicate
				lastComma := strings.LastIndex(text[:actualSecondIdx], ",")
				lastBracket := strings.LastIndex(text[:actualSecondIdx], "[")

				cutPoint := lastComma
				if lastBracket > lastComma {
					cutPoint = lastBracket
				}

				if cutPoint > firstBraceIdx {
					fmt.Printf("🔍 Mid-structure duplicate detected, cutting at position %d\n", cutPoint)
					// Try to close the JSON properly
					repaired := text[firstBraceIdx:cutPoint]
					return attemptJSONRepair(repaired)
				}
			}
		}
	}

	// Return as-is for other repair attempts
	return text
}

// extractFirstValidJSON attempts to find and extract the first valid JSON object from text
func extractFirstValidJSON(text string) string {
	text = strings.TrimSpace(text)

	// Try to find JSON in code blocks first
	if strings.Contains(text, "```json") {
		start := strings.Index(text, "```json") + 7
		end := strings.Index(text[start:], "```")
		if end != -1 {
			extracted := strings.TrimSpace(text[start : start+end])
			if strings.HasPrefix(extracted, "{") {
				return extracted
			}
		}
	}

	// Try to find JSON in regular code blocks
	if strings.Contains(text, "```") {
		start := strings.Index(text, "```")
		// Skip the ``` and optional language identifier
		for start < len(text) && text[start] != '\n' {
			start++
		}
		start++ // Skip newline
		end := strings.Index(text[start:], "```")
		if end != -1 {
			extracted := strings.TrimSpace(text[start : start+end])
			if strings.HasPrefix(extracted, "{") {
				return extracted
			}
		}
	}

	// Try to find first '{' and extract from there
	firstBrace := strings.Index(text, "{")
	if firstBrace != -1 {
		return text[firstBrace:]
	}

	return text
}

// attemptJSONRepair attempts to repair truncated JSON by closing open structures
func attemptJSONRepair(text string) string {
	text = strings.TrimSpace(text)

	// Count open/close brackets and braces (excluding those in strings)
	openBraces := 0
	closeBraces := 0
	openBrackets := 0
	closeBrackets := 0
	inString := false
	escapeNext := false

	for _, char := range text {
		if escapeNext {
			escapeNext = false
			continue
		}
		if char == '\\' {
			escapeNext = true
			continue
		}
		if char == '"' {
			inString = !inString
			continue
		}
		if !inString {
			switch char {
			case '{':
				openBraces++
			case '}':
				closeBraces++
			case '[':
				openBrackets++
			case ']':
				closeBrackets++
			}
		}
	}

	// If already balanced, return as-is
	if openBraces == closeBraces && openBrackets == closeBrackets {
		return text
	}

	fmt.Printf("🔧 JSON repair needed: braces=%d/%d, brackets=%d/%d\n",
		openBraces, closeBraces, openBrackets, closeBrackets)

	// Try to fix truncated strings (remove incomplete last item in array)
	if openBrackets > closeBrackets {
		// Find last complete item in array
		lastCommaPos := strings.LastIndex(text, ",")
		if lastCommaPos > 0 {
			// Truncate at last comma
			text = text[:lastCommaPos]
		}
	}

	// Close any open quotes
	quoteCount := 0
	escapeNext = false
	for _, char := range text {
		if escapeNext {
			escapeNext = false
			continue
		}
		if char == '\\' {
			escapeNext = true
			continue
		}
		if char == '"' {
			quoteCount++
		}
	}
	if quoteCount%2 != 0 {
		text += "\""
	}

	// Close brackets/braces
	for i := 0; i < openBrackets-closeBrackets; i++ {
		text += "]"
	}
	for i := 0; i < openBraces-closeBraces; i++ {
		text += "}"
	}

	return text
}
