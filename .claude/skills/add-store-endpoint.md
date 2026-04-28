# /add-store-endpoint

Create:
- handler stub
- service/store method stub
- route registration
- test stubs
- store UI hook if user-facing

Keep:
- decode -> service -> encode shape
- signed wallet request semantics
- `app_state_update.intent` as the store mutation dispatcher
- `session_data.intent` as the store action detail
- decimal string amounts
