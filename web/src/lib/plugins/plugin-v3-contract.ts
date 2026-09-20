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

function validateHTTPConnector(c: ObjectValue) {
    const name = /^[A-Za-z][A-Za-z0-9_-]{0,79}$/;
    const header = /^[A-Za-z][A-Za-z0-9-]{0,79}$/;
    const forbidden = (h: string) => ["authorization", "cookie", "host", "content-type", "content-length", "connection", "transfer-encoding", "proxy-authorization", "proxy-connection"].includes(h.toLowerCase());
    const pointer = (p: string) => p.startsWith("/") && p.length <= 500 && !/[\r\n]/.test(p);
    let base: URL;
    try {
        base = new URL(c.baseUrl);
    } catch {
        return fail("contract_invalid", "connector baseUrl");
    }
    if (base.protocol !== "https:" || !base.hostname || base.username || base.password || base.search || base.hash) fail("contract_invalid", "connector baseUrl");
    if (c.auth.type === "header" && (!header.test(c.auth.header || "") || forbidden(c.auth.header))) fail("contract_invalid", "auth header");
    if (c.auth.type !== "header" && c.auth.header) fail("contract_invalid", "unexpected auth header");
    for (const a of Object.values(c.actions) as ObjectValue[]) {
        const async = !!a.jobIdPath;
        if (async !== !!a.poll || async !== !!a.statusPath || async !== !!Object.keys(a.statusMap || {}).length) fail("contract_invalid", "async job contract");
        if (!async && (a.lookup || a.cancellation.mode !== "unsupported")) fail("contract_invalid", "sync recovery/cancellation");
        for (const p of [a.jobIdPath, a.statusPath, ...Object.values(a.outputs)] as string[]) if (p && !pointer(p)) fail("contract_invalid", "response pointer");
        if (a.artifact && (!pointer(a.artifact.urlPath) || !name.test(a.artifact.field) || Object.hasOwn(a.outputs, a.artifact.field))) fail("contract_invalid", "artifact mapping");
        for (const field of [...Object.keys(a.outputs), ...Object.keys(a.resources || {})]) if (!name.test(field)) fail("contract_invalid", "mapping field");
        if (a.idempotency.mode === "header" && (!header.test(a.idempotency.header || "") || forbidden(a.idempotency.header) || a.idempotency.header.toLowerCase() === (c.auth.header || "").toLowerCase() || !a.idempotency.retentionSeconds))
            fail("contract_invalid", "idempotency header/window");
        if (a.idempotency.mode === "unsupported" && a.idempotency.header) fail("contract_invalid", "unsupported idempotency header");
        if ((a.cancellation.mode === "request") !== !!a.cancellation.request) fail("contract_invalid", "cancel request");
        for (const [kind, r] of Object.entries({ submit: a.submit, poll: a.poll, lookup: a.lookup, cancel: a.cancellation.request }) as [string, ObjectValue][]) {
            if (!r) continue;
            if (!r.path.startsWith("/") || r.path.startsWith("//") || /[?#\\\r\n%]/.test(r.path) || r.path.includes("..")) fail("contract_invalid", "HTTP path");
            const token = kind === "lookup" ? "{submissionKey}" : kind === "submit" ? "" : "{jobId}";
            if (token && !r.path.includes(token)) fail("contract_invalid", "HTTP path variable");
            if (/[{}]/.test(token ? r.path.split(token).join("id") : r.path)) fail("contract_invalid", "HTTP path variable");
            if ((kind === "submit" && r.method !== "POST") || (["poll", "lookup"].includes(kind) && r.method !== "GET") || (kind === "cancel" && !["POST", "DELETE"].includes(r.method))) fail("contract_invalid", "HTTP method");
            if (kind !== "submit" && Object.keys(r.body || {}).length) fail("contract_invalid", "only submit maps body");
            for (const [field, b] of Object.entries(r.body || {}) as [string, ObjectValue][]) {
                if (!name.test(field)) fail("contract_invalid", "body field");
                if (Object.hasOwn(b, "literal")) continue;
                if (b.from?.startsWith("input.") && name.test(b.from.slice(6))) continue;
                if (b.from?.startsWith("resource.") && Object.hasOwn(a.resources || {}, b.from.slice(9))) continue;
                fail("contract_invalid", "unsupported mapping");
            }
        }
    }
}
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
    if (!/^(skills|operations|pipelines|schemas|views|connectors|blueprints|workbenches|recipes)\/[A-Za-z0-9_./-]+$/.test(name)) return false;
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
    for (const kind of ["skills", "operations", "views", "canvasBlueprints", "connectors", "pipelines", "workbenches", "recipes"]) {
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
            validatePluginContract(
                ({ operations: "operation", views: "view", canvasBlueprints: "blueprint", connectors: "httpConnector", pipelines: "pipeline", workbenches: "workbench", recipes: "recipe" } as Record<string, string>)[kind],
                files[target],
            );
            if (kind === "connectors") validateHTTPConnector(docs[target]);
            if (docs[target].id !== entry.id) fail("contract_invalid", "contribution id mismatch");
        }
    }
    const requireSchema = (name: string) => {
        if (!docs[name] || !name.startsWith("schemas/")) fail("package_reference_invalid", name);
    };
    const compiler = validateUserSchemas(files, docs);
    for (const entry of contributions.operations ?? []) {
        const op = docs[entry.ref];
        requireSchema(op.inputSchemaRef);
        requireSchema(op.outputSchemaRef);
        for (const p of op.requiredPermissions) if (!manifest.permissions.includes(p)) fail("scope_forbidden", "permission exceeds manifest");
        if (op.resultView && !registry.views.has(op.resultView)) fail("package_reference_invalid", op.resultView);
        if (op.execution.kind === "pipeline") {
            if (
                Object.entries(files)
                    .filter(([path]) => path.startsWith("schemas/"))
                    .reduce((total, [, raw]) => total + encoder.encode(raw).length, 0) > 24000
            )
                fail("operation_unavailable", "pipeline schema description exceeds 24000 bytes");
            const ref = contributions.pipelines?.find((r: ObjectValue) => r.id === op.execution.pipeline);
            if (!ref) fail("package_reference_invalid", "pipeline not registered");
            const p = docs[ref.ref];
            if (p.inputSchemaRef !== op.inputSchemaRef || p.outputSchemaRef !== op.outputSchemaRef) fail("contract_invalid", "pipeline entry schemas differ");
            const effects = new Set<string>(["draft_write"]);
            const permissions = new Set<string>();
            p.steps.forEach((step: ObjectValue) => {
                if (step.type === "wait_input") {
                    if (step.inputValidator) {
                        if (manifest.requires.hostApi !== "^3.3.0") fail("contract_invalid", "input validator requires hostApi ^3.3.0");
                        permissions.add("media.read");
                    }
                    if (step.view) {
                        const entry = contributions.views?.find((r: ObjectValue) => r.id === step.view);
                        const view = entry && docs[entry.ref];
                        if (!view) fail("package_reference_invalid", "input view");
                        if (view.schemaRef !== step.formSchemaRef || !["key-value/v1", "mapping-editor/v1", "video-report-editor/v1", "video-plan-editor/v1"].includes(view.component)) fail("contract_invalid", "input view schema/component");
                    }
                    requireSchema(step.formSchemaRef);
                    if (Object.keys(step.inputs || {}).length) {
                        if (manifest.requires.hostApi !== "^3.3.0") fail("contract_invalid", "input prefill requires hostApi ^3.3.0");
                        const schema = docs[step.formSchemaRef];
                        if (schema.type !== "object" || schema.$ref) fail("contract_invalid", "prefill requires explicit object schema");
                        for (const [field, binding] of Object.entries(step.inputs) as [string, ObjectValue][]) {
                            if (!/^[a-zA-Z][a-zA-Z0-9_]{0,79}$/.test(field) || !Object.hasOwn(schema.properties || {}, field)) fail("contract_invalid", "unknown prefill field");
                            if (Object.hasOwn(binding, "literal")) {
                                const validate = compiler.getSchema(`https://qimu.invalid/package/${step.formSchemaRef}#/properties/${field}`);
                                if (!validate || !validate(binding.literal)) fail("contract_invalid", "invalid prefill literal");
                            }
                        }
                    }
                    return;
                }
                const entry = contributions.operations?.find((r: ObjectValue) => pluginID + "." + r.id === step.operation);
                const child = entry && docs[entry.ref];
                if (!child || child.execution.kind === "pipeline") fail("operation_unavailable", "P05 steps must reference local non-pipeline operations");
                child.effects.forEach((e: string) => effects.add(e));
                child.requiredPermissions.forEach((r: string) => permissions.add(r));
                if ((child.context.requiresCanvas && !op.context.requiresCanvas) || (child.context.requiresProject && !op.context.requiresProject)) fail("scope_forbidden", "pipeline context weaker than step");
            });
            if ([...effects].some((e) => !op.effects.includes(e)) || [...permissions].some((r) => !op.requiredPermissions.includes(r))) fail("scope_forbidden", "pipeline must declare aggregate effects and permissions");
            continue;
        }
        if (op.execution.kind === "model") {
            if (manifest.requires.hostApi !== "^3.3.0") fail("contract_invalid", "model operation requires hostApi ^3.3.0");
            if (!op.requiredPermissions.includes("generation.run") || (Object.keys(op.execution.resources).length > 0 && !op.requiredPermissions.includes("media.read")) || op.effects.length !== 1 || op.effects[0] !== "generation")
                fail("scope_forbidden", "model operation minimum contract");
            if (op.execution.outputProfile === "video-report/v1" && (Object.keys(op.execution.resources).length !== 1 || op.execution.resources.resourceId !== "video")) fail("contract_invalid", "video report requires resourceId video");
            continue;
        }
        if (op.execution.kind === "http") {
            const ref = contributions.connectors?.find((r: ObjectValue) => r.id === op.execution.connector);
            if (!ref) fail("operation_unavailable", "HTTP connector not registered");
            const actions = docs[ref.ref].actions;
            const action = Object.hasOwn(actions, op.execution.action) ? actions[op.execution.action] : undefined;
            if (!action) fail("package_reference_invalid", "connector action");
            if (
                !op.requiredPermissions.includes("connection.use") ||
                (Object.keys(action.resources || {}).length && !op.requiredPermissions.includes("media.read")) ||
                (action.artifact && !op.requiredPermissions.includes("resource.create")) ||
                op.effects.length !== 1 ||
                !["external_write", "generation"].includes(op.effects[0])
            )
                fail("scope_forbidden", "HTTP operation effects and permissions");
            continue;
        }
        const objectPermissions: Record<string, string> = { "object.read": "asset.read", "object.search": "asset.search", "object.save": "asset.import" };
        const objectPermission = Object.hasOwn(objectPermissions, op.execution.adapter) ? objectPermissions[op.execution.adapter] : undefined;
        if (objectPermission) {
            if (op.execution.kind !== "host" || op.execution.mode !== "inline" || !["^3.2.0", "^3.3.0"].includes(manifest.requires.hostApi)) fail("operation_unavailable", "object host contract");
            if (!op.requiredPermissions.includes(objectPermission) || op.effects.length !== 1 || op.effects[0] !== (op.execution.adapter === "object.save" ? "draft_write" : "read")) fail("scope_forbidden", "object minimum contract");
            continue;
        }
        if (op.execution.kind !== "host" || !["resource.inspect", "resource.snapshot", "canvas.blueprint.instantiate"].includes(op.execution.adapter) || op.execution.mode !== "inline") fail("operation_unavailable", "host adapter profile");
        const snapshot = op.execution.adapter === "resource.snapshot";
        const projection = op.execution.adapter === "canvas.blueprint.instantiate";
        const permissions = projection
            ? op.requiredPermissions.includes("canvas.read") && op.requiredPermissions.includes("canvas.write") && op.context.requiresCanvas
            : op.requiredPermissions.includes("media.read") && (!snapshot || op.requiredPermissions.includes("resource.create"));
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
        const blueprint = docs[entry.ref];
        if (blueprint.nodes.length + (blueprint.connections?.length || 0) > 20) fail("contract_invalid", "blueprint exceeds 20 canvas mutations");
        const keys = new Set<string>();
        for (const node of docs[entry.ref].nodes) {
            if (node.binding !== blueprint.nodes[0].binding) fail("contract_invalid", "one blueprint binds one input request or one successful result");
            if (keys.has(node.key)) fail("contract_invalid", "duplicate blueprint key");
            keys.add(node.key);
            if (!registry.views.has(node.view)) fail("package_reference_invalid", "blueprint view");
            const view = docs[contributions.views.find((r: ObjectValue) => r.id === node.view).ref];
            if ((node.nodeType === "plugin-input") !== (node.binding === "input")) fail("contract_invalid", "node and binding kinds differ");
            if (node.binding === "input" && !["key-value/v1", "mapping-editor/v1", "video-report-editor/v1", "video-plan-editor/v1"].includes(view.component)) fail("contract_invalid", "input component is not editable");
            for (const action of node.actions || []) if (action !== "editor.focus" && !(action === "input.submit" && node.binding === "input") && !(action === "result.continue" && node.binding === "result")) fail("contract_invalid", "node action");
        }
        const edges = new Set<string>();
        for (const edge of blueprint.connections || []) {
            const key = `${edge.from}:${edge.to}`;
            if (!keys.has(edge.from) || !keys.has(edge.to) || edge.from === edge.to || edges.has(key)) fail("contract_invalid", "invalid blueprint connection");
            edges.add(key);
        }
    }
    const skills = contributions.skills || [];
    const boards = contributions.workbenches || [];
    const recipes = contributions.recipes || [];
    if ((boards.length || recipes.length || skills.some((s: ObjectValue) => s.launchOperation)) && !["^3.1.0", "^3.2.0", "^3.3.0"].includes(manifest.requires.hostApi)) fail("contract_invalid", "workbench requires hostApi ^3.1.0");
    const operation = (address: string) => {
        const ref = (contributions.operations || []).find((r: ObjectValue) => address === pluginID + "." + r.id);
        return ref ? docs[ref.ref] : undefined;
    };
    for (const skill of skills)
        if (skill.launchOperation) {
            if (!operation(skill.launchOperation)) fail("package_reference_invalid", "skill launch operation");
            if (!skill.operations.includes(skill.launchOperation)) fail("scope_forbidden", "skill launch must be declared");
        }
    for (const ref of boards) {
        const board = docs[ref.ref];
        requireSchema(board.contextSchemaRef);
        const schema = docs[board.contextSchemaRef];
        if (schema.type !== "object" || schema.$ref) fail("contract_invalid", "workbench requires object schema");
        if (Object.keys(board.objectInputs || {}).length) {
            if (!["^3.2.0", "^3.3.0"].includes(manifest.requires.hostApi)) fail("contract_invalid", "object inputs require hostApi ^3.2.0");
            if (!manifest.permissions.includes("asset.read")) fail("scope_forbidden", "object inputs require asset.read");
            for (const key of Object.keys(board.objectInputs)) {
                if (!Object.hasOwn(schema.properties || {}, key)) fail("package_reference_invalid", "object input field");
                if (!schema.required?.includes(key)) fail("contract_invalid", "object input must be required");
            }
        }
        if (Object.keys(schema.properties || {}).length > 32 || Object.keys(schema.properties || {}).some((key) => !/^[a-zA-Z][a-zA-Z0-9_]{0,79}$/.test(key))) fail("contract_invalid", "workbench fields");
        if (board.operation && operation(board.operation)?.inputSchemaRef !== board.contextSchemaRef) fail("package_reference_invalid", "workbench input schema");
        if (board.skill) {
            const skill = skills.find((s: ObjectValue) => s.id === board.skill);
            if (!skill) fail("package_reference_invalid", "workbench skill");
            if (skill.launchOperation && skill.launchOperation !== board.operation) fail("contract_invalid", "workbench skill launch mismatch");
        }
        const suggestions = [board.defaults];
        for (const id of board.recipes) {
            const recipe = recipes.find((r: ObjectValue) => r.id === id);
            if (!recipe) fail("package_reference_invalid", "workbench recipe");
            suggestions.push(docs[recipe.ref].defaults, docs[recipe.ref].requirements);
        }
        for (const values of suggestions)
            for (const [key, value] of Object.entries(values)) {
                if (!Object.hasOwn(schema.properties || {}, key)) fail("contract_invalid", "unknown recipe field");
                const validate = compiler.getSchema(`https://qimu.invalid/package/${board.contextSchemaRef}#/properties/${key}`);
                if (!validate || !validate(value)) fail("contract_invalid", "recipe field: " + key);
            }
    }
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
    return compiler;
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
        if (step.foreach) {
            check(step.foreach.from, allowed, false);
            check("item#" + step.foreach.itemKey, allowed, true);
        }
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
