# 0009 Order Capture And Settlement

scope:
- implement pay-link capture, order reservation, settlement/refund, and payout queueing

done:
- public pay route creates or reuses capture operations idempotently
- runner updates orders and payment requests on completion/failure
- operator routes queue settle, refund, and payout work

verify:
- route tests cover pay idempotency and merchant async contracts

decisions:
- one order maps to one app session
- payment request completion is back-updated by the runner

next:
- harden operator lease and concurrency edges
