(function merchantDashboard(global) {
  const UI = global.NitroliteUI;
  const state = {
    health: null,
    overview: null,
    lease: null,
    heartbeatTimer: null,
  };

  function currentAsset() {
    return state.overview?.selected_asset || "yusd";
  }

  function demoAssets() {
    if (!state.overview) {
      return [];
    }
    const homeBlockchains = state.overview.wallet?.home_blockchains || {};
    return (state.overview.assets || []).filter((asset) => homeBlockchains[asset.symbol]);
  }

  function leaseOwned() {
    return Boolean(state.lease?.held && state.lease?.owned_by_current_session);
  }

  function updateBanner(message, isError) {
    const banner = document.getElementById("dashboard-banner");
    if (!banner) {
      return;
    }
    banner.textContent = message;
    banner.classList.toggle("warning", Boolean(isError));
  }

  async function fetchHealth() {
    try {
      state.health = await UI.requestJSON("/healthz");
    } catch (err) {
      state.health = { status: "error", clearnode: "disconnected", signer: "", error: err.message };
    }
  }

  async function refreshAll() {
    await fetchHealth();
    try {
      const [overview, lease] = await Promise.all([
        UI.requestJSON(`/api/v1/dashboard/overview?asset=${encodeURIComponent(currentAsset())}`),
        UI.requestJSON("/api/v1/operator/lease/status"),
      ]);
      state.overview = overview;
      state.lease = lease;
      render();
    } catch (err) {
      state.overview = null;
      try {
        state.lease = await UI.requestJSON("/api/v1/operator/lease/status");
      } catch (leaseErr) {
        state.lease = null;
      }
      render();
      updateBanner(`Dashboard overview unavailable: ${err.message}. Health remains visible above.`, true);
      return;
    }
    updateBanner("Merchant dashboard is live. Create a pay link, watch the order reserve, then settle, refund, or pay out.", false);
  }

  function renderStatusCards() {
    UI.setText("#dashboard-health", state.health?.clearnode === "connected" ? "Connected" : "Disconnected");
    UI.setText("#dashboard-signer", UI.shortAddress(state.overview?.wallet?.address || state.health?.signer || "Unavailable"));
    UI.setText("#dashboard-available", `${state.overview?.summary?.available_balance || "0"} ${(currentAsset() || "").toUpperCase()}`);
    UI.setText("#dashboard-reserved", `${state.overview?.summary?.reserved_balance || "0"} ${(currentAsset() || "").toUpperCase()}`);
  }

  function renderLease() {
    const text = document.getElementById("lease-status-text");
    const hint = document.getElementById("lease-status-hint");
    const acquire = document.getElementById("lease-acquire");
    const release = document.getElementById("lease-release");
    if (!text || !hint || !acquire || !release) {
      return;
    }
    if (!state.lease?.held) {
      text.textContent = UI.auth.unlocked ? "Available to acquire" : "Unlock writes first";
      hint.textContent = UI.auth.unlocked
        ? "Acquire the operator lease to create pay links and queue merchant mutations."
        : "The operator lease only matters after you unlock writes for this browser session.";
      acquire.hidden = !UI.auth.unlocked;
      release.hidden = true;
      return;
    }
    if (leaseOwned()) {
      text.textContent = `Held by this browser until ${UI.formatTime(state.lease.expires_at)}`;
      hint.textContent = "The dashboard will heartbeat the lease while this tab stays active.";
      acquire.hidden = true;
      release.hidden = false;
    } else {
      text.textContent = `Held by another browser until ${UI.formatTime(state.lease.expires_at)}`;
      hint.textContent = "This dashboard stays read-only until the active operator releases the lease or it expires.";
      acquire.hidden = true;
      release.hidden = true;
    }
  }

  function renderSummaries() {
    UI.setText("#dashboard-balance-summary", "");
    const balanceRoot = document.getElementById("dashboard-balance-summary");
    const channelRoot = document.getElementById("dashboard-channel-summary");
    const stateRoot = document.getElementById("dashboard-state-summary");
    const workRoot = document.getElementById("dashboard-work-summary");
    if (!balanceRoot || !channelRoot || !stateRoot || !workRoot) {
      return;
    }

    if (!state.overview) {
      balanceRoot.innerHTML = '<p class="muted">Dashboard overview unavailable.</p>';
      channelRoot.innerHTML = '<p class="muted">Dashboard overview unavailable.</p>';
      stateRoot.innerHTML = '<p class="muted">Dashboard overview unavailable.</p>';
      workRoot.innerHTML = '<p class="muted">Dashboard overview unavailable.</p>';
      return;
    }

    balanceRoot.innerHTML = `
      <div class="summary-line"><span>Selected asset</span><strong>${state.overview.selected_asset.toUpperCase()}</strong></div>
      <div class="summary-line"><span>Available</span><strong>${state.overview.summary.available_balance}</strong></div>
      <div class="summary-line"><span>Reserved</span><strong>${state.overview.summary.reserved_balance}</strong></div>
      <div class="summary-line"><span>Merchant</span><strong>${state.overview.merchant_name}</strong></div>
    `;

    if (state.overview.channel) {
      channelRoot.innerHTML = `
        <div class="summary-line"><span>Status</span><strong>${UI.titleCase(state.overview.channel.status)}</strong></div>
        <div class="summary-line"><span>Version</span><strong>${state.overview.channel.stateVersion}</strong></div>
        <div class="summary-line"><span>Asset</span><strong>${state.overview.channel.asset.toUpperCase()}</strong></div>
        <div class="summary-line"><span>Chain</span><strong>${state.overview.channel.blockchainID}</strong></div>
      `;
    } else {
      channelRoot.innerHTML = '<p class="muted">No home channel returned for the selected asset.</p>';
    }

    if (state.overview.latest_state) {
      stateRoot.innerHTML = `
        <div class="summary-line"><span>Version</span><strong>${state.overview.latest_state.version}</strong></div>
        <div class="summary-line"><span>Transition</span><strong>${UI.titleCase(state.overview.latest_state.transition.type)}</strong></div>
        <div class="summary-line"><span>Amount</span><strong>${state.overview.latest_state.transition.amount}</strong></div>
        <div class="summary-line"><span>Channel</span><strong>${UI.shortAddress(state.overview.latest_state.homeChannelID)}</strong></div>
      `;
    } else {
      stateRoot.innerHTML = '<p class="muted">No latest signed state available for the selected asset.</p>';
    }

    workRoot.innerHTML = `
      <div class="summary-line"><span>Pending operations</span><strong>${state.overview.summary.pending_count}</strong></div>
      <div class="summary-line"><span>Open orders</span><strong>${state.overview.summary.open_orders}</strong></div>
      <div class="summary-line"><span>Lease</span><strong>${leaseOwned() ? "Owned here" : state.lease?.held ? "Held elsewhere" : "Not held"}</strong></div>
      <div class="summary-line"><span>Latest activity</span><strong>${state.overview.latest_activity ? UI.titleCase(state.overview.latest_activity.type) : "Unavailable"}</strong></div>
    `;
  }

  function renderSelectors() {
    const assets = demoAssets();
    UI.fillSelect(document.getElementById("payment-asset"), assets, (asset) => asset.symbol, (asset) => `${asset.symbol.toUpperCase()} • ${asset.name}`, currentAsset());
    UI.fillSelect(document.getElementById("payout-asset"), assets, (asset) => asset.symbol, (asset) => `${asset.symbol.toUpperCase()} • ${asset.name}`, currentAsset());
  }

  function renderPaymentRequests() {
    const root = document.getElementById("payment-request-list");
    if (!root) {
      return;
    }
    const items = state.overview?.payment_requests || [];
    if (items.length === 0) {
      root.innerHTML = '<div class="empty-state">No hosted pay links yet.</div>';
      return;
    }
    root.innerHTML = items.map((item) => `
      <article class="stack-item">
        <div class="stack-item-row">
          <strong>${item.title}</strong>
          <span class="chip">${UI.titleCase(item.status)}</span>
        </div>
        <span>${item.amount} ${item.asset.toUpperCase()}</span>
        <a class="inline-link" href="/pay/${item.slug}" target="_blank" rel="noreferrer">Open hosted pay page</a>
      </article>
    `).join("");
  }

  function renderOrders() {
    const root = document.getElementById("orders-list");
    if (!root) {
      return;
    }
    const orders = state.overview?.orders || [];
    if (orders.length === 0) {
      root.innerHTML = '<div class="empty-state">No orders have been captured yet.</div>';
      return;
    }
    root.innerHTML = `
      <div class="table-head">
        <span>Order</span>
        <span>Status</span>
        <span>Amount</span>
        <span>Action</span>
      </div>
      ${orders.map((order) => `
        <div class="table-row" data-order-status="${order.status}">
          <div>
            <strong>${order.title}</strong>
            <p class="muted">${UI.shortAddress(order.order_id)}</p>
          </div>
          <span>${UI.titleCase(order.status)}</span>
          <span>${order.amount} ${order.asset.toUpperCase()}</span>
          <div class="table-actions">
            <button type="button" class="ghost-button order-action" data-order-action="settle" data-order-id="${order.order_id}" ${order.status === "reserved" && leaseOwned() ? "" : "disabled"}>Settle</button>
            <button type="button" class="ghost-button order-action" data-order-action="refund" data-order-id="${order.order_id}" ${order.status === "reserved" && leaseOwned() ? "" : "disabled"}>Refund</button>
          </div>
        </div>
      `).join("")}
    `;
  }

  function renderPayouts() {
    const root = document.getElementById("payout-list");
    if (!root) {
      return;
    }
    const payouts = state.overview?.payouts || [];
    if (payouts.length === 0) {
      root.innerHTML = '<div class="empty-state">No payouts queued yet.</div>';
      return;
    }
    root.innerHTML = payouts.map((payout) => `
      <article class="stack-item">
        <div class="stack-item-row">
          <strong>${payout.amount} ${payout.asset.toUpperCase()}</strong>
          <span class="chip">${UI.titleCase(payout.status)}</span>
        </div>
        <span>Destination: ${UI.shortAddress(payout.destination_wallet)}</span>
      </article>
    `).join("");
  }

  function renderOperations() {
    const root = document.getElementById("operations-list");
    if (!root) {
      return;
    }
    const operations = state.overview?.operations || [];
    if (operations.length === 0) {
      root.innerHTML = '<div class="empty-state">No operations queued yet.</div>';
      return;
    }
    root.innerHTML = operations.map((operation) => `
      <article class="stack-item">
        <div class="stack-item-row">
          <strong>${UI.titleCase(operation.type)}</strong>
          <span class="chip">${UI.titleCase(operation.status)}</span>
        </div>
        <span>Resource: ${UI.shortAddress(operation.resource_id)}</span>
        <span>${operation.error_message || `Queued at ${UI.formatTime(operation.queued_at)}`}</span>
      </article>
    `).join("");
  }

  function renderActivity() {
    const root = document.getElementById("dashboard-activity-log");
    if (!root) {
      return;
    }
    if (UI.activity.length === 0) {
      root.textContent = "No activity yet.";
      return;
    }
    root.textContent = UI.formatJSON(UI.activity);
  }

  function updateControlState() {
    const canMutate = UI.auth.unlocked && leaseOwned();
    document.querySelectorAll("#payment-request-form button[type='submit'], #payout-form button[type='submit']").forEach((node) => {
      node.disabled = !canMutate;
    });
    document.querySelectorAll(".order-action").forEach((node) => {
      node.disabled = !canMutate || node.closest(".table-row")?.dataset.orderStatus !== "reserved";
    });
  }

  async function acquireLease() {
    if (!(await UI.ensureUnlocked())) {
      return;
    }
    await UI.requestJSON("/api/v1/operator/lease/acquire", { method: "POST", body: {} });
    await refreshAll();
  }

  async function releaseLease() {
    await UI.requestJSON("/api/v1/operator/lease/release", { method: "POST", body: {} });
    await refreshAll();
  }

  async function heartbeat() {
    if (!leaseOwned()) {
      return;
    }
    try {
      state.lease = await UI.requestJSON("/api/v1/operator/lease/heartbeat", { method: "POST", body: {} });
      renderLease();
    } catch (err) {
      clearHeartbeat();
      await refreshAll();
      updateBanner(`Operator lease lost: ${err.message}`, true);
    }
  }

  function clearHeartbeat() {
    if (state.heartbeatTimer) {
      global.clearInterval(state.heartbeatTimer);
      state.heartbeatTimer = null;
    }
  }

  function syncHeartbeat() {
    clearHeartbeat();
    if (leaseOwned()) {
      state.heartbeatTimer = global.setInterval(heartbeat, 30000);
    }
  }

  async function handlePaymentRequestSubmit(event) {
    event.preventDefault();
    if (!(await UI.ensureUnlocked())) {
      return;
    }
    if (!leaseOwned()) {
      updateBanner("Acquire the operator lease before creating pay links.", true);
      return;
    }
    const form = new FormData(event.currentTarget);
    const response = await UI.requestJSON("/api/v1/payment-requests", {
      method: "POST",
      body: {
        title: form.get("title"),
        description: form.get("description"),
        asset: form.get("asset"),
        amount: form.get("amount"),
      },
    });
    document.getElementById("payment-request-result").textContent = `Created pay link ${response.pay_url}`;
    await refreshAll();
  }

  async function handlePayoutSubmit(event) {
    event.preventDefault();
    if (!(await UI.ensureUnlocked())) {
      return;
    }
    if (!leaseOwned()) {
      updateBanner("Acquire the operator lease before queuing payouts.", true);
      return;
    }
    const form = new FormData(event.currentTarget);
    const response = await UI.requestJSON("/api/v1/payouts", {
      method: "POST",
      body: {
        asset: form.get("asset"),
        amount: form.get("amount"),
        destination_wallet: form.get("destination_wallet"),
      },
    });
    document.getElementById("payout-result").textContent = `Queued payout operation ${response.operation_id}`;
    await refreshAll();
  }

  async function handleOrderAction(event) {
    const button = event.target.closest(".order-action");
    if (!button) {
      return;
    }
    if (!(await UI.ensureUnlocked())) {
      return;
    }
    if (!leaseOwned()) {
      updateBanner("Acquire the operator lease before settling or refunding orders.", true);
      return;
    }
    const action = button.dataset.orderAction;
    const orderID = button.dataset.orderId;
    await UI.requestJSON(`/api/v1/orders/${orderID}/${action}`, { method: "POST", body: {} });
    await refreshAll();
  }

  function render() {
    renderStatusCards();
    renderLease();
    renderSummaries();
    renderSelectors();
    renderPaymentRequests();
    renderOrders();
    renderPayouts();
    renderOperations();
    renderActivity();
    syncHeartbeat();
    updateControlState();
  }

  function bind() {
    document.getElementById("dashboard-refresh")?.addEventListener("click", refreshAll);
    document.getElementById("lease-acquire")?.addEventListener("click", acquireLease);
    document.getElementById("lease-release")?.addEventListener("click", releaseLease);
    document.getElementById("payment-request-form")?.addEventListener("submit", handlePaymentRequestSubmit);
    document.getElementById("payout-form")?.addEventListener("submit", handlePayoutSubmit);
    document.getElementById("orders-list")?.addEventListener("click", handleOrderAction);

    UI.onAuthChange(() => {
      renderLease();
      updateControlState();
      if (!UI.auth.unlocked) {
        clearHeartbeat();
      }
    });
    UI.onActivity(renderActivity);
  }

  async function boot() {
    UI.initUnlockModal();
    bind();
    await UI.refreshAuthStatus();
    await refreshAll();
  }

  global.addEventListener("DOMContentLoaded", boot);
})(window);
