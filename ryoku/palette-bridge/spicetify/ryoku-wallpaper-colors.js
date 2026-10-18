// Update Spotify's color variables directly from Ryoku's wallpaper palette.
// This changes paint values in place: no renderer reload and no playback pause.
(function ryokuWallpaperColors() {
  if (window.__ryokuWallpaperColors) return;
  window.__ryokuWallpaperColors = true;
  const endpoint = "http://127.0.0.1:47616/v1/events";
  const paletteRoles = {
    text: "onSurface",
    subtext: "onSurfaceVariant",
    main: "background",
    "main-elevated": "surfaceContainer",
    "main-transition": "surface",
    highlight: "surfaceContainerHigh",
    "highlight-elevated": "surfaceContainerHighest",
    sidebar: "surfaceContainerLow",
    player: "background",
    card: "surfaceContainer",
    shadow: "shadow",
    "selected-row": "onSurface",
    button: "primary",
    "button-active": "primaryContainer",
    "button-disabled": "outlineVariant",
    "tab-active": "surfaceContainerHigh",
    notification: "tertiary",
    "notification-error": "error",
    misc: "surfaceVariant",
    "play-button": "primary",
    "play-button-active": "primaryContainer",
    "progress-fg": "secondary",
    "progress-bg": "surfaceVariant",
    heart: "error",
    "pagelink-active": "tertiary",
    "radio-btn-active": "primary",
  };

  const paletteCss = `
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
    :root .encore-bright-accent-set {
      --background-base: var(--spice-button) !important;
      --background-highlight: var(--spice-button-active) !important;
      --background-press: var(--spice-button-active) !important;
      --text-base: var(--spice-main) !important;
      --essential-base: var(--spice-main) !important;
    }
  `;
  let preferences = { neutralStyle: false, accentStrength: 100, surfaceTint: 5, neutralText: true, removeGradients: true };
  let latestPalette = null;
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
    const tint = mix("#c7c9d1", palette.primary, preferences.accentStrength / 100);
    const amount = preferences.surfaceTint / 100;
    const main = mix("#131416", tint, amount * 0.5);
    const card = mix("#222327", tint, amount);
    let accent = tint;
    for (let amount = 0.1; (luminance(accent) + 0.05) / (luminance(card) + 0.05) < 4.5; amount += 0.1) {
      accent = mix(tint, "#ffffff", Math.min(amount, 1));
    }
    return {
      text: preferences.neutralText ? "#f1f1f3" : (palette.onSurface || "#f1f1f3"),
      subtext: preferences.neutralText ? "#b6b8c0" : (palette.onSurfaceVariant || "#b6b8c0"), main,
      "main-elevated": mix("#1c1d21", tint, amount * 0.9),
      "main-transition": main,
      highlight: mix("#292b30", tint, amount * 1.5),
      "highlight-elevated": mix("#33353b", tint, amount * 1.8),
      sidebar: mix("#17181b", tint, amount * 0.7),
      player: mix("#101113", tint, amount * 0.4),
      card, shadow: "#000000",
      "selected-row": "#f1f1f3", button: accent,
      "button-active": mix(accent, "#ffffff", 0.15),
      "button-disabled": "#555760",
      "tab-active": mix("#292b30", tint, amount * 1.5),
      notification: accent, "notification-error": "#f28b82",
      misc: "#6b6e78", "play-button": accent,
      "play-button-active": mix(accent, "#ffffff", 0.15),
      "progress-fg": accent, "progress-bg": "#45474f",
      heart: accent, "pagelink-active": accent, "radio-btn-active": accent,
    };
  }

  // Modern Spotify surfaces use Encore tokens in addition to --spice-*.
  const neutralCss = `
    body,
    .Root__top-container,
    .Root__globalNav { background-color: var(--spice-player) !important; }
    .Root__now-playing-bar { background: var(--spice-player) !important; }
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
  const washCss = `
    .main-home-homeHeader,
    .main-home-homeBackground,
    .main-actionBarBackground-background,
    .main-entityHeader-backgroundColor,
    .main-topBar-overlay {
      background: var(--spice-main) !important;
      background-image: none !important;
    }
  `;

  let bridge = document.getElementById("ryoku-wallpaper-token-bridge");
  if (!bridge) {
    bridge = document.createElement("style");
    bridge.id = "ryoku-wallpaper-token-bridge";
    bridge.textContent = paletteCss;
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
    if (!palette || typeof palette.primary !== "string" || !/^#[0-9a-f]{6}$/i.test(palette.primary)) return;
    latestPalette = palette;
    render();
  }
  function render() {
    try {
      const palette = latestPalette;
      if (!palette || !/^#[0-9a-f]{6}$/i.test(palette.primary || "")) return;
      const colors = preferences.neutralStyle ? colorsFor(palette) : Object.fromEntries(
        Object.entries(paletteRoles).map(([name, role]) => [name, palette[role]]),
      );
      if (!colors) return;
      const signature = JSON.stringify({ palette, preferences });
      if (signature === last) return;
      last = signature;
      const css = paletteCss + (preferences.neutralStyle ? neutralCss + (preferences.removeGradients ? washCss : "") : "");
      if (bridge.textContent !== css) bridge.textContent = css;
      const style = document.documentElement.style;
      for (const [name, color] of Object.entries(colors)) {
        if (!/^#[0-9a-f]{6}$/i.test(color || "")) continue;
        setColor(style, "--spice-" + name, color);
        setColor(style, "--spice-rgb-" + name, rgb(color));
      }
      if (/^#[0-9a-f]{6}$/i.test(palette.primary || "")) {
        setColor(style, "--spice-primary", colors.button);
      }
      window.dispatchEvent(new CustomEvent("ryoku-palette-changed", { detail: palette }));
    } catch (_) {}
  }

  function updatePreferences(doc) {
    const next = doc && doc.spotify;
    if (!next || typeof next.neutralStyle !== "boolean" || typeof next.neutralText !== "boolean" ||
        typeof next.removeGradients !== "boolean" || !Number.isFinite(next.accentStrength) ||
        next.accentStrength < 0 || next.accentStrength > 100 || !Number.isFinite(next.surfaceTint) ||
        next.surfaceTint < 0 || next.surfaceTint > 20) return;
    preferences = {
      neutralStyle: next.neutralStyle, neutralText: next.neutralText,
      removeGradients: next.removeGradients, accentStrength: next.accentStrength, surfaceTint: next.surfaceTint,
    };
    render();
  }
  function subscribe(url, apply) {
    let reconnectTimer = null;
    function connect() {
      const events = new EventSource(url);
      events.onmessage = event => {
        try { apply(JSON.parse(event.data)); } catch (_) {}
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
  }
  subscribe("http://127.0.0.1:47616/v1/preferences/events", updatePreferences);
  subscribe(endpoint, update);
})();
