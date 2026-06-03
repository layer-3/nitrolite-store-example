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
10. `frontend/`
11. `internal/webui/handler.go`

Ownership:

- `internal/config`: env parsing and validation
- `internal/store`: SQLite wallet sessions and purchases
- `internal/signing`: signer abstraction and store app signer
- `internal/nitrolite`: SDK lifecycle and reconnect manager
- `internal/service`: wallet store orchestration, catalog, signed update verification, and content gating
- `internal/httpapi`: request decode, service calls, and response encoding
- `frontend`: React + direct Nitrolite v1 TypeScript SDK browser flow
- `internal/webui`: embedded asset serving

Current scope:

- same-binary Go server plus embedded frontend assets
- `/` content store
- one wallet-owned store session per asset
- seeded content catalog with purchased-content gating
