# CODEX STATE

goal: build `nitrolite-go-example` as a nested private repo with a Phase 1 Go service + embedded console
mode: scaffold-first, then implement Phase 1 incrementally
outer_repo_rule: never switch branches in `/Users/maharshimishra/Documents/nitrolite`
nested_repo: `/Users/maharshimishra/Documents/nitrolite/nitrolite-go-example`
remote_repo: pending GitHub auth + repo creation

done:
- created nested repo dir + git init
- added repo metadata files
- added compact agent rules and skills
- added minimal runnable server scaffold
- verified compile with `go test ./...`
- restored `gh` auth for repo creation

current:
- create private remote repo and push bootstrap
- replace placeholder manager with real SDK lifecycle

next:
- create first local commit
- create private GitHub repo and push
- implement Phase 1 read endpoints
- add signer abstraction and real SDK manager wiring

decisions:
- use `.codex/STATE.md` as canonical compact context file
- use `.codex/checkpoints/NNNN-*.md` as immutable milestone logs
- keep repo inside parent checkout for SDK/source proximity

risks:
- GitHub CLI auth pending
- actual `go:embed` cannot read parent dirs from `internal/webui`; solve via `web` package embedding

files:
- `cmd/server/main.go`: entrypoint
- `internal/config/config.go`: env config
- `internal/httpapi/*`: HTTP skeleton
- `internal/webui/handler.go`: UI serving
- `web/*`: UI assets
- `test/smoke/console_test.go`: scaffold route smoke test

last_verified:
- `gofmt -w .`
- `go test ./...`
