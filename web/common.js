(function bootstrapCommon(global) {
  const state = {
    auth: {
      unlocked: false,
      expires_at: "",
    },
    activity: [],
    pendingUnlockPromise: null,
    pendingUnlockResolve: null,
  };
  const authListeners = new Set();
  const activityListeners = new Set();

  function setText(selector, value) {
    const element = typeof selector === "string" ? document.querySelector(selector) : selector;
    if (element) {
      element.textContent = value;
    }
  }

  function formatJSON(value) {
    return JSON.stringify(value, null, 2);
  }

  function shortAddress(value) {
    if (!value || value.length < 12) {
      return value || "Unavailable";
    }
    return `${value.slice(0, 6)}…${value.slice(-4)}`;
  }

  function titleCase(value) {
    return String(value || "")
      .replace(/[_-]+/g, " ")
      .replace(/\b\w/g, (match) => match.toUpperCase());
  }

  function formatTime(value) {
    if (!value) {
      return "Unavailable";
    }
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) {
      return value;
    }
    return parsed.toLocaleString();
  }

  function notifyAuth() {
    const message = state.auth.unlocked
      ? state.auth.expires_at
        ? `Writes unlocked until ${formatTime(state.auth.expires_at)}`
        : "Writes unlocked for this browser session"
      : "Writes locked. Reads are public.";

    if (state.auth.unlocked) {
      closeUnlockModal();
    }

    document.querySelectorAll("[data-auth-status]").forEach((node) => {
      node.textContent = message;
    });
    document.querySelectorAll("[data-auth-open]").forEach((node) => {
      node.hidden = state.auth.unlocked;
    });
    document.querySelectorAll("[data-auth-lock]").forEach((node) => {
      node.hidden = !state.auth.unlocked;
    });
    authListeners.forEach((listener) => listener({ ...state.auth }));
  }

  function notifyActivity(entry) {
    activityListeners.forEach((listener) => listener(entry, state.activity.slice()));
  }

  function logActivity(entry) {
    const item = {
      timestamp: new Date().toISOString(),
      ...entry,
    };
    state.activity.unshift(item);
    state.activity = state.activity.slice(0, 30);
    notifyActivity(item);
  }

  async function requestJSON(url, options = {}) {
    const method = options.method || "GET";
    const headers = new Headers(options.headers || {});
    let body = options.body;

    if (body !== undefined && !(body instanceof FormData)) {
      headers.set("Content-Type", "application/json");
      body = JSON.stringify(body);
    }

    const response = await fetch(url, {
      method,
      headers,
      body,
      credentials: "same-origin",
    });
    const data = await response.json().catch(() => ({}));

    if (!options.skipLog) {
      logActivity({
        method,
        url,
        status: response.status,
        ok: response.ok,
        body: options.body ?? null,
        response: data,
      });
    }

    if (!response.ok) {
      const error = new Error(data?.error?.message || "request failed");
      error.status = response.status;
      error.payload = data;
      throw error;
    }

    return data;
  }

  async function refreshAuthStatus() {
    const data = await requestJSON("/api/v1/auth/status", { skipLog: true });
    state.auth = {
      unlocked: Boolean(data.unlocked),
      expires_at: data.expires_at || "",
    };
    notifyAuth();
    return { ...state.auth };
  }

  async function unlock(apiKey) {
    const data = await requestJSON("/api/v1/auth/unlock", {
      method: "POST",
      body: { api_key: apiKey },
      skipLog: true,
    });
    state.auth = {
      unlocked: Boolean(data.unlocked),
      expires_at: data.expires_at || "",
    };
    notifyAuth();
    return { ...state.auth };
  }

  async function lock() {
    const data = await requestJSON("/api/v1/auth/lock", {
      method: "POST",
      body: {},
      skipLog: true,
    });
    state.auth = {
      unlocked: Boolean(data.unlocked),
      expires_at: data.expires_at || "",
    };
    notifyAuth();
    return { ...state.auth };
  }

  function onAuthChange(listener) {
    authListeners.add(listener);
    listener({ ...state.auth });
    return () => authListeners.delete(listener);
  }

  function onActivity(listener) {
    activityListeners.add(listener);
    return () => activityListeners.delete(listener);
  }

  function openUnlockModal() {
    const modal = document.getElementById("unlock-modal");
    const input = document.getElementById("unlock-api-key");
    const error = document.getElementById("unlock-error");
    if (!modal || state.auth.unlocked) {
      return;
    }
    if (error) {
      error.hidden = true;
      error.textContent = "";
    }
    setModalVisibility(modal, true);
    if (input) {
      input.focus();
      input.select();
    }
  }

  function closeUnlockModal() {
    closeUnlockModalWithOptions({});
  }

  function closeUnlockModalWithOptions(options) {
    const modal = document.getElementById("unlock-modal");
    const form = document.getElementById("unlock-form");
    const error = document.getElementById("unlock-error");
    if (!modal) {
      return;
    }
    setModalVisibility(modal, false);
    if (form) {
      form.reset();
    }
    if (error) {
      error.hidden = true;
      error.textContent = "";
    }
    if (options.resolvePending) {
      settlePendingUnlock(false);
    }
  }

  function setModalVisibility(modal, isOpen) {
    modal.hidden = !isOpen;
    modal.setAttribute("aria-hidden", String(!isOpen));
    modal.style.display = isOpen ? "grid" : "none";
  }

  function settlePendingUnlock(value) {
    if (state.pendingUnlockResolve) {
      state.pendingUnlockResolve(value);
    }
    state.pendingUnlockResolve = null;
    state.pendingUnlockPromise = null;
  }

  async function ensureUnlocked() {
    if (state.auth.unlocked) {
      return true;
    }
    openUnlockModal();
    if (!state.pendingUnlockPromise) {
      state.pendingUnlockPromise = new Promise((resolve) => {
        state.pendingUnlockResolve = resolve;
      });
    }
    return state.pendingUnlockPromise;
  }

  function initUnlockModal() {
    document.querySelectorAll("[data-auth-open]").forEach((node) => {
      node.addEventListener("click", () => openUnlockModal());
    });
    document.querySelectorAll("[data-auth-lock]").forEach((node) => {
      node.addEventListener("click", async () => {
        await lock();
      });
    });

    const modal = document.getElementById("unlock-modal");
    const form = document.getElementById("unlock-form");
    const error = document.getElementById("unlock-error");
    if (!modal || !form) {
      return;
    }
    setModalVisibility(modal, false);

    modal.addEventListener("click", (event) => {
      if (event.target === modal) {
        closeUnlockModalWithOptions({ resolvePending: true });
      }
    });
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && !modal.hidden) {
        closeUnlockModalWithOptions({ resolvePending: true });
      }
    });
    document.querySelectorAll("[data-modal-close]").forEach((node) => {
      node.addEventListener("click", () => closeUnlockModalWithOptions({ resolvePending: true }));
    });
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      try {
        await unlock(new FormData(form).get("api_key"));
        settlePendingUnlock(true);
        closeUnlockModal();
      } catch (err) {
        if (error) {
          error.hidden = false;
          error.textContent = err.message;
        }
      }
    });
  }

  function fillSelect(select, items, getValue, getLabel, preferredValue) {
    if (!select) {
      return;
    }
    const previousValue = preferredValue ?? select.value;
    select.innerHTML = "";
    items.forEach((item) => {
      const option = document.createElement("option");
      option.value = getValue(item);
      option.textContent = getLabel(item);
      select.appendChild(option);
    });
    if (items.length === 0) {
      const option = document.createElement("option");
      option.value = "";
      option.textContent = "Unavailable";
      select.appendChild(option);
    }
    if ([...select.options].some((option) => option.value === previousValue)) {
      select.value = previousValue;
    }
  }

  function renderJSON(target, value) {
    const element = typeof target === "string" ? document.querySelector(target) : target;
    if (element) {
      element.textContent = formatJSON(value);
    }
  }

  global.NitroliteUI = {
    requestJSON,
    refreshAuthStatus,
    unlock,
    lock,
    ensureUnlocked,
    initUnlockModal,
    onAuthChange,
    onActivity,
    logActivity,
    setText,
    renderJSON,
    fillSelect,
    formatJSON,
    formatTime,
    shortAddress,
    titleCase,
    get auth() {
      return { ...state.auth };
    },
    get activity() {
      return state.activity.slice();
    },
  };
})(window);
