# ux-review

Purpose:
- review the store surfaces for discoverability, audience fit, docs parity, and developer evaluation quality

Required read order:
1. `.codex/STATE.md`
2. `README.md`
3. `docs/api.md`
4. `docs/architecture.md`
5. `internal/httpapi/server.go`
6. `internal/httpapi/openapi.go`
7. `internal/webui/handler.go`
8. `web/index.html`
9. `web/reference.html`
10. `web/advanced.html`
11. `web/store.js`
12. `web/console.js`
13. `web/style.css`

Required output:
- findings ordered by severity
- surface classification:
  - store app
  - API reference
  - advanced/developer tool
- friction points by audience:
  - first-time store visitor
  - returning store visitor
  - first-time developer
  - protocol/debug operator
- docs mismatches
- recommendations limited to product structure, discoverability, next-step guidance, and trust-building

Non-goals:
- no implementation
- no generic “make it prettier” comments
- no style advice without a product rationale
- no broad architecture rewrites that ignore the same-binary Go deployment model
