# 0005 UX Review Baseline

date: 2026-04-14
slice: guided-demo + embedded-reference repositioning

findings captured before redesign:
- the default surface behaved like an operator harness / manual REST driver, not a credible SDK evaluation product
- users had to know protocol sequencing up front to avoid avoidable 422s
- raw free-text fields for asset, chain, app, session, and JSON payloads made the product feel like infrastructure plumbing
- the top-level `CONSOLE_API_KEY` paste flow showed friction before value
- supported assets, latest signed state, and recommended next action were available from the backend but not surfaced in the UI
- README and API docs were stale enough to reduce trust

product response implemented in this slice:
- `/` now acts as the guided evaluation surface
- `/reference` is an embedded OpenAPI explorer
- `/advanced` is the explicit raw/operator surface
- browser writes now unlock via cookie session instead of a permanent first-screen key box
- guided UI summarizes outcomes and disables obvious invalid next steps

follow-up:
- run a second UX review after live manual validation of the new surfaces
- record remaining friction by audience:
  - stakeholder
  - first-time developer
  - operator/debug user
