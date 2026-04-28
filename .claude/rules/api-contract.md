# API Contract Rules

- active API routes are only:
  - `GET /api/store/bootstrap`
  - `POST /api/store/init`
  - `POST /api/store/update`
  - `POST /api/store/content/{id}/open`
- amounts are decimal strings, never floats
- error envelope:
```json
{"error":{"code":"snake_case","message":"lowercase message"}}
```
- store product routes are wallet-signature scoped
- `POST /api/store/init` verifies `packCreateAppSessionRequestV1(definition, session_data)`
- `POST /api/store/update` verifies `packAppStateUpdateV1(app_state_update)`
- `POST /api/store/update` dispatches from `app_state_update.intent`
- `POST /api/store/content/{id}/open` verifies a MetaMask-signed `open_content` proof before returning purchased content
- supported public actions are:
  - `deposit`
  - `purchase`
  - `withdraw`
- server must never trust client purchase price or allocation math
- supported demo assets are `yusd` and `yellow`
- one wallet-scoped store session per supported asset
