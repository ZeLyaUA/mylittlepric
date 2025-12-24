"use client";

import { useRef } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { ChatMessage as ChatMessageType } from "@/shared/types";
import { useAuthStore } from "@/shared/lib";
import { ProductCard, ProductTable } from "@/features/products";



// Helper function to generate initials from user's name or email
function getInitials(user: { full_name?: string; email: string } | null): string {
  if (!user) return "U";

  if (user.full_name) {
    const names = user.full_name.trim().split(/\s+/);
    if (names.length >= 2) {
      return (names[0][0] + names[names.length - 1][0]).toUpperCase();
    }
    return names[0][0].toUpperCase();
  }

  return user.email[0].toUpperCase();
}

interface ChatMessageProps {
  message: ChatMessageType;
  onQuickReply: (reply: string) => void;
  onRetry?: (messageId: string) => void;
}

// Parse quick reply to separate text and price
function parseQuickReply(reply: string): { text: string; price: string | null } {
  // Match various price patterns:
  // - "Option (≈CHF 100-200)"
  // - "Option (CHF 100–200)"
  // - "Option (≈$100)"
  // - "Option (CHF 500–1500+)"
  // - "Option (≈$100-200k)"
  // Support various dash types: - – — (hyphen, en-dash, em-dash)
  const priceMatch = reply.match(/\(([≈~]?[A-Z$€£¥]{1,4}[\s]?[\d,.\-–—]+[\+]?(?:[\s]?[kK]|[\s]?[\-–—][\s]?[\d,.\-–—]+[\+]?(?:[kK])?)?)\)$/);

  if (priceMatch) {
    const text = reply.substring(0, priceMatch.index).trim();
    const price = priceMatch[1];
    return { text, price };
  }

  return { text: reply, price: null };
}

export function ChatMessage({ message, onQuickReply, onRetry }: ChatMessageProps) {
  const isUser = message.role === "user";
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const { user } = useAuthStore();
  const isPending = message.status === "pending";
  const isFailed = message.status === "failed";

  const scroll = (direction: 'left' | 'right') => {
    if (scrollContainerRef.current) {
      const scrollAmount = 224; // Width of card (210px) + gap (14px)
      const newScrollLeft = direction === 'left'
        ? scrollContainerRef.current.scrollLeft - scrollAmount
        : scrollContainerRef.current.scrollLeft + scrollAmount;

      scrollContainerRef.current.scrollTo({
        left: newScrollLeft,
        behavior: 'smooth'
      });
    }
  };

  return (
    <div>
      <div
        className={`${message.products && message.products.length > 0 ? 'w-full' : 'max-w-3xl'} space-y-3`}
      >
        {message.content && message.content.trim() !== '' && message.content.trim() !== '...' && (
          <div
            className={`${
              isUser
                ? `rounded-2xl px-4 py-3 bg-secondary text-secondary-foreground flex items-start gap-3 ${isFailed ? 'opacity-60' : ''} ${isPending ? 'opacity-80' : ''}`
                : "text-foreground ml-3"
            }`}
          >
            {isUser && (
              <div className="shrink-0 w-9 h-9 rounded-full bg-primary text-primary-foreground flex items-center justify-center font-semibold text-sm self-start">
                {getInitials(user)}
              </div>
            )}
            <div className={isUser ? "min-w-0 pt-1.5" : ""}>
              <p className="whitespace-pre-wrap wrap-break-word">{message.content}</p>

              {/* Status indicators for user messages */}
              {isUser && (isPending || isFailed) && (
                <div className="flex items-center gap-1 mt-2 text-xs">
                  {isPending && (
                    <div className="flex items-center gap-1 text-muted-foreground">
                      <div className="w-2 h-2 rounded-full bg-current animate-pulse" />
                      <span>Sending...</span>
                    </div>
                  )}

                  {isFailed && (
                    <div className="flex items-center gap-2 text-red-500 dark:text-red-400">
                      <svg className="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
                        <path
                          fillRule="evenodd"
                          d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
                          clipRule="evenodd"
                        />
                      </svg>
                      <span>Failed to send</span>
                      {onRetry && (
                        <button
                          onClick={() => onRetry(message.id)}
                          className="underline hover:no-underline ml-1 font-medium cursor-pointer"
                        >
                          Retry
                        </button>
                      )}
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        )}

        {message.quick_replies && message.quick_replies.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {message.quick_replies.map((reply, index) => {
              const { text, price } = parseQuickReply(reply);

              return (
                <button
                  key={index}
                  onClick={() => onQuickReply(reply)}
                  className="px-3 py-1.5 rounded-lg bg-secondary hover:bg-secondary/80 text-sm border border-border/50 hover:border-primary/30 flex items-center gap-2 cursor-pointer"
                >
                  <span className="font-medium text-foreground">
                    {text}
                  </span>

                  {price && (
                    <span className="text-xs px-2 py-0.5 rounded-full bg-primary/15 text-primary font-bold">
                      {price}
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        )}

        {message.products && message.products.length > 0 && (
          <ProductTable products={message.products} description={message.product_description} />
        )}
      </div>
    </div>
  );
}