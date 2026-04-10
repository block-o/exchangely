import type { PropsWithChildren } from "react";
import { AuthProvider } from "./auth";
import { SettingsProvider } from "./settings";

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <AuthProvider>
      <SettingsProvider>{children}</SettingsProvider>
    </AuthProvider>
  );
}
