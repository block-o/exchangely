import { createContext, useContext, useEffect, useState, type PropsWithChildren } from "react";

type Theme = "light" | "dark";
export type QuoteCurrency = "EUR" | "USD";

type SettingsContextValue = {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  quoteCurrency: QuoteCurrency;
  setQuoteCurrency: (currency: QuoteCurrency) => void;
};

const SettingsContext = createContext<SettingsContextValue | undefined>(undefined);

const THEME_KEY = "exchangely_theme";
const QUOTE_CURRENCY_KEY = "exchangely_quote_currency";

/**
 * SettingsProvider manages global user preferences like UI theme (dark/light)
 * and quote currency (EUR/USD) using localStorage for persistence.
 */
export function SettingsProvider({ children }: PropsWithChildren) {
  const [theme, setThemeState] = useState<Theme>("dark");
  const [quoteCurrency, setQuoteCurrencyState] = useState<QuoteCurrency>("EUR");

  // Load from local storage on mount
  useEffect(() => {
    const savedTheme = localStorage.getItem(THEME_KEY) as Theme;
    if (savedTheme === "light" || savedTheme === "dark") {
      setThemeState(savedTheme);
    }
    
    const savedCurrency = localStorage.getItem(QUOTE_CURRENCY_KEY);
    if (savedCurrency === "EUR" || savedCurrency === "USD") {
      setQuoteCurrencyState(savedCurrency);
    }
  }, []);

  // Update theme and persist
  const setTheme = (newTheme: Theme) => {
    setThemeState(newTheme);
    localStorage.setItem(THEME_KEY, newTheme);
  };

  // Update quote currency and persist
  const setQuoteCurrency = (newCurrency: QuoteCurrency) => {
    setQuoteCurrencyState(newCurrency);
    localStorage.setItem(QUOTE_CURRENCY_KEY, newCurrency);
  };

  // Apply theme to document root
  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
  }, [theme]);

  return (
    <SettingsContext.Provider value={{ theme, setTheme, quoteCurrency, setQuoteCurrency }}>
      {children}
    </SettingsContext.Provider>
  );
}

/**
 * Hook to access global user settings. Must be used within a SettingsProvider.
 */
export function useSettings() {
  const context = useContext(SettingsContext);
  if (context === undefined) {
    throw new Error("useSettings must be used within a SettingsProvider");
  }
  return context;
}
