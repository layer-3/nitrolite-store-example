# CLAUDE

Module:

- `github.com/layer-3/nitrolite-store-example`

Commands:

```bash
go run ./cmd/server
env CGO_ENABLED=0 GOCACHE=/tmp/nitrolite-store-example-gocache go test $(go list ./... | grep -v '/frontend/node_modules/')
docker build -t nitrolite-store-example .
go vet ./...
gofmt -w .
cd frontend && npm run lint && npm run typecheck && npm run build
```

Key paths:

- server entry: `cmd/server/main.go`
- persistence: `internal/store`
- api: `internal/httpapi`
- services: `internal/service`
- frontend source: `frontend`
- web router: `internal/webui/handler.go`
- embedded assets: `internal/webui/dist`

Product surfaces:

- `/`: content store
- `/healthz`: process health
- `/readyz`: Clearnode readiness

Active API:

- `GET /api/store/bootstrap`
- `POST /api/store/init`
- `POST /api/store/update`
- `POST /api/store/content/{id}/open`
