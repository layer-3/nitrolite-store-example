# 0007 Merchant Data And Ops

scope:
- add SQLite store, operator lease, async operation model, and startup wiring

done:
- added `internal/store`
- added `payment_requests`, `orders`, `payouts`, `operations`, `operator_lease`
- added `store.ClearLease(ctx)` on startup
- started runner goroutine alongside `manager.Run(ctx)`

verify:
- `go test ./...`

decisions:
- resume `queued`, `waiting_chain`, `waiting_sync`
- mark `running` as failed on restart
- use backoff polling for chain/sync waits

next:
- expose merchant dashboard and hosted pay surfaces
