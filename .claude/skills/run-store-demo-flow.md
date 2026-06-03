# /run-store-demo-flow

Sequence:
1. open `/`
2. Connect wallet
3. create or load the wallet store session
4. deposit into the app session
5. purchase item `1` for `0.9` YUSD
6. read purchased content
7. withdraw remaining user balance

Require:
- funded demo wallet
- reachable clearnode
- selected asset supported by the catalog
- direct `@yellow-org/sdk` wallet signing available

Output:
- step result
- failure point
- next action
