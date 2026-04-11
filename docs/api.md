# API

Exchangely exposes a REST API for market data, authentication, and account management. All endpoints are served under `/api/v1`.

When authentication is enabled (`BACKEND_AUTH_MODE` is set), protected endpoints require either a JWT session token or an API token. Public endpoints remain accessible without credentials.

## Endpoints

### Public Endpoints

These endpoints require no authentication and are exempt from rate limiting.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/health` | Health check |
| `GET` | `/api/v1/assets` | List tracked assets |
| `GET` | `/api/v1/pairs` | List tracked trading pairs |
| `GET` | `/api/v1/config` | Frontend-facing app configuration (auth state, version) |

### Market Data

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/historical/{pair}` | Historical OHLCV candles. Supports `interval`, `start_time`, `end_time` query params. |
| `GET` | `/api/v1/ticker/{pair}` | Current ticker for a single pair |
| `GET` | `/api/v1/tickers` | All current tickers |
| `GET` | `/api/v1/tickers/stream` | SSE stream of ticker updates (delta-only) |
| `GET` | `/api/v1/news` | Latest crypto news |
| `GET` | `/api/v1/news/stream` | SSE stream of news updates |

### System (Admin Only)

These endpoints require the `admin` role.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/system/sync-status` | Sync status for all pairs |
| `GET` | `/api/v1/system/tasks` | Task history |
| `GET` | `/api/v1/system/tasks/stream` | SSE stream of task updates |
| `GET` | `/api/v1/system/users` | List users with pagination, search, role/status filters |
| `GET` | `/api/v1/system/users/{id}` | Get a single user by ID |
| `PATCH` | `/api/v1/system/users/{id}/role` | Update user role (`user`, `premium`, `admin`) |
| `PATCH` | `/api/v1/system/users/{id}/status` | Enable or disable a user account |
| `POST` | `/api/v1/system/users/{id}/force-password-reset` | Force password reset on next login |

### Authentication

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/auth/google/login` | Redirect to Google OAuth |
| `GET` | `/api/v1/auth/google/callback` | OAuth callback |
| `POST` | `/api/v1/auth/local/login` | Email/password login |
| `POST` | `/api/v1/auth/refresh` | Rotate refresh token, get new access token |
| `POST` | `/api/v1/auth/logout` | Invalidate session |
| `GET` | `/api/v1/auth/me` | Authenticated user profile |
| `POST` | `/api/v1/auth/local/change-password` | Change password |

### API Token Management

These endpoints require JWT session authentication. API tokens cannot be used to manage other API tokens.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/auth/api-tokens` | Create a new API token |
| `GET` | `/api/v1/auth/api-tokens` | List all tokens for the authenticated user |
| `DELETE` | `/api/v1/auth/api-tokens/{id}` | Revoke a token |

## Authentication Methods

Requests to protected endpoints can be authenticated in two ways:

**JWT Session** — Obtain an access token via login or refresh, then pass it as a Bearer token:
```
Authorization: Bearer eyJhbGciOiJIUzI1NiJ9...
```

**API Token** — Generate a token from the API Keys page or via `POST /api/v1/auth/api-tokens`, then pass it as:
```
Authorization: Bearer exly_...
```
or:
```
X-API-Key: exly_...
```

The middleware chain processes API tokens before JWT tokens. If a Bearer token starts with `exly_`, it is treated as an API token and JWT parsing is skipped.

## API Tokens

API tokens provide programmatic access to Exchangely data without browser sessions. They are intended for bots, scripts, and third-party integrations.

### Token Format

Tokens are prefixed with `exly_` followed by 32 cryptographically random bytes (base64-encoded). Only the SHA-256 hash is stored server-side. The raw token is returned exactly once at creation time and cannot be retrieved again.

### Limits

- Maximum 5 active (non-revoked, non-expired) tokens per user.
- Tokens expire 90 days after creation.
- Revocation is immediate and idempotent.

### Creating a Token

```bash
curl -X POST http://localhost:8080/api/v1/auth/api-tokens \
  -H "Authorization: Bearer <jwt-access-token>" \
  -H "Content-Type: application/json" \
  -d '{"label": "my-trading-bot"}'
```

Response (201):
```json
{
  "token": "exly_abc123...",
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "label": "my-trading-bot",
  "prefix": "exly_abc",
  "created_at": "2025-01-15T10:30:00Z",
  "expires_at": "2025-04-15T10:30:00Z"
}
```

Save the `token` value immediately — it will not be shown again.

### Listing Tokens

```bash
curl http://localhost:8080/api/v1/auth/api-tokens \
  -H "Authorization: Bearer <jwt-access-token>"
```

Response (200):
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "label": "my-trading-bot",
      "prefix": "exly_abc",
      "status": "active",
      "created_at": "2025-01-15T10:30:00Z",
      "last_used_at": "2025-01-20T14:00:00Z",
      "revoked_at": null,
      "expires_at": "2025-04-15T10:30:00Z"
    }
  ]
}
```

### Revoking a Token

```bash
curl -X DELETE http://localhost:8080/api/v1/auth/api-tokens/{id} \
  -H "Authorization: Bearer <jwt-access-token>"
```

Response: 204 No Content.

### Using a Token

```bash
curl http://localhost:8080/api/v1/tickers \
  -H "X-API-Key: exly_abc123..."
```

or:

```bash
curl http://localhost:8080/api/v1/historical/BTC-USD?interval=1h&start_time=2025-01-01T00:00:00Z \
  -H "Authorization: Bearer exly_abc123..."
```

## Rate Limiting

When authentication is enabled, all authenticated requests to protected endpoints are rate-limited using a sliding window counter backed by PostgreSQL.

### Tiers

Each user role has a configurable request limit per window:

| Role | Default Limit | Env Variable |
|------|:-------------:|--------------|
| `user` | 100 req/min | `BACKEND_RATELIMIT_USER` |
| `premium` | 500 req/min | `BACKEND_RATELIMIT_PREMIUM` |
| `admin` | 1000 req/min | `BACKEND_RATELIMIT_ADMIN` |

An additional per-IP limit of 200 req/min (`BACKEND_RATELIMIT_IP`) applies across all tokens from the same source address, preventing abuse via token rotation.

The sliding window duration defaults to 1 minute and can be changed with `BACKEND_RATELIMIT_WINDOW`.

### Response Headers

Every authenticated response includes rate limit headers:

| Header | Description |
|--------|-------------|
| `X-RateLimit-Limit` | Maximum requests allowed in the current window |
| `X-RateLimit-Remaining` | Requests remaining in the current window |
| `X-RateLimit-Reset` | Unix timestamp when the window resets |

### Rate Limit Exceeded

When the limit is exceeded, the API returns:

```
HTTP/1.1 429 Too Many Requests
Retry-After: 45
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1705312800

{"error": "rate limit exceeded"}
```

The `Retry-After` header indicates the number of seconds until the window resets.

### JWT Session Rate Limiting

Requests authenticated via JWT session (browser-based) are rate-limited by user ID and role, using the same tier system as API tokens.

### Fail-Open Behavior

If PostgreSQL is unreachable during a rate limit check, the request is allowed through (fail-open) and a warning is logged. This prevents a database outage from blocking all API traffic.

### Exemptions

Public endpoints (`/health`, `/assets`, `/pairs`, `/config`) are exempt from both authentication and rate limiting.

## Error Responses

All errors follow a consistent JSON format:

```json
{"error": "description"}
```

| Status | Error | When |
|--------|-------|------|
| 400 | `label is required` | Creating a token without a label |
| 400 | `invalid token id` | Revoking with a malformed UUID |
| 401 | `unauthorized` | Missing, invalid, revoked, or expired token |
| 403 | `forbidden` | Non-admin accessing system endpoints |
| 404 | `not found` | Revoking another user's token |
| 409 | `token limit reached` | Creating a 6th active token |
| 429 | `rate limit exceeded` | Per-token/user or per-IP limit exceeded |
