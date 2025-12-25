// backend/internal/services/response_schemas.go
package services

import "google.golang.org/genai"

// Helper functions for creating pointers
func int64Ptr(i int64) *int64 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

// GetDialogueResponseSchema returns the schema for dialogue responses
func GetDialogueResponseSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"response_type": {
				Type: genai.TypeString,
				Enum: []string{"dialogue"},
			},
			"output": {
				Type:        genai.TypeString,
				Description: "Short helpful question/message (<400 chars)",
			},
			"quick_replies": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeString,
				},
				MinItems:    int64Ptr(1),
				MaxItems:    int64Ptr(6),
				Description: "Quick reply options with price ranges (e.g., 'Option A (≈$X)')",
			},
			"category": {
				Type:        genai.TypeString,
				Enum:        []string{"brand_specific", "parametric", "generic_model", "unknown"},
				Description: "Product category classification",
			},
		},
		Required:         []string{"response_type", "output", "category"},
		PropertyOrdering: []string{"response_type", "output", "quick_replies", "category"},
	}
}

// GetSearchResponseSchema returns the schema for search responses
func GetSearchResponseSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"response_type": {
				Type: genai.TypeString,
				Enum: []string{"search"},
			},
			"search_phrase": {
				Type:        genai.TypeString,
				Description: "Product name with specifications ONLY. DO NOT include country, location, currency, or words like 'price'. Example: 'Samsung Galaxy S25 Ultra Titanium Black', NOT 'Samsung Galaxy S25 Ultra Titanium Black price Switzerland'",
			},
			"search_type": {
				Type:        genai.TypeString,
				Enum:        []string{"exact", "parameters", "category"},
				Description: "Type of search to perform",
			},
			"category": {
				Type:        genai.TypeString,
				Enum:        []string{"brand_specific", "parametric", "generic_model", "unknown"},
				Description: "Product category classification",
			},
			"price_filter": {
				Type:        genai.TypeString,
				Enum:        []string{"cheaper", "expensive"},
				Description: "Price filter preference",
				Nullable:    boolPtr(true),
			},
			"min_price": {
				Type:        genai.TypeNumber,
				Description: "DEPRECATED: Not used in search. Keep this field empty/null. Price ranges are for visual guidance only.",
				Nullable:    boolPtr(true),
			},
			"max_price": {
				Type:        genai.TypeNumber,
				Description: "DEPRECATED: Not used in search. Keep this field empty/null. Price ranges are for visual guidance only.",
				Nullable:    boolPtr(true),
			},
		},
		Required:         []string{"response_type", "search_phrase", "search_type", "category"},
		PropertyOrdering: []string{"response_type", "search_phrase", "search_type", "category", "price_filter", "min_price", "max_price"},
	}
}

// GetAPIRequestResponseSchema returns the schema for API request responses (final product search)
func GetAPIRequestResponseSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"response_type": {
				Type: genai.TypeString,
				Enum: []string{"api_request"},
			},
			"api": {
				Type:        genai.TypeString,
				Enum:        []string{"google_shopping"},
				Description: "API to call (REQUIRED for api_request)",
			},
			"params": {
				Type:        genai.TypeObject,
				Description: "API parameters (REQUIRED for api_request)",
				Properties: map[string]*genai.Schema{
					"q": {
						Type:        genai.TypeString,
						Description: "Final product name (full official or fully specified name)",
					},
					"gl": {
						Type:        genai.TypeString,
						Description: "Geographic location code",
					},
					"hl": {
						Type:        genai.TypeString,
						Description: "Host language code",
					},
					"currency": {
						Type:        genai.TypeString,
						Description: "Currency code",
					},
				},
				Required:         []string{"q", "gl", "hl", "currency"},
				PropertyOrdering: []string{"q", "gl", "hl", "currency"},
			},
			"category": {
				Type:        genai.TypeString,
				Enum:        []string{"brand_specific", "parametric", "generic_model"},
				Description: "Product category classification",
			},
			"output": {
				Type:        genai.TypeString,
				Description: "Optional message to accompany the results",
				Nullable:    boolPtr(true),
			},
			"product_description": {
				Type:        genai.TypeString,
				Description: "REQUIRED: Brief description about the product category or search results (1-2 sentences, max 200 chars)",
			},
		},
		Required:         []string{"response_type", "api", "params", "category", "product_description"},
		PropertyOrdering: []string{"response_type", "api", "params", "category", "output", "product_description"},
	}
}

// GetUniversalResponseSchema returns a union schema that accepts any of the three response types
// This uses anyOf to allow multiple possible response structures
func GetUniversalResponseSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"response_type": {
				Type:        genai.TypeString,
				Enum:        []string{"dialogue", "search", "api_request"},
				Description: "Type of response",
			},
			// Common fields
			"output": {
				Type:        genai.TypeString,
				Description: "Message output",
				Nullable:    boolPtr(true),
			},
			"category": {
				Type:        genai.TypeString,
				Enum:        []string{"brand_specific", "parametric", "generic_model", "unknown"},
				Description: "Product category",
			},
			// Dialogue-specific
			"quick_replies": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeString,
				},
				Nullable:    boolPtr(true),
				Description: "Quick reply options (for dialogue)",
			},
			// Search-specific
			"search_phrase": {
				Type:        genai.TypeString,
				Nullable:    boolPtr(true),
				Description: "Product name with specifications ONLY. DO NOT include country, location, currency, or words like 'price'",
			},
			"search_type": {
				Type:        genai.TypeString,
				Enum:        []string{"exact", "parameters", "category"},
				Nullable:    boolPtr(true),
				Description: "Search type (for search type)",
			},
			"price_filter": {
				Type:        genai.TypeString,
				Enum:        []string{"cheaper", "expensive"},
				Nullable:    boolPtr(true),
				Description: "Price filter (optional)",
			},
			"min_price": {
				Type:        genai.TypeNumber,
				Nullable:    boolPtr(true),
				Description: "Minimum price in user's currency",
			},
			"max_price": {
				Type:        genai.TypeNumber,
				Nullable:    boolPtr(true),
				Description: "Maximum price in user's currency",
			},
			// API request-specific (REQUIRED when response_type is "api_request")
			"api": {
				Type:        genai.TypeString,
				Enum:        []string{"google_shopping"},
				Description: "API name - REQUIRED when response_type is api_request",
			},
			"params": {
				Type:        genai.TypeObject,
				Description: "API parameters - REQUIRED when response_type is api_request",
				Properties: map[string]*genai.Schema{
					"q":        {Type: genai.TypeString, Description: "Product query"},
					"gl":       {Type: genai.TypeString, Description: "Geographic location"},
					"hl":       {Type: genai.TypeString, Description: "Host language"},
					"currency": {Type: genai.TypeString, Description: "Currency code"},
				},
				Required:         []string{"q"},
				PropertyOrdering: []string{"q", "gl", "hl", "currency"},
			},
			"product_description": {
				Type:        genai.TypeString,
				Description: "REQUIRED: Brief description about the product category or search results (1-2 sentences, max 200 chars). MUST be provided for api_request responses.",
			},
		},
		Required: []string{"response_type", "category"},
		PropertyOrdering: []string{
			"response_type",
			"product_description",  // CRITICAL: Must be second to ensure Gemini always sees it
			"output",
			"category",
			"quick_replies",
			"search_phrase",
			"search_type",
			"price_filter",
			"min_price",
			"max_price",
			"api",
			"params",
		},
	}
}
