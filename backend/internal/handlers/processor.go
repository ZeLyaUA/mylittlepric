package handlers

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"mylittleprice/internal/container"
	"mylittleprice/internal/models"
	"mylittleprice/internal/services"
	"mylittleprice/internal/utils"
)

// ChatProcessor handles the core chat processing logic shared between REST and WebSocket handlers
type ChatProcessor struct {
	container *container.Container
}

// NewChatProcessor creates a new chat processor
func NewChatProcessor(c *container.Container) *ChatProcessor {
	return &ChatProcessor{
		container: c,
	}
}

// ChatRequest represents a standardized chat request
type ChatRequest struct {
	SessionID         string
	UserID            *uuid.UUID // Optional user ID for authenticated users
	Message           string
	Country           string
	Language          string
	Currency          string
	NewSearch         bool
	CurrentCategory   string
	BrowserID         string // Persistent browser identifier for anonymous tracking
	UserMessageID     string // Pre-generated UUID for user message (for consistent sync)
	AssistantMessageID string // Pre-generated UUID for assistant message (for consistent sync)
}

// ChatProcessorResponse represents the standardized response from chat processing
type ChatProcessorResponse struct {
	Type               string
	Output             string
	QuickReplies       []string
	Products           []models.ProductCard
	ProductDescription string // AI-generated description about the products
	SearchType         string
	SessionID          string
	MessageCount       int
	SearchState        *models.SearchStateResponse
	Error              *ErrorInfo
}

// ErrorInfo contains error details
type ErrorInfo struct {
	Code    string
	Message string
}

// ProcessChat handles the main chat processing logic
func (p *ChatProcessor) ProcessChat(req *ChatRequest) *ChatProcessorResponse {
	// Create context with timeout for the entire operation
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Track metrics for message processing
	start := time.Now()
	status := "success"
	var response *ChatProcessorResponse

	defer func() {
		// Record processing duration
		duration := time.Since(start).Seconds()

		// Determine status based on response
		if response != nil && response.Error != nil {
			status = "error"
		}

		utils.LogInfo(ctx, "message processing completed",
			slog.String("session_id", req.SessionID),
			slog.String("status", status),
			slog.Float64("duration_seconds", duration),
		)
	}()

	// Validate input
	if req.Message == "" {
		response = &ChatProcessorResponse{
			Error: &ErrorInfo{
				Code:    "validation_error",
				Message: "Message is required",
			},
		}
		return response
	}

	// Set defaults
	if req.Country == "" {
		req.Country = p.container.Config.DefaultCountry
	}
	if req.Language == "" {
		req.Language = p.container.Config.DefaultLanguage
	}
	if req.Currency == "" {
		req.Currency = p.container.Config.DefaultCurrency
	}

	// Get or create session
	session, err := p.getOrCreateSession(req)
	if err != nil {
		response = &ChatProcessorResponse{
			Error: &ErrorInfo{
				Code:    "session_error",
				Message: "Failed to create session",
			},
		}
		return response
	}

	// Handle new search
	if req.NewSearch {
		utils.LogInfo(ctx, "new search started", slog.String("session_id", req.SessionID))
		p.container.SessionService.StartNewSearchInMemory(session)
	}

	// Handle category update
	if req.CurrentCategory != "" && req.CurrentCategory != session.SearchState.Category {
		session.SearchState.Category = req.CurrentCategory
		// Will be saved at the end with SaveSession()
	}

	// Check anonymous search limit (browser-based tracking)
	anonymousLimit := p.container.Config.AnonymousSearchLimit
	var anonymousSearchUsed int
	if req.UserID == nil && req.BrowserID != "" {
		// Get search count from Redis by browser ID
		count, err := p.container.CacheService.GetAnonymousSearchCount(req.BrowserID)
		if err != nil {
			utils.LogError(ctx, "failed to get anonymous search count", err, slog.String("browser_id", req.BrowserID))
			count = 0 // Continue on error, don't block user
		}
		anonymousSearchUsed = count

		// Check if limit reached
		if anonymousSearchUsed >= anonymousLimit {
			response = &ChatProcessorResponse{
				Type:         "text",
				Output:       "You've used all 3 free searches! Please sign up or log in to continue searching for products.",
				SessionID:    req.SessionID,
				MessageCount: session.MessageCount,
				SearchState: &models.SearchStateResponse{
					Status:                 string(session.SearchState.Status),
					Category:               session.SearchState.Category,
					CanContinue:            false,
					SearchCount:            session.SearchState.SearchCount,
					MaxSearches:            p.container.SessionService.GetMaxSearches(),
					AnonymousSearchUsed:    anonymousSearchUsed,
					AnonymousSearchLimit:   anonymousLimit,
					RequiresAuthentication: true,
					Message:                "Anonymous search limit reached - authentication required",
				},
			}
			return response
		}
	}

	// Check search limit
	if session.SearchState.SearchCount >= p.container.SessionService.GetMaxSearches() {
		response = &ChatProcessorResponse{
			Type:         "text",
			Output:       "You have reached the maximum number of searches. Please start a new search.",
			SessionID:    req.SessionID,
			MessageCount: session.MessageCount,
			SearchState: &models.SearchStateResponse{
				Status:                 string(session.SearchState.Status),
				Category:               session.SearchState.Category,
				CanContinue:            false,
				SearchCount:            session.SearchState.SearchCount,
				MaxSearches:            p.container.SessionService.GetMaxSearches(),
				AnonymousSearchUsed:    anonymousSearchUsed,
				AnonymousSearchLimit:   anonymousLimit,
				RequiresAuthentication: false,
				Message:                "Search limit reached",
			},
		}
		return response
	}

	// Store user message
	// Use pre-generated ID if provided, otherwise generate new one
	var userMsgID uuid.UUID
	if req.UserMessageID != "" {
		parsedID, err := uuid.Parse(req.UserMessageID)
		if err != nil {
			userMsgID = uuid.New()
		} else {
			userMsgID = parsedID
		}
	} else {
		userMsgID = uuid.New()
	}

	userMessage := &models.Message{
		ID:        userMsgID,
		SessionID: session.ID,
		Role:      "user",
		Content:   req.Message,
		CreatedAt: time.Now(),
	}

	if err := p.container.MessageService.AddMessageInMemory(session, userMessage); err != nil {
		response = &ChatProcessorResponse{
			Error: &ErrorInfo{
				Code:    "storage_error",
				Message: "Failed to store message",
			},
		}
		return response
	}

	p.container.MessageService.IncrementMessageCountInMemory(session)

	// Add user message to cycle history
	p.container.CycleService.AddToCycleHistoryInMemory(session, "user", req.Message)

	// Process with Universal Prompt System with retry logic
	var geminiResponse *models.GeminiResponse
	var geminiErr error
	const maxProcessingRetries = 2

	for attempt := 0; attempt <= maxProcessingRetries; attempt++ {
		if attempt > 0 {
			utils.LogInfo(ctx, "retry processing attempt",
				slog.Int("attempt", attempt+1),
				slog.Int("max_attempts", maxProcessingRetries+1),
			)
		}

		geminiResponse, geminiErr = p.container.GeminiService.ProcessWithUniversalPrompt(
			req.Message,
			session,
		)

		// Success - break out of retry loop
		if geminiErr == nil && geminiResponse != nil {
			if attempt > 0 {
				utils.LogInfo(ctx, "processing succeeded on retry",
					slog.Int("attempt", attempt+1),
				)
			}
			break
		}

		// Log the error
		if geminiErr != nil {
			utils.LogError(ctx, "gemini processing error", geminiErr,
				slog.Int("attempt", attempt+1),
				slog.Int("max_attempts", maxProcessingRetries+1),
			)
		}

		// If this is the last attempt, use fallback response
		if attempt == maxProcessingRetries {
			utils.LogWarn(ctx, "all processing attempts failed, using fallback response")

			// Return helpful fallback response instead of error
			response = &ChatProcessorResponse{
				Type:         "dialogue",
				Output:       "I'm having trouble processing your request right now. Could you please rephrase your question or try again in a moment?",
				QuickReplies: []string{"Start over", "Try again"},
				SessionID:    req.SessionID,
				MessageCount: session.MessageCount,
				SearchState: &models.SearchStateResponse{
					Status:      string(session.SearchState.Status),
					Category:    session.SearchState.Category,
					CanContinue: session.SearchState.SearchCount < p.container.SessionService.GetMaxSearches(),
					SearchCount: session.SearchState.SearchCount,
					MaxSearches: p.container.SessionService.GetMaxSearches(),
					Message:     "Temporary processing issue",
				},
			}
			return response
		}

		// Wait a bit before retry (500ms, 1s)
		if attempt < maxProcessingRetries {
			retryDelay := time.Duration(500*(attempt+1)) * time.Millisecond
			time.Sleep(retryDelay)
		}
	}

	// Log the Gemini response for debugging
	logAttrs := []any{
		slog.String("response_type", geminiResponse.ResponseType),
		slog.String("category", geminiResponse.Category),
		slog.String("search_phrase", geminiResponse.SearchPhrase),
	}
	if geminiResponse.MinPrice != nil {
		logAttrs = append(logAttrs, slog.Any("min_price", geminiResponse.MinPrice))
	}
	if geminiResponse.MaxPrice != nil {
		logAttrs = append(logAttrs, slog.Any("max_price", geminiResponse.MaxPrice))
	}
	utils.LogInfo(ctx, "gemini response received", logAttrs...)

	// Update category
	if geminiResponse.Category != "" {
		session.SearchState.Category = geminiResponse.Category
	}

	// Create assistant message (but don't save yet - we may need to add products first)
	// Use pre-generated ID if provided, otherwise generate new one
	var assistantMsgID uuid.UUID
	if req.AssistantMessageID != "" {
		parsedID, err := uuid.Parse(req.AssistantMessageID)
		if err != nil {
			assistantMsgID = uuid.New()
		} else {
			assistantMsgID = parsedID
		}
	} else {
		assistantMsgID = uuid.New()
	}

	assistantMessage := &models.Message{
		ID:           assistantMsgID,
		SessionID:    session.ID,
		Role:         "assistant",
		Content:      geminiResponse.Output,
		ResponseType: geminiResponse.ResponseType,
		QuickReplies: geminiResponse.QuickReplies,
		CreatedAt:    time.Now(),
	}

	// Build response
	response = &ChatProcessorResponse{
		Type:         geminiResponse.ResponseType,
		Output:       geminiResponse.Output,
		QuickReplies: geminiResponse.QuickReplies,
		SessionID:    req.SessionID,
		MessageCount: session.MessageCount + 1,
	}

	// Handle search (intermediate search for verification/grounding)
	if geminiResponse.ResponseType == "search" {
		searchLogAttrs := []any{
			slog.String("phrase", geminiResponse.SearchPhrase),
			slog.String("search_type", geminiResponse.SearchType),
		}
		if geminiResponse.MinPrice != nil {
			searchLogAttrs = append(searchLogAttrs, slog.Any("min_price", geminiResponse.MinPrice))
		}
		if geminiResponse.MaxPrice != nil {
			searchLogAttrs = append(searchLogAttrs, slog.Any("max_price", geminiResponse.MaxPrice))
		}
		utils.LogInfo(ctx, "search request detected", searchLogAttrs...)

		// Validate search phrase
		if geminiResponse.SearchPhrase == "" {
			utils.LogWarn(ctx, "empty search phrase in search request")
			response.Output = "I need more details about what product you're looking for. Could you be more specific?"
			response.Type = "dialogue"
		} else {
			products, translatedQuery, searchErr := p.performSearch(geminiResponse, req.Country, req.Language)
			if searchErr != nil {
				utils.LogWarn(ctx, "search failed", slog.Any("error", searchErr))
				response.Output = "Sorry, I couldn't find any products. Please try different keywords."
				response.Type = "text"
			} else if len(products) > 0 {
				response.Products = products
				response.ProductDescription = geminiResponse.ProductDescription // AI-generated description about products
				response.SearchType = geminiResponse.SearchType

				// Update last product
				if len(products) > 0 {
					price := parsePrice(products[0].Price)
					session.SearchState.LastProduct = &models.ProductInfo{
						Name:  products[0].Name,
						Price: price,
					}
				}

				session.SearchState.SearchCount++
				// Track anonymous search usage in Redis by browser ID
				if req.UserID == nil && req.BrowserID != "" {
					if err := p.container.CacheService.IncrementAnonymousSearchCount(req.BrowserID); err != nil {
						utils.LogError(ctx, "failed to increment anonymous search count", err, slog.String("browser_id", req.BrowserID))
					}
				}
				// Add products to assistant message BEFORE saving
				assistantMessage.Products = products

				// NEW: Update last search in conversation context
				productInfoList := make([]models.ProductInfo, 0, len(products))
				for _, p := range products {
					price := parsePrice(p.Price)
					productInfoList = append(productInfoList, models.ProductInfo{
						Name:  p.Name,
						Price: price,
					})
				}
				contextExtractor := p.container.GeminiService.GetContextExtractor()
				contextExtractor.UpdateLastSearch(session, translatedQuery, geminiResponse.Category, productInfoList, "")

				// Save search history
				p.saveSearchHistory(req, session, geminiResponse, translatedQuery, products)
			}
		}
	}

	// Handle api_request (final product search to complete cycle)
	if geminiResponse.ResponseType == "api_request" {
		utils.LogInfo(ctx, "API request detected",
			slog.String("api", geminiResponse.API),
			slog.Any("params", geminiResponse.Params),
		)

		if geminiResponse.API == "google_shopping" && geminiResponse.Params != nil {
			// Extract query from params
			query, ok := geminiResponse.Params["q"].(string)
			if !ok || query == "" {
				utils.LogWarn(ctx, "missing or invalid 'q' parameter in api_request")
				response.Output = "I need more details about what product you're looking for. Could you be more specific?"
				response.Type = "dialogue"
			} else {
				// Perform the final search
				utils.LogInfo(ctx, "final product search", slog.String("query", query))

				// Create a temporary GeminiResponse for search
				searchResp := &models.GeminiResponse{
					SearchPhrase: query,
					SearchType:   "exact", // Default to exact for final searches
					Category:     geminiResponse.Category,
					PriceFilter:  geminiResponse.PriceFilter,
				}

				products, translatedQuery, searchErr := p.performSearch(searchResp, req.Country, req.Language)
				if searchErr != nil {
					utils.LogWarn(ctx, "final search failed", slog.Any("error", searchErr))
					response.Output = "Sorry, I couldn't find any products. Please try different keywords."
					response.Type = "text"
				} else if len(products) > 0 {
					response.Products = products
					response.ProductDescription = geminiResponse.ProductDescription // AI-generated description about products
					response.SearchType = "exact"
					response.Output = geminiResponse.Output // Use AI's message if provided

					// Update last product
					if len(products) > 0 {
						price := parsePrice(products[0].Price)
						session.SearchState.LastProduct = &models.ProductInfo{
							Name:  products[0].Name,
							Price: price,
						}
					}

					session.SearchState.SearchCount++
					// Track anonymous search usage in Redis by browser ID
					if req.UserID == nil && req.BrowserID != "" {
						if err := p.container.CacheService.IncrementAnonymousSearchCount(req.BrowserID); err != nil {
							utils.LogError(ctx, "failed to increment anonymous search count", err, slog.String("browser_id", req.BrowserID))
						}
					}
					// Add products to assistant message BEFORE saving
					assistantMessage.Products = products

					// NEW: Update last search in conversation context
					productInfoList := make([]models.ProductInfo, 0, len(products))
					for _, p := range products {
						price := parsePrice(p.Price)
						productInfoList = append(productInfoList, models.ProductInfo{
							Name:  p.Name,
							Price: price,
						})
					}
					contextExtractor := p.container.GeminiService.GetContextExtractor()
					contextExtractor.UpdateLastSearch(session, translatedQuery, searchResp.Category, productInfoList, "")

					// Save search history
					p.saveSearchHistory(req, session, searchResp, translatedQuery, products)

					utils.LogInfo(ctx, "cycle completed", slog.Int("product_count", len(products)))
				} else {
					response.Output = "I couldn't find that exact product. Would you like to see similar alternatives?"
					response.Type = "dialogue"
				}
			}
		} else {
			utils.LogWarn(ctx, "unsupported API", slog.String("api", geminiResponse.API))
			response.Output = "I encountered an error processing your request. Please try again."
			response.Type = "dialogue"
		}
	}

	// IMPORTANT: Sync assistant message content with final response output
	// response.Output may have been modified after assistantMessage was created
	// (e.g., in error handling, empty search results, etc.)
	if response.Output != "" {
		assistantMessage.Content = response.Output
	} else if assistantMessage.Content == "" {
		// Fallback if both are empty - this should not happen, but prevents validation errors
		assistantMessage.Content = "..." // Minimal valid content
		response.Output = "..."
		utils.LogWarn(ctx, "both response.Output and assistantMessage.Content are empty - using fallback")
	}

	// Save assistant message (now with products if it was a search)
	if err := p.container.MessageService.AddMessageInMemory(session, assistantMessage); err != nil {
		utils.LogWarn(ctx, "failed to store assistant message (non-critical)", slog.Any("error", err))
		// This is not critical - the session will still be saved with other state
	}

	// Add assistant response to cycle history
	p.container.CycleService.AddToCycleHistoryInMemory(session, "assistant", geminiResponse.Output)

	// NEW: Update conversation context periodically
	contextExtractor := p.container.GeminiService.GetContextExtractor()
	contextOptimizer := p.container.GeminiService.GetContextOptimizer()

	if contextOptimizer.ShouldUpdateContext(session) {
		utils.LogInfo(ctx, "updating conversation context")
		if err := contextExtractor.UpdateConversationContext(session, session.CycleState.CycleHistory); err != nil {
			utils.LogWarn(ctx, "failed to update conversation context (non-critical)", slog.Any("error", err))
			// This is not critical - conversation will continue with existing context
		} else {
			utils.LogInfo(ctx, "conversation context updated successfully")
		}
		// Context is updated in-memory, will be saved at the end
	}

	// Check if we need to start a new cycle (iteration limit reached)
	// This checks BEFORE incrementing, so iteration 6 will trigger a new cycle
	shouldStartNewCycle := p.container.CycleService.IncrementCycleIterationInMemory(session)

	if shouldStartNewCycle {
		utils.LogInfo(ctx, "iteration limit reached, starting new cycle",
			slog.Int("max_iterations", services.MaxIterations),
		)

		// Collect products from last cycle
		products := []models.ProductInfo{}
		if session.SearchState.LastProduct != nil {
			products = append(products, *session.SearchState.LastProduct)
		}

		// Start new cycle with context carryover
		p.container.CycleService.StartNewCycleInMemory(session, req.Message, products)
	}

	// Update session state
	session.SearchState.Status = models.SearchStatusIdle

	// Save session once at the end with retry logic (CRITICAL!)
	retryConfig := utils.RetryConfig{
		MaxRetries:    3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      2 * time.Second,
		BackoffFactor: 2.0,
	}

	saveErr := utils.RetryWithBackoff(ctx, func() error {
		return p.container.SessionService.SaveSession(session)
	}, retryConfig)

	if saveErr != nil {
		utils.LogError(ctx, "CRITICAL: failed to save session after retries", saveErr)
		// This is critical - session state is lost!
		// Return error to client so they know something went wrong
		response = &ChatProcessorResponse{
			Type:         "error",
			Output:       "An error occurred while saving your conversation. Please try again.",
			SessionID:    req.SessionID,
			MessageCount: session.MessageCount,
			Error: &ErrorInfo{
				Code:    "session_save_failed",
				Message: "Failed to persist session changes",
			},
		}
		return response
	}

	// Build search state response with real-time anonymous count
	anonymousLimit = p.container.Config.AnonymousSearchLimit

	// Get real-time anonymous search count from Redis
	var currentAnonymousCount int
	if req.UserID == nil && req.BrowserID != "" {
		count, err := p.container.CacheService.GetAnonymousSearchCount(req.BrowserID)
		if err != nil {
			utils.LogError(ctx, "failed to get anonymous search count for response", err)
			count = 0
		}
		currentAnonymousCount = count
	}

	requiresAuth := req.UserID == nil && currentAnonymousCount >= anonymousLimit

	response.SearchState = &models.SearchStateResponse{
		Status:                 string(session.SearchState.Status),
		Category:               session.SearchState.Category,
		CanContinue:            session.SearchState.SearchCount < p.container.SessionService.GetMaxSearches() && !requiresAuth,
		SearchCount:            session.SearchState.SearchCount,
		MaxSearches:            p.container.SessionService.GetMaxSearches(),
		AnonymousSearchUsed:    currentAnonymousCount,
		AnonymousSearchLimit:   anonymousLimit,
		RequiresAuthentication: requiresAuth,
	}

	return response
}

// getOrCreateSession handles session retrieval or creation
func (p *ChatProcessor) getOrCreateSession(req *ChatRequest) (*models.ChatSession, error) {
	var session *models.ChatSession
	var err error

	if req.SessionID != "" {
		// Try to get existing session
		session, err = p.container.SessionService.GetSession(req.SessionID)
		if err != nil {
			// Session not found - create new one with the SAME ID
			utils.LogInfo(context.Background(), "session not found in Redis, creating new session with same ID",
				slog.String("session_id", req.SessionID),
			)
			session, err = p.container.SessionService.CreateSessionWithUser(req.SessionID, req.Country, req.Language, req.Currency, req.UserID)
			if err != nil {
				return nil, err
			}
		} else {
			// Session exists - preserve language, country, and currency from session
			// Only update if explicitly changed by user (non-empty AND different from session)

			// Update user_id if provided and different (user logged in)
			if req.UserID != nil && (session.UserID == nil || *session.UserID != *req.UserID) {
				utils.LogInfo(context.Background(), "linking session to user",
					slog.String("user_id", req.UserID.String()),
				)
				session.UserID = req.UserID
			}

			// Update language if changed and not empty
			if req.Language != "" && req.Language != session.LanguageCode {
				utils.LogInfo(context.Background(), "updating session language",
					slog.String("from", session.LanguageCode),
					slog.String("to", req.Language),
				)
				session.LanguageCode = req.Language
			}

			// Update currency if changed and not empty
			if req.Currency != "" && req.Currency != session.Currency {
				utils.LogInfo(context.Background(), "updating session currency",
					slog.String("from", session.Currency),
					slog.String("to", req.Currency),
				)
				session.Currency = req.Currency
			}

			// Update country if changed and not empty
			if req.Country != "" && req.Country != session.CountryCode {
				utils.LogInfo(context.Background(), "updating session country",
					slog.String("from", session.CountryCode),
					slog.String("to", req.Country),
				)
				session.CountryCode = req.Country
			}

			// Note: Session updates will be saved at the end of ProcessChat with SaveSession()
			// This avoids an extra save operation here

			// Override request with session values to ensure consistency
			req.Language = session.LanguageCode
			req.Country = session.CountryCode
			req.Currency = session.Currency
		}
	} else {
		// No session ID provided - generate new one
		req.SessionID = uuid.New().String()
		session, err = p.container.SessionService.CreateSessionWithUser(req.SessionID, req.Country, req.Language, req.Currency, req.UserID)
		if err != nil {
			return nil, err
		}
	}

	return session, nil
}

// performSearch executes product search with translation
func (p *ChatProcessor) performSearch(geminiResp *models.GeminiResponse, country, language string) ([]models.ProductCard, string, error) {
	ctx := context.Background()

	// Translate query to English for better search results
	utils.LogInfo(ctx, "translation check", slog.String("search_phrase", geminiResp.SearchPhrase))

	translatedQuery, err := p.container.GeminiService.TranslateToEnglish(geminiResp.SearchPhrase)
	if err != nil {
		utils.LogWarn(ctx, "translation failed, using original query", slog.Any("error", err))
		translatedQuery = geminiResp.SearchPhrase
	} else if translatedQuery != geminiResp.SearchPhrase {
		utils.LogInfo(ctx, "query translated",
			slog.String("original", geminiResp.SearchPhrase),
			slog.String("translated", translatedQuery),
		)
	} else {
		utils.LogInfo(ctx, "query already in English", slog.String("query", translatedQuery))
	}

	// Log price range if provided
	if geminiResp.MinPrice != nil || geminiResp.MaxPrice != nil {
		utils.LogInfo(ctx, "price range specified",
			slog.Any("min_price", geminiResp.MinPrice),
			slog.Any("max_price", geminiResp.MaxPrice),
		)
	}

	utils.LogInfo(ctx, "sending to SERP", slog.String("query", translatedQuery))

	// NOTE: Price range is for visual display only, not used in actual search
	// This allows broader search results while showing price guidance to users
	products, _, err := p.container.SerpService.SearchWithCache(
		translatedQuery,
		geminiResp.SearchType,
		country,
		nil, // minPrice - not used for search
		nil, // maxPrice - not used for search
		p.container.CacheService,
	)

	if err != nil {
		return nil, translatedQuery, err
	}

	return products, translatedQuery, nil
}

// saveSearchHistory saves the search to history
func (p *ChatProcessor) saveSearchHistory(req *ChatRequest, session *models.ChatSession, geminiResp *models.GeminiResponse, translatedQuery string, products []models.ProductCard) {
	// Set currency from request or use default
	currency := req.Currency
	if currency == "" {
		currency = session.Currency
	}

	// Use session ID as string (no parsing needed)
	var sessionIDStr *string
	if req.SessionID != "" {
		sessionIDStr = &req.SessionID
	}

	history := &models.SearchHistory{
		UserID:         req.UserID,
		SessionID:      sessionIDStr,
		SearchQuery:    geminiResp.SearchPhrase,
		OptimizedQuery: &translatedQuery,
		SearchType:     geminiResp.SearchType,
		Category:       &geminiResp.Category,
		CountryCode:    req.Country,
		LanguageCode:   req.Language,
		Currency:       currency,
		ResultCount:    len(products),
		ProductsFound:  products,
	}

	// Save asynchronously to avoid blocking
	go func() {
		ctx := context.Background()
		if err := p.container.SearchHistoryService.SaveSearchHistory(ctx, history); err != nil {
			utils.LogWarn(ctx, "failed to save search history", slog.Any("error", err))
		} else {
			utils.LogInfo(ctx, "search history saved",
				slog.String("search_query", geminiResp.SearchPhrase),
				slog.Int("result_count", len(products)),
			)
		}
	}()
}

// parsePrice extracts numeric price from price string
func parsePrice(priceStr string) float64 {
	priceStr = strings.ReplaceAll(priceStr, "$", "")
	priceStr = strings.ReplaceAll(priceStr, "€", "")
	priceStr = strings.ReplaceAll(priceStr, "£", "")
	priceStr = strings.ReplaceAll(priceStr, "CHF", "")
	priceStr = strings.TrimSpace(priceStr)
	priceStr = strings.ReplaceAll(priceStr, ",", "")

	price, _ := strconv.ParseFloat(priceStr, 64)
	return price
}
