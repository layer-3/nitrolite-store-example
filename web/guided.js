(function guidedDemo(global) {
  const UI = global.NitroliteUI;
  const state = {
    overview: null,
    blockchains: [],
    selectedAsset: "",
    history: [],
    pendingApproval: null,
    pendingCheckpoint: null,
  };

  function numberValue(raw) {
    const value = Number(raw);
    return Number.isFinite(value) ? value : 0;
  }

  function findAsset(symbol) {
    return state.overview?.assets?.find((asset) => asset.symbol === symbol) || null;
  }

  function homeChainForAsset(symbol) {
    return state.overview?.wallet?.home_blockchains?.[symbol] || 0;
  }

  function demoAssets() {
    if (!state.overview) {
      return [];
    }
    const homeBlockchains = state.overview.wallet.home_blockchains || {};
    return (state.overview.assets || []).filter((asset) => {
      const chainID = homeBlockchains[asset.symbol];
      if (!chainID) {
        return false;
      }
      return (asset.tokens || []).some((token) => token.blockchainId === chainID);
    });
  }

  function demoNetworkIDs() {
    const homeBlockchains = state.overview?.wallet?.home_blockchains || {};
    return [...new Set(Object.values(homeBlockchains))].sort((left, right) => left - right);
  }

  function tokenForHomeChain(asset) {
    const chainID = homeChainForAsset(asset.symbol);
    return (asset.tokens || []).find((token) => token.blockchainId === chainID) || null;
  }

  function gasAssetForChain(chainID) {
    return (state.overview?.assets || []).find((asset) =>
      (asset.tokens || []).some((token) => token.blockchainId === chainID && /^0x0+$/.test(token.address)),
    ) || null;
  }

  function findSession(sessionID) {
    return state.overview?.sessions?.find((session) => session.app_session_id === sessionID) || null;
  }

  function chainName(chainID) {
    const blockchain = state.blockchains.find((item) => item.id === chainID);
    return blockchain ? UI.titleCase(blockchain.name) : `Chain ${chainID}`;
  }

  function pushHistory(message) {
    state.history.unshift({
      timestamp: new Date().toISOString(),
      message,
    });
    state.history = state.history.slice(0, 10);
    renderHistory();
  }

  function rememberApproval(asset, chainID, amount, txHash) {
    state.pendingApproval = {
      asset,
      chainID,
      amount,
      txHash,
      createdAt: new Date().toISOString(),
    };
  }

  function clearPendingApproval() {
    state.pendingApproval = null;
  }

  function rememberCheckpoint(asset, version, txHash) {
    state.pendingCheckpoint = {
      asset,
      version,
      txHash,
      createdAt: new Date().toISOString(),
    };
  }

  function clearPendingCheckpoint() {
    state.pendingCheckpoint = null;
  }

  function approvalStillRelevant() {
    return Boolean(
      state.pendingApproval &&
        state.pendingApproval.asset === state.selectedAsset &&
        state.pendingApproval.chainID === homeChainForAsset(state.selectedAsset),
    );
  }

  function checkpointStillRelevant() {
    return Boolean(
      state.pendingCheckpoint &&
        state.pendingCheckpoint.asset === state.selectedAsset &&
        state.pendingCheckpoint.version === (state.overview?.latest_state?.version || 0),
    );
  }

  function checkpointAllowanceHint() {
    if (!approvalStillRelevant()) {
      return "";
    }
    return `The latest approval tx ${UI.shortAddress(state.pendingApproval.txHash)} may still be confirming on-chain. If checkpoint fails on allowance, wait for confirmation and retry.`;
  }

  function reconcilePendingMutations() {
    const channelVersion = state.overview?.channel?.stateVersion || 0;
    const latestVersion = state.overview?.latest_state?.version || 0;

    if (state.pendingCheckpoint) {
      if (
        state.pendingCheckpoint.asset !== state.selectedAsset ||
        channelVersion >= state.pendingCheckpoint.version ||
        latestVersion > state.pendingCheckpoint.version
      ) {
        clearPendingCheckpoint();
      }
    }

    if (state.pendingApproval && state.pendingApproval.asset !== state.selectedAsset) {
      clearPendingApproval();
    }
  }

  function renderHistory() {
    const list = document.getElementById("guided-history");
    if (!list) {
      return;
    }
    if (state.history.length === 0) {
      list.innerHTML = '<li class="muted">No guided actions yet.</li>';
      return;
    }
    list.innerHTML = state.history
      .map((entry) => `<li><strong>${UI.formatTime(entry.timestamp)}</strong><span>${entry.message}</span></li>`)
      .join("");
  }

  function renderStatus() {
    const overview = state.overview;
    if (!overview) {
      return;
    }
    UI.setText("#status-clearnode", overview.status.connected ? "Connected" : "Disconnected");
    UI.setText("#status-ready", overview.status.ready ? "Ready" : "Not ready");
    UI.setText("#status-signer", UI.shortAddress(overview.wallet.address));
    UI.setText("#status-next-action", UI.titleCase(overview.channel_guidance.next_action || "review_supported_assets"));
  }

  function renderAssets() {
    const assetRoot = document.getElementById("asset-cards");
    const networkRoot = document.getElementById("network-cards");
    if (!assetRoot || !networkRoot || !state.overview) {
      return;
    }

    const networkCards = demoNetworkIDs().map((chainID) => {
      const gasAsset = gasAssetForChain(chainID);
      const tokens = demoAssets()
        .filter((asset) => homeChainForAsset(asset.symbol) === chainID)
        .map((asset) => asset.symbol.toUpperCase())
        .join(", ");
      return `
        <article class="asset-card">
          <div class="asset-card-header">
            <div>
              <strong>${chainName(chainID)}</strong>
              <p>Configured demo network</p>
            </div>
            <span class="chip">Chain ${chainID}</span>
          </div>
          <p class="muted">Demo tokens on this network: ${tokens || "None"}.</p>
          <p class="muted">Gas asset: ${gasAsset ? gasAsset.symbol.toUpperCase() : "Native token not exposed"}.</p>
        </article>
      `;
    });

    const cards = demoAssets().map((asset) => {
      const homeChain = homeChainForAsset(asset.symbol);
      const token = tokenForHomeChain(asset);
      return `
        <article class="asset-card ${asset.symbol === state.selectedAsset ? "selected" : ""}">
          <div class="asset-card-header">
            <div>
              <strong>${asset.symbol.toUpperCase()}</strong>
              <p>${asset.name}</p>
            </div>
            <span class="chip">${chainName(homeChain)}</span>
          </div>
          <p class="muted">Configured home chain: ${chainName(homeChain)}</p>
          <ul class="detail-list">
            <li>ERC-20 address: ${token ? UI.shortAddress(token.address) : "Unavailable"}</li>
            <li>Decimals: ${token ? token.decimals : asset.decimals}</li>
          </ul>
        </article>
      `;
    });
    networkRoot.innerHTML = networkCards.join("") || '<div class="empty-state">No configured demo networks.</div>';
    assetRoot.innerHTML = cards.join("") || '<div class="empty-state">No configured demo tokens.</div>';
  }

  function renderBalances() {
    const root = document.getElementById("balance-list");
    if (!root || !state.overview) {
      return;
    }
    const balances = state.overview.balances || [];
    if (balances.length === 0) {
      root.innerHTML = '<div class="empty-state">No channel balances reported yet.</div>';
      return;
    }
    root.innerHTML = balances
      .map((balance) => `
        <div class="stack-item ${balance.asset === state.selectedAsset ? "selected" : ""}">
          <strong>${balance.balance}</strong>
          <span>${balance.asset.toUpperCase()}</span>
        </div>
      `)
      .join("");
  }

  function renderChannelSummary() {
    const root = document.getElementById("channel-summary");
    const overview = state.overview;
    if (!root || !overview) {
      return;
    }
    if (!overview.channel) {
      root.innerHTML = `<p class="muted">${overview.channel_guidance.description}</p>`;
      return;
    }
    root.innerHTML = `
      <div class="summary-line"><span>Status</span><strong>${UI.titleCase(overview.channel.status)}</strong></div>
      <div class="summary-line"><span>Version</span><strong>${overview.channel.stateVersion}</strong></div>
      <div class="summary-line"><span>Asset</span><strong>${overview.channel.asset.toUpperCase()}</strong></div>
      <div class="summary-line"><span>Home chain</span><strong>${chainName(overview.channel.blockchainID)}</strong></div>
      <p class="hint ${overview.channel_guidance.sync_pending ? "warning" : ""}">${overview.channel_guidance.description}</p>
    `;
  }

  function renderLatestState() {
    const root = document.getElementById("latest-state-summary");
    const overview = state.overview;
    if (!root || !overview) {
      return;
    }
    if (!overview.latest_state) {
      root.innerHTML = '<p class="muted">No signed state returned yet for the selected asset.</p>';
      return;
    }
    root.innerHTML = `
      <div class="summary-line"><span>Version</span><strong>${overview.latest_state.version}</strong></div>
      <div class="summary-line"><span>Transition</span><strong>${UI.titleCase(overview.latest_state.transition.type)}</strong></div>
      <div class="summary-line"><span>Amount</span><strong>${overview.latest_state.transition.amount}</strong></div>
      <div class="summary-line"><span>Channel</span><strong>${UI.shortAddress(overview.latest_state.homeChannelID)}</strong></div>
      <p class="hint">${overview.latest_state.userSig && overview.latest_state.nodeSig ? "Both signatures are present on the latest state." : "Waiting for both signatures on the latest state."}</p>
    `;
  }

  function renderLatestActivity() {
    const root = document.getElementById("activity-summary");
    const overview = state.overview;
    if (!root || !overview) {
      return;
    }
    if (!overview.latest_activity) {
      root.innerHTML = '<p class="muted">No recent activity returned by the node yet.</p>';
      return;
    }
    root.innerHTML = `
      <div class="summary-line"><span>Type</span><strong>${UI.titleCase(overview.latest_activity.type)}</strong></div>
      <div class="summary-line"><span>Asset</span><strong>${overview.latest_activity.asset.toUpperCase()}</strong></div>
      <div class="summary-line"><span>Amount</span><strong>${overview.latest_activity.amount}</strong></div>
      <div class="summary-line"><span>When</span><strong>${UI.formatTime(overview.latest_activity.timestamp)}</strong></div>
      <p class="hint">Latest transaction id: ${UI.shortAddress(overview.latest_activity.id)}</p>
    `;
  }

  function renderSelectors() {
    if (!state.overview) {
      return;
    }
    const assetSelect = document.getElementById("guided-asset-select");
    const chainSelect = document.getElementById("guided-chain-select");
    const appSelect = document.getElementById("guided-app-select");
    const sessionSelect = document.getElementById("guided-session-select");
    const availableDemoAssets = demoAssets();

    UI.fillSelect(assetSelect, availableDemoAssets, (asset) => asset.symbol, (asset) => `${asset.symbol.toUpperCase()} • ${asset.name}`, state.selectedAsset);
    state.selectedAsset = assetSelect?.value || state.selectedAsset;

    const selectedAsset = findAsset(state.selectedAsset);
    const configuredChainID = selectedAsset ? homeChainForAsset(selectedAsset.symbol) : 0;
    const chainOptions = configuredChainID ? [{ id: configuredChainID }] : [];
    UI.fillSelect(chainSelect, chainOptions, (item) => String(item.id), (item) => chainName(item.id), String(configuredChainID || ""));

    UI.fillSelect(appSelect, state.overview.apps || [], (item) => item.app_id, (item) => `${item.app_id}${item.creation_approval_not_required ? " • no owner approval" : ""}`, "default");
    UI.fillSelect(
      sessionSelect,
      state.overview.sessions || [],
      (item) => item.app_session_id,
      (item) => `${item.app_session_id.slice(0, 10)}… • ${item.status} • v${item.version}`,
      sessionSelect?.value || "",
    );
  }

  function renderGuidance() {
    const overview = state.overview;
    if (!overview) {
      return;
    }
    const channelBanner = document.getElementById("channel-guidance-banner");
    const appBanner = document.getElementById("app-guidance-banner");
    if (checkpointStillRelevant()) {
      UI.setText(
        channelBanner,
        `Checkpoint submitted for the current signed home state (${UI.shortAddress(state.pendingCheckpoint.txHash)}). Wait for chain confirmation and clearnode sync before retrying other channel transitions.`,
      );
    } else {
      const extraCheckpointHint =
        overview.channel_guidance.next_action === "checkpoint" ? checkpointAllowanceHint() : "";
      UI.setText(channelBanner, [overview.channel_guidance.description, extraCheckpointHint].filter(Boolean).join(" "));
    }
    UI.setText(appBanner, overview.app_guidance.description);
    channelBanner?.classList.toggle("warning", Boolean(overview.channel_guidance.sync_pending));
    appBanner?.classList.remove("warning");
  }

  function setStepResult(formID, message, isError) {
    const node = document.querySelector(`#${formID} [data-result]`);
    if (!node) {
      return;
    }
    node.textContent = message;
    node.classList.toggle("error-text", Boolean(isError));
  }

  function setStepNote(formID, message, isWarning) {
    const node = document.querySelector(`#${formID} [data-note]`);
    if (!node) {
      return;
    }
    node.textContent = message || "";
    node.classList.toggle("warning", Boolean(isWarning));
  }

  function friendlyWriteError(formID, err) {
    const message = err?.message || "request failed";
    if (formID === "guided-checkpoint-form" && message.includes("allowance is not sufficient to cover the deposit amount")) {
      return "Checkpoint failed because the approval is not usable on-chain yet. Wait for the approval transaction to confirm, or approve again with enough allowance, then retry checkpoint.";
    }
    if (
      (formID === "guided-deposit-form" || formID === "guided-withdraw-form" || formID === "guided-transfer-form") &&
      message.includes("ongoing state transitions check failed")
    ) {
      return "Clearnode is still reconciling the previous signed state. Wait for sync to finish before submitting the next channel transition.";
    }
    return message;
  }

  function currentBalance(asset) {
    const entry = state.overview?.balances?.find((balance) => balance.asset === asset);
    return entry ? Number(entry.balance) : 0;
  }

  function checkpointableTransition(overview) {
    const transitionType = overview?.latest_state?.transition?.type || "";
    return transitionType === "home_deposit" || transitionType === "home_withdrawal";
  }

  function updateActionState() {
    const overview = state.overview;
    if (!overview) {
      return;
    }
    const syncPending = overview.channel_guidance.sync_pending;
    const currentSession = findSession(document.getElementById("guided-session-select")?.value);
    const openSession = currentSession && currentSession.status === "open";
    const selectedAsset = state.selectedAsset;
    const canCheckpoint = Boolean(
      overview.channel &&
      overview.latest_state &&
      overview.latest_state.version > overview.channel.stateVersion &&
      checkpointableTransition(overview) &&
      !checkpointStillRelevant(),
    );
    const hasBalance = currentBalance(selectedAsset) > 0;

    document.querySelector("#guided-approve-form button").disabled = !selectedAsset;
    document.querySelector("#guided-deposit-form button").disabled = !selectedAsset || syncPending;
    document.querySelector("#guided-checkpoint-form button").disabled = !selectedAsset || !canCheckpoint;
    document.querySelector("#guided-transfer-form button").disabled = !selectedAsset || syncPending || !hasBalance;
    document.querySelector("#guided-withdraw-form button").disabled = !selectedAsset || syncPending || !hasBalance;

    document.querySelector("#guided-session-create-form button").disabled = !document.getElementById("guided-app-select")?.value || !hasBalance;
    document.querySelector("#guided-session-deposit-form button").disabled = !openSession;
    document.querySelector("#guided-session-operate-form button").disabled = !openSession;
    document.querySelector("#guided-session-close-form button").disabled = !openSession;

    setStepNote(
      "guided-approve-form",
      approvalStillRelevant()
        ? `Latest approval tx ${UI.shortAddress(state.pendingApproval.txHash)} was submitted. If the next checkpoint fails on allowance, wait for confirmation and retry.`
        : "Approve before the next deposit when you need fresh allowance.",
      approvalStillRelevant(),
    );
    setStepNote(
      "guided-deposit-form",
      syncPending
        ? "Deposit is disabled until the pending home state is checkpointed and clearnode sync completes."
        : "Deposit creates a new signed home state that must be checkpointed before transfer or withdraw.",
      syncPending,
    );
    setStepNote(
      "guided-checkpoint-form",
      checkpointStillRelevant()
        ? "Checkpoint already submitted for this signed state. Wait for chain confirmation and clearnode sync."
        : canCheckpoint
          ? checkpointAllowanceHint() || "Checkpoint is the next protocol step for the current pending home deposit or withdrawal."
          : overview.channel_guidance.sync_pending
            ? "Checkpoint is unavailable here because the pending signed state is not a checkpointable home deposit or withdrawal."
            : "No pending home deposit or withdrawal needs a checkpoint right now.",
      checkpointStillRelevant() || Boolean(checkpointAllowanceHint()) || Boolean(overview.channel_guidance.sync_pending && !canCheckpoint),
    );
    setStepNote(
      "guided-transfer-form",
      syncPending
        ? "Transfer is disabled until the current home state is checkpointed and clearnode sync completes."
        : !hasBalance
          ? "Transfer needs a positive channel balance."
          : "Transfer becomes available once the home channel is fully synced.",
      syncPending || !hasBalance,
    );
    setStepNote(
      "guided-withdraw-form",
      syncPending
        ? "Withdraw is disabled until the current home state is checkpointed and clearnode sync completes."
        : !hasBalance
          ? "Withdraw needs a positive channel balance."
          : "Withdraw creates a new signed state that must be checkpointed afterward.",
      syncPending || !hasBalance,
    );
  }

  async function refreshAll(preservedAsset) {
    const asset = preservedAsset || state.selectedAsset;
    const [overview, blockchains] = await Promise.all([
      UI.requestJSON(asset ? `/api/v1/demo/overview?asset=${encodeURIComponent(asset)}` : "/api/v1/demo/overview"),
      UI.requestJSON("/api/v1/node/blockchains").catch(() => ({ blockchains: [] })),
      UI.refreshAuthStatus(),
    ]);
    state.overview = overview;
    state.blockchains = blockchains.blockchains || [];
    state.selectedAsset = overview.selected_asset || state.selectedAsset;
    reconcilePendingMutations();
    renderStatus();
    renderAssets();
    renderBalances();
    renderChannelSummary();
    renderLatestState();
    renderLatestActivity();
    renderSelectors();
    renderGuidance();
    updateActionState();
  }

  async function runWrite(formID, action) {
    try {
      if (!(await UI.ensureUnlocked())) {
        setStepResult(formID, "Unlock write actions to continue.", false);
        return;
      }
      const message = await action();
      setStepResult(formID, message, false);
      await refreshAll(state.selectedAsset);
      pushHistory(message);
    } catch (err) {
      setStepResult(formID, friendlyWriteError(formID, err), true);
    }
  }

  function bindEvents() {
    document.getElementById("guided-refresh")?.addEventListener("click", () => {
      void refreshAll(state.selectedAsset);
    });

    document.getElementById("guided-asset-select")?.addEventListener("change", (event) => {
      state.selectedAsset = event.target.value;
      void refreshAll(state.selectedAsset);
    });

    document.getElementById("guided-session-select")?.addEventListener("change", () => {
      updateActionState();
    });

    document.getElementById("guided-approve-form")?.addEventListener("submit", (event) => {
      event.preventDefault();
      const formData = new FormData(event.currentTarget);
      void runWrite("guided-approve-form", async () => {
        const chainID = Number(document.getElementById("guided-chain-select")?.value);
        const amount = String(formData.get("amount") || "").trim();
        const response = await UI.requestJSON("/api/v1/approve", {
          method: "POST",
          body: { blockchain_id: chainID, asset: state.selectedAsset, amount },
        });
        rememberApproval(state.selectedAsset, chainID, amount, response.tx_hash);
        setStepResult(
          "guided-checkpoint-form",
          `Approval tx submitted: ${response.tx_hash}. Wait for chain confirmation if the next checkpoint fails on allowance.`,
          false,
        );
        return `Approval transaction submitted for ${amount} ${state.selectedAsset.toUpperCase()} on ${chainName(chainID)}: ${response.tx_hash}.`;
      });
    });

    document.getElementById("guided-deposit-form")?.addEventListener("submit", (event) => {
      event.preventDefault();
      const formData = new FormData(event.currentTarget);
      void runWrite("guided-deposit-form", async () => {
        const chainID = Number(document.getElementById("guided-chain-select")?.value);
        const amount = String(formData.get("amount") || "").trim();
        const response = await UI.requestJSON("/api/v1/deposit", {
          method: "POST",
          body: { blockchain_id: chainID, asset: state.selectedAsset, amount },
        });
        return response.ready_for_checkpoint
          ? approvalStillRelevant()
            ? `Deposited ${amount} ${state.selectedAsset.toUpperCase()} into the home channel. Checkpoint is the next protocol step, but the approval tx may still need on-chain confirmation.`
            : `Deposited ${amount} ${state.selectedAsset.toUpperCase()} into the home channel. Checkpoint is ready next.`
          : `Deposited ${amount} ${state.selectedAsset.toUpperCase()} into the home channel.`;
      });
    });

    document.getElementById("guided-checkpoint-form")?.addEventListener("submit", (event) => {
      event.preventDefault();
      void runWrite("guided-checkpoint-form", async () => {
        const response = await UI.requestJSON("/api/v1/checkpoint", {
          method: "POST",
          body: { asset: state.selectedAsset },
        });
        clearPendingApproval();
        rememberCheckpoint(state.selectedAsset, state.overview?.latest_state?.version || 0, response.tx_hash);
        return `Checkpoint submitted for ${state.selectedAsset.toUpperCase()}: ${response.tx_hash}. Waiting for clearnode sync.`;
      });
    });

    document.getElementById("guided-transfer-form")?.addEventListener("submit", (event) => {
      event.preventDefault();
      const formData = new FormData(event.currentTarget);
      void runWrite("guided-transfer-form", async () => {
        const recipient = String(formData.get("recipient") || "").trim();
        const amount = String(formData.get("amount") || "").trim();
        await UI.requestJSON("/api/v1/transfer", {
          method: "POST",
          body: { recipient, asset: state.selectedAsset, amount },
        });
        return `Transferred ${amount} ${state.selectedAsset.toUpperCase()} to ${UI.shortAddress(recipient)}.`;
      });
    });

    document.getElementById("guided-withdraw-form")?.addEventListener("submit", (event) => {
      event.preventDefault();
      const formData = new FormData(event.currentTarget);
      void runWrite("guided-withdraw-form", async () => {
        const chainID = Number(document.getElementById("guided-chain-select")?.value);
        const amount = String(formData.get("amount") || "").trim();
        const response = await UI.requestJSON("/api/v1/withdraw", {
          method: "POST",
          body: { blockchain_id: chainID, asset: state.selectedAsset, amount },
        });
        return response.ready_for_checkpoint
          ? `Prepared a ${amount} ${state.selectedAsset.toUpperCase()} withdrawal. Checkpoint it after review.`
          : `Prepared a ${amount} ${state.selectedAsset.toUpperCase()} withdrawal.`;
      });
    });

    document.getElementById("guided-session-create-form")?.addEventListener("submit", (event) => {
      event.preventDefault();
      const formData = new FormData(event.currentTarget);
      void runWrite("guided-session-create-form", async () => {
        const appID = document.getElementById("guided-app-select")?.value;
        const amount = String(formData.get("amount") || "").trim();
        const response = await UI.requestJSON("/api/v1/sessions", {
          method: "POST",
          body: {
            application_id: appID,
            initial_allocations: [{ asset: state.selectedAsset, amount }],
            session_data: "{}",
          },
        });
        return `Created session ${response.session_id} with ${amount} ${state.selectedAsset.toUpperCase()}.`;
      });
    });

    document.getElementById("guided-session-deposit-form")?.addEventListener("submit", (event) => {
      event.preventDefault();
      const formData = new FormData(event.currentTarget);
      void runWrite("guided-session-deposit-form", async () => {
        const sessionID = document.getElementById("guided-session-select")?.value;
        const amount = String(formData.get("amount") || "").trim();
        const response = await UI.requestJSON(`/api/v1/sessions/${encodeURIComponent(sessionID)}/deposit`, {
          method: "POST",
          body: { asset: state.selectedAsset, amount },
        });
        return `Deposited ${amount} ${state.selectedAsset.toUpperCase()} into session ${response.session_id}.`;
      });
    });

    document.getElementById("guided-session-operate-form")?.addEventListener("submit", (event) => {
      event.preventDefault();
      const formData = new FormData(event.currentTarget);
      void runWrite("guided-session-operate-form", async () => {
        const sessionID = document.getElementById("guided-session-select")?.value;
        const session = findSession(sessionID);
        const turn = numberValue(formData.get("turn"));
        if (!session) {
          throw new Error("Select an open session first.");
        }
        const allocations = (session.allocations || []).map((allocation) => ({
          participant: allocation.participant,
          asset: allocation.asset,
          amount: allocation.amount,
        }));
        const response = await UI.requestJSON(`/api/v1/sessions/${encodeURIComponent(sessionID)}/state`, {
          method: "POST",
          body: {
            allocations,
            session_data: JSON.stringify({ turn }),
          },
        });
        return `Session ${response.session_id} advanced to version ${response.version} with turn=${turn}.`;
      });
    });

    document.getElementById("guided-session-close-form")?.addEventListener("submit", (event) => {
      event.preventDefault();
      void runWrite("guided-session-close-form", async () => {
        const sessionID = document.getElementById("guided-session-select")?.value;
        const response = await UI.requestJSON(`/api/v1/sessions/${encodeURIComponent(sessionID)}/close`, {
          method: "POST",
          body: {},
        });
        return `Closed session ${response.session_id}.`;
      });
    });
  }

  async function init() {
    UI.initUnlockModal();
    bindEvents();
    await refreshAll("");
  }

  document.addEventListener("DOMContentLoaded", () => {
    void init();
  });
})(window);
