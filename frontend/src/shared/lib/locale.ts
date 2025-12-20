export async function detectCountryByIPFallback(): Promise<string | null> {
  try {
    const response = await fetch('http://ip-api.com/json/', {
      method: 'GET',
    });

    if (!response.ok) return null;

    const data = await response.json();
    return data.countryCode || null;
  } catch (error) {
    console.error('Failed to detect country by IP-API:', error);
    return null;
  }
}

export async function detectCountryByIP(): Promise<string | null> {
  try {
    const response = await fetch('https://ipapi.co/json/', {
      method: 'GET',
      headers: { 'Accept': 'application/json' }
    });

    if (!response.ok) return null;

    const data = await response.json();
    return data.country_code || null;
  } catch (error) {
    console.error('Failed to detect country by ipapi.co:', error);
    return null;
  }
}

// Compact timezone-to-country mapping for special cases
const TIMEZONE_OVERRIDES: Record<string, string> = {
  "Europe/Zurich": "CH", "Europe/Vaduz": "LI", "Europe/Luxembourg": "LU",
  "Europe/Monaco": "MC", "Europe/San_Marino": "SM", "Europe/Vatican": "VA",
  "Europe/Andorra": "AD", "Europe/Kiev": "UA", "Europe/Kyiv": "UA",
  "Asia/Hong_Kong": "HK", "Asia/Kolkata": "IN", "Asia/Mumbai": "IN", "Asia/Delhi": "IN",
  "Pacific/Honolulu": "US", "Pacific/Guam": "GU",
};

// Extract country code from timezone (e.g., "Europe/Berlin" -> "DE")
function extractCountryFromTimezone(timezone: string): string | null {
  // Check overrides first
  if (TIMEZONE_OVERRIDES[timezone]) return TIMEZONE_OVERRIDES[timezone];

  // Extract city name and try to match ISO country codes
  const parts = timezone.split("/");
  if (parts.length < 2) return null;

  const cityMap: Record<string, string> = {
    Berlin: "DE", Vienna: "AT", Paris: "FR", Rome: "IT", Madrid: "ES",
    Lisbon: "PT", Amsterdam: "NL", Brussels: "BE", Warsaw: "PL", Prague: "CZ",
    Stockholm: "SE", Oslo: "NO", Copenhagen: "DK", Helsinki: "FI", London: "GB",
    Dublin: "IE", Athens: "GR", Budapest: "HU", Bucharest: "RO", Sofia: "BG",
    Vilnius: "LT", Riga: "LV", Tallinn: "EE", Ljubljana: "SI", Bratislava: "SK",
    Zagreb: "HR", Belgrade: "RS", Moscow: "RU", Istanbul: "TR", Ankara: "TR",
    Minsk: "BY", Chisinau: "MD", Sarajevo: "BA", Podgorica: "ME", Skopje: "MK",
    Tirane: "AL", Toronto: "CA", Vancouver: "CA", Montreal: "CA", Halifax: "CA",
    Winnipeg: "CA", Tokyo: "JP", Seoul: "KR", Shanghai: "CN", Beijing: "CN",
    Taipei: "TW", Singapore: "SG", Bangkok: "TH", Ho_Chi_Minh: "VN", Jakarta: "ID",
    Manila: "PH", Kuala_Lumpur: "MY", Dhaka: "BD", Karachi: "PK", Dubai: "AE",
    Riyadh: "SA", Tel_Aviv: "IL", Tehran: "IR", Baghdad: "IQ", Kuwait: "KW",
    Sydney: "AU", Melbourne: "AU", Brisbane: "AU", Perth: "AU", Adelaide: "AU",
    Darwin: "AU", Hobart: "AU", Auckland: "NZ", Wellington: "NZ", Fiji: "FJ",
    Cairo: "EG", Johannesburg: "ZA", Lagos: "NG", Nairobi: "KE", Casablanca: "MA",
    Algiers: "DZ", Tunis: "TN", Tripoli: "LY", Accra: "GH", Addis_Ababa: "ET",
    Dar_es_Salaam: "TZ", Kampala: "UG",
  };

  const city = parts[parts.length - 1];
  if (cityMap[city]) return cityMap[city];

  // Generic region mapping (America/* -> US, etc.)
  const regionDefaults: Record<string, string> = {
    America: "US", Europe: "GB", Asia: "SG", Australia: "AU",
    Pacific: "NZ", Africa: "ZA", Atlantic: "PT", Indian: "MU",
  };

  return regionDefaults[parts[0]] || null;
}

// Extract country code from locale (e.g., "en-US" -> "US", "de-CH" -> "CH")
function extractCountryFromLocale(locale: string): string | null {
  const parts = locale.split("-");
  if (parts.length === 2 && parts[1].length === 2) {
    return parts[1].toUpperCase();
  }

  // Language-to-country fallback for ambiguous cases
  const langDefaults: Record<string, string> = {
    de: "DE", fr: "FR", it: "IT", es: "ES", pt: "PT", nl: "NL",
    pl: "PL", cs: "CZ", sv: "SE", no: "NO", da: "DK", fi: "FI",
    ja: "JP", ko: "KR", zh: "CN", th: "TH", vi: "VN", id: "ID",
    ar: "SA", he: "IL", tr: "TR", el: "GR", hu: "HU", ro: "RO",
    bg: "BG", hr: "HR", sk: "SK", sl: "SI", et: "EE", lv: "LV",
    lt: "LT", uk: "UA", ru: "RU", sr: "RS", sw: "KE", en: "US",
  };

  return langDefaults[parts[0]] || null;
}

export async function detectCountry(): Promise<string> {
  if (typeof window === "undefined") return "CH";

  // Try IP-based detection first
  let country = await detectCountryByIP();
  if (country) {
    return country;
  }

  country = await detectCountryByIPFallback();
  if (country) {
    return country;
  }

  // Extract from timezone
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;

  country = extractCountryFromTimezone(timezone);
  if (country) {
    return country;
  }

  // Extract from locale as last resort
  const locale = navigator.language || "en-US";

  country = extractCountryFromLocale(locale);
  if (country) {
    return country;
  }

  return "US";
}

export function detectLanguage(): string {
  if (typeof window === "undefined") return "en";

  const locale = navigator.language || "en";
  const langCode = locale.split("-")[0];

  const supportedLanguages = [
    "de", "fr", "it", "en", "es", "pt", "nl", "pl", "cs", "sv", "no", "da", "fi",
    "ja", "ko", "zh", "th", "vi", "id", "ms", "hi", "ar", "he", "tr", "el", "hu",
    "ro", "bg", "hr", "sk", "sl", "et", "lv", "lt", "uk", "ru", "sr", "sw",
    "ka", "az", "be", "bs", "ca", "cy", "eo", "eu", "fa", "fil", "ga", "gl",
    "gu", "ha", "hy", "is", "jv", "ka", "kk", "km", "kn", "ku", "ky", "lo",
    "mk", "ml", "mn", "mr", "mt", "my", "ne", "pa", "ps", "sd", "si", "so",
    "sq", "ta", "te", "tg", "tk", "tl", "ur", "uz", "xh", "yi", "yo", "zu",
    "af", "am", "bn", "ceb", "co", "fy", "gd", "haw", "hmn", "ig", "iw", "jw",
    "kn", "la", "lb", "mg", "mi", "ny", "or", "sm", "sn", "st", "su", "ti"
  ];

  if (supportedLanguages.includes(langCode)) {
    return langCode;
  }

  return "en";
}

export function getCurrencyForCountry(country: string): string {
  // Eurozone countries
  const eurozoneCountries = new Set([
    "DE", "AT", "FR", "IT", "ES", "PT", "NL", "BE", "FI", "IE", "GR",
    "HR", "SK", "SI", "EE", "LV", "LT", "CY", "MT", "LU",
  ]);

  if (eurozoneCountries.has(country)) return "EUR";

  // Non-EUR currencies
  const currencyMap: Record<string, string> = {
    CH: "CHF", PL: "PLN", CZ: "CZK", SE: "SEK", NO: "NOK", DK: "DKK", GB: "GBP",
    US: "USD", CA: "CAD", MX: "MXN", BR: "BRL", AR: "ARS", CL: "CLP", CO: "COP",
    PE: "PEN", VE: "VES", PA: "PAB", CU: "CUP", JP: "JPY", KR: "KRW", CN: "CNY",
    HK: "HKD", TW: "TWD", SG: "SGD", TH: "THB", VN: "VND", ID: "IDR", PH: "PHP",
    MY: "MYR", IN: "INR", AE: "AED", SA: "SAR", IL: "ILS", TR: "TRY", AU: "AUD",
    NZ: "NZD", EG: "EGP", ZA: "ZAR", NG: "NGN", KE: "KES", MA: "MAD", DZ: "DZD",
    TN: "TND", HU: "HUF", RO: "RON", BG: "BGN", UA: "UAH", RU: "RUB", RS: "RSD",
  };

  return currencyMap[country] || "USD";
}