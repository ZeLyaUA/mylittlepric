package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/google/uuid"

	"mylittleprice/internal/container"
	"mylittleprice/internal/models"
	"mylittleprice/internal/services"
	"mylittleprice/internal/utils"
)

type Client struct {
	Conn   *websocket.Conn
	UserID *uuid.UUID // nil for anonymous users
}

type WSHandler struct {
	container   *container.Container
	processor   *ChatProcessor
	clients     map[string]*Client            // clientID -> Client
	userConns   map[uuid.UUID]map[string]bool // userID -> set of clientIDs
	mu          sync.RWMutex
	pubsub      *services.PubSubService // Redis Pub/Sub for cross-server communication
	rateLimiter *utils.WSRateLimiter    // WebSocket message rate limiter
}

func NewWSHandler(c *container.Container) *WSHandler {
	// Create PubSub service
	pubsub := services.NewPubSubService(c.Redis)

	// Create WebSocket rate limiter
	rateLimiter := utils.NewWSRateLimiter(utils.DefaultWSRateLimitConfig())

	handler := &WSHandler{
		container:   c,
		processor:   NewChatProcessor(c),
		clients:     make(map[string]*Client),
		userConns:   make(map[uuid.UUID]map[string]bool),
		pubsub:      pubsub,
		rateLimiter: rateLimiter,
	}

	// Subscribe to all users broadcast channel
	// This allows this server to receive messages from other servers
	pubsub.SubscribeToAllUsers(handler.handleBroadcastMessage)

	log.Printf("🚀 WebSocket handler initialized with Pub/Sub and Rate Limiting (ServerID: %s)", pubsub.GetServerID()[:8])

	return handler
}

type WSMessage struct {
	Type            string                 `json:"type"`
	SessionID       string                 `json:"session_id"`
	Message         string                 `json:"message"`
	Country         string                 `json:"country"`
	Language        string                 `json:"language"`
	Currency        string                 `json:"currency"`
	NewSearch       bool                   `json:"new_search"`
	PageToken       string                 `json:"page_token"`
	CurrentCategory string                 `json:"current_category"`
	BrowserID       string                 `json:"browser_id,omitempty"` // Persistent browser identifier for anonymous tracking
	AccessToken     string                 `json:"access_token,omitempty"` // Optional JWT token for authentication
	Preferences     map[string]interface{} `json:"preferences,omitempty"`  // For preferences sync
	SavedSearch     *models.SavedSearch    `json:"saved_search,omitempty"` // For saved search sync
}

type WSResponse struct {
	Type               string                         `json:"type"`
	MessageID          string                         `json:"message_id,omitempty"` // Unique message ID for deduplication
	Output             string                         `json:"output,omitempty"`
	QuickReplies       []string                       `json:"quick_replies,omitempty"`
	Products           []models.ProductCard           `json:"products,omitempty"`
	ProductDescription string                         `json:"product_description,omitempty"` // AI-generated description about products
	SearchType         string                         `json:"search_type,omitempty"`
	SessionID          string                         `json:"session_id"`
	MessageCount       int                            `json:"message_count,omitempty"`
	SearchState        *models.SearchStateResponse    `json:"search_state,omitempty"`
	ProductDetails     *models.ProductDetailsResponse `json:"product_details,omitempty"`
	Error              string                         `json:"error,omitempty"`
	Message            string                         `json:"message,omitempty"`
}

func (h *WSHandler) HandleWebSocket(c *websocket.Conn) {
	clientID := uuid.New().String()
	var userID *uuid.UUID

	log.Printf("🔌 Client connected: %s", clientID)

	// Record connection metrics
	cleanup := h.recordConnectionStart()
	defer cleanup()

	// Set initial read deadline (60 seconds - client should ping within 30s)
	c.SetReadDeadline(time.Now().Add(60 * time.Second))

	// Set pong handler to reset read deadline
	c.SetPongHandler(func(string) error {
		// Reset read deadline when we receive pong
		c.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// First message should contain access_token if user is authenticated
	// We'll update userID as messages come in with access_token
	client := &Client{
		Conn:   c,
		UserID: nil,
	}
	h.addClient(clientID, client)
	defer h.removeClient(clientID)

	for {
		var msg WSMessage
		err := c.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("❌ WebSocket error: %v", err)
				h.recordConnectionFailed()
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Printf("⏱️ WebSocket timeout (no ping received): %s", clientID)
			}
			break
		}

		// Reset read deadline on any message (not just pong)
		c.SetReadDeadline(time.Now().Add(60 * time.Second))

		// Record message received
		h.recordMessageReceived(msg.Type)

		// Update userID if access_token is provided
		if msg.AccessToken != "" {
			claims, err := h.container.JWTService.ValidateAccessToken(msg.AccessToken)
			if err == nil {
				if userID == nil || *userID != claims.UserID {
					// First time or changed user - update mapping
					h.updateClientUser(clientID, &claims.UserID)
					userID = &claims.UserID
					client.UserID = userID
					log.Printf("🔐 Client %s authenticated as user %s", clientID, userID.String())
				}
			}
		}

		h.handleMessage(c, &msg, clientID)
	}

	log.Printf("🔌 Client disconnected: %s", clientID)
}

func (h *WSHandler) handleMessage(c *websocket.Conn, msg *WSMessage, clientID string) {
	// Skip rate limiting for ping messages
	if msg.Type != "ping" {
		// Check connection-level rate limit
		allowed, reason, retryAfter := h.rateLimiter.CheckConnection(clientID)
		if !allowed {
			h.recordRateLimitViolation("connection")
			h.sendRateLimitError(c, reason, retryAfter)
			return
		}

		// Check user-level rate limit if authenticated
		if msg.AccessToken != "" {
			claims, err := h.container.JWTService.ValidateAccessToken(msg.AccessToken)
			if err == nil {
				allowed, reason, retryAfter := h.rateLimiter.CheckUser(claims.UserID)
				if !allowed {
					h.recordRateLimitViolation("user")
					h.sendRateLimitError(c, reason, retryAfter)
					return
				}
			}
		}
	}

	switch msg.Type {
	case "chat":
		h.handleChat(c, msg, clientID)
	case "product_details":
		h.handleProductDetails(c, msg)
	case "ping":
		h.sendResponse(c, &WSResponse{Type: "pong"})
	case "sync_preferences":
		h.handleSyncPreferences(c, msg, clientID)
	case "sync_saved_search":
		h.handleSyncSavedSearch(c, msg, clientID)
	case "sync_session":
		h.handleSyncSession(c, msg, clientID)
	default:
		h.sendError(c, "unknown_message_type", "Unknown message type")
	}
}

func (h *WSHandler) handleChat(c *websocket.Conn, msg *WSMessage, clientID string) {
	// Extract user ID from access token if provided
	var userID *uuid.UUID
	if msg.AccessToken != "" {
		claims, err := h.container.JWTService.ValidateAccessToken(msg.AccessToken)
		if err == nil {
			userID = &claims.UserID
		}
	}

	// Extract base session ID from signed session ID if applicable
	sessionID := msg.SessionID
	if h.container.SessionOwnershipChecker.Signer.IsSignedSessionID(sessionID) {
		baseSessionID, embeddedUserID, err := h.container.SessionOwnershipChecker.Signer.VerifyAndExtractSessionID(sessionID, 24*time.Hour)
		if err != nil {
			h.sendError(c, "invalid_session", "Invalid or expired session signature")
			return
		}

		// Verify user ID matches if embedded in signature
		if embeddedUserID != nil && userID != nil {
			if *embeddedUserID != *userID {
				h.sendError(c, "session_ownership", "Session belongs to different user")
				return
			}
		}

		// Use base session ID for processing
		sessionID = baseSessionID
	}

	// Generate message IDs upfront for consistent deduplication across devices
	userMessageID := uuid.New().String()
	assistantMessageID := uuid.New().String()

	// Broadcast user message to other devices BEFORE processing
	if userID != nil {
		userMsgSync := &WSResponse{
			Type:      "user_message_sync",
			MessageID: userMessageID, // Use pre-generated ID for consistency
			Output:    msg.Message,
			SessionID: sessionID, // Use base session ID
		}
		h.broadcastToUser(*userID, userMsgSync, clientID)
	}

	// Process chat using shared processor
	processorReq := &ChatRequest{
		SessionID:         sessionID, // Use base session ID
		UserID:            userID,
		Message:           msg.Message,
		Country:           msg.Country,
		Language:          msg.Language,
		Currency:          msg.Currency,
		NewSearch:         msg.NewSearch,
		CurrentCategory:   msg.CurrentCategory,
		BrowserID:         msg.BrowserID, // Pass browser ID for anonymous tracking
		UserMessageID:     userMessageID,     // Pass pre-generated user message ID
		AssistantMessageID: assistantMessageID, // Pass pre-generated assistant message ID
	}

	result := h.processor.ProcessChat(processorReq)

	// Handle errors
	if result.Error != nil {
		h.sendError(c, result.Error.Code, result.Error.Message)
		return
	}

	// Use the pre-generated assistant message ID for consistent deduplication
	// (it was passed to processor and used for the message in database)
	messageID := assistantMessageID

	// Build response
	response := &WSResponse{
		Type:               result.Type,
		MessageID:          messageID, // Same ID as in database
		Output:             result.Output,
		QuickReplies:       result.QuickReplies,
		Products:           result.Products,
		ProductDescription: result.ProductDescription,
		SearchType:         result.SearchType,
		SessionID:          result.SessionID,
		MessageCount:       result.MessageCount,
		SearchState:        result.SearchState,
	}

	// Send response to the sender
	h.sendResponse(c, response)

	// Broadcast assistant message to other devices of the same user
	if userID != nil {
		// Create sync message for other devices (same message_id for deduplication)
		syncMsg := &WSResponse{
			Type:               "assistant_message_sync",
			MessageID:          messageID, // Same ID as sent to sender and in database
			Output:             result.Output,
			QuickReplies:       result.QuickReplies,
			Products:           result.Products,
			ProductDescription: result.ProductDescription,
			SearchType:         result.SearchType,
			SessionID:          result.SessionID,
			MessageCount:       result.MessageCount,
			SearchState:        result.SearchState,
		}
		h.broadcastToUser(*userID, syncMsg, clientID)
	}
}

func (h *WSHandler) handleProductDetails(c *websocket.Conn, msg *WSMessage) {
	if msg.PageToken == "" {
		h.sendError(c, "validation_error", "Page token is required")
		return
	}

	if msg.Country == "" {
		msg.Country = h.container.Config.DefaultCountry
	}

	// Extract base session ID from signed session ID if applicable
	sessionID := msg.SessionID
	if h.container.SessionOwnershipChecker.Signer.IsSignedSessionID(sessionID) {
		baseSessionID, _, err := h.container.SessionOwnershipChecker.Signer.VerifyAndExtractSessionID(sessionID, 24*time.Hour)
		if err != nil {
			h.sendError(c, "invalid_session", "Invalid or expired session signature")
			return
		}
		sessionID = baseSessionID
	}

	cachedProduct, err := h.container.CacheService.GetProductByToken(msg.PageToken)
	if err == nil && cachedProduct != nil {
		h.sendProductDetailsResponse(c, cachedProduct, sessionID)
		return
	}

	startTime := time.Now()
	ctx := context.Background()
	productDetails, keyIndex, err := h.container.SerpService.GetProductDetailsByToken(ctx, msg.PageToken)
	responseTime := time.Since(startTime)

	h.container.SerpRotator.RecordUsage(keyIndex, err == nil, responseTime)

	if err != nil {
		h.sendError(c, "fetch_error", "Failed to fetch product details")
		return
	}

	if err := h.container.CacheService.SetProductByToken(msg.PageToken, productDetails, h.container.Config.CacheImmersiveTTL); err != nil {
		fmt.Printf("⚠️ Failed to cache product details: %v\n", err)
	}

	h.sendProductDetailsResponse(c, productDetails, sessionID)
}

func (h *WSHandler) sendProductDetailsResponse(c *websocket.Conn, productData map[string]interface{}, sessionID string) {
	details, err := FormatProductDetails(productData)
	if err != nil {
		h.sendError(c, "parse_error", err.Error())
		return
	}

	h.sendResponse(c, &WSResponse{
		Type:           "product_details",
		ProductDetails: details,
		SessionID:      sessionID,
	})
}

func (h *WSHandler) addClient(id string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[id] = client
}

func (h *WSHandler) removeClient(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	client, exists := h.clients[id]
	if !exists {
		return
	}

	// Remove from userConns if user was authenticated
	if client.UserID != nil {
		if connSet, ok := h.userConns[*client.UserID]; ok {
			delete(connSet, id)
			// Remove user entry if no more connections
			if len(connSet) == 0 {
				delete(h.userConns, *client.UserID)
			}
		}
	}

	// Remove rate limit data for this connection
	h.rateLimiter.RemoveConnection(id)

	delete(h.clients, id)
}

func (h *WSHandler) updateClientUser(clientID string, userID *uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	client, exists := h.clients[clientID]
	if !exists {
		return
	}

	// Remove from old user's connection set
	if client.UserID != nil {
		if connSet, ok := h.userConns[*client.UserID]; ok {
			delete(connSet, clientID)
			if len(connSet) == 0 {
				delete(h.userConns, *client.UserID)
			}
		}
	}

	// Add to new user's connection set
	if userID != nil {
		if _, ok := h.userConns[*userID]; !ok {
			h.userConns[*userID] = make(map[string]bool)
		}
		h.userConns[*userID][clientID] = true
	}

	client.UserID = userID
}

// broadcastToUser sends a message to all connections of a user except the sender
// This also broadcasts via Redis Pub/Sub to other servers
func (h *WSHandler) broadcastToUser(userID uuid.UUID, response *WSResponse, excludeClientID string) {
	// 1. Broadcast to local clients on this server
	h.mu.RLock()
	clientIDs, hasLocalClients := h.userConns[userID]
	h.mu.RUnlock()

	if hasLocalClients {
		for cid := range clientIDs {
			if cid == excludeClientID {
				continue
			}

			h.mu.RLock()
			client, exists := h.clients[cid]
			h.mu.RUnlock()

			if !exists {
				continue
			}

			if err := client.Conn.WriteJSON(response); err != nil {
				log.Printf("❌ Failed to broadcast to client %s: %v", cid, err)
			}
		}
	}

	// 2. Broadcast to other servers via Redis Pub/Sub
	// This ensures users connected to other backend instances also receive the message
	if err := h.pubsub.BroadcastToAllUsers(userID, response.Type, response); err != nil {
		log.Printf("⚠️ Failed to broadcast to Pub/Sub: %v", err)
	} else {
		h.recordBroadcastSent()
	}
}

// handleBroadcastMessage handles messages received from other servers via Redis Pub/Sub
func (h *WSHandler) handleBroadcastMessage(msg *services.BroadcastMessage) {
	// Record broadcast received
	h.recordBroadcastReceived()

	// Check if we have any local clients for this user
	h.mu.RLock()
	clientIDs, hasClients := h.userConns[msg.UserID]
	h.mu.RUnlock()

	if !hasClients {
		// No local clients for this user, ignore message
		return
	}

	// Convert payload to WSResponse
	payload, ok := msg.Payload.(*WSResponse)
	if !ok {
		// Try to unmarshal from map
		data, err := json.Marshal(msg.Payload)
		if err != nil {
			log.Printf("❌ Failed to marshal broadcast payload: %v", err)
			return
		}

		var wsResp WSResponse
		if err := json.Unmarshal(data, &wsResp); err != nil {
			log.Printf("❌ Failed to unmarshal broadcast payload to WSResponse: %v", err)
			return
		}
		payload = &wsResp
	}

	// Send to all local clients for this user
	for cid := range clientIDs {
		h.mu.RLock()
		client, exists := h.clients[cid]
		h.mu.RUnlock()

		if !exists {
			continue
		}

		if err := client.Conn.WriteJSON(payload); err != nil {
			log.Printf("❌ Failed to send broadcast message to client %s: %v", cid, err)
		} else {
			log.Printf("📨 Broadcast from server %s delivered to client %s", msg.ServerID[:8], cid[:8])
		}
	}
}

// handleSyncPreferences handles preference synchronization across devices
func (h *WSHandler) handleSyncPreferences(c *websocket.Conn, msg *WSMessage, clientID string) {
	// Extract user ID from access token
	if msg.AccessToken == "" {
		h.sendError(c, "auth_required", "Authentication required for preferences sync")
		return
	}

	claims, err := h.container.JWTService.ValidateAccessToken(msg.AccessToken)
	if err != nil {
		h.sendError(c, "invalid_token", "Invalid access token")
		return
	}

	// Broadcast preferences update to other devices
	syncMsg := &WSResponse{
		Type:      "preferences_updated",
		SessionID: msg.SessionID,
		Message:   "Preferences updated",
	}

	h.sendResponse(c, &WSResponse{Type: "sync_ack"})
	h.broadcastToUser(claims.UserID, syncMsg, clientID)
}

// handleSyncSavedSearch handles saved search synchronization across devices
func (h *WSHandler) handleSyncSavedSearch(c *websocket.Conn, msg *WSMessage, clientID string) {
	// Extract user ID from access token
	if msg.AccessToken == "" {
		// Anonymous users can't sync across devices
		h.sendError(c, "auth_required", "Authentication required for saved search sync")
		return
	}

	claims, err := h.container.JWTService.ValidateAccessToken(msg.AccessToken)
	if err != nil {
		h.sendError(c, "invalid_token", "Invalid access token")
		return
	}

	// Save the saved_search to server
	err = h.container.PreferencesService.UpdateSavedSearch(claims.UserID, msg.SavedSearch)
	if err != nil {
		log.Printf("❌ Failed to update saved search for user %s: %v", claims.UserID.String(), err)
		h.sendError(c, "update_failed", "Failed to save search")
		return
	}

	// Broadcast saved search update to other devices
	syncMsg := &WSResponse{
		Type:      "saved_search_updated",
		SessionID: msg.SessionID,
	}

	h.sendResponse(c, &WSResponse{Type: "sync_ack"})
	h.broadcastToUser(claims.UserID, syncMsg, clientID)
}

// handleSyncSession handles session change synchronization across devices
func (h *WSHandler) handleSyncSession(c *websocket.Conn, msg *WSMessage, clientID string) {
	// Extract user ID from access token
	if msg.AccessToken == "" {
		return
	}

	claims, err := h.container.JWTService.ValidateAccessToken(msg.AccessToken)
	if err != nil {
		return
	}

	// Broadcast session change to other devices
	syncMsg := &WSResponse{
		Type:      "session_changed",
		SessionID: msg.SessionID,
	}

	h.sendResponse(c, &WSResponse{Type: "sync_ack"})
	h.broadcastToUser(claims.UserID, syncMsg, clientID)
}

func (h *WSHandler) sendResponse(c *websocket.Conn, response *WSResponse) {
	if err := c.WriteJSON(response); err != nil {
		log.Printf("❌ Failed to send response: %v", err)
		h.recordMessageSendFailed(response.Type, "write_error")
	} else {
		h.recordMessageSent(response.Type)
	}
}

func (h *WSHandler) sendError(c *websocket.Conn, errorCode, message string) {
	h.sendResponse(c, &WSResponse{
		Type:    "error",
		Error:   errorCode,
		Message: message,
	})
}

func (h *WSHandler) sendRateLimitError(c *websocket.Conn, reason string, retryAfter time.Duration) {
	h.sendResponse(c, &WSResponse{
		Type:    "error",
		Error:   "rate_limit_exceeded",
		Message: fmt.Sprintf("%s. Retry after %v seconds", reason, int(retryAfter.Seconds())),
	})
}

// GetRateLimiterStats returns rate limiter statistics for monitoring
func (h *WSHandler) GetRateLimiterStats() map[string]interface{} {
	return h.rateLimiter.GetStats()
}
