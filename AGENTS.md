# AGENTS

Read order:
1. `.codex/STATE.md`
2. `README.md`
3. `CLAUDE.md`
4. `docs/api.md`
5. `docs/architecture.md`
6. `internal/config/config.go`
7. `internal/store/`
8. `internal/nitrolite/manager.go`
9. `internal/service/`
10. `internal/httpapi/`
11. `internal/webui/handler.go`
12. `web/`

Ownership:
- `internal/config`: env parsing and validation
- `internal/store`: SQLite schema, queries, lease persistence, merchant records
- `internal/signing`: signer abstraction and demo signer
- `internal/nitrolite`: SDK lifecycle and reconnect manager
- `internal/service`: merchant orchestration, async runner, raw SDK-backed business logic
- `internal/httpapi`: decode -> service -> encode, auth tiers, OpenAPI surface
- `internal/webui`: page routing for embedded assets
- `web/`: operator dashboard, hosted pay page, embedded reference shell, advanced operator console

Checkpoint rule:
- update `.codex/STATE.md` after every meaningful green milestone
- add `.codex/checkpoints/NNNN-*.md` at context-boundary changes

UX review rule:
- before shipping changes to `web/`, `README.md`, or API/docs surfaces, run the `ux-review` agent
- the UX review must classify whether the surface is acting as:
  - operator dashboard
  - hosted pay page
  - API reference
  - advanced/operator tool
- fix docs parity when the UX review finds stale contracts or misleading product framing

Current scope:
- same-binary Go server + embedded web assets
- `/` operator dashboard
- `/pay/{slug}` hosted sandbox pay page
- `/reference` OpenAPI-backed embedded explorer
- `/advanced` raw operator/debug console
- SQLite-backed merchant records and operator lease
- nested repo lives under `nitrolite/nitrolite-go-example`
- no outer-repo branch switching
