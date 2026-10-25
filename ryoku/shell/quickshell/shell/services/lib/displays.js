// The part of the outputs list that decides which monitors answer DDC/CI: which
// connectors exist and what is plugged into each. A provider may re-emit outputs
// on every workspace switch or focus move, and mode, scale and position do not
// change which buses answer, so none of them count (#218).
function connectorSignature(outputs) {
    var keys = [];
    var count = outputs ? outputs.length : 0;
    for (var i = 0; i < count; i++) {
        var o = outputs[i];
        if (o)
            keys.push([o.name || "", o.make || "", o.model || ""].join("\u001f"));
    }
    return keys.sort().join("\u001e");
}

if (typeof module !== "undefined" && module.exports)
    module.exports = { connectorSignature };
