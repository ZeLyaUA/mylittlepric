"use client";

import { useState, useRef, useEffect, useMemo } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft, Globe, Languages, Check, ChevronDown, Moon, Sun, Monitor, Lock, Eye, EyeOff } from "lucide-react";
import { usePreferences, usePreferenceActions, getCurrencyForCountry, useAuthStore } from "@/shared/lib";
import { useTheme } from "next-themes";
import { COUNTRIES, LANGUAGES } from "@/shared/constants";
import { AuthAPI } from "@/shared/lib/api/auth";

export default function SettingsPage() {
  const router = useRouter();
  const { country, language, currency } = usePreferences();
  const { setCountry, setLanguage, setCurrency, syncPreferencesToServer } = usePreferenceActions();
  const { accessToken, user, isAuthenticated, _hasHydrated } = useAuthStore();
  const { theme, setTheme } = useTheme();

  // Redirect to login if not authenticated
  useEffect(() => {
    if (_hasHydrated && !isAuthenticated) {
      router.push("/login?redirect=/settings");
    }
  }, [_hasHydrated, isAuthenticated, router]);
  const [mounted, setMounted] = useState(false);
  const [countrySearchQuery, setCountrySearchQuery] = useState("");
  const [languageSearchQuery, setLanguageSearchQuery] = useState("");
  const [isCountryDropdownOpen, setIsCountryDropdownOpen] = useState(false);
  const [isLanguageDropdownOpen, setIsLanguageDropdownOpen] = useState(false);

  const countrySearchRef = useRef<HTMLInputElement>(null);
  const languageSearchRef = useRef<HTMLInputElement>(null);

  // Password change state
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [showCurrentPassword, setShowCurrentPassword] = useState(false);
  const [showNewPassword, setShowNewPassword] = useState(false);
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);
  const [passwordChangeError, setPasswordChangeError] = useState("");
  const [passwordChangeSuccess, setPasswordChangeSuccess] = useState("");
  const [isChangingPassword, setIsChangingPassword] = useState(false);

  // Check if user is using email provider (not Google OAuth)
  const isEmailUser = user?.provider === "email";

  const selectedCountry = useMemo(
    () => COUNTRIES.find((c) => c.code === country.toLowerCase()) || COUNTRIES[0],
    [country]
  );

  const selectedLanguage = useMemo(
    () => LANGUAGES.find((l) => l.code === language.toLowerCase()) || LANGUAGES[0],
    [language]
  );

  const filteredCountries = useMemo(
    () => COUNTRIES.filter(
      (c) =>
        c.name.toLowerCase().includes(countrySearchQuery.toLowerCase()) ||
        c.code.toLowerCase().includes(countrySearchQuery.toLowerCase())
    ),
    [countrySearchQuery]
  );

  const filteredLanguages = useMemo(
    () => LANGUAGES.filter(
      (l) =>
        l.name.toLowerCase().includes(languageSearchQuery.toLowerCase()) ||
        l.nativeName.toLowerCase().includes(languageSearchQuery.toLowerCase()) ||
        l.code.toLowerCase().includes(languageSearchQuery.toLowerCase())
    ),
    [languageSearchQuery]
  );

  // Set mounted state for theme
  useEffect(() => {
    setMounted(true);
  }, []);

  // Focus search input when dropdown opens
  useEffect(() => {
    if (isCountryDropdownOpen) {
      setTimeout(() => countrySearchRef.current?.focus(), 100);
    }
  }, [isCountryDropdownOpen]);

  useEffect(() => {
    if (isLanguageDropdownOpen) {
      setTimeout(() => languageSearchRef.current?.focus(), 100);
    }
  }, [isLanguageDropdownOpen]);

  const handleCountrySelect = async (countryCode: string) => {
    // Optimistic UI update - update immediately without waiting for server
    setCountry(countryCode);
    const newCurrency = getCurrencyForCountry(countryCode.toUpperCase());
    setCurrency(newCurrency);
    setIsCountryDropdownOpen(false);
    setCountrySearchQuery("");

    // Sync to server in background (non-blocking)
    if (accessToken) {
      syncPreferencesToServer().catch((error) => {
        console.error("Failed to sync country preference:", error);
        // Could show a toast notification here if needed
      });
    }
  };

  const handleLanguageSelect = async (languageCode: string) => {
    // Optimistic UI update - update immediately without waiting for server
    setLanguage(languageCode);
    setIsLanguageDropdownOpen(false);
    setLanguageSearchQuery("");

    // Sync to server in background (non-blocking)
    if (accessToken) {
      syncPreferencesToServer().catch((error) => {
        console.error("Failed to sync language preference:", error);
        // Could show a toast notification here if needed
      });
    }
  };

  const handleThemeChange = async (newTheme: string) => {
    // Optimistic UI update - apply theme immediately
    setTheme(newTheme);

    // Sync theme to server in background (non-blocking)
    if (accessToken) {
      import("@/shared/lib/preferences-api")
        .then(({ PreferencesAPI }) => PreferencesAPI.updateUserPreferences({ theme: newTheme }))
        .catch((error) => {
          console.error("Failed to sync theme preference:", error);
          // Could show a toast notification here if needed
        });
    }
  };

  const handleBack = () => {
    router.back();
  };

  const handlePasswordChange = async (e: React.FormEvent) => {
    e.preventDefault();
    setPasswordChangeError("");
    setPasswordChangeSuccess("");

    // Validation
    if (newPassword.length < 8) {
      setPasswordChangeError("New password must be at least 8 characters");
      return;
    }

    if (newPassword !== confirmPassword) {
      setPasswordChangeError("New passwords do not match");
      return;
    }

    if (!accessToken) {
      setPasswordChangeError("You must be logged in to change password");
      return;
    }

    setIsChangingPassword(true);

    try {
      await AuthAPI.changePassword(accessToken, currentPassword, newPassword);
      setPasswordChangeSuccess("Password changed successfully");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    } catch (error) {
      setPasswordChangeError(error instanceof Error ? error.message : "Failed to change password");
    } finally {
      setIsChangingPassword(false);
    }
  };

  return (
    <div className="min-h-screen bg-background">
      {/* Header */}
      <header className="sticky top-0 z-10 border-b border-border bg-background">
        <div className="container mx-auto px-4 h-16 flex items-center gap-4">
          <button
            onClick={handleBack}
            className="p-2 hover:bg-secondary rounded-lg transition-colors cursor-pointer"
            aria-label="Go back"
          >
            <ArrowLeft className="w-5 h-5" />
          </button>
          <h1 className="text-2xl font-semibold text-foreground">Settings</h1>
        </div>
      </header>

      {/* Content */}
      <main className="container mx-auto px-4 py-8 max-w-3xl">
        <div className="space-y-8">
          {/* Regional Settings Section */}
          <div className="space-y-4">
            <div className="flex items-center gap-2">
              <Globe className="w-5 h-5 text-primary" />
              <h2 className="text-lg font-semibold text-foreground">Regional Settings</h2>
            </div>

            <div className="space-y-4 pl-7">
              {/* Country Selection */}
              <div className="space-y-2">
                <label className="text-sm font-medium text-muted-foreground">
                  Country/Region
                </label>
                <div className="relative">
                  <button
                    type="button"
                    onClick={() => setIsCountryDropdownOpen(!isCountryDropdownOpen)}
                    className="w-full flex items-center justify-between px-4 py-3 bg-secondary hover:bg-secondary/80 border border-border rounded-lg transition-colors cursor-pointer"
                  >
                    <div className="flex items-center gap-3">
                      <span className="text-2xl emoji-flag">{selectedCountry.flag}</span>
                      <span className="text-sm font-medium">{selectedCountry.name}</span>
                    </div>
                    <ChevronDown className={`w-4 h-4 transition-transform ${isCountryDropdownOpen ? 'rotate-180' : ''}`} />
                  </button>

                  {isCountryDropdownOpen && (
                    <div className="absolute top-full left-0 right-0 mt-2 bg-background border border-border rounded-lg shadow-xl overflow-hidden z-50">
                      {/* Search Input */}
                      <div className="p-3 border-b border-border">
                        <input
                          ref={countrySearchRef}
                          type="text"
                          value={countrySearchQuery}
                          onChange={(e) => setCountrySearchQuery(e.target.value)}
                          placeholder="Search countries..."
                          className="w-full px-3 py-2 rounded-md bg-secondary border border-border focus:border-primary focus:outline-none transition-colors text-sm"
                        />
                      </div>

                      {/* Countries List */}
                      <div className="max-h-64 overflow-y-auto">
                        {filteredCountries.length > 0 ? (
                          filteredCountries.map((c) => (
                            <button
                              key={c.code}
                              onClick={() => handleCountrySelect(c.code)}
                              className={`w-full px-4 py-2.5 flex items-center justify-between hover:bg-secondary transition-colors text-left cursor-pointer ${
                                c.code === country.toLowerCase() ? "bg-secondary/50" : ""
                              }`}
                            >
                              <div className="flex items-center gap-3">
                                <span className="text-xl emoji-flag">{c.flag}</span>
                                <span className="text-sm font-medium">{c.name}</span>
                              </div>
                              {c.code === country.toLowerCase() && (
                                <Check className="w-4 h-4 text-primary" />
                              )}
                            </button>
                          ))
                        ) : (
                          <div className="px-4 py-8 text-center text-sm text-muted-foreground">
                            No countries found
                          </div>
                        )}
                      </div>
                    </div>
                  )}
                </div>
                <p className="text-xs text-muted-foreground">
                  Currency: {currency}
                </p>
              </div>
            </div>
          </div>

          {/* Language Settings Section */}
          <div className="space-y-4">
            <div className="flex items-center gap-2">
              <Languages className="w-5 h-5 text-primary" />
              <h2 className="text-lg font-semibold text-foreground">Language Settings</h2>
            </div>

            <div className="space-y-4 pl-7">
              {/* Language Selection */}
              <div className="space-y-2">
                <label className="text-sm font-medium text-muted-foreground">
                  Agent Communication Language
                </label>
                <p className="text-xs text-muted-foreground">
                  Choose the language for the AI assistant to communicate with you
                </p>
                <div className="relative">
                  <button
                    type="button"
                    onClick={() => setIsLanguageDropdownOpen(!isLanguageDropdownOpen)}
                    className="w-full flex items-center justify-between px-4 py-3 bg-secondary hover:bg-secondary/80 border border-border rounded-lg transition-colors cursor-pointer"
                  >
                    <div className="flex items-center gap-3">
                      <Languages className="w-5 h-5 text-muted-foreground" />
                      <div className="flex flex-col items-start">
                        <span className="text-sm font-medium">{selectedLanguage.name}</span>
                        <span className="text-xs text-muted-foreground">{selectedLanguage.nativeName}</span>
                      </div>
                    </div>
                    <ChevronDown className={`w-4 h-4 transition-transform ${isLanguageDropdownOpen ? 'rotate-180' : ''}`} />
                  </button>

                  {isLanguageDropdownOpen && (
                    <div className="absolute top-full left-0 right-0 mt-2 bg-background border border-border rounded-lg shadow-xl overflow-hidden z-50">
                      {/* Search Input */}
                      <div className="p-3 border-b border-border">
                        <input
                          ref={languageSearchRef}
                          type="text"
                          value={languageSearchQuery}
                          onChange={(e) => setLanguageSearchQuery(e.target.value)}
                          placeholder="Search languages..."
                          className="w-full px-3 py-2 rounded-md bg-secondary border border-border focus:border-primary focus:outline-none transition-colors text-sm"
                        />
                      </div>

                      {/* Languages List */}
                      <div className="max-h-64 overflow-y-auto">
                        {filteredLanguages.length > 0 ? (
                          filteredLanguages.map((l) => (
                            <button
                              key={l.code}
                              onClick={() => handleLanguageSelect(l.code)}
                              className={`w-full px-4 py-2.5 flex items-center justify-between hover:bg-secondary transition-colors text-left cursor-pointer ${
                                l.code === language.toLowerCase() ? "bg-secondary/50" : ""
                              }`}
                            >
                              <div className="flex flex-col">
                                <span className="text-sm font-medium">{l.name}</span>
                                <span className="text-xs text-muted-foreground">{l.nativeName}</span>
                              </div>
                              {l.code === language.toLowerCase() && (
                                <Check className="w-4 h-4 text-primary" />
                              )}
                            </button>
                          ))
                        ) : (
                          <div className="px-4 py-8 text-center text-sm text-muted-foreground">
                            No languages found
                          </div>
                        )}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>

          {/* Appearance Settings Section */}
          <div className="space-y-4">
            <div className="flex items-center gap-2">
              <Monitor className="w-5 h-5 text-primary" />
              <h2 className="text-lg font-semibold text-foreground">Appearance</h2>
            </div>

            <div className="space-y-4 pl-7">
              {/* Theme Selection */}
              <div className="space-y-2">
                <label className="text-sm font-medium text-muted-foreground">
                  Theme
                </label>
                <p className="text-xs text-muted-foreground">
                  Choose your preferred color scheme
                </p>
                {mounted && (
                  <div className="grid grid-cols-3 gap-3 pt-2">
                    <button
                      onClick={() => handleThemeChange("light")}
                      className={`flex flex-col items-center gap-2 p-4 rounded-lg border transition-all cursor-pointer ${
                        theme === "light"
                          ? "bg-primary/10 border-primary"
                          : "bg-secondary border-border hover:border-primary/50"
                      }`}
                    >
                      <Sun className={`w-6 h-6 ${theme === "light" ? "text-primary" : "text-muted-foreground"}`} />
                      <span className="text-sm font-medium">Light</span>
                      {theme === "light" && <Check className="w-4 h-4 text-primary" />}
                    </button>

                    <button
                      onClick={() => handleThemeChange("dark")}
                      className={`flex flex-col items-center gap-2 p-4 rounded-lg border transition-all cursor-pointer ${
                        theme === "dark"
                          ? "bg-primary/10 border-primary"
                          : "bg-secondary border-border hover:border-primary/50"
                      }`}
                    >
                      <Moon className={`w-6 h-6 ${theme === "dark" ? "text-primary" : "text-muted-foreground"}`} />
                      <span className="text-sm font-medium">Dark</span>
                      {theme === "dark" && <Check className="w-4 h-4 text-primary" />}
                    </button>

                    <button
                      onClick={() => handleThemeChange("system")}
                      className={`flex flex-col items-center gap-2 p-4 rounded-lg border transition-all cursor-pointer ${
                        theme === "system"
                          ? "bg-primary/10 border-primary"
                          : "bg-secondary border-border hover:border-primary/50"
                      }`}
                    >
                      <Monitor className={`w-6 h-6 ${theme === "system" ? "text-primary" : "text-muted-foreground"}`} />
                      <span className="text-sm font-medium">System</span>
                      {theme === "system" && <Check className="w-4 h-4 text-primary" />}
                    </button>
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Security Settings Section */}
          {isEmailUser && (
            <div className="space-y-4">
              <div className="flex items-center gap-2">
                <Lock className="w-5 h-5 text-primary" />
                <h2 className="text-lg font-semibold text-foreground">Security</h2>
              </div>

              <div className="space-y-4 pl-7">
                {/* Change Password Form */}
                <div className="space-y-2">
                  <label className="text-sm font-medium text-muted-foreground">
                    Change Password
                  </label>
                  <p className="text-xs text-muted-foreground">
                    Update your password to keep your account secure
                  </p>

                  <form onSubmit={handlePasswordChange} className="space-y-4 pt-2">
                    {/* Success Message */}
                    {passwordChangeSuccess && (
                      <div className="p-3 rounded-lg bg-green-50 dark:bg-green-900/10 border border-green-200 dark:border-green-800/30">
                        <p className="text-sm text-green-800 dark:text-green-200">{passwordChangeSuccess}</p>
                      </div>
                    )}

                    {/* Error Message */}
                    {passwordChangeError && (
                      <div className="p-3 rounded-lg bg-red-50 dark:bg-red-900/10 border border-red-200 dark:border-red-800/30">
                        <p className="text-sm text-red-800 dark:text-red-200">{passwordChangeError}</p>
                      </div>
                    )}

                    {/* Current Password */}
                    <div className="space-y-2">
                      <label htmlFor="current-password" className="block text-sm font-medium">
                        Current Password
                      </label>
                      <div className="relative">
                        <input
                          id="current-password"
                          type={showCurrentPassword ? "text" : "password"}
                          value={currentPassword}
                          onChange={(e) => setCurrentPassword(e.target.value)}
                          required
                          disabled={isChangingPassword}
                          className="w-full px-3 py-2 pr-10 rounded-md bg-secondary border border-border focus:border-primary focus:outline-none transition-colors text-sm disabled:opacity-50"
                          placeholder="Enter current password"
                        />
                        <button
                          type="button"
                          onClick={() => setShowCurrentPassword(!showCurrentPassword)}
                          className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                        >
                          {showCurrentPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                        </button>
                      </div>
                    </div>

                    {/* New Password */}
                    <div className="space-y-2">
                      <label htmlFor="new-password" className="block text-sm font-medium">
                        New Password
                      </label>
                      <div className="relative">
                        <input
                          id="new-password"
                          type={showNewPassword ? "text" : "password"}
                          value={newPassword}
                          onChange={(e) => setNewPassword(e.target.value)}
                          required
                          minLength={8}
                          disabled={isChangingPassword}
                          className="w-full px-3 py-2 pr-10 rounded-md bg-secondary border border-border focus:border-primary focus:outline-none transition-colors text-sm disabled:opacity-50"
                          placeholder="Enter new password"
                        />
                        <button
                          type="button"
                          onClick={() => setShowNewPassword(!showNewPassword)}
                          className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                        >
                          {showNewPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                        </button>
                      </div>
                      <p className="text-xs text-muted-foreground">
                        Must be at least 8 characters
                      </p>
                    </div>

                    {/* Confirm Password */}
                    <div className="space-y-2">
                      <label htmlFor="confirm-password" className="block text-sm font-medium">
                        Confirm New Password
                      </label>
                      <div className="relative">
                        <input
                          id="confirm-password"
                          type={showConfirmPassword ? "text" : "password"}
                          value={confirmPassword}
                          onChange={(e) => setConfirmPassword(e.target.value)}
                          required
                          minLength={8}
                          disabled={isChangingPassword}
                          className="w-full px-3 py-2 pr-10 rounded-md bg-secondary border border-border focus:border-primary focus:outline-none transition-colors text-sm disabled:opacity-50"
                          placeholder="Confirm new password"
                        />
                        <button
                          type="button"
                          onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                          className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                        >
                          {showConfirmPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                        </button>
                      </div>
                    </div>

                    {/* Submit Button */}
                    <button
                      type="submit"
                      disabled={isChangingPassword}
                      className="w-full bg-primary hover:bg-primary/90 text-primary-foreground font-medium py-2.5 rounded-lg transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      {isChangingPassword ? "Changing Password..." : "Change Password"}
                    </button>
                  </form>
                </div>
              </div>
            </div>
          )}
        </div>
      </main>
    </div>
  );
}
