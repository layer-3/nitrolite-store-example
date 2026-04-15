# API

Primary reference surface: [`/reference`](http://localhost:8080/reference)

Machine-readable spec: [`/openapi.json`](http://localhost:8080/openapi.json)

The OpenAPI document served by the Go backend is the source of truth for:

- route inventory
- request/response examples
- auth tier expectations
- async vs sync mutation contracts

## Main route groups

- Health / Wallet / Node
- Merchant Dashboard
- Payment Requests
- Orders
- Payouts
- Operations
- Operator Lease
- Raw Channel
- Raw Sessions
- Raw Session Keys
- Auth
- Legacy

## Auth tiers

Reads are public.

Writes use three tiers:

- `requireWriteAccess()`
  - bearer key or unlocked browser write-session cookie
  - used by `/advanced` raw mutation routes
  - used by `POST /api/v1/operator/lease/acquire`

- `requireLeaseOwnership()`
  - write-session cookie must be present
  - cookie token must match `operator_lease.session_token`
  - used by `POST /api/v1/operator/lease/release` and `/heartbeat`

- `requireOperatorLease()`
  - valid write-session cookie
  - active lease owned by that cookie token
  - used by merchant operator writes such as:
    - `POST /api/v1/payment-requests`
    - `POST /api/v1/orders/{id}/settle`
    - `POST /api/v1/orders/{id}/refund`
    - `POST /api/v1/payouts`

Public exception:

- `POST /api/v1/payment-requests/{slug}/pay`
  - intentionally unauthenticated
  - sandbox payment simulation only

## Response contracts

`POST /api/v1/payment-requests`

- returns `201 Created`
- body includes:
  - `payment_request_id`
  - `slug`
  - `pay_url`
  - `status`

Async merchant mutations

- return `202 Accepted`
- body includes:
  - `operation_id`
  - `resource_id`
  - `status`

These routes are async:

- `POST /api/v1/payment-requests/{slug}/pay`
- `POST /api/v1/orders/{id}/settle`
- `POST /api/v1/orders/{id}/refund`
- `POST /api/v1/payouts`

Raw protocol mutation routes stay synchronous.

## Merchant semantics

- `/pay/{slug}` is a sandbox payment simulation
- no customer wallet connect is involved
- dashboard overview is SDK-anchored and returns `503` if required SDK reads fail
- operator lease is public to inspect and exclusive for merchant writes

Use this file as orientation only. For concrete request and response bodies, use the embedded reference.
