# 0010 Operator Lease And Concurrency

scope:
- define and enforce lease ownership plus single-writer semantics

done:
- acquire uses write access only
- release/heartbeat require lease ownership
- merchant writes require active operator lease
- lease is cleared on startup

verify:
- route tests cover second-session release/heartbeat conflicts

decisions:
- TTL is 10 minutes
- heartbeat interval is 30 seconds
- background-tab throttling is a known v1 risk

next:
- refresh repo DX and agent assets
