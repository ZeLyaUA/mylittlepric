package services

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"google.golang.org/genai"

	"mylittleprice/internal/models"
)

// ContextExtractorService extracts structured information from conversation history
// Uses AI to intelligently identify user preferences, requirements, and context
type ContextExtractorService struct {
	client    *genai.Client
	ctx       context.Context
	modelName string
}

// NewContextExtractorService creates a new context extractor
func NewContextExtractorService(client *genai.Client, modelName string) *ContextExtractorService {
	return &ContextExtractorService{
		client:    client,
		ctx:       context.Background(),
		modelName: modelName,
	}
}

// ExtractUserPreferences analyzes conversation and extracts structured preferences
func (c *ContextExtractorService) ExtractUserPreferences(
	messages []models.CycleMessage,
	currentPreferences *models.ConversationPreferences,
	currency string,
) (*models.ConversationPreferences, error) {

	if len(messages) == 0 {
		return currentPreferences, nil
	}

	// Build conversation text from recent messages
	conversationText := c.buildConversationText(messages, 8) // Last 8 messages

	// Build current preferences JSON
	currentPrefJSON := "{}"
	if currentPreferences != nil {
		if data, err := json.Marshal(currentPreferences); err == nil {
			currentPrefJSON = string(data)
		}
	}

	prompt := fmt.Sprintf(`Analyze this shopping conversation and extract user preferences in JSON format.

Conversation:
%s

Current preferences: %s

Extract and return ONLY a JSON object with these fields (omit fields if not mentioned):
{
  "price_range": {"min": 30000, "max": 50000, "currency": "%s"},
  "brands": ["Apple", "Samsung"],
  "features": ["256GB storage", "OLED screen", "5G"],
  "requirements": ["2-year warranty", "fast delivery"]
}

Rules:
- Only include information explicitly mentioned by user
- Merge with current preferences (don't overwrite unless user changed preference)
- Extract price range in %s currency AS-IS (prices are already reduced by 30%% in conversation)
- Keep features and requirements concise
- Return ONLY valid JSON, no explanations`, conversationText, currentPrefJSON, currency, currency)

	// Use fast model for extraction (token efficiency)
	temp := float32(0.2) // Low temperature for more deterministic extraction
	resp, err := c.client.Models.GenerateContent(
		c.ctx,
		c.modelName, // Fast model for extraction
		genai.Text(prompt),
		&genai.GenerateContentConfig{
			Temperature:      &temp,
			ResponseMIMEType: "application/json",
			MaxOutputTokens:  500, // Small response
		},
	)

	if err != nil {
		fmt.Printf("⚠️ Failed to extract preferences: %v\n", err)
		return currentPreferences, err
	}

	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return currentPreferences, fmt.Errorf("empty response from preference extraction")
	}

	// Extract text from response
	responseText := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.Text != "" {
			responseText += part.Text
		}
	}

	responseText = strings.TrimSpace(responseText)

	// Parse JSON response
	var extracted models.ConversationPreferences
	if err := json.Unmarshal([]byte(responseText), &extracted); err != nil {
		fmt.Printf("⚠️ Failed to parse extracted preferences: %v\nResponse: %s\n", err, responseText)
		return currentPreferences, err
	}

	fmt.Printf("✅ Extracted preferences: brands=%v, features=%v, price_range=%v\n",
		extracted.Brands, extracted.Features, extracted.PriceRange)

	return &extracted, nil
}

// GenerateConversationSummary creates a compact summary of the conversation
func (c *ContextExtractorService) GenerateConversationSummary(
	messages []models.CycleMessage,
	previousSummary string,
	language string,
) (string, error) {

	if len(messages) == 0 {
		return previousSummary, nil
	}

	// Use only recent messages for summarization (last 6-8 messages)
	recentMessages := messages
	if len(messages) > 8 {
		recentMessages = messages[len(messages)-8:]
	}

	conversationText := c.buildConversationText(recentMessages, 8)

	previousSummaryText := "No previous summary"
	if previousSummary != "" {
		previousSummaryText = previousSummary
	}

	prompt := fmt.Sprintf(`Create a concise summary (2-3 sentences) of this shopping conversation.

Focus on:
- What product the user is looking for
- Key requirements and preferences mentioned
- Current status (still searching, narrowing down, found options, etc.)

Previous summary: %s

Recent conversation:
%s

Return a clear, concise summary in %s language. Maximum 3 sentences.`, previousSummaryText, conversationText, language)

	temp := float32(0.3) // Low temperature for consistent summaries
	resp, err := c.client.Models.GenerateContent(
		c.ctx,
		c.modelName,
		genai.Text(prompt),
		&genai.GenerateContentConfig{
			Temperature:     &temp,
			MaxOutputTokens: 200, // Short summary
		},
	)

	if err != nil {
		fmt.Printf("⚠️ Failed to generate summary: %v\n", err)
		return previousSummary, err
	}

	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return previousSummary, fmt.Errorf("empty response from summary generation")
	}

	// Extract summary text
	summary := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.Text != "" {
			summary += part.Text
		}
	}

	summary = strings.TrimSpace(summary)

	if summary == "" {
		return previousSummary, nil
	}

	fmt.Printf("📝 Generated summary: %s\n", summary)

	return summary, nil
}

// ExtractExclusions identifies things the user explicitly doesn't want
func (c *ContextExtractorService) ExtractExclusions(
	messages []models.CycleMessage,
) []string {

	exclusions := []string{}

	// Simple keyword-based extraction for now
	// Can be enhanced with AI if needed
	excludeKeywords := map[string][]string{
		"brand":       {"не хочу", "don't want", "не потрібно", "not interested", "exclude"},
		"chinese":     {"китайск", "chinese", "китайськ"},
		"refurbished": {"б/у", "refurbished", "used", "вживан"},
		"cheap":       {"дешев", "cheap", "низкого качества", "low quality"},
	}

	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}

		contentLower := strings.ToLower(msg.Content)

		for category, keywords := range excludeKeywords {
			matched := false
			for _, kw := range keywords {
				if strings.Contains(contentLower, kw) {
					matched = true
					break
				}
			}

			if matched && !slices.Contains(exclusions, category) {
				exclusions = append(exclusions, category)
			}
		}
	}

	return exclusions
}

// UpdateConversationContext updates the full conversation context
func (c *ContextExtractorService) UpdateConversationContext(
	session *models.ChatSession,
	newMessages []models.CycleMessage,
) error {

	// Initialize if doesn't exist
	if session.ConversationContext == nil {
		session.ConversationContext = &models.ConversationContext{
			Summary:     "",
			Preferences: models.ConversationPreferences{},
			Exclusions:  []string{},
			UpdatedAt:   time.Now(),
		}
	}

	ctx := session.ConversationContext

	// Extract preferences
	preferences, err := c.ExtractUserPreferences(
		newMessages,
		&ctx.Preferences,
		session.Currency,
	)
	if err == nil {
		ctx.Preferences = *preferences
	}

	// Generate summary
	summary, err := c.GenerateConversationSummary(
		newMessages,
		ctx.Summary,
		session.LanguageCode,
	)
	if err == nil {
		ctx.Summary = summary
	}

	// Extract exclusions
	exclusions := c.ExtractExclusions(newMessages)
	if len(exclusions) > 0 {
		// Merge with existing exclusions
		for _, excl := range exclusions {
			if !slices.Contains(ctx.Exclusions, excl) {
				ctx.Exclusions = append(ctx.Exclusions, excl)
			}
		}
	}

	ctx.UpdatedAt = time.Now()

	fmt.Printf("🧠 Context updated: summary_len=%d, brands=%d, features=%d, exclusions=%d\n",
		len(ctx.Summary), len(ctx.Preferences.Brands), len(ctx.Preferences.Features), len(ctx.Exclusions))

	return nil
}

// UpdateLastSearch updates the last search context
func (c *ContextExtractorService) UpdateLastSearch(
	session *models.ChatSession,
	query string,
	category string,
	products []models.ProductInfo,
	userFeedback string,
) {

	if session.ConversationContext == nil {
		session.ConversationContext = &models.ConversationContext{
			UpdatedAt: time.Now(),
		}
	}

	session.ConversationContext.LastSearch = &models.SearchContext{
		Query:         query,
		Category:      category,
		ProductsShown: products,
		UserFeedback:  userFeedback,
		Timestamp:     time.Now(),
	}

	fmt.Printf("🔍 Last search updated: query='%s', category='%s', products=%d\n",
		query, category, len(products))
}

// Helper functions

func (c *ContextExtractorService) buildConversationText(messages []models.CycleMessage, maxMessages int) string {
	var sb strings.Builder

	startIdx := 0
	if len(messages) > maxMessages {
		startIdx = len(messages) - maxMessages
	}

	for i := startIdx; i < len(messages); i++ {
		msg := messages[i]
		sb.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
	}

	return sb.String()
}
