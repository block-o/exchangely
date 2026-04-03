import { render, screen, act } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { SettingsProvider, useSettings } from "./settings";

// A mock consumer component
function SettingsConsumer() {
  const { theme, setTheme, quoteCurrency, setQuoteCurrency } = useSettings();
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <span data-testid="currency">{quoteCurrency}</span>
      <button onClick={() => setTheme("light")}>Set Light Theme</button>
      <button onClick={() => setQuoteCurrency("USDT")}>Set USDT</button>
    </div>
  );
}

describe("SettingsContext", () => {
  beforeEach(() => {
    localStorage.clear();
    // Clear dom
    document.documentElement.removeAttribute("data-theme");
  });

  it("provides default values", () => {
    render(
      <SettingsProvider>
        <SettingsConsumer />
      </SettingsProvider>
    );

    expect(screen.getByTestId("theme")).toHaveTextContent("dark");
    expect(screen.getByTestId("currency")).toHaveTextContent("EUR");
    // Verify document class
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("loads values from localStorage", () => {
    localStorage.setItem("exchangely_theme", "light");
    localStorage.setItem("exchangely_quote_currency", "USDT");

    render(
      <SettingsProvider>
        <SettingsConsumer />
      </SettingsProvider>
    );

    expect(screen.getByTestId("theme")).toHaveTextContent("light");
    expect(screen.getByTestId("currency")).toHaveTextContent("USDT");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("updates and persists theme", () => {
    render(
      <SettingsProvider>
        <SettingsConsumer />
      </SettingsProvider>
    );

    act(() => {
      screen.getByText("Set Light Theme").click();
    });

    expect(screen.getByTestId("theme")).toHaveTextContent("light");
    expect(localStorage.getItem("exchangely_theme")).toBe("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("updates and persists quote currency", () => {
    render(
      <SettingsProvider>
        <SettingsConsumer />
      </SettingsProvider>
    );

    act(() => {
      screen.getByText("Set USDT").click();
    });

    expect(screen.getByTestId("currency")).toHaveTextContent("USDT");
    expect(localStorage.getItem("exchangely_quote_currency")).toBe("USDT");
  });
});
