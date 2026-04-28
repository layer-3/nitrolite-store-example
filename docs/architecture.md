# Architecture

## Runtime Shape

- `cmd/server` loads config, signers, Nitrolite manager, SQLite store, and HTTP handler.
- `internal/nitrolite.Manager` owns the active SDK client, health state, and reconnect loop.
- `internal/store` persists wallet store sessions and purchases.
- `internal/service.WalletStoreService` owns bootstrap, app-session creation, signed update verification, catalog, purchase recording, and content gating.
- `internal/httpapi` is a thin transport layer.
- `frontend/` is the React source.
- `internal/webui/dist` is the embedded production build served by Go.

## Product Surface

The only product page is `/`.

It supports:

- MetaMask connection
- app session creation
- YUSD deposit
- YUSD withdraw
- YUSD item purchase
- Yellow testnet item purchase
- public demo purchased-content reading
- activity tracing

## Store Session Model

- One wallet-owned store session per supported asset.
- The app session has exactly two participants: shopper wallet and store app signer.
- Each participant has signature weight `1`.
- Quorum is `2`.
- Purchases are keyed by wallet, item, and asset. The active demo assets are `yusd` and `yellow`; `yusd` remains the source-story reference path.

## Submission Boundaries

- App session creation: backend calls `sdkClient.CreateAppSession`.
- Deposit: backend returns the store app signature, frontend calls `submitAppSessionDeposit`.
- Withdraw: backend calls `sdkClient.SubmitAppState`.
- Purchase: backend calls `sdkClient.SubmitAppState`.
- Content open: frontend calls a public demo GET route; backend verifies the wallet session and submitted purchase before returning content. This is not a production authorization boundary.

## Startup Sequence

1. Load config.
2. Initialize app signer.
3. Initialize Nitrolite manager.
4. Initialize SQLite store.
5. Start `manager.Run(ctx)`.
6. Start HTTP server.
