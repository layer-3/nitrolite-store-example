# CP 0003

when: private remote exists, sdk init works, read-only phase 1 api slice is green

done:
- bootstrap commit pushed to `origin/main`
- Nitrolite SDK pinned to `v1.2.0`
- env signer implemented and tested
- sdk-backed manager init added with:
  - blockchain RPC option wiring
  - home blockchain mapping
  - startup ping
- read services/handlers added for:
  - node config
  - blockchains
  - assets
  - balances
  - transactions
  - home channel
  - latest state
- smoke tests upgraded to use a fake Nitrolite client

verify:
- `go mod tidy`
- `gofmt -w cmd/server/main.go internal/nitrolite/manager.go internal/service/*.go internal/httpapi/*.go test/smoke/console_test.go`
- `go test ./...`

next:
- add mutation auth middleware
- implement approve/deposit/withdraw/transfer/checkpoint
- update console JS to hit real read endpoints
- commit and push this slice
