# 0011 AI DX Refresh

scope:
- align docs, rules, skills, and checkpoints with the merchant-settlement product

done:
- refreshed `README.md`, `AGENTS.md`, `CLAUDE.md`
- refreshed `docs/api.md` and `docs/architecture.md`
- updated `.claude/rules/*`
- replaced protocol-demo skills with merchant-product skills
- updated `.codex/STATE.md`

verify:
- docs and skill names match the current repo shape

decisions:
- keep `check-sdk-health`
- keep `ux-review` and retarget it to dashboard/pay/reference/advanced surfaces

next:
- manual browser validation and commit
