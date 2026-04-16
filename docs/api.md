# API

Primary reference surface: [`/reference`](http://localhost:8080/reference)

Machine-readable spec: [`/openapi.json`](http://localhost:8080/openapi.json)

The OpenAPI document served by the Go backend is the source of truth for:

- route inventory
- request/response examples
- raw write-auth expectations
- store session semantics

## Main route groups

- Health / Wallet / Node
- Store
- Catalog
- Content
- Raw Channel
- Raw Sessions
- Raw Session Keys
- Auth
- Legacy

## Store routes

- `GET /api/v1/store/config`
- `GET /api/v1/catalog`
- `GET /api/v1/catalog/{id}`
- `GET /api/v1/store/session?asset=...`
- `POST /api/v1/store/session/create`
- `POST /api/v1/app-session/submit-state`
- `GET /api/v1/purchases`
- `GET /api/v1/content/{id}`
- `GET /api/v1/balance?asset=...`

## Store auth model

Reads are public.

Store product routes use a browser-scoped cookie:

- `GET /api/v1/store/config` ensures the cookie exists
- store sessions and purchases are isolated per browser cookie value
- no wallet connect and no external login is required in v1

Raw mutation routes in `/advanced` use `requireWriteAccess()`:

- bearer key or unlocked write-session cookie

Hidden developer-only capability:

- `app_withdraw` still goes through `POST /api/v1/app-session/submit-state`
- but the server only allows it when write access is present

## Submit-state contract

`POST /api/v1/app-session/submit-state` is the single store mutation endpoint.

Required request fields:

- `session_id`
- `asset`
- `session_data`

`session_data` is JSON encoded as a string and must describe the action:

```json
{"action":"deposit","amount":"1.00"}
{"action":"purchase","item_id":"article-micropayments","price":"0.50"}
{"action":"user_withdraw","amount":"0.50"}
{"action":"app_withdraw","amount":"0.50"}
```

The server does not trust client business math:

- purchase price is revalidated against the seeded catalog
- duplicate purchase is rejected
- user/app withdraw amounts are capped by current allocation
- only one asset is allowed per store session

Use this file as orientation only. For concrete request and response bodies, use the embedded reference.
