# API Contract Rules

- prefix all API routes with `/api/v1`
- amounts are decimal strings, never floats
- error envelope:
```json
{"error":{"code":"snake_case","message":"lowercase message"}}
```
- auth tiers:
  - `requireWriteAccess()`: bearer key or unlocked write-session cookie
  - `requireLeaseOwnership()`: write-session cookie present and token matches `operator_lease.session_token`
  - `requireOperatorLease()`: valid write-session cookie plus active lease ownership
- raw mutation routes in `/advanced` use `requireWriteAccess()`
- merchant operator writes use `requireOperatorLease()`
- `POST /api/v1/operator/lease/acquire` uses `requireWriteAccess()`
- `POST /api/v1/operator/lease/release` and `/heartbeat` use `requireLeaseOwnership()`
- `POST /api/v1/payment-requests/{slug}/pay` is intentionally public
- `POST /api/v1/payment-requests` returns `201` with `payment_request_id`, `slug`, `pay_url`, `status`
- async merchant mutations return `202` with `operation_id`, `resource_id`, `status`
- `GET /api/v1/dashboard/overview` is SDK-anchored and returns `503` when required SDK reads fail
