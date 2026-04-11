# Authentication 

Exchangely supports two authentication methods — local admin and Google OAuth 2.0 (SSO) — controlled by the `BACKEND_AUTH_MODE` environment variable. When the variable is empty or unset, the application runs in unauthenticated mode with all endpoints publicly accessible, preserving backward compatibility.

## Auth Modes

| Mode | Description |
|------|-------------|
| _(empty)_ | Auth disabled. All endpoints are public. No login UI. |
| `local` | Local email/password login only. The login page shows only the email/password form. |
| `sso` | Google OAuth 2.0 only. The login page shows only the "Sign in with Google" button. |
| `local,sso` | Both methods enabled. The login page shows both options. |

Both methods share the same JWT access token and refresh token session model. Access tokens are short-lived (15 min default) and carried as `Authorization: Bearer` headers. Refresh tokens are stored in HTTP-only cookies and rotated on each use.

## Required Variables per Mode

The backend validates configuration at startup and refuses to start if required variables are missing.

| Variable | `local` | `sso` | `local,sso` |
|----------|:-------:|:-----:|:-----------:|
| `BACKEND_JWT_SECRET` | ✓ | ✓ | ✓ |
| `BACKEND_ADMIN_EMAIL` | ✓ | | ✓ |
| `BACKEND_GOOGLE_CLIENT_ID` | | ✓ | ✓ |
| `BACKEND_GOOGLE_CLIENT_SECRET` | | ✓ | ✓ |

## Authentication Methods

**Local Admin** — A built-in admin account bootstrapped on first startup. Useful for initial setup, development, and as a recovery path when Google OAuth is unavailable. The account is created with an auto-generated password that must be changed on first login.

**Google OAuth 2.0** — Users sign in with their Google account. The backend redirects to Google's authorization endpoint, exchanges the callback code for user profile information (email, name, avatar), and creates or updates a local user record. New Google users are assigned the `user` role.


### First-Run Workflow (Local Admin)

1. Set the required variables in your `.env` file:
   ```
   BACKEND_AUTH_MODE=local
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

### Creating Additional Local Users

Exchangely does not provide a self-service registration flow. To create additional local users you insert them directly into PostgreSQL. The password must be a bcrypt hash.

1. Generate a bcrypt hash for the initial password. You can use any tool; for example with `htpasswd`:
   ```bash
   htpasswd -nbBC 12 "" 'YourTempPassword123!' | cut -d: -f2
   ```
   Or with Python:
   ```bash
   python3 -c "import bcrypt; print(bcrypt.hashpw(b'YourTempPassword123!', bcrypt.gensalt(12)).decode())"
   ```

2. Connect to the database and insert the user:
   ```sql
   INSERT INTO users (id, email, name, role, password_hash, must_change_password, created_at, updated_at)
   VALUES (
     gen_random_uuid(),
     'newuser@example.com',
     'New User',
     'user',                          -- or 'admin'
     '$2a$12$...',                     -- paste the bcrypt hash here
     true,                             -- force password change on first login
     now(),
     now()
   );
   ```

3. The user can now sign in with the email and temporary password. If `must_change_password` is `true`, they will be redirected to the password change page on first login.

> [!TIP]
> For Docker Compose deployments, connect with:
> ```bash
> docker compose exec timescaledb psql -U postgres -d exchangely
> ```

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
   BACKEND_AUTH_MODE=sso
   BACKEND_GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
   BACKEND_GOOGLE_CLIENT_SECRET=your-client-secret
   BACKEND_JWT_SECRET=a-strong-random-secret-at-least-32-chars
   ```

To enable both methods simultaneously, use `BACKEND_AUTH_MODE=local,sso` and provide all variables from both sections.

## Role System

Exchangely uses two roles:

| Role | Access |
|------|--------|
| `admin` | Full access including the Operations panel (`/api/v1/system/*` endpoints) and user management |
| `premium` | Same as `user` with higher API rate limits (500 req/min vs 100) |
| `user` | Market dashboard, historical data, news, and personal settings. Operations panel is hidden. |

- New users created via Google OAuth are assigned the `user` role by default.
- The local admin account is created with the `admin` role.
- Unauthenticated visitors (when auth is enabled) can only see the Market dashboard and the login page.
- The Operations panel navigation item is only visible to `admin` users. Non-admin users who attempt to access system endpoints directly receive HTTP 403.


## Unauthenticated Mode

When `BACKEND_AUTH_MODE` is empty (or unset), authentication is completely disabled:

- All API endpoints are publicly accessible, matching the original behavior.
- The auth middleware passes all requests through without token validation.
- Auth-related API endpoints (`/api/v1/auth/*`) are not registered.
- The frontend shows the full UI without login prompts.

This is the default for existing deployments and local development without auth needs.
