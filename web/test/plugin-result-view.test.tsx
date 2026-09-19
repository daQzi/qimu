import { expect, test } from "bun:test";
import { readFileSync, readdirSync } from "node:fs";
import { relative, resolve } from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { PluginResultValues } from "../src/components/plugins/plugin-result-values";
import { pluginResultField, pluginResultFieldText } from "../src/lib/plugins/plugin-result-view";
import { validatePluginTextPackage } from "../src/lib/plugins/plugin-v3-contract";
import { getNodeDefinition } from "../src/lib/canvas/node-registry";
import "../src/lib/canvas/node-registry/definitions";

test("result values preserve missing/zero/null and resolve only own JSON fields", () => {
    const value = { zero: 0, no: false, empty: null, "a/b": { "~key": "value" } };
    expect(pluginResultField(value, "/a~1b/~0key")).toBe("value");
    expect(pluginResultField(value, "/toString")).toBeUndefined();
    expect(pluginResultField(value, "/a~2b")).toBeUndefined();
    expect(pluginResultFieldText(pluginResultField(value, "/zero"))).toBe("0");
    expect(pluginResultFieldText(pluginResultField(value, "/no"))).toBe("false");
    expect(pluginResultFieldText(pluginResultField(value, "/empty"))).toBe("空值");
    expect(pluginResultFieldText(pluginResultField(value, "/missing"))).toBe("未记录");
});

test("declarative result renderer escapes plugin labels and values", () => {
    const html = renderToStaticMarkup(<PluginResultValues value={{ name: "<script>alert(1)</script>" }} view={{ id: "view", component: "key-value/v1", fields: [{ path: "/name", label: "<img onerror=x>" }, { path: "/durationMs", label: "时长" }] }} />);
    expect(html).not.toContain("<script>");
    expect(html).not.toContain("<img");
    expect(html).toContain("未记录");
    expect(html).toContain("&lt;script&gt;");
});

test("P03 package passes shared contract and cannot lower canvas permission or context", () => {
    const root = resolve(import.meta.dir, "../../backend/internal/plugins/contracts/testdata/resource-helper-p03");
    const files = Object.fromEntries(readdirSync(root, { recursive: true, withFileTypes: true }).filter((entry) => entry.isFile()).map((entry) => { const path = resolve(entry.parentPath, entry.name); return [relative(root, path), readFileSync(path, "utf8")]; }));
    expect(() => validatePluginTextPackage(files)).not.toThrow();
    const path = "operations/place-result.json";
    const original = files[path];
    const op = JSON.parse(original);
    op.requiredPermissions = ["canvas.read"];
    expect(() => validatePluginTextPackage({ ...files, [path]: JSON.stringify(op) })).toThrow();
    const noCanvas = JSON.parse(original);
    noCanvas.context.requiresCanvas = false;
    expect(() => validatePluginTextPackage({ ...files, [path]: JSON.stringify(noCanvas) })).toThrow();
});

test("plugin-result registry remains non-generating and unavailable for empty manual creation", () => {
    const result = getNodeDefinition("plugin-result");
    expect(result?.showInCreateMenu).toBe(false);
    expect(result?.showInputConnection).toBe(false);
    expect(result?.showOutputConnection).toBe(false);
    expect(result?.generationMode).toBeUndefined();
    expect(getNodeDefinition("video")?.generationMode).toBeDefined();
    expect(getNodeDefinition("text")?.showInCreateMenu).toBe(true);
});
