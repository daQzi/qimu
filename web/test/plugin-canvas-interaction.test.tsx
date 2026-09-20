import { expect, test } from "bun:test";
import { readFileSync, readdirSync } from "node:fs";
import { relative, resolve } from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { validatePluginTextPackage } from "../src/lib/plugins/plugin-v3-contract";
import { PluginResultValues } from "../src/components/plugins/plugin-result-values";
import { PluginInputForm } from "../src/components/plugins/plugin-input-form";
import { PluginMediaValue } from "../src/components/plugins/plugin-media-value";
import { PluginInputNodeContent } from "../src/components/canvas/plugin-input-node-content";
import { pluginActionNode } from "../src/lib/plugins/plugin-editor-actions";
import { mappingSchema, mappingRows, mappingKey } from "../src/lib/plugins/plugin-mapping";
import { getNodeDefinition } from "../src/lib/canvas/node-registry";
import "../src/lib/canvas/node-registry/definitions";
import type { PluginRunView } from "../src/services/api/plugin-operations";
import { CanvasNodeType, type CanvasNodeData } from "../src/types/canvas";

function fixture() {
    const root = resolve(import.meta.dir, "../../backend/internal/plugins/contracts/testdata/canvas-helper-p07");
    return Object.fromEntries(
        readdirSync(root, { recursive: true, withFileTypes: true })
            .filter((f) => f.isFile())
            .map((f) => {
                const path = resolve(f.parentPath, f.name);
                return [relative(root, path), readFileSync(path, "utf8")];
            }),
    );
}
test("P07 actual package and forbidden components/actions/oversized blueprints", () => {
    expect(() => validatePluginTextPackage(fixture())).not.toThrow();
    for (const mutate of [
        (bp: any) => bp.nodes[0].actions.push("model.generate"),
        (bp: any) => bp.nodes[0].actions.push("input.submit"),
        (bp: any) => (bp.nodes[0].nodeType = "video"),
        (bp: any) => (bp.nodes[0].binding = "input"),
        (bp: any) => (bp.nodes[0].script = "alert(1)"),
        (bp: any) => (bp.connections[0].to = "missing"),
        (bp: any) => bp.connections.push(bp.connections[0]),
        (bp: any) => (bp.nodes = Array.from({ length: 20 }, (_, i) => ({ ...bp.nodes[0], key: `n${i}` }))),
    ]) {
        const files = fixture(),
            bp = JSON.parse(files["blueprints/results.json"]);
        mutate(bp);
        files["blueprints/results.json"] = JSON.stringify(bp);
        expect(() => validatePluginTextPackage(files)).toThrow();
    }
    const files = fixture(),
        view = JSON.parse(files["views/mapping.json"]);
    view.component = "table/v1";
    files["views/mapping.json"] = JSON.stringify(view);
    expect(() => validatePluginTextPackage(files)).toThrow();
});
test("P07 named components escape content, paginate without dropping data, and reject URLs", () => {
    const files = fixture();
    const value = { mappings: Array.from({ length: 21 }, (_, i) => ({ source: `<script>${i}</script>`, target: false })) };
    for (const name of ["table", "cards"]) {
        const html = renderToStaticMarkup(<PluginResultValues value={value} view={JSON.parse(files[`views/${name}.json`])} />);
        expect(html).not.toContain("<script>");
        expect(html).toContain("&lt;script&gt;");
        expect(html).toContain("false");
        expect(html).toContain("21");
        expect(html).toContain("下一页");
    }
    const unsafe = renderToStaticMarkup(<PluginMediaValue value={{ resourceId: "https://untrusted.test/x" }} label="对比" />);
    expect(unsafe).not.toContain("src=");
    expect(unsafe).toContain("需要已导入的媒体资源");
    const html = renderToStaticMarkup(<PluginResultValues value={{ mappings: {} }} view={JSON.parse(files["views/table.json"])} />);
    expect(html).toContain("组件需要数组数据");
});
test("P07 mapping editor preserves draft values and falls back for unsupported shape", () => {
    const files = fixture(),
        schema = JSON.parse(files["schemas/mapping.json"]),
        view = JSON.parse(files["views/mapping.json"]);
    const draft = { mappings: [{ source: "original", target: "replacement" }] };
    expect(mappingSchema(schema, view)?.maxItems).toBe(100);
    expect(mappingRows(draft, view)).toEqual(draft.mappings);
    expect(mappingRows({ mappings: [null] }, view)).toBeUndefined();
    expect(() => mappingKey("/__proto__")).toThrow();
    expect(() => mappingKey("/a~2b")).toThrow();
    const html = renderToStaticMarkup(<PluginInputForm run={{ id: "run" } as PluginRunView} input={{ id: "input", runId: "run", stepKey: "mapping", revision: 2, status: "pending", schema, view, draft }} onUpdated={() => {}} />);
    expect(html).toContain("original");
    expect(html).toContain("replacement");
    expect(html).toContain("添加行");
    expect(html).toContain("提交并继续");
    const malformed = renderToStaticMarkup(<PluginInputForm run={{ id: "run" } as PluginRunView} input={{ id: "input", runId: "run", stepKey: "mapping", revision: 2, status: "pending", schema, view, draft: { mappings: "bad" } }} onUpdated={() => {}} />);
    expect(malformed).toContain("切换 JSON 编辑");
});
test("P07 node binding and focus select an exact input of a run, never a result or different request", () => {
    const binding = { runId: "run", inputRequestId: "input", stepKey: "mapping", digest: "digest", releaseId: "release", viewId: "mapping", projectionId: "projection", bindingKey: "mapping", canvasId: "canvas" };
    const nodes = [
        { id: "one", type: CanvasNodeType.PluginInput, metadata: { pluginInput: binding } },
        { id: "two", type: CanvasNodeType.PluginInput, metadata: { pluginInput: { ...binding, inputRequestId: "other" } } },
    ] as CanvasNodeData[];
    expect(pluginActionNode(nodes, { canvasId: "canvas", runId: "run", inputRequestId: "input", command: "editor.focus" })?.id).toBe("one");
    expect(pluginActionNode(nodes, { canvasId: "canvas", runId: "foreign", inputRequestId: "input", command: "editor.focus" })).toBeUndefined();
    expect(pluginActionNode(nodes, { canvasId: "canvas", runId: "run", nodeId: "two", inputRequestId: "input", command: "editor.focus" })).toBeUndefined();
    const definition = getNodeDefinition(CanvasNodeType.PluginInput)!;
    expect(definition.showInCreateMenu).toBe(false);
    expect(definition.showInputConnection).toBe(false);
    expect(definition.showOutputConnection).toBe(false);
    const html = renderToStaticMarkup(<PluginInputNodeContent node={nodes[0]} />);
    expect(html).toContain("data-canvas-no-zoom");
    expect(html).toContain("data-canvas-wheel-scroll");
});
