"use client";

import { useState } from "react";
import { Send } from "lucide-react";
import { CountrySelector } from "@/shared/components/ui";
import { useChatStore } from "@/shared/lib";

interface ChatInputProps {
  onSend: (message: string) => void;
  isLoading: boolean;
  isConnected: boolean;
  connectionStatus: string;
}

export function ChatInput({
  onSend,
  isLoading,
  isConnected,
  connectionStatus,
}: ChatInputProps) {
  const [input, setInput] = useState("");
  const { rateLimitState } = useChatStore();

  const handleSend = () => {
    const trimmedInput = input.trim();
    if (trimmedInput && !rateLimitState.isLimited) {
      onSend(trimmedInput);
      setInput("");
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  // Calculate time remaining for rate limit
  const getTimeRemaining = () => {
    if (!rateLimitState.expiresAt) return 0;
    return Math.ceil((rateLimitState.expiresAt.getTime() - Date.now()) / 1000);
  };

  // Determine if input should be disabled
  const isDisabled = rateLimitState.isLimited || !isConnected || isLoading;

  // Get appropriate placeholder text
  const getPlaceholder = () => {
    if (rateLimitState.isLimited) {
      const timeRemaining = getTimeRemaining();
      return `Rate limit exceeded. Retry in ${timeRemaining}s`;
    }
    if (!isConnected) {
      return `${connectionStatus}...`;
    }
    return "Type your message...";
  };

  return (
    <div className="w-full max-w-3xl mx-auto px-2 py-4 pb-2">
      <div className="flex flex-col px-4 py-2 rounded-xl border bg-secondary border-border focus-within:border-primary transition-colors">
        <textarea
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={getPlaceholder()}
          disabled={isDisabled}
          rows={1}
          className="flex-1 px-2 py-3 bg-transparent focus:outline-none disabled:opacity-50 disabled:cursor-not-allowed resize-none text-base min-h-11 max-h-32 overflow-y-auto"
          style={{ fieldSizing: 'content' } as React.CSSProperties}
        />
       <div className="flex justify-between items-center">
        
        <CountrySelector />
        <button
          onClick={handleSend}
          disabled={!input.trim() || isDisabled}
          className="w-9 h-9 md:w-7.5 md:h-7.5 rounded-md bg-primary text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer flex items-center justify-center shrink-0"
        >
          <Send className="w-5 h-5 md:w-4 md:h-4" />
        </button>

        </div>
         
      </div>
    </div>
  );
}
