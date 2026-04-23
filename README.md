# nitrolite-go-example

Go backend reference app for a Nitrolite-backed **content store** with a **TypeScript + MetaMask** frontend.

One Go service serves the built frontend and the store API from the same origin. The shopper experience is intentionally small:

- connect MetaMask
- add funds
- buy content
- read purchased content
- withdraw remaining balance

Protocol details stay behind the implementation boundary. The main UI does not expose raw Nitrolite or channel-management surfaces.

## Surfaces

- `/` shopper-facing store UI
- `/healthz` process health
- `/readyz` Clearnode readiness

## What This Demonstrates

- a Go backend that validates, app-signs, and submits frontend-constructed app-session updates
- a React + Vite frontend that uses MetaMask as the real shopper identity
- automatic app-session bootstrap after wallet connect
- exact-payload forwarding: the update the user signs is the update the backend submits
- instant content gating after successful purchase
- YUSD and YELLOW catalog pricing from one seeded catalog

## Trust model

This refresh is wallet-first.

- MetaMask is the shopper identity
- the frontend constructs the full app-session update
- the shopper signs the update in MetaMask
- the backend validates the same payload, app-signs it, and submits it
- the backend does **not** sign as the user
- channel management is assumed to exist already and is out of scope for this app

Session keys are intentionally deferred to a later phase.

## Quickstart

```bash
cp .env.example .env
go run ./cmd/server
```

Open [http://localhost:8080/](http://localhost:8080/).

If you were previously running an older build of this app, stop that server first. Web assets are embedded into the Go binary, so an older process will keep serving stale UI until restarted.

## How To Use The Store

The main product surface is `/`.

Normal flow:

1. Open `/`
2. Connect MetaMask on Sepolia
3. Wait for the store session to bootstrap automatically
4. Add funds to the store balance
5. Buy an item from the catalog
6. Open it from `Library`
7. Withdraw any remaining balance

The main panels mean:

- `Wallet` shows the connected shopper identity
- `Available balance` is what can still be committed into the store
- `Store balance` is the shopper allocation already inside the store session
- `Catalog` lists seeded content and prices for the selected asset
- `Library` shows wallet-owned content
- `Reader` displays purchased content after ownership is confirmed
- `Browser activity` is a client-side trace for debugging

## Frontend development

The product UI now lives in `frontend/` and is built with React + Vite + TypeScript.

Development shape:

- run the Go API separately
- run Vite in `frontend/`
- proxy `/api/*` to the Go server

Production shape:

- build the frontend into `internal/webui/dist`
- run the Go server
- the Go server serves the built frontend and API from the same origin

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
railway link --workspace <your-workspace> --project <your-project>
```

2. Add or link the service that will run this repo.
3. Add a volume and mount it at `/app/data`.
4. Set the required variables on the service.
5. Deploy from the repo or run `railway up` from the repo root.

If you are deploying inside the Yellow org, use the appropriate internal workspace/project instead of the placeholders above.

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
- `/healthz` reports process health even while the SDK is reconnecting; `/readyz` stays `503` until the Clearnode connection is live

## API shape

Public store routes:

- `POST /api/store/connect/challenge`
- `POST /api/store/connect/verify`
- `GET /api/store/bootstrap?asset=...`
- `POST /api/store/update`
- `GET /api/store/content/{id}?asset=...`
- `GET /healthz`
- `GET /readyz`

The backend receives the full frontend-constructed update payload, validates it, app-signs it, and submits the same payload to Clearnode.

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
