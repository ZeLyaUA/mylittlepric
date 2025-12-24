/**
 * Browser identifier for tracking anonymous users across sessions
 * This persists in localStorage even after logout
 */

const BROWSER_ID_KEY = "anonymous_browser_id";

/**
 * Generate a unique browser ID
 * Format: timestamp-randomString
 */
function generateBrowserId(): string {
  const timestamp = Date.now();
  const random = Math.random().toString(36).substring(2, 15);
  return `${timestamp}-${random}`;
}

/**
 * Get or create browser ID for anonymous user tracking
 * This ID persists across login/logout cycles
 */
export function getBrowserId(): string {
  if (typeof window === "undefined") {
    return ""; // SSR
  }

  // Try to get existing browser ID
  let browserId = localStorage.getItem(BROWSER_ID_KEY);

  // Create new one if doesn't exist
  if (!browserId) {
    browserId = generateBrowserId();
    localStorage.setItem(BROWSER_ID_KEY, browserId);
  }

  return browserId;
}

/**
 * Clear browser ID (for testing purposes only)
 * WARNING: This will reset anonymous search limits
 */
export function clearBrowserId(): void {
  if (typeof window === "undefined") return;
  localStorage.removeItem(BROWSER_ID_KEY);
}
