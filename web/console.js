async function fetchJSON(url) {
  const response = await fetch(url);
  const data = await response.json();
  if (!response.ok) {
    throw new Error(data?.error?.message || "request failed");
  }
  return data;
}

async function refresh() {
  try {
    const [health, wallet] = await Promise.all([
      fetchJSON("/healthz"),
      fetchJSON("/api/v1/wallet"),
    ]);

    document.getElementById("clearnode").textContent = health.clearnode;
    document.getElementById("signer").textContent = health.signer || "pending signer wiring";
    document.getElementById("wallet").textContent = JSON.stringify(wallet, null, 2);

    try {
      await fetchJSON("/readyz");
      document.getElementById("ready").textContent = "ready";
    } catch {
      document.getElementById("ready").textContent = "not ready";
    }
  } catch (error) {
    document.getElementById("clearnode").textContent = "error";
    document.getElementById("ready").textContent = "error";
    document.getElementById("wallet").textContent = error.message;
  }
}

void refresh();

