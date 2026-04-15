# 0008 Dashboard And Pay Link Surface

scope:
- replace guided landing page with operator dashboard and add hosted pay page

done:
- `/` now renders the merchant operator dashboard
- `/pay/{slug}` now serves the hosted sandbox pay page
- `/reference` and `/advanced` nav copy updated to match the new product

verify:
- smoke tests cover `/`, `/pay/{slug}`, `/reference`, `/advanced`

decisions:
- hosted pay page stays public
- dashboard writes stay browser-session + lease oriented

next:
- wire order settlement and payout flows end to end
