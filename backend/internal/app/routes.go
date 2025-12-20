package app

import (
	"fmt"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	"mylittleprice/internal/container"
	"mylittleprice/internal/handlers"
	"mylittleprice/internal/middleware"
)

// SetupRoutes configures all application routes
func SetupRoutes(app *fiber.App, c *container.Container) {
	// Prometheus metrics endpoint (no auth required for scraping)
	metricsHandler := handlers.NewMetricsHandler()
	app.Get("/metrics", metricsHandler.GetMetrics)

	// Health check
	app.Get("/health", func(ctx *fiber.Ctx) error {
		return ctx.JSON(fiber.Map{
			"status":    "ok",
			"timestamp": time.Now(),
			"services":  c.HealthCheck(),
		})
	})

	// Apply Prometheus middleware to all /api routes
	api := app.Group("/api", middleware.PrometheusMiddleware())

	// Authentication routes (public)
	setupAuthRoutes(api, c)

	// WebSocket chat (optional authentication)
	setupWebSocketRoutes(app, c)

	// Chat endpoints (optional authentication)
	setupChatRoutes(api, c)

	// Product routes
	setupProductRoutes(api, c)

	// Search history routes (optional authentication)
	setupSearchHistoryRoutes(api, c)

	// Session routes (authenticated)
	setupSessionRoutes(api, c)

	// User preferences routes (authenticated)
	setupPreferencesRoutes(api, c)

	// Stats routes
	setupStatsRoutes(api, c)

	// Bug report routes
	setupBugReportRoutes(api, c)

	// Contact form routes
	setupContactRoutes(api, c)
}

func setupAuthRoutes(api fiber.Router, c *container.Container) {
	auth := api.Group("/auth")
	authHandler := handlers.NewAuthHandler(c)
	authMiddleware := middleware.AuthMiddleware(c.JWTService)
	authRateLimiter := middleware.AuthRateLimiter(c.Redis)

	// Public routes with rate limiting
	auth.Post("/signup", authRateLimiter, authHandler.Signup)
	auth.Post("/login", authRateLimiter, authHandler.Login)
	auth.Post("/google", authRateLimiter, authHandler.GoogleLogin)
	auth.Post("/refresh", authRateLimiter, authHandler.RefreshToken)
	auth.Post("/logout", authHandler.Logout)

	// Password reset routes (public)
	auth.Post("/request-password-reset", authRateLimiter, authHandler.RequestPasswordReset)
	auth.Post("/reset-password", authRateLimiter, authHandler.ResetPassword)

	// Protected routes
	auth.Get("/me", authMiddleware, authHandler.GetMe)
	auth.Post("/claim-sessions", authMiddleware, authHandler.ClaimSessions)
	auth.Post("/change-password", authMiddleware, authHandler.ChangePassword)
}

func setupWebSocketRoutes(app *fiber.App, c *container.Container) {
	wsHandler := handlers.NewWSHandler(c)
	wsRateLimiter := middleware.WebSocketRateLimiter(c.Redis, 30) // Max 30 connections per minute per IP

	app.Use("/ws", wsRateLimiter, func(ctx *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(ctx) {
			// Try to get token from query parameter or Authorization header
			var token string

			// First, try query parameter (for WebSocket compatibility)
			queryToken := ctx.Query("token")
			if queryToken != "" {
				token = queryToken
			} else {
				// Fallback to Authorization header
				authHeader := ctx.Get("Authorization")
				if authHeader != "" && len(authHeader) > 7 {
					token = authHeader[7:] // Remove "Bearer " prefix
				}
			}

			// If token is provided, validate it
			if token != "" {
				claims, err := c.JWTService.ValidateAccessToken(token)
				if err == nil {
					// Store user info in locals for WebSocket handler
					ctx.Locals("user_id", claims.UserID)
					ctx.Locals("user_email", claims.Email)
				}
				// If token validation fails, we just proceed without authentication
			}

			ctx.Locals("allowed", true)
			return ctx.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		wsHandler.HandleWebSocket(conn)
	}, websocket.Config{
		// Set read deadline to 60 seconds (should receive ping within this time)
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}))
}

func setupChatRoutes(api fiber.Router, c *container.Container) {
	chatHandler := handlers.NewChatHandler(c)
	optionalAuthMiddleware := middleware.OptionalAuthMiddleware(c.JWTService)
	sessionOwnership := c.SessionOwnershipChecker.ValidateSessionOwnership()

	api.Post("/chat", optionalAuthMiddleware, chatHandler.HandleChat)
	api.Get("/chat/messages/since", optionalAuthMiddleware, sessionOwnership, chatHandler.GetMessagesSince) // Reconnect endpoint with ownership check
	api.Get("/chat/messages", optionalAuthMiddleware, sessionOwnership, chatHandler.GetSessionMessages)     // Get messages with ownership check
}

func setupProductRoutes(api fiber.Router, c *container.Container) {
	productHandler := handlers.NewProductHandler(c)
	api.Post("/product-details", productHandler.HandleProductDetails)
}

func setupSearchHistoryRoutes(api fiber.Router, c *container.Container) {
	historyHandler := handlers.NewSearchHistoryHandler(c)
	authMiddleware := middleware.AuthMiddleware(c.JWTService)
	optionalAuthMiddleware := middleware.OptionalAuthMiddleware(c.JWTService)

	// Get search history - supports both authenticated and anonymous users
	api.Get("/search-history", optionalAuthMiddleware, historyHandler.GetSearchHistory)

	// Delete operations - support both authenticated and anonymous users
	api.Delete("/search-history/:id", optionalAuthMiddleware, historyHandler.DeleteSearchHistory)
	api.Post("/search-history/:id/click", optionalAuthMiddleware, historyHandler.TrackProductClick)

	// Delete all - requires authentication
	api.Delete("/search-history", authMiddleware, historyHandler.DeleteAllSearchHistory)
}

func setupSessionRoutes(api fiber.Router, c *container.Container) {
	sessionHandler := handlers.NewSessionHandler(c)
	authMiddleware := middleware.AuthMiddleware(c.JWTService)
	optionalAuthMiddleware := middleware.OptionalAuthMiddleware(c.JWTService)

	// Session management - requires authentication and ownership validation
	sessions := api.Group("/sessions")
	sessions.Get("/active", authMiddleware, sessionHandler.GetActiveSession)
	sessions.Get("/active-search", authMiddleware, sessionHandler.GetActiveSearchSession)
	sessions.Post("/link", authMiddleware, sessionHandler.LinkSessionToUser)

	// Sign session ID - optional authentication (works for both authenticated and anonymous users)
	sessions.Post("/sign", optionalAuthMiddleware, sessionHandler.GetSignedSessionID)
}

func setupPreferencesRoutes(api fiber.Router, c *container.Container) {
	preferencesHandler := handlers.NewPreferencesHandler(c)
	authMiddleware := middleware.AuthMiddleware(c.JWTService)

	// User preferences - requires authentication
	userGroup := api.Group("/user", authMiddleware)
	userGroup.Get("/preferences", preferencesHandler.GetUserPreferences)
	userGroup.Put("/preferences", preferencesHandler.UpdateUserPreferences)
}

func setupStatsRoutes(api fiber.Router, c *container.Container) {
	api.Get("/stats/keys", func(ctx *fiber.Ctx) error {
		geminiStats, _ := c.GeminiRotator.GetAllStats()
		serpStats, _ := c.SerpRotator.GetAllStats()

		return ctx.JSON(fiber.Map{
			"gemini": geminiStats,
			"serp":   serpStats,
		})
	})

	api.Get("/stats/grounding", func(ctx *fiber.Ctx) error {
		stats := c.GeminiService.GetGroundingStats()

		groundingPercentage := float32(0)
		if stats.TotalDecisions > 0 {
			groundingPercentage = float32(stats.GroundingEnabled) / float32(stats.TotalDecisions) * 100
		}

		return ctx.JSON(fiber.Map{
			"total_decisions":      stats.TotalDecisions,
			"grounding_enabled":    stats.GroundingEnabled,
			"grounding_disabled":   stats.GroundingDisabled,
			"grounding_percentage": fmt.Sprintf("%.1f%%", groundingPercentage),
			"reason_breakdown":     stats.ReasonCounts,
			"average_confidence":   fmt.Sprintf("%.2f", stats.AverageConfidence),
			"mode":                 c.Config.GeminiGroundingMode,
			"config": fiber.Map{
				"enabled":   c.Config.GeminiUseGrounding,
				"min_words": c.Config.GeminiGroundingMinWords,
			},
		})
	})

	api.Get("/stats/tokens", func(ctx *fiber.Ctx) error {
		tokenStats := c.GeminiService.GetTokenStats()

		return ctx.JSON(fiber.Map{
			"token_usage": tokenStats,
			"timestamp":   time.Now(),
		})
	})

	api.Get("/stats/all", func(ctx *fiber.Ctx) error {
		geminiStats, _ := c.GeminiRotator.GetAllStats()
		serpStats, _ := c.SerpRotator.GetAllStats()
		groundingStats := c.GeminiService.GetGroundingStats()
		tokenStats := c.GeminiService.GetTokenStats()

		groundingPercentage := float32(0)
		if groundingStats.TotalDecisions > 0 {
			groundingPercentage = float32(groundingStats.GroundingEnabled) / float32(groundingStats.TotalDecisions) * 100
		}

		return ctx.JSON(fiber.Map{
			"api_keys": fiber.Map{
				"gemini": geminiStats,
				"serp":   serpStats,
			},
			"grounding": fiber.Map{
				"total_decisions":      groundingStats.TotalDecisions,
				"grounding_enabled":    groundingStats.GroundingEnabled,
				"grounding_disabled":   groundingStats.GroundingDisabled,
				"grounding_percentage": fmt.Sprintf("%.1f%%", groundingPercentage),
				"reason_breakdown":     groundingStats.ReasonCounts,
				"average_confidence":   fmt.Sprintf("%.2f", groundingStats.AverageConfidence),
				"mode":                 c.Config.GeminiGroundingMode,
			},
			"tokens":    tokenStats,
			"timestamp": time.Now(),
		})
	})
}

func setupBugReportRoutes(api fiber.Router, c *container.Container) {
	bugReportHandler := handlers.NewBugReportHandler(c)
	bugReportRateLimiter := middleware.RateLimiter(middleware.RateLimiterConfig{
		Redis:      c.Redis,
		Max:        5,
		Window:     time.Minute,
		KeyPrefix:  "bug_report_limit:",
		Message:    "Too many bug reports, please try again later",
		StatusCode: fiber.StatusTooManyRequests,
		KeyGenerator: func(ctx *fiber.Ctx) string {
			return ctx.IP()
		},
	})

	// Public endpoint - anyone can submit bug reports
	api.Post("/bug-report", bugReportRateLimiter, bugReportHandler.SubmitBugReport)

	// Admin endpoint - requires authentication
	// TODO: Add admin middleware when implemented
	// api.Get("/bug-reports", authMiddleware, adminMiddleware, bugReportHandler.GetBugReports)
}

func setupContactRoutes(api fiber.Router, c *container.Container) {
	contactHandler := handlers.NewContactHandler(c)
	contactRateLimiter := middleware.RateLimiter(middleware.RateLimiterConfig{
		Redis:      c.Redis,
		Max:        3,
		Window:     time.Minute,
		KeyPrefix:  "contact_limit:",
		Message:    "Too many contact form submissions, please try again later",
		StatusCode: fiber.StatusTooManyRequests,
		KeyGenerator: func(ctx *fiber.Ctx) string {
			return ctx.IP()
		},
	})

	// Public endpoint - anyone can submit contact forms
	api.Post("/contact", contactRateLimiter, contactHandler.SubmitContactForm)
}
