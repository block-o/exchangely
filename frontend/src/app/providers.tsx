import type { PropsWithChildren } from "react";
import { SettingsProvider } from "./settings";

export function AppProviders({ children }: PropsWithChildren) {
  return <SettingsProvider>{children}</SettingsProvider>;
}
