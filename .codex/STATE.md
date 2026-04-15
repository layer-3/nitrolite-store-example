# CODEX STATE

goal: keep `nitrolite-go-example` as the live Go reference for a backend-owned Nitrolite merchant settlement service with embedded operator UI, hosted pay links, embedded API reference, and advanced protocol console
mode: incremental slices with compact checkpoints after each green test pass
outer_repo_rule: never switch branches in `/Users/maharshimishra/Documents/nitrolite`
nested_repo: `/Users/maharshimishra/Documents/nitrolite/nitrolite-go-example`
remote_repo: `https://github.com/ihsraham/nitrolite-go-example`
default_branch: `main`

done:
- bootstrapped the nested Go example repo and pinned `github.com/layer-3/nitrolite@v1.2.0`
- implemented signer init, SDK manager lifecycle, reconnect loop, raw read/mutation services, app sessions, and session keys
- added browser write unlock flow with HttpOnly cookie auth
- embedded `/reference` and `/advanced` surfaces in the same Go binary
- added SQLite-backed merchant persistence in `internal/store`
- added merchant services for payment requests, orders, payouts, operations, and dashboard aggregation
- added operator lease persistence and middleware tiers:
  - write access
  - lease ownership
  - active operator lease
- added startup lease clearing to avoid restart-orphaned lease rows
- added serialized merchant operation runner goroutine
- added operator dashboard at `/`
- added hosted sandbox pay page at `/pay/{slug}`
- rewrote `/openapi.json` around merchant routes while keeping raw protocol routes documented
- updated route and smoke tests for merchant routes, lease flow, and hosted pay page
- verified with `env GOCACHE=/tmp/nitrolite-go-example-gocache go test ./...`

current:
- same-binary Go server serves four surfaces:
  - `/` operator dashboard
  - `/pay/{slug}` hosted sandbox pay page
  - `/reference` API reference
  - `/advanced` raw operator console
- merchant records and operator lease persist in SQLite
- hosted pay is intentionally a sandbox simulation using the backend signer
- merchant writes are browser-session-oriented and require operator lease ownership
- raw protocol routes remain available for debugging

next:
- run the merchant flow manually in a live browser against the sandbox
- verify checkpoint and sync behavior on real chain latency
- add deeper runner tests for waiting-chain / waiting-sync transitions if coverage gaps show up
- run a UX review against the operator dashboard and hosted pay page
- checkpoint, commit, and push the merchant-settlement slice

decisions:
- keep backend demo signer as the only Nitrolite signer
- make `/pay/{slug}` public and wallet-less in v1
- require operator lease even for `POST /api/v1/payment-requests`
- clear persisted lease on startup because write sessions are in-memory
- serialize all merchant SDK writes through one runner goroutine
- keep `/reference` and `/advanced` instead of deleting raw protocol visibility
- use SQLite locally and document Railway Volume requirement for durability

risks:
- hosted pay remains a sandbox simulation, not a real customer payment rail
- lease heartbeat depends on browser timers and can be affected by background-tab throttling
- runner receipt polling currently uses direct RPC receipt reads because the SDK transaction list does not expose tx hashes cleanly enough for the runner
- the app still assumes the configured merchant app exists on the node

files:
- `cmd/server/main.go`: startup ordering, store init, runner wiring
- `internal/store/`: SQLite schema and merchant persistence
- `internal/service/merchant.go`: merchant services and dashboard aggregation
- `internal/service/merchant_runner.go`: serialized async merchant runner
- `internal/httpapi/merchant.go`: merchant routes and lease routes
- `internal/httpapi/openapi.go`: merchant-first OpenAPI document
- `internal/webui/handler.go`: operator/reference/advanced/pay routing
- `web/index.html`: operator dashboard
- `web/pay.html`: hosted sandbox pay page
- `web/merchant.js`: operator dashboard behavior
- `web/pay.js`: hosted pay page behavior
- `web/reference.html`: embedded API reference shell
- `web/advanced.html`: raw operator console
- `test/smoke/console_test.go`: embedded surface smoke tests

last_verified:
- `gofmt -w cmd/server/main.go internal/httpapi/auth.go internal/httpapi/merchant.go internal/httpapi/openapi.go internal/httpapi/routes_test.go internal/service/merchant.go internal/store/store.go internal/webui/handler.go test/smoke/console_test.go`
- `env GOCACHE=/tmp/nitrolite-go-example-gocache go test ./...`
