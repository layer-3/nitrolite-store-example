# API Contract Rules

- prefix all API routes with `/api/v1`
- amounts are decimal strings, never floats
- error envelope:
```json
{"error":{"code":"snake_case","message":"lowercase message"}}
```
- mutation routes require `Authorization: Bearer <CONSOLE_API_KEY>`

