(function storeApp(global) {
  const UI = global.NitroliteUI;
  const state = {
    config: null,
    asset: "",
    session: null,
    catalog: [],
  };

  function formatAmount(value, asset) {
    const amount = value || "0";
    return `${amount} ${String(asset || "").toUpperCase()}`.trim();
  }

  function setBanner(message) {
    const node = document.getElementById("store-banner");
    if (node) {
      node.textContent = message;
    }
  }

  function selectedAsset() {
    return state.asset || state.config?.store?.default_asset || "yusd";
  }

  async function loadConfig() {
    const payload = await UI.requestJSON("/api/v1/store/config");
    state.config = payload;
    const assets = payload.store?.supported_assets || [];
    const select = document.getElementById("store-asset-select");
    if (select) {
      select.innerHTML = "";
      assets.forEach((asset) => {
        const option = document.createElement("option");
        option.value = asset;
        option.textContent = String(asset).toUpperCase();
        select.appendChild(option);
      });
      state.asset = state.asset || payload.store?.default_asset || assets[0] || "yusd";
      select.value = state.asset;
    }

    UI.setText("#selected-asset-text", String(selectedAsset()).toUpperCase());
    renderConfigSummary();
  }

  async function loadSession() {
    const asset = selectedAsset();
    const summary = await UI.requestJSON(`/api/v1/store/session?asset=${encodeURIComponent(asset)}`);
    state.session = summary.session || null;
    renderSession();
    return state.session;
  }

  async function loadCatalog() {
    const asset = selectedAsset();
    const payload = await UI.requestJSON(`/api/v1/catalog?asset=${encodeURIComponent(asset)}`);
    state.catalog = payload.items || [];
    renderCatalog(state.catalog);
  }

  async function refresh() {
    await loadConfig();
    await Promise.all([loadSession(), loadCatalog()]);
  }

  function renderConfigSummary() {
    const container = document.getElementById("store-config-summary");
    if (!container || !state.config?.store) {
      return;
    }
    const store = state.config.store;
    container.innerHTML = `
      <div class="summary-row"><span class="label">Store</span><strong>${store.store_name}</strong></div>
      <div class="summary-row"><span class="label">App ID</span><strong>${store.app_id}</strong></div>
      <div class="summary-row"><span class="label">User signer</span><strong>${UI.shortAddress(store.user_signer)}</strong></div>
      <div class="summary-row"><span class="label">App signer</span><strong>${UI.shortAddress(store.app_signer)}</strong></div>
    `;
  }

  function renderSession() {
    const session = state.session || {};
    const asset = selectedAsset();
    UI.setText("#store-available-balance", formatAmount(session.available_balance, asset));
    UI.setText("#store-user-balance", formatAmount(session.user_allocation, asset));
    UI.setText("#store-app-balance", formatAmount(session.app_allocation, asset));
    UI.setText("#store-purchase-count", String((session.purchases || []).length));
    UI.setText("#selected-asset-text", String(asset).toUpperCase());

    const status = session.status || "missing";
    UI.setText("#store-status-text", UI.titleCase(status));
    UI.setText(
      "#store-status-hint",
      status === "missing"
        ? "Create a store session for this asset before you deposit or purchase."
        : `Session ${UI.shortAddress(session.app_session_id || "")} is ready for deposit, purchase, and withdrawal.`,
    );

    const summary = document.getElementById("store-session-summary");
    if (summary) {
      summary.innerHTML = `
        <div class="summary-row"><span class="label">Session</span><strong>${session.app_session_id ? UI.shortAddress(session.app_session_id) : "Not created"}</strong></div>
        <div class="summary-row"><span class="label">Status</span><strong>${UI.titleCase(status)}</strong></div>
        <div class="summary-row"><span class="label">Version</span><strong>${session.version ?? 0}</strong></div>
        <div class="summary-row"><span class="label">Latest session data</span><strong>${session.session_data || "Unavailable"}</strong></div>
      `;
    }

    const library = document.getElementById("store-library-list");
    if (library) {
      const purchases = session.purchases || [];
      if (!purchases.length) {
        library.innerHTML = `<p class="muted">No purchased content yet for ${String(asset).toUpperCase()}.</p>`;
      } else {
        library.innerHTML = purchases
          .map((purchase) => {
            const item = state.catalog.find((entry) => entry.id === purchase.item_id);
            const title = item?.title || purchase.item_id || "Purchased item";
            const version = purchase.version ?? "Unavailable";
            const itemID = purchase.item_id || "";
            return `
            <article class="stack-item">
              <div>
                <strong>${title}</strong>
                <p class="muted">Purchased at version ${version}</p>
              </div>
              <button type="button" class="ghost-button" data-read-item="${itemID}" ${itemID ? "" : "disabled"}>Open</button>
            </article>
          `;
          })
          .join("");
      }
    }

    setBanner(
      status === "missing"
        ? "No store session exists for this asset yet. Create one, then deposit balance from the current channel."
        : `Store session ${UI.shortAddress(session.app_session_id || "")} is open. Deposit, purchase items, or withdraw remaining balance.`,
    );
  }

  function renderCatalog(items) {
    const container = document.getElementById("store-catalog-list");
    if (!container) {
      return;
    }

    if (!items.length) {
      container.innerHTML = `<p class="muted">No catalog items available for ${String(selectedAsset()).toUpperCase()}.</p>`;
      return;
    }

    container.innerHTML = items
      .map((item) => {
        const price = item.prices?.[selectedAsset()] || "0";
        return `
          <article class="catalog-card">
            <p class="eyebrow">${UI.titleCase(item.type)}</p>
            <h3>${item.title}</h3>
            <p class="muted">${item.description}</p>
            <p class="price-tag">${formatAmount(price, selectedAsset())}</p>
            <div class="catalog-actions">
              <button type="button" class="primary-button" data-buy-item="${item.id}" data-price="${price}">Purchase</button>
              <button type="button" class="ghost-button" data-read-item="${item.id}">Read</button>
            </div>
          </article>
        `;
      })
      .join("");
  }

  async function createOrLoadSession() {
    const payload = await UI.requestJSON("/api/v1/store/session/create", {
      method: "POST",
      body: { asset: selectedAsset() },
    });
    state.session = payload.session;
    UI.setText("#store-action-result", `Session ready for ${String(selectedAsset()).toUpperCase()}.`);
    renderSession();
  }

  async function submitAction(action, extras = {}) {
    const payload = await UI.requestJSON("/api/v1/app-session/submit-state", {
      method: "POST",
      body: {
        session_id: state.session?.app_session_id || "",
        asset: selectedAsset(),
        session_data: JSON.stringify({
          action,
          ...extras,
        }),
      },
    });
    state.session = payload.session;
    renderSession();
    return payload.session;
  }

  async function readContent(itemID) {
    const payload = await UI.requestJSON(`/api/v1/content/${encodeURIComponent(itemID)}?asset=${encodeURIComponent(selectedAsset())}`);
    const item = payload.item;
    const reader = document.getElementById("store-reader");
    if (reader) {
      reader.innerHTML = `
        <article class="reader-content">
          <p class="eyebrow">${UI.titleCase(item.type)}</p>
          <h3>${item.title}</h3>
          <p class="muted">${item.description}</p>
          <pre>${item.content}</pre>
        </article>
      `;
    }
  }

  function bindForms() {
    const select = document.getElementById("store-asset-select");
    if (select) {
      select.addEventListener("change", async (event) => {
        state.asset = event.target.value;
        await Promise.all([loadSession(), loadCatalog()]);
      });
    }

    const createForm = document.getElementById("store-create-form");
    if (createForm) {
      createForm.addEventListener("submit", async (event) => {
        event.preventDefault();
        try {
          await createOrLoadSession();
        } catch (error) {
          UI.setText("#store-action-result", error.message || "Failed to create session.");
        }
      });
    }

    const depositForm = document.getElementById("store-deposit-form");
    if (depositForm) {
      depositForm.addEventListener("submit", async (event) => {
        event.preventDefault();
        const amount = new FormData(depositForm).get("amount");
        try {
          await submitAction("deposit", { amount });
          UI.setText("#store-action-result", `Deposited ${amount} ${String(selectedAsset()).toUpperCase()} into the store session.`);
        } catch (error) {
          UI.setText("#store-action-result", error.message || "Deposit failed.");
        }
      });
    }

    const withdrawForm = document.getElementById("store-withdraw-form");
    if (withdrawForm) {
      withdrawForm.addEventListener("submit", async (event) => {
        event.preventDefault();
        const amount = new FormData(withdrawForm).get("amount");
        try {
          await submitAction("user_withdraw", { amount });
          UI.setText("#store-action-result", `Withdrew ${amount} ${String(selectedAsset()).toUpperCase()} from the store session.`);
        } catch (error) {
          UI.setText("#store-action-result", error.message || "Withdraw failed.");
        }
      });
    }

    const refreshButton = document.getElementById("store-refresh");
    if (refreshButton) {
      refreshButton.addEventListener("click", async () => {
        await refresh();
      });
    }

    document.addEventListener("click", async (event) => {
      const buyButton = event.target.closest("[data-buy-item]");
      if (buyButton) {
        const itemID = buyButton.getAttribute("data-buy-item");
        const price = buyButton.getAttribute("data-price");
        try {
          await submitAction("purchase", { item_id: itemID, price });
          UI.setText("#store-action-result", `Purchased ${itemID}. Access is now unlocked in the reader.`);
        } catch (error) {
          UI.setText("#store-action-result", error.message || "Purchase failed.");
        }
        return;
      }

      const readButton = event.target.closest("[data-read-item]");
      if (readButton) {
        const itemID = readButton.getAttribute("data-read-item");
        try {
          await readContent(itemID);
        } catch (error) {
          const reader = document.getElementById("store-reader");
          if (reader) {
            reader.innerHTML = `<p class="error-text">${error.message || "Unable to open content."}</p>`;
          }
        }
      }
    });
  }

  function initActivityLog() {
    const log = document.getElementById("store-activity-log");
    if (!log) {
      return;
    }
    UI.onActivity((entry, entries) => {
      log.textContent = JSON.stringify(entries, null, 2);
    });
  }

  function bindCopyLog() {
    const button = document.getElementById("store-copy-log");
    const log = document.getElementById("store-activity-log");
    if (!button || !log) {
      return;
    }
    button.addEventListener("click", async () => {
      const originalLabel = button.textContent;
      try {
        await navigator.clipboard.writeText(log.textContent || "");
        button.textContent = "Copied";
      } catch (error) {
        button.textContent = "Copy failed";
      }
      global.setTimeout(() => {
        button.textContent = originalLabel;
      }, 1600);
    });
  }

  async function init() {
    UI.initUnlockModal();
    await UI.refreshAuthStatus();
    bindForms();
    initActivityLog();
    bindCopyLog();
    await refresh();
  }

  global.addEventListener("DOMContentLoaded", () => {
    init().catch((error) => {
      setBanner(error.message || "Failed to initialize store.");
      UI.setText("#store-action-result", error.message || "Initialization failed.");
    });
  });
})(window);
