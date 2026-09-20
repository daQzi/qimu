import { afterAll, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { tmpdir } from "node:os";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
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
for (const name of ["brand-points", "brand-script"])
    test(name + " uses the shared brand contract", () => {
        expect(() => validatePluginTextPackage(sample(name))).not.toThrow();
    });
test("host version, type family, permissions and bound fields are checked", () => {
    const cases: Array<[string, (v: any) => void]> = [
        [
            "manifest.json",
            (v) => {
                v.requires.hostApi = "^3.1.0";
            },
        ],
        [
            "manifest.json",
            (v) => {
                v.permissions = [];
            },
        ],
        [
            "operations/read.json",
            (v) => {
                v.requiredPermissions = [];
            },
        ],
        [
            "operations/read.json",
            (v) => {
                v.effects = ["draft_write"];
            },
        ],
        [
            "operations/read.json",
            (v) => {
                v.execution.adapter = "constructor";
            },
        ],
        [
            "workbenches/compose.json",
            (v) => {
                v.objectInputs = { reference: "product/v1" };
            },
        ],
        [
            "workbenches/compose.json",
            (v) => {
                v.objectInputs = { missing: "brand/v1" };
            },
        ],
        [
            "schemas/read-input.json",
            (v) => {
                v.required = [];
            },
        ],
    ];
    for (const [path, change] of cases) {
        const files = sample("brand-script");
        const v = JSON.parse(files[path]);
        change(v);
        files[path] = JSON.stringify(v);
        expect(() => validatePluginTextPackage(files)).toThrow();
    }
});
test("real workbench replaces a declared reference field with the shared picker", () => {
    const files = sample("brand-script");
    const schema = JSON.parse(files["schemas/read-input.json"]);
    const markup = renderToStaticMarkup(
        <MemoryRouter>
            <WorkbenchFields schema={schema} objectInputs={{ reference: "brand/v1" }} value={{}} suggested={{}} disabled={false} onChange={() => {}} onValidityChange={() => {}} />
        </MemoryRouter>,
    );
    expect(markup).toContain("选择品牌版本");
    expect(markup).toContain("管理品牌资料");
    expect(markup).not.toContain("请输入有效 JSON");
});

const dir = mkdtempSync(join(tmpdir(), "brand-pending-"));
writeFileSync(
    join(dir, "storage.ts"),
    `const stores=new Map();export function localForageStorageForScope(user){if(!stores.has(user))stores.set(user,new Map());const s=stores.get(user);return {getItem:async k=>s.get(k),setItem:async(k,v)=>s.set(k,v),removeItem:async k=>s.delete(k)}}`,
);
writeFileSync(join(dir, "pending.ts"), readFileSync(new URL("../src/services/business-object-pending.ts", import.meta.url), "utf8").replace('"@/lib/localforage-storage"', '"./storage"'));
const pending = (await import(join(dir, "pending.ts"))) as typeof import("../src/services/business-object-pending");
afterAll(() => rmSync(dir, { recursive: true, force: true }));
test("pending saves freeze request identity and remain account scoped", async () => {
    const request = { clientKey: "stable-request-key", expectedVersion: 0, brand: { name: "品牌", audience: "作者", positioning: "定位", claims: [], restrictions: [] } };
    await pending.saveObjectWrite("alice", request);
    request.brand.name = "后续编辑";
    expect((await pending.loadObjectWrite("alice"))?.brand.name).toBe("品牌");
    expect((await pending.loadObjectWrite("alice"))?.clientKey).toBe("stable-request-key");
    expect(await pending.loadObjectWrite("bob")).toBeNull();
    await pending.clearObjectWrite("bob");
    expect(await pending.loadObjectWrite("alice")).not.toBeNull();
    await pending.clearObjectWrite("alice");
    expect(await pending.loadObjectWrite("alice")).toBeNull();
    await pending.saveObjectWrite("alice", { ...request, brand: { ...request.brand, claims: [42] as any } });
    await expect(pending.loadObjectWrite("alice")).rejects.toThrow("记录损坏");
});
