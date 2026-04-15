(function payPage(global) {
  const UI = global.NitroliteUI;

  function slugFromPath() {
    const parts = global.location.pathname.split("/").filter(Boolean);
    return parts.length >= 2 && parts[0] === "pay" ? parts[1] : "";
  }

  function paymentStatus(page) {
    return page?.payment_request?.status || "unknown";
  }

  function orderStatus(page) {
    return page?.order?.status || "not_created";
  }

  function renderBanner(page) {
    const banner = document.getElementById("pay-status");
    if (!banner) {
      return;
    }
    banner.classList.remove("warning");
    switch (paymentStatus(page)) {
    case "pending":
      banner.textContent = "This is a sandbox payment request. Clicking pay triggers a backend capture simulation; no customer wallet is required.";
      break;
    case "processing":
      banner.textContent = "Payment already submitted. The backend is reserving the order funds and waiting for chain confirmation or clearnode sync.";
      break;
    case "completed":
      banner.textContent = "Payment already captured. The operator dashboard can now settle or refund the reserved order.";
      break;
    case "expired":
      banner.textContent = "This payment request has expired.";
      banner.classList.add("warning");
      break;
    case "failed":
      banner.textContent = "The previous capture attempt failed. The operator needs to inspect the dashboard and retry if appropriate.";
      banner.classList.add("warning");
      break;
    default:
      banner.textContent = `Request status: ${UI.titleCase(paymentStatus(page))}`;
      break;
    }
  }

  function render(page) {
    const request = page?.payment_request;
    UI.setText("#pay-title", request?.title || "Payment request unavailable");
    UI.setText("#pay-merchant", page?.merchant_name ? `${page.merchant_name} hosted sandbox payment` : "Hosted sandbox payment powered by the Go backend signer.");
    UI.setText("#pay-amount", request?.amount || "Unavailable");
    UI.setText("#pay-asset", (request?.asset || "Unavailable").toUpperCase());
    UI.setText("#pay-request-status", UI.titleCase(paymentStatus(page)));
    UI.setText("#pay-order-status", UI.titleCase(orderStatus(page)));
    UI.setText("#pay-description", request?.description || "This hosted page simulates a customer payment against the backend-owned signer.");
    renderBanner(page);

    const payButton = document.getElementById("pay-submit");
    if (payButton) {
      payButton.disabled = paymentStatus(page) !== "pending";
      payButton.textContent = paymentStatus(page) === "pending" ? "Pay now" : "Payment unavailable";
    }
  }

  async function loadPage() {
    const slug = slugFromPath();
    if (!slug) {
      throw new Error("missing payment request slug");
    }
    const page = await UI.requestJSON(`/api/v1/payment-requests/${encodeURIComponent(slug)}`);
    render(page);
    return page;
  }

  async function submitPayment() {
    const slug = slugFromPath();
    const result = await UI.requestJSON(`/api/v1/payment-requests/${encodeURIComponent(slug)}/pay`, {
      method: "POST",
      body: {},
    });
    document.getElementById("pay-result").textContent = UI.formatJSON(result);
    await loadPage();
  }

  async function boot() {
    document.getElementById("pay-submit")?.addEventListener("click", async () => {
      try {
        await submitPayment();
      } catch (err) {
        document.getElementById("pay-result").textContent = UI.formatJSON(err.payload || { error: err.message });
      }
    });

    try {
      await loadPage();
    } catch (err) {
      document.getElementById("pay-result").textContent = UI.formatJSON(err.payload || { error: err.message });
      render({ payment_request: { status: "failed" } });
    }
  }

  global.addEventListener("DOMContentLoaded", boot);
})(window);
