// backend/internal/services/serp.go
package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	g "github.com/serpapi/google-search-results-golang"

	"mylittleprice/internal/config"
	"mylittleprice/internal/domain"
	"mylittleprice/internal/models"
	"mylittleprice/internal/utils"
)

type SerpService struct {
	keyRotator *utils.KeyRotator
	config     *config.Config
}

type SearchResult struct {
	Products        []domain.ShoppingItem
	RelevanceScore  float32
	IsRelevant      bool
	AlternativeHint string
}

func NewSerpService(keyRotator *utils.KeyRotator, cfg *config.Config) *SerpService {
	return &SerpService{
		keyRotator: keyRotator,
		config:     cfg,
	}
}

func (s *SerpService) SearchProducts(ctx context.Context, query, searchType, country string, minPrice, maxPrice *float64) ([]models.ProductCard, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Validate input
	if err := validateSearchQuery(query); err != nil {
		return nil, -1, fmt.Errorf("invalid search query: %w", err)
	}

	// Try up to total number of keys + 2 (for network retries)
	maxRetries := s.keyRotator.GetTotalKeys() + 1
	var lastErr error
	var lastKeyIndex int = -1
	var lastWasQuotaError bool = false

	// Log initial search request
	searchLogAttrs := []any{
		slog.String("query", query),
		slog.String("search_type", searchType),
		slog.String("country", country),
		slog.String("language", getLanguageForCountry(country)),
	}
	if minPrice != nil {
		searchLogAttrs = append(searchLogAttrs, slog.Float64("min_price", *minPrice))
	}
	if maxPrice != nil {
		searchLogAttrs = append(searchLogAttrs, slog.Float64("max_price", *maxPrice))
	}
	utils.LogInfo(ctx, "🔍 SERP search initiated", searchLogAttrs...)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Only apply backoff for network errors, not quota errors
		if attempt > 0 && !lastWasQuotaError {
			// Exponential backoff: 500ms, 1s, 2s
			backoffDuration := time.Duration(500*(1<<uint(attempt-1))) * time.Millisecond
			if backoffDuration > 2*time.Second {
				backoffDuration = 2 * time.Second
			}
			utils.LogInfo(ctx, "⏳ SERP retry with backoff",
				slog.Int("attempt", attempt+1),
				slog.Int("max_attempts", maxRetries+1),
				slog.Duration("backoff", backoffDuration),
			)
			time.Sleep(backoffDuration)
		} else if attempt > 0 && lastWasQuotaError {
			utils.LogInfo(ctx, "🔄 SERP retry with next key",
				slog.Int("attempt", attempt+1),
				slog.Int("max_attempts", maxRetries+1),
			)
		}

		apiKey, keyIndex, err := s.keyRotator.GetNextKey()
		if err != nil {
			return nil, -1, fmt.Errorf("failed to get API key: %w", err)
		}
		lastKeyIndex = keyIndex
		lastWasQuotaError = false

		if attempt > 0 {
			utils.LogInfo(ctx, "Using API key", slog.Int("key_index", keyIndex))
		}

		parameter := map[string]string{
			"engine": "google_shopping",
			"q":      query,
			"gl":     country,
			"hl":     getLanguageForCountry(country),
		}

		// Add price range filters if provided
		if minPrice != nil {
			parameter["min_price"] = fmt.Sprintf("%.0f", *minPrice)
		}
		if maxPrice != nil {
			parameter["max_price"] = fmt.Sprintf("%.0f", *maxPrice)
		}

		search := g.NewGoogleSearch(parameter, apiKey)

		startTime := time.Now()
		data, err := search.GetJSON()
		elapsed := time.Since(startTime)

		if err != nil {
			lastErr = err
			utils.LogError(ctx, "❌ SERP API error", err,
				slog.Float64("duration_seconds", elapsed.Seconds()),
				slog.Int("attempt", attempt+1),
				slog.Int("max_attempts", maxRetries+1),
				slog.Int("key_index", keyIndex),
			)

			// Check if error is retryable
			errMsg := err.Error()
			isQuotaError := strings.Contains(errMsg, "run out of searches") ||
				strings.Contains(errMsg, "quota exceeded") ||
				strings.Contains(errMsg, "limit exceeded") ||
				strings.Contains(errMsg, "rate limit")

			isNetworkError := strings.Contains(errMsg, "timeout") ||
				strings.Contains(errMsg, "503") ||
				strings.Contains(errMsg, "502") ||
				strings.Contains(errMsg, "500")

			if isQuotaError {
				// Mark this key as exhausted
				utils.LogWarn(ctx, "⚠️ Quota error detected, marking key as exhausted", slog.Int("key_index", keyIndex))
				if markErr := s.keyRotator.MarkKeyAsExhausted(keyIndex); markErr != nil {
					utils.LogError(ctx, "Failed to mark key as exhausted", markErr, slog.Int("key_index", keyIndex))
				}
				// Try next key immediately (don't wait for backoff)
				lastWasQuotaError = true
				if attempt < maxRetries {
					continue
				}
			} else if isNetworkError {
				// Retryable network error - continue to next attempt
				utils.LogWarn(ctx, "Network error, retrying", slog.String("error_type", "network"))
				if attempt < maxRetries {
					continue
				}
			}

			// Non-retryable error or last attempt
			return nil, keyIndex, fmt.Errorf("SERP API error: %w", err)
		}

		// Success!
		if attempt > 0 {
			utils.LogInfo(ctx, "✅ SERP request succeeded on retry", slog.Int("attempt", attempt+1))
		}
		utils.LogInfo(ctx, "⏱️ SERP response received",
			slog.Float64("duration_seconds", elapsed.Seconds()),
			slog.Int("key_index", keyIndex),
		)

		shoppingItems := []domain.ShoppingItem{}

		if shoppingResults, ok := data["shopping_results"].([]interface{}); ok {
			utils.LogInfo(ctx, "📦 Raw SERP results received", slog.Int("product_count", len(shoppingResults)))

			for _, item := range shoppingResults {
				if itemMap, ok := item.(map[string]interface{}); ok {
					shoppingItem := domain.ShoppingItem{
						Position:    getIntFromInterface(itemMap["position"]),
						Title:       getStringFromInterface(itemMap["title"]),
						Link:        getStringFromInterface(itemMap["link"]),
						ProductLink: getStringFromInterface(itemMap["product_link"]),
						ProductID:   getStringFromInterface(itemMap["product_id"]),
						Thumbnail:   getStringFromInterface(itemMap["thumbnail"]),
						Price:       getStringFromInterface(itemMap["price"]),
						Merchant:    getStringFromInterface(itemMap["source"]),
						Rating:      getFloat32FromInterface(itemMap["rating"]),
						Reviews:     getIntFromInterface(itemMap["reviews"]),
						SerpAPILink: getStringFromInterface(itemMap["serpapi_product_api"]),
						PageToken:   getStringFromInterface(itemMap["immersive_product_page_token"]),
					}
					shoppingItems = append(shoppingItems, shoppingItem)
				}
			}
		} else {
			utils.LogWarn(ctx, "⚠️ No shopping_results in SERP response")
		}

		result := s.validateRelevance(query, shoppingItems, searchType)

		if !result.IsRelevant {
			utils.LogWarn(ctx, "⚠️ No relevant products found",
				slog.String("query", query),
				slog.Float64("relevance_score", float64(result.RelevanceScore)),
			)
			return nil, keyIndex, fmt.Errorf("no relevant products found")
		}

		cards := s.convertToProductCards(result.Products, searchType)

		// Log final results with product details
		productNames := make([]string, 0, min(3, len(cards)))
		for i := 0; i < min(3, len(cards)); i++ {
			productNames = append(productNames, cards[i].Name)
		}

		utils.LogInfo(ctx, "✅ SERP search completed successfully",
			slog.Int("product_count", len(cards)),
			slog.Float64("relevance_score", float64(result.RelevanceScore)),
			slog.Any("top_products", productNames),
		)

		return cards, keyIndex, nil
	}

	// All retries failed
	if lastErr != nil {
		return nil, lastKeyIndex, fmt.Errorf("SERP API failed after %d retries: %w", maxRetries+1, lastErr)
	}
	return nil, lastKeyIndex, fmt.Errorf("SERP API failed after %d retries", maxRetries+1)
}

func (s *SerpService) validateRelevance(query string, items []domain.ShoppingItem, searchType string) SearchResult {
	if len(items) == 0 {
		return SearchResult{
			Products:        []domain.ShoppingItem{},
			RelevanceScore:  0.0,
			IsRelevant:      false,
			AlternativeHint: "No products found",
		}
	}

	// ✅ НОВАЯ ЛОГИКА: Берем товары по позициям 1-10 от SerpAPI (уже отсортированы)
	// Вместо фильтрации по score, используем оригинальный порядок от Google Shopping

	maxProducts := 10 // Всегда берем до 10 товаров
	relevantProducts := []domain.ShoppingItem{}

	// Берем товары в оригинальном порядке (позиции 1-10)
	productCount := min(maxProducts, len(items))
	for i := 0; i < productCount; i++ {
		relevantProducts = append(relevantProducts, items[i])
	}

	// Считаем релевантным если есть хоть какие-то продукты
	isRelevant := len(relevantProducts) > 0

	result := SearchResult{
		Products:       relevantProducts,
		RelevanceScore: 1.0, // Не используется при новой логике
		IsRelevant:     isRelevant,
	}

	if !isRelevant && len(items) > 0 {
		result.AlternativeHint = fmt.Sprintf("Found similar products but exact match not available. Best alternative: %s", items[0].Title)
	}

	return result
}
func (s *SerpService) calculateRelevanceScore(queryWords []string, item domain.ShoppingItem) float32 {
	titleLower := strings.ToLower(item.Title)
	var score float32 = 0.0

	// ✅ 1. Полное совпадение всей фразы (бонус)
	queryStr := strings.Join(queryWords, " ")
	if strings.Contains(titleLower, queryStr) {
		score += float32(s.config.SerpScorePhraseMatch)
	}

	// ✅ 2. Все слова присутствуют (хороший сигнал)
	allWordsPresent := true
	for _, word := range queryWords {
		if len(word) <= s.config.SerpMinWordLength {
			continue // Игнорируем короткие слова
		}
		if !strings.Contains(titleLower, word) {
			allWordsPresent = false
			break
		}
	}
	if allWordsPresent {
		score += float32(s.config.SerpScoreAllWords)
	}

	// ✅ 3. Частичное совпадение слов (даже если не все слова есть)
	matchedWords := 0
	importantMatchedWords := 0
	for _, word := range queryWords {
		if len(word) <= 2 || isCommonWord(word) {
			continue
		}
		if strings.Contains(titleLower, word) {
			matchedWords++
			// Дополнительный бонус за важные слова (бренды, типы продуктов)
			if !isCommonWord(word) {
				importantMatchedWords++
			}
		}
	}

	significantWords := 0
	for _, word := range queryWords {
		if len(word) > s.config.SerpMinWordLength && !isCommonWord(word) {
			significantWords++
		}
	}

	if significantWords > 0 {
		matchRatio := float32(matchedWords) / float32(significantWords)
		score += matchRatio * float32(s.config.SerpScorePartialWords)
	}

	// ✅ 4. Порядок слов (менее важно)
	if len(queryWords) >= 2 {
		titleWords := strings.Fields(titleLower)
		orderScore := s.calculateWordOrderScore(queryWords, titleWords)
		score += orderScore * float32(s.config.SerpScoreWordOrderWeight)
	}

	// ✅ 5. Бренды (важно для точности)
	brands := []string{
		"apple", "iphone", "ipad", "macbook", "samsung", "galaxy",
		"google", "pixel", "xiaomi", "oneplus", "sony", "dell",
		"hp", "lenovo", "asus", "acer", "msi", "lg", "huawei",
		"nike", "adidas", "puma", "reebok", "under", "armour",
	}
	for _, brand := range brands {
		for _, word := range queryWords {
			if word == brand && strings.Contains(titleLower, brand) {
				score += float32(s.config.SerpScoreBrandMatch)
				break
			}
		}
	}

	// ✅ 6. Номера моделей (если есть в запросе, должны совпадать)
	modelNumbers := s.extractModelNumbers(queryWords)
	if len(modelNumbers) > 0 {
		hasModelMatch := false
		for _, modelNum := range modelNumbers {
			if strings.Contains(titleLower, modelNum) {
				hasModelMatch = true
				break
			}
		}
		if hasModelMatch {
			score += float32(s.config.SerpScoreModelMatch)
		}
		// Убираем штраф если модель не совпала - может быть похожий продукт
	}

	// ✅ 7. УБИРАЕМ ЖЕСТКИЙ ШТРАФ за лишние слова
	// Это слишком строго для гибкого поиска

	// Ограничиваем score в пределах [0, 1]
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score
}

func (s *SerpService) calculateWordOrderScore(queryWords, titleWords []string) float32 {
	if len(queryWords) < 2 || len(titleWords) < 2 {
		return 0
	}

	matches := 0
	for i := 0; i < len(queryWords)-1; i++ {
		word1 := queryWords[i]
		word2 := queryWords[i+1]

		pos1 := -1
		pos2 := -1

		for j, titleWord := range titleWords {
			if strings.Contains(titleWord, word1) {
				pos1 = j
			}
			if strings.Contains(titleWord, word2) {
				pos2 = j
			}
		}

		if pos1 != -1 && pos2 != -1 && pos1 < pos2 {
			matches++
		}
	}

	return float32(matches) / float32(len(queryWords)-1)
}

func (s *SerpService) extractModelNumbers(words []string) []string {
	modelNumbers := []string{}

	for _, word := range words {
		hasDigit := false
		for _, char := range word {
			if char >= '0' && char <= '9' {
				hasDigit = true
				break
			}
		}

		if hasDigit && len(word) >= s.config.SerpModelNumberMinLength {
			modelNumbers = append(modelNumbers, word)
		}
	}

	return modelNumbers
}

func isCommonWord(word string) bool {
	commonWords := []string{
		"the", "a", "an", "and", "or", "but", "in", "on", "at", "to", "for",
		"of", "with", "by", "from", "as", "is", "was", "are", "were", "be",
		"have", "has", "had", "do", "does", "did", "will", "would", "could",
		"should", "may", "might", "can", "new", "latest", "best", "pro", "air",
		"version", "model", "series", "generation", "gen",
	}

	for _, common := range commonWords {
		if word == common {
			return true
		}
	}

	return false
}

func (s *SerpService) GetProductDetailsByToken(ctx context.Context, pageToken string) (map[string]interface{}, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	maxRetries := s.keyRotator.GetTotalKeys() + 1
	var lastErr error
	var lastKeyIndex int = -1
	var lastWasQuotaError bool = false

	utils.LogInfo(ctx, "🔍 Fetching product details", slog.String("page_token", pageToken))

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 && !lastWasQuotaError {
			backoffDuration := time.Duration(500*(1<<uint(attempt-1))) * time.Millisecond
			if backoffDuration > 2*time.Second {
				backoffDuration = 2 * time.Second
			}
			utils.LogInfo(ctx, "⏳ Product details retry with backoff",
				slog.Int("attempt", attempt+1),
				slog.Duration("backoff", backoffDuration),
			)
			time.Sleep(backoffDuration)
		} else if attempt > 0 && lastWasQuotaError {
			utils.LogInfo(ctx, "🔄 Product details retry with next key",
				slog.Int("attempt", attempt+1),
			)
		}

		apiKey, keyIndex, err := s.keyRotator.GetNextKey()
		if err != nil {
			return nil, -1, fmt.Errorf("failed to get API key: %w", err)
		}
		lastKeyIndex = keyIndex
		lastWasQuotaError = false

		parameter := map[string]string{
			"engine":      "google_immersive_product",
			"page_token":  pageToken,
			"more_stores": "true",
		}

		search := g.NewGoogleSearch(parameter, apiKey)
		startTime := time.Now()
		data, err := search.GetJSON()
		elapsed := time.Since(startTime)

		if err != nil {
			lastErr = err
			utils.LogError(ctx, "❌ Product details API error", err,
				slog.Float64("duration_seconds", elapsed.Seconds()),
				slog.Int("attempt", attempt+1),
				slog.Int("key_index", keyIndex),
			)

			errMsg := err.Error()
			isQuotaError := strings.Contains(errMsg, "run out of searches") ||
				strings.Contains(errMsg, "quota exceeded") ||
				strings.Contains(errMsg, "limit exceeded") ||
				strings.Contains(errMsg, "rate limit")

			isNetworkError := strings.Contains(errMsg, "timeout") ||
				strings.Contains(errMsg, "503") ||
				strings.Contains(errMsg, "502") ||
				strings.Contains(errMsg, "500")

			if isQuotaError {
				utils.LogWarn(ctx, "⚠️ Quota error for product details", slog.Int("key_index", keyIndex))
				if markErr := s.keyRotator.MarkKeyAsExhausted(keyIndex); markErr != nil {
					utils.LogError(ctx, "Failed to mark key as exhausted", markErr)
				}
				lastWasQuotaError = true
				if attempt < maxRetries {
					continue
				}
			} else if isNetworkError {
				if attempt < maxRetries {
					continue
				}
			}

			return nil, keyIndex, fmt.Errorf("SERP API error: %w", err)
		}

		// Success!
		if attempt > 0 {
			utils.LogInfo(ctx, "✅ Product details request succeeded on retry", slog.Int("attempt", attempt+1))
		}
		utils.LogInfo(ctx, "✅ Product details fetched", slog.Float64("duration_seconds", elapsed.Seconds()))
		return data, keyIndex, nil
	}

	if lastErr != nil {
		return nil, lastKeyIndex, fmt.Errorf("product details failed after %d retries: %w", maxRetries+1, lastErr)
	}
	return nil, lastKeyIndex, fmt.Errorf("product details failed after %d retries", maxRetries+1)
}

func (s *SerpService) convertToProductCards(items []domain.ShoppingItem, searchType string) []models.ProductCard {
	// ✅ НОВАЯ ЛОГИКА: Всегда берем до 10 товаров независимо от типа поиска
	maxProducts := 10
	cards := make([]models.ProductCard, 0, maxProducts)

	for i, item := range items {
		if i >= maxProducts {
			break
		}

		pageToken := item.PageToken
		if pageToken == "" {
			pageToken = s.extractPageToken(item)
		}

		badge := ""
		if item.Rating > 0 {
			badge = fmt.Sprintf("⭐ %.1f", item.Rating)
		}

		card := models.ProductCard{
			Name:        item.Title,
			Price:       item.Price,
			OldPrice:    item.OldPrice,
			Link:        item.ProductLink,
			Image:       item.Thumbnail,
			Description: item.Merchant,
			Badge:       badge,
			PageToken:   pageToken,
		}

		cards = append(cards, card)
	}

	return cards
}

func (s *SerpService) extractPageToken(item domain.ShoppingItem) string {
	if item.PageToken != "" {
		return item.PageToken
	}

	if item.SerpAPILink != "" {
		return extractTokenFromSerpAPILink(item.SerpAPILink)
	}

	if item.ProductID != "" {
		return item.ProductID
	}

	return ""
}

func extractTokenFromSerpAPILink(link string) string {
	tokenStart := findSubstring(link, "page_token=")
	if tokenStart == -1 {
		return ""
	}

	tokenStart += len("page_token=")
	tokenEnd := findSubstring(link[tokenStart:], "&")
	if tokenEnd == -1 {
		return link[tokenStart:]
	}

	return link[tokenStart : tokenStart+tokenEnd]
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func getLanguageForCountry(country string) string {
	languageMap := map[string]string{
		"CH": "de", "DE": "de", "AT": "de",
		"FR": "fr", "IT": "it", "ES": "es",
		"PT": "pt", "NL": "nl", "BE": "nl",
		"PL": "pl", "CZ": "cs", "SE": "sv",
		"NO": "no", "DK": "da", "FI": "fi",
		"GB": "en", "US": "en",
	}

	if lang, ok := languageMap[country]; ok {
		return lang
	}
	return "en"
}

func (s *SerpService) GetProductByPageToken(ctx context.Context, pageToken string) (map[string]interface{}, int, error) {
	return s.GetProductDetailsByToken(ctx, pageToken)
}

func (s *SerpService) SearchWithCache(ctx context.Context, query, searchType, country string, minPrice, maxPrice *float64, cacheService *CacheService) ([]models.ProductCard, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Build cache key including price range
	cacheKey := fmt.Sprintf("search:%s:%s:%s", country, searchType, query)
	if minPrice != nil {
		cacheKey += fmt.Sprintf(":min%.0f", *minPrice)
	}
	if maxPrice != nil {
		cacheKey += fmt.Sprintf(":max%.0f", *maxPrice)
	}

	if cacheService != nil {
		if cached, err := cacheService.GetSearchResults(cacheKey); err == nil && cached != nil {
			utils.LogInfo(ctx, "📦 Using cached SERP results", slog.String("cache_key", cacheKey))
			return cached, -1, nil
		}
	}

	cards, keyIndex, err := s.SearchProducts(ctx, query, searchType, country, minPrice, maxPrice)
	if err != nil {
		return nil, keyIndex, err
	}

	if cacheService != nil {
		ttl := time.Duration(s.config.CacheSerpTTL) * time.Second
		_ = cacheService.SetSearchResults(cacheKey, cards, ttl)
	}

	return cards, keyIndex, nil
}

func getStringFromInterface(val interface{}) string {
	if str, ok := val.(string); ok {
		return str
	}
	return ""
}

func getIntFromInterface(val interface{}) int {
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func getFloat32FromInterface(val interface{}) float32 {
	switch v := val.(type) {
	case float32:
		return v
	case float64:
		return float32(v)
	case int:
		return float32(v)
	default:
		return 0
	}
}
