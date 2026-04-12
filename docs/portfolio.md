# Portfolio Tracker

The Portfolio Tracker lets authenticated users manage and monitor their crypto holdings from multiple sources — manual entries, centralized exchange syncs, on-chain wallet tracking, and Ledger Live imports. Portfolio value, allocation, and profit/loss are computed using Exchangely's own price data and updated in real time via SSE.

> [!NOTE]
> Portfolio features require `BACKEND_PORTFOLIO_ENABLED=true` and a valid `BACKEND_PORTFOLIO_ENCRYPTION_KEY`. The backend refuses to start if the encryption key is missing when portfolio is enabled.

## Holding Sources

| Source | How It Works |
|--------|-------------|
| Manual | User enters asset, quantity, and optional average buy price directly |
| Binance | Read-only API key syncs account balances via `/api/v3/account` |
| Kraken | Read-only API key syncs balances via `/0/private/Balance` |
| Coinbase | Read-only API key syncs balances via Coinbase Advanced Trade API |
| Ethereum | Public address tracked for native ETH + ERC-20 token balances |
| Solana | Public address tracked for native SOL + SPL token balances |
| Bitcoin | Public address tracked for BTC balance |
| Ledger Live | Hardware wallet balances imported via Ledger Live API |

## Security Model

All sensitive data is encrypted at rest using AES-256-GCM:

- Exchange API keys and secrets are encrypted before storage. Only a key prefix is exposed in listings.
- Wallet addresses are encrypted. Only an address prefix is shown in the UI.
- Holding metadata (notes, labels) is encrypted per-user.
- Encryption uses per-user derived keys (HKDF-SHA256 from the master key), so a single compromised key does not expose all users' data.
- Numeric fields (quantity, average buy price) remain as plaintext to allow efficient SQL-based valuation calculations.

Portfolio endpoints require JWT session authentication. API tokens (`exly_`-prefixed) are explicitly rejected on all portfolio routes — this prevents programmatic access to sensitive financial data.

## API Endpoints

All portfolio endpoints are under `/api/v1/portfolio/` and require JWT session auth.

### Holdings

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/portfolio/holdings` | List all holdings (manual + synced) |
| `POST` | `/api/v1/portfolio/holdings` | Create a manual holding |
| `PUT` | `/api/v1/portfolio/holdings/{id}` | Update a holding |
| `DELETE` | `/api/v1/portfolio/holdings/{id}` | Delete a holding |

### Valuation

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/portfolio/valuation` | Current portfolio valuation with allocation and P&L |
| `GET` | `/api/v1/portfolio/history?range=7d` | Historical portfolio value (1d, 7d, 30d, 1y) |
| `GET` | `/api/v1/portfolio/stream` | SSE stream of live portfolio value updates |

### Exchange Credentials

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/portfolio/credentials` | Store an exchange API key (encrypted) |
| `GET` | `/api/v1/portfolio/credentials` | List credentials (metadata only, no secrets) |
| `DELETE` | `/api/v1/portfolio/credentials/{id}` | Delete credential and associated holdings |
| `POST` | `/api/v1/portfolio/credentials/{id}/sync` | Trigger exchange balance sync |

### Wallet Addresses

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/portfolio/wallets` | Link a blockchain wallet address |
| `GET` | `/api/v1/portfolio/wallets` | List linked wallets (metadata only) |
| `DELETE` | `/api/v1/portfolio/wallets/{id}` | Delete wallet and associated holdings |
| `POST` | `/api/v1/portfolio/wallets/{id}/sync` | Trigger on-chain balance sync |

### Ledger Live

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/portfolio/ledger/connect` | Connect Ledger Live account |
| `POST` | `/api/v1/portfolio/ledger/sync` | Re-sync Ledger balances |
| `DELETE` | `/api/v1/portfolio/ledger` | Disconnect Ledger Live and remove holdings |

## Creating a Manual Holding

```bash
curl -X POST http://localhost:8080/api/v1/portfolio/holdings \
  -H "Authorization: Bearer <jwt-access-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "asset_symbol": "BTC",
    "quantity": 0.5,
    "avg_buy_price": 42000.00,
    "quote_currency": "USD"
  }'
```

Response (201):
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "asset_symbol": "BTC",
  "quantity": 0.5,
  "avg_buy_price": 42000.00,
  "quote_currency": "USD",
  "source": "manual",
  "created_at": "2026-04-12T10:30:00Z",
  "updated_at": "2026-04-12T10:30:00Z"
}
```

## Storing an Exchange Credential

```bash
curl -X POST http://localhost:8080/api/v1/portfolio/credentials \
  -H "Authorization: Bearer <jwt-access-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "exchange": "binance",
    "api_key": "your-read-only-api-key",
    "api_secret": "your-api-secret"
  }'
```

Response (201):
```json
{
  "id": "660e8400-e29b-41d4-a716-446655440001",
  "exchange": "binance",
  "api_key_prefix": "your-r",
  "status": "active",
  "created_at": "2026-04-12T10:35:00Z"
}
```

The full API key and secret are never returned after creation. Only the prefix is shown for identification.

## Linking a Wallet Address

```bash
curl -X POST http://localhost:8080/api/v1/portfolio/wallets \
  -H "Authorization: Bearer <jwt-access-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "chain": "ethereum",
    "address": "0x742d35Cc6634C0532925a3b844Bc9e7595f2bD18",
    "label": "My main wallet"
  }'
```

Response (201):
```json
{
  "id": "770e8400-e29b-41d4-a716-446655440002",
  "chain": "ethereum",
  "address_prefix": "0x742d",
  "label": "My main wallet",
  "created_at": "2026-04-12T10:40:00Z"
}
```

The full address is encrypted at rest. Only the prefix is returned in listings.

## Portfolio Valuation

```bash
curl http://localhost:8080/api/v1/portfolio/valuation \
  -H "Authorization: Bearer <jwt-access-token>"
```

Response (200):
```json
{
  "total_value": 52450.00,
  "quote_currency": "USD",
  "assets": [
    {
      "asset_symbol": "BTC",
      "quantity": 0.5,
      "current_price": 95000.00,
      "current_value": 47500.00,
      "allocation_pct": 90.56,
      "avg_buy_price": 42000.00,
      "unrealized_pnl": 26500.00,
      "unrealized_pnl_pct": 63.10,
      "priced": true,
      "source": "manual"
    },
    {
      "asset_symbol": "ETH",
      "quantity": 2.0,
      "current_price": 2475.00,
      "current_value": 4950.00,
      "allocation_pct": 9.44,
      "avg_buy_price": null,
      "unrealized_pnl": null,
      "unrealized_pnl_pct": null,
      "priced": true,
      "source": "binance"
    }
  ],
  "updated_at": "2026-04-12T10:45:00Z"
}
```

Holdings without an average buy price (e.g., exchange-synced without cost basis) report P&L as `null` rather than assuming zero cost.

## Historical Portfolio Value

```bash
curl "http://localhost:8080/api/v1/portfolio/history?range=7d" \
  -H "Authorization: Bearer <jwt-access-token>"
```

Response (200):
```json
{
  "range": "7d",
  "data": [
    {"timestamp": 1712880000, "value": 48200.00},
    {"timestamp": 1712883600, "value": 48750.00},
    {"timestamp": 1712887200, "value": 49100.00}
  ]
}
```

Supported ranges: `1d`, `7d`, `30d`, `1y`. Uses hourly candles for 1d/7d and daily candles for 30d/1y.

## Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKEND_PORTFOLIO_ENABLED` | Enable portfolio features | `false` |
| `BACKEND_PORTFOLIO_ENCRYPTION_KEY` | Hex-encoded 32-byte AES-256 master key for encrypting credentials and metadata. Required when portfolio is enabled. | _(none)_ |
| `BACKEND_ETHERSCAN_API_KEY` | Etherscan API key for Ethereum balance lookups | _(none)_ |
| `BACKEND_SOLANA_RPC_URL` | Solana RPC endpoint for balance queries | `https://api.mainnet-beta.solana.com` |
| `BACKEND_BITCOIN_API_URL` | Bitcoin API endpoint for balance queries | `https://blockstream.info/api` |

### Generating an Encryption Key

Generate a random 32-byte hex key:

```bash
openssl rand -hex 32
```

Set it in your `.env`:
```
BACKEND_PORTFOLIO_ENABLED=true
BACKEND_PORTFOLIO_ENCRYPTION_KEY=<your-64-char-hex-string>
```

## Error Responses

| Status | Error | When |
|--------|-------|------|
| 400 | `quantity must be positive` | Creating/updating a holding with quantity ≤ 0 |
| 400 | `unknown asset symbol` | Asset not in the Exchangely catalog |
| 400 | `unsupported exchange` | Exchange not in {binance, kraken, coinbase} |
| 400 | `unsupported chain` | Chain not in {ethereum, solana, bitcoin} |
| 400 | `invalid address format` | Wallet address doesn't match chain format |
| 401 | `unauthorized` | Missing or invalid JWT session |
| 403 | `forbidden` | API token used on portfolio endpoint, or cross-user access |
| 404 | `not found` | Resource doesn't exist or belongs to another user |
| 500 | `sync failed` | Exchange/wallet/Ledger API error after retries |
