# Authentication 


Exchangely supports two authentication methods that can be used independently or together. When neither is configured, the application runs in unauthenticated mode with all endpoints publicly accessible, preserving backward compatibility.

## Authentication Methods

**Local Admin** — A built-in admin account bootstrapped on first startup. Useful for initial setup, development, and as a recovery path when Google OAuth is unavailable. The account is created with an auto-generated password that must be changed on first login.

Both methods share the same JWT access token and refresh token session model. Access tokens are short-lived (15 min default) and carried as `Authorization: Bearer` headers. Refresh tokens are stored in HTTP-only cookies and rotated on each use.

**Google OAuth 2.0** — Users sign in with their Google account. The backend redirects to Google's authorization endpoint, exchanges the callback code for user profile information (email, name, avatar), and creates or updates a local user record. New Google users are assigned the `user` role.


### First-Run Workflow (Local Admin)

1. Set `BACKEND_ADMIN_EMAIL` in your `.env` file:
   ```
   BACKEND_ADMIN_EMAIL=admin@example.com
   BACKEND_JWT_SECRET=a-strong-random-secret-at-least-32-chars
   ```
2. Start the server (`docker compose up --build`).
3. Find the auto-generated password in the startup log:
   ```
   Local admin created — email: admin@example.com, password: <generated>. This password will not be shown again.
   ```
4. Open the UI and sign in with the email and generated password using the "Sign in with email" form.
5. You will be redirected to the password change page. Set a new password (min 12 characters, must include uppercase, lowercase, digit, and special character).
6. After changing the password, you have full admin access.

> [!NOTE]
> The generated password is logged exactly once. If you lose it, delete the user row from the `users` table and restart the server to re-bootstrap.

### Setting Up Google OAuth 2.0

1. Go to the [Google Cloud Console](https://console.cloud.google.com/) and create a new project (or select an existing one).
2. Navigate to **APIs & Services → OAuth consent screen**.
   - Choose "External" user type (or "Internal" for Google Workspace orgs).
   - Fill in the app name, user support email, and developer contact email.
   - Add the scopes: `openid`, `email`, `profile`.
   - Save and continue.
3. Navigate to **APIs & Services → Credentials**.
   - Click **Create Credentials → OAuth 2.0 Client ID**.
   - Select "Web application" as the application type.
   - Under **Authorized redirect URIs**, add your callback URL (default: `http://localhost:8080/api/v1/auth/google/callback`).
   - Click **Create** and note the Client ID and Client Secret.
4. Set the environment variables:
   ```
   BACKEND_GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
   BACKEND_GOOGLE_CLIENT_SECRET=your-client-secret
   BACKEND_JWT_SECRET=a-strong-random-secret-at-least-32-chars
   ```

## Role System

Exchangely uses two roles:

| Role | Access |
|------|--------|
| `admin` | Full access including the Operations panel (`/api/v1/system/*` endpoints) |
| `user` | Market dashboard, historical data, news, and personal settings. Operations panel is hidden. |

- New users created via Google OAuth are assigned the `user` role by default.
- The local admin account is created with the `admin` role.
- Unauthenticated visitors (when auth is enabled) can only see the Market dashboard and the login page.
- The Operations panel navigation item is only visible to `admin` users. Non-admin users who attempt to access system endpoints directly receive HTTP 403.


## Unauthenticated Mode

When `BACKEND_JWT_SECRET` is empty (or unset), authentication is completely disabled:

- All API endpoints are publicly accessible, matching the original behavior.
- The auth middleware passes all requests through without token validation.
- Auth-related API endpoints (`/api/v1/auth/*`) are not registered.
- The frontend shows the full UI without login prompts.

This is the default for existing deployments and local development without auth needs.

