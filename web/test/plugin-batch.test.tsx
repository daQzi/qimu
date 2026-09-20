import { expect, test } from "bun:test";
import { readFileSync, readdirSync } from "node:fs";
import { relative, resolve } from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { validatePluginTextPackage } from "../src/lib/plugins/plugin-v3-contract";
import { PluginBatchControls } from "../src/components/plugins/plugin-batch-controls";
import type { PluginRunView } from "../src/services/api/plugin-operations";

test("P06 package admits bounded foreach and conditions with declared dependencies", () => {
    const root = resolve(import.meta.dir, "../../backend/internal/plugins/contracts/testdata/batch-helper-p06");
    const files = Object.fromEntries(
        readdirSync(root, { recursive: true, withFileTypes: true })
            .filter((f) => f.isFile())
            .map((f) => {
                const path = resolve(f.parentPath, f.name);
                return [relative(root, path), readFileSync(path, "utf8")];
            }),
    );
    expect(() => validatePluginTextPackage(files)).not.toThrow();
    const original = files["pipelines/process.json"];
    for (const mutation of [(p: any) => (p.steps[2].foreach.maxConcurrency = 9), (p: any) => (p.steps[2].dependsOn = []), (p: any) => (p.steps[2].foreach.from = "item#/items"), (p: any) => (p.steps[2].when.exists = "steps/missing#/value")]) {
        const pipeline = JSON.parse(original);
        mutation(pipeline);
        files["pipelines/process.json"] = JSON.stringify(pipeline);
        expect(() => validatePluginTextPackage(files)).toThrow();
    }
});
test("batch card requires fetching an exact quote before presenting approval", () => {
    const html = renderToStaticMarkup(<PluginBatchControls run={{ id: "run", status: "waiting_approval" } as PluginRunView} onUpdated={() => {}} onDerived={() => {}} />);
    expect(html).toContain("核对当前批次费用");
    expect(html).not.toContain("批准此批次并预留服务费");
});
test("derived run displays escaped lineage and exposes new-run control", () => {
    const html = renderToStaticMarkup(<PluginBatchControls run={{ id: "run", status: "succeeded", derivedFromRunId: "<script>bad</script>", attempt: 2 } as PluginRunView} onUpdated={() => {}} onDerived={() => {}} />);
    expect(html).toContain("&lt;script&gt;");
    expect(html).toContain("基于此次运行重新执行");
    expect(html).not.toContain("核对当前批次费用");
});
