"use client";

import { useEffect, useRef } from "react";
import { ThemeProvider as NextThemesProvider } from "next-themes";
import { useAuthStore, useChatStore } from "@/shared/lib";

function PreferencesSync() {
  const { isAuthenticated, _hasHydrated } = useAuthStore();
  const syncPreferencesFromServer = useChatStore((state) => state.syncPreferencesFromServer);
  const syncingRef = useRef(false);

  useEffect(() => {
    // Only sync if user is authenticated and hydration is complete
    // Also prevent duplicate calls using ref
    if (_hasHydrated && isAuthenticated && !syncingRef.current) {
      syncingRef.current = true;
      syncPreferencesFromServer().finally(() => {
        syncingRef.current = false;
      });
    }
  }, [isAuthenticated, _hasHydrated, syncPreferencesFromServer]);

  return null;
}

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <NextThemesProvider
      attribute="class"
      defaultTheme="system"
      enableSystem={true}
      enableColorScheme={true}
      storageKey="mylittleprice-theme"
      disableTransitionOnChange={false}
    >
      <PreferencesSync />
      {children}
    </NextThemesProvider>
  );
}