// Update Spotify's color variables directly from Ryoku's wallpaper palette.
// This changes paint values in place: no renderer reload and no playback pause.
(function ryokuWallpaperColors() {
  if (window.__ryokuWallpaperColors) return;
  window.__ryokuWallpaperColors = true;
  const endpoint = "http://127.0.0.1:47616/v1/events";
  function mix(base, tint, amount) {
    return "#" + [1, 3, 5].map(i => Math.round(
      parseInt(base.slice(i, i + 2), 16) * (1 - amount) +
      parseInt(tint.slice(i, i + 2), 16) * amount,
    ).toString(16).padStart(2, "0")).join("");
  }
  function luminance(hex) {
    const channels = [1, 3, 5].map(i => {
      const value = parseInt(hex.slice(i, i + 2), 16) / 255;
      return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
    });
    return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
  }
  function colorsFor(palette) {
    if (!/^#[0-9a-f]{6}$/i.test(palette.primary || "")) return null;
    const tint = palette.primary;
    const main = mix("#131416", tint, 0.025);
    const card = mix("#222327", tint, 0.05);
    let accent = tint;
    for (let amount = 0.1; (luminance(accent) + 0.05) / (luminance(card) + 0.05) < 4.5; amount += 0.1) {
      accent = mix(tint, "#ffffff", Math.min(amount, 1));
    }
    return {
      text: "#f1f1f3", subtext: "#b6b8c0", main,
      "main-elevated": mix("#1c1d21", tint, 0.045),
      "main-transition": main,
      highlight: mix("#292b30", tint, 0.075),
      "highlight-elevated": mix("#33353b", tint, 0.09),
      sidebar: mix("#17181b", tint, 0.035),
      player: mix("#101113", tint, 0.02),
      card, shadow: "#000000",
      "selected-row": "#f1f1f3", button: accent,
      "button-active": mix(accent, "#ffffff", 0.15),
      "button-disabled": "#555760",
      "tab-active": mix("#292b30", tint, 0.075),
      notification: accent, "notification-error": "#f28b82",
      misc: "#6b6e78", "play-button": accent,
      "play-button-active": mix(accent, "#ffffff", 0.15),
      "progress-fg": accent, "progress-bg": "#45474f",
      heart: accent, "pagelink-active": accent, "radio-btn-active": accent,
    };
  }

  // Modern Spotify surfaces use Encore tokens in addition to --spice-*.
  const bridgeCss = `
    :root,
    .encore-dark-theme,
    .encore-dark-theme .encore-base-set {
      --background-base: var(--spice-main) !important;
      --background-highlight: var(--spice-highlight) !important;
      --background-press: var(--spice-highlight-elevated) !important;
      --background-elevated-base: var(--spice-main-elevated) !important;
      --background-elevated-highlight: var(--spice-highlight-elevated) !important;
      --background-elevated-press: var(--spice-highlight-elevated) !important;
      --background-tinted-base: var(--spice-main-elevated) !important;
      --background-tinted-highlight: var(--spice-highlight) !important;
      --background-tinted-press: var(--spice-highlight-elevated) !important;
      --text-base: var(--spice-text) !important;
      --text-subdued: var(--spice-subtext) !important;
      --text-bright-accent: var(--spice-button) !important;
      --essential-base: var(--spice-text) !important;
      --essential-subdued: var(--spice-subtext) !important;
      --essential-bright-accent: var(--spice-button) !important;
      --decorative-base: var(--spice-text) !important;
      --decorative-subdued: var(--spice-subtext) !important;
      --encore-app-semantic-background-color: var(--spice-main) !important;
      --encore-background-highlight: var(--spice-highlight) !important;
      --encore-background-elevated-highlight: var(--spice-highlight-elevated) !important;
      --encore-box-shadow-color: var(--spice-shadow) !important;
      --encore-focus-outline-color: var(--spice-button) !important;
    }
    body,
    .Root__top-container,
    .Root__globalNav { background-color: var(--spice-player) !important; }
    .Root__now-playing-bar { background: var(--spice-player) !important; }
    .main-home-homeHeader,
    .main-home-homeBackground,
    .main-actionBarBackground-background,
    .main-entityHeader-backgroundColor,
    .main-topBar-overlay {
      background: var(--spice-main) !important;
      background-image: none !important;
    }
    .Root__now-playing-bar [data-testid="control-button-playpause"] > span,
    .Root__main-view [data-encore-id="buttonPrimary"] > span {
      background-color: var(--spice-button) !important;
      color: #131416 !important;
    }
    .playback-progressbar .x-progressBar-fillColor {
      background-color: var(--spice-button) !important;
    }
    :root .encore-bright-accent-set {
      --background-base: var(--spice-button) !important;
      --background-highlight: var(--spice-button-active) !important;
      --background-press: var(--spice-button-active) !important;
      --text-base: #131416 !important;
      --essential-base: #131416 !important;
    }
  `;

  let bridge = document.getElementById("ryoku-wallpaper-token-bridge");
  if (!bridge) {
    bridge = document.createElement("style");
    bridge.id = "ryoku-wallpaper-token-bridge";
    bridge.textContent = bridgeCss;
    document.head.appendChild(bridge);
  }

  let last = "";
  const applied = new Map();
  function setColor(style, name, color) {
    if (applied.get(name) === color) return;
    style.setProperty(name, color);
    applied.set(name, color);
  }
  function rgb(hex) {
    const value = hex.replace("#", "");
    return [0, 2, 4].map((i) => parseInt(value.slice(i, i + 2), 16)).join(",");
  }
  function update(palette) {
    try {
      const colors = colorsFor(palette);
      if (!colors) return;
      const signature = JSON.stringify(palette);
      if (signature === last) return;
      last = signature;
      const style = document.documentElement.style;
      for (const [name, color] of Object.entries(colors)) {
        setColor(style, "--spice-" + name, color);
        setColor(style, "--spice-rgb-" + name, rgb(color));
      }
      if (/^#[0-9a-f]{6}$/i.test(palette.primary || "")) {
        setColor(style, "--spice-primary", colors.button);
      }
      window.dispatchEvent(new CustomEvent("ryoku-palette-changed", { detail: palette }));
    } catch (_) {}
  }

  let reconnectTimer = null;
  function connect() {
    const events = new EventSource(endpoint);
    events.onmessage = (event) => {
      try {
        update(JSON.parse(event.data));
      } catch (_) {}
    };
    events.onerror = () => {
      events.close();
      if (reconnectTimer !== null) return;
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        connect();
      }, 1000);
    };
  }
  connect();
})();
