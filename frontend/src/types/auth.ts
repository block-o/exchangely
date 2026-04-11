export type User = {
  id: string;
  email: string;
  name: string;
  avatar_url: string;
  role: "admin" | "user" | "premium";
  has_google: boolean;
  has_password: boolean;
  must_change_password: boolean;
  disabled?: boolean;
};

export type AuthMethods = {
  google: boolean;
  local: boolean;
};

/** Response from GET /api/v1/config — frontend-relevant backend configuration. */
export type AppConfig = {
  auth_enabled: boolean;
  auth_methods: AuthMethods;
  version: string;
};

export type TokenResponse = {
  access_token: string;
  must_change_password?: boolean;
};
