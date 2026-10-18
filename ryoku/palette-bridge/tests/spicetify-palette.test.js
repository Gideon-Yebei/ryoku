"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const properties = new Map();
const elements = new Map();
const streams = [];
const events = [];
const timers = [];
let writes = 0;
class EventSource {
  constructor(url) {
    this.url = url;
    this.closed = false;
    streams.push(this);
  }
  close() {
    this.closed = true;
  }
}
class CustomEvent {
  constructor(type, options) {
    this.type = type;
    this.detail = options.detail;
  }
}
const document = {
  documentElement: { style: { setProperty: (name, value) => { writes++; properties.set(name, value); } } },
  getElementById: id => elements.get(id) || null,
  createElement: () => ({}),
  head: { appendChild: element => elements.set(element.id, element) },
};
const window = { dispatchEvent: event => events.push(event) };
const setTimeout = (callback, delay) => {
  timers.push({ callback, delay });
  return timers.length;
};
const source = fs.readFileSync(
  path.join(__dirname, "..", "spicetify", "ryoku-wallpaper-colors.js"),
  "utf8",
);
vm.runInNewContext(source, {
  window, document, EventSource, CustomEvent, JSON, Object, setTimeout,
});

assert.equal(streams.length, 1);
assert.equal(streams[0].url, "http://127.0.0.1:47616/v1/events");
assert.ok(elements.get("ryoku-wallpaper-token-bridge").textContent.includes("--encore-focus-outline-color"));

const palette = {
  primary: "#112233", onSurface: "#eeeeee", onSurfaceVariant: "#cccccc",
  background: "#010203", surface: "#111213", surfaceContainer: "#212223",
  surfaceContainerHigh: "#313233", surfaceContainerHighest: "#414243",
  surfaceContainerLow: "#090a0b", shadow: "#000000", primaryContainer: "#445566",
  outlineVariant: "#777777", tertiary: "#abcdef", error: "#ff0000",
  surfaceVariant: "#515253", secondary: "#778899",
};
streams[0].onmessage({ data: JSON.stringify(palette) });

function luminance(hex) {
  const c = [1, 3, 5].map(i => {
    const v = parseInt(hex.slice(i, i + 2), 16) / 255;
    return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  });
  return c[0] * 0.2126 + c[1] * 0.7152 + c[2] * 0.0722;
}
function contrast(a, b) {
  const l = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (l[0] + 0.05) / (l[1] + 0.05);
}
assert.equal(properties.get("--spice-text"), "#f1f1f3", "body text stays neutral");
assert.equal(properties.get("--spice-subtext"), "#b6b8c0");
assert.notEqual(properties.get("--spice-main"), palette.background, "wallpaper backgrounds are not poured over the UI");
assert.ok(contrast(properties.get("--spice-button"), properties.get("--spice-main")) >= 4.5);
assert.ok(contrast(properties.get("--spice-text"), properties.get("--spice-card")) >= 7);
assert.equal(properties.get("--spice-primary"), properties.get("--spice-button"));
assert.equal(properties.get("--spice-progress-fg"), properties.get("--spice-button"));
assert.equal(events.at(-1).type, "ryoku-palette-changed");
assert.deepEqual(events.at(-1).detail, palette, "the complete source palette remains available to integrations");
const initialWrites = writes;
streams[0].onmessage({ data: JSON.stringify(palette) });
assert.equal(writes, initialWrites, "identical palettes do not cause repaint churn");
const eventCount = events.length;
streams[0].onmessage({ data: JSON.stringify({ ...palette, secondary: "#abcdef" }) });
assert.equal(writes, initialWrites, "unused role changes do not repaint surfaces");
assert.equal(events.length, eventCount + 1, "other integrations still receive the full palette event");
streams[0].onmessage({ data: JSON.stringify({ ...palette, primary: "#ffb4a8" }) });
assert.equal(properties.get("--spice-button"), "#ffb4a8", "readable wallpaper accents keep their hue");
assert.notEqual(properties.get("--spice-main"), palette.background);
assert.equal(properties.get("--spice-text"), "#f1f1f3");
assert.equal(properties.get("--spice-rgb-button"), "255,180,168");
for (const primary of ["#000000", "#0000ff", "#ffffff", "#002200"]) {
  streams[0].onmessage({ data: JSON.stringify({ ...palette, primary }) });
  assert.ok(contrast(properties.get("--spice-button"), properties.get("--spice-card")) >= 4.5, primary);
}
const validWrites = writes;
streams[0].onmessage({ data: JSON.stringify({ primary: "invalid" }) });
assert.equal(writes, validWrites, "invalid palette cannot replace the current colors");

streams[0].onerror();
assert.equal(streams[0].closed, true);
assert.equal(timers.length, 1);
assert.equal(timers[0].delay, 1000);
timers[0].callback();
assert.equal(streams.length, 2);
assert.equal(streams[1].url, "http://127.0.0.1:47616/v1/events");
console.log("PASS: Spotify extension maps Ryoku roles and reconnects its local SSE stream");
