# nitrolite-go-example

Same-binary Go reference app for a Nitrolite-backed **App Session Micropayment Store**.

The public product surface is a small content store at `/`. A user opens one store session per asset, deposits balance into that app session, buys content instantly, and withdraws what remains. `/reference` and `/advanced` stay available as hidden developer surfaces.

## Surfaces

- `/` App Session Micropayment Store
- `/reference` embedded OpenAPI reference
- `/advanced` raw protocol/developer console

## What This Demonstrates

- one Go binary serving product UI, API, and developer tooling
- Nitrolite app sessions as the store balance rail
- a single `POST /api/v1/app-session/submit-state` endpoint that dispatches by `session_data.action`
- instant content gating after successful purchase
- YUSD and YELLOW catalog pricing from the same seeded catalog

## Current trust model

This v1 implementation is intentionally simple.

- the browser gets a long-lived **store browser cookie** as its store identity namespace
- Nitrolite signing still happens server-side
- the Go server holds:
  - the demo/user signer
  - the store/app signer
- purchases and library access are isolated per browser cookie, not per external wallet

This is a pragmatic reference build, not the final non-custodial shape.

## Quickstart

```bash
cp .env.example .env
go run ./cmd/server
```

Open [http://localhost:8080/](http://localhost:8080/).

If you were previously running an older build of this app, stop that server first. Web assets are embedded into the Go binary, so an older process will keep serving stale UI until restarted.

## Railway deployment

The recommended production-like deployment for this repo is a single Railway service backed by the included `Dockerfile`.

Expected Railway shape:

- one web service for the whole app
- repo-linked to `main`
- one attached volume mounted at `/app/data`
- `SQLITE_PATH=/app/data/nitrolite-go-example.db`

Recommended first-time flow:

1. Link the repo to the target project:

```bash
railway link --workspace Yellow --project clearnet
```

2. Add or link the service that will run this repo.
3. Add a volume and mount it at `/app/data`.
4. Set the required variables on the service.
5. Deploy from the repo or run `railway up` from the repo root.

Useful Railway CLI commands:

```bash
railway volume add --mount-path /app/data
railway variable set CLEARNODE_WS_URL=... DEMO_PRIVATE_KEY=... CONSOLE_API_KEY=... BLOCKCHAIN_RPC_URLS='{"11155111":"https://..."}' HOME_BLOCKCHAINS='{"yusd":11155111,"yellow":11155111}' SQLITE_PATH=/app/data/nitrolite-go-example.db STORE_NAME="Nitrolite App Session Store" STORE_APP_ID=default
railway up --service <service-name>
railway service status --service <service-name>
```

If Railway cannot fetch the GitHub repo initially, the fallback is a first deploy from the local checkout with `railway up`, then converting the service to repo-linked afterward.

## Required environment

Required:

- `CLEARNODE_WS_URL`
- `DEMO_PRIVATE_KEY`
- `CONSOLE_API_KEY`
- `BLOCKCHAIN_RPC_URLS`
- `HOME_BLOCKCHAINS`

Optional with defaults:

- `PORT=8080` locally; Railway injects `PORT` automatically
- `LOG_LEVEL=info`
- `SQLITE_PATH=./data/nitrolite-go-example.db`
- `STORE_NAME=Nitrolite App Session Store`
- `STORE_APP_ID=default`
- `STORE_APP_PRIVATE_KEY=` to override the derived store-app signer

Runtime assumptions:

- the demo signer wallet must already have sandbox funds
- for a shared deployment, the signer should be a team-owned funded sandbox key
- the configured home-channel assets must exist on the node
- the RPC URLs must support the configured home chains

## Recommended local flow

1. Open `/`
2. Pick `YUSD` or `YELLOW`
3. Click `Create or load session`
4. Deposit into the store session
5. Purchase an item from the catalog
6. Open the purchased item in the reader or library
7. Withdraw any remaining user allocation

For hidden developer-only app revenue withdraws or raw protocol mutations:

1. Unlock writes with `CONSOLE_API_KEY`
2. Use `/advanced` or submit `app_withdraw` via the store mutation endpoint

## API shape

Store-first routes:

- `GET /api/v1/store/config`
- `GET /api/v1/catalog`
- `GET /api/v1/catalog/{id}`
- `GET /api/v1/store/session?asset=...`
- `POST /api/v1/store/session/create`
- `POST /api/v1/app-session/submit-state`
- `GET /api/v1/purchases`
- `GET /api/v1/content/{id}`
- `GET /api/v1/balance?asset=...`

The single store mutation endpoint expects `session_data` JSON. Examples:

```json
{"action":"deposit","amount":"1.00"}
{"action":"purchase","item_id":"article-micropayments","price":"0.50"}
{"action":"user_withdraw","amount":"0.50"}
```

The server always revalidates catalog price and allocation math before co-signing.

## Persistence

SQLite stores:

- browser-scoped store sessions
- purchases
- legacy unused tables that are no longer part of the active product path

Default path:

- `./data/nitrolite-go-example.db`

Recommended Railway path:

- `/app/data/nitrolite-go-example.db`

If you are switching from an older build, delete the old local DB first if you want a clean store state:

```bash
rm -f ./data/nitrolite-go-example.db
```

## Running tests

Standard:

```bash
go test ./...
```

If your machine blocks cgo builds because of the local Xcode license state, use:

```bash
CGO_ENABLED=0 GOCACHE=/tmp/nitrolite-go-example-gocache go test ./...
```

## Troubleshooting

If you still see an older UI or the old “Go SDK guided demo” UI:

- stop the existing process on `:8080`
- restart with `go run ./cmd/server`
- hard refresh the browser

If store actions fail:

- check `/healthz`
- confirm the demo wallet has balance in the selected asset
- on Railway, confirm the volume is attached and `SQLITE_PATH` points at `/app/data/nitrolite-go-example.db`
- inspect `/advanced` for raw SDK state
- use `/reference` to inspect the live request/response shapes

## Where to look

- [`AGENTS.md`](./AGENTS.md)
- [`CLAUDE.md`](./CLAUDE.md)
- [`docs/api.md`](./docs/api.md)
- [`docs/architecture.md`](./docs/architecture.md)
