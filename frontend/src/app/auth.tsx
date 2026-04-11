import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type PropsWithChildren,
} from "react";
import {
  API_BASE_URL,
  apiGet,
  authGet,
  refreshAccessToken,
  setAccessToken,
} from "../api/client";
import type { AppConfig, AuthMethods, User } from "../types/auth";

type AuthContextValue = {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  /** True when the backend has auth enabled (BACKEND_AUTH_MODE is set). */
  authEnabled: boolean;
  /** Which login methods the backend exposes. Null until config is fetched. */
  authMethods: AuthMethods | null;
  login: () => void;
  logout: () => Promise<void>;
  refreshToken: () => Promise<boolean>;
};

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

/**
 * AuthProvider manages authentication state for the entire app.
 *
 * On mount it fetches /api/v1/config to determine whether auth is enabled and
 * which methods are available. If auth is enabled it attempts a silent refresh
 * via the httpOnly refresh-token cookie. While the init is in flight a loading
 * state is exposed so the UI can show a spinner instead of flashing the login page.
 */
export function AuthProvider({ children }: PropsWithChildren) {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [authEnabled, setAuthEnabled] = useState(true);
  const [authMethods, setAuthMethods] = useState<AuthMethods | null>(null);

  /** Fetch the current user profile from /auth/me. */
  const fetchUser = useCallback(async (): Promise<User | null> => {
    try {
      return await authGet<User>("/auth/me");
    } catch {
      return null;
    }
  }, []);

  /** Attempt a token refresh and fetch the user profile. Returns true on success. */
  const refreshToken = useCallback(async (): Promise<boolean> => {
    try {
      const token = await refreshAccessToken();
      if (!token) {
        setUser(null);
        return false;
      }
      const profile = await fetchUser();
      if (!profile) {
        setAccessToken(null);
        setUser(null);
        return false;
      }
      setUser(profile);
      return true;
    } catch {
      setAccessToken(null);
      setUser(null);
      return false;
    }
  }, [fetchUser]);

  /** Redirect to the Google OAuth login endpoint. */
  const login = useCallback(() => {
    window.location.href = `${API_BASE_URL}/auth/google/login`;
  }, []);

  /** Log out: call the backend, then clear all local auth state. */
  const logout = useCallback(async () => {
    try {
      await fetch(`${API_BASE_URL}/auth/logout`, {
        method: "POST",
        credentials: "include",
      });
    } catch {
      // Best-effort — clear local state regardless.
    }
    setAccessToken(null);
    setUser(null);
  }, []);

  // On mount, fetch /config and optionally attempt silent refresh.
  useEffect(() => {
    let cancelled = false;

    async function init() {
      // Fetch backend config to determine auth state.
      let config: AppConfig | null = null;
      try {
        config = await apiGet<AppConfig>("/config");
      } catch {
        // Config endpoint unavailable — assume auth disabled.
      }

      if (cancelled) return;

      if (!config || !config.auth_enabled) {
        setAuthEnabled(false);
        setAuthMethods(config?.auth_methods ?? null);
        setIsLoading(false);
        return;
      }

      setAuthEnabled(true);
      setAuthMethods(config.auth_methods);

      // Auth is enabled — attempt silent refresh.
      try {
        const token = await refreshAccessToken();
        if (cancelled) return;
        if (token) {
          const profile = await fetchUser();
          if (cancelled) return;
          if (profile) {
            setUser(profile);
          } else {
            setAccessToken(null);
          }
        }
      } catch {
        // No valid session — stay unauthenticated.
      }
      if (!cancelled) {
        setIsLoading(false);
      }
    }

    init();
    return () => {
      cancelled = true;
    };
  }, [fetchUser]);

  const value: AuthContextValue = {
    user,
    isAuthenticated: user !== null,
    isLoading,
    authEnabled,
    authMethods,
    login,
    logout,
    refreshToken,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

/**
 * Hook to access auth state. Must be used within an AuthProvider.
 */
export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}
