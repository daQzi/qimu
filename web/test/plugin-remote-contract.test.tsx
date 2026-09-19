import { expect, test } from "bun:test";
import { readFileSync, readdirSync } from "node:fs";
import { relative, resolve } from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { validatePluginTextPackage } from "../src/lib/plugins/plugin-v3-contract";
import { PluginRemoteStatus } from "../src/components/plugins/plugin-remote-status";
import type { PluginRunView } from "../src/services/api/plugin-operations";

function fixture() {
    const root = resolve(import.meta.dir, "../../backend/internal/plugins/contracts/testdata/remote-helper-p04");
    return Object.fromEntries(
        readdirSync(root, { recursive: true, withFileTypes: true })
            .filter((f) => f.isFile())
            .map((f) => {
                const path = resolve(f.parentPath, f.name);
                return [relative(root, path), readFileSync(path, "utf8")];
            }),
    );
}
test("P04 package and bounded connector semantics", () => {
    const files = fixture();
    expect(() => validatePluginTextPackage(files)).not.toThrow();
    for (const mutate of [
        (c: any) => {
            c.actions.preview.idempotency.retentionSeconds = undefined;
        },
        (c: any) => {
            c.actions.preview.poll.path = "/jobs/{jobId}?auth=key";
        },
        (c: any) => {
            c.auth = { type: "header", header: "Host" };
        },
        (c: any) => {
            c.actions.preview.submit.body.prompt = { from: "resource.foreign" };
        },
        (c: any) => {
            c.actions.preview.outputs.preview = "/url";
        },
    ]) {
        const c = JSON.parse(files["connectors/api.json"]);
        mutate(c);
        expect(() => validatePluginTextPackage({ ...files, "connectors/api.json": JSON.stringify(c) })).toThrow();
    }
});
test("remote confirmation separates platform and vendor charges; unknown cannot silently replay", () => {
    const run = {
        id: "r",
        releaseId: "v",
        releaseVersion: "1.0.0",
        operation: "remote.echo",
        revision: 1,
        status: "waiting_approval",
        preview: { targetHost: "vendor.example", platformFeeMicrocredits: 250_000, resourceCount: 0, idempotency: "unsupported" },
    } as PluginRunView;
    const html = renderToStaticMarkup(<PluginRemoteStatus run={run} onUpdated={() => {}} />);
    expect(html).toContain("0.25");
    expect(html).toContain("供应商");
    expect(html).toContain("vendor.example");
    const paused = renderToStaticMarkup(<PluginRemoteStatus run={{ ...run, status: "paused", remote: { taskId: "t", submissionState: "unknown", cancelRequested: false, importAttempts: 0 } }} onUpdated={() => {}} />);
    expect(paused).toContain("不要直接重复生成");
    expect(paused).not.toContain("按原提交身份恢复");
});
