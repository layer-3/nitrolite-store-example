# nitrolite-store-example

Go backend and React frontend for a Nitrolite-backed content store using the Nitrolite v1 TypeScript SDK from the browser and the Go SDK from the server.

The shopper flow is intentionally narrow:

1. Connect MetaMask.
2. Create a two-party app session between the shopper wallet and store app signer.
3. Deposit YUSD into the store session.
4. Purchase content.
5. Read purchased content.
6. Withdraw remaining store balance.

`YUSD` is the canonical flow from the source story. `YELLOW` is also exposed as a second testnet asset so the same wallet/session flows can be exercised against another configured asset before mainnet asset names are finalized.

## Surfaces

- `/`: store UI
- `/healthz`: process health
- `/readyz`: Clearnode readiness
- `GET /api/store/bootstrap`
- `POST /api/store/init`
- `POST /api/store/update`
- `GET /api/store/content/{id}`

## Trust Model

- MetaMask is the shopper identity.
- The frontend constructs the app session definition and app state updates.
- The shopper signs with MetaMask using direct `@yellow-org/sdk` v1 packers.
- The backend verifies the exact signed payload before adding the store app signature.
- The backend never signs as the shopper.
- Store authorization is based on app-session signatures, not login cookies.
- This is not a production-ready authorization model: content reads use a public example-app gate keyed by wallet, item, and submitted purchase status, so production apps should add an authenticated read session, signed read proof, or equivalent boundary before serving confidential content.

## Required Flows

### App Session Creation

1. User presses `Connect`.
2. Frontend requests MetaMask accounts.
3. Frontend calls `GET /api/store/bootstrap?wallet_address=...&asset=yusd`.
4. Frontend constructs `AppDefinitionV1` with shopper and app signer participants, both weight `1`, quorum `2`, and `nonce: BigInt(Date.now() * 1000000)`.
5. Frontend encodes with `packCreateAppSessionRequestV1`.
6. MetaMask signs the encoded payload.
7. Frontend calls `POST /api/store/init`.
8. Backend verifies the definition and shopper signature.
9. Backend app-signs and calls `sdkClient.CreateAppSession`.
10. Frontend receives success and shows the session as ready.

### Deposit

1. User enters a YUSD amount and presses `Deposit`.
2. Frontend uses cached bootstrap data or refreshes `GET /api/store/bootstrap`.
3. Frontend constructs `AppStateUpdateV1` with `AppStateUpdateIntent.Deposit`.
4. `sessionData` is `{"intent":"user_deposit"}`.
5. Frontend encodes with `packAppStateUpdateV1`.
6. MetaMask signs the encoded payload.
7. Frontend calls `POST /api/store/update`.
8. Backend verifies the update, signature, version, participants, asset, and allocation delta.
9. Backend returns the store app signature.
10. Frontend submits to Clearnode with `submitAppSessionDeposit`.
11. Frontend refreshes bootstrap and shows the updated store balance.

### Withdraw

1. User enters a YUSD amount and presses `Withdraw`.
2. Frontend constructs `AppStateUpdateV1` with `AppStateUpdateIntent.Withdraw`.
3. `sessionData` is `{"intent":"user_withdraw"}`.
4. Frontend signs with MetaMask and calls `POST /api/store/update`.
5. Backend verifies, app-signs, submits with `sdkClient.SubmitAppState`, and returns refreshed bootstrap data.

The source story names `submitAppSessionDeposit` in the withdraw step, but that function is deposit-specific. Withdraw follows the same source story's backend `SubmitAppState` step.

### Purchase

1. User chooses item `1` and presses `Purchase`.
2. Frontend constructs `AppStateUpdateV1` with `AppStateUpdateIntent.Operate`.
3. The YUSD example payload is `{"intent":"purchase","item_id":1,"item_price":"0.9"}`.
4. Frontend signs with MetaMask and calls `POST /api/store/update`.
5. Backend verifies item ownership, catalog price, allocation delta, signature, and session version.
6. Backend app-signs, submits with `sdkClient.SubmitAppState`, records ownership, and returns refreshed bootstrap data.

The source story names `submitAppSessionDeposit` in the purchase step as well. This implementation uses backend `SubmitAppState` because purchase is not a deposit operation.

### Open Purchased Content

1. User chooses an owned library item and presses `Open`.
2. Frontend calls `GET /api/store/content/{id}?wallet_address=...&asset=...`.
3. Backend checks the wallet session, asset, item id, and submitted purchase before returning content.
4. This read path is intentionally public for demo ergonomics and should not be used as-is for confidential production content.

## Quickstart

```bash
cp .env.example .env
go run ./cmd/server
```

Open [http://localhost:8080/](http://localhost:8080/).

## Frontend Development

```bash
cd frontend
npm ci
npm run dev
```

The Vite dev server proxies `/api/*` to the Go server. Production assets build into `internal/webui/dist`.

## Environment

Required:

- `CLEARNODE_WS_URL`
- `DEMO_PRIVATE_KEY`
- `BLOCKCHAIN_RPC_URLS`
- `HOME_BLOCKCHAINS`

Optional:

- `PORT=8080`
- `LOG_LEVEL=info`
- `SQLITE_PATH=./data/nitrolite-store-example.db`
- `STORE_NAME=Nitrolite App Session Store`
- `STORE_APP_ID=default`
- `STORE_APP_PRIVATE_KEY=`

## Local Data

SQLite stores wallet app sessions and purchases.

Default local path:

- `./data/nitrolite-store-example.db`

Reset local store state:

```bash
rm -f ./data/nitrolite-store-example.db
```

## Verification

```bash
go test $(go list ./... | grep -v '/frontend/node_modules/')
go vet ./...
cd frontend
npm ci
npm run lint
npm run typecheck
npm run build
cd ..
docker build -t nitrolite-store-example .
```
