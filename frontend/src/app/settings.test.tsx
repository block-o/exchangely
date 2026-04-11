import { render, screen, act } from "@testing-library/react";
import { describe, it, expect, beforeEach } from "vitest";
import { SettingsProvider, useSettings } from "./settings";

// A mock consumer component
function SettingsConsumer() {
  const { theme, setTheme, quoteCurrency, setQuoteCurrency } = useSettings();
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <span data-testid="currency">{quoteCurrency}</span>
      <button onClick={() => setTheme("light")}>Set Light Theme</button>
      <button onClick={() => setQuoteCurrency("USD")}>Set USD</button>
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
    localStorage.setItem("exchangely_quote_currency", "USD");

    render(
      <SettingsProvider>
        <SettingsConsumer />
      </SettingsProvider>
    );

    expect(screen.getByTestId("theme")).toHaveTextContent("light");
    expect(screen.getByTestId("currency")).toHaveTextContent("USD");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("ignores unsupported stored quote currencies", () => {
    localStorage.setItem("exchangely_quote_currency", "USDT");

    render(
      <SettingsProvider>
        <SettingsConsumer />
      </SettingsProvider>
    );

    expect(screen.getByTestId("currency")).toHaveTextContent("EUR");
  });

  it("ignores unsupported stored themes", () => {
    localStorage.setItem("exchangely_theme", "blue");

    render(
      <SettingsProvider>
        <SettingsConsumer />
      </SettingsProvider>
    );

    expect(screen.getByTestId("theme")).toHaveTextContent("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
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
      screen.getByText("Set USD").click();
    });

    expect(screen.getByTestId("currency")).toHaveTextContent("USD");
    expect(localStorage.getItem("exchangely_quote_currency")).toBe("USD");
  });
});

/**
 * Two independent consumers sharing one SettingsProvider — simulates the gear
 * dropdown (SettingsControls) and the profile page (SettingsPage) both reading
 * and writing theme/currency through the same context.
 */
function ConsumerA() {
  const { theme, setTheme, quoteCurrency, setQuoteCurrency } = useSettings();
  return (
    <div data-testid="consumer-a">
      <span data-testid="a-theme">{theme}</span>
      <span data-testid="a-currency">{quoteCurrency}</span>
      <button onClick={() => setTheme("light")}>A: Light</button>
      <button onClick={() => setQuoteCurrency("USD")}>A: USD</button>
    </div>
  );
}

function ConsumerB() {
  const { theme, setTheme, quoteCurrency, setQuoteCurrency } = useSettings();
  return (
    <div data-testid="consumer-b">
      <span data-testid="b-theme">{theme}</span>
      <span data-testid="b-currency">{quoteCurrency}</span>
      <button onClick={() => setTheme("dark")}>B: Dark</button>
      <button onClick={() => setQuoteCurrency("EUR")}>B: EUR</button>
    </div>
  );
}

describe("SettingsContext — cross-component sync", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });

  it("theme change in consumer A is reflected in consumer B", () => {
    render(
      <SettingsProvider>
        <ConsumerA />
        <ConsumerB />
      </SettingsProvider>
    );

    // Both start at default dark
    expect(screen.getByTestId("a-theme")).toHaveTextContent("dark");
    expect(screen.getByTestId("b-theme")).toHaveTextContent("dark");

    // Consumer A switches to light
    act(() => { screen.getByText("A: Light").click(); });

    // Both consumers see light
    expect(screen.getByTestId("a-theme")).toHaveTextContent("light");
    expect(screen.getByTestId("b-theme")).toHaveTextContent("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("currency change in consumer A is reflected in consumer B", () => {
    render(
      <SettingsProvider>
        <ConsumerA />
        <ConsumerB />
      </SettingsProvider>
    );

    expect(screen.getByTestId("a-currency")).toHaveTextContent("EUR");
    expect(screen.getByTestId("b-currency")).toHaveTextContent("EUR");

    act(() => { screen.getByText("A: USD").click(); });

    expect(screen.getByTestId("a-currency")).toHaveTextContent("USD");
    expect(screen.getByTestId("b-currency")).toHaveTextContent("USD");
    expect(localStorage.getItem("exchangely_quote_currency")).toBe("USD");
  });

  it("consumer B can override what consumer A set", () => {
    render(
      <SettingsProvider>
        <ConsumerA />
        <ConsumerB />
      </SettingsProvider>
    );

    // A sets light
    act(() => { screen.getByText("A: Light").click(); });
    expect(screen.getByTestId("a-theme")).toHaveTextContent("light");
    expect(screen.getByTestId("b-theme")).toHaveTextContent("light");

    // B sets dark
    act(() => { screen.getByText("B: Dark").click(); });
    expect(screen.getByTestId("a-theme")).toHaveTextContent("dark");
    expect(screen.getByTestId("b-theme")).toHaveTextContent("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("rapid toggling between consumers stays consistent", () => {
    render(
      <SettingsProvider>
        <ConsumerA />
        <ConsumerB />
      </SettingsProvider>
    );

    act(() => { screen.getByText("A: USD").click(); });
    act(() => { screen.getByText("B: EUR").click(); });
    act(() => { screen.getByText("A: USD").click(); });

    expect(screen.getByTestId("a-currency")).toHaveTextContent("USD");
    expect(screen.getByTestId("b-currency")).toHaveTextContent("USD");
    expect(localStorage.getItem("exchangely_quote_currency")).toBe("USD");
  });
});
