# API

The store API is intentionally small. App-session creation and state changes are wallet-signature driven; content reads use a public demo gate.

## `GET /api/store/bootstrap`

Query:

- `wallet_address`
- `asset`, optional, defaults to the configured default asset

Returns store metadata, supported assets, available Clearnode balance, catalog, current wallet session state, wallet-owned library items, and an optional pending deposit action.

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

Handles user deposit, user withdraw, and purchase updates.

Request fields:

- `wallet_address`, optional consistency check
- `asset`
- `app_state_update`
- `user_signature`

The backend verifies the shopper signature over `packAppStateUpdateV1(app_state_update)` and dispatches by `app_state_update.intent`.

Deposit returns:

- `status: "signed"`
- `intent: "user_deposit"`
- `app_signature`
- `pending_action`, containing the signed deposit payload needed for browser-side resume

Withdraw and purchase return:

- `status: "submitted"`
- `intent`
- `bootstrap`

`app_state_update.intent` remains the Nitrolite protocol intent (`deposit`, `withdraw`, or `operate`). The nested `session_data.intent` is the store-level action (`user_deposit`, `user_withdraw`, or `purchase`).

For deposits, the backend stores a checkpoint before returning the app signature. Bootstrap returns that checkpoint as `pending_action` until Clearnode reflects the submitted app session version, so the frontend can resume after reload without asking MetaMask to sign again.

## `GET /api/store/content/{id}`

Opens purchased content for a wallet and asset.

Query:

- `wallet_address`
- `asset`, `yusd` or `yellow`

The backend confirms the wallet session, reconciles pending purchases, and returns content only for a submitted purchase.

This is intentionally a public example-app read path. It does not prove the caller controls `wallet_address`; production content APIs should use an authenticated read session, signed read proof, or equivalent authorization boundary.

`POST /api/store/content/{id}/open` is disabled and returns `405`.

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
{"intent":"user_deposit","amount":"1.00"}
```

Withdraw:

```json
{"intent":"user_withdraw"}
```

Purchase:

```json
{"intent":"purchase","item_id":1,"item_price":"0.9"}
```
