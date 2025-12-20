import { ProductDetailsResponse, ChatMessage } from "@/shared/types";
import { useAuthStore } from "./auth-store";
import { rateLimitTracker } from "./rate-limit-tracker";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

export async function getProductDetails(
  pageToken: string,
  country: string
): Promise<ProductDetailsResponse> {
  const response = await fetch(`${API_URL}/api/product-details`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ page_token: pageToken, country }),
  });

  // Track rate limit headers
  rateLimitTracker.updateFromHeaders(response.headers);

  if (!response.ok) {
    throw new Error("Failed to fetch product details");
  }

  return response.json();
}

export interface SessionMessagesResponse {
  messages: Array<{
    id?: string; // UUID from backend - use for deduplication
    role: string;
    content: string;
    timestamp?: string;
    quick_replies?: string[];
    products?: any[];
    search_type?: string;
  }>;
  session_id: string;
  message_count: number;
  search_state?: {
    status: string;
    category: string;
    search_count: number;
    last_product?: any;
  };
}

export async function getSessionMessages(
  sessionId: string
): Promise<SessionMessagesResponse> {
  // Get access token from auth store
  const accessToken = useAuthStore.getState().accessToken;

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };

  // Add Authorization header if token is available
  if (accessToken) {
    headers["Authorization"] = `Bearer ${accessToken}`;
  }

  const response = await fetch(
    `${API_URL}/api/chat/messages?session_id=${encodeURIComponent(sessionId)}`,
    {
      method: "GET",
      headers,
    }
  );

  // Track rate limit headers
  rateLimitTracker.updateFromHeaders(response.headers);

  // Handle 404 - session doesn't exist yet (new session before first message)
  if (response.status === 404) {
    return {
      messages: [],
      session_id: sessionId,
      message_count: 0,
    };
  }

  if (!response.ok) {
    throw new Error("Failed to fetch session messages");
  }

  return response.json();
}

export interface MessagesSinceResponse {
  messages: Array<{
    id: string; // UUID from backend - CRITICAL for deduplication
    role: string;
    content: string;
    timestamp: string;
    quick_replies?: string[];
    products?: any[];
    search_type?: string;
  }>;
  session_id: string;
  message_count: number;
  since: string;
}

/**
 * Get messages since a specific timestamp
 * Used for recovering missed messages after WebSocket reconnection
 *
 * @param sessionId - The session ID
 * @param since - The timestamp to get messages since
 * @returns Messages that were created after the given timestamp
 */
export async function getMessagesSince(
  sessionId: string,
  since: Date
): Promise<MessagesSinceResponse> {
  const accessToken = useAuthStore.getState().accessToken;

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };

  if (accessToken) {
    headers["Authorization"] = `Bearer ${accessToken}`;
  }

  const sinceISO = since.toISOString();
  const url = `${API_URL}/api/chat/messages/since?session_id=${encodeURIComponent(sessionId)}&since=${encodeURIComponent(sinceISO)}`;

  const response = await fetch(url, {
    method: "GET",
    headers,
  });

  // Track rate limit headers
  rateLimitTracker.updateFromHeaders(response.headers);

  if (!response.ok) {
    throw new Error("Failed to fetch messages since timestamp");
  }

  return response.json();
}