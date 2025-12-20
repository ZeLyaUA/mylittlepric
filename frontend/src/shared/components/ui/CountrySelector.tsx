"use client";

import { useState, useRef, useEffect, useMemo, useCallback } from "react";
import { Globe, Check, Settings } from "lucide-react";
import { usePreferences, usePreferenceActions } from "@/shared/lib";
import { useClickOutside } from "@/shared/hooks";
import { useRouter } from "next/navigation";
import { COUNTRIES, type Country } from "@/shared/constants";

// Flag component with emoji rendered using web fonts for cross-platform support
function CountryFlag({ country, size = "base" }: { country: Country; size?: "sm" | "base" | "lg" }) {
  const sizeClasses = {
    sm: "text-base w-4 h-4",
    base: "text-lg w-6 h-6",
    lg: "text-xl w-7 h-7",
  };

  return (
    <span className={`inline-flex items-center justify-center emoji-flag ${sizeClasses[size]}`}>
      {country.flag}
    </span>
  );
}

export function CountrySelector() {
  const { country } = usePreferences();
  const { setCountry } = usePreferenceActions();
  const [isOpen, setIsOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const dropdownRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const router = useRouter();

  const selectedCountry = useMemo(
    () => COUNTRIES.find((c) => c.code === country.toLowerCase()) || COUNTRIES[0],
    [country]
  );

  const filteredCountries = useMemo(
    () => COUNTRIES.filter(
      (c) =>
        c.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        c.code.toLowerCase().includes(searchQuery.toLowerCase())
    ),
    [searchQuery]
  );

  useClickOutside(
    dropdownRef,
    () => {
      setIsOpen(false);
      setSearchQuery("");
    },
    isOpen
  );

  useEffect(() => {
    if (isOpen) {
      // Focus search input when dropdown opens
      setTimeout(() => searchInputRef.current?.focus(), 100);
    }
  }, [isOpen]);

  const handleCountrySelect = useCallback((countryCode: string) => {
    // Optimistic UI update - update immediately without waiting
    setCountry(countryCode);
    setIsOpen(false);
    setSearchQuery("");
  }, [setCountry]);

  return (
    <>
      <div className="flex items-center gap-1">


        {/* Settings Icon Button */}
        <button
          type="button"
          onClick={() => router.push('/settings')}
          className="flex items-center justify-center p-2 md:p-1.5 rounded-md border border-border hover:bg-background/95 transition-colors shrink-0 cursor-pointer"
          title="Open settings"
        >
          <Settings className="w-5 h-5 md:w-4 md:h-4 text-muted-foreground" />
        </button>


                <div className="relative" ref={dropdownRef}>
          <button
            type="button"
            onClick={() => setIsOpen(!isOpen)}
            className="flex items-center gap-1.5 p-2 md:p-1.5 rounded-md border border-border hover:bg-background/95 transition-colors shrink-0 cursor-pointer"
            title="Select country"
          >
            <Globe className="w-5 h-5 md:w-4 md:h-4 text-muted-foreground" />
            <CountryFlag country={selectedCountry} size="sm" />
          </button>

      {isOpen && (
        <div className="absolute left-0 bottom-full mb-2 w-72 bg-background border border-border rounded-lg shadow-lg overflow-hidden z-50">
          {/* Search Input */}
          <div className="p-3 border-b border-border">
            <input
              ref={searchInputRef}
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search countries..."
              className="w-full px-3 py-2 rounded-md bg-secondary border border-border focus:border-primary focus:outline-none transition-colors text-base"
            />
          </div>

          {/* Countries List */}
          <div className="h-64 md:max-h-64 overflow-y-auto">
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
                    <CountryFlag country={c} size="lg" />
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
      </div>
    </>
  );
}
