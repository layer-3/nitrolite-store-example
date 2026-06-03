# API Contract Rules

- active API routes are only:
  - `GET /api/store/bootstrap`
  - `POST /api/store/init`
  - `POST /api/store/update`
  - `GET /api/store/content/{id}`
- amounts are decimal strings, never floats
- error envelope:
```json
{"error":{"code":"snake_case","message":"lowercase message"}}
```
- store state-changing routes are wallet-signature scoped
- `POST /api/store/init` verifies `packCreateAppSessionRequestV1(definition, session_data)`
- `POST /api/store/update` verifies `packAppStateUpdateV1(app_state_update)`
- `POST /api/store/update` dispatches from `app_state_update.intent`
- deposit responses are app-signed checkpoints; bootstrap may return `pending_action` until the browser-side Clearnode submit is observed
- `GET /api/store/content/{id}` is a public demo read path and must still require a submitted wallet purchase
- do not present the content read gate as production-grade authorization
- supported public actions are:
  - `user_deposit`
  - `purchase`
  - `user_withdraw`
- server must never trust client purchase price or allocation math
- supported demo assets are `yusd` and `yellow`
- one wallet-scoped store session per supported asset
