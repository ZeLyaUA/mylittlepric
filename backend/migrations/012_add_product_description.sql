-- Migration: Add product_description column to messages table
-- Purpose: Store AI-generated product descriptions for better UX
-- Date: 2025-12-25

-- Add product_description column to messages table
ALTER TABLE messages ADD COLUMN IF NOT EXISTS product_description TEXT;

-- Add comment for documentation
COMMENT ON COLUMN messages.product_description IS 'AI-generated description about the products in this message (optional, only for product search results)';
