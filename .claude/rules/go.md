# Go Rules

- context first for exported I/O methods
- lowercase error strings
- wrap errors with `%w`
- constructor injection only
- no business logic in HTTP handlers
- no direct `os.Getenv` outside `internal/config`
- no SQL outside `internal/store`
- no `sdk.Client` usage outside `internal/nitrolite`
- merchant mutations run through the runner in `internal/service`
- table-driven tests where cases > 1
- update `.codex/STATE.md` after milestone changes
