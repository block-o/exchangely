export type User = {
  id: string;
  email: string;
  name: string;
  avatar_url: string;
  role: "admin" | "user";
  must_change_password: boolean;
};

export type AuthMethods = {
  google: boolean;
  local: boolean;
};

export type TokenResponse = {
  access_token: string;
  must_change_password?: boolean;
};
