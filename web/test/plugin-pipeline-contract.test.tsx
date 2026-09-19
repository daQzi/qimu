import { expect, test } from "bun:test";
import { readFileSync, readdirSync } from "node:fs";
import { relative, resolve } from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { validatePluginTextPackage } from "../src/lib/plugins/plugin-v3-contract";
import { PluginInputForm } from "../src/components/plugins/plugin-input-form";
import type { PluginRunView } from "../src/services/api/plugin-operations";

function fixture() {
    const root = resolve(import.meta.dir, "../../backend/internal/plugins/contracts/testdata/pipeline-helper-p05");
    return Object.fromEntries(
        readdirSync(root, { recursive: true, withFileTypes: true })
            .filter((f) => f.isFile())
            .map((f) => {
                const path = resolve(f.parentPath, f.name);
                return [relative(root, path), readFileSync(path, "utf8")];
            }),
    );
}
test("P05 sequential package accepts chain and rejects unsupported or weaker contracts", () => {
    expect(() => validatePluginTextPackage(fixture())).not.toThrow();
    for (const mutate of [
        (p: any) => {
            p.steps[2].dependsOn = [];
        },
        (p: any) => {
            p.steps[0].dependsOn = ["remote"];
        },
        (p: any) => {
            p.steps[2].operation = "pipeline-helper.process";
        },
        (p: any) => {
            p.steps[2].operation = "foreign.echo";
        },
        (p: any) => {
            p.steps[2].inputs.prompt.from = "steps/inspect#/name";
        },
        (p: any) => {
            p.steps[2].when = { exists: "steps/choose#/prompt" };
        },
        (p: any) => {
            p.steps[1].formSchemaRef = "schemas/missing.json";
        },
    ]) {
        const files = fixture();
        const p = JSON.parse(files["pipelines/process.json"]);
        mutate(p);
        files["pipelines/process.json"] = JSON.stringify(p);
        expect(() => validatePluginTextPackage(files)).toThrow();
    }
    const files = fixture();
    const op = JSON.parse(files["operations/process.json"]);
    op.effects = ["read"];
    files["operations/process.json"] = JSON.stringify(op);
    expect(() => validatePluginTextPackage(files)).toThrow();
});
test("P05 input presentation uses server draft, explicit submit, no guessed content", () => {
    const html = renderToStaticMarkup(
        <PluginInputForm
            run={{ id: "run", status: "waiting_input" } as PluginRunView}
            input={{ id: "input", runId: "run", stepKey: "choose", revision: 2, status: "pending", schema: { type: "object", properties: { prompt: { type: "string" } }, required: ["prompt"] }, draft: { prompt: "<script>alert(1)</script>" } }}
            onUpdated={() => {}}
        />,
    );
    expect(html).toContain("保存草稿");
    expect(html).toContain("提交并继续");
    expect(html).toContain("仍需单独确认");
    expect(html).toContain("&lt;script&gt;");
    expect(html).not.toContain("<script>alert");
});
