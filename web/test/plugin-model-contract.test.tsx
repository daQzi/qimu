import { expect, test } from "bun:test";
import { readFileSync, readdirSync } from "node:fs";
import { relative, resolve } from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { validatePluginTextPackage } from "../src/lib/plugins/plugin-v3-contract";
import { PluginModelPreview } from "../src/components/plugins/plugin-model-preview";

function fixture() {
    const root = resolve(import.meta.dir, "../../examples/plugins/video-localization");
    return Object.fromEntries(
        readdirSync(root, { recursive: true, withFileTypes: true })
            .filter((f) => f.isFile())
            .map((f) => {
                const path = resolve(f.parentPath, f.name);
                return [relative(root, path), readFileSync(path, "utf8")];
            }),
    );
}
test("P11 real localization package and model admission boundaries", () => {
    expect(() => validatePluginTextPackage(fixture())).not.toThrow();
    for (const scenario of ["old-host", "missing-media", "false-effect", "foreign-profile", "external-url", "audio-resource", "unknown-validator"]) {
        const files = fixture();
        const manifest = JSON.parse(files["manifest.json"]);
        const operation = JSON.parse(files["operations/analyze.json"]);
        const pipeline = JSON.parse(files["pipelines/localize.json"]);
        if (scenario === "old-host") manifest.requires.hostApi = "^3.2.0";
        if (scenario === "missing-media") operation.requiredPermissions = ["generation.run"];
        if (scenario === "false-effect") operation.effects = ["read"];
        if (scenario === "foreign-profile") operation.execution.outputProfile = "arbitrary-code";
        if (scenario === "external-url") operation.execution.baseUrl = "https://example.invalid";
        if (scenario === "audio-resource") operation.execution.resources.resourceId = "audio";
        if (scenario === "unknown-validator") pipeline.steps[1].inputValidator = "eval";
        files["manifest.json"] = JSON.stringify(manifest);
        files["operations/analyze.json"] = JSON.stringify(operation);
        files["pipelines/localize.json"] = JSON.stringify(pipeline);
        expect(() => validatePluginTextPackage(files)).toThrow();
    }
});
test("P11 model quote renders resource scope, fee ceiling and no invented free quote", () => {
    const html = renderToStaticMarkup(<PluginModelPreview value={{ model: "<script>model</script>", amountMicrocredits: 2_000_000, estimated: true, resources: { resourceId: { resourceId: "video-one" } } }} />);
    expect(html).toContain("2 积分");
    expect(html).toContain("受此上限约束");
    expect(html).toContain("video-one");
    expect(html).toContain("不会自动重试");
    expect(html).not.toContain("<script>model");
    expect(renderToStaticMarkup(<PluginModelPreview value={undefined} />)).toContain("报价不可用");
});
