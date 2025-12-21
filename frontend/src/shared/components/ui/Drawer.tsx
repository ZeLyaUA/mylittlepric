"use client";

import { ReactNode, useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";
import { useEscape } from "@/shared/hooks";

interface DrawerProps {
  isOpen: boolean;
  onClose: () => void;
  children: ReactNode;
  title?: string;
}

export function Drawer({ isOpen, onClose, children, title }: DrawerProps) {
  const [isClosing, setIsClosing] = useState(false);
  const [touchStart, setTouchStart] = useState<number | null>(null);
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  const handleClose = () => {
    setIsClosing(true);
    setTimeout(() => {
      onClose();
      setIsClosing(false);
    }, 300);
  };

  const handleTouchStart = (e: React.TouchEvent) => {
    setTouchStart(e.touches[0].clientX);
  };

  const handleTouchMove = (e: React.TouchEvent) => {
    if (touchStart === null) return;
    const touchEnd = e.touches[0].clientX;
    const diff = touchEnd - touchStart;
    // If swiped right more than 50px, close drawer
    if (diff > 50) {
      handleClose();
      setTouchStart(null);
    }
  };

  const handleTouchEnd = () => {
    setTouchStart(null);
  };

  useEscape(handleClose, isOpen);

  useEffect(() => {
    if (isOpen) {
      // Get scrollbar width before hiding it
      const scrollbarWidth = window.innerWidth - document.documentElement.clientWidth;

      // Store scrollbar width as CSS variable
      document.documentElement.style.setProperty('--scrollbar-width', `${scrollbarWidth}px`);

      // Add class to body that handles all the layout shift prevention
      document.body.classList.add('drawer-open');
    }
    return () => {
      document.body.classList.remove('drawer-open');
      document.documentElement.style.removeProperty('--scrollbar-width');
    };
  }, [isOpen]);

  if (!isOpen && !isClosing) return null;
  if (!mounted) return null;

  const drawerContent = (
    <div
      className="fixed inset-0 z-[9999] flex"
      data-drawer-portal="true"
      style={{
        display: isOpen || isClosing ? 'flex' : 'none',
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        zIndex: 9999
      }}
    >
      {/* Backdrop */}
      <div
        className={`absolute inset-0 transition-opacity duration-300 ${
          isClosing ? "opacity-0" : "opacity-100"
        }`}
        style={{
          position: 'absolute',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          backgroundColor: 'rgba(0, 0, 0, 0.5)',
          opacity: isClosing ? 0 : 1,
          zIndex: 1
        }}
        onClick={handleClose}
        aria-hidden="true"
      />

      {/* Drawer */}
      <div
        className={`ml-auto relative bg-background border-l border-border w-full max-w-xl h-full overflow-y-auto shadow-2xl transition-transform duration-300 ease-in-out ${
          isClosing ? "translate-x-full" : "translate-x-0"
        }`}
        style={{
          position: 'relative',
          zIndex: 2,
          transform: isClosing ? 'translateX(100%)' : 'translateX(0)',
          WebkitTransform: isClosing ? 'translateX(100%)' : 'translateX(0)',
          transition: 'transform 300ms ease-in-out',
          WebkitTransition: '-webkit-transform 300ms ease-in-out',
          maxWidth: '36rem',
          width: '100%',
          height: '100%',
          overflowY: 'auto',
          WebkitOverflowScrolling: 'touch',
          marginLeft: 'auto'
        } as React.CSSProperties}
        onTouchStart={handleTouchStart}
        onTouchMove={handleTouchMove}
        onTouchEnd={handleTouchEnd}
        role="dialog"
        aria-modal="true"
        aria-labelledby={title ? "drawer-title" : undefined}
      >
        {/* Header */}
        <div
          className="sticky top-0 z-10 bg-background border-b border-border p-4 flex items-center justify-between"
          style={{
            position: 'sticky',
            top: 0,
            zIndex: 10,
            backgroundColor: 'var(--color-background)'
          }}
        >
          {title && (
            <h2 id="drawer-title" className="text-lg font-semibold">
              {title}
            </h2>
          )}
          <button
            onClick={handleClose}
            className="w-10 h-10 rounded-full bg-secondary hover:bg-secondary/80 flex items-center justify-center transition-colors ml-auto cursor-pointer"
            aria-label="Close drawer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Content */}
        <div className="p-6">{children}</div>
      </div>
    </div>
  );

  return createPortal(drawerContent, document.body);
}
