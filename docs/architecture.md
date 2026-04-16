# Architecture

## Runtime shape

- `cmd/server` loads config, signers, Nitrolite manager, SQLite store, and the HTTP/web handler
- `internal/nitrolite.Manager` owns the active SDK client, health state, and reconnect loop
- `internal/store` owns SQLite persistence for browser-scoped store sessions and purchases
- `internal/service.StorefrontService` owns catalog, session bootstrap, submit-state dispatch, and content gating
- `internal/httpapi` is a thin transport layer with decode -> service -> encode plus raw write auth
- `web/` is a no-build static UI embedded into the Go binary

## Product surfaces

- `/`
  - App Session Micropayment Store
  - deposit, purchase, withdraw, reader, and library
- `/reference`
  - embedded RapiDoc explorer backed by `/openapi.json`
- `/advanced`
  - raw developer/debug console
  - JSON panels, raw mutations, session keys, destructive operations

## Store session model

- one browser-scoped store session per asset
- supported assets are data-driven from `HOME_BLOCKCHAINS`
- current seeded catalog supports YUSD and YELLOW pricing
- purchases are tracked per browser cookie and asset

All product actions after session creation go through:

- `POST /api/v1/app-session/submit-state`

The server dispatches by `session_data.action`:

- `deposit`
- `purchase`
- `user_withdraw`
- `app_withdraw`

## Trust model

This v1 build keeps Nitrolite signing on the server:

- demo/user signer: `DEMO_PRIVATE_KEY`
- store/app signer: derived from `DEMO_PRIVATE_KEY` unless `STORE_APP_PRIVATE_KEY` is set
- browser identity: lightweight cookie namespace only

That means:

- the browser does not hold a Nitrolite signer yet
- the same backend signers service every browser
- store isolation is product-level and browser-scoped, not wallet-level

This is deliberate to keep the example simple and runnable with the current Go SDK setup.

## Startup sequence

1. load config
2. initialize demo/user signer
3. derive or load store/app signer
4. initialize Nitrolite manager
5. initialize SQLite store
6. start `manager.Run(ctx)`
7. start `http.Server`

There is no active merchant runner in the current product path.

## Catalog and content

- catalog metadata is seeded in `internal/service/store_app.go`
- each item has per-asset pricing
- content is stored inline with the seeded catalog for now
- successful purchase writes a `purchases` record immediately
- content access checks that purchase record before returning the body
