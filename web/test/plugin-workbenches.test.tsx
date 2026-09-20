import { afterAll, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { tmpdir } from "node:os";
import { renderToStaticMarkup } from "react-dom/server";
import { validatePluginTextPackage } from "../src/lib/plugins/plugin-v3-contract";
import { WorkbenchFields } from "../src/components/plugins/workbench-fields";

function sample(name: string): Record<string, string> {
    const root = resolve(import.meta.dir, "../../examples/plugins", name);
    return Object.fromEntries(
        readdirSync(root, { recursive: true, withFileTypes: true })
            .filter((f) => f.isFile())
            .map((f) => {
                const path = resolve(f.parentPath, f.name);
                return [relative(root, path), readFileSync(path, "utf8")];
            }),
    );
}
for (const name of ["resource-workbench", "brand-workbench", "brief-workbench"])
    test(`shared package ${name} is accepted`, () => {
        expect(() => validatePluginTextPackage(sample(name))).not.toThrow();
    });
test("references, host version, defaults and permission-shaped fields are strongly checked", () => {
    const cases: Array<[string, (value: any) => void]> = [
        [
            "manifest.json",
            (value) => {
                value.requires.hostApi = "^3.0.0";
            },
        ],
        [
            "workbenches/compose.json",
            (value) => {
                value.recipes = ["missing"];
            },
        ],
        [
            "workbenches/compose.json",
            (value) => {
                value.skill = "missing";
            },
        ],
        [
            "workbenches/compose.json",
            (value) => {
                value.defaults = { tone: false };
            },
        ],
        [
            "recipes/social.json",
            (value) => {
                value.defaults = { maxCredits: 999 };
            },
        ],
        [
            "workbenches/compose.json",
            (value) => {
                value.operation = "other.launch";
            },
        ],
    ];
    for (const [path, change] of cases) {
        const files = sample("brand-workbench");
        const value = JSON.parse(files[path]);
        change(value);
        files[path] = JSON.stringify(value);
        expect(() => validatePluginTextPackage(files)).toThrow();
    }
});
test("one Composer renders unrelated schemas and preserves explicit user values", () => {
    for (const name of ["brand-workbench", "brief-workbench"]) {
        const files = sample(name);
        const schema = JSON.parse(files["schemas/context.json"]);
        const markup = renderToStaticMarkup(<WorkbenchFields schema={schema} value={{ brand: "手改品牌", notes: "实际会议记录" }} suggested={{ brand: "建议品牌" }} disabled={false} onChange={() => undefined} onValidityChange={() => undefined} />);
        if (name === "brand-workbench") {
            expect(markup).toContain("手改品牌");
            expect(markup).not.toContain("建议品牌");
            expect(markup).toContain("目标人群");
        } else {
            expect(markup).toContain("实际会议记录");
            expect(markup).toContain("列出待确认风险");
        }
    }
});

// Only replace IndexedDB. Contract validation and draft recovery execute real code.
const dir = mkdtempSync(join(tmpdir(), "workbench-drafts-"));
writeFileSync(
    join(dir, "storage.ts"),
    `const stores = new Map(); export function localForageStorageForScope(scope) { if (!stores.has(scope)) stores.set(scope, new Map()); const store = stores.get(scope); return { getItem: async k => store.get(k), setItem: async (k,v) => store.set(k,v) }; }`,
);
writeFileSync(
    join(dir, "drafts.ts"),
    readFileSync(new URL("../src/services/workbench-drafts.ts", import.meta.url), "utf8")
        .replace('"@/lib/localforage-storage"', '"./storage"')
        .replace('"@/lib/plugins/plugin-v3-contract"', JSON.stringify(resolve(import.meta.dir, "../src/lib/plugins/plugin-v3-contract.ts"))),
);
const drafts = (await import(join(dir, "drafts.ts"))) as typeof import("../src/services/workbench-drafts");
afterAll(() => rmSync(dir, { recursive: true, force: true }));
test("draft, frozen Agent inputs and retry identity stay isolated by account/release/canvas", async () => {
    const selection = { id: "brand-workbench.compose", releaseId: "release-one", recipeIds: ["social"], input: { brand: "原始品牌" }, digest: "a".repeat(64) };
    const draft = {
        input: { brand: "新草稿" },
        recipes: ["social"],
        goal: "新目标",
        agent: selection,
        agentGoal: "原目标",
        pending: { key: "fixed-retry-key", request: { operation: "brand-workbench.run", releaseId: "release-one", input: selection.input, context: { hostSurface: "agent-home" as const }, workbench: selection } },
    };
    await drafts.saveWorkbenchDraft("alice", selection.id, "release-one", "", draft);
    expect(await drafts.loadWorkbenchDraft("alice", selection.id, "release-one", "")).toEqual(draft);
    expect(await drafts.loadWorkbenchDraft("bob", selection.id, "release-one", "")).toBeNull();
    expect(await drafts.loadWorkbenchDraft("alice", selection.id, "release-two", "")).toBeNull();
    expect(await drafts.loadWorkbenchDraft("alice", selection.id, "release-one", "canvas")).toBeNull();
    await drafts.saveWorkbenchDraft("alice", selection.id, "release-one", "", { ...draft, pending: { ...draft.pending, request: { ...draft.pending.request, context: { hostSurface: "canvas", canvasId: "foreign" } } } });
    await expect(drafts.loadWorkbenchDraft("alice", selection.id, "release-one", "")).rejects.toThrow("草稿损坏");
});
