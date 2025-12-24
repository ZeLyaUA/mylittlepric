import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import { ChatMessage, SearchState } from "@/shared/types";
import { detectCountry, detectLanguage, getCurrencyForCountry } from "./locale";

export interface SavedSearch {
  messages: ChatMessage[];
  sessionId: string;
  category: string;
  timestamp: number;
}

type WebSocketSender = (message: any) => void;

export interface RateLimitState {
  isLimited: boolean;
  reason: string | null;
  retryAfter: number | null; // seconds
  expiresAt: Date | null;
}

interface ChatStore {
  messages: ChatMessage[];
  sessionId: string;
  isLoading: boolean;
  country: string;
  language: string;
  currency: string;
  searchInProgress: boolean;
  currentCategory: string;
  isSidebarOpen: boolean;
  _hasInitialized: boolean; // Internal flag to track initialization
  savedSearch: SavedSearch | null; // Last search before "New Search" was clicked
  showSavedSearchPrompt: boolean; // Show dialog to continue or start new search
  _wsSender: WebSocketSender | null; // Internal WebSocket sender for realtime sync

  // Reconnect mechanism fields
  lastMessageTimestamp: Date | null;

  // Rate limiting fields
  rateLimitState: RateLimitState;

  // Session ownership validation (signed sessions)
  signedSessionId: string | null;

  // Search state from backend
  searchState: SearchState | null;

  addMessage: (message: ChatMessage) => void;
  setMessages: (messages: ChatMessage[]) => void;
  setSessionId: (id: string) => void;
  setLoading: (loading: boolean) => void;
  setCountry: (country: string) => void;
  setLanguage: (language: string) => void;
  setCurrency: (currency: string) => void;
  setSearchInProgress: (inProgress: boolean) => void;
  setCurrentCategory: (category: string) => void;
  clearMessages: () => void;
  clearAll: () => void;
  newSearch: () => void;
  initializeLocale: () => Promise<void>;
  loadSessionMessages: (sessionId: string) => Promise<void>;
  toggleSidebar: () => void;
  setSidebarOpen: (open: boolean) => void;
  saveCurrentSearch: () => void;
  restoreSavedSearch: () => void;
  clearSavedSearch: () => void;
  syncPreferencesFromServer: () => Promise<void>;
  syncPreferencesToServer: () => Promise<void>;
  registerWebSocketSender: (sender: WebSocketSender | null) => void;
  setShowSavedSearchPrompt: (show: boolean) => void;
  checkSavedSearchPrompt: () => void;

  // New methods for Priority 1 features
  setLastMessageTimestamp: (timestamp: Date | null) => void;
  setRateLimitState: (state: Partial<RateLimitState>) => void;
  clearRateLimitState: () => void;
  setSignedSessionId: (signedSessionId: string | null) => void;
  updateMessageStatus: (messageId: string, status: "pending" | "sent" | "failed", error?: string) => void;
  removeMessage: (messageId: string) => void;
  setSearchState: (searchState: SearchState | null) => void;
}

export const useChatStore = create<ChatStore>()(
  persist(
    (set, get) => ({
      messages: [],
      sessionId: "",
      isLoading: false,
      country: "",
      language: "",
      currency: "",
      searchInProgress: false,
      currentCategory: "",
      isSidebarOpen: true, // По умолчанию развернута
      _hasInitialized: false,
      savedSearch: null,
      showSavedSearchPrompt: false,
      _wsSender: null,

      // New fields initialization
      lastMessageTimestamp: null,
      rateLimitState: {
        isLimited: false,
        reason: null,
        retryAfter: null,
        expiresAt: null,
      },
      signedSessionId: null,
      searchState: null,

      addMessage: (message) =>
        set((state) => {
          // Check for duplicate message IDs to prevent duplicate messages
          const isDuplicate = state.messages.some((m) => m.id === message.id);
          if (isDuplicate) {
            return state; // No state change
          }

          const newMessages = [...state.messages, message];
          return {
            messages: newMessages,
            lastMessageTimestamp: new Date(),
          };
        }),

      setMessages: (messages) => {
        set({ messages });
      },

      setSessionId: (id) =>
        set((state) => ({
          sessionId: id,
          // Clear signed session ID when base session changes to avoid using stale signatures
          signedSessionId: state.sessionId !== id ? null : state.signedSessionId,
        })),

      setLoading: (loading) => set({ isLoading: loading }),

      setCountry: (country) => {
        const currency = getCurrencyForCountry(country.toUpperCase());
        set({ country, currency });
      },

      setLanguage: (language) => set({ language }),

      setCurrency: (currency) => set({ currency }),

      setSearchInProgress: (inProgress) => set({ searchInProgress: inProgress }),

      setCurrentCategory: (category) => set({ currentCategory: category }),

      clearMessages: () => set({ messages: [], isLoading: false, currentCategory: "" }),

      clearAll: () => {
        // Clear all chat data (for logout)
        localStorage.removeItem("chat_session_id");
        set({
          messages: [],
          sessionId: "",
          isLoading: false,
          searchInProgress: false,
          currentCategory: "",
          savedSearch: null,
          _hasInitialized: false,
        });
      },

      newSearch: () => {
        set({
          messages: [],
          searchInProgress: false,
          isLoading: false,
          currentCategory: "",
          signedSessionId: null, // Clear signed session ID for new session
          showSavedSearchPrompt: false, // Clear saved search prompt to show welcome message
        });
      },

      initializeLocale: async () => {
        const state = get();
        // Only initialize if country is not already set (either from localStorage or detection)
        if (!state.country) {
          const country = await detectCountry();
          const currency = getCurrencyForCountry(country);
          set({ country, currency });
        } else if (!state.currency) {
          // If country exists but currency doesn't (migration case)
          const currency = getCurrencyForCountry(state.country);
          set({ currency });
        }
        if (!state.language) {
          set({ language: detectLanguage() });
        }
      },

      loadSessionMessages: async (sessionId: string) => {
        if (!sessionId) {
          return;
        }

        try {
          const { getSessionMessages } = await import("./api");
          const response = await getSessionMessages(sessionId);

          // Handle case where session is new and has no messages yet
          if (!response.messages || response.messages.length === 0) {
            set({
              messages: [],
              showSavedSearchPrompt: false, // Clear prompt for empty session to show welcome
            });
            return;
          }

          if (response.messages && response.messages.length > 0) {
            const chatMessages: ChatMessage[] = response.messages.map((msg, index) => ({
              // Use message ID from backend if available, fallback to session-based ID
              id: msg.id || `${sessionId}-${index}`,
              role: msg.role as "user" | "assistant",
              content: msg.content,
              timestamp: msg.timestamp ? new Date(msg.timestamp).getTime() : Date.now(),
              quick_replies: msg.quick_replies,
              products: msg.products,
              search_type: msg.search_type,
              isLocal: true, // Messages loaded from session are considered local (already sent)
            }));

            // Deduplicate messages by ID before setting
            const uniqueMessages = chatMessages.reduce((acc, msg) => {
              const isDuplicate = acc.some((m) => m.id === msg.id);
              if (!isDuplicate) {
                acc.push(msg);
              }
              return acc;
            }, [] as ChatMessage[]);

            set({ messages: uniqueMessages });

            // Restore search state from server response
            let hasActiveSearch = false;
            let category = "";

            if (response.search_state) {
              category = response.search_state.category || "";
              hasActiveSearch = response.search_state.status === "completed";
            }

            // Also check if the last message has products - if so, consider it an active search
            // This ensures products are displayed when reopening a chat with search results
            if (!hasActiveSearch) {
              for (let i = chatMessages.length - 1; i >= 0; i--) {
                const msg = chatMessages[i];
                if (msg.products && msg.products.length > 0) {
                  hasActiveSearch = true;
                  // If we don't have a category from search_state, try to get it from message
                  if (!category && msg.search_type) {
                    category = msg.search_type;
                  }
                  break;
                }
              }
            }

            set({
              currentCategory: category,
              searchInProgress: hasActiveSearch,
            });
          }
        } catch (error) {
          // This is expected for new sessions that don't exist on server yet
          // Don't throw - this is not critical if it's a new session
          // Just keep current state and let user start fresh
          set({
            messages: [],
            showSavedSearchPrompt: false, // Clear prompt to show welcome message
          });
        }
      },

      toggleSidebar: () => {
        set((state) => ({ isSidebarOpen: !state.isSidebarOpen }));
      },

      setSidebarOpen: (open) => {
        set({ isSidebarOpen: open });
      },

      saveCurrentSearch: async () => {
        const state = get();

        // Don't save if there are no messages or no user messages
        if (state.messages.length === 0) {
          return;
        }

        const hasUserMessages = state.messages.some(msg => msg.role === "user");
        if (!hasUserMessages) {
          return;
        }

        const savedSearchData = {
          messages: [...state.messages],
          sessionId: state.sessionId,
          category: state.currentCategory,
          timestamp: Date.now(),
        };

        set({ savedSearch: savedSearchData });

        // Realtime sync to other devices via WebSocket
        if (state._wsSender) {
          const { useAuthStore } = await import("./auth-store");
          const accessToken = useAuthStore.getState().accessToken;
          if (accessToken) {
            // Convert to backend format
            const backendFormat = {
              session_id: savedSearchData.sessionId,
              category: savedSearchData.category,
              timestamp: savedSearchData.timestamp,
              messages: savedSearchData.messages.map(msg => ({
                id: msg.id,
                role: msg.role,
                content: msg.content,
                timestamp: msg.timestamp,
                quick_replies: msg.quick_replies,
                products: msg.products,
                search_type: msg.search_type,
              })),
            };

            state._wsSender({
              type: "sync_saved_search",
              session_id: state.sessionId,
              access_token: accessToken,
              saved_search: backendFormat,
            });
          }
        }
      },

      restoreSavedSearch: async () => {
        const state = get();
        if (state.savedSearch) {
          // Check if saved search has products to set searchInProgress correctly
          const hasProducts = state.savedSearch.messages.some(
            m => m.products && m.products.length > 0
          );

          set({
            messages: [...state.savedSearch.messages],
            sessionId: state.savedSearch.sessionId,
            currentCategory: state.savedSearch.category,
            searchInProgress: hasProducts, // Set based on presence of products
            isLoading: false,
          });
          // Save session ID to localStorage
          localStorage.setItem("chat_session_id", state.savedSearch.sessionId);

          // Realtime sync to other devices
          if (state._wsSender) {
            const { useAuthStore } = await import("./auth-store");
            const accessToken = useAuthStore.getState().accessToken;
            if (accessToken) {
              state._wsSender({
                type: "sync_session",
                session_id: state.savedSearch.sessionId,
                access_token: accessToken,
              });
            }
          }
        }
      },

      clearSavedSearch: async () => {
        const state = get();
        set({ savedSearch: null });

        // Realtime sync to other devices (send null to clear)
        if (state._wsSender) {
          const { useAuthStore } = await import("./auth-store");
          const accessToken = useAuthStore.getState().accessToken;
          if (accessToken) {
            state._wsSender({
              type: "sync_saved_search",
              session_id: state.sessionId,
              access_token: accessToken,
              saved_search: null, // Clear saved search on server
            });
          }
        }
      },

      syncPreferencesFromServer: async () => {
        try {
          const { PreferencesAPI } = await import("./preferences-api");
          const response = await PreferencesAPI.getUserPreferences();

          if (response.has_preferences && response.preferences) {
            const prefs = response.preferences;
            const updates: Partial<ChatStore> = {};

            // Only update if server has value (don't override local with null)
            if (prefs.country) updates.country = prefs.country;
            if (prefs.currency) updates.currency = prefs.currency;
            if (prefs.language) updates.language = prefs.language;

            // Sync saved_search from server
            if (prefs.saved_search !== undefined) {
              if (prefs.saved_search === null) {
                updates.savedSearch = null;
              } else {
                // Convert from server format to local format
                updates.savedSearch = {
                  sessionId: prefs.saved_search.session_id,
                  category: prefs.saved_search.category,
                  timestamp: prefs.saved_search.timestamp,
                  messages: prefs.saved_search.messages.map((msg: any) => ({
                    id: msg.id,
                    role: msg.role as "user" | "assistant",
                    content: msg.content,
                    timestamp: msg.timestamp,
                    quick_replies: msg.quick_replies,
                    products: msg.products,
                    search_type: msg.search_type,
                  })),
                };
              }
            }

            if (Object.keys(updates).length > 0) {
              set(updates);
            }
          }
        } catch (error) {
          console.error("Failed to sync preferences from server:", error);
        }
      },

      syncPreferencesToServer: async () => {
        try {
          const state = get();
          const { PreferencesAPI } = await import("./preferences-api");

          const update = {
            country: state.country || undefined,
            currency: state.currency || undefined,
            language: state.language || undefined,
          };

          await PreferencesAPI.updateUserPreferences(update);
        } catch (error) {
          console.error("Failed to sync preferences to server:", error);
        }
      },

      registerWebSocketSender: (sender) => {
        set({ _wsSender: sender });
      },

      setShowSavedSearchPrompt: (show) => {
        set({ showSavedSearchPrompt: show });
      },

      checkSavedSearchPrompt: () => {
        const state = get();

        // Only show prompt if:
        // 1. There is a savedSearch
        // 2. Current chat is empty (no messages)
        // 3. SavedSearch has messages but no products
        // 4. SavedSearch was created more than 10 seconds ago (avoid showing after "New Search")
        // 5. SavedSearch is not too old (< 24 hours)
        // 6. SavedSearch is from a DIFFERENT session (don't show for current session)
        if (state.savedSearch &&
            state.messages.length === 0 &&
            state.savedSearch.messages.length > 0) {

          // Don't show if savedSearch is from the CURRENT session
          // This prevents showing prompt when we're already in the saved session
          if (state.savedSearch.sessionId === state.sessionId) {
            set({ showSavedSearchPrompt: false }); // Explicitly set to false
            return;
          }

          const timeSinceSaved = Date.now() - state.savedSearch.timestamp;

          // Don't show prompt if savedSearch was just created (< 10 seconds ago)
          if (timeSinceSaved < 10000) {
            return;
          }

          // Don't show prompt if savedSearch is too old (> 24 hours)
          // User probably doesn't care about old searches anymore
          const MAX_AGE = 24 * 60 * 60 * 1000; // 24 hours
          if (timeSinceSaved > MAX_AGE) {
            set({ savedSearch: null, showSavedSearchPrompt: false });
            return;
          }

          const hasProducts = state.savedSearch.messages.some(
            m => m.products && m.products.length > 0
          );

          // Show prompt only if savedSearch has NO products
          if (!hasProducts) {
            set({ showSavedSearchPrompt: true });
          }
        } else {
          // Ensure prompt is cleared if conditions not met
          if (state.showSavedSearchPrompt) {
            set({ showSavedSearchPrompt: false });
          }
        }
      },

      // New methods for Priority 1 features
      setLastMessageTimestamp: (timestamp) => {
        set({ lastMessageTimestamp: timestamp });
      },

      setRateLimitState: (state) => {
        set((currentState) => ({
          rateLimitState: {
            ...currentState.rateLimitState,
            ...state,
          },
        }));
      },

      clearRateLimitState: () => {
        set({
          rateLimitState: {
            isLimited: false,
            reason: null,
            retryAfter: null,
            expiresAt: null,
          },
        });
      },

      setSignedSessionId: (signedSessionId) => {
        set({ signedSessionId });
      },

      updateMessageStatus: (messageId, status, error) => {
        set((state) => ({
          messages: state.messages.map((msg) =>
            msg.id === messageId ? { ...msg, status, error } : msg
          ),
        }));
      },

      removeMessage: (messageId) => {
        set((state) => ({
          messages: state.messages.filter((msg) => msg.id !== messageId),
        }));
      },

      setSearchState: (searchState) => {
        set({ searchState });
      },
    }),
    {
      name: "chat-storage",
      storage: createJSONStorage(() => localStorage),
      partialize: (state) => ({
        country: state.country,
        language: state.language,
        currency: state.currency,
        isSidebarOpen: state.isSidebarOpen,
        messages: state.messages,
        sessionId: state.sessionId,
        currentCategory: state.currentCategory,
        searchInProgress: state.searchInProgress,
        savedSearch: state.savedSearch,
        // Exclude _wsSender from persistence
      }),
      onRehydrateStorage: () => {
        return (state, error) => {
          if (error) {
            console.error("❌ Error rehydrating chat store:", error);
          }
        };
      },
    }
  )
);