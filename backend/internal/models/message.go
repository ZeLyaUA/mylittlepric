package models

import (
	"time"

	"github.com/google/uuid"
)

// ═══════════════════════════════════════════════════════════
// MESSAGE MODELS
// ═══════════════════════════════════════════════════════════

type Message struct {
	ID                 uuid.UUID              `json:"id" db:"id"`
	SessionID          uuid.UUID              `json:"session_id" db:"session_id"`
	Role               string                 `json:"role" db:"role"`
	Content            string                 `json:"content" db:"content"`
	ResponseType       string                 `json:"response_type,omitempty" db:"response_type"`
	QuickReplies       []string               `json:"quick_replies,omitempty" db:"quick_replies"`
	Products           []ProductCard          `json:"products,omitempty" db:"products"`
	ProductDescription string                 `json:"product_description,omitempty" db:"product_description"`
	SearchInfo         map[string]interface{} `json:"search_info,omitempty" db:"search_info"`
	CreatedAt          time.Time              `json:"created_at" db:"created_at"`
}
