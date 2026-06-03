# ux-review

Purpose:
- review the store surfaces for discoverability, audience fit, docs parity, and developer evaluation quality

Required read order:
1. `README.md`
2. `docs/api.md`
3. `docs/architecture.md`
4. `internal/httpapi/server.go`
5. `internal/httpapi/store_public.go`
6. `internal/service/storefront_wallet.go`
7. `frontend/src/App.tsx`
8. `internal/webui/handler.go`

Required output:
- findings ordered by severity
- friction points by audience:
  - first-time shopper
  - returning shopper
  - first-time developer
- docs mismatches
- recommendations limited to signed wallet flows, store usability, docs parity, and trust-building

Non-goals:
- no implementation
- no generic “make it prettier” comments
- no style advice without a product rationale
- no broad architecture rewrites that ignore the same-binary Go deployment model
