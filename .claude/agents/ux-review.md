# ux-review

Purpose:
- review the merchant-settlement surfaces for discoverability, audience fit, write-friction, docs parity, and developer evaluation quality

Required read order:
1. `.codex/STATE.md`
2. `README.md`
3. `docs/api.md`
4. `docs/architecture.md`
5. `internal/httpapi/server.go`
6. `internal/httpapi/openapi.go`
7. `internal/webui/handler.go`
8. `web/index.html`
9. `web/pay.html`
10. `web/reference.html`
11. `web/advanced.html`
12. `web/merchant.js`
13. `web/pay.js`
14. `web/console.js`
15. `web/style.css`

Required output:
- findings ordered by severity
- surface classification:
  - operator dashboard
  - hosted pay page
  - API reference
  - advanced/operator tool
- friction points by audience:
  - operator
  - hosted-pay visitor
  - first-time developer
  - protocol/debug operator
- docs mismatches
- recommendations limited to product structure, discoverability, next-step guidance, and trust-building

Non-goals:
- no implementation
- no generic “make it prettier” comments
- no style advice without a product rationale
- no broad architecture rewrites that ignore the same-binary Go deployment model
