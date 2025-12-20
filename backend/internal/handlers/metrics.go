package handlers

import (
	"log"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsHandler handles Prometheus metrics endpoint
type MetricsHandler struct {
	// Pre-created handler to avoid recreating on every request
	handler fiber.Handler
}

// NewMetricsHandler creates a new metrics handler
func NewMetricsHandler() *MetricsHandler {
	// Create the Prometheus handler once at initialization
	promHandler := promhttp.Handler()
	fiberHandler := adaptor.HTTPHandler(promHandler)

	return &MetricsHandler{
		handler: fiberHandler,
	}
}

// GetMetrics returns Prometheus metrics
// @Summary Get Prometheus metrics
// @Description Returns Prometheus metrics in text format
// @Tags monitoring
// @Produce plain
// @Success 200 {string} string "Prometheus metrics"
// @Router /metrics [get]
func (h *MetricsHandler) GetMetrics(c *fiber.Ctx) error {
	// Directly call the pre-created handler
	err := h.handler(c)

	// Only log errors
	if err != nil {
		statusCode := c.Response().StatusCode()
		log.Printf("❌ Metrics handler error - status: %d, error: %v", statusCode, err)

		if statusCode >= 500 {
			bodyBytes := c.Response().Body()
			if len(bodyBytes) > 0 {
				log.Printf("❌ Response body (first 1000 chars): %s", string(bodyBytes[:min(len(bodyBytes), 1000)]))
			}
		}
	}

	return err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
