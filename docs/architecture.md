# Architecture

Status: scaffold.

Current runtime:
- `cmd/server` loads config and serves HTTP
- `internal/nitrolite.Manager` is a placeholder lifecycle object
- `internal/httpapi` exposes `healthz`, `readyz`, and `wallet`
- `web/` assets are embedded through `web/assets.go`

Next:
- replace placeholder manager with real SDK client lifecycle
- add read endpoints
- add signer abstraction

