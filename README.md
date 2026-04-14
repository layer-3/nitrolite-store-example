# nitrolite-go-example

Go reference app for the Nitrolite SDK.

Status:
- private repo created and linked
- sdk-backed startup path implemented
- Phase 1 read-only API slice implemented
- write-path actions still pending

Local:
```bash
cp .env.example .env
go run ./cmd/server
```

Open `http://localhost:8080`.

Planned shape:
- single Go binary
- embedded web console
- JSON HTTP API
- Railway deploy target

See [AGENTS.md](/Users/maharshimishra/Documents/nitrolite/nitrolite-go-example/AGENTS.md) and [.codex/STATE.md](/Users/maharshimishra/Documents/nitrolite/nitrolite-go-example/.codex/STATE.md).
