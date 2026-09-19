import { describe, expect, test } from "bun:test";
import { readFileSync, readdirSync } from "node:fs";
import { resolve, relative } from "node:path";
import { PluginContractError, validatePluginContract, validatePluginTextPackage, validateDependencyGraph, pluginPackageDigest, pluginOperationDigest, pluginV3Profile } from "../src/lib/plugins/plugin-v3-contract";
import type { PluginManifestV3, PluginOperation } from "../src/lib/plugins/plugin-v3-types";

const root = resolve(import.meta.dir, "../../backend/internal/plugins/contracts/testdata");
function fixture() {
    const base = resolve(root, "resource-helper");
    return Object.fromEntries(
        readdirSync(base, { recursive: true, withFileTypes: true })
            .filter((f) => f.isFile())
            .map((f) => {
                const name = resolve(f.parentPath, f.name);
                return [relative(base, name), readFileSync(name, "utf8")];
            }),
    );
}
type Change = { file: string; pointer?: string; value?: unknown; remove?: boolean; removeFile?: boolean; text?: string };
type Case = { name: string; kind: string; contract: string; value: unknown; raw?: string; reason: string; changes?: Change[]; graph: Record<string, string[]> };
const cases: Case[] = JSON.parse(readFileSync(resolve(root, "cases.json"), "utf8"));
function mutate(files: Record<string, string>, change: Change) {
    if (change.removeFile) {
        delete files[change.file];
        return;
    }
    if (change.text !== undefined) {
        files[change.file] = change.text;
        return;
    }
    const doc = JSON.parse(files[change.file]);
    const parts = change.pointer!.slice(1).split("/");
    let value = doc;
    for (const part of parts.slice(0, -1)) value = value[part];
    if (change.remove) delete value[parts.at(-1)!];
    else value[parts.at(-1)!] = change.value;
    files[change.file] = JSON.stringify(doc);
}
describe("P00 shared Go/TypeScript contract corpus", () => {
    for (const c of cases)
        test(c.name, () => {
            let issue: unknown;
            try {
                if (c.kind === "package") {
                    const files = fixture();
                    for (const change of c.changes ?? []) mutate(files, change);
                    validatePluginTextPackage(files, ["official-tools"]);
                } else if (c.kind === "contract") validatePluginContract(c.contract, c.raw ?? JSON.stringify(c.value));
                else if (c.kind === "graph") validateDependencyGraph(c.graph);
                else throw new Error(`unknown fixture ${c.kind}`);
            } catch (error) {
                issue = error;
            }
            if (!c.reason) expect(issue).toBeUndefined();
            else {
                expect(issue).toBeInstanceOf(PluginContractError);
                expect((issue as PluginContractError).reason).toBe(c.reason);
            }
        });
    test("same exact byte identity as Go", async () => {
        const golden = JSON.parse(readFileSync(resolve(root, "digests.json"), "utf8"));
        const files = fixture();
        const digest = await pluginPackageDigest(files);
        expect(digest).toBe(golden.packageDigest);
        expect(await pluginOperationDigest(digest, "resource-helper.inspect-video")).toBe(golden.operationDigest);
        const reversed = Object.fromEntries(Object.entries(files).reverse());
        expect(await pluginPackageDigest(reversed)).toBe(digest);
        files["skills/check-source/SKILL.md"] += "\n";
        expect(await pluginPackageDigest(files)).not.toBe(digest);
    });
    test("does not enable runtime and preserves typed fixture values", () => {
        expect(pluginV3Profile.runtimeEnabled).toBe(false);
        const files = fixture();
        const manifest = validatePluginContract("manifest", files["manifest.json"]) as PluginManifestV3;
        const operation = validatePluginContract("operation", files["operations/inspect-video.json"]) as PluginOperation;
        expect(manifest.contributes.operations?.[0].id).toBe(operation.id);
        expect(operation.context.requiresCanvas).toBe(false);
    });
    test("bounded files", () => {
        const files = fixture();
        files["skills/check-source/large.md"] = "x".repeat(pluginV3Profile.limits.entryBytes + 1);
        expect(() => validatePluginTextPackage(files)).toThrow(PluginContractError);
    });
});
