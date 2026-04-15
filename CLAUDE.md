# CLAUDE

Module:
- `github.com/layer-3/nitrolite-go-example`

Commands:
```bash
go run ./cmd/server
env GOCACHE=/tmp/nitrolite-go-example-gocache go test ./...
go test ./... -race
go vet ./...
gofmt -w .
```

Key paths:
- server entry: `cmd/server/main.go`
- persistence: `internal/store`
- api: `internal/httpapi`
- services and runner: `internal/service`
- web router: `internal/webui/handler.go`
- web assets: `web`
- codex state: `.codex/STATE.md`
- checkpoints: `.codex/checkpoints/`

Product surfaces:
- `/`: merchant operator dashboard
- `/pay/{slug}`: public sandbox payment page
- `/reference`: embedded API reference
- `/advanced`: raw operator/debug console
