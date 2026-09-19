import Ajv2020 from "ajv/dist/2020";
import schema from "../../../../backend/internal/plugins/contracts/schema.json";
import profile from "../../../../backend/internal/plugins/contracts/profile.json";

// Offline P00 contract only. Importing this module does not register a plugin,
// enable v3 installation, or grant execution permissions.
export { profile as pluginV3Profile };
export type ContractIssue = { reason: string; location: string };
export class PluginContractError extends Error {
    constructor(
        public readonly reason: string,
        public readonly location: string,
    ) {
        super(`${reason}: ${location}`);
    }
}
const fail = (reason: string, location: string): never => {
    throw new PluginContractError(reason, location);
};
const validator = new Ajv2020({ strict: false, validateFormats: false, allErrors: false, ownProperties: true });
validator.addSchema(schema);
const encoder = new TextEncoder();
type ObjectValue = Record<string, any>;
export type PluginTextPackage = Record<string, string>;

// JSON.parse silently overwrites duplicate keys. Parse structural tokens first
// so package admission has the same bounds/duplicate semantics as the server.
export function decodeContractJSON(raw: string): unknown {
    if (encoder.encode(raw).length > profile.limits.jsonBytes) fail("contract_invalid", "JSON size");
    let at = 0;
    const whitespace = () => {
        while (/[\x20\t\r\n]/.test(raw[at] ?? "!")) at++;
    };
    const string = (): string => {
        const start = at++;
        while (at < raw.length) {
            const c = raw[at++];
            if (c === "\\") at++;
            else if (c === '"') return JSON.parse(raw.slice(start, at));
        }
        throw new Error("unterminated string");
    };
    const parse = (depth: number): unknown => {
        if (depth > profile.limits.jsonDepth) throw new Error("depth");
        whitespace();
        const char = raw[at];
        if (char === '"') return string();
        if (char === "{") {
            at++;
            whitespace();
            const out: ObjectValue = Object.create(null);
            if (raw[at] === "}") {
                at++;
                return out;
            }
            while (at < raw.length) {
                whitespace();
                if (raw[at] !== '"') throw new Error("key");
                const key = string();
                if (Object.hasOwn(out, key)) throw new Error("duplicate");
                whitespace();
                if (raw[at++] !== ":") throw new Error("colon");
                out[key] = parse(depth + 1);
                whitespace();
                const end = raw[at++];
                if (end === "}") return out;
                if (end !== ",") throw new Error("delimiter");
            }
            throw new Error("object");
        }
        if (char === "[") {
            at++;
            whitespace();
            const out: unknown[] = [];
            if (raw[at] === "]") {
                at++;
                return out;
            }
            while (at < raw.length) {
                out.push(parse(depth + 1));
                whitespace();
                const end = raw[at++];
                if (end === "]") return out;
                if (end !== ",") throw new Error("delimiter");
            }
            throw new Error("array");
        }
        const token = /^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/.exec(raw.slice(at));
        if (!token) throw new Error("token");
        at += token[0].length;
        const value: unknown = JSON.parse(token[0]);
        if (typeof value === "number" && (!Number.isFinite(value) || Math.abs(value) > Number.MAX_SAFE_INTEGER)) throw new Error("number");
        return value;
    };
    try {
        const value = parse(0);
        whitespace();
        if (at !== raw.length) throw new Error("trailing JSON");
        return value;
    } catch {
        return fail("contract_invalid", "JSON syntax/depth/number");
    }
}

export function validatePluginContract(kind: string, raw: string): unknown {
    const value = decodeContractJSON(raw);
    if (!Object.hasOwn(schema.$defs, kind)) fail("contract_invalid", "unknown contract kind");
    const check = validator.getSchema(`${schema.$id}#/$defs/${kind}`);
    if (!check || !check(value)) fail("contract_invalid", kind);
    if (kind === "pipeline") validatePipeline(value as ObjectValue);
    return value;
}

function validPath(name: string): boolean {
    if (name === "manifest.json") return true;
    if (!/^(skills|operations|pipelines|schemas|views|connectors|blueprints)\/[A-Za-z0-9_./-]+$/.test(name)) return false;
    if (name.split("/").some((s) => !s || s === "." || s === "..")) return false;
    return name.endsWith(".json") || (name.startsWith("skills/") && name.endsWith(".md"));
}

export function validatePluginTextPackage(files: PluginTextPackage, reservedIDs: string[] = []): void {
    const names = Object.keys(files),
        docs: Record<string, ObjectValue> = Object.create(null);
    if (!names.length || names.length > profile.limits.maxFiles) fail("contract_invalid", "file count");
    let total = 0;
    for (const name of names) {
        if (!validPath(name)) fail("package_reference_invalid", name);
        const bytes = encoder.encode(files[name]).length;
        total += bytes;
        if (bytes > profile.limits.entryBytes || total > profile.limits.expandedBytes) fail("contract_invalid", "file size");
        if (name.endsWith(".json")) {
            const value = decodeContractJSON(files[name]);
            if (!value || typeof value !== "object" || Array.isArray(value)) fail("contract_invalid", name);
            docs[name] = value as ObjectValue;
        }
    }
    const manifest = validatePluginContract("manifest", files["manifest.json"] ?? "") as ObjectValue;
    const pluginID = manifest.id as string;
    if (pluginID === "qimu" || pluginID.startsWith("qimu-") || reservedIDs.includes(pluginID)) fail("scope_forbidden", pluginID);
    const dependencies = new Set<string>();
    for (const d of manifest.dependencies) {
        if (d.id === pluginID) fail("dependency_cycle", d.id);
        if (dependencies.has(d.id)) fail("contract_invalid", "duplicate dependency");
        dependencies.add(d.id);
    }
    const contributions = manifest.contributes;
    const registry: Record<string, Set<string>> = Object.create(null);
    for (const kind of ["skills", "operations", "views", "canvasBlueprints"]) {
        registry[kind] = new Set();
        for (const entry of contributions[kind] ?? []) {
            if (registry[kind].has(entry.id)) fail("contract_invalid", "duplicate contribution");
            registry[kind].add(entry.id);
            const target = kind === "skills" ? entry.entry : entry.ref;
            const root = kind === "canvasBlueprints" ? "blueprints" : kind;
            if (!Object.hasOwn(files, target) || !validPath(target) || !target.startsWith(`${root}/`)) fail("package_reference_invalid", target);
            if (kind === "skills") {
                if (!target.endsWith("/SKILL.md") || !files[target]) fail("contract_invalid", "skill entry");
                continue;
            }
            validatePluginContract(({ operations: "operation", views: "view", canvasBlueprints: "blueprint" } as Record<string, string>)[kind], files[target]);
            if (docs[target].id !== entry.id) fail("contract_invalid", "contribution id mismatch");
        }
    }
    const requireSchema = (name: string) => {
        if (!docs[name] || !name.startsWith("schemas/")) fail("package_reference_invalid", name);
    };
    for (const entry of contributions.operations ?? []) {
        const op = docs[entry.ref];
        requireSchema(op.inputSchemaRef);
        requireSchema(op.outputSchemaRef);
        for (const p of op.requiredPermissions) if (!manifest.permissions.includes(p)) fail("scope_forbidden", "permission exceeds manifest");
        if (op.resultView && !registry.views.has(op.resultView)) fail("package_reference_invalid", op.resultView);
        if (op.execution.kind !== "host" || !["resource.inspect", "resource.snapshot", "canvas.blueprint.instantiate"].includes(op.execution.adapter) || op.execution.mode !== "inline") fail("operation_unavailable", "host adapter profile");
        const snapshot = op.execution.adapter === "resource.snapshot";
        const projection = op.execution.adapter === "canvas.blueprint.instantiate";
        const permissions = projection ? op.requiredPermissions.includes("canvas.read") && op.requiredPermissions.includes("canvas.write") && op.context.requiresCanvas : op.requiredPermissions.includes("media.read") && (!snapshot || op.requiredPermissions.includes("resource.create"));
        if (!permissions || op.effects.length !== 1 || op.effects[0] !== (snapshot || projection ? "draft_write" : "read")) fail("scope_forbidden", "adapter minimum contract");
    }
    for (const entry of contributions.skills ?? [])
        for (const address of entry.operations) {
            const [id, local] = address.split(".");
            if (id === pluginID) {
                if (!registry.operations.has(local)) fail("package_reference_invalid", address);
            } else if (!dependencies.has(id)) fail("plugin_dependency_missing", address);
        }
    for (const entry of contributions.views ?? []) requireSchema(docs[entry.ref].schemaRef);
    for (const entry of contributions.canvasBlueprints ?? []) {
        const keys = new Set<string>();
        for (const node of docs[entry.ref].nodes) {
            if (keys.has(node.key)) fail("contract_invalid", "duplicate blueprint key");
            keys.add(node.key);
            if (!registry.views.has(node.view)) fail("package_reference_invalid", "blueprint view");
        }
    }
    validateUserSchemas(files, docs);
}

function validateUserSchemas(files: PluginTextPackage, docs: Record<string, ObjectValue>) {
    const marks = new Map<string, number>();
    const resolve = (current: string, reference: string): [string, string, ObjectValue] => {
        const parts = reference.split("#"),
            file = parts[0] || current,
            pointer = parts[1] ?? "";
        if (parts.length > 2 || !validPath(file) || !file.startsWith("schemas/") || (pointer && !pointer.startsWith("/"))) return fail("package_reference_invalid", reference);
        let value: any = docs[file];
        for (const part of pointer ? pointer.slice(1).split("/") : []) {
            if (/~(?:[^01]|$)/.test(part)) fail("package_reference_invalid", reference);
            const key = part.replace(/~1/g, "/").replace(/~0/g, "~");
            value = value && Object.hasOwn(value, key) ? value[key] : undefined;
        }
        if (!value || typeof value !== "object" || Array.isArray(value)) return fail("package_reference_invalid", reference);
        return [file, pointer, value];
    };
    const visit = (file: string, pointer: string, node: ObjectValue, depth: number) => {
        if (depth > profile.limits.jsonDepth) fail("contract_invalid", "schema reference depth");
        const key = `${file}#${pointer}`;
        if (marks.get(key) === 1) fail("schema_reference_cycle", key);
        if (marks.get(key) === 2) return;
        marks.set(key, 1);
        if (node.$ref) {
            const [f, p, n] = resolve(file, node.$ref);
            validatePluginContract("userSchema", JSON.stringify(n));
            visit(f, p, n, depth + 1);
        }
        for (const field of ["properties", "$defs"]) for (const [name, child] of Object.entries(node[field] ?? {})) visit(file, `${pointer}/${field}/${name.replace(/~/g, "~0").replace(/\//g, "~1")}`, child as ObjectValue, depth + 1);
        if (node.items) visit(file, `${pointer}/items`, node.items, depth + 1);
        marks.set(key, 2);
    };
    const rewrite = (file: string, node: ObjectValue): ObjectValue => {
        const out: ObjectValue = Object.create(null);
        for (const [k, v] of Object.entries(node)) {
            if (k === "$ref") {
                const [f, p] = resolve(file, v);
                out[k] = `https://qimu.invalid/package/${f}#${p}`;
            } else if (k === "properties" || k === "$defs") out[k] = Object.fromEntries(Object.entries(v).map(([name, child]) => [name, rewrite(file, child as ObjectValue)]));
            else if (k === "items") out[k] = rewrite(file, v);
            else out[k] = v;
        }
        return out;
    };
    const names = Object.keys(docs).filter((f) => f.startsWith("schemas/"));
    for (const f of names) validatePluginContract("userSchema", files[f]);
    for (const f of names) visit(f, "", docs[f], 0);
    const compiler = new Ajv2020({ strict: false, validateFormats: false, ownProperties: true });
    try {
        for (const f of names) compiler.addSchema(rewrite(f, docs[f]), `https://qimu.invalid/package/${f}`);
        for (const f of names) compiler.getSchema(`https://qimu.invalid/package/${f}`);
    } catch {
        fail("contract_invalid", "user schema");
    }
}

export function validateDependencyGraph(graph: Record<string, string[]>): void {
    if (Object.keys(graph).length > 256 || Object.values(graph).some((deps) => deps.length > 256)) fail("contract_invalid", "dependency graph limit");
    const marks = new Map<string, number>();
    const visit = (id: string) => {
        if (marks.get(id) === 1) fail("dependency_cycle", id);
        if (marks.get(id) === 2) return;
        if (!Object.hasOwn(graph, id)) fail("plugin_dependency_missing", id);
        marks.set(id, 1);
        for (const dep of graph[id]) visit(dep);
        marks.set(id, 2);
    };
    for (const id of Object.keys(graph)) visit(id);
}

function validatePipeline(doc: ObjectValue): void {
    const steps: Record<string, ObjectValue> = Object.create(null),
        graph: Record<string, string[]> = Object.create(null);
    for (const step of doc.steps) {
        if (steps[step.key]) fail("contract_invalid", "duplicate step");
        steps[step.key] = step;
        graph[step.key] = step.dependsOn;
    }
    validateDependencyGraph(graph);
    const check = (reference: string, allowed: Set<string>, allowItem: boolean) => {
        if (!/^(input#|item#|steps\/[a-z][a-z0-9-]*#)(\/.*)?$/.test(reference)) fail("contract_invalid", "binding syntax");
        if (/~(?:[^01]|$)/.test(reference.slice(reference.indexOf("#") + 1))) fail("contract_invalid", "binding pointer");
        if (reference.startsWith("item#") && !allowItem) fail("contract_invalid", "item outside foreach");
        if (reference.startsWith("steps/") && !allowed.has(reference.slice(6).split("#")[0])) fail("package_reference_invalid", "undeclared step dependency");
    };
    for (const step of Object.values(steps)) {
        const allowed = new Set<string>(step.dependsOn),
            hasItems = !!step.foreach;
        if (step.foreach) check(step.foreach.from, allowed, false);
        const bindings: ObjectValue[] = Object.values(step.inputs ?? {});
        if (step.when?.exists) check(step.when.exists, allowed, hasItems);
        for (const binding of step.when?.equals ?? []) bindings.push(binding);
        for (const binding of bindings) if (typeof binding.from === "string") check(binding.from, allowed, hasItems);
    }
    const all = new Set(Object.keys(steps));
    for (const binding of Object.values(doc.outputs) as ObjectValue[]) if (typeof binding.from === "string") check(binding.from, all, false);
}

async function sha256(text: string) {
    return Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", encoder.encode(text))), (b) => b.toString(16).padStart(2, "0")).join("");
}
export async function pluginPackageDigest(files: PluginTextPackage): Promise<string> {
    const records = await Promise.all(
        Object.keys(files)
            .sort()
            .map(async (name) => `${name}\0${await sha256(files[name])}\n`),
    );
    return sha256(`qimu-package-v1\n${records.join("")}`);
}
export async function pluginOperationDigest(packageDigest: string, operationID: string): Promise<string> {
    return sha256(`qimu-operation-v1\n${packageDigest}\n${operationID}\n`);
}
