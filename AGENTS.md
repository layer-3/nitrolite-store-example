# AGENTS

Read order:
1. `README.md`
2. `CLAUDE.md`
3. `docs/api.md`
4. `docs/architecture.md`
5. `internal/config/config.go`
6. `internal/store/`
7. `internal/nitrolite/manager.go`
8. `internal/service/`
9. `internal/httpapi/`
10. `internal/webui/handler.go`
11. `web/`

Ownership:

- `internal/config`: env parsing and validation
- `internal/store`: SQLite schema, store sessions, purchases
- `internal/signing`: signer abstraction, demo signer, derived store-app signer
- `internal/nitrolite`: SDK lifecycle and reconnect manager
- `internal/service`: store orchestration, catalog/content logic, raw SDK-backed business logic
- `internal/httpapi`: decode -> service -> encode, browser cookie flow, write auth, OpenAPI surface
- `internal/webui`: page routing for embedded assets
- `web/`: store UI, embedded reference shell, advanced developer console

Current scope:

- same-binary Go server + embedded web assets
- `/` App Session Micropayment Store
- `/reference` OpenAPI-backed embedded explorer
- `/advanced` raw protocol/developer console
- one browser-scoped store session per asset
- seeded content catalog with purchased-content gating
