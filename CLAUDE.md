# CLAUDE

Module:
- `github.com/layer-3/nitrolite-go-example`

Commands:
```bash
go run ./cmd/server
env CGO_ENABLED=0 GOCACHE=/tmp/nitrolite-go-example-gocache go test ./...
go vet ./...
gofmt -w .
```

Key paths:

- server entry: `cmd/server/main.go`
- persistence: `internal/store`
- api: `internal/httpapi`
- services: `internal/service`
- web router: `internal/webui/handler.go`
- web assets: `web`

Product surfaces:

- `/`: App Session Micropayment Store
- `/reference`: embedded API reference
- `/advanced`: raw developer/debug console
