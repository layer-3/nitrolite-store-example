(function advancedConsole(global) {
  const UI = global.NitroliteUI;
  const state = {
    wallet: null,
    assets: [],
    apps: [],
    sessions: [],
    selectedAsset: "",
    selectedSessionID: "",
  };

  function setResult(formID, value, isError) {
    const node = document.querySelector(`#${formID} [data-result]`);
    if (!node) {
      return;
    }
    node.textContent = typeof value === "string" ? value : UI.formatJSON(value);
    node.classList.toggle("error-text", Boolean(isError));
  }

  function parseJSONArray(raw, label) {
    try {
      const parsed = JSON.parse(raw);
      if (!Array.isArray(parsed)) {
        throw new Error("must be a JSON array");
      }
      return parsed;
    } catch (err) {
      throw new Error(`${label}: ${err.message}`);
    }
  }

  function chainName(chainID) {
    const blockchains = JSON.parse(document.getElementById("advanced-node-config")?.dataset.blockchains || "[]");
    const found = blockchains.find((item) => item.id === chainID);
    return found ? UI.titleCase(found.name) : `Chain ${chainID}`;
  }

  function currentSession() {
    return state.sessions.find((session) => session.app_session_id === state.selectedSessionID) || null;
  }

  function focusedAssets() {
    const homeBlockchains = state.wallet?.homeBlockchains || {};
    const configured = state.assets.filter((asset) => homeBlockchains[asset.symbol]);
    if (configured.length === 0) {
      return state.assets;
    }
    return configured.sort((left, right) => {
      if (left.symbol === "yusd") {
        return -1;
      }
      if (right.symbol === "yusd") {
        return 1;
      }
      return left.symbol.localeCompare(right.symbol);
    });
  }

  function populateSelectors() {
    const assetSelect = document.getElementById("advanced-asset-select");
    const chainSelect = document.getElementById("advanced-chain-select");
    const appSelect = document.getElementById("advanced-app-select");
    const sessionSelect = document.getElementById("advanced-session-select");
    const assets = focusedAssets();

    const preferredAsset = state.selectedAsset || (assets.some((asset) => asset.symbol === "yusd") ? "yusd" : assets[0]?.symbol || "");
    UI.fillSelect(assetSelect, assets, (asset) => asset.symbol, (asset) => `${asset.symbol.toUpperCase()} • ${asset.name}`, preferredAsset);
    state.selectedAsset = assetSelect?.value || state.selectedAsset;

    const asset = assets.find((item) => item.symbol === state.selectedAsset);
    const chainOptions = [];
    const seen = new Set();
    (asset?.tokens || []).forEach((token) => {
      if (!seen.has(token.blockchainId)) {
        seen.add(token.blockchainId);
        chainOptions.push({ id: token.blockchainId });
      }
    });
    if (chainOptions.length === 0 && asset?.suggestedBlockchainID) {
      chainOptions.push({ id: asset.suggestedBlockchainID });
    }
    UI.fillSelect(
      chainSelect,
      chainOptions,
      (item) => String(item.id),
      (item) => chainName(item.id),
      String(state.wallet?.homeBlockchains?.[state.selectedAsset] || asset?.suggestedBlockchainID || ""),
    );
    UI.fillSelect(appSelect, state.apps, (item) => item.app_id, (item) => item.app_id, appSelect?.value || "default");
    UI.fillSelect(
      sessionSelect,
      state.sessions,
      (item) => item.app_session_id,
      (item) => `${item.app_session_id.slice(0, 10)}… • ${item.status} • v${item.version}`,
      state.selectedSessionID,
    );
    state.selectedSessionID = sessionSelect?.value || "";

    const createAppInput = document.querySelector('#create-session-form input[name="application_id"]');
    if (createAppInput && appSelect?.value) {
      createAppInput.value = appSelect.value;
    }

    const assetDefaults = {
      createAllocation: `[{"asset":"${state.selectedAsset || "yusd"}","amount":"0.5"}]`,
      channelKeyAssets: `["${state.selectedAsset || "yusd"}"]`,
      appKeyApps: `["${appSelect?.value || "default"}"]`,
    };
    const createAllocations = document.querySelector('#create-session-form textarea[name="initial_allocations"]');
    if (createAllocations && (!createAllocations.value || createAllocations.value === "[]")) {
      createAllocations.value = assetDefaults.createAllocation;
    }
    const channelKeyAssets = document.querySelector('#channel-key-form textarea[name="assets"]');
    if (channelKeyAssets) {
      channelKeyAssets.value = assetDefaults.channelKeyAssets;
    }
    const appKeyApps = document.querySelector('#app-key-form textarea[name="application_ids"]');
    if (appKeyApps) {
      appKeyApps.value = assetDefaults.appKeyApps;
    }

    hydrateSessionDefaults();
  }

  function hydrateSessionDefaults() {
    const session = currentSession();
    const allocations = document.querySelector('#session-state-form textarea[name="allocations"]');
    const sessionData = document.querySelector('#session-state-form textarea[name="session_data"]');
    if (session && allocations) {
      allocations.value = UI.formatJSON(session.allocations || []);
    }
    if (session && sessionData && (!sessionData.value || sessionData.value === "{}")) {
      sessionData.value = session.session_data || "{}";
    }
  }

  async function refreshAll() {
    const asset = state.selectedAsset || "";
    const [health, wallet, nodeConfig, assets, balances, channel, latestState, apps, sessions, channelKeys, appKeys] = await Promise.all([
      UI.requestJSON("/healthz"),
      UI.requestJSON("/api/v1/wallet"),
      UI.requestJSON("/api/v1/node/config"),
      UI.requestJSON("/api/v1/node/assets"),
      UI.requestJSON("/api/v1/balances"),
      asset ? UI.requestJSON(`/api/v1/channel?asset=${encodeURIComponent(asset)}`).catch((error) => ({ error: error.message })) : Promise.resolve({ error: "select an asset" }),
      asset ? UI.requestJSON(`/api/v1/channel/state?asset=${encodeURIComponent(asset)}&only_signed=true`).catch((error) => ({ error: error.message })) : Promise.resolve({ error: "select an asset" }),
      UI.requestJSON("/api/v1/apps?page=1&per_page=20").catch((error) => ({ apps: [], error: error.message })),
      UI.requestJSON("/api/v1/sessions?status=all&page=1&per_page=20").catch((error) => ({ sessions: [], error: error.message })),
      UI.requestJSON("/api/v1/session-keys/channel").catch((error) => ({ states: [], error: error.message })),
      UI.requestJSON("/api/v1/session-keys/app").catch((error) => ({ states: [], error: error.message })),
      UI.refreshAuthStatus(),
    ]);

    state.wallet = {
      address: wallet.address,
      homeBlockchains: wallet.homeBlockchains || wallet.home_blockchains || {},
    };
    state.assets = assets.assets || [];
    state.apps = apps.apps || [];
    state.sessions = sessions.sessions || [];

    document.getElementById("advanced-node-config").dataset.blockchains = UI.formatJSON(nodeConfig.blockchains || []);
    UI.renderJSON("#advanced-health", health);
    UI.renderJSON("#advanced-wallet", wallet);
    UI.renderJSON("#advanced-node-config", nodeConfig);
    UI.renderJSON("#advanced-assets", assets);
    UI.renderJSON("#advanced-balances", balances);
    UI.renderJSON("#advanced-channel", channel);
    UI.renderJSON("#advanced-latest-state", latestState);
    UI.renderJSON("#advanced-apps", apps);
    UI.renderJSON("#advanced-sessions", sessions);
    UI.renderJSON("#advanced-channel-keys", channelKeys);
    UI.renderJSON("#advanced-app-keys", appKeys);
    populateSelectors();
  }

  async function runWrite(formID, action) {
    try {
      if (!(await UI.ensureUnlocked())) {
        setResult(formID, "Unlock write actions to continue.", false);
        return;
      }
      const result = await action();
      setResult(formID, result, false);
      await refreshAll();
    } catch (err) {
      setResult(formID, err.message, true);
    }
  }

  function bindForm(formID, handler) {
    document.getElementById(formID)?.addEventListener("submit", (event) => {
      event.preventDefault();
      const formData = new FormData(event.currentTarget);
      void runWrite(formID, () => handler(formData));
    });
  }

  function bindEvents() {
    document.getElementById("advanced-refresh")?.addEventListener("click", () => {
      void refreshAll();
    });
    document.getElementById("advanced-asset-select")?.addEventListener("change", (event) => {
      state.selectedAsset = event.target.value;
      void refreshAll();
    });
    document.getElementById("advanced-app-select")?.addEventListener("change", () => populateSelectors());
    document.getElementById("advanced-session-select")?.addEventListener("change", (event) => {
      state.selectedSessionID = event.target.value;
      hydrateSessionDefaults();
    });

    bindForm("approve-form", async (formData) => {
      return UI.requestJSON("/api/v1/approve", {
        method: "POST",
        body: {
          blockchain_id: Number(document.getElementById("advanced-chain-select")?.value),
          asset: state.selectedAsset,
          amount: String(formData.get("amount") || "").trim(),
        },
      });
    });
    bindForm("deposit-form", async (formData) => {
      return UI.requestJSON("/api/v1/deposit", {
        method: "POST",
        body: {
          blockchain_id: Number(document.getElementById("advanced-chain-select")?.value),
          asset: state.selectedAsset,
          amount: String(formData.get("amount") || "").trim(),
        },
      });
    });
    bindForm("withdraw-form", async (formData) => {
      return UI.requestJSON("/api/v1/withdraw", {
        method: "POST",
        body: {
          blockchain_id: Number(document.getElementById("advanced-chain-select")?.value),
          asset: state.selectedAsset,
          amount: String(formData.get("amount") || "").trim(),
        },
      });
    });
    bindForm("transfer-form", async (formData) => {
      return UI.requestJSON("/api/v1/transfer", {
        method: "POST",
        body: {
          recipient: String(formData.get("recipient") || "").trim(),
          asset: state.selectedAsset,
          amount: String(formData.get("amount") || "").trim(),
        },
      });
    });
    bindForm("checkpoint-form", async () => {
      return UI.requestJSON("/api/v1/checkpoint", {
        method: "POST",
        body: { asset: state.selectedAsset },
      });
    });
    bindForm("close-channel-form", async () => {
      return UI.requestJSON("/api/v1/channel/close", {
        method: "POST",
        body: { asset: state.selectedAsset },
      });
    });
    bindForm("register-app-form", async (formData) => {
      return UI.requestJSON("/api/v1/apps/register", {
        method: "POST",
        body: {
          app_id: String(formData.get("app_id") || "").trim(),
          metadata: String(formData.get("metadata") || "").trim(),
          creation_approval_not_required: formData.get("creation_approval_not_required") === "on",
        },
      });
    });
    bindForm("create-session-form", async (formData) => {
      return UI.requestJSON("/api/v1/sessions", {
        method: "POST",
        body: {
          application_id: String(formData.get("application_id") || "").trim(),
          initial_allocations: parseJSONArray(String(formData.get("initial_allocations") || "[]"), "initial_allocations"),
          session_data: String(formData.get("session_data") || "{}"),
        },
      });
    });
    bindForm("session-deposit-form", async (formData) => {
      return UI.requestJSON(`/api/v1/sessions/${encodeURIComponent(state.selectedSessionID)}/deposit`, {
        method: "POST",
        body: {
          asset: state.selectedAsset,
          amount: String(formData.get("amount") || "").trim(),
        },
      });
    });
    bindForm("session-state-form", async (formData) => {
      return UI.requestJSON(`/api/v1/sessions/${encodeURIComponent(state.selectedSessionID)}/state`, {
        method: "POST",
        body: {
          allocations: parseJSONArray(String(formData.get("allocations") || "[]"), "allocations"),
          session_data: String(formData.get("session_data") || "{}"),
        },
      });
    });
    bindForm("session-close-form", async () => {
      return UI.requestJSON(`/api/v1/sessions/${encodeURIComponent(state.selectedSessionID)}/close`, {
        method: "POST",
        body: {},
      });
    });
    bindForm("channel-key-form", async (formData) => {
      return UI.requestJSON("/api/v1/session-keys/channel", {
        method: "POST",
        body: {
          session_key: String(formData.get("session_key") || "").trim(),
          assets: parseJSONArray(String(formData.get("assets") || "[]"), "assets"),
          expires_at: String(formData.get("expires_at") || "").trim(),
        },
      });
    });
    bindForm("app-key-form", async (formData) => {
      return UI.requestJSON("/api/v1/session-keys/app", {
        method: "POST",
        body: {
          session_key: String(formData.get("session_key") || "").trim(),
          application_ids: parseJSONArray(String(formData.get("application_ids") || "[]"), "application_ids"),
          app_session_ids: parseJSONArray(String(formData.get("app_session_ids") || "[]"), "app_session_ids"),
          expires_at: String(formData.get("expires_at") || "").trim(),
        },
      });
    });
    bindForm("challenge-form", async (formData) => {
      return UI.requestJSON("/api/v1/challenge", {
        method: "POST",
        body: {
          asset: state.selectedAsset,
        },
      });
    });
  }

  function initActivityLog() {
    const target = document.getElementById("advanced-activity-log");
    if (!target) {
      return;
    }
    UI.onActivity(() => {
      target.textContent = UI.formatJSON(UI.activity);
    });
  }

  async function init() {
    UI.initUnlockModal();
    initActivityLog();
    bindEvents();
    await refreshAll();
  }

  document.addEventListener("DOMContentLoaded", () => {
    void init();
  });
})(window);
