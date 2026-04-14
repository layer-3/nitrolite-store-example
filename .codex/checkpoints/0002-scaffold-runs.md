# CP 0002

when: first runnable scaffold
done:
- config loader with `.env` support
- minimal HTTP server
- embedded console
- wallet/health routes
- smoke test
- `gh` auth restored

verify:
- `gofmt -w .`
- `go test ./...`

next:
- commit bootstrap
- create private GitHub repo
- push initial snapshot
- start real Nitrolite manager/signer wiring
