# AGENTS

Read order:
1. `.codex/STATE.md`
2. `internal/config/config.go`
3. `internal/nitrolite/manager.go`
4. `internal/service/`
5. `internal/httpapi/`
6. `internal/webui/handler.go`
7. `web/`

Ownership:
- `internal/config`: env parsing and validation
- `internal/signing`: signer abstraction and demo signer
- `internal/nitrolite`: SDK lifecycle and reconnect manager
- `internal/service`: business logic only
- `internal/httpapi`: decode -> service -> encode
- `web/`: console assets and embedded FS

Checkpoint rule:
- update `.codex/STATE.md` after every meaningful milestone
- add `.codex/checkpoints/NNNN-*.md` at context-boundary changes

Current scope:
- Phase 1 scaffold
- no outer-repo branch switching
- nested repo lives under `nitrolite/nitrolite-go-example`

