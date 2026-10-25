import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";

const src = (path) => fs.readFileSync(new URL(path, import.meta.url), "utf8");

test("Subshells has a dedicated Hub page above Bar Studio", () => {
    const hub = src("../Hub.qml");
    const a = hub.indexOf('{ key: "subshells", name: "Subshells"');
    const b = hub.indexOf('{ key: "bar-studio", name: "Bar Studio"');
    assert.ok(a >= 0 && b > a);
    assert.match(hub, /"subshells": "SubshellsPage"/);
});

test("Both views share the same selection and distinct Store destinations", () => {
    const studio = src("../pages/BarStudioPage.qml");
    const shells = src("../pages/SubshellsPage.qml");
    const gallery = src("../pages/StoreDesignGallery.qml");
    assert.match(shells, /subshellsOnly: true/);
    assert.match(studio, /onlySubshells: page\.subshellsOnly/);
    assert.match(studio, /page\.fedit\("barStyle", designId\)/);
    assert.match(gallery, /gallery\.isSubshellStyle\(card\.id\) === gallery\.onlySubshells/);
    assert.match(gallery, /"ryostore", "open", gallery\.browseCategory/);
    assert.match(gallery, /"ryostore", "install", gallery\.category, id/);
});
