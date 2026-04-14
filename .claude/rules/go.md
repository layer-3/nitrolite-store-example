# Go Rules

- context first for exported I/O methods
- lowercase error strings
- wrap errors with `%w`
- constructor injection only
- no business logic in HTTP handlers
- no direct `os.Getenv` outside `internal/config`
- no `sdk.Client` usage outside `internal/nitrolite`
- table-driven tests where cases > 1
- update `.codex/STATE.md` after milestone changes

