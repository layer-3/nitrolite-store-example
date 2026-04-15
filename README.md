# nitrolite-go-example

Same-binary Go reference app for a Nitrolite-backed **merchant settlement service**.

This repo is no longer a protocol-first guided demo. The primary product surface is a merchant operator dashboard backed by a Go service that owns the signer, persists merchant state in SQLite, and serializes settlement operations through one runner.

## Surfaces

The same Go binary serves four surfaces:

- `/` operator dashboard
- `/pay/{slug}` hosted sandbox payment page
- `/reference` embedded OpenAPI reference
- `/advanced` raw protocol/operator console

## What This Demonstrates

- backend-owned Nitrolite signer
- long-lived SDK manager with reconnect behavior
- hosted pay links with sandbox capture simulation
- order reservation, settlement, refund, and payout flows
- single active operator lease for merchant writes
- embedded browser UI plus JSON API from one deployable Go service

This is intentionally:

- not multi-tenant
- not customer-wallet-based
- not production custody guidance

## Quickstart

```bash
cp .env.example .env
go run ./cmd/server
```

Open [http://localhost:8080/](http://localhost:8080/).

If you already had an older server running, stop it first. The web assets are embedded into the Go binary, so an old process will keep serving the old UI until you restart it.

## Required Environment

Required:

- `CLEARNODE_WS_URL`
- `DEMO_PRIVATE_KEY`
- `CONSOLE_API_KEY`
- `BLOCKCHAIN_RPC_URLS`
- `HOME_BLOCKCHAINS`

Optional with defaults:

- `PORT=8080`
- `LOG_LEVEL=info`
- `SQLITE_PATH=./data/nitrolite-go-example.db`
- `MERCHANT_NAME=Nitrolite Sandbox Merchant`
- `MERCHANT_APP_ID=default`

Important runtime assumptions:

- the demo signer wallet must have gas and sandbox funds
- the configured `MERCHANT_APP_ID` must exist on the node
- the RPC URLs must support the configured home chains

## Recommended Local Flow

1. Open `/`
2. Click `Unlock write actions`
3. Enter `CONSOLE_API_KEY`
4. Click `Acquire operator lease`
5. Create a payment request
6. Open the generated `/pay/{slug}` link in another tab
7. Click `Pay now`
8. Return to `/` and watch the order/operation progress
9. Settle, refund, or queue a payout

## Auth And Concurrency Model

Reads are public.

Writes use three tiers:

- raw protocol writes in `/advanced`
  - bearer key or unlocked browser write cookie
- merchant operator writes
  - unlocked browser write cookie
  - active operator lease ownership
- hosted pay action `POST /api/v1/payment-requests/{slug}/pay`
  - public
  - no customer auth

The lease exists because the backend signer is shared and merchant mutations are async, stateful, and order-sensitive.

## Sandbox Pay Flow

`/pay/{slug}` is a **sandbox payment simulation**.

That means:

- no customer wallet connect
- no card acquiring
- clicking `Pay now` queues a backend capture flow using the demo signer

The point is to demonstrate how a Go backend can make blockchain mostly invisible while still using Nitrolite underneath.

## Persistence

SQLite stores:

- payment requests
- orders
- payouts
- operations
- operator lease

Default path:

- `./data/nitrolite-go-example.db`

`data/` is gitignored.

## Railway Note

SQLite is only durable on Railway if you mount a Volume.

Recommended deploy setup:

- mount a Volume at `/data`
- set `SQLITE_PATH=/data/nitrolite-go-example.db`

Without a Volume, SQLite data is lost on redeploy/rebuild.

## Running Tests

```bash
go test ./...
```

The current test suite covers:

- merchant route contracts
- auth tier behavior
- operator lease ownership rules
- embedded surface smoke checks

## Troubleshooting

If you still see the old “Go SDK guided demo” UI:

- you are running an older compiled server
- stop the process on `:8080`
- start `go run ./cmd/server` again
- hard refresh the browser

If merchant actions fail:

- check `/healthz`
- confirm the demo wallet has funds
- confirm `MERCHANT_APP_ID` exists
- inspect the operations list on `/`

## Production Caveats

This repo is a reference app, not a production service.

Before using this pattern in production, change at least:

- demo signer storage and key management
- single-process in-memory write sessions
- SQLite durability and backup strategy
- hosted pay flow semantics
- operator identity and lease model
- tx confirmation and clearnode sync observability

## Where To Look

- [`AGENTS.md`](/Users/maharshimishra/Documents/nitrolite/nitrolite-go-example/AGENTS.md)
- [`CLAUDE.md`](/Users/maharshimishra/Documents/nitrolite/nitrolite-go-example/CLAUDE.md)
- [`docs/api.md`](/Users/maharshimishra/Documents/nitrolite/nitrolite-go-example/docs/api.md)
- [`docs/architecture.md`](/Users/maharshimishra/Documents/nitrolite/nitrolite-go-example/docs/architecture.md)
- [`.codex/STATE.md`](/Users/maharshimishra/Documents/nitrolite/nitrolite-go-example/.codex/STATE.md)
