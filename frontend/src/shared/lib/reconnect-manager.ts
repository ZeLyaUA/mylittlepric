import { getMessagesSince } from "./api";

/**
 * ReconnectManager handles WebSocket reconnection logic
 * and recovers missed messages after disconnection
 */
export class ReconnectManager {
  private lastMessageTimestamp: Date | null = null;
  private isRecovering = false;

  /**
   * Update the last message timestamp
   * This should be called every time a message is received
   */
  setLastMessageTimestamp(timestamp: Date) {
    this.lastMessageTimestamp = timestamp;
  }

  /**
   * Get the last message timestamp
   */
  getLastMessageTimestamp(): Date | null {
    return this.lastMessageTimestamp;
  }

  /**
   * Check if we're currently recovering missed messages
   */
  isRecoveringMessages(): boolean {
    return this.isRecovering;
  }

  /**
   * Recover messages that were missed during disconnection
   *
   * @param sessionId - The current session ID
   * @returns Array of missed messages, or empty array if recovery fails
   */
  async recoverMissedMessages(sessionId: string): Promise<any[]> {
    if (!this.lastMessageTimestamp || this.isRecovering) {
      return [];
    }

    this.isRecovering = true;

    try {
      const response = await getMessagesSince(sessionId, this.lastMessageTimestamp);

      if (response.message_count > 0) {
        // Update last message timestamp to the most recent message
        if (response.messages.length > 0) {
          const lastMessage = response.messages[response.messages.length - 1];
          if (lastMessage.timestamp) {
            this.lastMessageTimestamp = new Date(lastMessage.timestamp);
          }
        }
      }

      return response.messages;
    } catch (error) {
      console.error("❌ Failed to recover missed messages:", error);
      return [];
    } finally {
      this.isRecovering = false;
    }
  }

  /**
   * Reset the reconnect manager state
   * Useful when starting a new session
   */
  reset() {
    this.lastMessageTimestamp = null;
    this.isRecovering = false;
  }
}

// Create a singleton instance
export const reconnectManager = new ReconnectManager();
