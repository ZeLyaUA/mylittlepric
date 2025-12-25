import { useEffect, useRef, useMemo } from "react";
import useWebSocket, { ReadyState } from "react-use-websocket";
import { useChatStore } from "@/shared/lib";
import { useAuthStore } from "@/shared/lib";
import { SessionAPI } from "@/shared/lib";
import { generateId } from "@/shared/lib";
import { getBrowserId } from "@/shared/lib";
import { reconnectManager } from "@/shared/lib/reconnect-manager";

/**
 * Build WebSocket URL dynamically based on current page protocol
 */
function getWebSocketUrl(token?: string | null): string {
  if (typeof window === "undefined") return "";

  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const apiUrl = process.env.NEXT_PUBLIC_API_URL;

  let baseUrl: string;
  if (apiUrl) {
    const url = new URL(apiUrl);
    baseUrl = `${protocol}//${url.host}/ws`;
  } else {
    baseUrl = `${protocol}//localhost:8080/ws`;
  }

  // Add token as query parameter if available
  if (token) {
    baseUrl += `?token=${encodeURIComponent(token)}`;
  }

  return baseUrl;
}

export interface UseChatOptions {
  initialQuery?: string;
  sessionId?: string;
  reconnectAttempts?: number;
  reconnectInterval?: number;
}

export interface UseChatReturn {
  sendMessage: (message: string) => Promise<void>;
  handleNewSearch: () => void;
  syncSession: (newSessionId: string) => void;
  syncPreferences: () => void;
  syncSavedSearch: () => void;
  connectionStatus: string;
  isConnected: boolean;
  readyState: ReadyState;
}

/**
 * Extract base session ID from a signed session ID
 * Example: "1762943791279-78vn1po5d.1762955546...." -> "1762943791279-78vn1po5d"
 */
function getBaseSessionId(sessionId: string): string {
  // Base session ID format: timestamp-randomId (e.g., "1762943791279-78vn1po5d")
  // Signed session ID adds more parts: baseId.timestamp.userId.signature
  // Extract just the first part (timestamp-randomId)
  const match = sessionId.match(/^(\d+-[a-z0-9]+)/);
  return match ? match[1] : sessionId;
}

/**
 * Extract timestamp from a session ID
 * Example: "1762943791279-78vn1po5d" -> 1762943791279
 */
function getSessionTimestamp(sessionId: string): number {
  const baseId = getBaseSessionId(sessionId);
  const match = baseId.match(/^(\d+)-/);
  return match ? parseInt(match[1], 10) : 0;
}

/**
 * Check if session A is newer than session B based on timestamp
 */
function isSessionNewer(sessionA: string, sessionB: string): boolean {
  const timestampA = getSessionTimestamp(sessionA);
  const timestampB = getSessionTimestamp(sessionB);
  return timestampA > timestampB;
}

/**
 * Custom hook for managing WebSocket chat connection
 * Handles connection, message sending, and session management
 */
export function useChat(options: UseChatOptions = {}): UseChatReturn {
  const {
    initialQuery,
    sessionId: initialSessionId,
    reconnectAttempts = 10,
    reconnectInterval = 3000,
  } = options;

  const initialQuerySentRef = useRef(false);
  const processedMessageIds = useRef<Set<string>>(new Set());
  const sessionLoadedRef = useRef(false);
  const initializingRef = useRef(false); // Prevent duplicate initialization calls
  const signingSessionRef = useRef(false); // Prevent duplicate session signing calls

  const {
    messages,
    sessionId,
    country,
    language,
    currency,
    currentCategory,
    _hasInitialized,
    addMessage,
    setLoading,
    setSessionId,
    setSearchInProgress,
    setCurrentCategory,
    newSearch,
    initializeLocale,
    loadSessionMessages,
    saveCurrentSearch,
    registerWebSocketSender,
    checkSavedSearchPrompt,
    setSearchState,
  } = useChatStore();

  const { accessToken, isTokenExpired, refreshToken } = useAuthStore();

  // Memoize WebSocket URL to prevent creating multiple connections on re-renders
  const socketUrl = useMemo(() => {
    return getWebSocketUrl(accessToken);
  }, [accessToken]);

  const { sendJsonMessage, lastJsonMessage, readyState } = useWebSocket(
    socketUrl,
    {
      shouldReconnect: (closeEvent) => {
        // For authenticated users - check if token is expired
        if (accessToken && refreshToken) {
          // If token is expired, refresh before reconnecting
          if (isTokenExpired()) {
            import('@/shared/lib/auth-api').then(({ authAPI }) => {
              authAPI.refreshAccessToken().catch((error) => {
                console.error("❌ Failed to refresh token on reconnect:", error);
                useAuthStore.getState().clearAuth();
              });
            });
            // Don't reconnect immediately, wait for token refresh to trigger new connection
            return false;
          }
        }

        // Always allow reconnection for both anonymous and authenticated users
        // Only block if it's a deliberate close (code 1000 - normal closure)
        if (closeEvent?.code === 1000) {
          return false;
        }

        return true;
      },
      reconnectAttempts,
      reconnectInterval,
      onOpen: async () => {
        // Recover missed messages after reconnect
        const lastTimestamp = reconnectManager.getLastMessageTimestamp();
        if (sessionId && lastTimestamp) {
          setLoading(true);

          try {
            const missedMessages = await reconnectManager.recoverMissedMessages(sessionId);

            // Add missed messages to store
            // IMPORTANT: Use msg.id from backend for deduplication
            missedMessages.forEach((msg) => {
              addMessage({
                id: msg.id, // Use backend UUID for deduplication across reconnects
                role: msg.role as "user" | "assistant",
                content: msg.content,
                timestamp: msg.timestamp ? new Date(msg.timestamp).getTime() : Date.now(),
                quick_replies: msg.quick_replies,
                products: msg.products,
                product_description: msg.product_description,
                search_type: msg.search_type,
                isLocal: false, // Recovered messages are not local
              });
            });
          } catch (error) {
            console.error("Failed to sync missed messages:", error);
          } finally {
            setLoading(false);
          }
        }
      },
      onError: (event) => {
        // Extract useful error information from the WebSocket event
        const errorInfo = {
          type: event?.type || 'unknown',
          target: {
            readyState: (event?.target as any)?.readyState,
            url: (event?.target as any)?.url,
          },
          // Check if connection is closing/closed (expected during page reload)
          isClosing: readyState === ReadyState.CLOSING || readyState === ReadyState.CLOSED,
        };

        // Only log as error if it's not just a normal disconnect
        if (!errorInfo.isClosing) {
          console.error("❌ WebSocket error:", errorInfo);
        }
      },
      onClose: () => {
        // Connection closed
      },
    }
  );

  const connectionStatus = {
    [ReadyState.CONNECTING]: "Connecting",
    [ReadyState.OPEN]: "Connected",
    [ReadyState.CLOSING]: "Closing",
    [ReadyState.CLOSED]: "Disconnected",
    [ReadyState.UNINSTANTIATED]: "Uninstantiated",
  }[readyState];

  const isConnected = readyState === ReadyState.OPEN;

  // Send ping every 30 seconds to keep connection alive
  useEffect(() => {
    if (!isConnected) return;

    const pingInterval = setInterval(() => {
      if (readyState === ReadyState.OPEN) {
        sendJsonMessage({ type: 'ping' });
      }
    }, 30000); // 30 seconds

    return () => {
      clearInterval(pingInterval);
    };
  }, [isConnected, readyState, sendJsonMessage]);

  // Register WebSocket sender in store for realtime sync
  useEffect(() => {
    if (isConnected) {
      registerWebSocketSender(sendJsonMessage);
    } else {
      registerWebSocketSender(null);
    }
  }, [isConnected, sendJsonMessage, registerWebSocketSender]);

  // Proactive token refresh mechanism
  // Checks every 5 minutes if token is close to expiring and refreshes it
  useEffect(() => {
    // Only run for authenticated users
    if (!accessToken || !refreshToken) {
      return;
    }

    const checkAndRefreshToken = async () => {
      // Double-check we still have refresh token (user might have logged out)
      const { refreshToken: currentRefreshToken, tokenExpiresAt } = useAuthStore.getState();
      if (!currentRefreshToken || !tokenExpiresAt) {
        return;
      }

      const timeUntilExpiry = tokenExpiresAt - Date.now();
      const twoMinutes = 2 * 60 * 1000;

      if (timeUntilExpiry < twoMinutes && timeUntilExpiry > 0) {
        try {
          const { authAPI } = await import('@/shared/lib/auth-api');
          await authAPI.refreshAccessToken();
        } catch (error) {
          console.error("❌ Failed to refresh token proactively:", error);
          // Don't clear auth here, let it fail naturally on next request
        }
      }
    };

    // Check immediately on mount
    checkAndRefreshToken();

    // Then check every 5 minutes
    const intervalId = setInterval(checkAndRefreshToken, 5 * 60 * 1000);

    return () => clearInterval(intervalId);
  }, [accessToken, refreshToken]);

  // Initialize locale on mount
  useEffect(() => {
    initializeLocale();
  }, [initializeLocale]);

  // Initialize session on mount
  useEffect(() => {
    const initializeSession = async () => {
      // Prevent duplicate initialization calls
      if (initializingRef.current) {
        return;
      }

      const store = useChatStore.getState();

      // Skip if already initialized AND not authenticated
      // For authenticated users, we ALWAYS want to reload from server
      if (store._hasInitialized && !initialSessionId && !accessToken) {
        return;
      }

      initializingRef.current = true;
      useChatStore.setState({ _hasInitialized: true });

      // If session_id is provided in URL, use it and load messages
      if (initialSessionId && !sessionLoadedRef.current) {
        sessionLoadedRef.current = true;
        setSessionId(initialSessionId);
        localStorage.setItem("chat_session_id", initialSessionId);

        try {
          await loadSessionMessages(initialSessionId);
        } catch (error) {
          console.error("Failed to load session from URL:", error);
        }
        return;
      }

      // For authenticated users, check for active session on server
      if (accessToken) {
        // Note: Preferences sync is handled by PreferencesSync component in providers.tsx
        // No need to sync here to avoid duplicate requests

        try {
          const activeSessionResponse = await SessionAPI.getActiveSession();

          if (activeSessionResponse.has_active_session && activeSessionResponse.session) {
            const serverSessionId = activeSessionResponse.session.session_id;
            const localSessionId = store.sessionId || localStorage.getItem("chat_session_id");

            // If server has a different session, check if we should switch
            if (localSessionId && localSessionId !== serverSessionId) {
              // ALWAYS keep session if it came from URL (user explicitly wants to view this session)
              if (initialSessionId) {
                return;
              }

              // If we have messages loaded, check which session is newer
              if (store.messages.length > 0) {
                // Compare timestamps to determine which session is newer
                const serverIsNewer = isSessionNewer(serverSessionId, localSessionId);

                if (serverIsNewer) {
                  // Server session is newer (user started new chat, then reloaded page)
                  // Switch to the newer session - will switch below
                } else {
                  // Local session is newer or equal - keep it
                  return;
                }
              }

              // No messages in current session, safe to switch to server session
              setSessionId(serverSessionId);
              localStorage.setItem("chat_session_id", serverSessionId);

              try {
                await loadSessionMessages(serverSessionId);
              } catch (error) {
                console.error("Failed to load server session:", error);
              }
              return;
            } else if (!localSessionId) {
              // No local session, use server session
              setSessionId(serverSessionId);
              localStorage.setItem("chat_session_id", serverSessionId);

              try {
                await loadSessionMessages(serverSessionId);
              } catch (error) {
                console.error("Failed to load server session:", error);
              }
              return;
            }
          } else if (store.sessionId) {
            // No active session on server, but we have local session
            // Link it to user account
            try {
              await SessionAPI.linkSessionToUser(store.sessionId);
            } catch (error) {
              console.error("Failed to link session to user:", error);
            }
          }
        } catch (error) {
          console.error("Failed to check active session:", error);
          // Continue with local session logic
        }
      }

      // Determine which session to use
      const savedSessionId = localStorage.getItem("chat_session_id");
      const currentSessionId = store.sessionId || savedSessionId;

      if (!currentSessionId) {
        // No session at all - create new one
        const newSessionId = generateId();
        setSessionId(newSessionId);
        localStorage.setItem("chat_session_id", newSessionId);
        return;
      }

      // Update session ID if needed
      if (currentSessionId !== store.sessionId) {
        setSessionId(currentSessionId);
      }
      localStorage.setItem("chat_session_id", currentSessionId);

      // IMPORTANT: For authenticated users, ALWAYS reload from server to ensure sync
      // localStorage is only used for optimistic initial render
      if (accessToken) {
        try {
          await loadSessionMessages(currentSessionId);
        } catch (error) {
          // Failed to load from server, using local cache
        }
      } else {
        // For anonymous users, show local cache but attempt to load from server
        try {
          await loadSessionMessages(currentSessionId);
        } catch (error) {
          // Failed to load from server
        }
      }
    };

    initializeSession().finally(() => {
      initializingRef.current = false;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialSessionId, accessToken]);

  // Check if we should show savedSearch prompt after initialization
  useEffect(() => {
    if (_hasInitialized && !initialSessionId) {
      // Small delay to let state settle
      const timer = setTimeout(() => {
        checkSavedSearchPrompt();

        // Additional safety: If messages are empty after check, ensure prompt is cleared
        const store = useChatStore.getState();
        if (store.messages.length === 0 && store.showSavedSearchPrompt) {
          const savedSearch = store.savedSearch;
          // Only keep prompt if savedSearch is from a different session
          if (!savedSearch || savedSearch.sessionId === store.sessionId) {
            store.setShowSavedSearchPrompt(false);
          }
        }
      }, 100);
      return () => clearTimeout(timer);
    }
  }, [_hasInitialized, initialSessionId, checkSavedSearchPrompt]);

  // Sync sessionId to localStorage when it changes
  useEffect(() => {
    if (sessionId && _hasInitialized) {
      const currentStoredId = localStorage.getItem("chat_session_id");
      if (currentStoredId !== sessionId) {
        localStorage.setItem("chat_session_id", sessionId);
      }
    }
  }, [sessionId, _hasInitialized]);

  // Get signed session ID for authenticated users
  useEffect(() => {
    const signSessionIfAuthenticated = async () => {
      // Only sign sessions for authenticated users
      if (!accessToken || !sessionId) {
        return;
      }

      // Prevent duplicate signing calls
      if (signingSessionRef.current) {
        return;
      }

      // Check if we already have a valid signed session for THIS session ID
      const store = useChatStore.getState();
      if (store.signedSessionId) {
        // Verify it's for the current session (extract base ID from signed session)
        const signedBaseId = getBaseSessionId(store.signedSessionId);
        const currentBaseId = getBaseSessionId(sessionId);
        if (signedBaseId === currentBaseId) {
          return; // Already signed for this session
        }
      }

      signingSessionRef.current = true;
      try {
        const signedResponse = await SessionAPI.signSession(sessionId);
        store.setSignedSessionId(signedResponse.signed_session_id);
      } catch (error) {
        console.error("Failed to sign session:", error);
        // Continue with unsigned session (backward compatible)
      } finally {
        signingSessionRef.current = false;
      }
    };

    signSessionIfAuthenticated();
  }, [accessToken, sessionId]);

  // Handle incoming WebSocket messages
  useEffect(() => {
    if (lastJsonMessage !== null) {
      const data: any = lastJsonMessage;

      if (data.type === "pong") {
        return;
      }

      // Ignore sync acknowledgment messages - they don't need to be displayed
      if (data.type === "sync_ack") {
        return;
      }

      // Use message_id for deduplication if available, otherwise fall back to hash
      const messageId = data.message_id || JSON.stringify(data);

      if (processedMessageIds.current.has(messageId)) {
        return;
      }

      processedMessageIds.current.add(messageId);

      // Clean up old message IDs to prevent memory leak (keep last 100)
      if (processedMessageIds.current.size > 100) {
        const idsArray = Array.from(processedMessageIds.current);
        processedMessageIds.current = new Set(idsArray.slice(-100));
      }

      setLoading(false);

      // Handle realtime sync messages
      if (data.type === "user_message_sync") {
        // User message from another device
        // Ignore if session doesn't match (compare base IDs)
        if (data.session_id && sessionId) {
          const incomingBaseId = getBaseSessionId(data.session_id);
          const currentBaseId = getBaseSessionId(sessionId);
          if (incomingBaseId !== currentBaseId) {
            return;
          }
        }

        const userMessage = {
          id: data.message_id || generateId(), // Use message_id from backend for deduplication
          role: "user" as const,
          content: data.output || "",
          timestamp: Date.now(),
          isLocal: false, // This message is from another device
        };

        addMessage(userMessage);
        setLoading(true);
        return;
      }

      if (data.type === "assistant_message_sync") {
        // Assistant message from another device
        // Ignore if session doesn't match (compare base IDs)
        if (data.session_id && sessionId) {
          const incomingBaseId = getBaseSessionId(data.session_id);
          const currentBaseId = getBaseSessionId(sessionId);
          if (incomingBaseId !== currentBaseId) {
            return;
          }
        }

        const assistantMessage = {
          id: data.message_id || generateId(), // Use message_id from backend for deduplication
          role: "assistant" as const,
          content: data.output || "",
          timestamp: Date.now(),
          quick_replies: data.quick_replies,
          products: data.products,
          product_description: data.product_description,
          search_type: data.search_type,
          isLocal: false, // Synced messages are not local
        };

        addMessage(assistantMessage);
        setLoading(false);

        if (data.search_state && data.search_state.category) {
          setCurrentCategory(data.search_state.category);
        }

        if (data.search_state) {
          setSearchInProgress(data.search_state.status === "completed");
          // Update search state in store for anonymous search tracking
          setSearchState(data.search_state);
        }
        return;
      }

      // Support old message_sync type for backwards compatibility
      if (data.type === "message_sync") {
        // Message from another device (legacy format - assistant only)
        // Ignore if session doesn't match (compare base IDs)
        if (data.session_id && sessionId) {
          const incomingBaseId = getBaseSessionId(data.session_id);
          const currentBaseId = getBaseSessionId(sessionId);
          if (incomingBaseId !== currentBaseId) {
            return;
          }
        }

        const assistantMessage = {
          id: data.message_id || generateId(), // Use message_id from backend for deduplication
          role: "assistant" as const,
          content: data.output || "",
          timestamp: Date.now(),
          quick_replies: data.quick_replies,
          products: data.products,
          product_description: data.product_description,
          search_type: data.search_type,
          isLocal: false, // Legacy synced messages are not local
        };

        addMessage(assistantMessage);

        if (data.search_state && data.search_state.category) {
          setCurrentCategory(data.search_state.category);
        }

        if (data.search_state) {
          setSearchInProgress(data.search_state.status === "completed");
          // Update search state in store for anonymous search tracking
          setSearchState(data.search_state);
        }
        return;
      }

      if (data.type === "preferences_updated") {
        // Preferences changed on another device
        const store = useChatStore.getState();
        store.syncPreferencesFromServer();
        return;
      }

      if (data.type === "saved_search_updated") {
        // Saved search changed on another device
        const store = useChatStore.getState();
        store.syncPreferencesFromServer();
        return;
      }

      if (data.type === "session_changed") {
        // Session changed on another device (e.g., New Search)
        if (data.session_id && sessionId) {
          const incomingBaseId = getBaseSessionId(data.session_id);
          const currentBaseId = getBaseSessionId(sessionId);
          // Only switch if the base session ID is different
          if (incomingBaseId !== currentBaseId) {
            setSessionId(data.session_id);
            localStorage.setItem("chat_session_id", data.session_id);

            // Load messages for new session (may be empty if it's a brand new session)
            loadSessionMessages(data.session_id).catch(() => {
              // Ignore errors for new sessions - they're expected to be empty initially
            });
          }
        }
        return;
      }

      if (data.type === "error") {
        const errorMessage = data.message || data.error || "An error occurred";

        // Check if it's a rate limit error
        if (data.error === "rate_limit_exceeded" || errorMessage.toLowerCase().includes("rate limit exceeded")) {
          // Parse retry_after from message if available
          const retryMatch = errorMessage.match(/retry after (\d+) seconds?/i);
          const retryAfter = retryMatch ? parseInt(retryMatch[1], 10) : 30;

          // Set rate limit state
          const expiresAt = new Date(Date.now() + retryAfter * 1000);
          const store = useChatStore.getState();
          store.setRateLimitState({
            isLimited: true,
            reason: errorMessage,
            retryAfter,
            expiresAt,
          });

          // Auto-clear after retry_after seconds
          setTimeout(() => {
            const store = useChatStore.getState();
            store.clearRateLimitState();
          }, retryAfter * 1000);

          // Don't add error message to chat (notification will be shown instead)
          return;
        }

        // Check if it's a session ownership error
        if (errorMessage.toLowerCase().includes("session ownership") || errorMessage.toLowerCase().includes("unauthorized")) {
          console.error("❌ Session ownership validation failed");

          // Clear invalid session and start fresh
          const newSessionId = generateId();
          setSessionId(newSessionId);
          const store = useChatStore.getState();
          store.setSignedSessionId(null);
          localStorage.setItem("chat_session_id", newSessionId);
          reconnectManager.reset();
          newSearch();

          // Show user-friendly error
          addMessage({
            id: generateId(),
            role: "assistant",
            content: "Your session has expired. Please start a new conversation.",
            timestamp: Date.now(),
            isLocal: true,
          });
          return;
        }

        // Regular error handling
        addMessage({
          id: generateId(),
          role: "assistant",
          content: errorMessage,
          timestamp: Date.now(),
          isLocal: true, // Error responses are local to this device
        });
        return;
      }

      // Ignore messages from old sessions (e.g., after New Search)
      // Compare base session IDs to handle signed session ID variations
      if (data.session_id && sessionId) {
        const incomingBaseId = getBaseSessionId(data.session_id);
        const currentBaseId = getBaseSessionId(sessionId);

        if (incomingBaseId !== currentBaseId) {
          return;
        }
      }

      const assistantMessage = {
        id: generateId(),
        role: "assistant" as const,
        content: data.output || "",
        timestamp: Date.now(),
        quick_replies: data.quick_replies,
        products: data.products,
        product_description: data.product_description,
        search_type: data.search_type,
        isLocal: true, // Direct response to local message
      };

      addMessage(assistantMessage);

      if (data.search_state && data.search_state.category) {
        setCurrentCategory(data.search_state.category);
      }

      if (data.search_state) {
        setSearchInProgress(data.search_state.status === "completed");
        // Update search state in store for anonymous search tracking
        setSearchState(data.search_state);
      }

      // Note: Search history is now managed entirely by the backend
      // History is accessible via SearchHistoryAPI.getSearchHistory()
    }
  }, [
    lastJsonMessage,
    addMessage,
    setLoading,
    setSearchInProgress,
    setCurrentCategory,
    sessionId,
    setSessionId,
    loadSessionMessages,
  ]);

  // Send initial query if provided
  useEffect(() => {
    if (
      initialQuery &&
      !initialQuerySentRef.current &&
      sessionId &&
      isConnected
    ) {
      initialQuerySentRef.current = true;
      sendMessage(initialQuery);
    }
  }, [initialQuery, sessionId, isConnected]);

  const sendMessage = async (message: string) => {
    const textToSend = message.trim();
    if (!textToSend || !isConnected) {
      return;
    }

    const messageId = generateId();
    const userMessage = {
      id: messageId,
      role: "user" as const,
      content: textToSend,
      timestamp: Date.now(),
      isLocal: true, // Message sent from this device
      status: "pending" as const, // Mark as pending
    };

    addMessage(userMessage);
    setLoading(true);

    try {
      const store = useChatStore.getState();
      // Prefer signed session ID if available
      const sessionIdToSend = store.signedSessionId || sessionId;

      sendJsonMessage({
        type: "chat",
        session_id: sessionIdToSend,
        message: textToSend,
        country,
        language,
        currency,
        new_search: false,
        current_category: currentCategory,
        browser_id: getBrowserId(),
        ...(accessToken && { access_token: accessToken }),
      });

      // Mark as sent after successful send
      store.updateMessageStatus(messageId, "sent");

      // Update reconnectManager timestamp
      reconnectManager.setLastMessageTimestamp(new Date());
    } catch (error) {
      console.error("Error sending message:", error);
      setLoading(false);

      // Mark as failed
      const store = useChatStore.getState();
      store.updateMessageStatus(messageId, "failed", "Failed to send message");

      // Show error to user
      addMessage({
        id: generateId(),
        role: "assistant",
        content: "Failed to send message. Please check your connection.",
        timestamp: Date.now(),
        isLocal: true, // Error messages are local
      });
    }
  };

  const handleNewSearch = () => {
    // Save current search before clearing (only if user has sent messages)
    const hasUserMessages = messages.some(msg => msg.role === "user");
    if (hasUserMessages) {
      saveCurrentSearch();
    }

    processedMessageIds.current.clear();
    initialQuerySentRef.current = false;
    // Don't reset _hasInitialized - locale is already initialized
    // Resetting it would trigger unnecessary re-initialization
    newSearch();
    const newSessionId = generateId();
    setSessionId(newSessionId);
    localStorage.setItem("chat_session_id", newSessionId);

    // Sync new session to other devices
    if (accessToken && isConnected) {
      sendJsonMessage({
        type: "sync_session",
        session_id: newSessionId,
        access_token: accessToken,
      });
    }
  };

  const syncSession = (newSessionId: string) => {
    if (accessToken && isConnected) {
      sendJsonMessage({
        type: "sync_session",
        session_id: newSessionId,
        access_token: accessToken,
      });
    }
  };

  const syncPreferences = () => {
    if (accessToken && isConnected) {
      sendJsonMessage({
        type: "sync_preferences",
        session_id: sessionId,
        access_token: accessToken,
      });
    }
  };

  const syncSavedSearch = () => {
    if (accessToken && isConnected) {
      sendJsonMessage({
        type: "sync_saved_search",
        session_id: sessionId,
        access_token: accessToken,
      });
    }
  };

  return {
    sendMessage,
    handleNewSearch,
    syncSession,
    syncPreferences,
    syncSavedSearch,
    connectionStatus,
    isConnected,
    readyState,
  };
}
