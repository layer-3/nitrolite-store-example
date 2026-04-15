(function referencePage(global) {
  const UI = global.NitroliteUI;

  async function init() {
    UI.initUnlockModal();
    await UI.refreshAuthStatus();
  }

  document.addEventListener("DOMContentLoaded", () => {
    void init();
  });
})(window);
