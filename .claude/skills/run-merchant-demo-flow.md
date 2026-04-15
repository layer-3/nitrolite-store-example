# /run-merchant-demo-flow

Sequence:
1. unlock writes
2. acquire operator lease
3. create payment request
4. call hosted pay endpoint
5. poll operation/order status
6. settle or refund
7. optionally queue payout

Require:
- `CONSOLE_API_KEY`
- funded demo wallet
- reachable clearnode

Output:
- step result
- failure point
- next action
