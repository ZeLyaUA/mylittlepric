package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"mylittleprice/internal/container"
	"mylittleprice/internal/models"
)

type ProductHandler struct {
	container *container.Container
}

func NewProductHandler(c *container.Container) *ProductHandler {
	return &ProductHandler{
		container: c,
	}
}

func (h *ProductHandler) HandleProductDetails(c *fiber.Ctx) error {
	var req models.ProductDetailsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Failed to parse request body",
		})
	}

	if req.PageToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Page token is required",
		})
	}

	if req.Country == "" {
		req.Country = h.container.Config.DefaultCountry
	}

	cachedProduct, err := h.container.CacheService.GetProductByToken(req.PageToken)
	if err == nil && cachedProduct != nil {
		return h.formatProductResponse(c, cachedProduct)
	}

	startTime := time.Now()
	productDetails, keyIndex, err := h.container.SerpService.GetProductDetailsByToken(c.UserContext(), req.PageToken)
	responseTime := time.Since(startTime)

	h.container.SerpRotator.RecordUsage(keyIndex, err == nil, responseTime)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "fetch_error",
			Message: "Failed to fetch product details",
		})
	}

	if err := h.container.CacheService.SetProductByToken(req.PageToken, productDetails, h.container.Config.CacheImmersiveTTL); err != nil {
		c.Context().Logger().Printf("Warning: Failed to cache product details: %v", err)
	}

	return h.formatProductResponse(c, productDetails)
}

func (h *ProductHandler) formatProductResponse(c *fiber.Ctx, productData map[string]interface{}) error {
	response, err := FormatProductDetails(productData)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "parse_error",
			Message: err.Error(),
		})
	}

	return c.JSON(response)
}
