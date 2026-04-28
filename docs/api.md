# API

The store API is intentionally small and wallet-signature driven.

## `GET /api/store/bootstrap`

Query:

- `wallet_address`
- `asset`, optional, defaults to the configured default asset

Returns store metadata, supported assets, available Clearnode balance, catalog, current wallet session state, and wallet-owned library items.

Supported demo assets are `yusd` and `yellow`. `yusd` is the source-story path; `yellow` is retained as a second testnet asset for the same app-session flows.

## `POST /api/store/init`

Creates the app session after the frontend signs the app definition.

Request fields:

- `wallet_address`, optional consistency check
- `asset`
- `definition`
- `session_data`
- `user_signature`

The backend verifies the shopper signature over `packCreateAppSessionRequestV1(definition, session_data)`, adds the store app signature, calls `sdkClient.CreateAppSession`, persists the session, and returns bootstrap data.

## `POST /api/store/update`

Handles deposit, withdraw, and purchase updates.

Request fields:

- `wallet_address`, optional consistency check
- `asset`
- `app_state_update`
- `user_signature`

The backend verifies the shopper signature over `packAppStateUpdateV1(app_state_update)` and dispatches by `app_state_update.intent`.

Deposit returns:

- `status: "signed"`
- `intent: "deposit"`
- `app_signature`

Withdraw and purchase return:

- `status: "submitted"`
- `intent`
- `bootstrap`

## `POST /api/store/content/{id}/open`

Opens purchased content after the shopper signs a read proof.

Request fields:

- `content_request`
- `user_signature`

`content_request` fields:

- `domain: "nitrolite-store-example"`
- `version: "1"`
- `action: "open_content"`
- `wallet_address`
- `asset`, `yusd` or `yellow`
- `app_session_id`
- `item_id`
- `issued_at`, RFC3339 timestamp

The backend verifies the signature, requires `issued_at` to be within 5 minutes, confirms the wallet session and item match the request, reconciles pending purchases, and returns content only for a submitted purchase.

`GET /api/store/content/{id}` is intentionally disabled and returns `405`.

## Errors

Errors use:

```json
{"error":{"code":"stale_version","message":"app session version is stale"}}
```

Important signed-flow codes:

- `invalid_signature`
- `stale_version`
- `insufficient_balance`
- `duplicate_purchase`
- `clearnode_unavailable`
- `clearnode_operation_failed`

## Session Data

Deposit:

```json
{"intent":"deposit"}
```

Withdraw:

```json
{"intent":"withdraw"}
```

Purchase:

```json
{"intent":"purchase","item_id":1,"item_price":"0.9"}
```
