// Holocron's own script, for what htmx does not cover: running as an app
// installed on the phone's home screen.
(() => {
  "use strict";

  if ("serviceWorker" in navigator) {
    navigator.serviceWorker.register("/sw.js", { scope: "/" }).catch(() => {});
  }

  // «Reintentar» on the offline page reloads whatever address failed.
  document.addEventListener("click", (e) => {
    const el = e.target.closest("[data-reload]");
    if (el) {
      e.preventDefault();
      location.reload();
    }
  });

  // A boosted link that cannot reach the server fails quietly: htmx keeps
  // the old page and nothing says why. A real navigation lands on the offline
  // page instead.
  document.addEventListener("htmx:sendError", (e) => {
    const d = e.detail || {};
    const path = d.pathInfo && d.pathInfo.requestPath;
    if (d.boosted && path) {
      location.href = path;
    }
  });

  // Pull to refresh. Installed on iOS there is no browser chrome and so no
  // gesture to reload; a phone app is expected to have one.
  const standalone = matchMedia("(display-mode: standalone)").matches || navigator.standalone === true;
  if (!standalone) {
    return;
  }
  const THRESHOLD = 80;
  let startY = null;
  let pulled = 0;
  let pill = null;

  const show = (ready) => {
    if (!pill) {
      pill = document.createElement("div");
      pill.className = "ptr";
      pill.setAttribute("aria-hidden", "true");
      document.body.appendChild(pill);
    }
    pill.textContent = ready ? "Soltá para actualizar" : "Tirá para actualizar";
    pill.classList.toggle("ptr-ready", ready);
    pill.classList.add("ptr-on");
  };
  const hide = () => {
    if (pill) {
      pill.classList.remove("ptr-on", "ptr-ready");
    }
  };

  addEventListener("touchstart", (e) => {
    // Only from the very top, and not inside a popover or something that
    // scrolls on its own.
    if (scrollY > 0 || e.touches.length !== 1 || e.target.closest("[popover], .poster-row, .table-scroll")) {
      startY = null;
      return;
    }
    startY = e.touches[0].clientY;
    pulled = 0;
  }, { passive: true });

  addEventListener("touchmove", (e) => {
    if (startY === null) {
      return;
    }
    pulled = e.touches[0].clientY - startY;
    if (pulled > 20 && scrollY <= 0) {
      show(pulled >= THRESHOLD);
    } else {
      hide();
    }
  }, { passive: true });

  addEventListener("touchend", () => {
    if (startY !== null && pulled >= THRESHOLD && scrollY <= 0) {
      if (pill) {
        pill.textContent = "Actualizando…";
      }
      location.reload();
      return;
    }
    startY = null;
    hide();
  }, { passive: true });
})();
