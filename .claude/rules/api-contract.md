# API Contract Rules

- prefix all API routes with `/api/v1`
- amounts are decimal strings, never floats
- error envelope:
```json
{"error":{"code":"snake_case","message":"lowercase message"}}
```
- raw mutation routes in `/advanced` use `requireWriteAccess()`
- `requireWriteAccess()` accepts:
  - bearer key
  - unlocked write-session cookie
- store product routes are browser-cookie scoped and do not require write unlock for normal use
- `POST /api/v1/app-session/submit-state` is the single store mutation endpoint
- `POST /api/v1/app-session/submit-state` must dispatch from `session_data.action`
- supported public actions are:
  - `deposit`
  - `purchase`
  - `user_withdraw`
- hidden developer-only action:
  - `app_withdraw`
  - only allowed when write access is present
- server must never trust client purchase price or allocation math
- one browser-scoped store session per asset
