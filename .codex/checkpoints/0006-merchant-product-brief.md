# 0006 Merchant Product Brief

scope:
- reposition the example from protocol walkthrough to merchant settlement service

done:
- committed to `/` as operator dashboard
- committed to `/pay/{slug}` as hosted sandbox pay page
- kept `/reference` and `/advanced` as supporting surfaces

verify:
- reflected in `README.md`, `docs/api.md`, and `docs/architecture.md`

decisions:
- hosted pay is a backend-simulated sandbox flow
- operator lease is a product requirement, not just a UX detail

next:
- add persistence, lease, and runner wiring
